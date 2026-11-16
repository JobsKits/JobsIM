//
//  IMSelfStateUITests.swift
//  JobsIMUITests
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import XCTest

@MainActor
final class IMSelfStateUITests: XCTestCase {

    private func tap(_ element: XCUIElement, app: XCUIApplication, scroll: Bool = false) {
        for _ in 0..<16 {
            if element.exists && element.isHittable && element.isEnabled {
                element.tap()
                return
            }
            if scroll, app.tables.firstMatch.exists {
                app.tables.firstMatch.swipeUp()
            }
            Thread.sleep(forTimeInterval: 0.25)
        }
        XCTFail("Enabled native control unavailable: " + element.identifier)
    }

    private func state(_ key: String, app: XCUIApplication) -> XCUIElement {
        return app.tables.cells["self." + key]
    }

    private func capture(_ name: String, app: XCUIApplication) {
        let capture = XCTAttachment(screenshot: app.screenshot())
        capture.name = name
        capture.lifetime = .keepAlways
        add(capture)
    }

    private func replace(_ text: String, app: XCUIApplication) {
        let field = app.textFields["Write a message…"]
        XCTAssertTrue(field.waitForExistence(timeout: 15))
        tap(field, app: app)
        if let current = field.value as? String, current != "Write a message…" {
            field.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: current.count))
        }
        field.typeText(text)
        if app.keyboards.buttons["Done"].exists {
            app.keyboards.buttons["Done"].tap()
        }
    }

    func testNativeSelfStateViewsCloudDraftAndExplicitAdoption() {
        continueAfterFailure = false
        let run = ProcessInfo.processInfo.environment["JOBS_IM_SELF_STATE_RUN"] ?? ""
        XCTAssertNotNil(UUID(uuidString: run))
        let app = XCUIApplication()
        app.launchArguments = ["--ui-test-profile", "s2-" + run, "--language", "en", "--theme", "light"]
        app.launchEnvironment["JOBS_IM_SYNC_INJECTED_TESTS"] = "0"
        app.launch()
        defer {
            capture("99-native-self-final", app: app)
            // The following native control phase validates real server values, logs out and checks original AT401.
            app.terminate()
        }
        let conversation = app.tables.cells.containing(.staticText, identifier: "Native S2 controls").firstMatch
        XCTAssertTrue(conversation.waitForExistence(timeout: 30))
        tap(conversation, app: app)
        tap(app.buttons["self.open"], app: app)
        tap(state("markManualUnread", app: app), app: app, scroll: true)
        XCTAssertTrue(state("clearManualUnread", app: app).waitForExistence(timeout: 15))
        tap(state("pinChat", app: app), app: app, scroll: true)
        XCTAssertTrue(state("unpinChat", app: app).waitForExistence(timeout: 15))
        tap(state("muteChat", app: app), app: app, scroll: true)
        XCTAssertTrue(state("unmuteChat", app: app).waitForExistence(timeout: 15))
        capture("01-native-self-state", app: app)
        tap(state("archiveChat", app: app), app: app, scroll: true)
        XCTAssertTrue(state("unarchiveChat", app: app).waitForExistence(timeout: 15))
        tap(state("hideChat", app: app), app: app, scroll: true)
        XCTAssertTrue(state("showChat", app: app).waitForExistence(timeout: 15))
        tap(app.navigationBars.buttons.firstMatch, app: app)
        tap(app.navigationBars.buttons.firstMatch, app: app)
        tap(app.buttons["self.recoveryLists"], app: app)
        tap(app.buttons["Show hidden conversations"], app: app)
        XCTAssertTrue(conversation.waitForExistence(timeout: 15))
        tap(conversation, app: app)
        tap(app.buttons["self.open"], app: app)
        tap(state("showChat", app: app), app: app, scroll: true)
        XCTAssertTrue(state("hideChat", app: app).waitForExistence(timeout: 15))
        tap(state("unarchiveChat", app: app), app: app, scroll: true)
        XCTAssertTrue(state("archiveChat", app: app).waitForExistence(timeout: 15))
        tap(app.navigationBars.buttons.firstMatch, app: app)
        replace("Independent iOS cloud draft", app: app)
        tap(app.buttons["self.open"], app: app)
        tap(state("saveCloudDraft", app: app), app: app, scroll: true)
        XCTAssertTrue(app.staticTexts["Independent iOS cloud draft"].waitForExistence(timeout: 15))
        capture("02-native-cloud-draft", app: app)
        tap(app.navigationBars.buttons.firstMatch, app: app)
        replace("New local input until explicit adoption", app: app)
        tap(app.buttons["self.open"], app: app)
        tap(state("loadCloudDraft", app: app), app: app, scroll: true)
        tap(app.alerts.buttons["Continue"], app: app)
        let editor = app.textFields["Write a message…"]
        let loaded = NSPredicate(format: "value == %@", "Independent iOS cloud draft")
        expectation(for: loaded, evaluatedWith: editor)
        waitForExpectations(timeout: 15)
        capture("03-native-explicit-adoption", app: app)
        tap(app.tabBars.buttons["Settings"], app: app)
        let font = app.tables.cells.containing(.staticText, identifier: "Text size").firstMatch
        tap(font, app: app, scroll: true)
        tap(app.buttons["Large"], app: app)
        capture("04-native-font", app: app)
    }
}
