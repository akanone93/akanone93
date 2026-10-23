// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_flutter

import android.content.Context
import android.os.Handler
import android.os.Looper
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequest
import androidx.work.WorkManager
import androidx.work.Worker
import androidx.work.WorkerParameters
import io.flutter.FlutterInjector
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.embedding.engine.dart.DartExecutor
import io.flutter.plugin.common.MethodCall
import io.flutter.plugin.common.MethodChannel
import io.flutter.view.FlutterCallbackInformation
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicReference

/**
 * Opt-in background sync on Android (SYN-014, ADR-0092): one unique periodic
 * WorkManager job with network and battery constraints. The job starts a
 * headless Flutter engine that runs the Dart entry point
 * `pluxBackgroundSyncMain`, which verifies and stages a release and never
 * activates it. `backgroundSyncSchedule` replaces the job (`UPDATE`, so a
 * changed interval keeps the run in progress), and `backgroundSyncCancel`
 * removes it. WorkManager's own initializer needs nothing from the host.
 */
class PluxBackgroundSync(private val context: Context) {
    fun handle(call: MethodCall, result: MethodChannel.Result) {
        when (call.method) {
            "backgroundSyncSchedule" -> {
                val args = call.arguments as? Map<*, *>
                val callback = (args?.get("callback") as? Number)?.toLong()
                val descriptor = args?.get("descriptor") as? String
                if (args == null || callback == null || descriptor == null) {
                    result.error("PLUX_PLATFORM", "arguments", null)
                    return
                }
                schedule(callback, descriptor, BackgroundSyncSpec.fromArguments(args))
                result.success(null)
            }
            "backgroundSyncCancel" -> {
                cancel()
                result.success(null)
            }
            else -> result.notImplemented()
        }
    }

    private fun schedule(callback: Long, descriptor: String, spec: BackgroundSyncSpec) {
        PluxBackgroundSyncStore(context).save(callback, descriptor)
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(
            UNIQUE_WORK_NAME,
            ExistingPeriodicWorkPolicy.UPDATE,
            buildRequest(spec),
        )
    }

    private fun cancel() {
        WorkManager.getInstance(context).cancelUniqueWork(UNIQUE_WORK_NAME)
        PluxBackgroundSyncStore(context).clear()
    }

    companion object {
        /** The unique name of the periodic job; one per app. */
        const val UNIQUE_WORK_NAME = "dev.plux.backgroundSync"

        /** The first retry after a failed run; WorkManager backs off from here. */
        const val BACKOFF_MINUTES = 15L

        /** The periodic request for [spec]: its interval and constraints. */
        fun buildRequest(spec: BackgroundSyncSpec): PeriodicWorkRequest {
            val constraints = Constraints.Builder()
                .setRequiredNetworkType(
                    if (spec.requiresUnmeteredNetwork) NetworkType.UNMETERED else NetworkType.CONNECTED,
                )
                .setRequiresCharging(spec.requiresCharging)
                .setRequiresBatteryNotLow(spec.requiresBatteryNotLow)
                .build()
            return PeriodicWorkRequest.Builder(
                PluxBackgroundSyncWorker::class.java,
                spec.intervalMinutes,
                TimeUnit.MINUTES,
            )
                .setConstraints(constraints)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, BACKOFF_MINUTES, TimeUnit.MINUTES)
                .build()
        }
    }
}

/**
 * What the foreground left for the headless run: the raw handle of the Dart
 * entry point and the descriptor of the sync. Both are public data; no token
 * or key is kept here.
 */
internal class PluxBackgroundSyncStore(context: Context) {
    private val prefs = context.getSharedPreferences("dev.plux.background-sync", Context.MODE_PRIVATE)

    val callbackHandle: Long?
        get() = if (prefs.contains(CALLBACK)) prefs.getLong(CALLBACK, 0L) else null

    val descriptor: String?
        get() = prefs.getString(DESCRIPTOR, null)

    fun save(callback: Long, descriptor: String) {
        prefs.edit().putLong(CALLBACK, callback).putString(DESCRIPTOR, descriptor).apply()
    }

    fun clear() {
        prefs.edit().clear().apply()
    }

    private companion object {
        const val CALLBACK = "callback"
        const val DESCRIPTOR = "descriptor"
    }
}

/** The periodic job: runs one headless sync and maps how it ended to WorkManager's result. */
class PluxBackgroundSyncWorker(context: Context, params: WorkerParameters) : Worker(context, params) {
    @Volatile private var headless: PluxHeadlessEngine? = null

    override fun doWork(): Result {
        val store = PluxBackgroundSyncStore(applicationContext)
        val callback = store.callbackHandle ?: return Result.success()
        if (store.descriptor == null) return Result.success()
        val engine = PluxHeadlessEngine(applicationContext, store)
        headless = engine
        return when (engine.run(callback, TIMEOUT_MINUTES)) {
            // A verified release was staged, or there was nothing newer.
            "staged", "upToDate" -> Result.success()
            // The release failed verification (PLX-3503): trying again changes nothing.
            "rejected" -> Result.success()
            else -> Result.retry()
        }
    }

    override fun onStopped() {
        headless?.stop()
    }

    private companion object {
        /** WorkManager stops a job after ten minutes. */
        const val TIMEOUT_MINUTES = 8L
    }
}

/**
 * A Flutter engine without a screen, running the Dart entry point. Only the
 * Plux plugin is registered in it: the sync needs the runtime channel and
 * nothing else of the host.
 */
internal class PluxHeadlessEngine(private val context: Context, private val store: PluxBackgroundSyncStore) {
    private val main = Handler(Looper.getMainLooper())
    private val finished = CountDownLatch(1)
    @Volatile private var engine: FlutterEngine? = null

    /** Runs the entry point of [callbackHandle]; how it ended, or `timeout`. */
    fun run(callbackHandle: Long, timeoutMinutes: Long): String {
        val outcome = AtomicReference("failed")
        main.post {
            try {
                start(callbackHandle, outcome)
            } catch (e: Exception) {
                finished.countDown()
            }
        }
        val done = finished.await(timeoutMinutes, TimeUnit.MINUTES)
        stop()
        return if (done) outcome.get() else "timeout"
    }

    fun stop() {
        finished.countDown()
        main.post {
            engine?.destroy()
            engine = null
        }
    }

    private fun start(callbackHandle: Long, outcome: AtomicReference<String>) {
        val loader = FlutterInjector.instance().flutterLoader()
        loader.startInitialization(context)
        loader.ensureInitializationComplete(context, null)
        val info = FlutterCallbackInformation.lookupCallbackInformation(callbackHandle)
        if (info == null) {
            finished.countDown()
            return
        }
        // No automatic registration: the host's other plugins stay out.
        val flutter = FlutterEngine(context, null, false)
        flutter.plugins.add(PluxFlutterPlugin())
        engine = flutter
        MethodChannel(flutter.dartExecutor.binaryMessenger, CHANNEL).setMethodCallHandler { call, result ->
            when (call.method) {
                "descriptor" -> result.success(store.descriptor)
                "done" -> {
                    outcome.set(call.argument<String>("outcome") ?: "failed")
                    result.success(null)
                    finished.countDown()
                }
                else -> result.notImplemented()
            }
        }
        flutter.dartExecutor.executeDartCallback(
            DartExecutor.DartCallback(context.assets, loader.findAppBundlePath(), info),
        )
    }

    private companion object {
        const val CHANNEL = "dev.plux/background-sync"
    }
}
