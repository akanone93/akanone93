// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//! Guest-side SDK for Plux Functions (ABI v1, ADR-0062).
//!
//! A function is a plain Rust function `fn(&C, I) -> Result<O, BusinessError>`
//! where `C` is [`DeviceContext`] or [`ServerContext`]. The context type fixes
//! the placement at compile time: a device handler cannot name `http`,
//! `secret`, `claims` or `invoke` (`FN-008`, `FN-052`). [`function!`] exports
//! the handler from its crate; [`module!`] links one or more such crates into
//! a WebAssembly module with the ABI exports.

#![deny(unsafe_code)]

#[cfg(target_arch = "wasm32")]
#[allow(unsafe_code)]
mod abi;
mod base64;
mod context;
mod error;
mod host;
mod http;
mod macros;
#[doc(hidden)]
pub mod rt;
#[cfg(not(target_arch = "wasm32"))]
pub mod testing;

pub use context::{Claims, ContextKind, DeviceContext, Kv, ServerContext};
pub use error::{BusinessError, HostError, InvalidCode, InvokeError};
pub use host::{Host, Level, Placement};
pub use http::{Http, HttpRequest, HttpResponse};

/// The placement a handler's context type fixes. Used by [`function!`].
#[doc(hidden)]
pub const fn placement_of<C, I, O, F>(_handler: &F) -> Placement
where
    C: ContextKind,
    F: Fn(&C, I) -> Result<O, BusinessError>,
{
    C::PLACEMENT
}
