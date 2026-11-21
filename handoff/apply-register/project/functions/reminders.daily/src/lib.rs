// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `reminders.daily`: Run daily: reads the reminders due today and asks the API to push them.

use plux_fn::{BusinessError, HttpRequest, ServerContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// How many reminders at most this run sends.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// 1 to 50.
    pub limit: u32,
}

/// What the run did.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// Reminders handed to the API.
    pub sent: u32,
}

#[derive(Deserialize)]
struct Due {
    id: String,
}

fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    if !(1..=50).contains(&i.limit) {
        return Err(BusinessError::new("LIMIT_INVALID", "reminders.limitInvalid").with_param("max", 50));
    }
    let down = || BusinessError::new("UPSTREAM_DOWN", "reminders.upstreamDown");
    let base = ctx.env("AR_BASE").ok().flatten().ok_or_else(down)?;
    let key = ctx.secret("AR_PARTNER_KEY").map_err(|_| down())?;
    let res = ctx
        .http()
        .send(&HttpRequest::get(&format!("{base}/reminders/due?limit={}", i.limit)).header("X-AR-Key", &key))
        .map_err(|_| down())?;
    if res.status != 200 {
        return Err(down());
    }
    let due: Vec<Due> = serde_json::from_slice(&res.body).map_err(|_| down())?;
    let ids: Vec<&str> = due.iter().take(i.limit as usize).map(|d| d.id.as_str()).collect();
    if ids.is_empty() {
        return Ok(Output { sent: 0 });
    }
    // One call sends them all, so the run stays within its HTTP calls.
    let body = serde_json::to_vec(&serde_json::json!({ "ids": ids })).unwrap_or_default();
    let res = ctx
        .http()
        .send(&HttpRequest::post(&format!("{base}/reminders/push"), body).header("Content-Type", "application/json").header("X-AR-Key", &key))
        .map_err(|_| down())?;
    if res.status != 202 {
        return Err(down());
    }
    Ok(Output { sent: u32::try_from(ids.len()).unwrap_or(0) })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::HttpResponse;
    use plux_fn::testing::{TestServer, replay};

    #[test]
    fn pushes_what_is_due_in_one_call() {
        let host = TestServer::new();
        host.host().set_env("AR_BASE", "https://api.dev.applyregister.app/v1").set_secret("AR_PARTNER_KEY", "k");
        host.host().respond("https://api.dev.applyregister.app/v1/reminders/due?limit=10",
            HttpResponse { status: 200, headers: Default::default(), body: br#"[{"id":"a"},{"id":"b"}]"#.to_vec() });
        host.host().respond("https://api.dev.applyregister.app/v1/reminders/push",
            HttpResponse { status: 202, headers: Default::default(), body: vec![] });
        assert_eq!(handler(&host.context(), Input { limit: 10 }).unwrap().sent, 2);
        assert_eq!(host.host().http_requests().len(), 2);
    }

    #[test]
    fn every_stored_case_replays() {
        let host = TestServer::new();
        let cases = replay(include_str!("../function.json"), |case, input| {
            if let Some(claims) = case.get("claims") {
                host.host().set_claims(claims.clone());
            }
            __plux_entry_with(host.static_host(), input)
        });
        assert!(cases >= 2);
    }
}
