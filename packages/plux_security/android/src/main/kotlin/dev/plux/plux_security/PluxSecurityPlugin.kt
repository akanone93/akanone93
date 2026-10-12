// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_security

import android.app.Activity
import android.os.Handler
import android.os.Looper
import io.flutter.embedding.engine.plugins.FlutterPlugin
import io.flutter.embedding.engine.plugins.activity.ActivityAware
import io.flutter.embedding.engine.plugins.activity.ActivityPluginBinding
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors

/**
 * The platform channel of plux_security (SEC-070, ADR-0057): one call,
 * `scan`, that runs the checks of [PluxRasp] on a background thread and
 * answers with what they found. Nothing here reads or returns a path, an
 * identifier or any value from the device, only the kind of finding and the
 * codes of the checks that tripped (SEC-092).
 */
class PluxSecurityPlugin : FlutterPlugin, ActivityAware, MethodChannel.MethodCallHandler {
    private lateinit var channel: MethodChannel
    private lateinit var rasp: PluxRasp
    private lateinit var worker: ExecutorService
    private val main = Handler(Looper.getMainLooper())
    private val touches = ObscuredTouchWatch()

    override fun onAttachedToEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        rasp = PluxRasp(binding.applicationContext)
        worker = Executors.newSingleThreadExecutor()
        channel = MethodChannel(binding.binaryMessenger, CHANNEL)
        channel.setMethodCallHandler(this)
    }

    override fun onDetachedFromEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        channel.setMethodCallHandler(null)
        worker.shutdown()
    }

    override fun onAttachedToActivity(binding: ActivityPluginBinding) = watch(binding.activity)

    override fun onDetachedFromActivityForConfigChanges() = touches.detach()

    override fun onReattachedToActivityForConfigChanges(binding: ActivityPluginBinding) =
        watch(binding.activity)

    override fun onDetachedFromActivity() = touches.detach()

    private fun watch(activity: Activity) = touches.attach(activity.window)

    override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        if (call.method != "scan") {
            result.notImplemented()
            return
        }
        val signers = call.argument<List<String>>("signingCertSha256") ?: emptyList()
        val installers = call.argument<List<String>>("trustedInstallers") ?: emptyList()
        worker.execute {
            try {
                val found = rasp.scan(signers, installers, touches.take())
                main.post { result.success(found) }
            } catch (e: Exception) {
                main.post { result.error("PLUX_PLATFORM", e.javaClass.simpleName, null) }
            }
        }
    }

    private companion object {
        const val CHANNEL = "dev.plux/security"
    }
}
