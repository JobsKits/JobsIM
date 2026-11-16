//
//  IMUITests.swift
//  JobsIMUITests
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import XCTest

@MainActor
final class IMUITests: XCTestCase {

    func testDemoMessageAndAppearanceLanguageControls() {
        let messageText = "Native durable " + UUID().uuidString.prefix(8)
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "light"]
        app.launch()
        XCTAssertTrue(app.staticTexts["演示空间"].waitForExistence(timeout: 10))
        let conversation = app.tables.cells["conversation.lin"]
        XCTAssertTrue(conversation.waitForExistence(timeout: 5))
        conversation.tap()
        let input = app.textFields["写一条消息…"]
        XCTAssertTrue(input.waitForExistence(timeout: 5))
        input.tap()
        input.typeText(messageText)
        app.buttons["发送"].tap()
        XCTAssertTrue(app.staticTexts[messageText].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts[messageText].isHittable)
        let messageCapture = XCTAttachment(screenshot: app.screenshot())
        messageCapture.name = "native-message-light"
        messageCapture.lifetime = .keepAlways
        add(messageCapture)
        app.terminate()
        app.launch()
        XCTAssertTrue(conversation.waitForExistence(timeout: 10))
        conversation.tap()
        XCTAssertTrue(app.staticTexts[messageText].waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts[messageText].isHittable)
        app.terminate()
        app.launch()
        XCTAssertTrue(app.tabBars.buttons["设置"].waitForExistence(timeout: 10))
        app.tabBars.buttons["设置"].tap()
        app.tables.cells.containing(.staticText, identifier: "外观").firstMatch.tap()
        app.buttons["黑夜"].tap()
        XCTAssertTrue(app.staticTexts["演示空间"].waitForExistence(timeout: 5))
        app.tabBars.buttons["设置"].tap()
        app.tables.cells.containing(.staticText, identifier: "语言").firstMatch.tap()
        app.buttons["English"].tap()
        XCTAssertTrue(app.tabBars.buttons["Messages"].waitForExistence(timeout: 5))
        let darkCapture = XCTAttachment(screenshot: app.screenshot())
        darkCapture.name = "native-message-dark-English"
        darkCapture.lifetime = .keepAlways
        add(darkCapture)
    }
    func testNativeHTTPSRegistrationAndFormalSend() {
        continueAfterFailure = false
        let interruption = addUIInterruptionMonitor(withDescription: "Dismiss saving ephemeral test password")
        { alert in
            for label in ["以后", "稍后", "Not Now", "Later"] where alert.buttons[label].exists {
                alert.buttons[label].tap()
                return true
            }
            return false
        }
        defer {
            removeUIInterruptionMonitor(interruption)
        }
        let app = XCUIApplication()
        app.launchArguments = ["--language", "en", "--ui-test-generated-password"]
        app.launch()
        if app.tabBars.buttons["Settings"].exists {
            app.tabBars.buttons["Settings"].tap()
            app.tables.cells.containing(.staticText, identifier: "Sign out").firstMatch.tap()
        }
        let address = app.textFields["auth.address"]
        XCTAssertTrue(address.waitForExistence(timeout: 10))
        address.tap()
        if let value = address.value as? String, value.hasPrefix("http") {
            address.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: value.count))
        }
        address.typeText("https://127.0.0.1:8443")
        app.buttons["auth.connect"].tap()
        XCTAssertTrue(app.alerts.firstMatch.waitForExistence(timeout: 10))
        app.alerts.buttons["Verify and connect"].tap()
        app.buttons["Create account"].firstMatch.tap()
        app.textFields["auth.username"].tap()
        app.textFields["auth.username"].typeText(
            "ios_" + UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(16))
        app.buttons["auth.primary"].tap()
        XCTAssertTrue(app.tabBars.buttons["Messages"].waitForExistence(timeout: 12))
        let create = app.buttons["New conversation"]
        if !create.isHittable {
            let owners = [
                app, XCUIApplication(bundleIdentifier: "com.apple.springboard"),
                XCUIApplication(
                    bundleIdentifier: "com.apple.AuthenticationServicesUI.AuthenticationServicesAgent"),
            ]
            var dismissed = false
            for owner in owners {
                for label in ["以后", "稍后", "Not Now", "Later"] where owner.buttons[label].exists {
                    owner.buttons[label].tap()
                    dismissed = true
                    break
                }
                if dismissed {
                    break
                }
            }
            if !dismissed {
                // iOS 27.2 的远程密码浮窗缺少AX节点；只在专用固定模拟器点已截图核实的“以后”。
                app.coordinate(withNormalizedOffset: CGVector(dx: 0.316, dy: 0.615)).tap()
            }
        }
        // 重启同时验证Keychain恢复，避免系统凭据远程窗口的退场快照拦截下一次命中测试。
        app.terminate()
        app.launch()
        XCTAssertTrue(app.tabBars.buttons["Messages"].waitForExistence(timeout: 10))
        app.buttons["New conversation"].tap()
        XCTAssertTrue(app.alerts.textFields.firstMatch.waitForExistence(timeout: 5))
        app.alerts.textFields.firstMatch.typeText("Native integration")
        app.alerts.buttons["Continue"].tap()
        let input = app.textFields["Write a message…"]
        XCTAssertTrue(input.waitForExistence(timeout: 10))
        input.tap()
        input.typeText("A real native conversation")
        app.buttons["Send"].tap()
        XCTAssertTrue(app.staticTexts["A real native conversation"].waitForExistence(timeout: 10))
        let accepted = app.staticTexts.matching(
            NSPredicate(format: "label CONTAINS %@", "Accepted by server")
        ).firstMatch
        XCTAssertTrue(accepted.waitForExistence(timeout: 10))
        XCTAssertTrue(app.staticTexts["Your conversations"].waitForExistence(timeout: 8))
        XCTAssertFalse(app.staticTexts["Offline · history kept"].exists)
        let evidence = XCTAttachment(screenshot: app.screenshot())
        evidence.name = "native-HTTPS-formal-send"
        evidence.lifetime = .keepAlways
        add(evidence)
    }

    func testGovernanceNativeUI() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--language", "en", "--theme", "light", "--ui-test-profile", "governance"]
        app.launch()
        XCTAssertTrue(app.tabBars.buttons["Contacts"].waitForExistence(timeout: 10))
        app.tabBars.buttons["Contacts"].tap()
        app.buttons["Contact requests"].tap()
        let pending = app.tables.cells.containing(.staticText, identifier: "Pending").firstMatch
        XCTAssertTrue(pending.waitForExistence(timeout: 10))
        pending.tap()
        app.buttons["Accept"].tap()
        XCTAssertTrue(
            app.tables.cells.containing(.staticText, identifier: "Accepted").firstMatch.waitForExistence(
                timeout: 10))
        app.navigationBars.buttons.firstMatch.tap()
        let incoming = app.tables.cells.containing(.staticText, identifier: "Native incoming peer").firstMatch
        XCTAssertTrue(incoming.waitForExistence(timeout: 10))
        incoming.tap()
        app.buttons["Remove contact"].tap()
        XCTAssertTrue(incoming.waitForNonExistence(timeout: 10))
        app.tabBars.buttons["Messages"].tap()
        let group = app.tables.cells.containing(.staticText, identifier: "Native governance group").firstMatch
        XCTAssertTrue(group.waitForExistence(timeout: 10))
        group.tap()
        app.buttons["Members"].tap()
        app.tables.cells.containing(.staticText, identifier: "Rename group").firstMatch.tap()
        let name = app.alerts.textFields.firstMatch
        let old = name.value as? String ?? ""
        name.tap()
        name.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: old.count))
        name.typeText("Native renamed group")
        app.alerts.buttons["Continue"].tap()
        let peer = app.tables.cells.containing(.staticText, identifier: "Native governance peer").firstMatch
        XCTAssertTrue(peer.waitForExistence(timeout: 10))
        peer.tap()
        app.buttons["Transfer ownership"].tap()
        XCTAssertTrue(
            app.tables.cells.containing(.staticText, identifier: "Rename group").firstMatch
                .waitForNonExistence(timeout: 10))
        let capture = XCTAttachment(screenshot: app.screenshot())
        capture.name = "native-group-owner-transferred"
        capture.lifetime = .keepAlways
        add(capture)
        app.tables.cells.containing(.staticText, identifier: "Leave group").firstMatch.tap()
        app.buttons["Leave group"].tap()
        XCTAssertTrue(app.buttons["Members"].waitForExistence(timeout: 10))
    }

    func testNotificationPrivacySettings() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--language", "en", "--theme", "light"]
        app.launch()
        XCTAssertTrue(app.tabBars.buttons["Settings"].waitForExistence(timeout: 10))
        app.tabBars.buttons["Settings"].tap()
        app.tables.cells.containing(.staticText, identifier: "Notifications").firstMatch.tap()
        let enable = app.buttons.matching(NSPredicate(format: "label BEGINSWITH %@", "Enable local alerts"))
            .firstMatch
        XCTAssertTrue(enable.waitForExistence(timeout: 5))
        if enable.label.hasSuffix("Off") {
            enable.tap()
            let springboard = XCUIApplication(bundleIdentifier: "com.apple.springboard")
            let allow = springboard.buttons.matching(
                NSPredicate(format: "label IN %@", ["Allow", "允许", "允許"])
            ).firstMatch
            if allow.waitForExistence(timeout: 4) { allow.tap() }
            app.tables.cells.containing(.staticText, identifier: "Notifications").firstMatch.tap()
        }
        XCTAssertTrue(app.buttons["Enable local alerts · On"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["Message previews · Off"].exists)
        let capture = XCTAttachment(screenshot: app.screenshot())
        capture.name = "native-notification-default-privacy"
        capture.lifetime = .keepAlways
        add(capture)
    }

    private func choosePublicFile(_ name: String, app: XCUIApplication) {
        let direct = app.cells.matching(NSPredicate(format: "label CONTAINS %@", name)).firstMatch
        if direct.waitForExistence(timeout: 5) { direct.tap(); return }
        let tabs = app.tabBars["DOC.browsingModeTabBar"]
        XCTAssertTrue(tabs.waitForExistence(timeout: 8))
        let browse = tabs.buttons.matching(NSPredicate(format: "label IN %@", ["Browse", "浏览", "瀏覽"]))
            .firstMatch
        XCTAssertTrue(browse.waitForExistence(timeout: 5))
        browse.tap()
        let local = app.staticTexts.matching(
            NSPredicate(format: "label IN %@", ["On My iPhone", "我的 iPhone", "我的iPhone", "在我的 iPhone 上"])
        ).firstMatch
        if local.waitForExistence(timeout: 3) { local.tap() }
        let folder = app.cells.containing(.staticText, identifier: "JobsIM").firstMatch
        if folder.waitForExistence(timeout: 3) { folder.tap() }
        let file = app.cells.matching(NSPredicate(format: "label CONTAINS %@", name)).firstMatch
        if file.waitForExistence(timeout: 5) { file.tap(); return }
        let text = app.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", name)).firstMatch
        XCTAssertTrue(text.waitForExistence(timeout: 5), app.debugDescription)
        text.tap()
    }
    func testAvatarPickerAuthenticatedListsAndCurrentDeviceRevocation() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--language", "en", "--theme", "light", "--ui-test-profile", "picker"]
        app.launch()
        XCTAssertTrue(app.tabBars.buttons["Contacts"].waitForExistence(timeout: 10))
        app.tabBars.buttons["Contacts"].tap()
        XCTAssertTrue(app.staticTexts["Native portrait peer"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.otherElements["avatar.remote"].firstMatch.waitForExistence(timeout: 10))
        app.tabBars.buttons["Messages"].tap()
        XCTAssertTrue(app.otherElements["avatar.remote"].firstMatch.waitForExistence(timeout: 10))
        let group = app.tables.cells.containing(.staticText, identifier: "Native picker group").firstMatch
        XCTAssertTrue(group.waitForExistence(timeout: 10))
        group.tap()
        app.buttons["Members"].tap()
        XCTAssertTrue(app.otherElements["avatar.remote"].firstMatch.waitForExistence(timeout: 10))
        let members = XCTAttachment(screenshot: app.screenshot())
        members.name = "native-group-authenticated-member-avatars"
        members.lifetime = .keepAlways
        add(members)
        app.navigationBars.buttons.firstMatch.tap()
        app.navigationBars.buttons.firstMatch.tap()
        let direct = app.tables.cells.matching(NSPredicate(format: "label CONTAINS %@", "Native image.png"))
            .firstMatch
        XCTAssertTrue(direct.waitForExistence(timeout: 10))
        direct.tap()
        XCTAssertTrue(app.images["message.image"].waitForExistence(timeout: 10))
        XCTAssertTrue(app.otherElements["avatar.remote"].firstMatch.exists)
        let draft = app.textFields["Write a message…"]
        draft.tap()
        draft.typeText("A draft survives native restart")
        app.terminate()
        app.launch()
        let restored = app.tables.cells.matching(NSPredicate(format: "label CONTAINS %@", "Native image.png"))
            .firstMatch
        XCTAssertTrue(restored.waitForExistence(timeout: 10))
        restored.tap()
        XCTAssertEqual(app.textFields["Write a message…"].value as? String, "A draft survives native restart")
        let capture = XCTAttachment(screenshot: app.screenshot())
        capture.name = "authenticated-image-avatar-and-restored-draft"
        capture.lifetime = .keepAlways
        add(capture)
        app.navigationBars.buttons.firstMatch.tap()
        app.tabBars.buttons["Settings"].tap()
        app.tables.cells.containing(.staticText, identifier: "Profile photo").firstMatch.tap()
        app.buttons["avatar.choose"].tap()
        choosePublicFile("JobsIM-Avatar", app: app)
        XCTAssertTrue(app.otherElements["avatar.remote"].waitForExistence(timeout: 15))
        let avatar = XCTAttachment(screenshot: app.screenshot())
        avatar.name = "native-avatar-document-picker-applied"
        avatar.lifetime = .keepAlways
        add(avatar)
        app.navigationBars.buttons.firstMatch.tap()
        app.tables.cells.containing(.staticText, identifier: "Recovery materials").firstMatch.tap()
        XCTAssertTrue(app.alerts.secureTextFields.firstMatch.waitForExistence(timeout: 5))
        app.alerts.buttons["Cancel"].tap()
        app.tables.cells.containing(.staticText, identifier: "Export local history").firstMatch.tap()
        let copy = app.cells.matching(NSPredicate(format: "label IN %@", ["Copy", "拷贝", "复制"])).firstMatch
        XCTAssertTrue(copy.waitForExistence(timeout: 8), app.debugDescription)
        let export = XCTAttachment(screenshot: app.screenshot())
        export.name = "native-local-history-export-share-sheet"
        export.lifetime = .keepAlways
        add(export)
        let close = app.buttons.matching(NSPredicate(format: "label IN %@", ["Close", "关闭", "取消", "Cancel"]))
            .firstMatch
        if close.exists {
            close.tap()
        } else {
            let dismiss = app.otherElements["PopoverDismissRegion"]
            XCTAssertTrue(dismiss.exists)
            dismiss.tap()
        }
        app.tables.cells.containing(.staticText, identifier: "Your devices").firstMatch.tap()
        let current = app.tables.cells.containing(.staticText, identifier: "This device").firstMatch
        XCTAssertTrue(current.waitForExistence(timeout: 10))
        current.tap()
        app.alerts.buttons["Continue"].tap()
        XCTAssertTrue(app.textFields["auth.address"].waitForExistence(timeout: 10))
        app.terminate()
        app.launch()
        XCTAssertTrue(app.textFields["auth.address"].waitForExistence(timeout: 10))
    }
    func testCertificateDocumentPickerWithIndependentFingerprint() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--language", "en", "--theme", "light"]
        app.launch()
        if app.tabBars.buttons["Settings"].exists {
            app.tabBars.buttons["Settings"].tap()
            app.tables.cells.containing(.staticText, identifier: "Sign out").firstMatch.tap()
        }
        XCTAssertTrue(app.buttons["auth.ca"].waitForExistence(timeout: 10))
        app.buttons["Remove instance trust"].tap()
        app.alerts.buttons["Continue"].tap()
        XCTAssertTrue(app.alerts.firstMatch.waitForNonExistence(timeout: 5))
        app.buttons["auth.connect"].tap()
        XCTAssertTrue(app.alerts.firstMatch.waitForExistence(timeout: 10))
        XCTAssertFalse(app.alerts.buttons["Verify and connect"].exists)
        app.alerts.buttons["Done"].tap()
        app.buttons["auth.ca"].tap()
        choosePublicFile("JobsIM-CA", app: app)
        XCTAssertTrue(app.alerts.textFields.firstMatch.waitForExistence(timeout: 8))
        app.alerts.textFields.firstMatch.tap()
        app.alerts.textFields.firstMatch.typeText(
            "a5303379ba7fafcb197ed761a6ac132c6f9c2e762a06c205bba6d6372cb4a9d8")
        app.alerts.buttons["Continue"].tap()
        XCTAssertTrue(
            app.alerts.staticTexts.matching(NSPredicate(format: "label CONTAINS %@", "SHA256")).firstMatch
                .waitForExistence(timeout: 5))
        let capture = XCTAttachment(screenshot: app.screenshot())
        capture.name = "native-CA-picker-explicit-instance-confirmation"
        capture.lifetime = .keepAlways
        add(capture)
        app.alerts.buttons["Continue"].tap()
        app.buttons["auth.connect"].tap()
        XCTAssertTrue(app.alerts.buttons["Verify and connect"].waitForExistence(timeout: 10))
        app.alerts.buttons["Verify and connect"].tap()
        XCTAssertTrue(app.staticTexts["Connected"].waitForExistence(timeout: 5))
    }

}
