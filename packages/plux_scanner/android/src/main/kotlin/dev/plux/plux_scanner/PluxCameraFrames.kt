// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_scanner

import android.Manifest
import android.app.Activity
import android.content.pm.PackageManager
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.view.Surface
import androidx.camera.core.Camera
import androidx.camera.core.CameraSelector
import androidx.camera.core.ImageAnalysis
import androidx.camera.core.Preview
import androidx.camera.lifecycle.ProcessCameraProvider
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import androidx.lifecycle.LifecycleOwner
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodChannel
import io.flutter.plugin.common.PluginRegistry
import io.flutter.view.TextureRegistry
import java.util.concurrent.Executor
import java.util.concurrent.Executors

/**
 * The camera of the ZXing engine (decision B21): CameraX shows its preview
 * in a Flutter texture and hands the luminance plane of at most ten frames a
 * second to Dart on the event channel `dev.plux/scanner/frames`, as a map of
 * `bytes`, `width`, `height`, `rowStride` and `rotation`. At most one camera
 * runs. Errors are `permissionDenied`, `unsupported` and `failed`.
 */
class PluxCameraFrames(
    private val textures: TextureRegistry,
    private val activity: () -> Activity?,
) : EventChannel.StreamHandler, PluginRegistry.RequestPermissionsResultListener {
    private val main = Handler(Looper.getMainLooper())
    private val mainExecutor = Executor { main.post(it) }
    private val analysisExecutor = Executors.newSingleThreadExecutor()
    private var sink: EventChannel.EventSink? = null
    private var provider: ProcessCameraProvider? = null
    private var camera: Camera? = null
    private var texture: TextureRegistry.SurfaceTextureEntry? = null
    private var permissionWait: (() -> Unit)? = null
    private var permissionDenied: (() -> Unit)? = null
    private var lastFrame = 0L

    override fun onListen(arguments: Any?, events: EventChannel.EventSink) {
        sink = events
    }

    override fun onCancel(arguments: Any?) {
        sink = null
    }

    /** Starts the camera and answers with `textureId`, `width`, `height` and `rotation`. */
    fun start(back: Boolean, torch: Boolean, result: MethodChannel.Result) {
        stop()
        val host = activity()
        val owner = host as? LifecycleOwner
        if (host == null || owner == null) {
            result.error("unsupported", "no activity to bind the camera to", null)
            return
        }
        if (ContextCompat.checkSelfPermission(host, Manifest.permission.CAMERA) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            permissionWait = { bind(host, owner, back, torch, result) }
            permissionDenied = { result.error("permissionDenied", "camera denied", null) }
            ActivityCompat.requestPermissions(host, arrayOf(Manifest.permission.CAMERA), REQUEST)
            return
        }
        bind(host, owner, back, torch, result)
    }

    private fun bind(
        host: Activity,
        owner: LifecycleOwner,
        back: Boolean,
        torch: Boolean,
        result: MethodChannel.Result,
    ) {
        val future = ProcessCameraProvider.getInstance(host)
        future.addListener({
            try {
                val cameraProvider = future.get()
                val entry = textures.createSurfaceTexture()
                var answered = false
                val preview = Preview.Builder().build()
                preview.setSurfaceProvider(mainExecutor) { request ->
                    val size = request.resolution
                    entry.surfaceTexture().setDefaultBufferSize(size.width, size.height)
                    val surface = Surface(entry.surfaceTexture())
                    request.provideSurface(surface, mainExecutor) { surface.release() }
                    request.setTransformationInfoListener(mainExecutor) { info ->
                        if (!answered) {
                            answered = true
                            result.success(
                                mapOf(
                                    "textureId" to entry.id(),
                                    "width" to size.width,
                                    "height" to size.height,
                                    "rotation" to info.rotationDegrees,
                                )
                            )
                        }
                    }
                }
                val analysis = ImageAnalysis.Builder()
                    .setBackpressureStrategy(ImageAnalysis.STRATEGY_KEEP_ONLY_LATEST)
                    .setOutputImageFormat(ImageAnalysis.OUTPUT_IMAGE_FORMAT_YUV_420_888)
                    .build()
                analysis.setAnalyzer(analysisExecutor) { image ->
                    try {
                        val now = SystemClock.elapsedRealtime()
                        if (now - lastFrame >= FRAME_MS) {
                            lastFrame = now
                            val plane = image.planes[0]
                            val buffer = plane.buffer
                            val bytes = ByteArray(buffer.remaining())
                            buffer.get(bytes)
                            val frame = mapOf(
                                "bytes" to bytes,
                                "width" to image.width,
                                "height" to image.height,
                                "rowStride" to plane.rowStride,
                                "rotation" to image.imageInfo.rotationDegrees,
                            )
                            main.post { sink?.success(frame) }
                        }
                    } finally {
                        image.close()
                    }
                }
                cameraProvider.unbindAll()
                camera = cameraProvider.bindToLifecycle(
                    owner,
                    if (back) CameraSelector.DEFAULT_BACK_CAMERA else CameraSelector.DEFAULT_FRONT_CAMERA,
                    preview,
                    analysis,
                )
                camera?.cameraControl?.enableTorch(torch)
                provider = cameraProvider
                texture = entry
            } catch (e: Exception) {
                result.error("failed", e.javaClass.simpleName, null)
            }
        }, ContextCompat.getMainExecutor(host))
    }

    fun setTorch(on: Boolean) {
        camera?.cameraControl?.enableTorch(on)
    }

    fun stop() {
        permissionWait = null
        permissionDenied = null
        provider?.unbindAll()
        provider = null
        camera = null
        texture?.release()
        texture = null
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray,
    ): Boolean {
        if (requestCode != REQUEST) return false
        val granted = grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED
        val next = if (granted) permissionWait else permissionDenied
        permissionWait = null
        permissionDenied = null
        next?.invoke()
        return true
    }

    companion object {
        private const val REQUEST = 7411
        private const val FRAME_MS = 100L
    }
}
