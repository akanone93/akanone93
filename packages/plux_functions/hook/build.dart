// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/// Builds `plux_functions_native` — the WebAssembly Micro Runtime's fast
/// interpreter and the engine's C layer — from source for every target the
/// Flutter tool asks for (E9 of docs/plans/p7.md, FN-050).
library;

import 'package:code_assets/code_assets.dart';
import 'package:hooks/hooks.dart';
import 'package:native_toolchain_c/native_toolchain_c.dart';

const _wamr = 'native/wamr/core';

/// The runtime sources compiled on every target (native/wamr/SHA256SUMS lists
/// the vendored files; AOT, JIT, WASI libraries, sockets and files are not
/// among them).
const _common = [
  '$_wamr/iwasm/common/arch/invokeNative_general.c',
  '$_wamr/iwasm/common/wasm_application.c',
  '$_wamr/iwasm/common/wasm_blocking_op.c',
  '$_wamr/iwasm/common/wasm_c_api.c',
  '$_wamr/iwasm/common/wasm_exec_env.c',
  '$_wamr/iwasm/common/wasm_loader_common.c',
  '$_wamr/iwasm/common/wasm_memory.c',
  '$_wamr/iwasm/common/wasm_native.c',
  '$_wamr/iwasm/common/wasm_runtime_common.c',
  '$_wamr/iwasm/common/wasm_shared_memory.c',
  '$_wamr/iwasm/interpreter/wasm_interp_fast.c',
  '$_wamr/iwasm/interpreter/wasm_loader.c',
  '$_wamr/iwasm/interpreter/wasm_runtime.c',
  '$_wamr/iwasm/libraries/thread-mgr/thread_manager.c',
  '$_wamr/shared/mem-alloc/ems/ems_alloc.c',
  '$_wamr/shared/mem-alloc/ems/ems_gc.c',
  '$_wamr/shared/mem-alloc/ems/ems_hmu.c',
  '$_wamr/shared/mem-alloc/ems/ems_kfc.c',
  '$_wamr/shared/mem-alloc/mem_alloc.c',
  '$_wamr/shared/platform/common/memory/mremap.c',
  '$_wamr/shared/platform/common/posix/posix_blocking_op.c',
  '$_wamr/shared/platform/common/posix/posix_malloc.c',
  '$_wamr/shared/platform/common/posix/posix_memmap.c',
  '$_wamr/shared/platform/common/posix/posix_sleep.c',
  '$_wamr/shared/platform/common/posix/posix_thread.c',
  '$_wamr/shared/platform/common/posix/posix_time.c',
  '$_wamr/shared/utils/bh_assert.c',
  '$_wamr/shared/utils/bh_bitmap.c',
  '$_wamr/shared/utils/bh_common.c',
  '$_wamr/shared/utils/bh_hashmap.c',
  '$_wamr/shared/utils/bh_leb128.c',
  '$_wamr/shared/utils/bh_list.c',
  '$_wamr/shared/utils/bh_log.c',
  '$_wamr/shared/utils/bh_queue.c',
  '$_wamr/shared/utils/bh_vector.c',
  '$_wamr/shared/utils/runtime_timer.c',
];

/// WAMR's name for the platform directory and define of [os].
({String dir, String define}) _platform(OS os) {
  if (os == OS.linux) return (dir: 'linux', define: 'BH_PLATFORM_LINUX');
  if (os == OS.android) return (dir: 'android', define: 'BH_PLATFORM_ANDROID');
  if (os == OS.iOS || os == OS.macOS) {
    return (dir: 'darwin', define: 'BH_PLATFORM_DARWIN');
  }
  throw UnsupportedError('plux_functions does not support $os');
}

/// WAMR's `BUILD_TARGET_*` macro for [arch].
String _target(Architecture arch) {
  if (arch == Architecture.x64) return 'BUILD_TARGET_X86_64';
  if (arch == Architecture.arm64) return 'BUILD_TARGET_AARCH64';
  if (arch == Architecture.arm) return 'BUILD_TARGET_ARM';
  if (arch == Architecture.ia32) return 'BUILD_TARGET_X86_32';
  if (arch == Architecture.riscv64) return 'BUILD_TARGET_RISCV64_LP64D';
  throw UnsupportedError('plux_functions does not support $arch');
}

void main(List<String> args) async {
  await build(args, (input, output) async {
    if (!input.config.buildCodeAssets) return;
    final code = input.config.code;
    final os = code.targetOS;
    final platform = _platform(os);
    final apple = os == OS.iOS || os == OS.macOS;
    await CBuilder.library(
      name: 'plux_functions_native',
      assetName: 'src/native/plux_functions_native.dart',
      sources: [
        'native/plux_functions.c',
        ..._common,
        '$_wamr/shared/platform/${platform.dir}/platform_init.c',
      ],
      includes: [
        'native',
        '$_wamr/iwasm/include',
        '$_wamr/iwasm/common',
        '$_wamr/iwasm/interpreter',
        '$_wamr/iwasm/libraries/thread-mgr',
        '$_wamr/shared/utils',
        '$_wamr/shared/mem-alloc',
        '$_wamr/shared/platform/include',
        '$_wamr/shared/platform/${platform.dir}',
        _wamr,
      ],
      defines: {
        platform.define: null,
        _target(code.targetArchitecture): null,
        // The fast interpreter only: no AOT, no JIT, no WASI library, no
        // built-in libc, no second module, no threads for the guest.
        'WASM_ENABLE_INTERP': '1',
        'WASM_ENABLE_FAST_INTERP': '1',
        'WASM_ENABLE_AOT': '0',
        'WASM_ENABLE_JIT': '0',
        'WASM_ENABLE_FAST_JIT': '0',
        'WASM_ENABLE_LIBC_BUILTIN': '0',
        'WASM_ENABLE_LIBC_WASI': '0',
        'WASM_ENABLE_MULTI_MODULE': '0',
        'WASM_ENABLE_SHARED_MEMORY': '0',
        // What Rust's wasm32-wasip1 emits by default.
        'WASM_ENABLE_BULK_MEMORY': '1',
        'WASM_ENABLE_BULK_MEMORY_OPT': '1',
        'WASM_ENABLE_REF_TYPES': '1',
        // wasm-ld pads the table operand of call_indirect.
        'WASM_ENABLE_CALL_INDIRECT_OVERLONG': '1',
        // The thread manager only supplies termination: the watchdog ends a
        // running instance through it (FN-053).
        'WASM_ENABLE_THREAD_MGR': '1',
        // Software bounds checks: no signal handlers in the host process.
        // The guest's linear memory goes through the engine's allocator, which
        // enforces the memory limit (FN-053).
        'WASM_MEM_ALLOC_WITH_USAGE': '1',
        'WASM_DISABLE_HW_BOUND_CHECK': '1',
        'WASM_DISABLE_STACK_HW_BOUND_CHECK': '1',
        'WASM_DISABLE_WAKEUP_BLOCKING_OP': '1',
        'BH_MALLOC': 'wasm_runtime_malloc',
        'BH_FREE': 'wasm_runtime_free',
        // The runtime's own API stays hidden; only pf_* is exported.
        'WASM_RUNTIME_API_EXTERN': '',
        'WASM_API_EXTERN': '',
      },
      flags: [
        '-fvisibility=hidden',
        '-ffunction-sections',
        '-fdata-sections',
        '-Wno-unused-parameter',
        // An unresolved symbol is a build error, not a crash on first call.
        if (apple) '-Wl,-dead_strip' else '-Wl,--gc-sections,--no-undefined',
        // Google Play requires 16 KiB page alignment for Android 15+.
        if (os == OS.android) '-Wl,-z,max-page-size=16384',
      ],
      libraries: [
        if (os == OS.linux) ...['pthread', 'm'],
        // WAMR's Android platform layer logs through liblog
        // (__android_log_vprint in platform_init.c).
        if (os == OS.android) 'log',
      ],
      optimizationLevel: OptimizationLevel.o3,
    ).run(input: input, output: output);
  });
}
