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

    func testDemoContactsFirstNavigationAndKeyboardDismissal() {
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
        XCTAssertEqual(tabs.buttons.element(boundBy: 1).label, "消息")
        XCTAssertTrue(tabs.buttons.element(boundBy: 0).isSelected)
        let duplicateTitles = app.staticTexts.matching(identifier: "联系人").allElementsBoundByIndex.filter {
            $0.isHittable && $0.frame.maxY < tabs.frame.minY
        }
        XCTAssertTrue(duplicateTitles.isEmpty)
        for title in ["新的朋友", "群聊", "星标朋友"] {
            XCTAssertTrue(app.buttons[title].isHittable)
        }
        XCTAssertFalse(app.buttons["新建会话"].exists)
        let rootCapture = XCTAttachment(screenshot: app.screenshot())
        rootCapture.name = "demo-contacts-first-three-tabs-no-duplicate-title"
        rootCapture.lifetime = .keepAlways
        add(rootCapture)

        let visibleContact = app.tables.cells["conversation.lin"]
        XCTAssertTrue(visibleContact.descendants(matching: .any).matching(identifier: "avatar.fallback").firstMatch.isHittable)
        visibleContact.tap()
        let input = app.textFields["写一条消息…"]
        XCTAssertTrue(input.waitForExistence(timeout: 5))
        XCTAssertFalse(tabs.isHittable)
        input.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForExistence(timeout: 5))
        guard let visibleMessage = app.tables.cells.allElementsBoundByIndex.first(where: { $0.isHittable }) else {
            XCTFail("No visible message cell available for a direct tap")
            return
        }
        visibleMessage.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForNonExistence(timeout: 5))
        XCTAssertTrue(input.isHittable)
        XCTAssertTrue(app.buttons["发送"].isHittable)
        let chatCapture = XCTAttachment(screenshot: app.screenshot())
        chatCapture.name = "demo-direct-chat-hidden-tab-cell-tap-dismisses-keyboard"
        chatCapture.lifetime = .keepAlways
        add(chatCapture)
    }

    func testDemoRecallKeepsExistingDraftAndCanRestoreIt() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "light"]
        app.launch()
        defer {
            app.terminate()
        }
        let contact = app.tables.cells["conversation.lin"]
        XCTAssertTrue(contact.waitForExistence(timeout: 10))
        contact.tap()
        XCTAssertTrue(app.buttons["聊天设置"].waitForExistence(timeout: 5))
        app.buttons["聊天设置"].tap()
        let relationshipReady = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: "enabled == YES"), object: app.buttons["删除联系人"])
        XCTAssertEqual(XCTWaiter.wait(for: [relationshipReady], timeout: 5), .completed)
        if app.buttons["解除拉黑"].exists {
            app.buttons["解除拉黑"].tap()
            XCTAssertTrue(app.buttons["拉黑"].waitForExistence(timeout: 5))
        }
        app.navigationBars.buttons.firstMatch.tap()

        let suffix = String(UUID().uuidString.prefix(8))
        let original = "UI recall original " + suffix
        let currentDraft = "UI preserved draft " + suffix
        let input = app.textFields["写一条消息…"]
        XCTAssertTrue(input.waitForExistence(timeout: 5))
        replaceComposerText(input, with: original, app: app)
        app.buttons["发送"].tap()
        let message = app.staticTexts[original]
        XCTAssertTrue(message.waitForExistence(timeout: 5))
        input.tap()
        input.typeText(currentDraft)
        message.tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForNonExistence(timeout: 5))
        message.press(forDuration: 1.2)
        let edit = app.buttons["撤回后编辑"]
        XCTAssertTrue(edit.waitForExistence(timeout: 5))
        edit.tap()
        let conflict = app.alerts["已有输入草稿"]
        XCTAssertTrue(conflict.waitForExistence(timeout: 5))
        let overwrite = conflict.buttons["覆盖"]
        let keep = conflict.buttons["保存"]
        XCTAssertLessThan(overwrite.frame.midX, keep.frame.midX)
        XCTAssertTrue(message.exists)
        let conflictCapture = XCTAttachment(screenshot: app.screenshot())
        conflictCapture.name = "demo-recall-edit-before-choice-preserves-original-and-draft"
        conflictCapture.lifetime = .keepAlways
        add(conflictCapture)
        keep.tap()
        let originalInEditor = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: "value == %@", original), object: input)
        XCTAssertEqual(XCTWaiter.wait(for: [originalInEditor], timeout: 5), .completed)
        XCTAssertTrue(message.waitForNonExistence(timeout: 5))

        app.buttons["聊天设置"].tap()
        let savedDrafts = app.buttons["已保存草稿"]
        XCTAssertTrue(savedDrafts.waitForExistence(timeout: 5))
        if !savedDrafts.isHittable {
            app.scrollViews.firstMatch.swipeUp()
        }
        savedDrafts.tap()
        let draft = app.tables.cells.containing(.staticText, identifier: currentDraft).firstMatch
        XCTAssertTrue(draft.waitForExistence(timeout: 5))
        draft.tap()
        XCTAssertTrue(app.alerts["已有输入草稿"].waitForExistence(timeout: 5))
        app.alerts.buttons["取消"].tap()
        XCTAssertTrue(draft.isHittable)
        draft.tap()
        XCTAssertTrue(app.alerts["已有输入草稿"].waitForExistence(timeout: 5))
        app.alerts.buttons["继续"].tap()
        let restored = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: "value == %@", currentDraft), object: input)
        XCTAssertEqual(XCTWaiter.wait(for: [restored], timeout: 5), .completed)
        let restoreCapture = XCTAttachment(screenshot: app.screenshot())
        restoreCapture.name = "demo-saved-existing-draft-restored-after-explicit-confirmation"
        restoreCapture.lifetime = .keepAlways
        add(restoreCapture)
        replaceComposerText(input, with: "", app: app)
    }

    func testDemoContactBlockPreservesFriendAndDeletionCancel() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "dark"]
        app.launch()
        defer {
            app.terminate()
        }
        let contact = app.tables.cells["conversation.lin"]
        XCTAssertTrue(contact.waitForExistence(timeout: 10))
        contact.tap()
        app.buttons["聊天设置"].tap()
        let relationshipReady = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: "enabled == YES"), object: app.buttons["删除联系人"])
        XCTAssertEqual(XCTWaiter.wait(for: [relationshipReady], timeout: 5), .completed)
        if app.buttons["解除拉黑"].exists {
            app.buttons["解除拉黑"].tap()
            XCTAssertTrue(app.buttons["拉黑"].waitForExistence(timeout: 5))
        }
        app.buttons["拉黑"].tap()
        XCTAssertTrue(app.buttons["解除拉黑"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.buttons["删除联系人"].exists)
        let blockedCapture = XCTAttachment(screenshot: app.screenshot())
        blockedCapture.name = "demo-block-preserves-contact-and-remove-action"
        blockedCapture.lifetime = .keepAlways
        add(blockedCapture)
        app.navigationBars.buttons.firstMatch.tap()
        app.navigationBars.buttons.firstMatch.tap()
        XCTAssertTrue(contact.waitForExistence(timeout: 5))
        contact.tap()
        app.buttons["聊天设置"].tap()
        XCTAssertTrue(app.buttons["解除拉黑"].waitForExistence(timeout: 5))
        app.buttons["解除拉黑"].tap()
        XCTAssertTrue(app.buttons["拉黑"].waitForExistence(timeout: 5))
        app.buttons["删除联系人"].tap()
        let alert = app.alerts["删除联系人"]
        XCTAssertTrue(alert.waitForExistence(timeout: 5))
        XCTAssertTrue(alert.staticTexts["删除联系人后无法撤销，合法聊天记录仍保留。"].exists)
        XCTAssertLessThan(alert.buttons["删除"].frame.midX, alert.buttons["手滑"].frame.midX)
        let confirmationCapture = XCTAttachment(screenshot: app.screenshot())
        confirmationCapture.name = "demo-contact-delete-left-right-cancel-protection"
        confirmationCapture.lifetime = .keepAlways
        add(confirmationCapture)
        alert.buttons["手滑"].tap()
        XCTAssertTrue(alert.waitForNonExistence(timeout: 5))
        XCTAssertTrue(app.buttons["删除联系人"].isHittable)
        app.navigationBars.buttons.firstMatch.tap()
        app.navigationBars.buttons.firstMatch.tap()
        XCTAssertTrue(contact.waitForExistence(timeout: 5))
        contact.swipeLeft()
        let trailingDelete = app.buttons["删除"].firstMatch
        XCTAssertTrue(trailingDelete.waitForExistence(timeout: 5))
        trailingDelete.tap()
        XCTAssertTrue(alert.waitForExistence(timeout: 5))
        alert.buttons["手滑"].tap()
        XCTAssertTrue(alert.waitForNonExistence(timeout: 5))
        XCTAssertTrue(contact.isHittable)
    }

    func testDemoAvatarLongPressOpensReadOnlyPeerProfile() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "light"]
        app.launch()
        defer {
            app.terminate()
        }
        let contact = app.tables.cells["conversation.lin"]
        XCTAssertTrue(contact.waitForExistence(timeout: 10))
        contact.tap()
        let inbound = app.tables.cells.containing(.staticText, identifier: "那就沿着河边慢慢走吧。").firstMatch
        XCTAssertTrue(inbound.waitForExistence(timeout: 5))
        for _ in 0..<5 where !inbound.isHittable {
            app.tables.firstMatch.swipeDown()
        }
        let avatar = inbound.descendants(matching: .any).matching(identifier: "avatar.fallback").firstMatch
        XCTAssertTrue(avatar.waitForExistence(timeout: 5))
        XCTAssertTrue(avatar.isHittable)
        avatar.press(forDuration: 1.2)
        let username = app.textFields["profile.username"]
        XCTAssertTrue(username.waitForExistence(timeout: 5))
        XCTAssertFalse(username.isEnabled)
        XCTAssertFalse(app.textFields["profile.displayName"].isEnabled)
        XCTAssertTrue(app.staticTexts["仅显示当前有权查看的资料"].exists)
        XCTAssertFalse(app.buttons["profile.save"].isHittable)
        let capture = XCTAttachment(screenshot: app.screenshot())
        capture.name = "demo-peer-avatar-long-press-read-only-profile"
        capture.lifetime = .keepAlways
        add(capture)
    }

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
        XCTAssertTrue(theme.waitForExistence(timeout: 5))
        XCTAssertTrue(theme.isHittable)
        XCTAssertTrue(theme.staticTexts["黑夜"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.tables.cells["settings.language"].isHittable)
        let settingsCapture = XCTAttachment(screenshot: app.screenshot())
        settingsCapture.name = "demo-dark-settings-retained-after-appearance-change"
        settingsCapture.lifetime = .keepAlways
        add(settingsCapture)

        for (option, pictureName) in [
            ("白天", "settings-switched-back-to-light-detail-and-cells"),
            ("跟随系统", "settings-system-choice-detail-and-current-effective-appearance"),
            ("黑夜", "settings-repeated-dark-choice-detail-and-cells"),
        ] {
            theme.tap()
            let choice = app.buttons[option]
            XCTAssertTrue(choice.waitForExistence(timeout: 5))
            choice.tap()
            XCTAssertTrue(theme.staticTexts[option].waitForExistence(timeout: 5))
            let picture = XCTAttachment(screenshot: app.screenshot())
            picture.name = pictureName
            picture.lifetime = .keepAlways
            add(picture)
        }

        app.navigationBars.buttons.firstMatch.tap()
        XCTAssertTrue(app.tables.cells["personal.settings"].waitForExistence(timeout: 5))
        let personalCapture = XCTAttachment(screenshot: app.screenshot())
        personalCapture.name = "personal-center-dark-cells-after-repeated-theme-switches"
        personalCapture.lifetime = .keepAlways
        add(personalCapture)
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
        let environment = app.buttons["demo.environment"]
        XCTAssertTrue(environment.waitForExistence(timeout: 5))
        let origin = environment.frame
        let from = environment.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5))
        from.press(forDuration: 0.1, thenDragTo: from.withOffset(CGVector(dx: -70, dy: 80)))
        XCTAssertNotEqual(environment.frame, origin)
        XCTAssertFalse(app.staticTexts["演示空间"].exists)
        environment.tap()
        let connectionWarning = app.alerts.firstMatch
        if connectionWarning.waitForExistence(timeout: 2) {
            XCTAssertTrue(connectionWarning.buttons["取消"].exists)
            XCTAssertTrue(connectionWarning.buttons["跳转设置"].exists)
            connectionWarning.buttons["取消"].tap()
        }
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
    private func replaceComposerText(_ input: XCUIElement, with value: String, app: XCUIApplication) {
        input.tap()
        let existing = input.value as? String ?? ""
        if !existing.isEmpty && existing != "写一条消息…" {
            input.press(forDuration: 1.2)
            let predicate = NSPredicate(format: "label IN %@", ["Select All", "全选", "選取全部", "全選"])
            let item = app.menuItems.matching(predicate).firstMatch
            let button = app.buttons.matching(predicate).firstMatch
            if item.waitForExistence(timeout: 1) {
                item.tap()
            } else {
                XCTAssertTrue(button.waitForExistence(timeout: 4), app.debugDescription)
                button.tap()
            }
        }
        input.typeText(value.isEmpty ? XCUIKeyboardKey.delete.rawValue : value)
        if !value.isEmpty {
            XCTAssertEqual(input.value as? String, value)
        }
    }


    func testDemoReportSystemPickerPrivateCopyPreviewRemovalAndSubmit() throws {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "light"]
        app.launch()
        defer {
            app.terminate()
        }
        let contact = app.tables.cells["conversation.lin"]
        XCTAssertTrue(contact.waitForExistence(timeout: 10))
        contact.tap()
        app.buttons["聊天设置"].tap()
        let report = app.buttons["举报"]
        XCTAssertTrue(report.waitForExistence(timeout: 5))
        report.tap()
        let reason = app.textViews.firstMatch
        XCTAssertTrue(reason.waitForExistence(timeout: 5))
        reason.tap()
        reason.typeText("UI demo report fixture " + String(UUID().uuidString.prefix(8)))
        let material = app.tables.cells.containing(.staticText, identifier: "JobsIM-UI-Report-Fixture-20261118.png").firstMatch

        func pickOnlyLocalFixture() throws {
            let addMedia = app.buttons["添加图片或视频"]
            XCTAssertTrue(addMedia.waitForExistence(timeout: 5))
            if !addMedia.isHittable {
                app.scrollViews.firstMatch.swipeUp()
            }
            addMedia.tap()
            // 只选择本任务导入且SHA已核对的AppIcon；创建时间来自该副本的Photos元数据。
            let fixtures = app.images.matching(identifier: "PXGGridLayout-Info").matching(
                NSPredicate(format: "label CONTAINS %@", "11月18日, 15:09"))
            XCTAssertTrue(fixtures.firstMatch.waitForExistence(timeout: 8), app.debugDescription)
            XCTAssertEqual(fixtures.count, 1, "Never select an unknown or ambiguous photo")
            let asset = fixtures.firstMatch
            XCTAssertTrue(asset.isHittable)
            let pickerSnapshot = XCTAttachment(screenshot: app.screenshot())
            pickerSnapshot.name = "native-PHPicker-only-AppIcon-fixture-before-selection"
            pickerSnapshot.lifetime = .keepAlways
            add(pickerSnapshot)
            asset.tap()
            let accept = app.navigationBars.buttons.matching(
                NSPredicate(format: "label BEGINSWITH %@ OR label == %@", "添加", "完成")
            ).firstMatch
            XCTAssertTrue(accept.waitForExistence(timeout: 5), app.debugDescription)
            accept.tap()
            // The provider callback must have copied its expiring URL before this material row appears.
            XCTAssertTrue(material.waitForExistence(timeout: 15), app.debugDescription)
            let metadata = material.staticTexts.matching(NSPredicate(format: "label BEGINSWITH %@", "image/")).firstMatch
            XCTAssertTrue(metadata.exists)
            XCTAssertGreaterThan(metadata.frame.height, 10)
            XCTAssertTrue(metadata.label.range(of: "[1-9][0-9]* B", options: .regularExpression) != nil,
                "The material must show a visible nonzero staged-file size")
        }

        try pickOnlyLocalFixture()
        let stagedMetadata = XCTAttachment(string: material.staticTexts.allElementsBoundByIndex.map(\.label).joined(separator: " | "))
        stagedMetadata.name = "report-staged-system-filename-MIME-and-size"
        stagedMetadata.lifetime = .keepAlways
        add(stagedMetadata)
        material.tap()
        let closePreview = app.buttons["report.preview.close"]
        XCTAssertTrue(closePreview.waitForExistence(timeout: 8), app.debugDescription)
        XCTAssertGreaterThanOrEqual(closePreview.frame.width, 64)
        XCTAssertLessThanOrEqual(closePreview.frame.height, 48)
        // 验证系统已加载图片画布，不把预览容器出现当成材料已显示。
        let previewCanvas = app.otherElements["com.apple.paper.imageCanvasElementView"]
        XCTAssertTrue(previewCanvas.waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertTrue(previewCanvas.images.firstMatch.waitForExistence(timeout: 10))
        XCTAssertGreaterThan(previewCanvas.frame.width, 100)
        XCTAssertGreaterThan(previewCanvas.frame.height, 100)
        let previewTree = XCTAttachment(string: app.debugDescription)
        previewTree.name = "report-QuickLook-rendered-accessibility-tree"
        previewTree.lifetime = .keepAlways
        add(previewTree)
        let preview = XCTAttachment(screenshot: app.screenshot())
        preview.name = "report-private-copy-QuickLook-after-PHPicker-dismissed"
        preview.lifetime = .keepAlways
        add(preview)
        closePreview.tap()
        XCTAssertTrue(material.waitForExistence(timeout: 5))
        material.swipeLeft()
        let remove = app.buttons["删除"].firstMatch
        XCTAssertTrue(remove.waitForExistence(timeout: 5))
        remove.tap()
        XCTAssertTrue(material.waitForNonExistence(timeout: 5))
        let removed = XCTAttachment(screenshot: app.screenshot())
        removed.name = "report-selected-material-removed-before-submit"
        removed.lifetime = .keepAlways
        add(removed)

        // Re-select the same sole fixture so demo submission still validates one actual material.
        try pickOnlyLocalFixture()
        let submit = app.buttons["提交举报"]
        XCTAssertTrue(submit.waitForExistence(timeout: 5))
        if !submit.isHittable {
            app.scrollViews.firstMatch.swipeUp()
        }
        submit.tap()
        let confirmation = app.alerts["提交举报"]
        XCTAssertTrue(confirmation.waitForExistence(timeout: 5))
        XCTAssertTrue(confirmation.staticTexts["举报材料 · 1 / 0"].exists)
        confirmation.buttons["继续"].tap()
        let completed = app.alerts["完成"]
        XCTAssertTrue(completed.waitForExistence(timeout: 10), app.debugDescription)
        XCTAssertFalse(completed.staticTexts["演示空间"].exists)
        let success = XCTAttachment(screenshot: app.screenshot())
        success.name = "demo-report-one-real-material-submitted-without-formal-API"
        success.lifetime = .keepAlways
        add(success)
        completed.buttons["完成"].tap()
        XCTAssertTrue(material.waitForNonExistence(timeout: 5))
        let submitted = app.buttons["举报已受理"]
        XCTAssertTrue(submitted.waitForExistence(timeout: 5))
        XCTAssertFalse(submitted.isEnabled)
    }
    func testDemoInspectReportPhotoFixtureOnly() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "light"]
        app.launch()
        defer {
            app.terminate()
        }
        let contact = app.tables.cells["conversation.lin"]
        XCTAssertTrue(contact.waitForExistence(timeout: 10))
        contact.tap()
        app.buttons["聊天设置"].tap()
        app.buttons["举报"].tap()
        let addMedia = app.buttons["添加图片或视频"]
        XCTAssertTrue(addMedia.waitForExistence(timeout: 5))
        addMedia.tap()
        XCTAssertTrue(app.images.matching(identifier: "PXGGridLayout-Info").firstMatch.waitForExistence(timeout: 10), app.debugDescription)
        let picture = XCTAttachment(screenshot: app.screenshot())
        picture.name = "picker-inspection-existing-stock-and-new-AppIcon-no-selection"
        picture.lifetime = .keepAlways
        add(picture)
        let tree = XCTAttachment(string: app.debugDescription)
        tree.name = "picker-inspection-accessibility-tree-no-selection"
        tree.lifetime = .keepAlways
        add(tree)
    }


    func testAuthSettingsPasswordEyesAndRememberLayout() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "dark"]
        app.launch()
        defer {
            app.terminate()
        }
        let environment = app.buttons["demo.environment"]
        XCTAssertTrue(environment.waitForExistence(timeout: 10))
        environment.tap()
        if app.alerts.firstMatch.waitForExistence(timeout: 2) {
            let alert = app.alerts.firstMatch
            XCTAssertTrue(alert.buttons["取消"].exists)
            XCTAssertTrue(alert.buttons["跳转设置"].exists)
            // 提示须保持到用户作选择，不能数秒后自行消失。
            let retained = XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == true"), object: alert)
            XCTAssertEqual(XCTWaiter.wait(for: [retained], timeout: 2), .completed)
            alert.buttons["取消"].tap()
        }
        let gear = app.buttons["auth.settings"]
        XCTAssertTrue(gear.waitForExistence(timeout: 5))
        XCTAssertFalse(app.textFields["auth.address"].exists)
        XCTAssertTrue(app.staticTexts["Jobs荣誉出品"].exists)
        for title in ["重新验证身份", "核对原改密结果", "只核对原申请", "登录帮助与原操作核对", "已启用双重验证?"] {
            XCTAssertFalse(app.buttons[title].exists, title)
        }
        XCTAssertFalse(app.buttons["auth.help"].exists)
        XCTAssertFalse(app.textFields["auth.otp"].exists)
        let remember = app.buttons["auth.rememberPassword"]
        XCTAssertTrue(remember.isHittable)
        XCTAssertGreaterThanOrEqual(remember.frame.height, 44)
        let recover = app.buttons["auth.recover"]
        XCTAssertTrue(recover.isHittable)
        XCTAssertLessThan(remember.frame.midX, recover.frame.midX)
        let heading = app.staticTexts["auth.cardTitle"]
        let cardScroll = app.scrollViews["auth.cardScroll"]
        XCTAssertTrue(cardScroll.exists)
        let headingY = heading.frame.minY
        cardScroll.swipeUp()
        XCTAssertEqual(heading.frame.minY, headingY, accuracy: 1)
        cardScroll.swipeDown()
        XCTAssertEqual(heading.frame.minY, headingY, accuracy: 1)
        let password = app.secureTextFields["auth.password"]
        XCTAssertTrue(password.isHittable)
        let passwordEye = app.buttons["auth.password.visibility"]
        XCTAssertGreaterThanOrEqual(passwordEye.frame.width, 44)
        XCTAssertGreaterThanOrEqual(passwordEye.frame.height, 44)
        XCTAssertEqual(heading.frame.midX, password.frame.midX, accuracy: 3)
        XCTAssertLessThan(heading.frame.height, 34)
        password.tap()
        password.typeText("Fixture123!")
        app.buttons["auth.password.visibility"].tap()
        XCTAssertTrue(app.textFields["auth.password"].waitForExistence(timeout: 3))
        XCTAssertEqual(app.textFields["auth.password"].value as? String, "Fixture123!")
        app.buttons["auth.password.visibility"].tap()
        XCTAssertTrue(app.secureTextFields["auth.password"].exists)
        heading.tap()
        remember.tap()
        XCTAssertTrue(remember.isSelected)
        remember.tap()
        XCTAssertFalse(remember.isSelected)
        gear.tap()
        XCTAssertTrue(app.textFields["auth.address"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.switches["auth.settings.demo"].exists)
        XCTAssertTrue(app.buttons["auth.settings.reauthenticate"].exists)
        let settingsCapture = XCTAttachment(screenshot: app.screenshot())
        settingsCapture.name = "auth-settings-separate-connection-and-demo-switch"
        settingsCapture.lifetime = .keepAlways
        add(settingsCapture)
        app.navigationBars.buttons.firstMatch.tap()
        app.buttons["auth.modeSwitch"].tap()
        let confirmation = app.secureTextFields["auth.confirmPassword"]
        XCTAssertTrue(confirmation.waitForExistence(timeout: 5))
        XCTAssertTrue(app.staticTexts["Jobs荣誉出品"].exists)
        let registrationUsername = app.textFields["auth.username"]
        registrationUsername.tap()
        registrationUsername.typeText("ui-fixture\n")
        let registrationPassword = app.secureTextFields["auth.password"]
        XCTAssertTrue(registrationPassword.isHittable)
        XCTAssertLessThanOrEqual(registrationPassword.frame.maxY, app.keyboards.firstMatch.frame.minY + 1)
        registrationPassword.typeText("X\n")
        XCTAssertTrue(confirmation.isHittable)
        XCTAssertLessThanOrEqual(confirmation.frame.maxY, app.keyboards.firstMatch.frame.minY + 1)
        confirmation.typeText("Fixture123!X\n")
        XCTAssertTrue(app.keyboards.firstMatch.waitForNonExistence(timeout: 5))
        app.buttons["auth.confirmPassword.visibility"].tap()
        XCTAssertTrue(app.textFields["auth.confirmPassword"].exists)
        let formCapture = XCTAttachment(screenshot: app.screenshot())
        formCapture.name = "auth-frosted-card-photo-background-centered-register-eyes"
        formCapture.lifetime = .keepAlways
        add(formCapture)
    }

    func testDemoOwnProfileAvatarAndUnifiedEmptyButton() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "dark"]
        app.launch()
        defer {
            app.terminate()
        }
        let tabs = app.tabBars.firstMatch
        XCTAssertTrue(tabs.waitForExistence(timeout: 10))
        tabs.buttons.element(boundBy: 2).tap()
        app.tables.cells["personal.saved"].tap()
        let empty = app.buttons["empty.state"]
        XCTAssertTrue(empty.waitForExistence(timeout: 5))
        XCTAssertTrue(empty.isHittable)
        XCTAssertTrue(empty.label.contains("这里还没有内容"))
        XCTAssertTrue(empty.label.contains("重新加载"))
        empty.tap()
        XCTAssertTrue(empty.waitForExistence(timeout: 3))
        let capture = XCTAttachment(screenshot: app.screenshot())
        capture.name = "dark-saved-single-image-title-subtitle-empty-button"
        capture.lifetime = .keepAlways
        add(capture)
        app.navigationBars.buttons.firstMatch.tap()
        app.tables.cells["personal.profile"].tap()
        XCTAssertTrue(app.textFields["profile.displayName"].waitForExistence(timeout: 5))
        XCTAssertTrue(app.textViews["profile.signature"].exists)
        XCTAssertFalse(app.buttons["静态头像"].exists)
        XCTAssertFalse(app.buttons["refresh"].exists)
        let avatar = app.descendants(matching: .any).matching(identifier: "avatar.fallback").firstMatch
        XCTAssertTrue(avatar.isHittable)
        avatar.tap()
        XCTAssertTrue(app.buttons["avatar.choose"].waitForExistence(timeout: 5))
        app.navigationBars.buttons.firstMatch.tap()
        let scroll = app.scrollViews.firstMatch
        let save = app.buttons["profile.save"]
        for _ in 0..<6 {
            if save.exists, save.isHittable,
                save.frame.minY >= scroll.frame.minY, save.frame.maxY <= scroll.frame.maxY - 4 {
                break
            }
            scroll.swipeUp()
        }
        XCTAssertTrue(save.isHittable)
        XCTAssertGreaterThanOrEqual(save.frame.minY, scroll.frame.minY)
        XCTAssertLessThanOrEqual(save.frame.maxY, scroll.frame.maxY - 4)
        for key in ["profileUID", "profileRegisteredAt", "profileEmail", "profilePhone"] {
            XCTAssertTrue(app.staticTexts["profile." + key].exists)
        }
        let profileCapture = XCTAttachment(screenshot: app.screenshot())
        profileCapture.name = "own-profile-readonly-facts-signature-bottom-save"
        profileCapture.lifetime = .keepAlways
        add(profileCapture)
    }


    func testNativeMacDemoSwitchPrefillsAndAuthenticatesThroughServer() {
        continueAfterFailure = false
        let app = XCUIApplication()
        app.launchArguments = ["--demo", "--language", "zh-Hans", "--theme", "dark"]
        app.launch()
        defer {
            app.terminate()
        }
        let environment = app.buttons["demo.environment"]
        XCTAssertTrue(environment.waitForExistence(timeout: 10))
        environment.tap()
        if app.alerts.firstMatch.waitForExistence(timeout: 2) {
            let warning = app.alerts.firstMatch
            XCTAssertTrue(warning.buttons["取消"].exists)
            XCTAssertTrue(warning.buttons["跳转设置"].exists)
            warning.buttons["跳转设置"].tap()
        } else {
            app.buttons["auth.settings"].tap()
        }
        let demo = app.switches["auth.settings.demo"]
        XCTAssertTrue(demo.waitForExistence(timeout: 5))
        XCTAssertEqual(demo.value as? String, "0")
        demo.tap()
        let ready = XCTNSPredicateExpectation(
            predicate: NSPredicate(format: "enabled == YES AND value == '1'"), object: demo)
        XCTAssertEqual(XCTWaiter.wait(for: [ready], timeout: 150), .completed)
        XCTAssertTrue(app.textFields["auth.address"].exists)
        app.navigationBars.buttons.firstMatch.tap()
        let username = app.textFields["auth.username"]
        let password = app.textFields["auth.password"]
        XCTAssertTrue(username.waitForExistence(timeout: 5))
        XCTAssertFalse((username.value as? String ?? "").isEmpty)
        XCTAssertTrue(password.exists, "The dedicated demo candidate must be visible plaintext")
        XCTAssertFalse((password.value as? String ?? "").isEmpty)
        XCTAssertFalse(app.buttons["auth.rememberPassword"].isEnabled)
        app.buttons["auth.primary"].tap()
        let tabs = app.tabBars.firstMatch
        XCTAssertTrue(tabs.waitForExistence(timeout: 30))
        XCTAssertEqual(tabs.buttons.count, 3)
        XCTAssertTrue(app.buttons["demo.environment"].exists)
        XCTAssertFalse(app.staticTexts["演示空间"].exists)
        let contact = app.tables.cells.matching(NSPredicate(format: "label CONTAINS %@", "小林")).firstMatch
        XCTAssertTrue(contact.waitForExistence(timeout: 15))
        contact.tap()
        XCTAssertTrue(app.navigationBars.staticTexts["小林"].waitForExistence(timeout: 5))
        XCTAssertFalse(tabs.isHittable)
        let capture = XCTAttachment(screenshot: app.screenshot())
        capture.name = "native-Mac-demo-normal-API-login-contacts-chat-and-single-floating-exit"
        capture.lifetime = .keepAlways
        add(capture)
        app.buttons["demo.environment"].tap()
        XCTAssertTrue(app.buttons["auth.primary"].waitForExistence(timeout: 10))
        XCTAssertFalse(app.tabBars.firstMatch.exists)
        XCTAssertFalse(app.buttons["demo.environment"].exists)
    }

}
