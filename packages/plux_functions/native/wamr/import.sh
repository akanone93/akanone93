#!/bin/sh
# SPDX-FileCopyrightText: 2026 Plux contributors
# SPDX-License-Identifier: Apache-2.0
#
# Re-creates the WebAssembly Micro Runtime subset vendored here (the fast
# interpreter, E9 and E10 of docs/plans/p7.md) from the upstream source tree
# of tag WAMR-2.4.3, then checks every file against SHA256SUMS.
#
#   WAMR_SRC=<dir> ./import.sh      use an extracted source tree
#   ./import.sh                     download the tag archive (needs curl, tar)
#   UPDATE=1 ./import.sh            also rewrite SHA256SUMS (a version bump)
#
# The tree is the root of the upstream repository: it holds core/ and
# LICENSE. Review the diff before committing. Only the files below are
# copied; AOT, JIT, WASI libraries, sockets and files stay out (FN-052).
set -eu

TAG=WAMR-2.4.3
URL=https://github.com/bytecodealliance/wasm-micro-runtime/archive/refs/tags/$TAG.tar.gz

here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if [ -n "${WAMR_SRC:-}" ]; then
	src=$WAMR_SRC
else
	curl -fsSL "$URL" -o "$tmp/wamr.tar.gz"
	mkdir "$tmp/src"
	tar -xzf "$tmp/wamr.tar.gz" -C "$tmp/src" --strip-components=1
	src=$tmp/src
fi
[ -f "$src/core/version.h" ] || { echo "no WAMR tree at $src" >&2; exit 1; }
grep -q 'WAMR_VERSION_MINOR 4' "$src/core/version.h" &&
	grep -q 'WAMR_VERSION_PATCH 3' "$src/core/version.h" ||
	{ echo "the tree is not WAMR 2.4.3" >&2; exit 1; }

FILES="
LICENSE
core/config.h
core/version.h
core/iwasm/aot/aot_runtime.h
core/iwasm/common/arch/invokeNative_general.c
core/iwasm/common/wasm_application.c
core/iwasm/common/wasm_blocking_op.c
core/iwasm/common/wasm_c_api.c
core/iwasm/common/wasm_c_api_internal.h
core/iwasm/common/wasm_exec_env.c
core/iwasm/common/wasm_exec_env.h
core/iwasm/common/wasm_loader_common.c
core/iwasm/common/wasm_loader_common.h
core/iwasm/common/wasm_memory.c
core/iwasm/common/wasm_memory.h
core/iwasm/common/wasm_native.c
core/iwasm/common/wasm_native.h
core/iwasm/common/wasm_runtime_common.c
core/iwasm/common/wasm_runtime_common.h
core/iwasm/common/wasm_shared_memory.c
core/iwasm/common/wasm_shared_memory.h
core/iwasm/common/wasm_suspend_flags.h
core/iwasm/compilation/aot.h
core/iwasm/include/lib_export.h
core/iwasm/include/wasm_c_api.h
core/iwasm/include/wasm_export.h
core/iwasm/interpreter/wasm.h
core/iwasm/interpreter/wasm_interp.h
core/iwasm/interpreter/wasm_interp_fast.c
core/iwasm/interpreter/wasm_loader.c
core/iwasm/interpreter/wasm_loader.h
core/iwasm/interpreter/wasm_opcode.h
core/iwasm/interpreter/wasm_runtime.c
core/iwasm/interpreter/wasm_runtime.h
core/iwasm/libraries/thread-mgr/thread_manager.c
core/iwasm/libraries/thread-mgr/thread_manager.h
core/shared/mem-alloc/ems/ems_alloc.c
core/shared/mem-alloc/ems/ems_gc.c
core/shared/mem-alloc/ems/ems_gc.h
core/shared/mem-alloc/ems/ems_gc_internal.h
core/shared/mem-alloc/ems/ems_hmu.c
core/shared/mem-alloc/ems/ems_kfc.c
core/shared/mem-alloc/mem_alloc.c
core/shared/mem-alloc/mem_alloc.h
core/shared/platform/android/platform_init.c
core/shared/platform/android/platform_internal.h
core/shared/platform/common/memory/mremap.c
core/shared/platform/common/posix/posix_blocking_op.c
core/shared/platform/common/posix/posix_malloc.c
core/shared/platform/common/posix/posix_memmap.c
core/shared/platform/common/posix/posix_sleep.c
core/shared/platform/common/posix/posix_thread.c
core/shared/platform/common/posix/posix_time.c
core/shared/platform/darwin/platform_init.c
core/shared/platform/darwin/platform_internal.h
core/shared/platform/include/platform_api_extension.h
core/shared/platform/include/platform_api_vmcore.h
core/shared/platform/include/platform_common.h
core/shared/platform/include/platform_wasi_types.h
core/shared/platform/linux/platform_init.c
core/shared/platform/linux/platform_internal.h
core/shared/utils/bh_assert.c
core/shared/utils/bh_assert.h
core/shared/utils/bh_atomic.h
core/shared/utils/bh_bitmap.c
core/shared/utils/bh_bitmap.h
core/shared/utils/bh_common.c
core/shared/utils/bh_common.h
core/shared/utils/bh_hashmap.c
core/shared/utils/bh_hashmap.h
core/shared/utils/bh_leb128.c
core/shared/utils/bh_leb128.h
core/shared/utils/bh_list.c
core/shared/utils/bh_list.h
core/shared/utils/bh_log.c
core/shared/utils/bh_log.h
core/shared/utils/bh_platform.h
core/shared/utils/bh_queue.c
core/shared/utils/bh_queue.h
core/shared/utils/bh_vector.c
core/shared/utils/bh_vector.h
core/shared/utils/gnuc.h
core/shared/utils/runtime_timer.c
core/shared/utils/runtime_timer.h
"

rm -rf "${here:?}/core" "${here:?}/LICENSE"
for f in $FILES; do
	mkdir -p "$here/$(dirname "$f")"
	cp "$src/$f" "$here/$f"
done

cd "$here"
if [ "${UPDATE:-0}" = 1 ]; then
	# shellcheck disable=SC2086
	LC_ALL=C sha256sum $(printf '%s\n' $FILES | LC_ALL=C sort) > SHA256SUMS
	echo "rewrote SHA256SUMS"
fi
sha256sum -c --quiet SHA256SUMS
printf 'imported WAMR %s into %s\n' "$TAG" "$here"
