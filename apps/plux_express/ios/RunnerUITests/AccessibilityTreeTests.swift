// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import XCTest

/// A scripted pass over the platform accessibility tree of Plux Express's
/// catalogue, product and cart pages (QA-011, A11Y-001, A11Y-004): the iOS
/// twin of the Android AccessibilityTreeTest. The labels VoiceOver reads, in
/// reading order, on controls of at least 44 pt. The app is the one built
/// with the flows' defines and lib/main.dart; the Go driver
/// (accessibilityTreePassIOS) builds it and runs these tests on the simulator.
/// Flutter reports a widget's label as the accessibility label. The second
/// test runs the catalogue with the app in Arabic, whose labels are the
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
  func testCatalogueProductAndCartPagesReadInOrder() {
    let app = launch()
    // A card reads its badge, name and price as one.
    waitFor(app, "Loaded 10 products", exact: true)
    assertReadsInOrder(app, ["Loaded 10 products", "Product 01"], exact: false)
    assertInteractiveControlsAreLabelled(app)

    tap(app, "Product 01", opens: "In cart: 0", exactly: true)
    assertReadsInOrder(app, ["Add to cart", "In cart: 0"])
    assertInteractiveControlsAreLabelled(app)

    tap(app, "Add to cart", exact: true)
    waitFor(app, "In cart: 1", exact: true)
    // The summary's total reads its label, then the amount in words.
    tap(app, "Cart (1)", exact: true, opens: "Total", exactly: false)
    assertReadsInOrder(app, ["Total", "Place order"], exact: false)
    assertInteractiveControlsAreLabelled(app)
  }

  /// The catalogue in Arabic: as many controls as in English, each labelled.
  func testCatalogueReadsInArabic() {
    let english = launch()
    waitFor(english, "Loaded 10 products", exact: true)
    let inEnglish = interactiveElements(english).count
    english.terminate()

    let arabic = launch(arabic: true)
    waitForAny(arabic) {
      !self.interactiveElements(arabic).isEmpty && !self.element(arabic, "Loaded 10 products", exact: true).exists
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
      XCTFail("nothing on screen reads \"\(label)\"")
    }
    return found
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
