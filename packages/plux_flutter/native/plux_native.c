// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

// The native part of the Plux runtime (ADR-0030): read-only memory maps of
// verified bundle files, one-shot zstd decompression with an optional
// raw-content dictionary for section deltas (ADR-0003), and the entry point
// of the platform's AES-256-GCM (ADR-0058). The functions keep no state,
// take bytes and return bytes; every bound is checked in Dart before a
// call, and again here.

#include <errno.h>
#include <fcntl.h>
#include <stdint.h>
#include <stdlib.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <unistd.h>

#include "zstd/zstd.h"

#if defined(__ANDROID__) && !defined(PLUX_AEAD_JNI)
#define PLUX_AEAD_JNI 1
#endif

#if defined(PLUX_AEAD_JNI)
#include <jni.h>
#elif defined(__APPLE__)
#include <dlfcn.h>
#endif

#define PLUX_EXPORT __attribute__((visibility("default"))) __attribute__((used))

// A mapped file; Dart reads the fields and passes the handle back to
// plux_release, directly or through a NativeFinalizer.
typedef struct {
  void* address;
  uint64_t length;
  int32_t error;
} plux_mapping;

// Maps the file at the NUL-terminated path read-only. Returns a handle,
// or NULL when out of memory. On failure the handle's error is the errno
// value and its address is NULL; an empty file maps to a NULL address with
// length zero and no error.
PLUX_EXPORT plux_mapping* plux_map(const char* path) {
  plux_mapping* m = (plux_mapping*)calloc(1, sizeof(plux_mapping));
  if (m == NULL) return NULL;
  int fd = open(path, O_RDONLY | O_CLOEXEC);
  if (fd < 0) {
    m->error = errno;
    return m;
  }
  struct stat st;
  if (fstat(fd, &st) != 0) {
    m->error = errno;
    close(fd);
    return m;
  }
  if (!S_ISREG(st.st_mode)) {
    m->error = EINVAL;
    close(fd);
    return m;
  }
  if (st.st_size > 0) {
    void* a = mmap(NULL, (size_t)st.st_size, PROT_READ, MAP_PRIVATE, fd, 0);
    if (a == MAP_FAILED) {
      m->error = errno;
    } else {
      m->address = a;
      m->length = (uint64_t)st.st_size;
    }
  }
  close(fd);
  return m;
}

// Unmaps a mapping and frees its handle. Safe to call with NULL.
PLUX_EXPORT void plux_release(plux_mapping* m) {
  if (m == NULL) return;
  if (m->address != NULL) munmap(m->address, (size_t)m->length);
  free(m);
}

// Makes the entries of the directory at the NUL-terminated path durable
// (fsync of the directory), which the release store needs after a rename
// (ADR-0021). Returns 0, or the errno value.
PLUX_EXPORT int32_t plux_fsync_dir(const char* path) {
  int fd = open(path, O_RDONLY | O_CLOEXEC | O_DIRECTORY);
  if (fd < 0) return errno;
  int r = fsync(fd) == 0 ? 0 : errno;
  close(fd);
  return r;
}

// Error results of the zstd functions; zstd's own error codes are
// reported negated below these.
enum {
  PLUX_ZSTD_UNKNOWN_SIZE = -1,
  PLUX_ZSTD_BAD_FRAME = -2,
  PLUX_ZSTD_NO_MEMORY = -3,
};

// Returns the content size a zstd frame declares, PLUX_ZSTD_UNKNOWN_SIZE
// when it declares none, or PLUX_ZSTD_BAD_FRAME when the header is not a
// zstd frame header.
PLUX_EXPORT int64_t plux_zstd_content_size(const uint8_t* src, uint64_t length) {
  unsigned long long n = ZSTD_getFrameContentSize(src, (size_t)length);
  if (n == ZSTD_CONTENTSIZE_UNKNOWN) return PLUX_ZSTD_UNKNOWN_SIZE;
  if (n == ZSTD_CONTENTSIZE_ERROR || n > (unsigned long long)INT64_MAX) return PLUX_ZSTD_BAD_FRAME;
  return (int64_t)n;
}

// Decodes exactly one zstd frame from src into dst, which holds capacity
// bytes, with dict as a raw-content dictionary when dict_length is not
// zero (zstd's --patch-from). Returns the number of bytes written, or a
// negative error: the frame is invalid, its checksum fails, it has
// trailing data, or it decodes to more than capacity.
PLUX_EXPORT int64_t plux_zstd_decompress(uint8_t* dst, uint64_t capacity, const uint8_t* src,
                                         uint64_t length, const uint8_t* dict,
                                         uint64_t dict_length) {
  if (length == 0) return PLUX_ZSTD_BAD_FRAME;
  unsigned long long frame = ZSTD_findFrameCompressedSize(src, (size_t)length);
  if (ZSTD_isError(frame) || frame != length) return PLUX_ZSTD_BAD_FRAME;
  ZSTD_DCtx* ctx = ZSTD_createDCtx();
  if (ctx == NULL) return PLUX_ZSTD_NO_MEMORY;
  size_t r = 0;
  if (dict_length > 0) r = ZSTD_DCtx_refPrefix(ctx, dict, (size_t)dict_length);
  if (!ZSTD_isError(r)) r = ZSTD_decompressDCtx(ctx, dst, (size_t)capacity, src, (size_t)length);
  ZSTD_freeDCtx(ctx);
  if (ZSTD_isError(r)) return PLUX_ZSTD_NO_MEMORY - (int64_t)ZSTD_getErrorCode(r);
  return (int64_t)r;
}

// AES-256-GCM by the platform (ADR-0058): javax.crypto on Android, reached
// through JNI, and CryptoKit on iOS, reached through the symbol the plugin
// exports. Other platforms have no native implementation, and Dart uses
// its own.
enum {
  PLUX_AEAD_SEAL = 0,
  PLUX_AEAD_OPEN = 1,
  PLUX_AEAD_KEY_LENGTH = 32,
  PLUX_AEAD_NONCE_LENGTH = 12,
  PLUX_AEAD_TAG_LENGTH = 16,
};

// Results other than a length.
enum {
  PLUX_AEAD_UNSUPPORTED = -1,  // no platform implementation here
  PLUX_AEAD_FAILED = -2,       // the platform failed, or a length is wrong
  PLUX_AEAD_AUTH = -3,         // the tag does not verify
  PLUX_AEAD_CAPACITY = -4,     // the output buffer is too small
};

#if defined(PLUX_AEAD_JNI)

// The Java VM, kept when the plugin loads the library with
// System.loadLibrary; written once, before any call.
static JavaVM* plux_vm;

PLUX_EXPORT jint JNI_OnLoad(JavaVM* vm, void* reserved) {
  (void)reserved;
  plux_vm = vm;
  return JNI_VERSION_1_6;
}

#define PLUX_JNI_OK(env) (!(*(env))->ExceptionCheck(env))

// The NDK's AttachCurrentThread takes a JNIEnv**; other JNI headers a void**.
#if defined(__ANDROID__)
typedef JNIEnv** plux_attach_arg;
#else
typedef void** plux_attach_arg;
#endif

// javax.crypto.Cipher "AES/GCM/NoPadding" with a 128-bit tag. Every local
// reference lives in one frame that is popped on the way out.
static int64_t plux_aead_jni(JNIEnv* env, int32_t mode, const uint8_t* key, const uint8_t* nonce,
                             const uint8_t* aad, uint64_t aad_length, const uint8_t* in,
                             uint64_t in_length, uint8_t* out, uint64_t out_capacity) {
  int64_t result = PLUX_AEAD_FAILED;
  if ((*env)->PushLocalFrame(env, 32) != JNI_OK) {
    (*env)->ExceptionClear(env);
    return result;
  }
  jclass cipher_class = (*env)->FindClass(env, "javax/crypto/Cipher");
  jclass key_class = (*env)->FindClass(env, "javax/crypto/spec/SecretKeySpec");
  jclass spec_class = (*env)->FindClass(env, "javax/crypto/spec/GCMParameterSpec");
  if (!PLUX_JNI_OK(env) || cipher_class == NULL || key_class == NULL || spec_class == NULL) goto done;

  jmethodID get_instance = (*env)->GetStaticMethodID(env, cipher_class, "getInstance",
                                                     "(Ljava/lang/String;)Ljavax/crypto/Cipher;");
  jmethodID init = (*env)->GetMethodID(env, cipher_class, "init",
                                       "(ILjava/security/Key;Ljava/security/spec/AlgorithmParameterSpec;)V");
  jmethodID update_aad = (*env)->GetMethodID(env, cipher_class, "updateAAD", "([B)V");
  jmethodID do_final = (*env)->GetMethodID(env, cipher_class, "doFinal", "([B)[B");
  jmethodID key_new = (*env)->GetMethodID(env, key_class, "<init>", "([BLjava/lang/String;)V");
  jmethodID spec_new = (*env)->GetMethodID(env, spec_class, "<init>", "(I[B)V");
  if (!PLUX_JNI_OK(env) || get_instance == NULL || init == NULL || update_aad == NULL ||
      do_final == NULL || key_new == NULL || spec_new == NULL)
    goto done;

  jbyteArray jkey = (*env)->NewByteArray(env, PLUX_AEAD_KEY_LENGTH);
  jbyteArray jnonce = (*env)->NewByteArray(env, PLUX_AEAD_NONCE_LENGTH);
  jbyteArray jin = (*env)->NewByteArray(env, (jsize)in_length);
  if (!PLUX_JNI_OK(env) || jkey == NULL || jnonce == NULL || jin == NULL) goto done;
  (*env)->SetByteArrayRegion(env, jkey, 0, PLUX_AEAD_KEY_LENGTH, (const jbyte*)key);
  (*env)->SetByteArrayRegion(env, jnonce, 0, PLUX_AEAD_NONCE_LENGTH, (const jbyte*)nonce);
  if (in_length > 0) (*env)->SetByteArrayRegion(env, jin, 0, (jsize)in_length, (const jbyte*)in);

  jstring transformation = (*env)->NewStringUTF(env, "AES/GCM/NoPadding");
  jstring algorithm = (*env)->NewStringUTF(env, "AES");
  if (!PLUX_JNI_OK(env) || transformation == NULL || algorithm == NULL) goto done;
  jobject cipher = (*env)->CallStaticObjectMethod(env, cipher_class, get_instance, transformation);
  if (!PLUX_JNI_OK(env) || cipher == NULL) goto done;
  jobject secret = (*env)->NewObject(env, key_class, key_new, jkey, algorithm);
  if (!PLUX_JNI_OK(env) || secret == NULL) goto done;
  jobject spec = (*env)->NewObject(env, spec_class, spec_new, (jint)(PLUX_AEAD_TAG_LENGTH * 8), jnonce);
  if (!PLUX_JNI_OK(env) || spec == NULL) goto done;

  // Cipher.ENCRYPT_MODE is 1 and DECRYPT_MODE is 2.
  (*env)->CallVoidMethod(env, cipher, init, mode == PLUX_AEAD_SEAL ? 1 : 2, secret, spec);
  if (!PLUX_JNI_OK(env)) goto done;
  if (aad_length > 0) {
    jbyteArray jaad = (*env)->NewByteArray(env, (jsize)aad_length);
    if (!PLUX_JNI_OK(env) || jaad == NULL) goto done;
    (*env)->SetByteArrayRegion(env, jaad, 0, (jsize)aad_length, (const jbyte*)aad);
    (*env)->CallVoidMethod(env, cipher, update_aad, jaad);
    if (!PLUX_JNI_OK(env)) goto done;
  }
  jbyteArray joutput = (jbyteArray)(*env)->CallObjectMethod(env, cipher, do_final, jin);
  if (!PLUX_JNI_OK(env)) {
    // After a successful init, an open fails only because the tag does not
    // verify. The exception's class does not decide the result.
    result = mode == PLUX_AEAD_OPEN ? PLUX_AEAD_AUTH : PLUX_AEAD_FAILED;
    goto done;
  }
  if (joutput == NULL) goto done;
  jsize n = (*env)->GetArrayLength(env, joutput);
  if ((uint64_t)n > out_capacity) {
    result = PLUX_AEAD_CAPACITY;
    goto done;
  }
  if (n > 0) (*env)->GetByteArrayRegion(env, joutput, 0, n, (jbyte*)out);
  if (PLUX_JNI_OK(env)) result = (int64_t)n;

done:
  (*env)->ExceptionClear(env);
  (*env)->PopLocalFrame(env, NULL);
  return result;
}

static int64_t plux_aead_platform(int32_t mode, const uint8_t* key, const uint8_t* nonce,
                                  const uint8_t* aad, uint64_t aad_length, const uint8_t* in,
                                  uint64_t in_length, uint8_t* out, uint64_t out_capacity) {
  JavaVM* vm = plux_vm;
  if (vm == NULL) return PLUX_AEAD_UNSUPPORTED;
  JNIEnv* env = NULL;
  int attached = 0;
  jint status = (*vm)->GetEnv(vm, (void**)&env, JNI_VERSION_1_6);
  if (status == JNI_EDETACHED) {
    if ((*vm)->AttachCurrentThread(vm, (plux_attach_arg)&env, NULL) != JNI_OK) return PLUX_AEAD_FAILED;
    attached = 1;
  } else if (status != JNI_OK) {
    return PLUX_AEAD_FAILED;
  }
  int64_t r = plux_aead_jni(env, mode, key, nonce, aad, aad_length, in, in_length, out, out_capacity);
  // A thread that was not attached on entry must not stay attached: the VM
  // aborts when such a thread exits.
  if (attached) (*vm)->DetachCurrentThread(vm);
  return r;
}

#elif defined(__APPLE__)

// The plugin's `plux_swift_aead` (PluxAead.swift), found when used; absent
// where there is no plugin, as on macOS test hosts.
typedef int64_t (*plux_swift_aead_fn)(int32_t, const uint8_t*, const uint8_t*, const uint8_t*,
                                      uint64_t, const uint8_t*, uint64_t, uint8_t*, uint64_t);

static int64_t plux_aead_platform(int32_t mode, const uint8_t* key, const uint8_t* nonce,
                                  const uint8_t* aad, uint64_t aad_length, const uint8_t* in,
                                  uint64_t in_length, uint8_t* out, uint64_t out_capacity) {
  plux_swift_aead_fn fn = (plux_swift_aead_fn)dlsym(RTLD_DEFAULT, "plux_swift_aead");
  if (fn == NULL) return PLUX_AEAD_UNSUPPORTED;
  return fn(mode, key, nonce, aad, aad_length, in, in_length, out, out_capacity);
}

#else

static int64_t plux_aead_platform(int32_t mode, const uint8_t* key, const uint8_t* nonce,
                                  const uint8_t* aad, uint64_t aad_length, const uint8_t* in,
                                  uint64_t in_length, uint8_t* out, uint64_t out_capacity) {
  (void)mode;
  (void)key;
  (void)nonce;
  (void)aad;
  (void)aad_length;
  (void)in;
  (void)in_length;
  (void)out;
  (void)out_capacity;
  return PLUX_AEAD_UNSUPPORTED;
}

#endif

// Seals (mode 0) or opens (mode 1) one record with AES-256-GCM: key is 32
// bytes, nonce 12, aad aad_length bytes. Sealing takes in_length bytes of
// plaintext and writes the ciphertext then the 16-byte tag; opening takes
// ciphertext then tag and writes the plaintext. out holds out_capacity bytes.
// Returns the bytes written, or a negative PLUX_AEAD_* result.
PLUX_EXPORT int64_t plux_aead(int32_t mode, const uint8_t* key, const uint8_t* nonce,
                              const uint8_t* aad, uint64_t aad_length, const uint8_t* in,
                              uint64_t in_length, uint8_t* out, uint64_t out_capacity) {
  if (mode != PLUX_AEAD_SEAL && mode != PLUX_AEAD_OPEN) return PLUX_AEAD_FAILED;
  if (key == NULL || nonce == NULL || out == NULL) return PLUX_AEAD_FAILED;
  if ((aad == NULL && aad_length > 0) || (in == NULL && in_length > 0)) return PLUX_AEAD_FAILED;
  // Records are far below 2 GiB; JNI's array lengths are 32-bit.
  if (in_length > 0x7fffffffu || aad_length > 0x7fffffffu) return PLUX_AEAD_FAILED;
  if (mode == PLUX_AEAD_OPEN && in_length < PLUX_AEAD_TAG_LENGTH) return PLUX_AEAD_AUTH;
  uint64_t expected =
      mode == PLUX_AEAD_SEAL ? in_length + PLUX_AEAD_TAG_LENGTH : in_length - PLUX_AEAD_TAG_LENGTH;
  if (out_capacity < expected) return PLUX_AEAD_CAPACITY;
  int64_t r = plux_aead_platform(mode, key, nonce, aad, aad_length, in, in_length, out, out_capacity);
  // Whatever the platform answers, only the exact length is a success.
  if (r >= 0 && (uint64_t)r != expected) return PLUX_AEAD_FAILED;
  return r;
}
