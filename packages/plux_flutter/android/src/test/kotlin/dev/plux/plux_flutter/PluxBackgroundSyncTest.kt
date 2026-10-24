// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_flutter

import androidx.work.NetworkType
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import java.util.concurrent.atomic.AtomicInteger
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** The rules of the Android scheduler and of the process-wide sync lock for background sync (SYN-014). */
class PluxBackgroundSyncTest {
    @Test
    fun theDefaultsAreTheSafestConstraintsAndSixHours() {
        val spec = BackgroundSyncSpec.fromArguments(emptyMap<String, Any>())
        assertEquals(360L, spec.intervalMinutes)
        assertFalse(spec.requiresUnmeteredNetwork)
        assertFalse(spec.requiresCharging)
        assertTrue(spec.requiresBatteryNotLow)
    }

    @Test
    fun anIntervalBelowFifteenMinutesCountsAsFifteen() {
        for (minutes in listOf(0, 1, 14, -5)) {
            val spec = BackgroundSyncSpec.fromArguments(mapOf("intervalMinutes" to minutes))
            assertEquals(15L, spec.intervalMinutes)
        }
        assertEquals(15L, BackgroundSyncSpec.fromArguments(mapOf("intervalMinutes" to 15)).intervalMinutes)
        assertEquals(720L, BackgroundSyncSpec.fromArguments(mapOf("intervalMinutes" to 720L)).intervalMinutes)
    }

    @Test
    fun theConstraintsComeFromTheArguments() {
        val spec = BackgroundSyncSpec.fromArguments(
            mapOf(
                "intervalMinutes" to 60,
                "requiresUnmeteredNetwork" to true,
                "requiresCharging" to true,
                "requiresBatteryNotLow" to false,
            ),
        )
        assertEquals(60L, spec.intervalMinutes)
        assertTrue(spec.requiresUnmeteredNetwork)
        assertTrue(spec.requiresCharging)
        assertFalse(spec.requiresBatteryNotLow)
    }

    @Test
    fun theRequestRepeatsAtTheIntervalWithTheConstraints() {
        val request = PluxBackgroundSync.buildRequest(
            BackgroundSyncSpec(
                intervalMinutes = 90,
                requiresUnmeteredNetwork = true,
                requiresCharging = true,
                requiresBatteryNotLow = true,
            ),
        )
        val work = request.workSpec
        assertEquals(TimeUnit.MINUTES.toMillis(90), work.intervalDuration)
        assertEquals(PluxBackgroundSyncWorker::class.java.name, work.workerClassName)
        assertEquals(NetworkType.UNMETERED, work.constraints.requiredNetworkType)
        assertTrue(work.constraints.requiresCharging())
        assertTrue(work.constraints.requiresBatteryNotLow())
    }

    @Test
    fun theRequestNeedsAnyNetworkWhenUnmeteredIsNotAsked() {
        val request = PluxBackgroundSync.buildRequest(BackgroundSyncSpec.fromArguments(emptyMap<String, Any>()))
        assertEquals(NetworkType.CONNECTED, request.workSpec.constraints.requiredNetworkType)
        assertFalse(request.workSpec.constraints.requiresCharging())
    }

    @Test
    fun theUniqueWorkNameIsOnePerApp() {
        assertEquals("dev.plux.backgroundSync", PluxBackgroundSync.UNIQUE_WORK_NAME)
    }

    @Test
    fun theSyncLockIsHeldByOneRunAtATime() {
        PluxSyncLock.release()
        try {
            assertTrue(PluxSyncLock.tryAcquire())
            assertFalse(PluxSyncLock.tryAcquire())
            PluxSyncLock.release()
            assertTrue(PluxSyncLock.tryAcquire())
        } finally {
            PluxSyncLock.release()
        }
    }

    @Test
    fun releasingAFreeSyncLockChangesNothing() {
        PluxSyncLock.release()
        PluxSyncLock.release()
        try {
            assertTrue(PluxSyncLock.tryAcquire())
        } finally {
            PluxSyncLock.release()
        }
    }

    @Test
    fun aSingleThreadWinsTheSyncLockWhenManyRaceForIt() {
        PluxSyncLock.release()
        val winners = AtomicInteger(0)
        val start = CountDownLatch(1)
        val threads = (1..16).map {
            Thread {
                start.await()
                if (PluxSyncLock.tryAcquire()) winners.incrementAndGet()
            }.also { it.start() }
        }
        start.countDown()
        threads.forEach { it.join() }
        try {
            assertEquals(1, winners.get())
        } finally {
            PluxSyncLock.release()
        }
    }
}
