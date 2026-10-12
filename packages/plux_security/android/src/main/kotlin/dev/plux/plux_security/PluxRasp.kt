// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_security

import android.accessibilityservice.AccessibilityServiceInfo
import android.content.Context
import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.hardware.display.DisplayManager
import android.os.Build
import android.os.Debug
import android.view.Display
import android.view.accessibility.AccessibilityManager
import java.io.File
import java.io.IOException
import java.net.InetSocketAddress
import java.net.Socket
import java.security.MessageDigest

/**
 * Plux's own checks for a hostile Android device (SEC-070, ADR-0057). Each
 * check is small and answers with the codes of the indicators that tripped;
 * a check that fails finds nothing instead of failing the scan.
 *
 * Every check can be evaded by someone who controls the device: root hiders
 * rename `su` and hide mounts, Frida can be renamed and moved to another
 * port, a debugger can be hidden from `TracerPid`. The checks raise the
 * cost of an attack; they are signals for the server to weigh, not proof.
 */
internal class PluxRasp(private val context: Context) {

    /**
     * Runs every check. [signers] are the expected signing certificate
     * digests (hex, lower case) and [installers] the trusted stores; a check
     * with nothing to compare against is not run. [overlay] holds the codes
     * of the obscured touches seen since the last scan.
     */
    fun scan(signers: List<String>, installers: List<String>, overlay: Set<String>): List<Map<String, Any>> =
        listOfNotNull(
            finding("root", safely { root() }),
            finding("hooking", safely { hooking() }),
            finding("debugger", safely { debugger() }),
            finding("emulator", safely { emulator() }),
            finding("repackaging", safely { repackaging(signers) }),
            finding("untrusted_installer", safely { installer(installers) }),
            finding("screen_capture", safely { screenCapture() }),
            finding("overlay", overlay.sorted()),
            finding("accessibility", safely { accessibility() }),
        )

    private fun finding(kind: String, indicators: List<String>): Map<String, Any>? =
        if (indicators.isEmpty()) null else mapOf("kind" to kind, "indicators" to indicators)

    private fun safely(check: () -> List<String>): List<String> =
        try {
            check()
        } catch (_: Exception) {
            emptyList()
        }

    /** Root: an `su` binary, test-keys firmware, Magisk's traces, a writable system partition. */
    private fun root(): List<String> = buildList {
        val path = System.getenv("PATH").orEmpty().split(':').map { "$it/su" }
        if ((SU_PATHS + path).any { File(it).exists() }) add("su_binary")
        if (Build.TAGS?.contains("test-keys") == true) add("test_keys")
        if (MAGISK_PATHS.any { File(it).exists() } || mounts().any { "magisk" in it.lowercase() }) add("magisk")
        if (writableSystem()) add("writable_system")
    }

    /** A partition that should be read-only is mounted read-write. */
    private fun writableSystem(): Boolean = mounts().any { line ->
        val fields = line.split(' ')
        fields.size > 3 && fields[1] in SYSTEM_MOUNTS && "rw" in fields[3].split(',')
    }

    private fun mounts(): List<String> = File("/proc/self/mounts").readLines()

    /** Hooking: Frida's port, files, library and threads; Xposed and LSPosed classes and libraries. */
    private fun hooking(): List<String> = buildList {
        if (listenerOn(FRIDA_PORT)) add("frida_port")
        if (FRIDA_FILES.any { File(it).exists() }) add("frida_files")
        val mapped = File("/proc/self/maps").readLines().map { it.lowercase() }
        if (mapped.any { line -> FRIDA_MAPS.any { it in line } }) add("frida_maps")
        if (mapped.any { line -> XPOSED_MAPS.any { it in line } }) add("xposed_maps")
        if (threadNames().any { it in FRIDA_THREADS }) add("frida_threads")
        try {
            Class.forName("de.robv.android.xposed.XposedBridge")
            add("xposed_class")
        } catch (_: ClassNotFoundException) {
            // Not present: the normal case.
        }
    }

    private fun listenerOn(port: Int): Boolean =
        try {
            Socket().use {
                it.connect(InetSocketAddress("127.0.0.1", port), CONNECT_TIMEOUT_MS)
                true
            }
        } catch (_: IOException) {
            false
        }

    private fun threadNames(): List<String> =
        File("/proc/self/task").listFiles().orEmpty().mapNotNull {
            try {
                File(it, "comm").readText().trim()
            } catch (_: IOException) {
                null
            }
        }

    /** A debugger is attached: the runtime says so, or a tracer holds the process. */
    private fun debugger(): List<String> = buildList {
        if (Debug.isDebuggerConnected()) add("jdwp")
        val tracer = File("/proc/self/status").readLines()
            .firstOrNull { it.startsWith("TracerPid:") }
            ?.substringAfter(':')?.trim()
        if (tracer != null && tracer != "0") add("tracer_pid")
    }

    /** An emulator: the build properties and the device files say so. */
    private fun emulator(): List<String> = buildList {
        val fingerprint = Build.FINGERPRINT.lowercase()
        if (fingerprint.startsWith("generic") || fingerprint.contains("emulator") ||
            Build.HARDWARE in EMULATOR_HARDWARE || Build.PRODUCT.startsWith("sdk") ||
            Build.MODEL.contains("Emulator") || Build.MODEL.contains("Android SDK built for") ||
            Build.MANUFACTURER.contains("Genymotion")
        ) {
            add("build_props")
        }
        if (QEMU_FILES.any { File(it).exists() }) add("qemu_files")
    }

    /** The signing certificate is not one of the expected ones. */
    @Suppress("DEPRECATION")
    private fun repackaging(expected: List<String>): List<String> {
        if (expected.isEmpty()) return emptyList()
        val pm = context.packageManager
        val signatures = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            pm.getPackageInfo(context.packageName, PackageManager.GET_SIGNING_CERTIFICATES)
                .signingInfo?.apkContentsSigners
        } else {
            pm.getPackageInfo(context.packageName, PackageManager.GET_SIGNATURES).signatures
        }
        val digests = signatures.orEmpty().map { sha256(it.toByteArray()) }
        return if (digests.isEmpty() || !expected.containsAll(digests)) listOf("signing_cert") else emptyList()
    }

    private fun sha256(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

    /** The app was installed by something other than a trusted store. */
    @Suppress("DEPRECATION")
    private fun installer(trusted: List<String>): List<String> {
        if (trusted.isEmpty()) return emptyList()
        val pm = context.packageManager
        val source = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            pm.getInstallSourceInfo(context.packageName).installingPackageName
        } else {
            pm.getInstallerPackageName(context.packageName)
        }
        return when {
            source == null -> listOf("installer_unknown")
            source !in trusted -> listOf("installer_untrusted")
            else -> emptyList()
        }
    }

    /**
     * A display other than the device's own is attached: a cast or mirroring
     * session. A recording of the device's own display is not visible to the
     * app on every Android version, so this finds mirroring, not recording.
     */
    private fun screenCapture(): List<String> = buildList {
        val displays = (context.getSystemService(Context.DISPLAY_SERVICE) as DisplayManager).displays
        if (displays.any { it.displayId != Display.DEFAULT_DISPLAY }) add("external_display")
        if (displays.any { it.flags and Display.FLAG_PRESENTATION != 0 }) add("presentation_display")
    }

    /**
     * An enabled accessibility service that is not part of the system and
     * can both read the window content and perform gestures: what banking
     * malware uses to read the screen and press buttons.
     */
    private fun accessibility(): List<String> {
        val manager = context.getSystemService(Context.ACCESSIBILITY_SERVICE) as AccessibilityManager
        val wanted = AccessibilityServiceInfo.CAPABILITY_CAN_RETRIEVE_WINDOW_CONTENT or
            AccessibilityServiceInfo.CAPABILITY_CAN_PERFORM_GESTURES
        val suspicious = manager.getEnabledAccessibilityServiceList(AccessibilityServiceInfo.FEEDBACK_ALL_MASK)
            .any { info ->
                val app = info.resolveInfo.serviceInfo.applicationInfo
                info.capabilities and wanted == wanted && app.flags and ApplicationInfo.FLAG_SYSTEM == 0
            }
        return if (suspicious) listOf("a11y_content_gestures") else emptyList()
    }

    private companion object {
        const val FRIDA_PORT = 27042
        const val CONNECT_TIMEOUT_MS = 100

        val SU_PATHS = listOf(
            "/system/bin/su", "/system/xbin/su", "/sbin/su", "/system/su", "/system/sd/xbin/su",
            "/system/bin/failsafe/su", "/data/local/xbin/su", "/data/local/bin/su", "/data/local/su",
            "/su/bin/su", "/vendor/bin/su",
        )
        val MAGISK_PATHS = listOf(
            "/sbin/.magisk", "/data/adb/magisk", "/data/adb/modules", "/cache/.disable_magisk",
            "/dev/.magisk.unblock",
        )
        val SYSTEM_MOUNTS = setOf("/system", "/vendor", "/product", "/system_ext")
        val FRIDA_FILES = listOf("/data/local/tmp/frida-server", "/data/local/tmp/re.frida.server")
        val FRIDA_MAPS = listOf("frida", "linjector")
        val XPOSED_MAPS = listOf("xposedbridge", "lspd", "lsposed", "edxp", "libsandhook")
        val FRIDA_THREADS = setOf("gum-js-loop", "pool-frida")
        val EMULATOR_HARDWARE = setOf("goldfish", "ranchu", "vbox86")
        val QEMU_FILES = listOf("/dev/qemu_pipe", "/dev/socket/qemud", "/system/lib/libc_malloc_debug_qemu.so")
    }
}
