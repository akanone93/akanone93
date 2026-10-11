// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_flutter

import android.app.Activity
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyInfo
import android.security.keystore.KeyPermanentlyInvalidatedException
import android.security.keystore.KeyProperties
import android.security.keystore.StrongBoxUnavailableException
import androidx.annotation.RequiresApi
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.math.BigInteger
import java.security.KeyFactory
import java.security.KeyPairGenerator
import java.security.KeyStore
import java.security.PrivateKey
import java.security.ProviderException
import java.security.Signature
import java.security.UnrecoverableKeyException
import java.security.interfaces.ECPublicKey
import java.security.spec.ECGenParameterSpec
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.RejectedExecutionException

/**
 * The device keys of the Plux runtime (SEC-001, SEC-021, ADR-0012): P-256 key
 * pairs created inside the Android Keystore and never exported. A "dpop" key
 * signs ES256 proofs; an "agreement" key is created for ECDH (API 31 and
 * later). Keystore work runs on one background thread and the result is
 * posted back to the main thread, which `MethodChannel.Result` requires.
 * Neither key material nor an alias ever appears in an error.
 */
class PluxKeys {
    private val executor: ExecutorService = Executors.newSingleThreadExecutor()
    private val main = Handler(Looper.getMainLooper())

    /** The host activity the system prompt of an "sca" key needs; set by the plugin. */
    var activity: Activity? = null

    /** Handles keyCreate, keyPublic, keySign and keyDelete; never throws. */
    fun handle(call: MethodCall, result: MethodChannel.Result) {
        val task: () -> Any? = try {
            when (call.method) {
                "keyCreate" -> createTask(call)
                "keyPublic" -> publicTask(call)
                "keySign" -> signTask(call)
                "keyDelete" -> deleteTask(call)
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

    /**
     * Handles keySignAuth (SEC-027): signs with an "sca" key behind the system prompt, which
     * asks for a biometric or, from API 30, the device credential. The signature object is
     * unlocked by the prompt itself (a CryptoObject), so the key is usable only after the
     * user authenticated for this very signature.
     */
    fun handleAuth(call: MethodCall, result: MethodChannel.Result) {
        val host = activity as? FragmentActivity
        val alias: String
        val data: ByteArray
        val title: String
        val subtitle: String
        try {
            alias = alias(call)
            data = call.argument<ByteArray>("data") ?: throw IllegalArgumentException("data")
            title = call.argument<String>("title") ?: throw IllegalArgumentException("title")
            subtitle = call.argument<String>("subtitle") ?: ""
        } catch (e: IllegalArgumentException) {
            result.error(PLATFORM, e.javaClass.simpleName, null)
            return
        }
        if (host == null) {
            // The prompt needs a FragmentActivity: the host uses FlutterFragmentActivity.
            result.error(PLATFORM, "activity", null)
            return
        }
        try {
            executor.execute {
                val outcome = runTask { initSignature(alias) }
                main.post {
                    val signature = outcome.value as? Signature
                    if (signature == null) outcome.deliver(result) else prompt(host, signature, data, title, subtitle, result)
                }
            }
        } catch (e: RejectedExecutionException) {
            result.error(PLATFORM, e.javaClass.simpleName, null)
        }
    }

    private fun initSignature(alias: String): Signature {
        val keyStore = store()
        val key = privateKey(keyStore, alias) ?: throw KeyFailure(MISSING, "missing")
        try {
            return Signature.getInstance("SHA256withECDSA").apply { initSign(key) }
        } catch (e: KeyPermanentlyInvalidatedException) {
            // A new biometric enrolment invalidates the key; the runtime enrols again.
            keyStore.deleteEntry(alias)
            throw KeyFailure(MISSING, "invalidated")
        }
    }

    private fun prompt(
        host: FragmentActivity,
        signature: Signature,
        data: ByteArray,
        title: String,
        subtitle: String,
        result: MethodChannel.Result,
    ) {
        val callback = object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(authenticated: BiometricPrompt.AuthenticationResult) {
                val unlocked = authenticated.cryptoObject?.signature
                if (unlocked == null) {
                    result.error(PLATFORM, "crypto", null)
                    return
                }
                executor.execute {
                    val outcome = runTask {
                        unlocked.update(data)
                        derToRaw(unlocked.sign())
                    }
                    main.post { outcome.deliver(result) }
                }
            }

            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                // Only the code is reported, never the system's text.
                when (errorCode) {
                    BiometricPrompt.ERROR_USER_CANCELED,
                    BiometricPrompt.ERROR_NEGATIVE_BUTTON,
                    BiometricPrompt.ERROR_CANCELED -> result.error(CANCELLED, null, null)
                    else -> result.error(PLATFORM, "auth $errorCode", null)
                }
            }
        }
        // Before API 30 a CryptoObject cannot be unlocked with the device credential.
        val credential = Build.VERSION.SDK_INT >= Build.VERSION_CODES.R
        val authenticators = if (credential) {
            BiometricManager.Authenticators.BIOMETRIC_STRONG or BiometricManager.Authenticators.DEVICE_CREDENTIAL
        } else {
            BiometricManager.Authenticators.BIOMETRIC_STRONG
        }
        val info = BiometricPrompt.PromptInfo.Builder()
            .setTitle(title)
            .setSubtitle(subtitle)
            .setAllowedAuthenticators(authenticators)
            .apply { if (!credential) setNegativeButtonText("Cancel") }
            .build()
        BiometricPrompt(host, ContextCompat.getMainExecutor(host), callback)
            .authenticate(info, BiometricPrompt.CryptoObject(signature))
    }

    /** Stops the background thread; called when the engine detaches. */
    fun close() {
        executor.shutdown()
    }

    private fun createTask(call: MethodCall): () -> Any? {
        val alias = alias(call)
        val purpose = call.argument<String>("purpose") ?: throw IllegalArgumentException("purpose")
        require(purpose == DPOP || purpose == AGREEMENT || purpose == SCA) { "purpose" }
        val challenge = call.argument<ByteArray>("challenge")?.takeIf { it.isNotEmpty() }
        val strongBox = call.argument<Boolean>("strongBox") == true
        return { create(alias, purpose, challenge, strongBox) }
    }

    private fun publicTask(call: MethodCall): () -> Any? {
        val alias = alias(call)
        return { describe(store(), alias, false, false) }
    }

    private fun signTask(call: MethodCall): () -> Any? {
        val alias = alias(call)
        val data = call.argument<ByteArray>("data") ?: throw IllegalArgumentException("data")
        return { sign(alias, data) }
    }

    private fun deleteTask(call: MethodCall): () -> Any? {
        val alias = alias(call)
        return {
            store().deleteEntry(alias)
            null
        }
    }

    private fun alias(call: MethodCall): String {
        val a = call.argument<String>("alias") ?: throw IllegalArgumentException("alias")
        require(ALIAS.matches(a)) { "alias" }
        return a
    }

    private fun runTask(task: () -> Any?): Outcome =
        try {
            Outcome(null, null, task())
        } catch (e: KeyFailure) {
            Outcome(e.code, e.reason, null)
        } catch (e: Exception) {
            Outcome(PLATFORM, e.javaClass.simpleName, null)
        }

    private fun store(): KeyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }

    private fun create(alias: String, purpose: String, challenge: ByteArray?, strongBox: Boolean): Map<String, Any> {
        // Agreement keys need PURPOSE_AGREE_KEY, which exists from API 31.
        if (purpose == AGREEMENT && Build.VERSION.SDK_INT < Build.VERSION_CODES.S) {
            throw KeyFailure(UNSUPPORTED, "agreement")
        }
        val keyStore = store()
        keyStore.deleteEntry(alias)
        val inStrongBox = generate(alias, purpose, challenge, strongBox)
        return describe(keyStore, alias, challenge != null, inStrongBox)
            ?: throw IllegalStateException("missing")
    }

    /** Generates the pair; returns whether it lives in StrongBox. */
    private fun generate(alias: String, purpose: String, challenge: ByteArray?, strongBox: Boolean): Boolean {
        if (strongBox && Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            try {
                generateWith(spec(alias, purpose, challenge, true))
                return true
            } catch (e: ProviderException) {
                // Only an absent StrongBox is retried; any other failure stands.
                if (!isStrongBoxUnavailable(e)) throw e
            }
        }
        generateWith(spec(alias, purpose, challenge, false))
        return false
    }

    @RequiresApi(Build.VERSION_CODES.P)
    private fun isStrongBoxUnavailable(e: ProviderException): Boolean =
        e is StrongBoxUnavailableException || e.cause is StrongBoxUnavailableException

    private fun generateWith(spec: KeyGenParameterSpec) {
        val generator = KeyPairGenerator.getInstance(KeyProperties.KEY_ALGORITHM_EC, KEYSTORE)
        generator.initialize(spec)
        generator.generateKeyPair()
    }

    private fun spec(alias: String, purpose: String, challenge: ByteArray?, strongBox: Boolean): KeyGenParameterSpec {
        val purposes = if (purpose == AGREEMENT) KeyProperties.PURPOSE_AGREE_KEY else KeyProperties.PURPOSE_SIGN
        val builder = KeyGenParameterSpec.Builder(alias, purposes)
            .setAlgorithmParameterSpec(ECGenParameterSpec(CURVE))
        if (purpose != AGREEMENT) builder.setDigests(KeyProperties.DIGEST_SHA256)
        if (purpose == SCA) requireUserAuthentication(builder)
        if (challenge != null) builder.setAttestationChallenge(challenge)
        if (strongBox && Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) requestStrongBox(builder)
        return builder.build()
    }

    /**
     * Every signature needs the user (SEC-027): a biometric or, from API 30, the device
     * credential, for each use. Creation fails where the device has no secure screen lock.
     */
    @Suppress("DEPRECATION")
    private fun requireUserAuthentication(builder: KeyGenParameterSpec.Builder) {
        builder.setUserAuthenticationRequired(true)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            builder.setUserAuthenticationParameters(
                0,
                KeyProperties.AUTH_BIOMETRIC_STRONG or KeyProperties.AUTH_DEVICE_CREDENTIAL,
            )
        } else {
            builder.setUserAuthenticationValidityDurationSeconds(-1)
        }
    }

    @RequiresApi(Build.VERSION_CODES.P)
    private fun requestStrongBox(builder: KeyGenParameterSpec.Builder) {
        builder.setIsStrongBoxBacked(true)
    }

    /**
     * The public key, where the key lives and, when [withChain], the
     * attestation chain, leaf first; null when no key exists under [alias].
     */
    private fun describe(
        keyStore: KeyStore,
        alias: String,
        withChain: Boolean,
        inStrongBox: Boolean,
    ): Map<String, Any>? {
        val privateKey = privateKey(keyStore, alias) ?: return null
        val publicKey = keyStore.getCertificate(alias)?.publicKey as? ECPublicKey ?: return null
        val chain = if (withChain) {
            (keyStore.getCertificateChain(alias) ?: emptyArray()).map { it.encoded }
        } else {
            emptyList()
        }
        return mapOf(
            "x" to unsigned32(publicKey.w.affineX),
            "y" to unsigned32(publicKey.w.affineY),
            "storage" to storage(privateKey, inStrongBox),
            "chain" to chain,
        )
    }

    @Suppress("DEPRECATION")
    private fun storage(privateKey: PrivateKey, inStrongBox: Boolean): String {
        val info = KeyFactory.getInstance(privateKey.algorithm, KEYSTORE).getKeySpec(privateKey, KeyInfo::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) return levelOf(info)
        // Before API 31 only a boolean exists. A StrongBox key cannot be told
        // from a TEE key afterwards, so keyPublic reports "tee" for both.
        if (!info.isInsideSecureHardware) return "software"
        return if (inStrongBox) "strongbox" else "tee"
    }

    @RequiresApi(Build.VERSION_CODES.S)
    private fun levelOf(info: KeyInfo): String =
        when (info.securityLevel) {
            KeyProperties.SECURITY_LEVEL_STRONGBOX -> "strongbox"
            KeyProperties.SECURITY_LEVEL_TRUSTED_ENVIRONMENT -> "tee"
            else -> "software"
        }

    /** The private key handle; an unrecoverable entry is deleted and reads as missing. */
    private fun privateKey(keyStore: KeyStore, alias: String): PrivateKey? =
        try {
            keyStore.getKey(alias, null) as? PrivateKey
        } catch (e: UnrecoverableKeyException) {
            keyStore.deleteEntry(alias)
            throw KeyFailure(MISSING, "invalidated")
        }

    private fun sign(alias: String, data: ByteArray): ByteArray {
        val keyStore = store()
        val key = privateKey(keyStore, alias) ?: throw KeyFailure(MISSING, "missing")
        try {
            val signature = Signature.getInstance("SHA256withECDSA")
            signature.initSign(key)
            signature.update(data)
            return derToRaw(signature.sign())
        } catch (e: KeyPermanentlyInvalidatedException) {
            // The runtime re-registers a key reported missing.
            keyStore.deleteEntry(alias)
            throw KeyFailure(MISSING, "invalidated")
        }
    }

    /** A failure the channel reports under a specific code. */
    private class KeyFailure(val code: String, val reason: String) : Exception(reason)

    private class Outcome(val code: String?, val message: String?, val value: Any?) {
        fun deliver(result: MethodChannel.Result) {
            if (code == null) result.success(value) else result.error(code, message, null)
        }
    }

    private companion object {
        const val PLATFORM = "PLUX_PLATFORM"
        const val MISSING = "PLUX_KEY_MISSING"
        const val UNSUPPORTED = "PLUX_KEY_UNSUPPORTED"
        const val CANCELLED = "PLUX_USER_CANCELLED"
        const val KEYSTORE = "AndroidKeyStore"
        const val CURVE = "secp256r1"
        const val DPOP = "dpop"
        const val AGREEMENT = "agreement"
        const val SCA = "sca"
        val ALIAS = Regex("^[a-z0-9._-]{1,64}$")
    }
}

/** A non-negative integer as 32 big-endian bytes, left-padded; a sign byte is dropped. */
internal fun unsigned32(value: BigInteger): ByteArray {
    require(value.signum() >= 0) { "negative" }
    val bytes = value.toByteArray()
    var start = 0
    while (start < bytes.size - 1 && bytes[start] == 0.toByte()) start++
    val length = bytes.size - start
    require(length <= 32) { "length" }
    val out = ByteArray(32)
    System.arraycopy(bytes, start, out, 32 - length, length)
    return out
}

/**
 * Converts a DER ECDSA signature (SEQUENCE of two INTEGERs) of a P-256 key to
 * the raw 64-byte r||s form of JWS ES256. The encoding is checked strictly:
 * definite short-form lengths, two positive minimally-encoded INTEGERs of at
 * most 32 significant bytes, and no trailing bytes. Anything else throws
 * [IllegalArgumentException].
 */
internal fun derToRaw(der: ByteArray): ByteArray {
    // 30 06 02 01 r 02 01 s is the shortest; two 33-byte integers the longest.
    require(der.size in 8..72) { "length" }
    require(der[0] == 0x30.toByte()) { "sequence" }
    require((der[1].toInt() and 0xFF) == der.size - 2) { "sequence length" }
    val raw = ByteArray(64)
    val next = readInteger(der, 2, raw, 0)
    require(readInteger(der, next, raw, 32) == der.size) { "trailing bytes" }
    return raw
}

/** Reads the INTEGER at [pos] into the 32-byte slot at [rawOffset]; returns the end offset. */
private fun readInteger(der: ByteArray, pos: Int, raw: ByteArray, rawOffset: Int): Int {
    require(pos + 2 <= der.size) { "truncated" }
    require(der[pos] == 0x02.toByte()) { "integer" }
    val length = der[pos + 1].toInt() and 0xFF
    require(length in 1..33) { "integer length" }
    val start = pos + 2
    val end = start + length
    require(end <= der.size) { "truncated" }
    val first = der[start].toInt() and 0xFF
    require((first and 0x80) == 0) { "negative" }
    var from = start
    if (first == 0) {
        // A leading zero is allowed only to keep a high bit from reading as a
        // sign; that also rules out zero itself.
        require(length > 1 && (der[start + 1].toInt() and 0x80) != 0) { "padding" }
        from = start + 1
    }
    val size = end - from
    require(size <= 32) { "integer size" }
    System.arraycopy(der, from, raw, rawOffset + 32 - size, size)
    return end
}
