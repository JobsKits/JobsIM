//
//  IMMessageListUITests.swift
//  JobsIMUITests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import XCTest

@MainActor
final class IMMessageListUITests: XCTestCase {

    func testSavedPaginationAndExactSearchSource() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = [
            "--language", "en", "--theme", "light", "--ui-test-profile", "saved-search",
        ]
        app.launch()
        defer {
            if app.tabBars.buttons["Settings"].exists {
                app.tabBars.buttons["Settings"].tap()
                let logout = app.tables.cells.containing(.staticText, identifier: "Sign out").firstMatch
                if logout.waitForExistence(timeout: 5) { logout.tap() }
            }
        }
        XCTAssertTrue(app.tabBars.buttons["Saved"].waitForExistence(timeout: 10))
        app.tabBars.buttons["Saved"].tap()
        let more = app.buttons["Load more"]
        XCTAssertTrue(more.waitForExistence(timeout: 10))
        XCTAssertTrue(more.isHittable)
        let source = app.staticTexts.matching(
            NSPredicate(format: "label BEGINSWITH %@", "Native selected source ")
        ).firstMatch
        XCTAssertFalse(source.exists)
        more.tap()
        XCTAssertTrue(more.waitForNonExistence(timeout: 10))
        for _ in 0..<12 {
            if source.exists && source.isHittable { break }
            app.tables.firstMatch.swipeUp()
        }
        XCTAssertTrue(source.waitForExistence(timeout: 5))
        XCTAssertTrue(source.isHittable)
        let selectedText = source.label
        source.tap()
        XCTAssertTrue(app.textFields["Write a message…"].waitForExistence(timeout: 10))
        let exact = app.staticTexts[selectedText]
        XCTAssertTrue(exact.waitForExistence(timeout: 10))
        XCTAssertTrue(exact.isHittable)
        let savedCapture = XCTAttachment(screenshot: app.screenshot())
        savedCapture.name = "native-paged-favorite-exact-source"
        savedCapture.lifetime = .keepAlways
        add(savedCapture)
        app.tabBars.buttons["Messages"].tap()
        let search = app.searchFields["Search"]
        XCTAssertTrue(search.waitForExistence(timeout: 10))
        search.tap()
        search.typeText("Native selected source")
        if app.keyboards.buttons["Search"].exists { app.keyboards.buttons["Search"].tap() }
        let result = app.tables.cells.containing(.staticText, identifier: selectedText).firstMatch
        XCTAssertTrue(result.waitForExistence(timeout: 10))
        XCTAssertTrue(result.isHittable)
        result.tap()
        XCTAssertTrue(app.textFields["Write a message…"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.staticTexts[selectedText].waitForExistence(timeout: 10))
        XCTAssertTrue(app.staticTexts[selectedText].isHittable)
        let searchCapture = XCTAttachment(screenshot: app.screenshot())
        searchCapture.name = "native-search-result-revalidated-source"
        searchCapture.lifetime = .keepAlways
        add(searchCapture)
    }

    func testStructuredMentionAndAnnouncementNativeControls() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--language", "en", "--theme", "light", "--ui-test-profile", "interactions"]
        app.launch()
        defer {
            if app.tabBars.buttons["Settings"].exists {
                app.tabBars.buttons["Settings"].tap()
                let logout = app.tables.cells.containing(.staticText, identifier: "Sign out").firstMatch
                if logout.waitForExistence(timeout: 5) { logout.tap() }
            }
        }
        let group = app.tables.cells.containing(.staticText, identifier: "Native interactions").firstMatch
        XCTAssertTrue(group.waitForExistence(timeout: 10))
        group.tap()
        XCTAssertTrue(app.buttons["Members"].waitForExistence(timeout: 10))
        app.buttons["Members"].tap()
        let announcement = app.tables.cells.containing(.staticText, identifier: "Group announcement")
            .firstMatch
        XCTAssertTrue(announcement.waitForExistence(timeout: 10))
        announcement.tap()
        XCTAssertTrue(app.alerts.staticTexts["Native announcement source"].waitForExistence(timeout: 10))
        let recovered = XCTAttachment(screenshot: app.screenshot())
        recovered.name = "native-announcement-original-operation-after-restart"
        recovered.lifetime = .keepAlways
        add(recovered)
        app.alerts.buttons["Done"].tap()
        app.navigationBars.buttons.firstMatch.tap()
        let input = app.textFields["Write a message…"]
        XCTAssertTrue(app.buttons["Mention"].waitForExistence(timeout: 10))
        app.buttons["Mention"].tap()
        app.buttons["Mention Native mention peer"].tap()
        input.tap()
        input.typeText("Native structured UI mention")
        app.buttons["Send"].tap()
        XCTAssertTrue(app.staticTexts["Native structured UI mention"].waitForExistence(timeout: 10))
        app.buttons["Mention"].tap()
        app.buttons["Mention all members"].tap()
        input.tap()
        input.typeText("Native all UI mention")
        app.buttons["Send"].tap()
        XCTAssertTrue(app.staticTexts["Native all UI mention"].waitForExistence(timeout: 10))
        let mentioned = XCTAttachment(screenshot: app.screenshot())
        mentioned.name = "native-structured-member-and-all-mentions"
        mentioned.lifetime = .keepAlways
        add(mentioned)
        viewSource(app).tap()
        app.buttons["Publish group announcement"].tap()
        XCTAssertTrue(app.alerts.buttons["Continue"].waitForExistence(timeout: 10))
        app.alerts.buttons["Continue"].tap()
        XCTAssertTrue(app.alerts.firstMatch.waitForNonExistence(timeout: 10))
        app.buttons["Members"].tap()
        let clear = app.tables.cells.containing(.staticText, identifier: "Clear group announcement")
            .firstMatch
        XCTAssertTrue(clear.waitForExistence(timeout: 10))
        clear.tap()
        app.alerts.buttons["Continue"].tap()
        XCTAssertTrue(clear.waitForNonExistence(timeout: 10))
        announcement.tap()
        XCTAssertTrue(app.alerts.staticTexts["No group announcement"].waitForExistence(timeout: 10))
        let cleared = XCTAttachment(screenshot: app.screenshot())
        cleared.name = "native-announcement-clear-confirmed"
        cleared.lifetime = .keepAlways
        add(cleared)
        app.alerts.buttons["Done"].tap()
    }

    private func viewSource(_ app: XCUIApplication) -> XCUIElement {
        if app.keyboards.firstMatch.exists {
            app.tables.firstMatch.swipeDown()
        }
        let source = app.staticTexts["Native announcement source"]
        for _ in 0..<6 {
            if source.exists && source.isHittable { break }
            app.tables.firstMatch.swipeDown()
        }
        XCTAssertTrue(source.waitForExistence(timeout: 5))
        XCTAssertTrue(source.isHittable)
        return source
    }

    func testNativeReferenceForwardAndOwnRecall() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--language", "en", "--theme", "light", "--ui-test-profile", "forwarding"]
        app.launch()
        defer {
            if app.tabBars.buttons["Settings"].exists {
                app.tabBars.buttons["Settings"].tap()
                let logout = app.tables.cells.containing(.staticText, identifier: "Sign out").firstMatch
                if logout.waitForExistence(timeout: 5) { logout.tap() }
            }
        }
        let source = app.tables.cells.containing(.staticText, identifier: "Native forward source").firstMatch
        XCTAssertTrue(source.waitForExistence(timeout: 10))
        source.tap()
        let text = app.staticTexts["Native forwarding selected source"]
        XCTAssertTrue(text.waitForExistence(timeout: 10))
        text.tap()
        XCTAssertTrue(app.buttons["Forward"].waitForExistence(timeout: 10))
        app.buttons["Forward"].tap()
        XCTAssertTrue(app.buttons["Native forward target"].waitForExistence(timeout: 10))
        app.buttons["Native forward target"].tap()
        XCTAssertTrue(app.sheets.firstMatch.waitForNonExistence(timeout: 10))
        app.navigationBars.buttons.firstMatch.tap()
        let target = app.tables.cells.containing(.staticText, identifier: "Native forward target").firstMatch
        XCTAssertTrue(target.waitForExistence(timeout: 10))
        target.tap()
        XCTAssertTrue(text.waitForExistence(timeout: 10))
        XCTAssertTrue(text.isHittable)
        XCTAssertTrue(
            app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "Forwarded")).firstMatch
                .waitForExistence(timeout: 10))
        let attachment = XCTAttachment(screenshot: app.screenshot())
        attachment.name = "native-reference-forward-current-projection"
        attachment.lifetime = .keepAlways
        add(attachment)
        text.tap()
        XCTAssertTrue(app.buttons["Recall message"].waitForExistence(timeout: 10))
        XCTAssertFalse(app.buttons["Edit"].exists)
        app.buttons["Recall message"].tap()
        XCTAssertTrue(text.waitForNonExistence(timeout: 10))
    }
}
