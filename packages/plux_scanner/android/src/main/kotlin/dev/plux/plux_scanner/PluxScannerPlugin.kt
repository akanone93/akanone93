// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_scanner

import android.app.Activity
import android.content.Context
import com.google.android.gms.common.ConnectionResult
import com.google.android.gms.common.GoogleApiAvailability
import com.google.android.gms.common.moduleinstall.ModuleInstall
import com.google.android.gms.common.moduleinstall.ModuleInstallRequest
import com.google.mlkit.vision.barcode.BarcodeScanning
import io.flutter.embedding.engine.plugins.FlutterPlugin
import io.flutter.embedding.engine.plugins.activity.ActivityAware
import io.flutter.embedding.engine.plugins.activity.ActivityPluginBinding
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel

/**
 * ML Kit's unbundled barcode model for `plux_scanner` (decision B21,
 * ADR-0051). The model is downloaded by Google Play services and shared
 * between apps. At the first app open the plugin asks for it in the
 * background; Dart asks for its state before it opens a scanner and falls
 * back to ZXing-C++ while the state is not `ready`.
 *
 * Channel `dev.plux/scanner`:
 * - `mlKitState` answers `ready` (the model is installed), `downloading`
 *   (Play services is there and the model was requested) or `unavailable`
 *   (no Play services, or the module install API failed).
 * - `requestModel` asks for the download and answers at once.
 * - `start` (`back`, `torch`), `stop` and `setTorch` (`on`) run the CameraX
 *   camera of the ZXing engine, see [PluxCameraFrames]; its frames arrive on
 *   the event channel `dev.plux/scanner/frames`.
 */
class PluxScannerPlugin : FlutterPlugin, ActivityAware, MethodChannel.MethodCallHandler {
    private var channel: MethodChannel? = null
    private var frameChannel: EventChannel? = null
    private var context: Context? = null
    private var activity: Activity? = null
    private var camera: PluxCameraFrames? = null
    private var binding: ActivityPluginBinding? = null

    override fun onAttachedToEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        context = binding.applicationContext
        channel = MethodChannel(binding.binaryMessenger, CHANNEL).also {
            it.setMethodCallHandler(this)
        }
        camera = PluxCameraFrames(binding.textureRegistry) { activity }.also {
            frameChannel = EventChannel(binding.binaryMessenger, FRAMES).apply { setStreamHandler(it) }
        }
        // The first app open: the model should be on its way before a scan.
        state { }
    }

    override fun onDetachedFromEngine(binding: FlutterPlugin.FlutterPluginBinding) {
        camera?.stop()
        camera = null
        frameChannel?.setStreamHandler(null)
        frameChannel = null
        channel?.setMethodCallHandler(null)
        channel = null
        context = null
    }

    override fun onAttachedToActivity(binding: ActivityPluginBinding) {
        activity = binding.activity
        this.binding = binding
        camera?.let { binding.addRequestPermissionsResultListener(it) }
    }

    override fun onDetachedFromActivity() {
        camera?.stop()
        onDetachedFromActivityForConfigChanges()
    }

    override fun onReattachedToActivityForConfigChanges(binding: ActivityPluginBinding) =
        onAttachedToActivity(binding)

    override fun onDetachedFromActivityForConfigChanges() {
        camera?.let { binding?.removeRequestPermissionsResultListener(it) }
        binding = null
        activity = null
    }

    override fun onMethodCall(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "mlKitState" -> state { result.success(it) }
            "requestModel" -> {
                state { }
                result.success(null)
            }
            "start" -> {
                val cam = camera
                if (cam == null) {
                    result.error("unsupported", "no camera", null)
                } else {
                    cam.start(
                        call.argument<Boolean>("back") ?: true,
                        call.argument<Boolean>("torch") ?: false,
                        result,
                    )
                }
            }
            "stop" -> {
                camera?.stop()
                result.success(null)
            }
            "setTorch" -> {
                camera?.setTorch(call.argument<Boolean>("on") ?: false)
                result.success(null)
            }
            else -> result.notImplemented()
        }
    }

    /**
     * Reports the model's state to [done] exactly once and, when the model is
     * missing but Play services is there, requests its download. Never throws:
     * a failure of the Play services API is `unavailable`.
     */
    private fun state(done: (String) -> Unit) {
        val app = context
        if (app == null) {
            done("unavailable")
            return
        }
        try {
            if (GoogleApiAvailability.getInstance().isGooglePlayServicesAvailable(app) !=
                ConnectionResult.SUCCESS
            ) {
                done("unavailable")
                return
            }
            val installer = ModuleInstall.getClient(app)
            val scanner = BarcodeScanning.getClient()
            installer.areModulesAvailable(scanner)
                .addOnSuccessListener { availability ->
                    if (availability.areModulesAvailable()) {
                        done("ready")
                    } else {
                        installer.installModules(
                            ModuleInstallRequest.newBuilder().addApi(scanner).build()
                        )
                        done("downloading")
                    }
                }
                .addOnFailureListener { done("unavailable") }
        } catch (e: Exception) {
            done("unavailable")
        }
    }

    companion object {
        private const val CHANNEL = "dev.plux/scanner"
        private const val FRAMES = "dev.plux/scanner/frames"
    }
}
