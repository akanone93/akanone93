// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_flutter

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.view.WindowManager
import androidx.biometric.BiometricManager
import androidx.biometric.BiometricPrompt
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import androidx.fragment.app.FragmentActivity
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import io.flutter.plugin.common.PluginRegistry
import java.io.File

/**
 * The device actions that need the platform (SEC-080): the share sheet and
 * permission prompts, the biometric prompt (SEC-027, SEC-094) and the
 * protection of the window while secure pages show (SEC-091, SEC-093).
 * Nothing here reads or returns a picked file's contents; files are handed
 * to the share sheet through a content URI, and only files under the app's
 * cache or files directories can be shared. The biometric prompt answers
 * with a word and never with biometric data.
 */
internal class PluxDevice(private val context: Context) :
    PluginRegistry.RequestPermissionsResultListener {
    /**
     * The foreground activity, or null while none is attached. A new
     * activity (after a configuration change) gets the window protection
     * the secure pages on screen still need.
     */
    var activity: Activity? = null
        set(value) {
            field = value
            applySecureScreen()
        }

    private var secure = false
    private var prompting = false
    private val pending = HashMap<Int, MethodChannel.Result>()
    private var nextCode = REQUEST_BASE

    fun share(call: MethodCall, result: MethodChannel.Result) {
        val host: Context = activity ?: context
        val text = listOfNotNull(call.argument<String>("text"), call.argument<String>("url"))
            .joinToString("\n")
        val path = call.argument<String>("filePath")
        val mime = call.argument<String>("mimeType")
        val send = Intent(Intent.ACTION_SEND)
        if (path != null) {
            val file = File(path).canonicalFile
            if (!allowed(file)) {
                result.error("PLUX_PLATFORM", "file", null)
                return
            }
            val uri = FileProvider.getUriForFile(context, "${context.packageName}.plux.fileprovider", file)
            send.putExtra(Intent.EXTRA_STREAM, uri)
            send.type = mime ?: "application/octet-stream"
            send.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION)
            if (text.isNotEmpty()) send.putExtra(Intent.EXTRA_TEXT, text)
        } else {
            send.type = "text/plain"
            send.putExtra(Intent.EXTRA_TEXT, text)
        }
        val chooser = Intent.createChooser(send, null)
        if (host !is Activity) chooser.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        host.startActivity(chooser)
        result.success(null)
    }

    /**
     * Turns the protection of the window on or off: FLAG_SECURE blocks
     * screenshots and screen recording and blurs the app switcher
     * (SEC-091), and the window drops touches while another window covers
     * it (SEC-093).
     */
    fun secureScreen(call: MethodCall, result: MethodChannel.Result) {
        val enabled = call.argument<Boolean>("enabled")
        if (enabled == null) {
            result.error("PLUX_PLATFORM", "enabled", null)
            return
        }
        secure = enabled
        applySecureScreen()
        result.success(null)
    }

    private fun applySecureScreen() {
        val window = activity?.window ?: return
        if (secure) window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        else window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)
        window.decorView.filterTouchesWhenObscured = secure
    }

    /**
     * Asks the user to confirm with a biometric or, when allowed, the
     * device credential. The answer is one of "success", "cancelled",
     * "failed" or "unavailable". The host activity must be a
     * FragmentActivity, as FlutterFragmentActivity is.
     */
    fun biometricAuth(call: MethodCall, result: MethodChannel.Result) {
        val reason = call.argument<String>("reason")
        val credential = call.argument<Boolean>("allowDeviceCredential") ?: true
        if (reason.isNullOrEmpty()) {
            result.error("PLUX_PLATFORM", "reason", null)
            return
        }
        val host = activity as? FragmentActivity
        if (host == null) {
            result.error("PLUX_PLATFORM", "activity", null)
            return
        }
        if (prompting) {
            result.error("PLUX_PLATFORM", "busy", null)
            return
        }
        // BIOMETRIC_STRONG with DEVICE_CREDENTIAL needs API 30.
        val authenticators = when {
            !credential -> BiometricManager.Authenticators.BIOMETRIC_STRONG
            Build.VERSION.SDK_INT >= 30 ->
                BiometricManager.Authenticators.BIOMETRIC_STRONG or BiometricManager.Authenticators.DEVICE_CREDENTIAL
            else ->
                BiometricManager.Authenticators.BIOMETRIC_WEAK or BiometricManager.Authenticators.DEVICE_CREDENTIAL
        }
        if (BiometricManager.from(context).canAuthenticate(authenticators) != BiometricManager.BIOMETRIC_SUCCESS) {
            result.success("unavailable")
            return
        }
        val info = BiometricPrompt.PromptInfo.Builder()
            .setTitle(reason)
            .setAllowedAuthenticators(authenticators)
            .apply { if (!credential) setNegativeButtonText(host.getString(android.R.string.cancel)) }
            .build()
        prompting = true
        val prompt = BiometricPrompt(host, ContextCompat.getMainExecutor(host), object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(authResult: BiometricPrompt.AuthenticationResult) {
                prompting = false
                result.success("success")
            }

            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                prompting = false
                result.success(
                    when (errorCode) {
                        BiometricPrompt.ERROR_USER_CANCELED,
                        BiometricPrompt.ERROR_NEGATIVE_BUTTON,
                        BiometricPrompt.ERROR_CANCELED -> "cancelled"
                        BiometricPrompt.ERROR_NO_BIOMETRICS,
                        BiometricPrompt.ERROR_NO_DEVICE_CREDENTIAL,
                        BiometricPrompt.ERROR_HW_NOT_PRESENT,
                        BiometricPrompt.ERROR_HW_UNAVAILABLE,
                        BiometricPrompt.ERROR_SECURITY_UPDATE_REQUIRED -> "unavailable"
                        else -> "failed"
                    },
                )
            }
        })
        prompt.authenticate(info)
    }

    private fun allowed(file: File): Boolean {
        val roots = listOf(context.cacheDir, context.filesDir).map { it.canonicalFile }
        return roots.any { file.path.startsWith(it.path + File.separator) }
    }

    fun requestPermission(call: MethodCall, result: MethodChannel.Result) {
        val name = call.argument<String>("permission")
        val wanted = name?.let { permissions(it) }
        if (wanted == null) {
            result.error("PLUX_PLATFORM", "permission", null)
            return
        }
        if (wanted.isEmpty() || granted(wanted)) {
            result.success(true)
            return
        }
        val host = activity
        if (host == null) {
            result.error("PLUX_PLATFORM", "activity", null)
            return
        }
        val code = nextCode++
        if (nextCode > REQUEST_BASE + REQUEST_RANGE) nextCode = REQUEST_BASE
        pending[code] = result
        host.requestPermissions(wanted.toTypedArray(), code)
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray,
    ): Boolean {
        val result = pending.remove(requestCode) ?: return false
        // Location is granted when either precision is; the others need
        // every permission they asked for.
        val any = permissions.any { it.startsWith("android.permission.ACCESS_") }
        val ok = if (any) grantResults.any { it == PackageManager.PERMISSION_GRANTED }
        else grantResults.isNotEmpty() && grantResults.all { it == PackageManager.PERMISSION_GRANTED }
        result.success(ok)
        return true
    }

    private fun granted(wanted: List<String>): Boolean {
        val states = wanted.map { ContextCompat.checkSelfPermission(context, it) == PackageManager.PERMISSION_GRANTED }
        return if (wanted.any { it.startsWith("android.permission.ACCESS_") }) states.any { it } else states.all { it }
    }

    /** The runtime permissions behind a device API; empty when none is needed. */
    private fun permissions(api: String): List<String>? = when (api) {
        "camera" -> listOf(Manifest.permission.CAMERA)
        "photos" -> if (Build.VERSION.SDK_INT >= 33) listOf(Manifest.permission.READ_MEDIA_IMAGES)
        else listOf(Manifest.permission.READ_EXTERNAL_STORAGE)
        "location" -> listOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION)
        "contacts" -> listOf(Manifest.permission.READ_CONTACTS)
        "notifications" -> if (Build.VERSION.SDK_INT >= 33) listOf(Manifest.permission.POST_NOTIFICATIONS)
        else emptyList()
        // USE_BIOMETRIC is a normal permission: granted at install.
        "biometrics" -> emptyList()
        else -> null
    }

    private companion object {
        const val REQUEST_BASE = 0x5100
        const val REQUEST_RANGE = 0xFF
    }
}
