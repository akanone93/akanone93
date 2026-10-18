// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_express

import android.content.Context
import android.content.Intent
import android.graphics.Rect
import android.os.Build
import android.os.SystemClock
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.UiObject2
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/**
 * A scripted pass over the platform accessibility tree of Plux Express's
 * catalogue, product and cart pages (QA-011, A11Y-001, A11Y-004): the labels
 * a screen reader reads, in reading order, on controls of at least 48 dp.
 * The app is the one built with the flows' defines against the server
 * TestPluxExpressAgainstTheServer starts; Flutter reports a widget's label
 * as the content description, a native view its text. The second test runs
 * the catalogue with the app in Arabic, whose labels are the translators'
 * and so are checked for being there, not for their words.
 */
@RunWith(AndroidJUnit4::class)
class AccessibilityTreeTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val device = UiDevice.getInstance(instrumentation)
    private val context: Context = instrumentation.targetContext
    private val pkg = context.packageName

    /** The three pages' labels read in order, each control labelled and large enough. */
    @Test
    fun catalogueProductAndCartPagesReadInOrder() {
        setAppLocale("en")
        launch()
        // A row reads its name, then its price.
        waitFor("Loaded 10 products")
        assertReadsInOrder(listOf("Product 01", "Product 02"), prefix = true)
        assertInteractiveControlsAreLabelled()

        waitFor("Product 01", prefix = true).click()
        waitFor("In cart: 0")
        assertReadsInOrder(listOf("In cart: 0", "Add to cart"))
        assertInteractiveControlsAreLabelled()

        waitFor("Add to cart").click()
        waitFor("In cart: 1")
        waitFor("Cart (1)").click()
        // The summary's total reads its label, then the amount in words.
        waitFor("Total", prefix = true)
        assertReadsInOrder(listOf("Total", "Place order"), prefix = true)
        assertInteractiveControlsAreLabelled()
    }

    /** The catalogue in Arabic: as many controls as in English, each labelled. */
    @Test
    fun catalogueReadsInArabic() {
        assumeTrue("per-app locales need Android 13", Build.VERSION.SDK_INT >= 33)
        setAppLocale("en")
        launch()
        waitFor("Loaded 10 products")
        val inEnglish = interactiveNodes().size

        setAppLocale("ar")
        launch()
        waitForAny { interactiveNodes().isNotEmpty() && find("Loaded 10 products") == null }
        assertTrue(
            device.executeShellCommand("cmd locale get-app-locales $pkg").contains("ar"),
        )
        assertEquals(inEnglish, interactiveNodes().size)
        assertInteractiveControlsAreLabelled()
    }

    @After
    fun restoreTheLocale() {
        if (Build.VERSION.SDK_INT >= 33) setAppLocale("")
    }

    // The labels, or with prefix the labels' starts, appear on screen top to
    // bottom in the order given.
    private fun assertReadsInOrder(labels: List<String>, prefix: Boolean = false) {
        val tops = labels.map { waitFor(it, prefix).visibleBounds.top }
        assertEquals("the reading order of $labels", tops.sorted(), tops)
    }

    // Every control a person can operate is named and at least 48 dp square:
    // what a screen reader announces and what a finger can hit.
    private fun assertInteractiveControlsAreLabelled() {
        val minimum = (48 * context.resources.displayMetrics.density).toInt()
        val nodes = interactiveNodes()
        assertTrue("nothing interactive on screen", nodes.isNotEmpty())
        for (node in nodes) {
            val bounds: Rect = node.visibleBounds
            assertTrue("an unlabelled control at $bounds", label(node).isNotEmpty())
            assertTrue(
                "\"${label(node)}\" is ${bounds.width()} x ${bounds.height()} px, under 48 dp",
                bounds.width() >= minimum && bounds.height() >= minimum,
            )
        }
    }

    private fun interactiveNodes(): List<UiObject2> =
        device.findObjects(By.pkg(pkg)).filter {
            runCatching { it.isClickable || it.className == "android.widget.EditText" }.getOrDefault(false)
        }

    private fun label(node: UiObject2): String =
        runCatching { node.contentDescription ?: node.text ?: "" }.getOrDefault("")

    // Starts the app anew; its settings are those it was built with.
    private fun launch() {
        val intent =
            context.packageManager.getLaunchIntentForPackage(pkg)!!
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
        context.startActivity(intent)
    }

    // The app's own language, as the system's per-app setting does (Android 13).
    private fun setAppLocale(tag: String) {
        if (Build.VERSION.SDK_INT < 33) return
        val locales = if (tag.isEmpty()) "" else " --locales $tag"
        device.executeShellCommand("cmd locale set-app-locales $pkg$locales")
    }

    private fun find(label: String, prefix: Boolean = false): UiObject2? {
        val selectors =
            if (prefix) listOf(By.descStartsWith(label), By.textStartsWith(label))
            else listOf(By.desc(label), By.text(label))
        return selectors.firstNotNullOfOrNull { device.findObject(it) }
    }

    // Waits up to a minute: the first page starts the engine and the runtime.
    private fun waitFor(label: String, prefix: Boolean = false): UiObject2 {
        val deadline = SystemClock.uptimeMillis() + 60_000
        while (true) {
            find(label, prefix)?.let { return it }
            if (SystemClock.uptimeMillis() > deadline) {
                throw AssertionError("nothing on screen reads \"$label\"")
            }
            SystemClock.sleep(200)
        }
    }

    private fun waitForAny(condition: () -> Boolean) {
        val deadline = SystemClock.uptimeMillis() + 60_000
        while (!condition()) {
            if (SystemClock.uptimeMillis() > deadline) throw AssertionError("the page never appeared")
            SystemClock.sleep(200)
        }
    }
}
