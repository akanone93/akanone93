// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

use std::fmt;

use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};

/// A failure of a host call, one variant per ABI status (-1 to -7).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HostError {
    /// Status -1.
    NotFound,
    /// Status -2: the capability is not granted to this function.
    CapabilityDenied,
    /// Status -3.
    LimitExceeded,
    /// Status -4.
    EgressRefused,
    /// Status -5.
    UpstreamFailed,
    /// Status -6.
    InvalidArgument,
    /// Status -7, and any status a newer host adds.
    Unavailable,
}

impl HostError {
    /// The ABI status code.
    pub const fn status(self) -> i32 {
        match self {
            Self::NotFound => -1,
            Self::CapabilityDenied => -2,
            Self::LimitExceeded => -3,
            Self::EgressRefused => -4,
            Self::UpstreamFailed => -5,
            Self::InvalidArgument => -6,
            Self::Unavailable => -7,
        }
    }

    /// Maps an ABI status; unknown negative statuses are a generic failure
    /// (ADR-0062, versioning).
    pub const fn from_status(status: i32) -> Self {
        match status {
            -1 => Self::NotFound,
            -2 => Self::CapabilityDenied,
            -3 => Self::LimitExceeded,
            -4 => Self::EgressRefused,
            -5 => Self::UpstreamFailed,
            -6 => Self::InvalidArgument,
            _ => Self::Unavailable,
        }
    }

    /// Ends the invocation as a system error: the panic is logged and, in a
    /// module, aborts into a trap the host classifies.
    pub fn trap(self) -> ! {
        panic!("host call failed: {self}")
    }
}

impl fmt::Display for HostError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        f.write_str(match self {
            Self::NotFound => "not found",
            Self::CapabilityDenied => "capability denied",
            Self::LimitExceeded => "limit exceeded",
            Self::EgressRefused => "egress refused",
            Self::UpstreamFailed => "upstream failed",
            Self::InvalidArgument => "invalid argument",
            Self::Unavailable => "unavailable",
        })
    }
}

impl std::error::Error for HostError {}

/// A business error code was not `^[A-Z][A-Z0-9_]*$`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct InvalidCode(pub String);

impl fmt::Display for InvalidCode {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(
            f,
            "business error code {:?} does not match ^[A-Z][A-Z0-9_]*$",
            self.0
        )
    }
}

impl std::error::Error for InvalidCode {}

/// Whether `code` matches `^[A-Z][A-Z0-9_]*$` (checked by hand: no regex crate).
pub fn is_valid_code(code: &str) -> bool {
    let mut bytes = code.bytes();
    matches!(bytes.next(), Some(b'A'..=b'Z'))
        && bytes.all(|b| matches!(b, b'A'..=b'Z' | b'0'..=b'9' | b'_'))
}

#[derive(Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct Wire {
    code: String,
    message_key: String,
    #[serde(default)]
    params: Map<String, Value>,
}

/// A domain failure a function returns on purpose (`FN-022`): a stable code,
/// a message key the app localises, and parameters for the message. The code
/// is validated on construction and on decoding, so every value is well formed.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(try_from = "Wire", into = "Wire")]
pub struct BusinessError {
    code: String,
    message_key: String,
    params: Map<String, Value>,
}

impl BusinessError {
    /// Builds an error, rejecting a malformed code.
    pub fn try_new(
        code: impl Into<String>,
        message_key: impl Into<String>,
    ) -> Result<Self, InvalidCode> {
        let code = code.into();
        if !is_valid_code(&code) {
            return Err(InvalidCode(code));
        }
        Ok(Self {
            code,
            message_key: message_key.into(),
            params: Map::new(),
        })
    }

    /// Builds an error from a literal code. A malformed literal is a
    /// programming error, caught by the function's first test: this panics.
    pub fn new(code: &str, message_key: &str) -> Self {
        match Self::try_new(code, message_key) {
            Ok(error) => error,
            Err(invalid) => panic!("{invalid}"),
        }
    }

    /// Adds a message parameter. A value that cannot be represented as JSON
    /// is stored as `null`.
    #[must_use]
    pub fn with_param(mut self, name: &str, value: impl Serialize) -> Self {
        let value = serde_json::to_value(value).unwrap_or(Value::Null);
        self.params.insert(name.to_owned(), value);
        self
    }

    /// The stable error code.
    pub fn code(&self) -> &str {
        &self.code
    }

    /// The message key.
    pub fn message_key(&self) -> &str {
        &self.message_key
    }

    /// The message parameters.
    pub fn params(&self) -> &Map<String, Value> {
        &self.params
    }
}

impl From<BusinessError> for Wire {
    fn from(error: BusinessError) -> Self {
        Self {
            code: error.code,
            message_key: error.message_key,
            params: error.params,
        }
    }
}

impl TryFrom<Wire> for BusinessError {
    type Error = InvalidCode;

    fn try_from(wire: Wire) -> Result<Self, InvalidCode> {
        Ok(Self::try_new(wire.code, wire.message_key)?.with_params(wire.params))
    }
}

impl BusinessError {
    fn with_params(mut self, params: Map<String, Value>) -> Self {
        self.params = params;
        self
    }
}

impl fmt::Display for BusinessError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{} ({})", self.code, self.message_key)
    }
}

impl std::error::Error for BusinessError {}

/// The failure of a nested invocation: the callee's business error, or a
/// system failure.
#[derive(Debug, Clone, PartialEq)]
pub enum InvokeError {
    /// The callee returned a business error (tag 1).
    Business(BusinessError),
    /// The host refused or the callee failed.
    System(HostError),
}

impl fmt::Display for InvokeError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Business(error) => write!(f, "business error {error}"),
            Self::System(error) => write!(f, "system error: {error}"),
        }
    }
}

impl std::error::Error for InvokeError {}

impl From<HostError> for InvokeError {
    fn from(error: HostError) -> Self {
        Self::System(error)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn code_validation_follows_the_pattern() {
        for ok in ["A", "PRINCIPAL_TOO_SMALL", "E2", "X_1"] {
            assert!(is_valid_code(ok), "{ok}");
        }
        for bad in ["", "a", "1A", "_A", "Ab", "A-B", "A B", "ÄB", "A\n"] {
            assert!(!is_valid_code(bad), "{bad:?}");
        }
    }

    #[test]
    fn try_new_rejects_a_malformed_code() {
        assert_eq!(
            BusinessError::try_new("bad", "k"),
            Err(InvalidCode("bad".into()))
        );
        assert!(BusinessError::try_new("GOOD", "k").is_ok());
    }

    #[test]
    #[should_panic(expected = "does not match")]
    fn new_panics_on_a_malformed_literal() {
        let _ = BusinessError::new("nope", "k");
    }

    #[test]
    fn json_shape_is_code_message_key_params() {
        let error = BusinessError::new("RATE_OUT_OF_RANGE", "loan.rate").with_param("max", 1);
        let json = serde_json::to_value(&error).unwrap();
        assert_eq!(
            json,
            serde_json::json!({"code": "RATE_OUT_OF_RANGE", "messageKey": "loan.rate", "params": {"max": 1}})
        );
        let back: BusinessError = serde_json::from_value(json).unwrap();
        assert_eq!(back, error);
    }

    #[test]
    fn decoding_validates_the_code() {
        let bad = serde_json::json!({"code": "lower", "messageKey": "k"});
        assert!(serde_json::from_value::<BusinessError>(bad).is_err());
    }

    #[test]
    fn statuses_round_trip() {
        for status in -7..=-1 {
            assert_eq!(HostError::from_status(status).status(), status);
        }
        assert_eq!(HostError::from_status(-99), HostError::Unavailable);
    }
}
