// SPDX-FileCopyrightText: 2026 Plux contributors
// SPDX-License-Identifier: Apache-2.0

import XCTest

/// The add-to-app flows (HST-033) on a simulator, against the server
/// TestAddToAppAgainstTheServer starts. They find what is on screen by its
/// accessibility label, which Flutter sets from a widget's semantics. The
/// driver runs each test in an xcodebuild run of its own, passing the
/// runtime's settings as TEST_RUNNER_PLUX_* variables, which reach this
/// runner without the prefix and the app through its launch environment.
final class HostFlowsTests: XCTestCase {
  /// Set while waitFor waits; see record(_:).
  private var waiting = false

  override func setUp() {
    continueAfterFailure = false
  }

  // A debug build runs Dart on the app's main thread, which Flutter shares
  // with UIKit on iOS, so on a slow simulator the first page's frame can
  // hold that thread past XCTest's 30-second limit for a query (39 s after
  // "Open welcome" in CI run 38278383954), and XCTest fails the query. While
  // waitFor waits, that failure only means the label is not on screen yet:
  // waitFor queries again, and still fails once its minute is over.
  override func record(_ issue: XCTIssue) {
    if waiting, Self.mainThreadBusy(issue) {
      print("HostFlowsTests: the app's main thread was busy; waiting on")
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

  /// A native screen opens a Plux page, rendered from the baseline with
  /// the server out of reach.
  func testRendersOfflineFromTheBaseline() {
    let app = launch()
    tap(app, "Open welcome")
    waitFor(app, "Welcome to Plux", exact: true)
    tap(app, "Back", exact: true)
    waitFor(app, "Open welcome")
  }

  /// Sync applies the newer release once no Plux page is open, and a
  /// plugin page shown below a native header opens the host's native
  /// screen.
  func testAppliesUpdatesAndOpensNativeScreens() {
    let app = launch()
    let deadline = Date().addingTimeInterval(60)
    while true {
      tap(app, "Open welcome")
      waitFor(app, "Welcome to Plux")
      if element(app, "Welcome to Plux, again", exact: true).exists { break }
      XCTAssertLessThan(Date(), deadline, "the update never applied")
      tap(app, "Back", exact: true)
      waitFor(app, "Open welcome")
      Thread.sleep(forTimeInterval: 0.5)
    }
    tap(app, "Back", exact: true)
    waitFor(app, "Open welcome")

    tap(app, "Open host link")
    waitFor(app, "Native header", exact: true)
    tap(app, "Open host settings")
    waitFor(app, "Host settings", exact: true)
    tap(app, "Close", exact: true)
    waitFor(app, "Open host settings")
    tap(app, "Back", exact: true)
    waitFor(app, "Open welcome")
  }

  // Launches the app with the runtime's settings.
  private func launch() -> XCUIApplication {
    let app = XCUIApplication()
    let env = ProcessInfo.processInfo.environment
    for name in ["PLUX_ENDPOINT", "PLUX_APP_ID", "PLUX_ENVIRONMENT", "PLUX_HOST_BUILD", "PLUX_ROOT_KEYS"] {
      if let value = env[name] { app.launchEnvironment[name] = value }
    }
    app.launch()
    waitFor(app, "Open welcome")
    return app
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
      XCTFail("nothing on screen reads \"\(label)\"; it reads: \(labels(app))")
    }
    return found
  }

  // Every label on screen, for a failure's message.
  private func labels(_ app: XCUIApplication) -> String {
    guard let root = try? app.snapshot() else { return "(no snapshot)" }
    var labels: [String] = []
    var pending = [root]
    while let next = pending.popLast() {
      if !next.label.isEmpty { labels.append(next.label) }
      pending.append(contentsOf: next.children.reversed())
    }
    return labels.isEmpty ? "nothing" : labels.map { "\"\($0)\"" }.joined(separator: ", ")
  }

  private func tap(_ app: XCUIApplication, _ label: String, exact: Bool = false) {
    waitFor(app, label, exact: exact).tap()
  }
}
