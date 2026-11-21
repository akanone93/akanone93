// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `workshops.capacity`: Places left in DevHarbor's workshops; a full one is named.

use plux_fn::{BusinessError, ServerContext};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// The workshops to look at.
#[derive(Debug, Deserialize, JsonSchema)]
pub struct Input {
    /// One to twenty workshop IDs.
    pub ids: Vec<String>,
}

/// Places left, in the order asked.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// One count per ID.
    pub left: Vec<u32>,
}

const DEFAULT_CAPACITY: u32 = 30;

fn count(ctx: &ServerContext, key: &str) -> Option<u32> {
    let raw = ctx.kv().get(key).unwrap_or_else(|e| e.trap())?;
    std::str::from_utf8(&raw).ok()?.trim().parse().ok()
}

fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    if i.ids.is_empty() || i.ids.len() > 20 || i.ids.iter().any(|id| id.is_empty() || id.len() > 40) {
        return Err(BusinessError::new("IDS_INVALID", "workshops.idsInvalid"));
    }
    let mut left = vec![];
    for id in &i.ids {
        let capacity = count(ctx, &format!("cap:{id}")).unwrap_or(DEFAULT_CAPACITY);
        let taken = count(ctx, &format!("taken:{id}")).unwrap_or(0);
        let n = capacity.saturating_sub(taken);
        if n == 0 {
            return Err(BusinessError::new("FULL", "workshops.full").with_param("id", id));
        }
        left.push(n);
    }
    Ok(Output { left })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestServer, replay};

    #[test]
    fn counts_places_and_names_a_full_workshop() {
        let host = TestServer::new();
        host.host().set_kv("cap:rust", b"10").set_kv("taken:rust", b"4").set_kv("taken:flutter", b"30");
        let out = handler(&host.context(), Input { ids: vec!["rust".into(), "go".into()] }).unwrap();
        assert_eq!(out.left, vec![6, 30]);
        let e = handler(&host.context(), Input { ids: vec!["flutter".into()] }).unwrap_err();
        assert_eq!(e.code(), "FULL");
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
