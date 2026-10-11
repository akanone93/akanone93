// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_security

import android.os.Build
import android.view.MotionEvent
import android.view.Window

/**
 * Notices touches that arrived while another window covered the app
 * (tapjacking, SEC-070). Android marks such a touch with
 * `FLAG_WINDOW_IS_OBSCURED`, or, from Android 10, `FLAG_WINDOW_IS_PARTIALLY_OBSCURED`;
 * the watch wraps the activity window's callback, looks at the flag of every
 * touch and passes the touch on unchanged. The flag is kept until [take]
 * reads it, so a scan sees a touch that happened since the one before.
 */
internal class ObscuredTouchWatch {
    private var seen = emptySet<String>()
    private var window: Window? = null
    private var wrapper: Window.Callback? = null
    private var original: Window.Callback? = null

    /** Starts watching [target]; a window already watched is released first. */
    fun attach(target: Window) {
        detach()
        val inner = target.callback ?: return
        val watch = object : Window.Callback by inner {
            override fun dispatchTouchEvent(event: MotionEvent): Boolean {
                record(event.flags)
                return inner.dispatchTouchEvent(event)
            }
        }
        window = target
        original = inner
        wrapper = watch
        target.callback = watch
    }

    /** Stops watching; the window's callback is restored if nothing wrapped it since. */
    fun detach() {
        val target = window
        if (target != null && target.callback === wrapper) target.callback = original
        window = null
        wrapper = null
        original = null
    }

    /** The codes of the obscured touches seen since the last call. */
    @Synchronized
    fun take(): Set<String> {
        val s = seen
        seen = emptySet()
        return s
    }

    @Synchronized
    private fun record(flags: Int) {
        if (flags and MotionEvent.FLAG_WINDOW_IS_OBSCURED != 0) seen = seen + "obscured_touch"
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q &&
            flags and MotionEvent.FLAG_WINDOW_IS_PARTIALLY_OBSCURED != 0
        ) {
            seen = seen + "partially_obscured_touch"
        }
    }
}
