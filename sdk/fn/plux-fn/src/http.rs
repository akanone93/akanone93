// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

use std::collections::BTreeMap;

use serde::{Deserialize, Serialize};

use crate::base64::serde_bytes;
use crate::error::HostError;
use crate::host::Host;

/// An outbound HTTP request (server functions only). The body travels as
/// base64 in the host JSON (E6); here it is plain bytes.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct HttpRequest {
    /// HTTP method, upper case.
    pub method: String,
    /// Absolute URL; the host checks it against the function's allowlist.
    pub url: String,
    /// Request headers.
    #[serde(default)]
    pub headers: BTreeMap<String, String>,
    /// Request body.
    #[serde(default, with = "serde_bytes")]
    pub body: Vec<u8>,
}

impl HttpRequest {
    /// A request with no headers and no body.
    pub fn new(method: &str, url: &str) -> Self {
        Self {
            method: method.to_owned(),
            url: url.to_owned(),
            headers: BTreeMap::new(),
            body: Vec::new(),
        }
    }

    /// A `GET` request.
    pub fn get(url: &str) -> Self {
        Self::new("GET", url)
    }

    /// A `POST` request with a body.
    pub fn post(url: &str, body: impl Into<Vec<u8>>) -> Self {
        Self {
            body: body.into(),
            ..Self::new("POST", url)
        }
    }

    /// Adds a header.
    #[must_use]
    pub fn header(mut self, name: &str, value: &str) -> Self {
        self.headers.insert(name.to_owned(), value.to_owned());
        self
    }
}

/// The response to an [`HttpRequest`].
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct HttpResponse {
    /// Status code.
    pub status: u16,
    /// Response headers.
    #[serde(default)]
    pub headers: BTreeMap<String, String>,
    /// Response body.
    #[serde(default, with = "serde_bytes")]
    pub body: Vec<u8>,
}

/// Outbound HTTP, reached through `ServerContext::http`.
pub struct Http<'a> {
    pub(crate) host: &'a dyn Host,
}

impl Http<'_> {
    /// Sends a request. Egress is refused (`EgressRefused`) for a URL outside
    /// the function's allowlist.
    pub fn send(&self, request: &HttpRequest) -> Result<HttpResponse, HostError> {
        let payload = serde_json::to_vec(request).map_err(|_| HostError::InvalidArgument)?;
        let reply = self.host.http_request(&payload)?;
        serde_json::from_slice(&reply).map_err(|_| HostError::UpstreamFailed)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn body_is_base64_in_json() {
        let request = HttpRequest::post("https://x.test/a", "hi").header("k", "v");
        let json = serde_json::to_value(&request).unwrap();
        assert_eq!(json["body"], "aGk=");
        assert_eq!(
            serde_json::from_value::<HttpRequest>(json).unwrap(),
            request
        );
    }

    #[test]
    fn missing_body_and_headers_default() {
        let response: HttpResponse = serde_json::from_str(r#"{"status":204}"#).unwrap();
        assert!(response.body.is_empty() && response.headers.is_empty());
    }
}
