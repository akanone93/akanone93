// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//! Conformance probe for the Plux Function host ABI v1 (ADR-0062, FN-015).
//!
//! The module has no dependencies: it declares the `plux_v1` imports itself
//! and exports the four ABI functions. Every host runs the vectors in
//! `schema/testdata/functions/abi-v1.json` against it, so a host that passes
//! them implements the ABI the same way as every other host.
//!
//! Each entry takes the invocation input as raw bytes and returns a tag-0
//! output unless stated otherwise. A host call that returns a negative status
//! is reported to the caller as `{"status":N}` (or as a number where a value
//! was expected), which is how the vectors observe capability refusals.
//!
//! | Index | Behaviour |
//! |---|---|
//! | 0 | Echo: the output is the input. |
//! | 1 | Key-value round trip. Input `key\nvalue`. Calls `kv_put`, `kv_get`, `kv_delete`, `kv_get`; the output is `{"put":S,"get":V,"delete":S,"after":V}` where `S` is a status and `V` a string or a negative status. |
//! | 2 | HTTP. Input is a URL; sends `GET`. The output is the host's response JSON, or `{"status":N}`. Server only. |
//! | 3 | Secret. Input is the name. The output is `{"value":"…"}` or `{"status":N}`. Server only. |
//! | 4 | Claims. The output is the host's claims JSON, or `{"status":N}`. Server only. |
//! | 5 | Nested invocation. Input `name\ninput`. The output is `{"tag":T,"body":…}` for the nested result, or `{"status":N}`. Server only. |
//! | 6 | Context dump. Input is an environment variable name. The output is `{"context":…,"clock":NS,"wall":NS,"env":V}`; `clock` comes from `clock_now`, `wall` from the WASI wall clock. |
//! | 7 | Log flood. Input is a decimal count of 64-byte lines sent through `log`; the output is `{"logged":N}`. |
//! | 8 | Infinite loop. |
//! | 9 | Memory growth: allocates 1 MiB chunks until the host refuses a grow, which traps. |
//! | 10 | Large output. Input is a decimal byte count; the output is that many `a` bytes. |
//! | 11 | Panic: a trap. |
//! | 12 | Business error, tag 1: `{"code":"probe.rejected","messageKey":"probe.rejected","params":{"input":"…"}}`. |
//! | 13 | Refusal of the input, tag 2: `{"message":"probe refuses its input"}`. |
//! | 14 | Failure to encode the output, tag 3: `{"message":"probe cannot encode its output"}`. |
//! | 15 | Malformed result: tag 9. |
//! | 16 | Opens `/etc/passwd`; the output is `{"kind":"<std::io::ErrorKind>"}` or `{"opened":true}`. |
//! | 17 | Random draw: eight bytes from the WASI `random_get`, as `{"errno":N,"random":"<hex>"}`. |
//! | 18 | Counter in a static; the output is `{"counter":N}`. N is 1 in every fresh instance. |
//! | 19 | Writes a line to standard output and one to standard error; the output is `{}`. |
//! | 20 | Counts the WASI arguments and environment variables: `{"args":N,"vars":N}`. |
//! | 21 | Passes an out-of-bounds pointer to `log`. |
//! | 22 | HTTP repeated. Input `count\nurl`; sends `count` requests and outputs `{"results":[R,…]}` where `R` is 0 for a response or the negative status. Server only. |
//! | 23 | Raw result: the first input byte is the tag and the rest is the body, returned unchanged. |
//!
//! Any other index is refused with a tag-2 result. Entries 2 to 5 and 22 are absent
//! from the `device` build, which does not import the server-only functions.

use std::alloc::{Layout, alloc};
use std::fmt::Write as _;
use std::sync::atomic::{AtomicU32, Ordering};

#[link(wasm_import_module = "plux_v1")]
unsafe extern "C" {
    #[link_name = "log"]
    fn host_log(level: i32, ptr: *const u8, len: usize);
    #[link_name = "clock_now"]
    fn host_clock_now() -> i64;
    #[link_name = "context"]
    fn host_context() -> i64;
    #[link_name = "env_get"]
    fn host_env_get(ptr: *const u8, len: usize) -> i64;
    #[link_name = "kv_get"]
    fn host_kv_get(ptr: *const u8, len: usize) -> i64;
    #[link_name = "kv_put"]
    fn host_kv_put(kptr: *const u8, klen: usize, vptr: *const u8, vlen: usize) -> i32;
    #[link_name = "kv_delete"]
    fn host_kv_delete(ptr: *const u8, len: usize) -> i32;
}

#[cfg(not(feature = "device"))]
#[link(wasm_import_module = "plux_v1")]
unsafe extern "C" {
    #[link_name = "http_request"]
    fn host_http_request(ptr: *const u8, len: usize) -> i64;
    #[link_name = "secret_get"]
    fn host_secret_get(ptr: *const u8, len: usize) -> i64;
    #[link_name = "claims"]
    fn host_claims() -> i64;
    #[link_name = "invoke_function"]
    fn host_invoke_function(nptr: *const u8, nlen: usize, iptr: *const u8, ilen: usize) -> i64;
}

#[link(wasm_import_module = "wasi_snapshot_preview1")]
unsafe extern "C" {
    fn random_get(buf: *mut u8, len: usize) -> u16;
}

/// The ABI major version this module implements.
#[unsafe(no_mangle)]
pub extern "C" fn plux_abi_version() -> i32 {
    1
}

/// Returns a buffer of `len` bytes the host may write into. Buffers are
/// never freed: each invocation has a fresh instance.
#[unsafe(no_mangle)]
pub extern "C" fn plux_alloc(len: i32) -> i32 {
    let Ok(layout) = Layout::from_size_align(len.max(1) as usize, 1) else {
        return 0;
    };
    unsafe { alloc(layout) as i32 }
}

/// Runs entry `index` on the input at `ptr`, `len`.
#[unsafe(no_mangle)]
pub extern "C" fn plux_invoke(index: i32, ptr: i32, len: i32) -> i64 {
    let input: &[u8] = if len == 0 {
        &[]
    } else {
        unsafe { std::slice::from_raw_parts(ptr as *const u8, len as usize) }
    };
    dispatch(index, input)
}

fn dispatch(index: i32, input: &[u8]) -> i64 {
    match index {
        0 => reply(0, input.to_vec()),
        1 => kv_round_trip(input),
        #[cfg(not(feature = "device"))]
        2 => http(input),
        #[cfg(not(feature = "device"))]
        3 => secret(input),
        #[cfg(not(feature = "device"))]
        4 => claims(),
        #[cfg(not(feature = "device"))]
        5 => nested(input),
        6 => context_dump(input),
        7 => log_flood(input),
        8 => spin(),
        9 => grow_until_refused(),
        10 => large_output(input),
        11 => panic!("probe panic"),
        12 => business_error(input),
        13 => reply(2, br#"{"message":"probe refuses its input"}"#.to_vec()),
        14 => reply(3, br#"{"message":"probe cannot encode its output"}"#.to_vec()),
        15 => reply(9, b"x".to_vec()),
        16 => open_file(),
        17 => random_draw(),
        18 => counter(),
        19 => standard_streams(),
        20 => args_and_vars(),
        21 => bad_pointer(),
        23 => raw(input),
        #[cfg(not(feature = "device"))]
        22 => http_repeat(input),
        _ => reply(2, br#"{"message":"unknown function index"}"#.to_vec()),
    }
}

/// Packs `tag || body` into the result the host reads: `(ptr << 32) | len`.
fn reply(tag: u8, body: Vec<u8>) -> i64 {
    let mut out = Vec::with_capacity(body.len() + 1);
    out.push(tag);
    out.extend_from_slice(&body);
    let packed = ((out.as_ptr() as u64) << 32) | out.len() as u64;
    std::mem::forget(out);
    packed as i64
}

/// Copies the buffer a host call returned, or yields its negative status.
fn take(packed: i64) -> Result<Vec<u8>, i64> {
    if packed < 0 {
        return Err(packed);
    }
    let ptr = (packed >> 32) as usize;
    let len = (packed & 0xffff_ffff) as usize;
    if len == 0 {
        return Ok(Vec::new());
    }
    Ok(unsafe { std::slice::from_raw_parts(ptr as *const u8, len) }.to_vec())
}

fn status_json(status: i64) -> Vec<u8> {
    format!(r#"{{"status":{status}}}"#).into_bytes()
}

/// A JSON string for a buffer, or the status number.
fn string_or_status(result: Result<Vec<u8>, i64>) -> String {
    match result {
        Ok(bytes) => quote(&bytes),
        Err(status) => status.to_string(),
    }
}

/// The buffer as raw JSON, or `{"status":N}`.
#[cfg(not(feature = "device"))]
fn raw_or_status(result: Result<Vec<u8>, i64>) -> Vec<u8> {
    match result {
        Ok(bytes) => bytes,
        Err(status) => status_json(status),
    }
}

fn quote(bytes: &[u8]) -> String {
    let mut out = String::from("\"");
    for c in String::from_utf8_lossy(bytes).chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            c if (c as u32) < 0x20 => {
                let _ = write!(out, "\\u{:04x}", c as u32);
            }
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

fn number(input: &[u8]) -> usize {
    std::str::from_utf8(input)
        .ok()
        .and_then(|s| s.trim().parse().ok())
        .unwrap_or(0)
}

fn split_line(input: &[u8]) -> (&[u8], &[u8]) {
    match input.iter().position(|&b| b == b'\n') {
        Some(i) => (&input[..i], &input[i + 1..]),
        None => (input, &[]),
    }
}

fn kv_round_trip(input: &[u8]) -> i64 {
    let (key, value) = split_line(input);
    let put = unsafe { host_kv_put(key.as_ptr(), key.len(), value.as_ptr(), value.len()) };
    let got = take(unsafe { host_kv_get(key.as_ptr(), key.len()) });
    let delete = unsafe { host_kv_delete(key.as_ptr(), key.len()) };
    let after = take(unsafe { host_kv_get(key.as_ptr(), key.len()) });
    let body = format!(
        r#"{{"put":{put},"get":{},"delete":{delete},"after":{}}}"#,
        string_or_status(got),
        string_or_status(after)
    );
    reply(0, body.into_bytes())
}

#[cfg(not(feature = "device"))]
fn http(input: &[u8]) -> i64 {
    let request = format!(r#"{{"method":"GET","url":{}}}"#, quote(input));
    let response = unsafe { host_http_request(request.as_ptr(), request.len()) };
    reply(0, raw_or_status(take(response)))
}

#[cfg(not(feature = "device"))]
fn http_repeat(input: &[u8]) -> i64 {
    let (count, url) = split_line(input);
    let request = format!(r#"{{"method":"GET","url":{}}}"#, quote(url));
    let mut results = Vec::new();
    for _ in 0..number(count) {
        let response = unsafe { host_http_request(request.as_ptr(), request.len()) };
        results.push(if response < 0 { response.to_string() } else { "0".to_string() });
    }
    reply(0, format!(r#"{{"results":[{}]}}"#, results.join(",")).into_bytes())
}

#[cfg(not(feature = "device"))]
fn secret(input: &[u8]) -> i64 {
    let value = take(unsafe { host_secret_get(input.as_ptr(), input.len()) });
    let body = match value {
        Ok(bytes) => format!(r#"{{"value":{}}}"#, quote(&bytes)).into_bytes(),
        Err(status) => status_json(status),
    };
    reply(0, body)
}

#[cfg(not(feature = "device"))]
fn claims() -> i64 {
    reply(0, raw_or_status(take(unsafe { host_claims() })))
}

#[cfg(not(feature = "device"))]
fn nested(input: &[u8]) -> i64 {
    let (name, nested_input) = split_line(input);
    let result = unsafe {
        host_invoke_function(name.as_ptr(), name.len(), nested_input.as_ptr(), nested_input.len())
    };
    let body = match take(result) {
        Ok(bytes) if !bytes.is_empty() => {
            let mut out = format!(r#"{{"tag":{},"body":"#, bytes[0]).into_bytes();
            out.extend_from_slice(&bytes[1..]);
            out.push(b'}');
            out
        }
        Ok(_) => status_json(0),
        Err(status) => status_json(status),
    };
    reply(0, body)
}

fn context_dump(input: &[u8]) -> i64 {
    let context = take(unsafe { host_context() }).unwrap_or_else(|s| status_json(s));
    let clock = unsafe { host_clock_now() };
    let wall = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    let env = string_or_status(take(unsafe { host_env_get(input.as_ptr(), input.len()) }));
    let mut body = br#"{"context":"#.to_vec();
    body.extend_from_slice(&context);
    body.extend_from_slice(format!(r#","clock":{clock},"wall":{wall},"env":{env}}}"#).as_bytes());
    reply(0, body)
}

fn log_flood(input: &[u8]) -> i64 {
    let count = number(input);
    let line = [b'x'; 64];
    for _ in 0..count {
        unsafe { host_log(1, line.as_ptr(), line.len()) };
    }
    reply(0, format!(r#"{{"logged":{count}}}"#).into_bytes())
}

fn spin() -> i64 {
    loop {
        std::hint::black_box(0u8);
    }
}

fn grow_until_refused() -> i64 {
    let mut kept: Vec<Vec<u8>> = Vec::new();
    loop {
        let mut chunk = vec![0u8; 1 << 20];
        chunk[0] = 1;
        kept.push(std::hint::black_box(chunk));
    }
}

fn large_output(input: &[u8]) -> i64 {
    reply(0, vec![b'a'; number(input)])
}

fn business_error(input: &[u8]) -> i64 {
    let body = format!(
        r#"{{"code":"probe.rejected","messageKey":"probe.rejected","params":{{"input":{}}}}}"#,
        quote(input)
    );
    reply(1, body.into_bytes())
}

fn open_file() -> i64 {
    let body = match std::fs::File::open("/etc/passwd") {
        Ok(_) => r#"{"opened":true}"#.to_string(),
        Err(e) => format!(r#"{{"kind":"{:?}"}}"#, e.kind()),
    };
    reply(0, body.into_bytes())
}

fn random_draw() -> i64 {
    let mut bytes = [0u8; 8];
    let errno = unsafe { random_get(bytes.as_mut_ptr(), bytes.len()) };
    let mut hex = String::new();
    for b in bytes {
        let _ = write!(hex, "{b:02x}");
    }
    reply(0, format!(r#"{{"errno":{errno},"random":"{hex}"}}"#).into_bytes())
}

fn counter() -> i64 {
    static COUNTER: AtomicU32 = AtomicU32::new(0);
    let n = COUNTER.fetch_add(1, Ordering::SeqCst) + 1;
    reply(0, format!(r#"{{"counter":{n}}}"#).into_bytes())
}

fn standard_streams() -> i64 {
    println!("stdout line");
    eprintln!("stderr line");
    reply(0, b"{}".to_vec())
}

fn args_and_vars() -> i64 {
    let body = format!(
        r#"{{"args":{},"vars":{}}}"#,
        std::env::args().count(),
        std::env::vars().count()
    );
    reply(0, body.into_bytes())
}

fn raw(input: &[u8]) -> i64 {
    match input.split_first() {
        Some((tag, body)) => reply(*tag, body.to_vec()),
        None => reply(2, br#"{"message":"raw needs a tag byte"}"#.to_vec()),
    }
}

fn bad_pointer() -> i64 {
    unsafe { host_log(1, 0xffff_fff0usize as *const u8, 64) };
    reply(0, b"{}".to_vec())
}
