// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};
use serde_json::{Map, Value};

use crate::error::{BusinessError, HostError, InvokeError};
use crate::host::{Host, Level, Placement};
use crate::http::Http;

/// Ties a context type to its placement. Sealed: only [`DeviceContext`] and
/// [`ServerContext`] implement it.
pub trait ContextKind: Sized + sealed::Sealed {
    /// Where a function with this context runs.
    const PLACEMENT: Placement;

    #[doc(hidden)]
    fn from_host(host: &'static dyn Host) -> Self;

    #[doc(hidden)]
    fn abi_host() -> &'static dyn Host;
}

mod sealed {
    pub trait Sealed {}
    impl Sealed for super::DeviceContext<'_> {}
    impl Sealed for super::ServerContext<'_> {}
}

#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct Info {
    app: String,
    environment: String,
    assurance_level: String,
}

/// Typed access to the function's key-value store (`kv_*`).
pub struct Kv<'a> {
    host: &'a dyn Host,
}

impl Kv<'_> {
    /// Reads a key; `None` when it does not exist.
    pub fn get(&self, key: &str) -> Result<Option<Vec<u8>>, HostError> {
        match self.host.kv_get(key) {
            Ok(value) => Ok(Some(value)),
            Err(HostError::NotFound) => Ok(None),
            Err(error) => Err(error),
        }
    }

    /// Writes a key.
    pub fn put(&self, key: &str, value: &[u8]) -> Result<(), HostError> {
        self.host.kv_put(key, value)
    }

    /// Deletes a key; deleting a missing key succeeds.
    pub fn delete(&self, key: &str) -> Result<(), HostError> {
        match self.host.kv_delete(key) {
            Err(HostError::NotFound) => Ok(()),
            other => other,
        }
    }

    /// Reads a key holding JSON; undecodable content is `InvalidArgument`.
    pub fn get_json<T: DeserializeOwned>(&self, key: &str) -> Result<Option<T>, HostError> {
        match self.get(key)? {
            None => Ok(None),
            Some(bytes) => serde_json::from_slice(&bytes)
                .map(Some)
                .map_err(|_| HostError::InvalidArgument),
        }
    }

    /// Writes a value as JSON.
    pub fn put_json<T: Serialize>(&self, key: &str, value: &T) -> Result<(), HostError> {
        let bytes = serde_json::to_vec(value).map_err(|_| HostError::InvalidArgument)?;
        self.put(key, &bytes)
    }
}

/// The verified claims of the caller (server functions only, `claims`).
#[derive(Debug, Clone, PartialEq, Default)]
pub struct Claims(Map<String, Value>);

impl Claims {
    /// A claim by name.
    pub fn get(&self, name: &str) -> Option<&Value> {
        self.0.get(name)
    }

    /// A string claim by name.
    pub fn get_str(&self, name: &str) -> Option<&str> {
        self.0.get(name).and_then(Value::as_str)
    }

    /// All claims.
    pub fn as_map(&self) -> &Map<String, Value> {
        &self.0
    }
}

macro_rules! common_context {
    ($name:ident) => {
        impl<'a> $name<'a> {
            /// Reads the invocation context from the host.
            pub fn new(host: &'a dyn Host) -> Result<Self, HostError> {
                let info = serde_json::from_slice(&host.context()?)
                    .map_err(|_| HostError::InvalidArgument)?;
                Ok(Self { host, info })
            }

            /// Logs a line at `level`.
            pub fn log(&self, level: Level, message: &str) {
                self.host.log(level, message);
            }

            /// Logs at debug level.
            pub fn debug(&self, message: &str) {
                self.log(Level::Debug, message);
            }

            /// Logs at info level.
            pub fn info(&self, message: &str) {
                self.log(Level::Info, message);
            }

            /// Logs at warning level.
            pub fn warn(&self, message: &str) {
                self.log(Level::Warn, message);
            }

            /// Logs at error level.
            pub fn error(&self, message: &str) {
                self.log(Level::Error, message);
            }

            /// The host clock in Unix nanoseconds.
            pub fn now(&self) -> Result<i64, HostError> {
                self.host.clock_now()
            }

            /// The app the invocation belongs to.
            pub fn app_id(&self) -> &str {
                &self.info.app
            }

            /// The environment the invocation runs in.
            pub fn environment_id(&self) -> &str {
                &self.info.environment
            }

            /// The assurance level the caller reached.
            pub fn assurance_level(&self) -> &str {
                &self.info.assurance_level
            }

            /// An environment value declared in the function document; `None`
            /// when it is not set.
            pub fn env(&self, name: &str) -> Result<Option<String>, HostError> {
                match self.host.env_get(name) {
                    Ok(bytes) => String::from_utf8(bytes)
                        .map(Some)
                        .map_err(|_| HostError::InvalidArgument),
                    Err(HostError::NotFound) => Ok(None),
                    Err(error) => Err(error),
                }
            }

            /// The key-value store.
            pub fn kv(&self) -> Kv<'a> {
                Kv { host: self.host }
            }
        }
    };
}

/// The context of a device function (`FN-008`): logging, clock, environment
/// and key-value store. Nothing that needs the network, a secret or verified
/// claims is reachable from it.
pub struct DeviceContext<'a> {
    host: &'a dyn Host,
    info: Info,
}

common_context!(DeviceContext);

/// The context of a server function: everything a [`DeviceContext`] offers
/// plus `http`, `secret`, `claims` and `invoke`.
pub struct ServerContext<'a> {
    host: &'a dyn Host,
    info: Info,
}

common_context!(ServerContext);

impl<'a> ServerContext<'a> {
    /// Outbound HTTP.
    pub fn http(&self) -> Http<'a> {
        Http { host: self.host }
    }

    /// A secret granted to this function by name.
    pub fn secret(&self, name: &str) -> Result<String, HostError> {
        String::from_utf8(self.host.secret_get(name)?).map_err(|_| HostError::InvalidArgument)
    }

    /// The verified claims of the caller.
    pub fn claims(&self) -> Result<Claims, HostError> {
        let map =
            serde_json::from_slice(&self.host.claims()?).map_err(|_| HostError::InvalidArgument)?;
        Ok(Claims(map))
    }

    /// Invokes another function by name. The callee's business error comes
    /// back as [`InvokeError::Business`].
    pub fn invoke<I: Serialize, O: DeserializeOwned>(
        &self,
        name: &str,
        input: &I,
    ) -> Result<O, InvokeError> {
        let payload = serde_json::to_vec(input).map_err(|_| HostError::InvalidArgument)?;
        let reply = self.host.invoke_function(name, &payload)?;
        let upstream = InvokeError::System(HostError::UpstreamFailed);
        let (tag, body) = reply.split_first().ok_or_else(|| upstream.clone())?;
        match tag {
            0 => serde_json::from_slice(body).map_err(|_| upstream),
            1 => match serde_json::from_slice::<BusinessError>(body) {
                Ok(error) => Err(InvokeError::Business(error)),
                Err(_) => Err(upstream),
            },
            _ => Err(upstream),
        }
    }
}

impl ContextKind for DeviceContext<'_> {
    const PLACEMENT: Placement = Placement::Device;

    fn from_host(host: &'static dyn Host) -> Self {
        Self::new(host).unwrap_or_else(|error| error.trap())
    }

    fn abi_host() -> &'static dyn Host {
        crate::rt::device_host()
    }
}

impl ContextKind for ServerContext<'_> {
    const PLACEMENT: Placement = Placement::Server;

    fn from_host(host: &'static dyn Host) -> Self {
        Self::new(host).unwrap_or_else(|error| error.trap())
    }

    fn abi_host() -> &'static dyn Host {
        crate::rt::server_host()
    }
}
