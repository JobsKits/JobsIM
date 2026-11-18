//
//  IMReportPickerDraftTests.swift
//  JobsIMUITestsDraft
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import XCTest

// Draft only. The dedicated simulator must contain exactly one root-provided AppIcon test PNG.
@MainActor
final class IMReportPickerDraftTests: XCTestCase {
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
        let material = app.tables.cells.matching(NSPredicate(format: "label CONTAINS %@", "image/")).firstMatch

        func pickOnlyLocalFixture() throws {
            let addMedia = app.buttons["添加图片或视频"]
            XCTAssertTrue(addMedia.waitForExistence(timeout: 5))
            if !addMedia.isHittable {
                app.scrollViews.firstMatch.swipeUp()
            }
            addMedia.tap()
            // Expected native PHPicker AX shape; confirm it from the first actual picker snapshot.
            let grid = app.collectionViews.firstMatch
            XCTAssertTrue(grid.waitForExistence(timeout: 8), app.debugDescription)
            let assets = grid.cells.allElementsBoundByIndex.filter {
                $0.isHittable
            }
            XCTAssertEqual(assets.count, 1,
                "Stop if the dedicated photo library or picker AX differs; never select an unknown photo")
            let asset = try XCTUnwrap(assets.count == 1 ? assets.first : nil)
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
            XCTAssertTrue(material.label.range(of: "[1-9][0-9]* B", options: .regularExpression) != nil,
                "The material must show a nonzero staged-file size")
        }

        try pickOnlyLocalFixture()
        let stagedMetadata = XCTAttachment(string: material.label)
        stagedMetadata.name = "report-staged-system-filename-MIME-and-size"
        stagedMetadata.lifetime = .keepAlways
        add(stagedMetadata)
        material.tap()
        let closePreview = app.buttons.matching(
            NSPredicate(format: "label IN %@", ["完成", "Done", "关闭", "Close"])
        ).firstMatch
        XCTAssertTrue(closePreview.waitForExistence(timeout: 8), app.debugDescription)
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
        XCTAssertTrue(completed.staticTexts["演示空间"].exists)
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
}
