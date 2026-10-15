// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_flutter

import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import androidx.core.content.ContextCompat
import androidx.core.os.BundleCompat
import com.google.android.gms.auth.api.phone.SmsRetriever
import com.google.android.gms.common.api.CommonStatusCodes
import com.google.android.gms.common.api.Status
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import io.flutter.plugin.common.PluginRegistry

/**
 * One-time codes by SMS for `OtpInput` (SEC-090) through the SMS User
 * Consent API: the user is asked to share the one message that matches, and
 * the app holds no SMS permission. At most one wait runs; the answer is the
 * message text, or null when the user declines, no message arrives in five
 * minutes, or the wait is cancelled. The text is a secret and is never
 * logged (SEC-092).
 */
class PluxSmsConsent : PluginRegistry.ActivityResultListener {
    /** The foreground activity, or null while none is attached. */
    var activity: Activity? = null

    private var pending: MethodChannel.Result? = null
    private var receiver: BroadcastReceiver? = null

    /** The context [receiver] is registered with, kept so it is unregistered there. */
    private var registeredOn: Context? = null

    fun handle(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "smsConsent" -> start(result)
            else -> {
                finish(null)
                result.success(null)
            }
        }
    }

    private fun start(result: MethodChannel.Result) {
        val host = activity
        if (host == null || pending != null) {
            result.error("PLUX_PLATFORM", if (host == null) "activity" else "busy", null)
            return
        }
        pending = result
        val listener = object : BroadcastReceiver() {
            override fun onReceive(context: Context, intent: Intent) {
                if (intent.action != SmsRetriever.SMS_RETRIEVED_ACTION) return
                val extras = intent.extras ?: return
                val status = BundleCompat.getParcelable(extras, SmsRetriever.EXTRA_STATUS, Status::class.java)
                unregister()
                if (status?.statusCode != CommonStatusCodes.SUCCESS) {
                    finish(null)
                    return
                }
                val consent = BundleCompat.getParcelable(extras, SmsRetriever.EXTRA_CONSENT_INTENT, Intent::class.java)
                val current = activity
                if (consent == null || current == null) {
                    finish(null)
                    return
                }
                try {
                    current.startActivityForResult(consent, REQUEST_CODE)
                } catch (e: Exception) {
                    finish(null)
                }
            }
        }
        receiver = listener
        // The application context outlives a configuration change of the
        // activity, so the receiver is always unregistered where it was
        // registered.
        val context = host.applicationContext
        registeredOn = context
        // The documented pattern: the sender is Play Services, which holds
        // SEND_PERMISSION, so only it can deliver to this receiver.
        ContextCompat.registerReceiver(
            context,
            listener,
            IntentFilter(SmsRetriever.SMS_RETRIEVED_ACTION),
            SmsRetriever.SEND_PERMISSION,
            null,
            ContextCompat.RECEIVER_EXPORTED,
        )
        SmsRetriever.getClient(host).startSmsUserConsent(null).addOnFailureListener {
            unregister()
            finish(null)
        }
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?): Boolean {
        if (requestCode != REQUEST_CODE) return false
        val message = if (resultCode == Activity.RESULT_OK) data?.getStringExtra(SmsRetriever.EXTRA_SMS_MESSAGE) else null
        finish(message)
        return true
    }

    /** Ends the wait, as when the activity goes. */
    fun detach() {
        finish(null)
        activity = null
    }

    private fun unregister() {
        val listener = receiver ?: return
        val context = registeredOn
        receiver = null
        registeredOn = null
        try {
            context?.unregisterReceiver(listener)
        } catch (e: IllegalArgumentException) {
            // Already unregistered.
        }
    }

    private fun finish(message: String?) {
        unregister()
        val result = pending ?: return
        pending = null
        result.success(message)
    }

    private companion object {
        const val REQUEST_CODE = 0x5200
    }
}
