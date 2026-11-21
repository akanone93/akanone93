// SPDX-FileCopyrightText: 2026 Apply Register
// SPDX-License-Identifier: Apache-2.0

//! `partner.submit`: Forwards an organisation's request to Apply Register's API with the server-held key, once per e-mail.

use plux_fn::{BusinessError, HttpRequest, ServerContext};
use sha2::{Digest, Sha256};
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};

/// Lower-case hexadecimal.
fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

/// The organisation's request.
#[derive(Debug, Deserialize, Serialize, JsonSchema)]
#[serde(rename_all = "camelCase")]
pub struct Input {
    /// 2 to 80 characters.
    pub org_name: String,
    /// The contact's e-mail.
    pub email: String,
    /// One of the Apply Register categories.
    pub category: String,
    /// 1 to 50.
    pub pages: u32,
    /// At most ten, each at most 40 characters.
    pub needs: Vec<String>,
    /// starter, standard or pro.
    pub plan: String,
    /// At most 1000 characters.
    pub notes: Option<String>,
}

/// The request's reference.
#[derive(Debug, Serialize, JsonSchema)]
pub struct Output {
    /// As the API gave it.
    pub reference: String,
}

const CATEGORIES: [&str; 8] = ["education", "finance", "sports", "charity", "health", "housing", "events", "venues"];

fn invalid(field: &str) -> BusinessError {
    BusinessError::new("INPUT_INVALID", "partner.inputInvalid").with_param("field", field)
}

fn check(i: &Input) -> Result<(), BusinessError> {
    let name = i.org_name.trim().chars().count();
    if !(2..=80).contains(&name) {
        return Err(invalid("orgName"));
    }
    let (local, domain) = i.email.split_once('@').ok_or_else(|| invalid("email"))?;
    if local.is_empty() || !domain.contains('.') || i.email.len() > 254 || i.email.contains(char::is_whitespace) {
        return Err(invalid("email"));
    }
    if !CATEGORIES.contains(&i.category.as_str()) {
        return Err(invalid("category"));
    }
    if !(1..=50).contains(&i.pages) {
        return Err(invalid("pages"));
    }
    if i.needs.len() > 10 || i.needs.iter().any(|n| n.chars().count() > 40) {
        return Err(invalid("needs"));
    }
    if !["starter", "standard", "pro"].contains(&i.plan.as_str()) {
        return Err(invalid("plan"));
    }
    if i.notes.as_ref().is_some_and(|n| n.chars().count() > 1000) {
        return Err(invalid("notes"));
    }
    Ok(())
}

fn handler(ctx: &ServerContext, i: Input) -> Result<Output, BusinessError> {
    check(&i)?;
    // The store keeps a hash of the address, not the address.
    let key = format!("req:{}", hex(&Sha256::digest(i.email.trim().to_lowercase().as_bytes())));
    if ctx.kv().get(&key).unwrap_or_else(|e| e.trap()).is_some() {
        return Err(BusinessError::new("DUPLICATE_REQUEST", "partner.duplicate"));
    }
    let base = ctx.env("AR_BASE").ok().flatten().ok_or_else(|| BusinessError::new("UPSTREAM_DOWN", "partner.upstreamDown"))?;
    let body = serde_json::to_vec(&i).map_err(|_| invalid("body"))?;
    let key_value = ctx.secret("AR_PARTNER_KEY").map_err(|_| BusinessError::new("UPSTREAM_DOWN", "partner.upstreamDown"))?;
    let req = HttpRequest::post(&format!("{base}/partner-requests"), body)
        .header("Content-Type", "application/json")
        .header("X-AR-Key", &key_value);
    let res = ctx.http().send(&req).map_err(|_| BusinessError::new("UPSTREAM_DOWN", "partner.upstreamDown"))?;
    if res.status != 201 {
        return Err(BusinessError::new("UPSTREAM_REFUSED", "partner.refused").with_param("status", res.status));
    }
    let reference = String::from_utf8_lossy(&res.body).trim().to_string();
    if reference.is_empty() || reference.len() > 64 {
        return Err(BusinessError::new("UPSTREAM_REFUSED", "partner.refused").with_param("status", res.status));
    }
    ctx.kv().put(&key, b"1").unwrap_or_else(|e| e.trap());
    Ok(Output { reference })
}

plux_fn::function!(handler);

#[cfg(test)]
mod tests {
    use super::*;
    use plux_fn::HttpResponse;
    use plux_fn::testing::{TestServer, replay};

    fn request() -> Input {
        Input { org_name: "Aster University".into(), email: "Admissions@Aster.example".into(), category: "education".into(),
                pages: 4, needs: vec!["payments".into()], plan: "standard".into(), notes: None }
    }

    #[test]
    fn forwards_once_with_the_server_key_and_refuses_a_second_time() {
        let host = TestServer::new();
        host.host().set_env("AR_BASE", "https://api.dev.applyregister.app/v1");
        host.host().set_secret("AR_PARTNER_KEY", "k-123");
        host.host().respond("https://api.dev.applyregister.app/v1/partner-requests",
            HttpResponse { status: 201, headers: Default::default(), body: b"PR-0042".to_vec() });
        let out = handler(&host.context(), request()).unwrap();
        assert_eq!(out.reference, "PR-0042");
        let sent = host.host().http_requests();
        assert_eq!(sent.len(), 1);
        assert!(sent[0].headers.iter().any(|(k, v)| k == "X-AR-Key" && v == "k-123"));
        // The same address, in another case, is a duplicate; the store holds no address.
        let mut again = request();
        again.email = "admissions@aster.example".into();
        assert_eq!(handler(&host.context(), again).unwrap_err().code(), "DUPLICATE_REQUEST");
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
