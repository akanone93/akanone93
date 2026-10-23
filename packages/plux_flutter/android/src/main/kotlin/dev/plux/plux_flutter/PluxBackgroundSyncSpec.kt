// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_flutter

/**
 * What the app asked of background sync (SYN-014, ADR-0092), as the Dart side
 * sends it. It is plain Kotlin so that the rules below are unit tested
 * without the Android framework.
 */
data class BackgroundSyncSpec(
    val intervalMinutes: Long,
    val requiresUnmeteredNetwork: Boolean,
    val requiresCharging: Boolean,
    val requiresBatteryNotLow: Boolean,
) {
    companion object {
        /** The least interval WorkManager runs periodic work at, and the limit's minimum. */
        const val MIN_INTERVAL_MINUTES = 15L

        /** The default of the limit `sync.backgroundIntervalMinutes`. */
        const val DEFAULT_INTERVAL_MINUTES = 360L

        /**
         * Reads the arguments of `backgroundSyncSchedule`. A missing interval
         * counts as the default and a shorter one as the minimum; the
         * constraints default to the safest ones (any network, no charging
         * needed, battery not low).
         */
        fun fromArguments(args: Map<*, *>): BackgroundSyncSpec {
            val minutes = (args["intervalMinutes"] as? Number)?.toLong() ?: DEFAULT_INTERVAL_MINUTES
            return BackgroundSyncSpec(
                intervalMinutes = maxOf(minutes, MIN_INTERVAL_MINUTES),
                requiresUnmeteredNetwork = args["requiresUnmeteredNetwork"] as? Boolean ?: false,
                requiresCharging = args["requiresCharging"] as? Boolean ?: false,
                requiresBatteryNotLow = args["requiresBatteryNotLow"] as? Boolean ?: true,
            )
        }
    }
}
