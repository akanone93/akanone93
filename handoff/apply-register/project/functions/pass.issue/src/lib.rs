// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `pass.issue`: Issues a registration's pass code, bound to its owner with HMAC-SHA-256 under a server-held key.

use plux_fn::{BusinessError, HostError, ServerContext};
use sha2::{Digest, Sha256};
use hmac::{Hmac, KeyInit, Mac};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// Lower-case hexadecimal.
fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

/// The registration to issue a pass for.
#[derive(Debug, Deserialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Input {
    /// The registration's ID.
    pub registration_id: String,
    /// The registration's owner, as the API records it.
    pub owner: String,
}

/// The pass code.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// 12 hexadecimal characters.
    pub code: String,
}

fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    if i.registration_id.is_empty() || i.registration_id.len() > 64 {
        return Err(BusinessError::new("REGISTRATION_INVALID", "pass.registrationInvalid"));
    }
    let claims = ctx.claims().map_err(|_| BusinessError::new("NOT_OWNER", "pass.notOwner"))?;
    let sub = claims.get_str("sub").unwrap_or_default();
    // Only the owner gets the code, and the code is bound to the verified caller, never to
    // what the input says.
    if sub.is_empty() || sub != i.owner {
        return Err(BusinessError::new("NOT_OWNER", "pass.notOwner"));
    }
    let key = ctx.secret("PASS_SIGNING_KEY").unwrap_or_else(|e| e.trap());
    let mut mac = <Hmac<Sha256> as KeyInit>::new_from_slice(key.as_bytes()).unwrap_or_else(|_| HostError::InvalidArgument.trap());
    mac.update(sub.as_bytes());
    mac.update(b"\x00");
    mac.update(i.registration_id.as_bytes());
    let digest = mac.finalize().into_bytes();
    Ok(Output { code: hex(&digest)[..12].to_string() })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::testing::{TestServer, replay};

    #[test]
    fn the_owner_gets_a_stable_code_and_no_one_else_does() {
        let host = TestServer::new();
        host.host().set_secret("PASS_SIGNING_KEY", "test-key").set_claims(serde_json::json!({"sub": "u-1"}));
        let issue = |owner: &str| handler(&host.context(), Input { registration_id: "r-001".into(), owner: owner.into() });
        let a = issue("u-1").unwrap().code;
        assert_eq!(a.len(), 12);
        assert_eq!(a, issue("u-1").unwrap().code);
        assert_eq!(issue("u-2").unwrap_err().code(), "NOT_OWNER");
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
