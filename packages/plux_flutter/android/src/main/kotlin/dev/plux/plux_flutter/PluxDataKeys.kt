// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_flutter

import android.os.Handler
import android.os.Looper
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.security.KeyStore
import java.security.UnrecoverableKeyException
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.RejectedExecutionException
import javax.crypto.AEADBadTagException
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * The key that wraps the data keys of the encrypted local stores (SEC-073,
 * ADR-0058): one AES-256 key inside the Android Keystore, never exported. The
 * runtime hands over a data key to wrap and gets it back once per launch;
 * the wrapped bytes are the 12-byte nonce, the ciphertext and the tag.
 * Keystore work runs on one background thread and the result is posted back
 * to the main thread, which `MethodChannel.Result` requires. Neither key
 * material nor an alias ever appears in an error.
 */
class PluxDataKeys {
    private val executor: ExecutorService = Executors.newSingleThreadExecutor()
    private val main = Handler(Looper.getMainLooper())

    /** Handles dataKeyWrap and dataKeyUnwrap; never throws. */
    fun handle(call: MethodCall, result: MethodChannel.Result) {
        val task: () -> Any? = try {
            when (call.method) {
                "dataKeyWrap" -> wrapTask(call)
                "dataKeyUnwrap" -> unwrapTask(call)
                else -> throw IllegalArgumentException("method")
            }
        } catch (e: IllegalArgumentException) {
            result.error(PLATFORM, e.javaClass.simpleName, null)
            return
        }
        try {
            executor.execute {
                val outcome = runTask(task)
                main.post { outcome.deliver(result) }
            }
        } catch (e: RejectedExecutionException) {
            result.error(PLATFORM, e.javaClass.simpleName, null)
        }
    }

    /** Stops the background thread; called when the engine detaches. */
    fun close() {
        executor.shutdown()
    }

    private fun wrapTask(call: MethodCall): () -> Any? {
        val data = call.argument<ByteArray>("data") ?: throw IllegalArgumentException("data")
        require(data.isNotEmpty() && data.size <= MAX_BYTES) { "data" }
        return { wrap(data) }
    }

    private fun unwrapTask(call: MethodCall): () -> Any? {
        val wrapped = call.argument<ByteArray>("wrapped") ?: throw IllegalArgumentException("wrapped")
        require(wrapped.size > IV_BYTES + TAG_BYTES && wrapped.size <= MAX_BYTES + IV_BYTES + TAG_BYTES) { "wrapped" }
        return { unwrap(wrapped) }
    }

    private fun runTask(task: () -> Any?): Outcome =
        try {
            Outcome(null, null, task())
        } catch (e: KeyFailure) {
            Outcome(MISSING, e.reason, null)
        } catch (e: Exception) {
            Outcome(PLATFORM, e.javaClass.simpleName, null)
        }

    private fun store(): KeyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }

    /** The wrapping key, made on first use. */
    private fun key(create: Boolean): SecretKey? {
        val keyStore = store()
        try {
            (keyStore.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        } catch (e: UnrecoverableKeyException) {
            // Invalidated: nothing it wrapped can be opened again.
            keyStore.deleteEntry(ALIAS)
        }
        if (!create) return null
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return generator.generateKey()
    }

    private fun wrap(data: ByteArray): ByteArray {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key(true))
        return cipher.iv + cipher.doFinal(data)
    }

    private fun unwrap(wrapped: ByteArray): ByteArray {
        val key = key(false) ?: throw KeyFailure("missing")
        val cipher = Cipher.getInstance(TRANSFORMATION)
        // Only a key that is gone or does not match is a lost key; any other
        // failure may be transient and reaches the caller as one.
        try {
            cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(TAG_BYTES * 8, wrapped, 0, IV_BYTES))
            return cipher.doFinal(wrapped, IV_BYTES, wrapped.size - IV_BYTES)
        } catch (e: KeyPermanentlyInvalidatedException) {
            store().deleteEntry(ALIAS)
            throw KeyFailure("invalidated")
        } catch (e: AEADBadTagException) {
            // Another key wrapped these bytes, or they were altered.
            throw KeyFailure("unwrap")
        }
    }

    /** The key that wrapped the bytes is gone or is not this one. */
    private class KeyFailure(val reason: String) : Exception(reason)

    private class Outcome(val code: String?, val message: String?, val value: Any?) {
        fun deliver(result: MethodChannel.Result) {
            if (code == null) result.success(value) else result.error(code, message, null)
        }
    }

    private companion object {
        const val PLATFORM = "PLUX_PLATFORM"
        const val MISSING = "PLUX_KEY_MISSING"
        const val KEYSTORE = "AndroidKeyStore"
        const val ALIAS = "dev.plux.datakeys"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val IV_BYTES = 12
        const val TAG_BYTES = 16
        const val MAX_BYTES = 1024
    }
}
