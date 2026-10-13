// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

/*
 * The device function engine's C interface (FN-050, ADR-0062): a thin layer
 * over the WebAssembly Micro Runtime's fast interpreter. Dart binds only the
 * pf_* symbols; every other symbol of the library is hidden.
 *
 * Threading: pf_init, pf_module_load, pf_module_free and pf_invoke run on
 * the worker isolate's thread. pf_cancel may be called from any thread while
 * the invocation it names has not been freed.
 */
#ifndef PLUX_FUNCTIONS_H
#define PLUX_FUNCTIONS_H

#include <stddef.h>
#include <stdint.h>

#if defined(__GNUC__) || defined(__clang__)
#define PF_API __attribute__((visibility("default")))
#else
#define PF_API
#endif

/* PLX codes the engine reports (Appendix F). */
#define PF_OK 0
#define PF_BUSINESS_ERROR 1
#define PF_TIMEOUT 7001
#define PF_MEMORY_EXCEEDED 7002
#define PF_OUTPUT_TOO_LARGE 7004
#define PF_TRAPPED 7007
#define PF_INPUT_INVALID 7008
#define PF_OUTPUT_INVALID 7009
#define PF_ABI_UNSUPPORTED 7017
#define PF_MODULE_INVALID 7018
#define PF_CANCELLED 7021

typedef struct pf_module pf_module;
typedef struct pf_invocation pf_invocation;

/* Limits of one invocation; the caller takes every value from the limits
 * registry (LIM-001), so none has a default here. */
typedef struct pf_limits {
	uint32_t timeout_ms;   /* wall clock, instantiation included */
	uint64_t memory_bytes; /* everything the engine allocates for the call */
	uint32_t output_bytes; /* the tagged result, tag byte excluded */
	uint32_t log_bytes;    /* log and stdio bytes forwarded to the host */
} pf_limits;

/*
 * The host's side of the plux_v1 imports. Every callback runs on the thread
 * that called pf_invoke. A callback returns 0 or a negative ABI status
 * (-1 not found, -2 capability denied, -3 limit exceeded, -6 invalid
 * argument, -7 unavailable). A buffer a callback returns is allocated with
 * pf_alloc; the engine copies it into the guest and frees it. A null
 * callback answers -7 (clock_now: the system clock; log: dropped).
 */
typedef struct pf_host {
	void (*log)(int32_t level, const uint8_t *message, uint32_t length);
	int64_t (*clock_now)(void);
	int32_t (*context)(uint8_t **out, uint32_t *out_length);
	int32_t (*env_get)(const uint8_t *key, uint32_t key_length, uint8_t **out,
	                   uint32_t *out_length);
	int32_t (*kv_get)(const uint8_t *key, uint32_t key_length, uint8_t **out,
	                  uint32_t *out_length);
	int32_t (*kv_put)(const uint8_t *key, uint32_t key_length,
	                  const uint8_t *value, uint32_t value_length);
	int32_t (*kv_delete)(const uint8_t *key, uint32_t key_length);
} pf_host;

/* Starts the runtime; idempotent. Returns 0 or -1. */
PF_API int32_t pf_init(void);

/* The WAMR version the library was built with, for example "2.4.3". */
PF_API const char *pf_runtime_version(void);

PF_API void *pf_alloc(size_t size);
PF_API void pf_free(void *ptr);

/*
 * Loads and checks a module: imports and exports (PLX-7017, PLX-7018) and
 * plux_abi_version() == 1 (PLX-7017). Returns null and sets *status to a PLX
 * code with a short diagnostic in error on refusal. The bytes are copied.
 */
PF_API pf_module *pf_module_load(const uint8_t *bytes, uint32_t length,
                                 int32_t *status, char *error,
                                 uint32_t error_capacity);
PF_API void pf_module_free(pf_module *module);

/* A handle to cancel one invocation; freed by the caller after pf_invoke. */
PF_API pf_invocation *pf_invocation_new(void);
PF_API void pf_invocation_free(pf_invocation *invocation);
PF_API void pf_cancel(pf_invocation *invocation);

/*
 * Runs function `index` of the module on a fresh instance. Returns
 *   0     the output; *out is the JSON (no tag byte);
 *   1     a business error; *out is its JSON;
 *   other a PLX code; *out is a short diagnostic (UTF-8, never guest data
 *         other than the guest's own refusal message) or null.
 * *out is allocated with pf_alloc; the caller releases it with pf_free.
 */
PF_API int32_t pf_invoke(pf_module *module, uint32_t index,
                         const uint8_t *input, uint32_t input_length,
                         const pf_limits *limits, const pf_host *host,
                         pf_invocation *invocation, uint8_t **out,
                         uint32_t *out_length);

#endif
