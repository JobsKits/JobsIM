//
//  SceneDelegate.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsByUIKit
import JobsSwiftDSL

@MainActor
final class SceneDelegate: UIResponder, UIWindowSceneDelegate {

    var window: UIWindow?

    func scene(
        _ scene: UIScene, willConnectTo session: UISceneSession,
        options connectionOptions: UIScene.ConnectionOptions
    ) {
        guard let scene = scene as? UIWindowScene, let app = UIApplication.shared.delegate as? AppDelegate
        else {
            return
        }
        let window = UIWindow.make(scene: scene)
        self.window = window
        app.window = window
        app.showRoot()
        window.byMakeKeyAndVisible()
        // 首次显示会在窗口中安装根视图，之后再将浮标置于最上层。
        app.refreshDemoOverlay()
    }
    func sceneDidBecomeActive(_ scene: UIScene) {
        IMSyncCoordinator.shared.start()
    }
    func sceneWillResignActive(_ scene: UIScene) {
        IMSyncCoordinator.shared.stop()
    }

}
