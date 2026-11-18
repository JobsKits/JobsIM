//
//  UIViewController+IMNavigationAppearance.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import GKNavigationBarSwift
import JobsByUIKit
import JobsSwiftDSL
import UIKit

extension UIViewController {
    @MainActor
    @discardableResult
    func byIMNavigationAppearance(hidesHeader: Bool) -> Self {
        let traits = traitCollection
        let background = IMUI.background.resolvedColor(with: traits)
        let foreground = IMUI.foreground.resolvedColor(with: traits)
        let accent = IMUI.green.resolvedColor(with: traits)
        let dark = traits.userInterfaceStyle == .dark
        gk_navBackgroundImage = nil
        gk_darkNavBackgroundImage = nil
        gk_navShadowImage = nil
        gk_darkNavShadowImage = nil
        gk_navBackgroundColor = background
        gk_navShadowColor = IMUI.separator.resolvedColor(with: traits)
        gk_navTitleColor = foreground
        gk_statusBarStyle = dark ? .lightContent : .darkContent

        // GK's setters reset appearance fields; install the complete snapshot after those setters finish.
        let appearance = UINavigationBarAppearance.makeJobsIM(
            traits: traits, titleFont: gk_navTitleFont ?? IMUI.font(.headline))
        var bars: [UINavigationBar] = [gk_navigationBar]
        if let native = navigationController?.navigationBar {
            bars.append(native)
        }
        for bar in bars {
            bar.byBarStyle(dark ? .black : .default)
                .byTranslucent(false)
                .byBarTintColor(background)
                .byTintColor(accent)
                .byBackgroundColor(background)
                .byTitleTextAttributes(appearance.titleTextAttributes)
                .byLargeTitleTextAttributes(appearance.largeTitleTextAttributes)
                .byUnifiedAppearance(appearance)
        }
        let items =
            [gk_navLeftBarButtonItem, gk_navRightBarButtonItem].compactMap { $0 }
            + (gk_navLeftBarButtonItems ?? []) + (gk_navRightBarButtonItems ?? [])
        for item in items {
            item.tintColor = accent
            if let custom = item.customView {
                applyIMNavigationButtons(in: custom, traits: traits)
            }
        }
        gk_navigationBar.byHidden(hidesHeader)
        navigationController?.byNavBarHidden(true)
        setNeedsStatusBarAppearanceUpdate()
        return self
    }

    @MainActor
    private func applyIMNavigationButtons(in view: UIView, traits: UITraitCollection) {
        if let button = view as? UIButton {
            let accent = IMUI.green.resolvedColor(with: traits)
            button.byTintColor(accent)
                .byTitleColor(accent)
                .byTitleColor(accent, for: .highlighted)
                .byTitleColor(accent, for: .selected)
                .byTitleColor(IMUI.secondary.resolvedColor(with: traits), for: .disabled)
            return
        }
        // Traverse only supplied custom views, never UIKit's private navigation subview hierarchy.
        for child in view.subviews {
            applyIMNavigationButtons(in: child, traits: traits)
        }
    }
}
