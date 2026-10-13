// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//! The `plux_v1` imports, as the guest sees them (wasm32 only).

use crate::error::HostError;
use crate::host::{Host, Level};

#[link(wasm_import_module = "plux_v1")]
unsafe extern "C" {
    fn log(level: i32, ptr: *const u8, len: i32);
    fn clock_now() -> i64;
    fn context() -> i64;
    fn env_get(ptr: *const u8, len: i32) -> i64;
    fn kv_get(ptr: *const u8, len: i32) -> i64;
    fn kv_put(key_ptr: *const u8, key_len: i32, value_ptr: *const u8, value_len: i32) -> i32;
    fn kv_delete(ptr: *const u8, len: i32) -> i32;
    fn http_request(ptr: *const u8, len: i32) -> i64;
    fn secret_get(ptr: *const u8, len: i32) -> i64;
    fn claims() -> i64;
    fn invoke_function(name_ptr: *const u8, name_len: i32, in_ptr: *const u8, in_len: i32) -> i64;
}

fn len(bytes: &[u8]) -> i32 {
    i32::try_from(bytes.len()).unwrap_or(i32::MAX)
}

/// Reads a host-returned buffer: negative is a status, otherwise
/// `(ptr << 32) | len` of memory the host allocated through `plux_alloc`.
fn take(packed: i64) -> Result<Vec<u8>, HostError> {
    if packed < 0 {
        return Err(HostError::from_status(packed as i32));
    }
    let ptr = (packed as u64 >> 32) as usize;
    let size = (packed as u64 & 0xffff_ffff) as usize;
    if size == 0 {
        return Ok(Vec::new());
    }
    // SAFETY: the host wrote `size` bytes at `ptr` into a buffer it
    // allocated through `plux_alloc`; the guest owns it and never frees it.
    Ok(unsafe { core::slice::from_raw_parts(ptr as *const u8, size) }.to_vec())
}

fn status(code: i32) -> Result<(), HostError> {
    if code < 0 {
        Err(HostError::from_status(code))
    } else {
        Ok(())
    }
}

/// Logs through the `log` import.
pub(crate) fn log_line(level: Level, message: &str) {
    // SAFETY: the pointer and length describe `message`, which outlives the call.
    unsafe { log(level as i32, message.as_ptr(), len(message.as_bytes())) }
}

/// The host of a device module. It overrides no server-only method, so the
/// server imports are never referenced and never appear in the module
/// (`PLX-7018`).
pub(crate) struct DeviceAbi;

/// The host of a server module.
pub(crate) struct ServerAbi;

macro_rules! shared_imports {
    () => {
        fn log(&self, level: Level, message: &str) {
            log_line(level, message);
        }

        fn clock_now(&self) -> Result<i64, HostError> {
            // SAFETY: no arguments; the import returns a plain integer.
            let now = unsafe { clock_now() };
            if now < 0 {
                Err(HostError::from_status(now as i32))
            } else {
                Ok(now)
            }
        }

        fn context(&self) -> Result<Vec<u8>, HostError> {
            // SAFETY: no arguments; the result is decoded by `take`.
            take(unsafe { context() })
        }

        fn env_get(&self, name: &str) -> Result<Vec<u8>, HostError> {
            // SAFETY: pointer and length describe `name`.
            take(unsafe { env_get(name.as_ptr(), len(name.as_bytes())) })
        }

        fn kv_get(&self, key: &str) -> Result<Vec<u8>, HostError> {
            // SAFETY: pointer and length describe `key`.
            take(unsafe { kv_get(key.as_ptr(), len(key.as_bytes())) })
        }

        fn kv_put(&self, key: &str, value: &[u8]) -> Result<(), HostError> {
            // SAFETY: pointers and lengths describe `key` and `value`.
            status(unsafe {
                kv_put(
                    key.as_ptr(),
                    len(key.as_bytes()),
                    value.as_ptr(),
                    len(value),
                )
            })
        }

        fn kv_delete(&self, key: &str) -> Result<(), HostError> {
            // SAFETY: pointer and length describe `key`.
            status(unsafe { kv_delete(key.as_ptr(), len(key.as_bytes())) })
        }
    };
}

impl Host for DeviceAbi {
    shared_imports!();
}

impl Host for ServerAbi {
    shared_imports!();

    fn http_request(&self, request: &[u8]) -> Result<Vec<u8>, HostError> {
        // SAFETY: pointer and length describe `request`.
        take(unsafe { http_request(request.as_ptr(), len(request)) })
    }

    fn secret_get(&self, name: &str) -> Result<Vec<u8>, HostError> {
        // SAFETY: pointer and length describe `name`.
        take(unsafe { secret_get(name.as_ptr(), len(name.as_bytes())) })
    }

    fn claims(&self) -> Result<Vec<u8>, HostError> {
        // SAFETY: no arguments; the result is decoded by `take`.
        take(unsafe { claims() })
    }

    fn invoke_function(&self, name: &str, input: &[u8]) -> Result<Vec<u8>, HostError> {
        // SAFETY: pointers and lengths describe `name` and `input`.
        take(unsafe {
            invoke_function(
                name.as_ptr(),
                len(name.as_bytes()),
                input.as_ptr(),
                len(input),
            )
        })
    }
}
