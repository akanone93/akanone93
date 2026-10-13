// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Bindings to `plux_functions_native`, built by `hook/build.dart` (E9). Only
/// the worker isolate (`worker.dart`) calls these, apart from the
/// cancellation handle, which the engine drives from the calling isolate.
///
/// The C interface and its contract are in `native/plux_functions.h`.
library;

import 'dart:ffi';

/// A loaded, checked module.
final class PfModule extends Opaque {}

/// The cancellation handle of one invocation.
final class PfInvocation extends Opaque {}

/// `pf_limits`: the limits of one invocation.
final class PfLimits extends Struct {
  /// Wall-clock limit in milliseconds.
  @Uint32()
  external int timeoutMs;

  /// Bytes the engine may allocate for the call.
  @Uint64()
  external int memoryBytes;

  /// Largest result in bytes, tag byte excluded.
  @Uint32()
  external int outputBytes;

  /// Log bytes forwarded to the host.
  @Uint32()
  external int logBytes;
}

/// The host callbacks the engine's `plux_v1` imports call.
typedef PfLogNative = Void Function(Int32, Pointer<Uint8>, Uint32);

/// `clock_now`.
typedef PfClockNative = Int64 Function();

/// `context`.
typedef PfContextNative = Int32 Function(
  Pointer<Pointer<Uint8>>,
  Pointer<Uint32>,
);

/// `env_get` and `kv_get`.
typedef PfReadNative = Int32 Function(
  Pointer<Uint8>,
  Uint32,
  Pointer<Pointer<Uint8>>,
  Pointer<Uint32>,
);

/// `kv_put`.
typedef PfWriteNative = Int32 Function(
  Pointer<Uint8>,
  Uint32,
  Pointer<Uint8>,
  Uint32,
);

/// `kv_delete`.
typedef PfDeleteNative = Int32 Function(Pointer<Uint8>, Uint32);

/// `pf_host`: function pointers into the worker isolate.
final class PfHost extends Struct {
  /// `log`.
  external Pointer<NativeFunction<PfLogNative>> log;

  /// `clock_now`.
  external Pointer<NativeFunction<PfClockNative>> clockNow;

  /// `context`.
  external Pointer<NativeFunction<PfContextNative>> context;

  /// `env_get`.
  external Pointer<NativeFunction<PfReadNative>> envGet;

  /// `kv_get`.
  external Pointer<NativeFunction<PfReadNative>> kvGet;

  /// `kv_put`.
  external Pointer<NativeFunction<PfWriteNative>> kvPut;

  /// `kv_delete`.
  external Pointer<NativeFunction<PfDeleteNative>> kvDelete;
}

/// Starts the runtime; zero on success.
@Native<Int32 Function()>(symbol: 'pf_init')
external int pfInit();

/// The WAMR version of the build.
@Native<Pointer<Uint8> Function()>(symbol: 'pf_runtime_version', isLeaf: true)
external Pointer<Uint8> pfRuntimeVersion();

/// Allocates memory the engine and Dart both release with [pfFree].
@Native<Pointer<Uint8> Function(Size)>(symbol: 'pf_alloc', isLeaf: true)
external Pointer<Uint8> pfAlloc(int size);

/// Releases [pfAlloc] memory.
@Native<Void Function(Pointer<Void>)>(symbol: 'pf_free', isLeaf: true)
external void pfFree(Pointer<Void> pointer);

/// Loads and checks a module; null on refusal, with the PLX code in
/// [status] and a diagnostic in [error].
@Native<
  Pointer<PfModule> Function(
    Pointer<Uint8>,
    Uint32,
    Pointer<Int32>,
    Pointer<Uint8>,
    Uint32,
  )
>(symbol: 'pf_module_load')
external Pointer<PfModule> pfModuleLoad(
  Pointer<Uint8> bytes,
  int length,
  Pointer<Int32> status,
  Pointer<Uint8> error,
  int errorCapacity,
);

/// Releases a module.
@Native<Void Function(Pointer<PfModule>)>(symbol: 'pf_module_free')
external void pfModuleFree(Pointer<PfModule> module);

/// Creates a cancellation handle.
@Native<Pointer<PfInvocation> Function()>(
  symbol: 'pf_invocation_new',
  isLeaf: true,
)
external Pointer<PfInvocation> pfInvocationNew();

/// Releases a cancellation handle.
@Native<Void Function(Pointer<PfInvocation>)>(
  symbol: 'pf_invocation_free',
  isLeaf: true,
)
external void pfInvocationFree(Pointer<PfInvocation> invocation);

/// Ends an invocation as cancelled; safe from any thread.
@Native<Void Function(Pointer<PfInvocation>)>(symbol: 'pf_cancel', isLeaf: true)
external void pfCancel(Pointer<PfInvocation> invocation);

/// Runs a function; not a leaf call, because the host callbacks re-enter Dart.
@Native<
  Int32 Function(
    Pointer<PfModule>,
    Uint32,
    Pointer<Uint8>,
    Uint32,
    Pointer<PfLimits>,
    Pointer<PfHost>,
    Pointer<PfInvocation>,
    Pointer<Pointer<Uint8>>,
    Pointer<Uint32>,
  )
>(symbol: 'pf_invoke')
external int pfInvoke(
  Pointer<PfModule> module,
  int index,
  Pointer<Uint8> input,
  int inputLength,
  Pointer<PfLimits> limits,
  Pointer<PfHost> host,
  Pointer<PfInvocation> invocation,
  Pointer<Pointer<Uint8>> out,
  Pointer<Uint32> outLength,
);
