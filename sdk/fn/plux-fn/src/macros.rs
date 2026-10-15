// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

//! The macros that export a handler and link handlers into a module.

/// Exports a handler from its crate.
///
/// The handler is `fn(&C, I) -> Result<O, BusinessError>` with `C` a
/// [`DeviceContext`](crate::DeviceContext) or
/// [`ServerContext`](crate::ServerContext), `I: DeserializeOwned + JsonSchema`
/// and `O: Serialize + JsonSchema`. The macro emits three hidden items that
/// [`module!`](crate::module) consumes: `__PLUX_PLACEMENT`, `__plux_entry` and
/// `__plux_schemas` (`FN-004`), plus `__plux_entry_with` for tests that
/// supply their own host.
///
/// ```
/// use plux_fn::{BusinessError, DeviceContext};
///
/// #[derive(serde::Deserialize, schemars::JsonSchema)]
/// struct In { n: u32 }
/// #[derive(serde::Serialize, schemars::JsonSchema)]
/// struct Out { doubled: u32 }
///
/// fn double(_ctx: &DeviceContext, input: In) -> Result<Out, BusinessError> {
///     Ok(Out { doubled: input.n * 2 })
/// }
/// plux_fn::function!(double);
///
/// assert_eq!(__PLUX_PLACEMENT, plux_fn::Placement::Device);
/// ```
#[macro_export]
macro_rules! function {
    ($handler:path) => {
        /// The placement the handler's context type fixes.
        #[doc(hidden)]
        pub const __PLUX_PLACEMENT: $crate::Placement = $crate::placement_of(&$handler);

        /// ABI entry: decodes the input, runs the handler, tags the result.
        #[doc(hidden)]
        pub fn __plux_entry(input: &[u8]) -> ::std::vec::Vec<u8> {
            $crate::rt::entry(input, &$handler)
        }

        /// As `__plux_entry`, with an explicit host (native tests).
        #[doc(hidden)]
        pub fn __plux_entry_with(
            host: &'static dyn $crate::Host,
            input: &[u8],
        ) -> ::std::vec::Vec<u8> {
            $crate::rt::entry_with(host, input, &$handler)
        }

        /// The input and output JSON Schemas, `{"input": …, "output": …}`.
        #[doc(hidden)]
        pub fn __plux_schemas() -> ::std::string::String {
            $crate::rt::schemas(&$handler)
        }
    };
}

#[doc(hidden)]
#[macro_export]
macro_rules! __placement {
    (device) => {
        $crate::Placement::Device
    };
    (server) => {
        $crate::Placement::Server
    };
}

/// Links function crates into one module and, on `wasm32`, emits the ABI
/// exports (`plux_abi_version`, `plux_alloc`, `plux_invoke`, and
/// `plux_schemas` behind the wrapper's `plux-schema` feature). Dispatch is by
/// position in the list.
///
/// Each entry is `crate: placement`. The placement is what the function's
/// document declares, and the build fails when the crate's handler disagrees
/// (E5, `PLX-7205`).
///
/// ```
/// use plux_fn::{BusinessError, DeviceContext};
///
/// #[derive(serde::Deserialize, schemars::JsonSchema)]
/// struct In {}
/// #[derive(serde::Serialize, schemars::JsonSchema)]
/// struct Out {}
///
/// mod ping {
///     use super::*;
///     fn handler(_: &DeviceContext, _: In) -> Result<Out, BusinessError> { Ok(Out {}) }
///     plux_fn::function!(handler);
/// }
/// plux_fn::module!(ping: device);
/// # fn main() {}
/// ```
///
/// A document that declares `server` for a device handler does not compile:
///
/// ```compile_fail
/// use plux_fn::{BusinessError, DeviceContext};
///
/// #[derive(serde::Deserialize, schemars::JsonSchema)]
/// struct In {}
/// #[derive(serde::Serialize, schemars::JsonSchema)]
/// struct Out {}
///
/// mod ping {
///     use super::*;
///     fn handler(_: &DeviceContext, _: In) -> Result<Out, BusinessError> { Ok(Out {}) }
///     plux_fn::function!(handler);
/// }
/// plux_fn::module!(ping: server);
/// # fn main() {}
/// ```
#[macro_export]
macro_rules! module {
    ($($krate:ident : $placement:ident),+ $(,)?) => {
        $(
            const _: () = ::core::assert!(
                ($krate::__PLUX_PLACEMENT as u8) == ($crate::__placement!($placement) as u8),
                ::core::concat!(
                    "PLX-7205: the placement of `",
                    ::core::stringify!($krate),
                    "` differs from the one its function document declares"
                ),
            );
        )+

        #[cfg(target_arch = "wasm32")]
        const _: () = {
            const ENTRIES: &[$crate::rt::EntryFn] = &[$($krate::__plux_entry),+];

            #[allow(unsafe_code)]
            #[unsafe(no_mangle)]
            pub extern "C" fn plux_abi_version() -> i32 {
                1
            }

            #[allow(unsafe_code)]
            #[unsafe(no_mangle)]
            pub extern "C" fn plux_alloc(len: i32) -> i32 {
                $crate::rt::alloc(len)
            }

            #[allow(unsafe_code)]
            #[unsafe(no_mangle)]
            pub extern "C" fn plux_invoke(index: i32, ptr: i32, len: i32) -> i64 {
                $crate::rt::invoke(ENTRIES, index, ptr, len)
            }

            #[cfg(feature = "plux-schema")]
            #[allow(unsafe_code)]
            #[unsafe(no_mangle)]
            pub extern "C" fn plux_schemas(index: i32) -> i64 {
                const SCHEMAS: &[$crate::rt::SchemasFn] =
                    &[$($krate::__plux_schemas),+];
                $crate::rt::schemas_of_index(SCHEMAS, index)
            }
        };
    };
}
