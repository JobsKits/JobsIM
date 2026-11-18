//
//  IMAdditionalUIDraftTests.swift
//  JobsIMUITestsDraft
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import XCTest

// Draft only: copy the two methods into IMUITests after its current build inputs are fixed.
@MainActor
final class IMAdditionalUIDraftTests: XCTestCase {
    func testDemoPersonalSavedSettingsDarkAppearanceAndFirstTab() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "light"]
        app.launch()
        defer {
            app.terminate()
        }
        let tabs = app.tabBars.firstMatch
        XCTAssertTrue(tabs.waitForExistence(timeout: 10))
        XCTAssertEqual(tabs.buttons.count, 3)
        XCTAssertEqual(tabs.buttons.element(boundBy: 0).label, "联系人")
        tabs.buttons.element(boundBy: 2).tap()
        let saved = app.tables.cells["personal.saved"]
        XCTAssertTrue(saved.waitForExistence(timeout: 5))
        XCTAssertTrue(app.tables.cells["personal.settings"].exists)
        saved.tap()
        XCTAssertTrue(app.navigationBars.staticTexts["收藏"].waitForExistence(timeout: 5))
        app.navigationBars.buttons.firstMatch.tap()
        app.tables.cells["personal.settings"].tap()
        let theme = app.tables.cells["settings.theme"]
        XCTAssertTrue(theme.waitForExistence(timeout: 5))
        theme.tap()
        let dark = app.buttons["黑夜"]
        XCTAssertTrue(dark.waitForExistence(timeout: 5))
        dark.tap()
        // This asserts the Settings route survives the appearance change, not pixel contrast.
        XCTAssertTrue(theme.waitForExistence(timeout: 5))
        XCTAssertTrue(theme.isHittable)
        XCTAssertTrue(app.tables.cells["settings.language"].isHittable)
        let settingsCapture = XCTAttachment(screenshot: app.screenshot())
        settingsCapture.name = "demo-dark-settings-retained-after-appearance-change"
        settingsCapture.lifetime = .keepAlways
        add(settingsCapture)

        app.navigationBars.buttons.firstMatch.tap()
        XCTAssertTrue(app.tables.cells["personal.settings"].waitForExistence(timeout: 5))
        tabs.buttons.element(boundBy: 0).tap()
        XCTAssertTrue(tabs.buttons.element(boundBy: 0).isSelected)
        XCTAssertTrue(app.tables.cells["conversation.lin"].waitForExistence(timeout: 5))
        for title in ["新的朋友", "群聊", "星标朋友"] {
            XCTAssertTrue(app.buttons[title].isHittable)
        }
        let contactsCapture = XCTAttachment(screenshot: app.screenshot())
        contactsCapture.name = "demo-dark-contacts-first-tab-after-settings"
        contactsCapture.lifetime = .keepAlways
        add(contactsCapture)
    }

    func testDemoExitToUnauthenticatedAppDoorLoginRegistrationRail() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "light"]
        app.launch()
        defer {
            app.terminate()
        }
        let tabs = app.tabBars.firstMatch
        XCTAssertTrue(tabs.waitForExistence(timeout: 10))
        tabs.buttons.element(boundBy: 2).tap()
        let settings = app.tables.cells["personal.settings"]
        XCTAssertTrue(settings.waitForExistence(timeout: 5))
        settings.tap()
        let connection = app.tables.cells["settings.connection"]
        XCTAssertTrue(connection.waitForExistence(timeout: 5))
        // On the dedicated unauthenticated demo simulator this exits locally; no connect/submit is tapped.
        connection.tap()
        let primary = app.buttons["auth.primary"]
        XCTAssertTrue(primary.waitForExistence(timeout: 10),
            "Requires the dedicated simulator with no saved formal session; do not sign out a real account")
        XCTAssertEqual(primary.label, "登录")
        XCTAssertTrue(app.textFields["auth.username"].isHittable)
        XCTAssertTrue(app.secureTextFields["auth.password"].isHittable)
        let confirmation = app.secureTextFields["auth.confirmPassword"]
        XCTAssertFalse(confirmation.isHittable)
        let rail = app.buttons["auth.modeSwitch"]
        XCTAssertTrue(rail.isHittable)
        let loginCapture = XCTAttachment(screenshot: app.screenshot())
        loginCapture.name = "demo-exit-unauthenticated-AppDoor-login-right-rail"
        loginCapture.lifetime = .keepAlways
        add(loginCapture)

        rail.tap()
        XCTAssertTrue(confirmation.waitForExistence(timeout: 5))
        XCTAssertTrue(confirmation.isHittable)
        XCTAssertEqual(primary.label, "注册")
        XCTAssertLessThan(rail.frame.midX, app.textFields["auth.username"].frame.midX)
        let registrationCapture = XCTAttachment(screenshot: app.screenshot())
        registrationCapture.name = "AppDoor-registration-left-rail-confirm-password-visible"
        registrationCapture.lifetime = .keepAlways
        add(registrationCapture)

        rail.tap()
        XCTAssertEqual(primary.label, "登录")
        XCTAssertFalse(confirmation.isHittable)
        XCTAssertGreaterThan(rail.frame.midX, app.textFields["auth.username"].frame.midX)
        let restoredCapture = XCTAttachment(screenshot: app.screenshot())
        restoredCapture.name = "AppDoor-login-restored-confirm-password-hidden"
        restoredCapture.lifetime = .keepAlways
        add(restoredCapture)
    }
}
