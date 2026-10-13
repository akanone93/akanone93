// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//! Native test support (`FN-030`): a scriptable in-memory host and builders
//! that hand out contexts, so a function is unit-tested with `cargo test`.

use std::cell::RefCell;
use std::collections::BTreeMap;
use std::ops::Deref;
use std::rc::Rc;

use serde::Serialize;
use serde_json::{Map, Value, json};

use crate::context::{DeviceContext, ServerContext};
use crate::error::{BusinessError, HostError};
use crate::host::{Host, Level, Placement};
use crate::http::{HttpRequest, HttpResponse};

type InvokeHandler = Box<dyn Fn(&str, &[u8]) -> Result<Vec<u8>, HostError>>;

struct State {
    placement: Placement,
    app: String,
    environment: String,
    assurance_level: String,
    now: i64,
    env: BTreeMap<String, String>,
    kv: BTreeMap<String, Vec<u8>>,
    logs: Vec<(Level, String)>,
    secrets: BTreeMap<String, String>,
    claims: Option<Map<String, Value>>,
    http: BTreeMap<String, Result<HttpResponse, HostError>>,
    http_requests: Vec<HttpRequest>,
    invoke: Option<InvokeHandler>,
}

/// An in-memory [`Host`]: key-value store, fixed clock, captured logs,
/// scripted HTTP responses, secrets, claims and a nested-invoke handler.
/// Clones share state. Server-only calls answer `CapabilityDenied` on a
/// device host, as the real one does.
#[derive(Clone)]
pub struct MockHost {
    state: Rc<RefCell<State>>,
}

impl MockHost {
    /// A host for `placement` with fixed identifiers and clock.
    pub fn new(placement: Placement) -> Self {
        Self {
            state: Rc::new(RefCell::new(State {
                placement,
                app: "app-test".to_owned(),
                environment: "env-test".to_owned(),
                assurance_level: "standard".to_owned(),
                now: 1_700_000_000_000_000_000,
                env: BTreeMap::new(),
                kv: BTreeMap::new(),
                logs: Vec::new(),
                secrets: BTreeMap::new(),
                claims: None,
                http: BTreeMap::new(),
                http_requests: Vec::new(),
                invoke: None,
            })),
        }
    }

    /// Sets the app identifier.
    pub fn set_app(&self, app: &str) -> &Self {
        self.state.borrow_mut().app = app.to_owned();
        self
    }

    /// Sets the environment identifier.
    pub fn set_environment(&self, environment: &str) -> &Self {
        self.state.borrow_mut().environment = environment.to_owned();
        self
    }

    /// Sets the assurance level.
    pub fn set_assurance_level(&self, level: &str) -> &Self {
        self.state.borrow_mut().assurance_level = level.to_owned();
        self
    }

    /// Fixes the clock (Unix nanoseconds).
    pub fn set_now(&self, unix_nanos: i64) -> &Self {
        self.state.borrow_mut().now = unix_nanos;
        self
    }

    /// Sets an environment value.
    pub fn set_env(&self, name: &str, value: &str) -> &Self {
        self.state
            .borrow_mut()
            .env
            .insert(name.to_owned(), value.to_owned());
        self
    }

    /// Seeds a key-value entry.
    pub fn set_kv(&self, key: &str, value: &[u8]) -> &Self {
        self.state
            .borrow_mut()
            .kv
            .insert(key.to_owned(), value.to_vec());
        self
    }

    /// Grants a secret.
    pub fn set_secret(&self, name: &str, value: &str) -> &Self {
        self.state
            .borrow_mut()
            .secrets
            .insert(name.to_owned(), value.to_owned());
        self
    }

    /// Sets the verified claims; without this call `claims` returns `NotFound`.
    pub fn set_claims(&self, claims: Value) -> &Self {
        let map = match claims {
            Value::Object(map) => map,
            _ => Map::new(),
        };
        self.state.borrow_mut().claims = Some(map);
        self
    }

    /// Scripts the response for an exact URL. An unscripted URL is
    /// `EgressRefused`, the deny-by-default behaviour of the real host.
    pub fn respond(&self, url: &str, response: HttpResponse) -> &Self {
        self.state
            .borrow_mut()
            .http
            .insert(url.to_owned(), Ok(response));
        self
    }

    /// Scripts a failure for an exact URL.
    pub fn respond_error(&self, url: &str, error: HostError) -> &Self {
        self.state
            .borrow_mut()
            .http
            .insert(url.to_owned(), Err(error));
        self
    }

    /// Installs the nested-invoke handler: `(name, input JSON)` to
    /// `tag || JSON` bytes (see [`output_reply`] and [`business_reply`]).
    pub fn on_invoke(
        &self,
        handler: impl Fn(&str, &[u8]) -> Result<Vec<u8>, HostError> + 'static,
    ) -> &Self {
        self.state.borrow_mut().invoke = Some(Box::new(handler));
        self
    }

    /// The key-value entry at `key`, if any.
    pub fn kv(&self, key: &str) -> Option<Vec<u8>> {
        self.state.borrow().kv.get(key).cloned()
    }

    /// Captured log lines in order.
    pub fn logs(&self) -> Vec<(Level, String)> {
        self.state.borrow().logs.clone()
    }

    /// Captured HTTP requests in order.
    pub fn http_requests(&self) -> Vec<HttpRequest> {
        self.state.borrow().http_requests.clone()
    }

    /// A `'static` handle sharing this host's state, for `__plux_entry_with`.
    /// It leaks a few bytes per call, which is fine in a test process.
    pub fn static_host(&self) -> &'static MockHost {
        Box::leak(Box::new(self.clone()))
    }

    fn server_only(&self) -> Result<(), HostError> {
        match self.state.borrow().placement {
            Placement::Server => Ok(()),
            Placement::Device => Err(HostError::CapabilityDenied),
        }
    }
}

impl Host for MockHost {
    fn log(&self, level: Level, message: &str) {
        self.state
            .borrow_mut()
            .logs
            .push((level, message.to_owned()));
    }

    fn clock_now(&self) -> Result<i64, HostError> {
        Ok(self.state.borrow().now)
    }

    fn context(&self) -> Result<Vec<u8>, HostError> {
        let state = self.state.borrow();
        let placement = match state.placement {
            Placement::Server => "server",
            Placement::Device => "device",
        };
        Ok(json!({
            "app": state.app,
            "environment": state.environment,
            "placement": placement,
            "assuranceLevel": state.assurance_level,
        })
        .to_string()
        .into_bytes())
    }

    fn env_get(&self, name: &str) -> Result<Vec<u8>, HostError> {
        let state = self.state.borrow();
        state
            .env
            .get(name)
            .map(|v| v.clone().into_bytes())
            .ok_or(HostError::NotFound)
    }

    fn kv_get(&self, key: &str) -> Result<Vec<u8>, HostError> {
        self.state
            .borrow()
            .kv
            .get(key)
            .cloned()
            .ok_or(HostError::NotFound)
    }

    fn kv_put(&self, key: &str, value: &[u8]) -> Result<(), HostError> {
        self.state
            .borrow_mut()
            .kv
            .insert(key.to_owned(), value.to_vec());
        Ok(())
    }

    fn kv_delete(&self, key: &str) -> Result<(), HostError> {
        self.state
            .borrow_mut()
            .kv
            .remove(key)
            .map(|_| ())
            .ok_or(HostError::NotFound)
    }

    fn http_request(&self, request: &[u8]) -> Result<Vec<u8>, HostError> {
        self.server_only()?;
        let request: HttpRequest =
            serde_json::from_slice(request).map_err(|_| HostError::InvalidArgument)?;
        let mut state = self.state.borrow_mut();
        let scripted = state.http.get(&request.url).cloned();
        state.http_requests.push(request);
        match scripted {
            Some(Ok(response)) => {
                serde_json::to_vec(&response).map_err(|_| HostError::UpstreamFailed)
            }
            Some(Err(error)) => Err(error),
            None => Err(HostError::EgressRefused),
        }
    }

    fn secret_get(&self, name: &str) -> Result<Vec<u8>, HostError> {
        self.server_only()?;
        let state = self.state.borrow();
        state
            .secrets
            .get(name)
            .map(|v| v.clone().into_bytes())
            .ok_or(HostError::NotFound)
    }

    fn claims(&self) -> Result<Vec<u8>, HostError> {
        self.server_only()?;
        match &self.state.borrow().claims {
            Some(map) => serde_json::to_vec(map).map_err(|_| HostError::Unavailable),
            None => Err(HostError::NotFound),
        }
    }

    fn invoke_function(&self, name: &str, input: &[u8]) -> Result<Vec<u8>, HostError> {
        self.server_only()?;
        match &self.state.borrow().invoke {
            Some(handler) => handler(name, input),
            None => Err(HostError::NotFound),
        }
    }
}

/// `tag 0 || JSON` for an `on_invoke` handler.
pub fn output_reply(output: &impl Serialize) -> Vec<u8> {
    let mut reply = vec![crate::rt::TAG_OUTPUT];
    reply.extend(serde_json::to_vec(output).unwrap_or_default());
    reply
}

/// `tag 1 || JSON` for an `on_invoke` handler.
pub fn business_reply(error: &BusinessError) -> Vec<u8> {
    let mut reply = vec![crate::rt::TAG_BUSINESS];
    reply.extend(serde_json::to_vec(error).unwrap_or_default());
    reply
}

/// Hands out [`DeviceContext`]s over a [`MockHost`]. Derefs to the host, so
/// `device.set_kv(…)` configures it.
pub struct TestDevice {
    host: MockHost,
}

impl TestDevice {
    /// A device host with default identifiers.
    pub fn new() -> Self {
        Self {
            host: MockHost::new(Placement::Device),
        }
    }

    /// The underlying host.
    pub fn host(&self) -> &MockHost {
        &self.host
    }

    /// A context for the handler under test.
    pub fn context(&self) -> DeviceContext<'_> {
        DeviceContext::new(&self.host).expect("the mock host always has a context")
    }
}

impl Default for TestDevice {
    fn default() -> Self {
        Self::new()
    }
}

impl Deref for TestDevice {
    type Target = MockHost;

    fn deref(&self) -> &MockHost {
        &self.host
    }
}

/// Hands out [`ServerContext`]s over a [`MockHost`].
pub struct TestServer {
    host: MockHost,
}

impl TestServer {
    /// A server host with default identifiers.
    pub fn new() -> Self {
        Self {
            host: MockHost::new(Placement::Server),
        }
    }

    /// The underlying host.
    pub fn host(&self) -> &MockHost {
        &self.host
    }

    /// A context for the handler under test.
    pub fn context(&self) -> ServerContext<'_> {
        ServerContext::new(&self.host).expect("the mock host always has a context")
    }
}

impl Default for TestServer {
    fn default() -> Self {
        Self::new()
    }
}

impl Deref for TestServer {
    type Target = MockHost;

    fn deref(&self) -> &MockHost {
        &self.host
    }
}

/// How an entry reply decodes.
#[derive(Debug, Clone, PartialEq)]
pub enum Reply {
    /// Tag 0: the output.
    Output(Value),
    /// Tag 1: a business error document.
    Business(Value),
    /// Tag 2: the guest refused its input.
    Refused(Value),
    /// Tag 3: the guest could not encode its output.
    EncodeFailed(Value),
}

/// Decodes `tag || JSON`; panics on a malformed reply (a test failure).
pub fn decode_reply(reply: &[u8]) -> Reply {
    let (tag, body) = reply.split_first().expect("empty reply");
    let body: Value = serde_json::from_slice(body).expect("reply body is JSON");
    match tag {
        0 => Reply::Output(body),
        1 => Reply::Business(body),
        2 => Reply::Refused(body),
        3 => Reply::EncodeFailed(body),
        other => panic!("unknown reply tag {other}"),
    }
}

/// Replays the `tests` of a function document: every case runs `run` (which
/// receives the case and its input bytes and returns the entry reply) and
/// compares the output, or the business error code, with the stored one.
/// Authors and the examples use it so cases and code cannot drift (`FN-030`).
pub fn replay(document: &str, mut run: impl FnMut(&Value, &[u8]) -> Vec<u8>) -> usize {
    let document: Value = serde_json::from_str(document).expect("function.json is JSON");
    let cases = document["tests"]
        .as_array()
        .expect("function.json has tests");
    for case in cases {
        let name = case["name"].as_str().expect("case name");
        let input = serde_json::to_vec(&case["input"]).expect("case input");
        let reply = decode_reply(&run(case, &input));
        if let Some(code) = case.get("error").map(|e| &e["code"]) {
            match reply {
                Reply::Business(body) => assert_eq!(&body["code"], code, "case {name}"),
                other => panic!("case {name}: expected business error {code}, got {other:?}"),
            }
        } else {
            match reply {
                Reply::Output(body) => assert_eq!(body, case["output"], "case {name}"),
                other => panic!("case {name}: expected output, got {other:?}"),
            }
        }
    }
    cases.len()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::Level;

    #[test]
    fn kv_round_trips_and_missing_is_none() {
        let device = TestDevice::new();
        let ctx = device.context();
        assert_eq!(ctx.kv().get("a").unwrap(), None);
        ctx.kv().put("a", b"1").unwrap();
        assert_eq!(ctx.kv().get("a").unwrap(), Some(b"1".to_vec()));
        ctx.kv().put_json("j", &json!({"x": 1})).unwrap();
        assert_eq!(
            ctx.kv().get_json::<Value>("j").unwrap(),
            Some(json!({"x": 1}))
        );
        ctx.kv().delete("a").unwrap();
        ctx.kv().delete("a").unwrap();
        assert_eq!(device.kv("a"), None);
    }

    #[test]
    fn context_exposes_identifiers_clock_env_and_logs() {
        let device = TestDevice::new();
        device
            .set_app("a1")
            .set_environment("e1")
            .set_now(42)
            .set_env("MODE", "x");
        let ctx = device.context();
        assert_eq!((ctx.app_id(), ctx.environment_id()), ("a1", "e1"));
        assert_eq!(ctx.assurance_level(), "standard");
        assert_eq!(ctx.now().unwrap(), 42);
        assert_eq!(ctx.env("MODE").unwrap().as_deref(), Some("x"));
        assert_eq!(ctx.env("OTHER").unwrap(), None);
        ctx.warn("careful");
        assert_eq!(device.logs(), vec![(Level::Warn, "careful".to_owned())]);
    }

    #[test]
    fn a_device_host_denies_server_capabilities() {
        let device = TestDevice::new();
        assert_eq!(device.claims(), Err(HostError::CapabilityDenied));
        assert_eq!(device.secret_get("s"), Err(HostError::CapabilityDenied));
        assert_eq!(device.http_request(b"{}"), Err(HostError::CapabilityDenied));
        assert_eq!(
            device.invoke_function("f", b"{}"),
            Err(HostError::CapabilityDenied)
        );
    }

    #[test]
    fn server_context_reaches_claims_secrets_and_http() {
        let server = TestServer::new();
        server
            .set_claims(json!({"tier": "premium"}))
            .set_secret("key", "s3");
        server.respond(
            "https://api.test/x",
            HttpResponse {
                status: 200,
                headers: BTreeMap::new(),
                body: b"ok".to_vec(),
            },
        );
        let ctx = server.context();
        assert_eq!(ctx.claims().unwrap().get_str("tier"), Some("premium"));
        assert_eq!(ctx.secret("key").unwrap(), "s3");
        assert_eq!(ctx.secret("none"), Err(HostError::NotFound));
        let response = ctx
            .http()
            .send(&HttpRequest::get("https://api.test/x"))
            .unwrap();
        assert_eq!(
            (response.status, response.body.as_slice()),
            (200, b"ok".as_slice())
        );
        assert_eq!(
            ctx.http().send(&HttpRequest::get("https://other.test/")),
            Err(HostError::EgressRefused)
        );
        assert_eq!(server.http_requests().len(), 2);
    }

    #[test]
    fn missing_claims_are_not_found() {
        assert_eq!(
            TestServer::new().context().claims(),
            Err(HostError::NotFound)
        );
    }

    #[test]
    fn nested_invoke_decodes_output_and_business_error() {
        let server = TestServer::new();
        server.on_invoke(|name, input| match name {
            "ok" => Ok(output_reply(
                &json!({"echo": serde_json::from_slice::<Value>(input).unwrap()}),
            )),
            "biz" => Ok(business_reply(&BusinessError::new("NOPE", "k"))),
            _ => Err(HostError::NotFound),
        });
        let ctx = server.context();
        let out: Value = ctx.invoke("ok", &json!(1)).unwrap();
        assert_eq!(out, json!({"echo": 1}));
        let err = ctx.invoke::<_, Value>("biz", &json!(1)).unwrap_err();
        assert_eq!(err, InvokeErrorProbe::business("NOPE"));
        let err = ctx.invoke::<_, Value>("missing", &json!(1)).unwrap_err();
        assert_eq!(err, crate::InvokeError::System(HostError::NotFound));
    }

    struct InvokeErrorProbe;
    impl InvokeErrorProbe {
        fn business(code: &str) -> crate::InvokeError {
            crate::InvokeError::Business(BusinessError::new(code, "k"))
        }
    }
}
