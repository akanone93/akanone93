// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

use crate::error::HostError;

/// Where a function runs. The discriminants are part of the build contract.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum Placement {
    /// On the Plux Server (`fnrunner`).
    Server = 0,
    /// On the device, in the interpreter.
    Device = 1,
}

/// Severity of a log line, as the ABI `log` import numbers it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[repr(u8)]
pub enum Level {
    /// Debug.
    Debug = 0,
    /// Info.
    Info = 1,
    /// Warning.
    Warn = 2,
    /// Error.
    Error = 3,
}

/// The guest's view of the host: one method per `plux_v1` import.
///
/// Byte-oriented on purpose; the typed layer lives in the contexts. The
/// server-only methods default to [`HostError::CapabilityDenied`], which is
/// also what a device host answers, so a device module never imports them.
pub trait Host {
    /// `log(level, ptr, len)`.
    fn log(&self, level: Level, message: &str);
    /// `clock_now()`: Unix nanoseconds.
    fn clock_now(&self) -> Result<i64, HostError>;
    /// `context()`: JSON with app, environment, placement and assurance level.
    fn context(&self) -> Result<Vec<u8>, HostError>;
    /// `env_get(name)`.
    fn env_get(&self, name: &str) -> Result<Vec<u8>, HostError>;
    /// `kv_get(key)`.
    fn kv_get(&self, key: &str) -> Result<Vec<u8>, HostError>;
    /// `kv_put(key, value)`.
    fn kv_put(&self, key: &str, value: &[u8]) -> Result<(), HostError>;
    /// `kv_delete(key)`.
    fn kv_delete(&self, key: &str) -> Result<(), HostError>;
    /// `http_request(json)`: server only.
    fn http_request(&self, _request: &[u8]) -> Result<Vec<u8>, HostError> {
        Err(HostError::CapabilityDenied)
    }
    /// `secret_get(name)`: server only.
    fn secret_get(&self, _name: &str) -> Result<Vec<u8>, HostError> {
        Err(HostError::CapabilityDenied)
    }
    /// `claims()`: JSON object of verified claims, server only.
    fn claims(&self) -> Result<Vec<u8>, HostError> {
        Err(HostError::CapabilityDenied)
    }
    /// `invoke_function(name, input)`: returns `tag || JSON`, server only.
    fn invoke_function(&self, _name: &str, _input: &[u8]) -> Result<Vec<u8>, HostError> {
        Err(HostError::CapabilityDenied)
    }
}

#[cfg(not(target_arch = "wasm32"))]
/// A host that answers every call with [`HostError::Unavailable`]. Native
/// builds use it where no module host exists.
#[derive(Debug, Clone, Copy, Default)]
pub struct UnavailableHost;

#[cfg(not(target_arch = "wasm32"))]
impl Host for UnavailableHost {
    fn log(&self, _level: Level, _message: &str) {}
    fn clock_now(&self) -> Result<i64, HostError> {
        Err(HostError::Unavailable)
    }
    fn context(&self) -> Result<Vec<u8>, HostError> {
        Err(HostError::Unavailable)
    }
    fn env_get(&self, _name: &str) -> Result<Vec<u8>, HostError> {
        Err(HostError::Unavailable)
    }
    fn kv_get(&self, _key: &str) -> Result<Vec<u8>, HostError> {
        Err(HostError::Unavailable)
    }
    fn kv_put(&self, _key: &str, _value: &[u8]) -> Result<(), HostError> {
        Err(HostError::Unavailable)
    }
    fn kv_delete(&self, _key: &str) -> Result<(), HostError> {
        Err(HostError::Unavailable)
    }
}
