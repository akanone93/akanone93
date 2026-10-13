// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//! A device module with no dependencies that misbehaves on request, for the
//! engine's tests (FN-052, FN-053). The input is one ASCII command line,
//! `<op> [argument...]`; see `run` for the operations.

use std::hint::black_box;
use std::sync::atomic::{AtomicU32, Ordering};

/// Counts calls in this instance; a fresh instance per invocation (FN-010)
/// means it never exceeds one.
static CALLS: AtomicU32 = AtomicU32::new(0);

#[link(wasm_import_module = "plux_v1")]
unsafe extern "C" {
    fn log(level: i32, ptr: *const u8, len: i32);
    fn clock_now() -> i64;
    fn context() -> i64;
    fn env_get(ptr: *const u8, len: i32) -> i64;
    fn kv_get(ptr: *const u8, len: i32) -> i64;
    fn kv_put(key_ptr: *const u8, key_len: i32, value_ptr: *const u8, value_len: i32) -> i32;
    fn kv_delete(ptr: *const u8, len: i32) -> i32;
}

#[unsafe(no_mangle)]
pub extern "C" fn plux_abi_version() -> i32 {
    1
}

#[unsafe(no_mangle)]
pub extern "C" fn plux_alloc(len: i32) -> i32 {
    let mut buffer = Vec::<u8>::with_capacity(len.max(1) as usize);
    let ptr = buffer.as_mut_ptr();
    core::mem::forget(buffer);
    ptr as i32
}

#[unsafe(no_mangle)]
pub extern "C" fn plux_invoke(_index: i32, ptr: i32, len: i32) -> i64 {
    // SAFETY: the host wrote `len` bytes at `ptr`, which plux_alloc returned.
    let input = unsafe { core::slice::from_raw_parts(ptr as *const u8, len as usize) };
    let (tag, body) = run(input);
    let mut reply = Vec::with_capacity(body.len() + 1);
    reply.push(tag);
    reply.extend_from_slice(&body);
    let packed = ((reply.as_ptr() as i64) << 32) | reply.len() as i64;
    core::mem::forget(reply);
    packed
}

fn quoted(text: &[u8]) -> Vec<u8> {
    let mut out = vec![b'"'];
    for &byte in text {
        match byte {
            b'"' | b'\\' => out.extend_from_slice(&[b'\\', byte]),
            0..=0x1f => out.extend_from_slice(format!("\\u{byte:04x}").as_bytes()),
            _ => out.push(byte),
        }
    }
    out.push(b'"');
    out
}

fn number(value: i64) -> Vec<u8> {
    value.to_string().into_bytes()
}

/// Reads a packed host buffer, or the negative status it carries.
fn take(packed: i64) -> Result<Vec<u8>, i64> {
    if packed < 0 {
        return Err(packed);
    }
    let ptr = (packed as u64 >> 32) as usize;
    let size = (packed as u64 & 0xffff_ffff) as usize;
    if size == 0 {
        return Ok(Vec::new());
    }
    // SAFETY: the host allocated `size` bytes at `ptr` through plux_alloc.
    Ok(unsafe { core::slice::from_raw_parts(ptr as *const u8, size) }.to_vec())
}

fn recurse(depth: u64) -> u64 {
    let pad = [depth as u8; 256];
    if depth == 0 {
        return black_box(pad[0]) as u64;
    }
    black_box(pad[1]) as u64 + recurse(depth - 1)
}

fn run(input: &[u8]) -> (u8, Vec<u8>) {
    let mut parts = input.splitn(3, |byte| *byte == b' ');
    let op = parts.next().unwrap_or_default();
    let first = parts.next().unwrap_or_default();
    let second = parts.next().unwrap_or_default();
    let count = std::str::from_utf8(first)
        .ok()
        .and_then(|text| text.parse::<u64>().ok())
        .unwrap_or(0);
    match op {
        // Returns the rest of the line as a JSON string.
        b"echo" => (0, quoted(&input[input.len().min(5)..])),
        // Counts the calls this instance has served.
        b"counter" => (0, number(i64::from(CALLS.fetch_add(1, Ordering::Relaxed) + 1))),
        // Never returns.
        b"loop" => loop {
            black_box(());
        },
        // Allocates and touches `count` bytes.
        b"alloc" => {
            let block = vec![7u8; count as usize];
            (0, number(black_box(block).len() as i64))
        }
        b"trap" => core::arch::wasm32::unreachable(),
        // Recurses `count` frames of about 300 bytes.
        b"deep" => (0, number(recurse(count) as i64)),
        b"exit" => std::process::exit(count as i32),
        // The host calls.
        b"clock" => (0, number(unsafe { clock_now() })),
        b"context" => match take(unsafe { context() }) {
            Ok(json) => (0, json),
            Err(status) => (0, number(status)),
        },
        b"env" => match take(unsafe { env_get(first.as_ptr(), first.len() as i32) }) {
            Ok(value) => (0, quoted(&value)),
            Err(status) => (0, number(status)),
        },
        b"kv_get" => match take(unsafe { kv_get(first.as_ptr(), first.len() as i32) }) {
            Ok(value) => (0, quoted(&value)),
            Err(status) => (0, number(status)),
        },
        b"kv_put" => {
            let status = unsafe {
                kv_put(first.as_ptr(), first.len() as i32, second.as_ptr(), second.len() as i32)
            };
            (0, number(i64::from(status)))
        }
        b"kv_delete" => {
            let status = unsafe { kv_delete(first.as_ptr(), first.len() as i32) };
            (0, number(i64::from(status)))
        }
        // `log` uses the import; `stdout` and `stderr` use the WASI shim.
        b"log" => {
            unsafe { log(1, input.as_ptr().wrapping_add(4), input.len().saturating_sub(4) as i32) };
            (0, number(0))
        }
        b"stdout" => {
            println!("{}", String::from_utf8_lossy(&input[input.len().min(7)..]));
            (0, number(0))
        }
        b"stderr" => {
            eprintln!("{}", String::from_utf8_lossy(&input[input.len().min(7)..]));
            (0, number(0))
        }
        // Writes `count` bytes of log output.
        b"flood" => {
            let line = vec![b'x'; count as usize];
            unsafe { log(1, line.as_ptr(), line.len() as i32) };
            (0, number(0))
        }
        // Standard-library randomness seeds its hash maps through random_get.
        b"random" => {
            let map: std::collections::HashMap<u8, u8> = (0..4).map(|n| (n, n)).collect();
            (0, number(map.len() as i64))
        }
        // The result tags other than the output.
        b"business" => (
            1,
            br#"{"code":"PROBE_ERROR","messageKey":"probe.error","params":{"n":3}}"#.to_vec(),
        ),
        b"refuse" => (2, br#"{"message":"probe refused its input"}"#.to_vec()),
        b"unencodable" => (3, br#"{"message":"probe could not encode"}"#.to_vec()),
        // An output of `count` bytes: a JSON string of that length.
        b"big" => {
            let mut out = vec![b'"'];
            out.extend(std::iter::repeat_n(b'a', count.saturating_sub(2) as usize));
            out.push(b'"');
            (0, out)
        }
        _ => (2, br#"{"message":"unknown operation"}"#.to_vec()),
    }
}
