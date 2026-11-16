//
//  IMNotifications.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import JobsByUIKit
import JobsIMCore
import UIKit
import UserNotifications

@MainActor
final class IMNotifications: NSObject, UNUserNotificationCenterDelegate {
    static let shared = IMNotifications()
    private var delivered: Set<String> = []
    func configure() {
        UNUserNotificationCenter.current().delegate = self
    }
    func enabled(namespace: IMNamespace) -> Bool {
        return UserDefaults.standard.bool(forKey: "notifications:" + namespace.key)
    }
    func request(namespace: IMNamespace) async throws {
        let allowed = try await UNUserNotificationCenter.current().requestAuthorization(options: [
            .alert, .sound, .badge,
        ])
        UserDefaults.standard.set(allowed, forKey: "notifications:" + namespace.key)
    }
    func deliver(_ message: IMJSON, namespace: IMNamespace) {
        guard enabled(namespace: namespace), namespace == IMSession.shared.namespace, !namespace.demo else {
            return
        }
        let id = "JobsIM:" + namespace.key + ":" + message["conversationId"].text + ":" + message["id"].text
        guard delivered.insert(id).inserted else {
            return
        }
        let content = UNMutableNotificationContent.jobsMake { content in
            content.title = "JobsIM"
            content.body =
                UserDefaults.standard.bool(forKey: "notificationPreview:" + namespace.key)
                    && !IMForwardService.isForward(message)
                ? message["text"].text : IMStrings.text("notificationMessage")
            if message["mentionedMe"] == .bool(true) {
                content.body = IMStrings.text("mentionedYou") + " · " + content.body
            }
            content.userInfo = ["namespace": namespace.key, "conversationId": message["conversationId"].text]
        }
        let request = UNNotificationRequest.make(identifier: id, content: content)
        UNUserNotificationCenter.current().add(request)
    }
    func clearAll(namespace: IMNamespace) {
        clearPrefix("JobsIM:" + namespace.key + ":")
    }
    func clearAll(namespaceKeys: [String]) {
        for key in namespaceKeys {
            clearPrefix("JobsIM:" + key + ":")
        }
    }
    func clear(conversation: String, namespace: IMNamespace) {
        clearPrefix("JobsIM:" + namespace.key + ":" + conversation + ":")
    }
    private func clearPrefix(_ prefix: String) {
        Task {
            let center = UNUserNotificationCenter.current()
            let pending = await center.pendingNotificationRequests().filter {
                $0.identifier.hasPrefix(prefix)
            }
            center.removePendingNotificationRequests(withIdentifiers: pending.map(\.identifier))
            let shown = await center.deliveredNotifications().filter {
                $0.request.identifier.hasPrefix(prefix)
            }
            center.removeDeliveredNotifications(withIdentifiers: shown.map { $0.request.identifier })
        }
    }
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        let namespace = notification.request.content.userInfo["namespace"] as? String
        return await presentation(namespace)
    }
    private func presentation(_ namespace: String?) -> UNNotificationPresentationOptions {
        return namespace == IMSession.shared.namespace.key && enabled(namespace: IMSession.shared.namespace)
            ? [.banner, .list] : []
    }
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse
    ) async {
        let namespace = response.notification.request.content.userInfo["namespace"] as? String
        let id = response.notification.request.content.userInfo["conversationId"] as? String
        await open(namespace: namespace, id: id)
    }
    private func open(namespace: String?, id: String?) async {
        let session = IMSession.shared
        guard namespace == session.namespace.key, !session.demo, session.token != nil, let id else {
            return
        }
        let conversations = await session.conversationSnapshot()
        guard namespace == session.namespace.key,
            let conversation = conversations.first(where: { $0["id"].text == id }),
            let app = UIApplication.shared.delegate as? AppDelegate,
            let tabs = app.window?.rootViewController as? UITabBarController,
            let nav = tabs.viewControllers?.first as? UINavigationController
        else {
            return
        }
        tabs.selectedIndex = 0
        nav.popToRootViewController(animated: false)
        nav.pushViewController(IMChatVC(conversation: conversation), animated: true)
    }
}
