// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `hooks.orgSync`: Inbound webhook: keeps an organisation's open state; the platform checks signatures and replays.

use plux_fn::{BusinessError, ServerContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// The webhook's payload.
#[derive(Debug, Deserialize, Serialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Input {
    /// The organisation's slug.
    pub slug: String,
    /// Whether registration is open.
    pub open: bool,
    /// When it closes, ISO 8601, if known.
    pub closes_at: Option<String>,
}

/// Whether it was stored.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// Always true on success.
    pub stored: bool,
}

fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    let slug_ok = (2..=40).contains(&i.slug.len()) && i.slug.bytes().all(|b| b.is_ascii_lowercase() || b.is_ascii_digit() || b == b'-');
    if !slug_ok {
        return Err(BusinessError::new("SLUG_INVALID", "hooks.slugInvalid"));
    }
    if i.closes_at.as_ref().is_some_and(|c| c.len() > 40) {
        return Err(BusinessError::new("CLOSES_AT_INVALID", "hooks.closesAtInvalid"));
    }
    let body = serde_json::to_vec(&i).unwrap_or_default();
    ctx.kv().put(&format!("org:{}", i.slug), &body).unwrap_or_else(|e| e.trap());
    Ok(Output { stored: true })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestServer, replay};

    #[test]
    fn stores_the_state_by_slug() {
        let host = TestServer::new();
        handler(&host.context(), Input { slug: "campus".into(), open: true, closes_at: None }).unwrap();
        assert!(host.host().kv("org:campus").is_some());
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
