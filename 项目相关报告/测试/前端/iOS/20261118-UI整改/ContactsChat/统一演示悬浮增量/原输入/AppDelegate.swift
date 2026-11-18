//
//  AppDelegate.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsByUIKit
import JobsSwiftBaseDefines
import JobsSwiftDSL

import GKNavigationBarSwift

@main
@MainActor
final class AppDelegate: UIResponder, UIApplicationDelegate {

    var window: UIWindow?

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        GKNavigationBarConfigure
            .bySetupDefault()
            .byAwake()
            .byBackground(IMUI.background)
            .byTitleCor(IMUI.foreground)
            .byTitleFont(JobsFont.systemFont(ofSize: 18, weight: .semibold))
        #if DEBUG
            let arguments = ProcessInfo.processInfo.arguments
            // UI测试仅选择已由真实API建立、凭据已进Keychain的隔离连接档案。
            if let index = arguments.firstIndex(of: "--ui-test-profile"),
                arguments.indices.contains(index + 1),
                let snapshot = Self.nativeUITestFixture(arguments[index + 1])?["snapshot"] as? [String: Any]
                    ?? UserDefaults.standard.dictionary(forKey: "ui-fixture:" + arguments[index + 1])
            {
                for (key, value) in snapshot {
                    UserDefaults.standard.set(value, forKey: key)
                }
            }
            if let index = arguments.firstIndex(of: "--language"), arguments.indices.contains(index + 1) {
                UserDefaults.standard.set(arguments[index + 1], forKey: "language")
            }
            if let index = arguments.firstIndex(of: "--theme"), arguments.indices.contains(index + 1) {
                UserDefaults.standard.set(arguments[index + 1], forKey: "theme")
            }
        #endif
        IMNotifications.shared.configure()
        IMSession.shared.changed = { [weak self] in
            self?.showRoot()
        }
        #if DEBUG
        if ProcessInfo.processInfo.arguments.contains("--demo") {
            IMSession.shared.enterDemo()
        }
        #endif
        return true
    }

    #if DEBUG
        static func nativeUITestFixtureURL(_ identifier: String) -> URL? {
            guard identifier.hasPrefix("s2-"),
                let run = UUID(uuidString: String(identifier.dropFirst(3))),
                identifier == "s2-" + run.uuidString.lowercased()
            else {
                return nil
            }
            return FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
                .appendingPathComponent("NativeSelfStateEvidence")
                .appendingPathComponent(run.uuidString.lowercased() + "-ui.plist")
        }

        static func nativeUITestFixture(_ identifier: String) -> [String: Any]? {
            // 跨测试进程用原子文件交接公开档案，凭据仍只从原Keychain与GRDB恢复。
            guard let url = nativeUITestFixtureURL(identifier),
                let bytes = try? Data(contentsOf: url), bytes.count <= 256 * 1024,
                let record = try? PropertyListSerialization.propertyList(from: bytes, format: nil) as? [String: Any],
                record["identifier"] as? String == identifier,
                record["schemaVersion"] as? Int == 1,
                let profile = record["profile"] as? String, profile.hasPrefix("s2-ios-"),
                UUID(uuidString: String(profile.dropFirst(7))) != nil,
                let conversation = record["conversation"] as? String, UUID(uuidString: conversation) != nil,
                let snapshot = record["snapshot"] as? [String: Any],
                snapshot["instanceProfile"] as? String == profile,
                let address = snapshot["instanceURL"] as? String,
                let parsed = URL(string: address), parsed.scheme == "https", parsed.host != nil,
                snapshot["instanceCA:" + address] is Data
            else {
                return nil
            }
            let keys: Set<String> = ["instanceURL", "instanceProfile", "instanceDeployment", "instanceTrustFingerprint",
                                     "user:" + profile, "epoch:" + profile, "instanceCA:" + address,
                                     "language", "theme", "fontScale"]
            let previousKeys: Set<String> = ["instanceURL", "instanceProfile", "instanceDeployment", "instanceTrustFingerprint",
                                             "language", "theme", "fontScale"]
            guard Set(snapshot.keys) == keys,
                let previous = record["previous"] as? [String: Any], Set(previous.keys).isSubset(of: previousKeys),
                Set(record.keys) == ["identifier", "schemaVersion", "profile", "conversation", "snapshot", "previous"]
            else {
                return nil
            }
            return record
        }
    #endif

    func showRoot() {
        guard let window else {
            return
        }
        window.byEndEditing(true)
        let mode = UserDefaults.standard.string(forKey: "theme") ?? "system"
        window.overrideUserInterfaceStyle = mode == "light" ? .light : mode == "dark" ? .dark : .unspecified
        JobsThemeCenter.shared.setStyle(window.traitCollection.userInterfaceStyle == .dark ? .dark : .light)
        let session = IMSession.shared
        IMSyncCoordinator.shared.start()
        if session.demo || session.token != nil {
            if let root = window.rootViewController as? IMRootVC, root.representsCurrentSession {
                root.refreshIdentity()
                return
            }
            let root = IMRootVC()
            if let current = window.rootViewController as? IMRootVC {
                root.preferredIndex = current.selectedIndex
                root.restoresSettings = (current.selectedViewController as? UINavigationController)?.topViewController is IMSettingsVC
            }
            window.byRootViewController(root)
        } else {
            if let navigation = window.rootViewController as? UINavigationController,
                navigation.viewControllers.first is IMAuthVC {
                return
            }
            window.byRootViewController(UINavigationController.make(IMAuthVC()))
        }
    }
}
