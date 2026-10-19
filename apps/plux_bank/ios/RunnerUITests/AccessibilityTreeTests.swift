// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import XCTest

/// A scripted pass over the platform accessibility tree of Plux Bank's login,
/// dashboard and transfer pages (QA-011, A11Y-001, A11Y-004): the iOS twin of
/// the Android AccessibilityTreeTest. The labels VoiceOver reads, in reading
/// order, on controls of at least 44 pt. The app is the one built with the
/// flows' defines and integration_test/a11y_main.dart; the Go driver
/// (accessibilityTreePassIOS) builds it and runs these tests on the simulator.
/// Flutter reports a widget's label as the accessibility label. The second
/// test runs the login page with the app in Arabic, whose labels are the
/// translators' and so are checked for being there, not for their words.
final class AccessibilityTreeTests: XCTestCase {
  /// Set while waitFor waits; see record(_:).
  private var waiting = false

  /// The smallest side of a control a finger can hit, in points.
  private let minimumSide: CGFloat = 44

  override func setUp() {
    continueAfterFailure = false
  }

  // A debug build runs Dart on the app's main thread, which Flutter shares
  // with UIKit on iOS, so on a slow simulator a page's frame can hold that
  // thread past XCTest's query limit. While waitFor waits, that failure only
  // means the label is not on screen yet: waitFor queries again, and still
  // fails once its minute is over.
  override func record(_ issue: XCTIssue) {
    if waiting, Self.mainThreadBusy(issue) {
      print("AccessibilityTreeTests: the app's main thread was busy; waiting on")
      return
    }
    super.record(issue)
  }

  private static func mainThreadBusy(_ issue: XCTIssue) -> Bool {
    if let error = issue.associatedError {
      let e = error as NSError
      if e.domain == "com.apple.dt.xctest.automation-support.error", e.code == 6 { return true }
    }
    return issue.compactDescription.contains("main thread busy")
  }

  /// The three pages' labels read in order, each control labelled and large enough.
  func testLoginDashboardAndTransferPagesReadInOrder() {
    let app = launch()
    // The code field and the button are below the fold on a phone: the
    // order is read from what is on screen, the rest scrolled to.
    waitFor(app, "Welcome back", exact: true)
    assertReadsInOrder(app, ["Welcome back", "Username"], exact: false)
    assertInteractiveControlsAreLabelled(app)

    // Return closes the keyboard, which would cover the PIN pad and the
    // button: a touch outside a field does not, on a phone.
    type(waitFor(app, "Username"), "demo\n")
    enterPin(app, "246810")
    // The code's last digit completes it, and the field ends its autofill
    // context, which on iOS takes the keyboard away: there is no field to
    // send Return to (run 39153556766; pinned in plux_flutter's
    // test/secure/secure_widgets_test.dart).
    type(reveal(app, "One-time code", exact: false), "123456")
    reveal(app, "Sign in")
    tap(app, "Sign in", exact: true, opens: "New transfer", exactly: true)

    assertReadsInOrder(app, ["New transfer"])
    assertInteractiveControlsAreLabelled(app)

    // The balance is what the flows' transfers left, so only its start is known.
    tap(app, "New transfer", exact: true, opens: "Available: ", exactly: false)
    assertReadsInOrder(app, ["Available: ", "Recipient IBAN", "Amount (EUR)"], exact: false)
    assertInteractiveControlsAreLabelled(app)
  }

  /// The login page in Arabic: as many controls as in English, each labelled.
  func testLoginReadsInArabic() {
    let english = launch()
    waitFor(english, "Welcome back", exact: true)
    let inEnglish = interactiveElements(english).count
    english.terminate()

    let arabic = launch(arabic: true)
    waitForAny(arabic) {
      !self.interactiveElements(arabic).isEmpty && !self.element(arabic, "Welcome back", exact: true).exists
    }
    XCTAssertEqual(interactiveElements(arabic).count, inEnglish)
    assertInteractiveControlsAreLabelled(arabic)
  }

  // Starts the app anew, in the language the test asks for; its settings are
  // those it was built with.
  private func launch(arabic: Bool = false) -> XCUIApplication {
    let app = XCUIApplication()
    if arabic {
      app.launchArguments = ["-AppleLanguages", "(ar)"]
    }
    app.launch()
    return app
  }

  // The labels, or with exact false labels containing them, appear on screen
  // top to bottom in the order given.
  private func assertReadsInOrder(_ app: XCUIApplication, _ labels: [String], exact: Bool = true) {
    let tops = labels.map { waitFor(app, $0, exact: exact).frame.minY }
    XCTAssertEqual(tops, tops.sorted(), "the reading order of \(labels)")
  }

  // Every control a person can operate is named and at least 44 pt square:
  // what VoiceOver announces and what a finger can hit.
  private func assertInteractiveControlsAreLabelled(_ app: XCUIApplication) {
    let nodes = interactiveElements(app)
    XCTAssertFalse(nodes.isEmpty, "nothing interactive on screen")
    for node in nodes {
      let frame = node.frame
      XCTAssertFalse(node.label.isEmpty, "an unlabelled control at \(frame)")
      XCTAssertGreaterThanOrEqual(
        frame.width, minimumSide, "\"\(node.label)\" is \(frame.width) x \(frame.height) pt, under 44 pt")
      XCTAssertGreaterThanOrEqual(
        frame.height, minimumSide, "\"\(node.label)\" is \(frame.width) x \(frame.height) pt, under 44 pt")
    }
  }

  private func interactiveElements(_ app: XCUIApplication) -> [XCUIElement] {
    let kinds: [XCUIElement.ElementType] = [.button, .link, .textField, .secureTextField, .switch, .slider]
    return app.descendants(matching: .any).allElementsBoundByIndex.filter { kinds.contains($0.elementType) }
  }

  private func element(_ app: XCUIApplication, _ label: String, exact: Bool) -> XCUIElement {
    let predicate =
      exact ? NSPredicate(format: "label == %@", label) : NSPredicate(format: "label CONTAINS %@", label)
    return app.descendants(matching: .any).matching(predicate).firstMatch
  }

  // Waits up to a minute: the first page starts the engine and the runtime.
  @discardableResult
  private func waitFor(_ app: XCUIApplication, _ label: String, exact: Bool = false) -> XCUIElement {
    let found = element(app, label, exact: exact)
    let deadline = Date().addingTimeInterval(60)
    var shown = false
    waiting = true
    while !shown, Date() < deadline {
      shown = found.waitForExistence(timeout: deadline.timeIntervalSinceNow)
    }
    waiting = false
    if !shown {
      let labels = app.descendants(matching: .any).allElementsBoundByIndex.map(\.label).filter { !$0.isEmpty }
      XCTFail("nothing on screen reads \"\(label)\"; it reads \(labels)")
    }
    return found
  }

  // A field's value, as VoiceOver reads it.
  private func text(of field: XCUIElement) -> String { (field.value as? String) ?? "" }

  // Focuses the field and types into it one key at a time, reading the field
  // after each key: XCTest's first key into a Flutter field arrives twice
  // (run 39146472448 typed "ddemo"), so a key that added more than itself is
  // taken back. A final newline is Return, sent once the text is right.
  private func type(_ field: XCUIElement, _ text: String) {
    let expected = text.trimmingCharacters(in: .newlines)
    for _ in 0..<3 {
      field.tap()
      // Flutter attaches its text input a frame after the tap.
      let deadline = Date().addingTimeInterval(5)
      while (field.value(forKey: "hasKeyboardFocus") as? Bool) != true, Date() < deadline {
        Thread.sleep(forTimeInterval: 0.2)
      }
      let current = self.text(of: field)
      if !current.isEmpty {
        field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: current.count))
      }
      var typed = ""
      for key in expected {
        typed.append(key)
        field.typeText(String(key))
        let extra = self.text(of: field).count - typed.count
        if extra > 0 {
          field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: extra))
        }
      }
      if self.text(of: field) == expected {
        if text.hasSuffix("\n") { field.typeText("\n") }
        return
      }
    }
    XCTFail("the field holds \"\(self.text(of: field))\", not \"\(expected)\"")
  }

  // Taps the PIN's digits one at a time and checks the pad counted each of
  // them once. Its value reads "N of M digits entered" after the Nth digit;
  // the last digit completes the PIN and the pad clears itself (SEC-090,
  // pinned in plux_flutter's test/secure/secure_widgets_test.dart), so it
  // then reads "0 of M", never "M of M". A digit the pad missed or counted
  // twice fails here, named by its position.
  private func enterPin(_ app: XCUIApplication, _ pin: String) {
    let pad = app.descendants(matching: .any)
      .matching(NSPredicate(format: "value CONTAINS %@", "digits entered")).firstMatch
    for (index, digit) in pin.enumerated() {
      reveal(app, String(digit)).tap()
      let expected = "\((index + 1) % pin.count) of \(pin.count) digits entered"
      if !waitForValue(pad, expected) {
        XCTFail(
          "after digit \(index + 1) of \(pin.count) the PIN pad reads \"\(text(of: pad))\", not \"\(expected)\"")
        return
      }
    }
  }

  // Waits up to ten seconds for the element's value: Flutter sends the
  // accessibility tree a frame after the tap changed it.
  private func waitForValue(_ element: XCUIElement, _ value: String) -> Bool {
    let deadline = Date().addingTimeInterval(10)
    waiting = true
    defer { waiting = false }
    while !(element.exists && text(of: element) == value) {
      if Date() >= deadline { return false }
      Thread.sleep(forTimeInterval: 0.2)
    }
    return true
  }

  // The control with the label, scrolled to when the keyboard hides it: a
  // control below the focused field is off screen, and Flutter leaves it out
  // of the tree.
  @discardableResult
  private func reveal(_ app: XCUIApplication, _ label: String, exact: Bool = true) -> XCUIElement {
    let found = element(app, label, exact: exact)
    for _ in 0..<5 where !(found.waitForExistence(timeout: 1) && found.isHittable) {
      app.scrollViews.firstMatch.swipeUp()
    }
    return waitFor(app, label, exact: exact)
  }

  // Waits up to a minute for a condition on the page.
  private func waitForAny(_ app: XCUIApplication, _ condition: () -> Bool) {
    let deadline = Date().addingTimeInterval(60)
    while !condition() {
      if Date() >= deadline {
        XCTFail("the page never appeared")
        return
      }
      Thread.sleep(forTimeInterval: 0.2)
    }
  }

  private func tap(_ app: XCUIApplication, _ label: String, exact: Bool = false) {
    waitFor(app, label, exact: exact).tap()
  }

  // Taps the control and waits for what it opens, tapping it again up to
  // twice more while it is still there and nothing opened: a page that has
  // just been pushed may still be moving when the first tap lands.
  private func tap(_ app: XCUIApplication, _ label: String, exact: Bool = false, opens: String, exactly: Bool) {
    let control = waitFor(app, label, exact: exact)
    for _ in 0..<3 where control.exists {
      control.tap()
      if element(app, opens, exact: exactly).waitForExistence(timeout: 10) { return }
    }
    waitFor(app, opens, exact: exactly)
  }
}
