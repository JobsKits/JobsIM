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
                let snapshot = UserDefaults.standard.dictionary(forKey: "ui-fixture:" + arguments[index + 1])
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
        if ProcessInfo.processInfo.arguments.contains("--demo") {
            IMSession.shared.enterDemo()
        }
        return true
    }

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
            let tabs: [(String, String, UIViewController)] = [
                ("messages", "bubble.left.and.bubble.right", IMListVC(kind: .conversations)),
                ("contacts", "person.2", IMListVC(kind: .contacts)),
                ("saved", "bookmark", IMListVC(kind: .saved)),
                ("settings", "slider.horizontal.3", IMSettingsVC()),
            ]
            let controllers = tabs.enumerated().map { index, item in
                let nav = UINavigationController.make(item.2)
                nav.tabBarItem = UITabBarItem.make(IMStrings.text(item.0), symbol: item.1, tag: index)
                return nav
            }
            let root = UITabBarController.jobsMake { tabs in
                tabs.viewControllers = controllers
                tabs.tabBar.tintColor = IMUI.green
            }
            window.byRootViewController(root)
        } else {
            window.byRootViewController(UINavigationController.make(IMAuthVC()))
        }
    }
}
