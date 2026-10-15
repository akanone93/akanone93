// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/*
 * The device function engine over WAMR's fast interpreter (FN-050, FN-052,
 * FN-053, ADR-0062, E6 and E9 of docs/plans/p7.md).
 *
 * What the engine guarantees, and where:
 *  - a module that imports a server-only function or anything unknown, or
 *    that exports a different ABI, is refused at load (check_module);
 *  - every invocation runs on a fresh instance, so no state survives it;
 *  - wall-clock time is bounded by a watchdog thread that terminates the
 *    instance (watchdog_main), and an invocation can be cancelled (pf_cancel);
 *  - memory is bounded by an allocator that counts every byte the invocation
 *    requests from the runtime, linear memory growth included (pf_malloc);
 *  - the guest sees no file, socket, environment or argument (the WASI shim),
 *    and what it writes to stdout or stderr is forwarded, bounded, as a log.
 *
 * Nothing here logs, stores or returns guest data except the result the
 * guest produced for the caller and its own refusal message.
 */

#include <errno.h>
#include <pthread.h>
#include <stdio.h>
#include <stdatomic.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <unistd.h>

#include "plux_functions.h"
#include "wasm_export.h"

#if defined(__APPLE__) || defined(__ANDROID__)
#define PF_USE_ARC4RANDOM 1
#endif

/* The wasm operand stack of one call. A guest that recurses deeper traps. */
#define PF_OPERAND_STACK (128u * 1024u)
/* The watchdog's own stack; it runs a few lines of code. */
#define PF_WATCHDOG_STACK (64u * 1024u)
/* A load runs plux_abi_version() only; this is its fixed budget. */
#define PF_PROBE_TIMEOUT_MS 2000u
#define PF_PROBE_MEMORY_BYTES (16u * 1024u * 1024u)
/* The longest guest refusal message handed back (tags 2 and 3). */
#define PF_MESSAGE_MAX 4096u

#define ERRNO_BADF 8u
#define ERRNO_INVAL 28u
#define ERRNO_NOSYS 52u
#define ERRNO_FAULT 21u
#define ERRNO_IO 29u

#define STATUS_NOT_FOUND (-1)
#define STATUS_LIMIT (-3)
#define STATUS_INVALID (-6)
#define STATUS_UNAVAILABLE (-7)

/* ---- the runtime's allocator: counts bytes per invocation ------------- */

typedef struct pf_run pf_run;

struct pf_invocation {
	pthread_mutex_t mutex;
	wasm_module_inst_t instance; /* set while a call runs; under mutex */
	int cancelled;               /* under mutex */
	int reason;                  /* PF_TIMEOUT or PF_CANCELLED; under mutex */
};

struct pf_run {
	const pf_host *host;
	pf_limits limits;
	uint64_t memory_used;
	int memory_denied;
	uint32_t log_used;
	int exited;
	int32_t exit_code;
	wasm_function_inst_t alloc;
};

struct pf_module {
	wasm_module_t module;
	uint8_t *code;
};

/* The run the calling thread executes; the allocator reads it. */
static _Thread_local pf_run *current_run;

typedef struct block_header {
	uint64_t size;
	uint64_t counted;
} block_header;

_Static_assert(sizeof(block_header) == 16, "blocks stay 16-byte aligned");

/* The runtime's allocator. WASM_MEM_ALLOC_WITH_USAGE routes the guest's
 * linear memory through it too (otherwise WAMR maps that memory itself, out
 * of reach of the limit). Linear memory must start zeroed and grow zeroed,
 * which the system allocator does not promise. */
static void *pf_malloc(mem_alloc_usage_t usage, unsigned int size) {
	pf_run *run = current_run;
	if (run != NULL && run->memory_used + size > run->limits.memory_bytes) {
		run->memory_denied = 1;
		return NULL;
	}
	block_header *block = malloc(sizeof(block_header) + (size_t)size);
	if (block == NULL) {
		if (run != NULL) {
			run->memory_denied = 1;
		}
		return NULL;
	}
	block->size = size;
	block->counted = run != NULL;
	if (run != NULL) {
		run->memory_used += size;
	}
	if (usage == Alloc_For_LinearMemory) {
		memset(block + 1, 0, size);
	}
	return block + 1;
}

static void pf_release(mem_alloc_usage_t usage, void *ptr) {
	if (ptr == NULL) {
		return;
	}
	block_header *block = (block_header *)ptr - 1;
	if (block->counted && current_run != NULL) {
		current_run->memory_used -= block->size;
	}
	free(block);
}

static void *pf_realloc(mem_alloc_usage_t usage, bool full_size_mmaped,
                        void *ptr, unsigned int size) {
	if (ptr == NULL) {
		return pf_malloc(usage, size);
	}
	block_header *old = (block_header *)ptr - 1;
	uint64_t old_size = old->size;
	pf_run *run = current_run;
	if (old->counted && run != NULL &&
	    run->memory_used - old_size + size > run->limits.memory_bytes) {
		run->memory_denied = 1;
		return NULL;
	}
	block_header *block = realloc(old, sizeof(block_header) + (size_t)size);
	if (block == NULL) {
		if (run != NULL) {
			run->memory_denied = 1;
		}
		return NULL;
	}
	if (block->counted && run != NULL) {
		run->memory_used = run->memory_used - old_size + size;
	}
	block->size = size;
	if (usage == Alloc_For_LinearMemory && size > old_size) {
		memset((uint8_t *)(block + 1) + old_size, 0, size - old_size);
	}
	return block + 1;
}

/* ---- small helpers ----------------------------------------------------- */

PF_API void *pf_alloc(size_t size) { return malloc(size == 0 ? 1 : size); }

PF_API void pf_free(void *ptr) { free(ptr); }

static void set_error(char *buffer, uint32_t capacity, const char *message) {
	if (buffer == NULL || capacity == 0) {
		return;
	}
	strncpy(buffer, message, capacity - 1);
	buffer[capacity - 1] = '\0';
}

static uint8_t *duplicate(const char *text, uint32_t *length) {
	size_t n = strlen(text);
	uint8_t *copy = pf_alloc(n);
	if (copy == NULL) {
		*length = 0;
		return NULL;
	}
	memcpy(copy, text, n);
	*length = (uint32_t)n;
	return copy;
}

static int64_t monotonic_ns(void) {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (int64_t)ts.tv_sec * 1000000000LL + ts.tv_nsec;
}

static int64_t realtime_ns(void) {
	struct timespec ts;
	clock_gettime(CLOCK_REALTIME, &ts);
	return (int64_t)ts.tv_sec * 1000000000LL + ts.tv_nsec;
}

/* ---- guest memory ------------------------------------------------------ */

static pf_run *run_of(wasm_exec_env_t env) {
	return (pf_run *)wasm_runtime_get_user_data(env);
}

static int in_range(wasm_module_inst_t inst, uint32_t offset, uint32_t length) {
	return length == 0 || wasm_runtime_validate_app_addr(inst, offset, length);
}

/* Copies host bytes into a buffer the guest allocates, and returns them
 * packed (ptr << 32) | len, or a negative status (ADR-0062). */
static int64_t give(wasm_exec_env_t env, const uint8_t *data, uint32_t length) {
	if (length == 0) {
		return 0;
	}
	pf_run *run = run_of(env);
	wasm_module_inst_t inst = wasm_runtime_get_module_inst(env);
	uint32_t argv[2] = {length, 0};
	if (run->alloc == NULL ||
	    !wasm_runtime_call_wasm(env, run->alloc, 1, argv)) {
		return STATUS_LIMIT;
	}
	uint32_t ptr = argv[0];
	if (ptr == 0 || !in_range(inst, ptr, length)) {
		return STATUS_LIMIT;
	}
	memcpy(wasm_runtime_addr_app_to_native(inst, ptr), data, length);
	return ((int64_t)ptr << 32) | length;
}

/* ---- natives ------------------------------------------------------------
 *
 * Every native is registered raw (wasm_runtime_register_natives_raw): the
 * interpreter hands it the guest's arguments as an array of 64-bit slots,
 * an i32 in the low half, and takes the result back from slot 0. That keeps
 * the call independent of the platform's calling convention, so no per-CPU
 * assembly is needed and the C invoker of invokeNative_general.c is only
 * linked, never used.
 */

#define IN32(i) (*(uint32_t *)&args[i])
#define IN64(i) (*(int64_t *)&args[i])
#define OUT32(v) (*(uint32_t *)&args[0] = (uint32_t)(v))
#define OUT64(v) (*(int64_t *)&args[0] = (int64_t)(v))

/* ---- plux_v1 ----------------------------------------------------------- */

static void do_log(wasm_exec_env_t env, int32_t level, uint32_t ptr,
                   uint32_t length) {
	pf_run *run = run_of(env);
	wasm_module_inst_t inst = wasm_runtime_get_module_inst(env);
	if (run->host == NULL || run->host->log == NULL ||
	    !in_range(inst, ptr, length)) {
		return;
	}
	uint32_t room = run->limits.log_bytes - run->log_used;
	if (length > room) {
		length = room;
	}
	if (length == 0) {
		return;
	}
	run->log_used += length;
	run->host->log(level < 0 ? 0 : (level > 3 ? 3 : level),
	               wasm_runtime_addr_app_to_native(inst, ptr), length);
}

static void host_log(wasm_exec_env_t env, uint64_t *args) {
	do_log(env, (int32_t)IN32(0), IN32(1), IN32(2));
}

static void host_clock_now(wasm_exec_env_t env, uint64_t *args) {
	pf_run *run = run_of(env);
	OUT64(run->host != NULL && run->host->clock_now != NULL
	          ? run->host->clock_now()
	          : realtime_ns());
}

static void host_context(wasm_exec_env_t env, uint64_t *args) {
	pf_run *run = run_of(env);
	if (run->host == NULL || run->host->context == NULL) {
		OUT64(STATUS_UNAVAILABLE);
		return;
	}
	uint8_t *out = NULL;
	uint32_t length = 0;
	int32_t status = run->host->context(&out, &length);
	int64_t packed = status < 0 ? status : give(env, out, length);
	pf_free(out);
	OUT64(packed);
}

typedef int32_t (*keyed_read)(const uint8_t *, uint32_t, uint8_t **, uint32_t *);

static int64_t keyed_get(wasm_exec_env_t env, keyed_read read, uint32_t ptr,
                         uint32_t length) {
	wasm_module_inst_t inst = wasm_runtime_get_module_inst(env);
	if (read == NULL) {
		return STATUS_UNAVAILABLE;
	}
	if (length == 0 || !in_range(inst, ptr, length)) {
		return STATUS_INVALID;
	}
	uint8_t *out = NULL;
	uint32_t out_length = 0;
	int32_t status = read(wasm_runtime_addr_app_to_native(inst, ptr), length,
	                      &out, &out_length);
	int64_t packed = status < 0 ? status : give(env, out, out_length);
	pf_free(out);
	return packed;
}

static void host_env_get(wasm_exec_env_t env, uint64_t *args) {
	pf_run *run = run_of(env);
	uint32_t ptr = IN32(0), length = IN32(1);
	OUT64(keyed_get(env, run->host ? run->host->env_get : NULL, ptr, length));
}

static void host_kv_get(wasm_exec_env_t env, uint64_t *args) {
	pf_run *run = run_of(env);
	uint32_t ptr = IN32(0), length = IN32(1);
	OUT64(keyed_get(env, run->host ? run->host->kv_get : NULL, ptr, length));
}

static void host_kv_put(wasm_exec_env_t env, uint64_t *args) {
	pf_run *run = run_of(env);
	wasm_module_inst_t inst = wasm_runtime_get_module_inst(env);
	uint32_t key_ptr = IN32(0), key_len = IN32(1);
	uint32_t value_ptr = IN32(2), value_len = IN32(3);
	if (run->host == NULL || run->host->kv_put == NULL) {
		OUT32(STATUS_UNAVAILABLE);
		return;
	}
	if (key_len == 0 || !in_range(inst, key_ptr, key_len) ||
	    !in_range(inst, value_ptr, value_len)) {
		OUT32(STATUS_INVALID);
		return;
	}
	OUT32(run->host->kv_put(
	    wasm_runtime_addr_app_to_native(inst, key_ptr), key_len,
	    value_len == 0 ? (const uint8_t *)""
	                   : wasm_runtime_addr_app_to_native(inst, value_ptr),
	    value_len));
}

static void host_kv_delete(wasm_exec_env_t env, uint64_t *args) {
	pf_run *run = run_of(env);
	wasm_module_inst_t inst = wasm_runtime_get_module_inst(env);
	uint32_t ptr = IN32(0), length = IN32(1);
	if (run->host == NULL || run->host->kv_delete == NULL) {
		OUT32(STATUS_UNAVAILABLE);
		return;
	}
	if (length == 0 || !in_range(inst, ptr, length)) {
		OUT32(STATUS_INVALID);
		return;
	}
	OUT32(run->host->kv_delete(wasm_runtime_addr_app_to_native(inst, ptr),
	                           length));
}

static NativeSymbol plux_v1[] = {
    {"log", host_log, "(iii)", NULL},
    {"clock_now", host_clock_now, "()I", NULL},
    {"context", host_context, "()I", NULL},
    {"env_get", host_env_get, "(ii)I", NULL},
    {"kv_get", host_kv_get, "(ii)I", NULL},
    {"kv_put", host_kv_put, "(iiii)i", NULL},
    {"kv_delete", host_kv_delete, "(ii)i", NULL},
};

/* ---- wasi_snapshot_preview1: the shim of ADR-0062 ---------------------- */

static uint32_t put_u32(wasm_exec_env_t env, uint32_t ptr, uint32_t value) {
	wasm_module_inst_t inst = wasm_runtime_get_module_inst(env);
	if (!in_range(inst, ptr, 4)) {
		return ERRNO_FAULT;
	}
	memcpy(wasm_runtime_addr_app_to_native(inst, ptr), &value, 4);
	return 0;
}

static uint32_t put_u64(wasm_exec_env_t env, uint32_t ptr, uint64_t value) {
	wasm_module_inst_t inst = wasm_runtime_get_module_inst(env);
	if (!in_range(inst, ptr, 8)) {
		return ERRNO_FAULT;
	}
	memcpy(wasm_runtime_addr_app_to_native(inst, ptr), &value, 8);
	return 0;
}

/* No arguments and no environment: nothing to copy out. */
static void wasi_nothing(wasm_exec_env_t env, uint64_t *args) { OUT32(0); }

/* args_sizes_get and environ_sizes_get: a count of zero and zero bytes. */
static void wasi_zero_sizes(wasm_exec_env_t env, uint64_t *args) {
	uint32_t count = IN32(0), size = IN32(1);
	uint32_t status = put_u32(env, count, 0);
	OUT32(status != 0 ? status : put_u32(env, size, 0));
}

static void wasi_clock_res_get(wasm_exec_env_t env, uint64_t *args) {
	uint32_t id = IN32(0), out = IN32(1);
	OUT32(id > 1 ? ERRNO_INVAL : put_u64(env, out, 1000));
}

static void wasi_clock_time_get(wasm_exec_env_t env, uint64_t *args) {
	uint32_t id = IN32(0), out = IN32(2);
	if (id == 0) {
		pf_run *run = run_of(env);
		int64_t now = run->host != NULL && run->host->clock_now != NULL
		                  ? run->host->clock_now()
		                  : realtime_ns();
		OUT32(put_u64(env, out, (uint64_t)now));
	} else if (id == 1) {
		OUT32(put_u64(env, out, (uint64_t)monotonic_ns()));
	} else {
		OUT32(ERRNO_INVAL);
	}
}

/* Descriptors 1 and 2 become log lines at info and error level. */
static void wasi_fd_write(wasm_exec_env_t env, uint64_t *args) {
	uint32_t fd = IN32(0), iovs = IN32(1), iovs_len = IN32(2);
	uint32_t written_ptr = IN32(3);
	if (fd != 1 && fd != 2) {
		OUT32(ERRNO_BADF);
		return;
	}
	wasm_module_inst_t inst = wasm_runtime_get_module_inst(env);
	if (iovs_len > (UINT32_MAX / 8) || !in_range(inst, iovs, iovs_len * 8)) {
		OUT32(ERRNO_FAULT);
		return;
	}
	uint64_t total = 0;
	for (uint32_t i = 0; i < iovs_len; i++) {
		uint32_t entry[2];
		memcpy(entry, wasm_runtime_addr_app_to_native(inst, iovs + i * 8), 8);
		if (!in_range(inst, entry[0], entry[1])) {
			OUT32(ERRNO_FAULT);
			return;
		}
		total += entry[1];
		if (entry[1] > 0) {
			do_log(env, fd == 1 ? 1 : 3, entry[0], entry[1]);
		}
	}
	/* The guest is told everything was written, so it does not loop on a
	 * log budget it cannot see. */
	OUT32(put_u32(env, written_ptr,
	              total > UINT32_MAX ? UINT32_MAX : (uint32_t)total));
}

static void wasi_random_get(wasm_exec_env_t env, uint64_t *args) {
	uint32_t ptr = IN32(0), length = IN32(1);
	wasm_module_inst_t inst = wasm_runtime_get_module_inst(env);
	if (!in_range(inst, ptr, length)) {
		OUT32(ERRNO_FAULT);
		return;
	}
	uint8_t *buffer = length == 0 ? NULL : wasm_runtime_addr_app_to_native(inst, ptr);
#ifdef PF_USE_ARC4RANDOM
	arc4random_buf(buffer, length);
#else
	for (uint32_t done = 0; done < length;) {
		uint32_t chunk = length - done > 256 ? 256 : length - done;
		if (getentropy(buffer + done, chunk) != 0) {
			OUT32(ERRNO_IO);
			return;
		}
		done += chunk;
	}
#endif
	OUT32(0);
}

/* The guest asked to stop: the call ends as a trap that names the code. */
static void wasi_proc_exit(wasm_exec_env_t env, uint64_t *args) {
	pf_run *run = run_of(env);
	run->exited = 1;
	run->exit_code = (int32_t)IN32(0);
	wasm_runtime_terminate(wasm_runtime_get_module_inst(env));
}

static void wasi_sched_yield(wasm_exec_env_t env, uint64_t *args) { OUT32(0); }

/* Every other function: no file system, no sockets, no polling. Each answers
 * EBADF (a descriptor or directory that does not exist) or ENOSYS. */
#define ANSWER(name, errno_value)                                  \
	static void wasi_##name(wasm_exec_env_t env, uint64_t *args) { \
		OUT32(errno_value);                                        \
	}
ANSWER(fd_advise, ERRNO_BADF)
ANSWER(fd_allocate, ERRNO_BADF)
ANSWER(fd_close, ERRNO_BADF)
ANSWER(fd_datasync, ERRNO_BADF)
ANSWER(fd_fdstat_get, ERRNO_BADF)
ANSWER(fd_fdstat_set_flags, ERRNO_BADF)
ANSWER(fd_fdstat_set_rights, ERRNO_BADF)
ANSWER(fd_filestat_get, ERRNO_BADF)
ANSWER(fd_filestat_set_size, ERRNO_BADF)
ANSWER(fd_filestat_set_times, ERRNO_BADF)
ANSWER(fd_pread, ERRNO_BADF)
ANSWER(fd_prestat_get, ERRNO_BADF)
ANSWER(fd_prestat_dir_name, ERRNO_BADF)
ANSWER(fd_pwrite, ERRNO_BADF)
ANSWER(fd_read, ERRNO_BADF)
ANSWER(fd_readdir, ERRNO_BADF)
ANSWER(fd_renumber, ERRNO_BADF)
ANSWER(fd_seek, ERRNO_BADF)
ANSWER(fd_sync, ERRNO_BADF)
ANSWER(fd_tell, ERRNO_BADF)
ANSWER(path_create_directory, ERRNO_BADF)
ANSWER(path_filestat_get, ERRNO_BADF)
ANSWER(path_filestat_set_times, ERRNO_BADF)
ANSWER(path_link, ERRNO_BADF)
ANSWER(path_open, ERRNO_BADF)
ANSWER(path_readlink, ERRNO_BADF)
ANSWER(path_remove_directory, ERRNO_BADF)
ANSWER(path_rename, ERRNO_BADF)
ANSWER(path_symlink, ERRNO_BADF)
ANSWER(path_unlink_file, ERRNO_BADF)
ANSWER(sock_accept, ERRNO_BADF)
ANSWER(sock_recv, ERRNO_BADF)
ANSWER(sock_send, ERRNO_BADF)
ANSWER(sock_shutdown, ERRNO_BADF)
ANSWER(poll_oneoff, ERRNO_NOSYS)
ANSWER(proc_raise, ERRNO_NOSYS)

/* The full preview1 list, so linking never fails on a name the shim lacks.
 * The signatures are the ones the interpreter checks the import against. */
static NativeSymbol wasi_preview1[] = {
    {"args_get", wasi_nothing, "(ii)i", NULL},
    {"args_sizes_get", wasi_zero_sizes, "(ii)i", NULL},
    {"clock_res_get", wasi_clock_res_get, "(ii)i", NULL},
    {"clock_time_get", wasi_clock_time_get, "(iIi)i", NULL},
    {"environ_get", wasi_nothing, "(ii)i", NULL},
    {"environ_sizes_get", wasi_zero_sizes, "(ii)i", NULL},
    {"fd_advise", wasi_fd_advise, "(iIIi)i", NULL},
    {"fd_allocate", wasi_fd_allocate, "(iII)i", NULL},
    {"fd_close", wasi_fd_close, "(i)i", NULL},
    {"fd_datasync", wasi_fd_datasync, "(i)i", NULL},
    {"fd_fdstat_get", wasi_fd_fdstat_get, "(ii)i", NULL},
    {"fd_fdstat_set_flags", wasi_fd_fdstat_set_flags, "(ii)i", NULL},
    {"fd_fdstat_set_rights", wasi_fd_fdstat_set_rights, "(iII)i", NULL},
    {"fd_filestat_get", wasi_fd_filestat_get, "(ii)i", NULL},
    {"fd_filestat_set_size", wasi_fd_filestat_set_size, "(iI)i", NULL},
    {"fd_filestat_set_times", wasi_fd_filestat_set_times, "(iIIi)i", NULL},
    {"fd_pread", wasi_fd_pread, "(iiiIi)i", NULL},
    {"fd_prestat_get", wasi_fd_prestat_get, "(ii)i", NULL},
    {"fd_prestat_dir_name", wasi_fd_prestat_dir_name, "(iii)i", NULL},
    {"fd_pwrite", wasi_fd_pwrite, "(iiiIi)i", NULL},
    {"fd_read", wasi_fd_read, "(iiii)i", NULL},
    {"fd_readdir", wasi_fd_readdir, "(iiiIi)i", NULL},
    {"fd_renumber", wasi_fd_renumber, "(ii)i", NULL},
    {"fd_seek", wasi_fd_seek, "(iIii)i", NULL},
    {"fd_sync", wasi_fd_sync, "(i)i", NULL},
    {"fd_tell", wasi_fd_tell, "(ii)i", NULL},
    {"fd_write", wasi_fd_write, "(iiii)i", NULL},
    {"path_create_directory", wasi_path_create_directory, "(iii)i", NULL},
    {"path_filestat_get", wasi_path_filestat_get, "(iiiii)i", NULL},
    {"path_filestat_set_times", wasi_path_filestat_set_times, "(iiiiIIi)i", NULL},
    {"path_link", wasi_path_link, "(iiiiiii)i", NULL},
    {"path_open", wasi_path_open, "(iiiiiIIii)i", NULL},
    {"path_readlink", wasi_path_readlink, "(iiiiii)i", NULL},
    {"path_remove_directory", wasi_path_remove_directory, "(iii)i", NULL},
    {"path_rename", wasi_path_rename, "(iiiiii)i", NULL},
    {"path_symlink", wasi_path_symlink, "(iiiii)i", NULL},
    {"path_unlink_file", wasi_path_unlink_file, "(iii)i", NULL},
    {"poll_oneoff", wasi_poll_oneoff, "(iiii)i", NULL},
    {"proc_exit", wasi_proc_exit, "(i)", NULL},
    {"proc_raise", wasi_proc_raise, "(i)i", NULL},
    {"random_get", wasi_random_get, "(ii)i", NULL},
    {"sched_yield", wasi_sched_yield, "()i", NULL},
    {"sock_accept", wasi_sock_accept, "(iii)i", NULL},
    {"sock_recv", wasi_sock_recv, "(iiiiii)i", NULL},
    {"sock_send", wasi_sock_send, "(iiiii)i", NULL},
    {"sock_shutdown", wasi_sock_shutdown, "(ii)i", NULL},
};

/* ---- runtime ----------------------------------------------------------- */

static pthread_once_t init_once = PTHREAD_ONCE_INIT;
static int init_status = -1;

static void init_runtime(void) {
	RuntimeInitArgs args;
	memset(&args, 0, sizeof(args));
	args.mem_alloc_type = Alloc_With_Allocator;
	args.mem_alloc_option.allocator.malloc_func = pf_malloc;
	args.mem_alloc_option.allocator.realloc_func = pf_realloc;
	args.mem_alloc_option.allocator.free_func = pf_release;
	args.max_thread_num = 1;
	if (!wasm_runtime_full_init(&args)) {
		return;
	}
	if (!wasm_runtime_register_natives_raw(
	        "plux_v1", plux_v1, sizeof(plux_v1) / sizeof(plux_v1[0])) ||
	    !wasm_runtime_register_natives_raw(
	        "wasi_snapshot_preview1", wasi_preview1,
	        sizeof(wasi_preview1) / sizeof(wasi_preview1[0]))) {
		wasm_runtime_destroy();
		return;
	}
	init_status = 0;
}

PF_API int32_t pf_init(void) {
	pthread_once(&init_once, init_runtime);
	return init_status;
}

PF_API const char *pf_runtime_version(void) { return "2.4.3"; }

/* ---- invocation handles and the watchdog ------------------------------- */

PF_API pf_invocation *pf_invocation_new(void) {
	pf_invocation *invocation = calloc(1, sizeof(*invocation));
	if (invocation != NULL) {
		pthread_mutex_init(&invocation->mutex, NULL);
	}
	return invocation;
}

PF_API void pf_invocation_free(pf_invocation *invocation) {
	if (invocation != NULL) {
		pthread_mutex_destroy(&invocation->mutex);
		free(invocation);
	}
}

/* Ends the running call with `reason` unless one already ended it. */
static void stop(pf_invocation *invocation, int reason) {
	pthread_mutex_lock(&invocation->mutex);
	if (invocation->reason == 0) {
		invocation->reason = reason;
		if (invocation->instance != NULL) {
			wasm_runtime_terminate(invocation->instance);
		}
	}
	pthread_mutex_unlock(&invocation->mutex);
}

PF_API void pf_cancel(pf_invocation *invocation) {
	if (invocation == NULL) {
		return;
	}
	pthread_mutex_lock(&invocation->mutex);
	invocation->cancelled = 1;
	pthread_mutex_unlock(&invocation->mutex);
	stop(invocation, PF_CANCELLED);
}

typedef struct watchdog {
	pthread_t thread;
	pthread_mutex_t mutex;
	pthread_cond_t cond;
	int done;
	uint32_t timeout_ms;
	pf_invocation *invocation;
} watchdog;

static void *watchdog_main(void *argument) {
	watchdog *dog = argument;
	pthread_mutex_lock(&dog->mutex);
#ifdef __APPLE__
	struct timespec relative = {dog->timeout_ms / 1000,
	                            (long)(dog->timeout_ms % 1000) * 1000000L};
	while (!dog->done) {
		if (pthread_cond_timedwait_relative_np(&dog->cond, &dog->mutex,
		                                       &relative) == ETIMEDOUT) {
			break;
		}
	}
#else
	struct timespec deadline;
	clock_gettime(CLOCK_MONOTONIC, &deadline);
	deadline.tv_sec += dog->timeout_ms / 1000;
	deadline.tv_nsec += (long)(dog->timeout_ms % 1000) * 1000000L;
	if (deadline.tv_nsec >= 1000000000L) {
		deadline.tv_sec += 1;
		deadline.tv_nsec -= 1000000000L;
	}
	while (!dog->done) {
		if (pthread_cond_timedwait(&dog->cond, &dog->mutex, &deadline) ==
		    ETIMEDOUT) {
			break;
		}
	}
#endif
	int expired = !dog->done;
	pthread_mutex_unlock(&dog->mutex);
	if (expired) {
		stop(dog->invocation, PF_TIMEOUT);
	}
	return NULL;
}

static int watchdog_start(watchdog *dog, pf_invocation *invocation,
                          uint32_t timeout_ms) {
	dog->done = 0;
	dog->timeout_ms = timeout_ms;
	dog->invocation = invocation;
	pthread_mutex_init(&dog->mutex, NULL);
	pthread_condattr_t attr;
	pthread_condattr_init(&attr);
#ifndef __APPLE__
	pthread_condattr_setclock(&attr, CLOCK_MONOTONIC);
#endif
	pthread_cond_init(&dog->cond, &attr);
	pthread_condattr_destroy(&attr);
	pthread_attr_t thread_attr;
	pthread_attr_init(&thread_attr);
	pthread_attr_setstacksize(&thread_attr, PF_WATCHDOG_STACK);
	int rc = pthread_create(&dog->thread, &thread_attr, watchdog_main, dog);
	pthread_attr_destroy(&thread_attr);
	if (rc != 0) {
		pthread_cond_destroy(&dog->cond);
		pthread_mutex_destroy(&dog->mutex);
	}
	return rc;
}

static void watchdog_stop(watchdog *dog) {
	pthread_mutex_lock(&dog->mutex);
	dog->done = 1;
	pthread_cond_signal(&dog->cond);
	pthread_mutex_unlock(&dog->mutex);
	pthread_join(dog->thread, NULL);
	pthread_cond_destroy(&dog->cond);
	pthread_mutex_destroy(&dog->mutex);
}

/* ---- loading and checking a module ------------------------------------- */

static int in_list(const char *name, const char *const *list, size_t count) {
	for (size_t i = 0; i < count; i++) {
		if (strcmp(name, list[i]) == 0) {
			return 1;
		}
	}
	return 0;
}

#define COUNT(list) (sizeof(list) / sizeof((list)[0]))

static const char *const device_imports[] = {
    "log", "clock_now", "context", "env_get", "kv_get", "kv_put", "kv_delete",
};
/* FN-052: the server's host functions; a device host refuses them. */
static const char *const server_imports[] = {
    "http_request", "secret_get", "claims", "invoke_function",
};

/* Whether [type] has the signature written in WAMR's native notation, such
 * as "(ii)I": i is i32, I is i64, and what follows ")" is the result. */
static int signature_matches(wasm_func_type_t type, const char *signature) {
	const char *cursor = signature;
	if (*cursor++ != '(') {
		return 0;
	}
	uint32_t index = 0;
	for (; *cursor != ')' && *cursor != '\0'; cursor++, index++) {
		wasm_valkind_t kind = *cursor == 'i' ? WASM_I32 : WASM_I64;
		if ((*cursor != 'i' && *cursor != 'I') ||
		    index >= wasm_func_type_get_param_count(type) ||
		    wasm_func_type_get_param_valkind(type, index) != kind) {
			return 0;
		}
	}
	if (*cursor++ != ')' || index != wasm_func_type_get_param_count(type)) {
		return 0;
	}
	if (*cursor == '\0') {
		return wasm_func_type_get_result_count(type) == 0;
	}
	wasm_valkind_t result = *cursor == 'i' ? WASM_I32 : WASM_I64;
	return wasm_func_type_get_result_count(type) == 1 &&
	       wasm_func_type_get_result_valkind(type, 0) == result;
}

static const char *symbol_signature(const NativeSymbol *table, size_t count,
                                    const char *name) {
	for (size_t i = 0; i < count; i++) {
		if (strcmp(name, table[i].symbol) == 0) {
			return table[i].signature;
		}
	}
	return NULL;
}

/* Returns 0 or the PLX code that refuses the module. */
static int32_t check_module(wasm_module_t module, char *error,
                            uint32_t capacity) {
	int32_t imports = wasm_runtime_get_import_count(module);
	for (int32_t i = 0; i < imports; i++) {
		wasm_import_t import;
		wasm_runtime_get_import_type(module, i, &import);
		char message[160];
		if (import.kind != WASM_IMPORT_EXPORT_KIND_FUNC) {
			snprintf(message, sizeof(message),
			         "import %s.%s is not a function", import.module_name,
			         import.name);
			set_error(error, capacity, message);
			return PF_MODULE_INVALID;
		}
		const char *signature = NULL;
		if (strcmp(import.module_name, "plux_v1") == 0) {
			if (in_list(import.name, server_imports, COUNT(server_imports))) {
				snprintf(message, sizeof(message),
				         "a device module imports the server-only function %s",
				         import.name);
				set_error(error, capacity, message);
				return PF_MODULE_INVALID;
			}
			if (in_list(import.name, device_imports, COUNT(device_imports))) {
				signature = symbol_signature(plux_v1, COUNT(plux_v1), import.name);
			}
		} else if (strcmp(import.module_name, "wasi_snapshot_preview1") == 0) {
			signature = symbol_signature(wasi_preview1, COUNT(wasi_preview1),
			                             import.name);
		}
		if (signature == NULL) {
			snprintf(message, sizeof(message), "unknown import %s.%s",
			         import.module_name, import.name);
		} else if (!signature_matches(import.u.func_type, signature)) {
			snprintf(message, sizeof(message),
			         "import %s.%s has the wrong signature", import.module_name,
			         import.name);
		} else {
			continue;
		}
		set_error(error, capacity, message);
		return PF_MODULE_INVALID;
	}
	int has_memory = 0, has_version = 0, has_alloc = 0, has_invoke = 0;
	int32_t exports = wasm_runtime_get_export_count(module);
	for (int32_t i = 0; i < exports; i++) {
		wasm_export_t export;
		wasm_runtime_get_export_type(module, i, &export);
		if (export.kind == WASM_IMPORT_EXPORT_KIND_MEMORY &&
		    strcmp(export.name, "memory") == 0) {
			has_memory = 1;
		} else if (export.kind == WASM_IMPORT_EXPORT_KIND_FUNC) {
			if (strcmp(export.name, "plux_abi_version") == 0) {
				has_version = signature_matches(export.u.func_type, "()i");
			} else if (strcmp(export.name, "plux_alloc") == 0) {
				has_alloc = signature_matches(export.u.func_type, "(i)i");
			} else if (strcmp(export.name, "plux_invoke") == 0) {
				has_invoke = signature_matches(export.u.func_type, "(iii)I");
			}
		}
	}
	if (!(has_memory && has_version && has_alloc && has_invoke)) {
		set_error(error, capacity,
		          "the module must export memory, plux_abi_version, "
		          "plux_alloc and plux_invoke");
		return PF_MODULE_INVALID;
	}
	return 0;
}

/* ---- running a function ------------------------------------------------ */

static int32_t classify_failure(pf_run *run, pf_invocation *invocation,
                                wasm_module_inst_t inst, uint8_t **out,
                                uint32_t *out_length) {
	int reason;
	pthread_mutex_lock(&invocation->mutex);
	reason = invocation->reason;
	pthread_mutex_unlock(&invocation->mutex);
	if (reason == PF_TIMEOUT) {
		*out = duplicate("the invocation exceeded its time limit", out_length);
		return PF_TIMEOUT;
	}
	if (reason == PF_CANCELLED) {
		*out = duplicate("the invocation was cancelled", out_length);
		return PF_CANCELLED;
	}
	if (run->memory_denied) {
		*out = duplicate("the invocation exceeded its memory limit", out_length);
		return PF_MEMORY_EXCEEDED;
	}
	if (run->exited) {
		char message[64];
		snprintf(message, sizeof(message), "the module called proc_exit(%d)",
		         (int)run->exit_code);
		*out = duplicate(message, out_length);
		return PF_TRAPPED;
	}
	const char *exception = inst != NULL ? wasm_runtime_get_exception(inst) : NULL;
	*out = duplicate(exception != NULL && exception[0] != '\0'
	                     ? exception
	                     : "the module trapped",
	                 out_length);
	return PF_TRAPPED;
}

/* Runs the exported function on a fresh instance. When `probe_version` is
 * set it calls plux_abi_version instead and returns its value in *version. */
static int32_t run_guest(pf_module *module, uint32_t index,
                         const uint8_t *input, uint32_t input_length,
                         const pf_limits *limits, const pf_host *host,
                         pf_invocation *invocation, int probe_version,
                         int32_t *version, uint8_t **out,
                         uint32_t *out_length) {
	*out = NULL;
	*out_length = 0;
	pf_run run;
	memset(&run, 0, sizeof(run));
	run.host = host;
	run.limits = *limits;

	pthread_mutex_lock(&invocation->mutex);
	int already = invocation->cancelled;
	pthread_mutex_unlock(&invocation->mutex);
	if (already) {
		*out = duplicate("the invocation was cancelled", out_length);
		return PF_CANCELLED;
	}

	wasm_runtime_init_thread_env();
	current_run = &run;

	watchdog dog;
	int32_t status;
	if (watchdog_start(&dog, invocation, limits->timeout_ms) != 0) {
		current_run = NULL;
		wasm_runtime_destroy_thread_env();
		*out = duplicate("the watchdog thread could not start", out_length);
		return PF_TRAPPED;
	}

	char error[128] = {0};
	InstantiationArgs args;
	memset(&args, 0, sizeof(args));
	args.default_stack_size = PF_OPERAND_STACK;
	wasm_module_inst_t inst =
	    wasm_runtime_instantiate_ex(module->module, &args, error, sizeof(error));
	wasm_exec_env_t env = NULL;
	if (inst == NULL) {
		if (run.memory_denied) {
			*out = duplicate("the invocation exceeded its memory limit", out_length);
			status = PF_MEMORY_EXCEEDED;
		} else {
			*out = duplicate(error[0] != '\0' ? error : "instantiation failed",
			                 out_length);
			status = PF_MODULE_INVALID;
		}
		goto done;
	}

	pthread_mutex_lock(&invocation->mutex);
	invocation->instance = inst;
	if (invocation->cancelled && invocation->reason == 0) {
		invocation->reason = PF_CANCELLED;
		wasm_runtime_terminate(inst);
	}
	pthread_mutex_unlock(&invocation->mutex);

	env = wasm_runtime_create_exec_env(inst, PF_OPERAND_STACK);
	if (env == NULL) {
		*out = duplicate(run.memory_denied ? "the invocation exceeded its memory limit"
		                                   : "no execution environment",
		                 out_length);
		status = run.memory_denied ? PF_MEMORY_EXCEEDED : PF_TRAPPED;
		goto done;
	}
	wasm_runtime_set_user_data(env, &run);

	run.alloc = wasm_runtime_lookup_function(inst, "plux_alloc");
	wasm_function_inst_t entry = wasm_runtime_lookup_function(
	    inst, probe_version ? "plux_abi_version" : "plux_invoke");
	if (run.alloc == NULL || entry == NULL) {
		*out = duplicate("the module lacks a required export", out_length);
		status = PF_MODULE_INVALID;
		goto done;
	}

	uint32_t argv[4] = {0, 0, 0, 0};
	if (probe_version) {
		if (!wasm_runtime_call_wasm(env, entry, 0, argv)) {
			status = classify_failure(&run, invocation, inst, out, out_length);
			goto done;
		}
		*version = (int32_t)argv[0];
		status = 0;
		goto done;
	}

	argv[0] = input_length;
	if (!wasm_runtime_call_wasm(env, run.alloc, 1, argv)) {
		status = classify_failure(&run, invocation, inst, out, out_length);
		goto done;
	}
	uint32_t input_ptr = argv[0];
	if ((input_ptr == 0 && input_length > 0) ||
	    !in_range(inst, input_ptr, input_length)) {
		*out = duplicate(run.memory_denied ? "the invocation exceeded its memory limit"
		                                   : "the module could not take the input",
		                 out_length);
		status = run.memory_denied ? PF_MEMORY_EXCEEDED : PF_TRAPPED;
		goto done;
	}
	if (input_length > 0) {
		memcpy(wasm_runtime_addr_app_to_native(inst, input_ptr), input,
		       input_length);
	}

	argv[0] = index;
	argv[1] = input_ptr;
	argv[2] = input_length;
	argv[3] = 0;
	if (!wasm_runtime_call_wasm(env, entry, 3, argv)) {
		status = classify_failure(&run, invocation, inst, out, out_length);
		goto done;
	}
	{
		/* A termination that left the interpreter without an exception still
		 * ends the call; the stop reason wins over any result. */
		int reason;
		pthread_mutex_lock(&invocation->mutex);
		reason = invocation->reason;
		pthread_mutex_unlock(&invocation->mutex);
		if (reason != 0) {
			status = classify_failure(&run, invocation, inst, out, out_length);
			goto done;
		}
	}

	uint64_t packed = (uint64_t)argv[0] | ((uint64_t)argv[1] << 32);
	uint32_t result_ptr = (uint32_t)(packed >> 32);
	uint32_t result_len = (uint32_t)(packed & 0xffffffffu);
	if (result_len < 1 || !in_range(inst, result_ptr, result_len)) {
		/* FN-015: a result that cannot be read is a trap, as on every host. */
		*out = duplicate("the module returned a malformed result", out_length);
		status = PF_TRAPPED;
		goto done;
	}
	if (result_len - 1 > limits->output_bytes) {
		*out = duplicate("the output exceeds its size limit", out_length);
		status = PF_OUTPUT_TOO_LARGE;
		goto done;
	}
	{
		const uint8_t *result = wasm_runtime_addr_app_to_native(inst, result_ptr);
		uint8_t tag = result[0];
		uint32_t body_len = result_len - 1;
		if (tag > 3) {
			*out = duplicate("the module returned an unknown result tag", out_length);
			status = PF_TRAPPED;
			goto done;
		}
		if (tag >= 2 && body_len > PF_MESSAGE_MAX) {
			body_len = PF_MESSAGE_MAX;
		}
		uint8_t *copy = pf_alloc(body_len);
		if (copy == NULL) {
			*out = duplicate("no memory for the result", out_length);
			status = PF_MEMORY_EXCEEDED;
			goto done;
		}
		memcpy(copy, result + 1, body_len);
		*out = copy;
		*out_length = body_len;
		status = tag == 0 ? PF_OK
		         : tag == 1 ? PF_BUSINESS_ERROR
		         : tag == 2 ? PF_INPUT_INVALID
		                    : PF_OUTPUT_INVALID;
	}

done:
	pthread_mutex_lock(&invocation->mutex);
	invocation->instance = NULL;
	pthread_mutex_unlock(&invocation->mutex);
	watchdog_stop(&dog);
	if (env != NULL) {
		wasm_runtime_destroy_exec_env(env);
	}
	if (inst != NULL) {
		wasm_runtime_deinstantiate(inst);
	}
	current_run = NULL;
	wasm_runtime_destroy_thread_env();
	return status;
}

PF_API pf_module *pf_module_load(const uint8_t *bytes, uint32_t length,
                                 int32_t *status, char *error,
                                 uint32_t error_capacity) {
	*status = PF_MODULE_INVALID;
	if (pf_init() != 0) {
		set_error(error, error_capacity, "the runtime did not start");
		return NULL;
	}
	if (bytes == NULL || length == 0) {
		set_error(error, error_capacity, "the module is empty");
		return NULL;
	}
	pf_module *module = calloc(1, sizeof(*module));
	if (module == NULL) {
		set_error(error, error_capacity, "out of memory");
		return NULL;
	}
	/* The interpreter rewrites code in place, and the module must stay
	 * readable until unload, so it keeps its own copy. */
	module->code = malloc(length);
	if (module->code == NULL) {
		free(module);
		set_error(error, error_capacity, "out of memory");
		return NULL;
	}
	memcpy(module->code, bytes, length);

	char wamr_error[128] = {0};
	module->module = wasm_runtime_load(module->code, length, wamr_error,
	                                    sizeof(wamr_error));
	if (module->module == NULL) {
		char message[192];
		snprintf(message, sizeof(message), "the module does not load: %s",
		         wamr_error);
		set_error(error, error_capacity, message);
		goto refuse;
	}
	int32_t refused = check_module(module->module, error, error_capacity);
	if (refused != 0) {
		*status = refused;
		goto refuse;
	}

	pf_invocation *probe = pf_invocation_new();
	if (probe == NULL) {
		set_error(error, error_capacity, "out of memory");
		goto refuse;
	}
	pf_limits limits = {PF_PROBE_TIMEOUT_MS, PF_PROBE_MEMORY_BYTES, 0, 0};
	int32_t version = 0;
	uint8_t *detail = NULL;
	uint32_t detail_length = 0;
	int32_t result = run_guest(module, 0, NULL, 0, &limits, NULL, probe, 1,
	                           &version, &detail, &detail_length);
	pf_invocation_free(probe);
	if (result != 0) {
		char message[192];
		snprintf(message, sizeof(message), "plux_abi_version failed: %.*s",
		         (int)(detail_length > 120 ? 120 : detail_length),
		         detail != NULL ? (const char *)detail : "");
		set_error(error, error_capacity, message);
		pf_free(detail);
		goto refuse;
	}
	if (version != 1) {
		char message[64];
		snprintf(message, sizeof(message), "unsupported plux_abi_version %d",
		         (int)version);
		set_error(error, error_capacity, message);
		*status = PF_ABI_UNSUPPORTED;
		goto refuse;
	}
	*status = 0;
	return module;

refuse:
	pf_module_free(module);
	return NULL;
}

PF_API void pf_module_free(pf_module *module) {
	if (module == NULL) {
		return;
	}
	if (module->module != NULL) {
		wasm_runtime_unload(module->module);
	}
	free(module->code);
	free(module);
}

PF_API int32_t pf_invoke(pf_module *module, uint32_t index,
                         const uint8_t *input, uint32_t input_length,
                         const pf_limits *limits, const pf_host *host,
                         pf_invocation *invocation, uint8_t **out,
                         uint32_t *out_length) {
	if (module == NULL || limits == NULL || invocation == NULL || out == NULL ||
	    out_length == NULL || limits->timeout_ms == 0) {
		return PF_TRAPPED;
	}
	return run_guest(module, index, input, input_length, limits, host,
	                 invocation, 0, NULL, out, out_length);
}
