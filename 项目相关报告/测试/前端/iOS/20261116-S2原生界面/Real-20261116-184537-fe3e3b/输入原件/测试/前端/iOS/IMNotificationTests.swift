//
//  IMNotificationTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import XCTest
import UserNotifications

import JobsIMCore
@testable import JobsIM

@MainActor
final class IMNotificationTests: XCTestCase {
    func testLocalDeliveryHidesBodyAndRejectsOtherNamespace() async throws {
        let center = UNUserNotificationCenter.current()
        let settings = await center.notificationSettings()
        guard settings.authorizationStatus == .authorized || settings.authorizationStatus == .provisional
        else {
            throw XCTSkip("Run native notification authorization UI first")
        }
        let ns = IMSession.shared.namespace
        guard !ns.demo else { throw XCTSkip("A formal fixture account is required") }
        try await IMNotifications.shared.request(namespace: ns)
        UserDefaults.standard.set(false, forKey: "notificationPreview:" + ns.key)
        let id = UUID().uuidString
        let message = IMJSON.object([
            "id": .string(id), "conversationId": .string("notification-fixture"),
            "text": .string("Private content must stay hidden"),
        ])
        IMNotifications.shared.deliver(message, namespace: ns)
        var delivered: [UNNotification] = []
        for _ in 0..<25 {
            delivered = await center.deliveredNotifications().filter {
                $0.request.identifier.hasSuffix(":" + id)
            }
            if !delivered.isEmpty { break }
            try await Task.sleep(for: .milliseconds(100))
        }
        let notification = try XCTUnwrap(delivered.first)
        XCTAssertEqual(notification.request.content.body, IMStrings.text("notificationMessage"))
        XCTAssertFalse(notification.request.content.body.contains("Private content"))
        let foreign = IMNamespace(profile: "foreign", account: ns.account, demo: false)
        UserDefaults.standard.set(true, forKey: "notifications:" + foreign.key)
        IMNotifications.shared.deliver(message, namespace: foreign)
        let all = await center.deliveredNotifications()
        XCTAssertFalse(all.contains { $0.request.identifier.hasPrefix("JobsIM:" + foreign.key) })
        IMNotifications.shared.clear(conversation: "notification-fixture", namespace: ns)
    }
}
