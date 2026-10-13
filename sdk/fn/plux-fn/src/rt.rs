// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//! Runtime support for the macros. Not a stable API.

use serde::Serialize;
use serde::de::DeserializeOwned;
use serde_json::json;

use crate::context::ContextKind;
use crate::error::BusinessError;
use crate::host::Host;
use schemars::JsonSchema;

/// Result tags (ADR-0062).
pub const TAG_OUTPUT: u8 = 0;
/// A business error `{code, messageKey, params}`.
pub const TAG_BUSINESS: u8 = 1;
/// The guest refused its input (`PLX-7008`).
pub const TAG_REFUSED: u8 = 2;
/// The guest could not encode its output (`PLX-7009`).
pub const TAG_ENCODE_FAILED: u8 = 3;

/// A function's ABI entry point.
pub type EntryFn = fn(&[u8]) -> Vec<u8>;

/// A function's schemas document producer.
pub type SchemasFn = fn() -> String;

/// Longest panic message the hook logs.
pub const PANIC_LOG_BYTES: usize = 1024;

fn tagged(tag: u8, mut json: Vec<u8>) -> Vec<u8> {
    json.insert(0, tag);
    json
}

fn message(text: &str) -> Vec<u8> {
    json!({ "message": text }).to_string().into_bytes()
}

/// Runs a handler against the module's own host.
pub fn entry<C, I, O, F>(input: &[u8], handler: &F) -> Vec<u8>
where
    C: ContextKind,
    I: DeserializeOwned,
    O: Serialize,
    F: Fn(&C, I) -> Result<O, BusinessError>,
{
    entry_with(C::abi_host(), input, handler)
}

/// Runs a handler against `host`: decode, call, tag.
pub fn entry_with<C, I, O, F>(host: &'static dyn Host, input: &[u8], handler: &F) -> Vec<u8>
where
    C: ContextKind,
    I: DeserializeOwned,
    O: Serialize,
    F: Fn(&C, I) -> Result<O, BusinessError>,
{
    let input: I = match serde_json::from_slice(input) {
        Ok(value) => value,
        Err(error) => return tagged(TAG_REFUSED, message(&error.to_string())),
    };
    let context = C::from_host(host);
    let (tag, encoded) = match handler(&context, input) {
        Ok(output) => (TAG_OUTPUT, serde_json::to_vec(&output)),
        Err(error) => (TAG_BUSINESS, serde_json::to_vec(&error)),
    };
    match encoded {
        Ok(json) => tagged(tag, json),
        Err(error) => tagged(TAG_ENCODE_FAILED, message(&error.to_string())),
    }
}

/// The schemas document of a handler, `{"input": …, "output": …}` (`FN-004`).
pub fn schemas<C, I, O, F>(_handler: &F) -> String
where
    C: ContextKind,
    I: JsonSchema,
    O: JsonSchema,
    F: Fn(&C, I) -> Result<O, BusinessError>,
{
    let input = schemars::SchemaGenerator::default().into_root_schema_for::<I>();
    let output = schemars::SchemaGenerator::default().into_root_schema_for::<O>();
    json!({ "input": input, "output": output }).to_string()
}

/// Cuts a panic message to [`PANIC_LOG_BYTES`] on a character boundary.
pub fn bounded_panic_message(text: &str) -> &str {
    if text.len() <= PANIC_LOG_BYTES {
        return text;
    }
    let mut end = PANIC_LOG_BYTES;
    while !text.is_char_boundary(end) {
        end -= 1;
    }
    &text[..end]
}

#[cfg(target_arch = "wasm32")]
pub use wasm::*;

#[cfg(target_arch = "wasm32")]
#[allow(unsafe_code)]
mod wasm {
    use std::sync::atomic::{AtomicBool, Ordering};

    use crate::abi;
    use crate::host::{Host, Level};

    /// The host of a device module: server imports are never referenced.
    pub fn device_host() -> &'static dyn Host {
        &abi::DeviceAbi
    }

    /// The host of a server module.
    pub fn server_host() -> &'static dyn Host {
        &abi::ServerAbi
    }

    /// `plux_alloc`: memory for a host-written buffer. Nothing is freed; the
    /// instance is discarded after one call (`FN-010`).
    pub fn alloc(len: i32) -> i32 {
        let len = usize::try_from(len).unwrap_or(0);
        Box::leak(vec![0u8; len].into_boxed_slice()).as_mut_ptr() as usize as i32
    }

    /// Packs a result buffer as `(ptr << 32) | len`, leaking it to the host.
    pub fn pack(bytes: Vec<u8>) -> i64 {
        let len = bytes.len() as u64;
        let ptr = Box::leak(bytes.into_boxed_slice()).as_ptr() as usize as u64;
        ((ptr << 32) | len) as i64
    }

    /// Logs a panic message at error level before the abort.
    pub fn install_panic_hook() {
        static INSTALLED: AtomicBool = AtomicBool::new(false);
        if !INSTALLED.swap(true, Ordering::Relaxed) {
            std::panic::set_hook(Box::new(|info| {
                let text = info.to_string();
                abi::log_line(Level::Error, super::bounded_panic_message(&text));
            }));
        }
    }

    /// `plux_invoke`: dispatch by position; an unknown index traps.
    pub fn invoke(table: &[super::EntryFn], index: i32, ptr: i32, len: i32) -> i64 {
        install_panic_hook();
        let Some(entry) = usize::try_from(index).ok().and_then(|i| table.get(i)) else {
            core::arch::wasm32::unreachable()
        };
        let input: &[u8] = if len <= 0 {
            &[]
        } else {
            // SAFETY: the host wrote `len` bytes at `ptr` into memory
            // obtained from `plux_alloc`, and the instance is single-threaded.
            unsafe { core::slice::from_raw_parts(ptr as u32 as usize as *const u8, len as usize) }
        };
        pack(entry(input))
    }

    /// `plux_schemas`: the schemas document of the function at `index`.
    pub fn schemas_of_index(table: &[super::SchemasFn], index: i32) -> i64 {
        let Some(schemas) = usize::try_from(index).ok().and_then(|i| table.get(i)) else {
            core::arch::wasm32::unreachable()
        };
        pack(schemas().into_bytes())
    }
}

#[cfg(not(target_arch = "wasm32"))]
pub use native::*;

#[cfg(not(target_arch = "wasm32"))]
mod native {
    use crate::host::{Host, UnavailableHost};

    /// Native builds have no module host.
    pub fn device_host() -> &'static dyn Host {
        &UnavailableHost
    }

    /// Native builds have no module host.
    pub fn server_host() -> &'static dyn Host {
        &UnavailableHost
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::testing::TestDevice;
    use crate::{DeviceContext, Placement};
    use serde::Deserialize;

    #[derive(Deserialize, JsonSchema)]
    struct In {
        n: u32,
    }

    #[derive(Serialize, JsonSchema)]
    struct Out {
        doubled: u32,
    }

    fn handler(ctx: &DeviceContext, input: In) -> Result<Out, BusinessError> {
        ctx.info("called");
        match input.n {
            0 => Err(BusinessError::new("ZERO", "test.zero").with_param("n", 0)),
            n => Ok(Out { doubled: n * 2 }),
        }
    }

    crate::function!(handler);

    fn run(device: &TestDevice, input: &str) -> (u8, serde_json::Value) {
        let host = device.host().static_host();
        let reply = __plux_entry_with(host, input.as_bytes());
        (reply[0], serde_json::from_slice(&reply[1..]).unwrap())
    }

    #[test]
    fn placement_const_follows_the_context_type() {
        assert_eq!(__PLUX_PLACEMENT, Placement::Device);
        assert_eq!(Placement::Server as u8, 0);
        assert_eq!(Placement::Device as u8, 1);
    }

    #[test]
    fn tag_0_carries_the_output() {
        let device = TestDevice::new();
        assert_eq!(run(&device, r#"{"n":4}"#), (0, json!({"doubled": 8})));
        assert_eq!(
            device.host().logs(),
            vec![(crate::Level::Info, "called".to_owned())]
        );
    }

    #[test]
    fn tag_1_carries_the_business_error() {
        let (tag, body) = run(&TestDevice::new(), r#"{"n":0}"#);
        assert_eq!(tag, 1);
        assert_eq!(
            body,
            json!({"code": "ZERO", "messageKey": "test.zero", "params": {"n": 0}})
        );
    }

    #[test]
    fn tag_2_refuses_undecodable_input() {
        let (tag, body) = run(&TestDevice::new(), r#"{"n":"x"}"#);
        assert_eq!(tag, 2);
        assert!(body["message"].as_str().unwrap().contains("invalid type"));
    }

    #[test]
    fn tag_3_reports_an_unencodable_output() {
        struct Bad;
        impl Serialize for Bad {
            fn serialize<S: serde::Serializer>(&self, _: S) -> Result<S::Ok, S::Error> {
                Err(serde::ser::Error::custom("no"))
            }
        }
        fn bad(_: &DeviceContext, _: In) -> Result<Bad, BusinessError> {
            Ok(Bad)
        }
        let device = TestDevice::new();
        let host = device.host().static_host();
        let reply = entry_with(host, br#"{"n":1}"#, &bad);
        assert_eq!(reply[0], 3);
        assert_eq!(
            serde_json::from_slice::<serde_json::Value>(&reply[1..]).unwrap(),
            json!({"message": "no"})
        );
    }

    #[test]
    fn schemas_name_input_and_output() {
        let document: serde_json::Value = serde_json::from_str(&__plux_schemas()).unwrap();
        assert!(document["input"]["properties"]["n"].is_object());
        assert!(document["output"]["properties"]["doubled"].is_object());
    }

    #[test]
    fn panic_message_is_bounded_on_a_char_boundary() {
        let long = "é".repeat(PANIC_LOG_BYTES);
        let cut = bounded_panic_message(&long);
        assert!(cut.len() <= PANIC_LOG_BYTES && cut.chars().all(|c| c == 'é'));
        assert_eq!(bounded_panic_message("short"), "short");
    }
}
