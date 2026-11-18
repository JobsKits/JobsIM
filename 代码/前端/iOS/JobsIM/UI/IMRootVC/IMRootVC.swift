//
//  IMRootVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftBaseDefines
import JobsSwiftDSL
import UIKit

@MainActor
final class IMRootVC: UITabBarController {
    override var childForStatusBarStyle: UIViewController? {
        return selectedViewController
    }

    var preferredIndex = 0
    var restoresSettings = false
    private let namespaceKey = IMSession.shared.namespace.key
    private let preferences = IMRootVC.preferenceKey
    private var avatarTask: Task<Void, Never>?

    private static var preferenceKey: String {
        [
            UserDefaults.standard.string(forKey: "language") ?? "zh-Hans",
            String(UserDefaults.standard.integer(forKey: "fontScale")),
        ].joined(separator: "|")
    }

    var representsCurrentSession: Bool {
        namespaceKey == IMSession.shared.namespace.key && preferences == Self.preferenceKey
    }

    override func viewDidLoad() {
        super.viewDidLoad()
        let contacts = IMContactsVC()
        let conversations = IMConversationsVC()
        let personal = IMPersonalCenterVC()
        let items: [(String, String, IMBaseVC)] = [
            ("contacts", "person.2", contacts),
            ("messages", "bubble.left.and.bubble.right", conversations),
            ("personalCenter", "person.crop.circle", personal),
        ]
        let controllers = items.enumerated().map { index, item in
            item.2.hidesScreenHeader = true
            let navigation = UINavigationController.make(item.2)
            navigation.byTabBarItem(
                UITabBarItem.make(IMStrings.text(item.0), symbol: item.1, tag: index)
                    .byTitleColor(IMUI.secondary).byTitleColor(IMUI.green, for: .selected))
            return navigation
        }
        byViewControllers(controllers)
            .bySelectedIndex(min(max(preferredIndex, 0), controllers.count - 1))
            .byTabBarTintColor(IMUI.green)
            .byTabBarBackgroundColor(IMUI.panel)
        view.byBackgroundColor(IMUI.background)
        refreshIdentity()
        if restoresSettings, let navigation = selectedViewController as? UINavigationController {
            navigation.pushViewController(IMSettingsVC().byHidesBottomBarWhenPushed(true), animated: false)
        }
    }

    func refreshIdentity() {
        guard let item = viewControllers?.last?.tabBarItem else {
            return
        }
        let session = IMSession.shared
        item.byTitle(
            session.user.firstText("username", "displayName").isEmpty
                ? IMStrings.text("personalCenter") : session.user.firstText("username", "displayName")
        )
        .byImage(UIImage.makeTabAvatar(UIImage.make(named: "Logo")))
        .bySelectedImage(item.image)
        avatarTask?.cancel()
        let namespace = session.namespace
        let path = session.user.firstText("avatarThumbnailURL", "avatarURL")
        guard !path.isEmpty else {
            return
        }
        avatarTask = Task { [weak self] in
            guard let image = await IMAvatarLoader.image(path, namespace: namespace), !Task.isCancelled,
                let self, self.representsCurrentSession, session.namespace == namespace,
                session.user.firstText("avatarThumbnailURL", "avatarURL") == path,
                let current = self.viewControllers?.last?.tabBarItem
            else {
                return
            }
            current.byImage(UIImage.makeTabAvatar(image)).bySelectedImage(current.image)
        }
    }
}

extension UIImage {
    fileprivate static func makeTabAvatar(_ image: UIImage?) -> UIImage? {
        guard let image else {
            return nil
        }
        let renderer = UIGraphicsImageRenderer.make(size: CGSize(width: 28, height: 28))
        return renderer.image { context in
            let bounds = CGRect(x: 0, y: 0, width: 28, height: 28)
            context.cgContext.addEllipse(in: bounds)
            context.cgContext.clip()
            let scale = max(28 / image.size.width, 28 / image.size.height)
            let size = CGSize(width: image.size.width * scale, height: image.size.height * scale)
            image.draw(
                in: CGRect(
                    x: (28 - size.width) / 2, y: (28 - size.height) / 2,
                    width: size.width, height: size.height))
        }.withRenderingMode(.alwaysOriginal)
    }
}

extension UIViewController {
    @discardableResult
    fileprivate func byTabBarItem(_ value: UITabBarItem) -> Self {
        tabBarItem = value
        return self
    }
}

extension UITabBarItem {
    @discardableResult
    fileprivate func bySelectedImage(_ value: UIImage?) -> Self {
        selectedImage = value
        return self
    }
}
