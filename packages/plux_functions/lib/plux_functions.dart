// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// The Plux device function engine: runs Plux Functions compiled to
/// WebAssembly on the device, in WAMR's interpreter, off the UI isolate
/// (FN-050, FN-052, FN-053).
///
/// An app adds this optional package to run `device`-placed functions; an
/// app without it pays nothing in size, and a device-placed
/// `invokeFunction` fails with `PLX-5401` (RT-060, E9 of docs/plans/p7.md).
library;

export 'src/cancellation.dart' show FunctionCancellation;
export 'src/engine.dart' show FunctionEngine, FunctionModule;
export 'src/host.dart' show DeviceFunctionHost, FunctionLogLevel, HostStatus;
export 'src/limits.dart' show FunctionLimits;
export 'src/outcome.dart'
    show FunctionBusinessError, FunctionOutcome, FunctionOutput;
export 'src/plux_functions.dart' show PluxFunctions, RuntimeFunctionHost;
