// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

package dev.plux.plux_bank

import android.content.Context
import android.content.Intent
import android.graphics.Rect
import android.os.Build
import android.os.SystemClock
import android.view.accessibility.AccessibilityNodeInfo
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.UiDevice
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test

/**
 * A scripted pass over the platform accessibility tree of Plux Bank's login,
 * dashboard and transfer pages (QA-011, A11Y-001, A11Y-004): the labels a
 * screen reader reads, in reading order, on controls of at least 48 dp. The
 * app is the one built with the flows' defines against the server
 * TestPluxBankAgainstTheServer starts (integration_test/a11y_main.dart);
 * Flutter reports a widget's label as the content description, a text
 * field's as its hint, a native view's as its text. The second test runs the login page with the app in
 * Arabic, whose labels are the translators' and so are checked for being
 * there, not for their words.
 */
class AccessibilityTreeTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val device = UiDevice.getInstance(instrumentation)
    private val context: Context = instrumentation.targetContext
    private val pkg = context.packageName

    /** The three pages' labels read in order, each control labelled and large enough. */
    @Test
    fun loginDashboardAndTransferPagesReadInOrder() {
        // Flutter gives Android a text field's label from Android 9 (API 28);
        // before it the login's fields have no name to find them by.
        assumeTrue("text fields are named from Android 9", Build.VERSION.SDK_INT >= 28)
        setAppLocale("en")
        launch()
        // The code field and the button are below the fold on a phone: the
        // order is read from what is on screen, the rest scrolled to.
        waitFor("Welcome back")
        assertReadsInOrder(listOf("Welcome back", "Username"), partial = true)
        assertInteractiveControlsAreLabelled()

        type(waitFor("Username", partial = true), "demo")
        for (digit in "246810") click(reveal(digit.toString()))
        type(reveal("One-time code", partial = true), "123456")
        click(reveal("Sign in"))

        waitFor("New transfer")
        assertReadsInOrder(listOf("New transfer"))
        assertInteractiveControlsAreLabelled()

        // The balance is what the flows' transfers left, so only its start is known.
        click(waitFor("New transfer"))
        waitFor("Available: ", partial = true)
        assertReadsInOrder(listOf("Available: ", "Recipient IBAN", "Amount (EUR)"), partial = true)
        assertInteractiveControlsAreLabelled()
    }

    /** The login page in Arabic: mirrored, as many controls as in English, each labelled. */
    @Test
    fun loginReadsInArabic() {
        assumeTrue("per-app locales need Android 13", Build.VERSION.SDK_INT >= 33)
        setAppLocale("en")
        launch()
        waitFor("Welcome back")
        val inEnglish = interactiveNodes().size

        setAppLocale("ar")
        launch()
        waitForAny { interactiveNodes().isNotEmpty() && find("Welcome back") == null }
        assertTrue(
            device.executeShellCommand("cmd locale get-app-locales $pkg").contains("ar"),
        )
        // The tree is briefly empty while the app starts again in Arabic.
        runCatching { waitForAny { interactiveNodes().size == inEnglish } }
        assertEquals(inEnglish, interactiveNodes().size)
        assertInteractiveControlsAreLabelled()
    }

    @After
    fun restoreTheLocale() {
        if (Build.VERSION.SDK_INT >= 33) setAppLocale("")
    }

    // The labels, or with partial labels containing them, appear on screen
    // top to bottom in the order given.
    private fun assertReadsInOrder(labels: List<String>, partial: Boolean = false) {
        val tops = labels.map { waitFor(it, partial).bounds.top }
        assertEquals("the reading order of $labels", tops.sorted(), tops)
    }

    // Every control a person can operate is named and at least 48 dp square:
    // what a screen reader announces and what a finger can hit.
    private fun assertInteractiveControlsAreLabelled() {
        val minimum = (48 * context.resources.displayMetrics.density).toInt()
        val nodes = interactiveNodes()
        assertTrue("nothing interactive on screen", nodes.isNotEmpty())
        for (node in nodes) {
            val bounds = node.bounds
            assertTrue("an unlabelled control at $bounds", node.label.isNotEmpty())
            assertTrue(
                "\"${node.label}\" is ${bounds.width()} x ${bounds.height()} px, under 48 dp",
                bounds.width() >= minimum && bounds.height() >= minimum,
            )
        }
    }

    /** A node of the app's accessibility tree, as a screen reader meets it. */
    private class Node(val info: AccessibilityNodeInfo) {
        val bounds: Rect get() = Rect().also { info.getBoundsInScreen(it) }

        // What a screen reader reads: the description, the text or, for a
        // text field, the hint Flutter puts its label in.
        val labels: List<String>
            get() =
                listOfNotNull(info.contentDescription, info.text, if (Build.VERSION.SDK_INT >= 26) info.hintText else null)
                    .map { it.toString() }
                    .filter { it.isNotEmpty() }

        val label: String get() = labels.firstOrNull() ?: ""

        val interactive: Boolean get() = info.isClickable || info.className?.toString() == "android.widget.EditText"

        // A merged node reads its parts on separate lines, so partial
        // matches a part.
        fun reads(label: String, partial: Boolean): Boolean =
            labels.any { if (partial) it.contains(label) else it == label }
    }

    // The app's nodes on screen, in the tree's order.
    private fun nodes(): List<Node> {
        val found = mutableListOf<Node>()
        fun walk(info: AccessibilityNodeInfo?) {
            if (info == null) return
            if (info.packageName == pkg) found += Node(info)
            for (i in 0 until info.childCount) walk(info.getChild(i))
        }
        walk(instrumentation.uiAutomation.rootInActiveWindow)
        return found
    }

    private fun interactiveNodes(): List<Node> = nodes().filter { it.interactive }

    private fun click(node: Node) {
        val bounds = node.bounds
        device.click(bounds.centerX(), bounds.centerY())
    }

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

    private fun find(label: String, partial: Boolean = false): Node? = nodes().firstOrNull { it.reads(label, partial) }

    // Waits up to a minute: the first page starts the engine and the runtime.
    private fun waitFor(label: String, partial: Boolean = false): Node {
        val deadline = SystemClock.uptimeMillis() + 60_000
        while (true) {
            find(label, partial)?.let { return it }
            if (SystemClock.uptimeMillis() > deadline) {
                throw AssertionError("nothing on screen reads \"$label\"")
            }
            SystemClock.sleep(200)
        }
    }

    // Focuses the field and types into it, as the keyboard does.
    private fun type(field: Node, text: String) {
        click(field)
        SystemClock.sleep(500)
        device.executeShellCommand("input text $text")
    }

    // The control with the label, scrolled to when the keyboard hides it: a
    // control below the focused field is off screen, and Flutter leaves it
    // out of the tree.
    private fun reveal(label: String, partial: Boolean = false): Node {
        repeat(5) {
            find(label, partial)?.let { return it }
            SystemClock.sleep(1_000)
            nodes().firstOrNull { it.info.isScrollable }?.info?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_FORWARD)
        }
        return waitFor(label, partial)
    }

    private fun waitForAny(condition: () -> Boolean) {
        val deadline = SystemClock.uptimeMillis() + 60_000
        while (!condition()) {
            if (SystemClock.uptimeMillis() > deadline) throw AssertionError("the page never appeared")
            SystemClock.sleep(200)
        }
    }
}
