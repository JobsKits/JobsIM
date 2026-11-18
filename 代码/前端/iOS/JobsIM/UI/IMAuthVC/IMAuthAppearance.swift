//
//  IMAuthAppearance.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsSwiftBaseDefines
import JobsSwiftDSL
import UIKit

@MainActor
enum IMAuthAppearance {

    static let input = UIColor.make(light: 0xF2F6F3, dark: 0x14251C)
    static let outline = UIColor.make(light: 0xA9BEB1, dark: 0x658776)
    static let secondary = UIColor.make(light: 0x485B50, dark: 0xBACBC0)
    static let accent = UIColor.make(light: 0x1B5D47, dark: 0x8DDFC0)
    static let smallIconConfiguration = UIImage.SymbolConfiguration(pointSize: 19, weight: .regular, scale: .medium)
    private static let glassSecondary = UIColor.make(light: 0x17201B, dark: 0xBACBC0)

    static func font(_ size: CGFloat, weight: UIFont.Weight = .regular) -> UIFont {
        let adjustment = CGFloat(UserDefaults.standard.integer(forKey: "fontScale"))
        return JobsFont.systemFont(ofSize: size + adjustment, weight: weight)
    }

    static func label(_ text: String, size: CGFloat = 12, strong: Bool = false) -> UILabel {
        return UILabel.jobsMake {
            $0.byText(text)
                .byDynamicFont(font(size, weight: strong ? .semibold : .regular), style: .subheadline)
                .byTextColor(strong ? IMUI.foreground : glassSecondary)
                .byNumberOfLines(0)
        }
    }

    static func button(
        _ title: String, primary: Bool = false, compact: Bool = false, action: @escaping () -> Void
    ) -> UIButton {
        let background = UIBackgroundConfiguration.clear()
            .byBackgroundColor(primary ? IMUI.greenFill : JobsCor.clear)
            .byCornerRadius(primary ? 12 : 0)
        let configuration = UIButton.Configuration.plain()
            .byBackground(background)
            .byBaseForegroundColor(primary ? JobsCor.white : accent)
            .byContentInsets(NSDirectionalEdgeInsets(top: compact ? 4 : 10, leading: 0, bottom: compact ? 4 : 10, trailing: 0))
            .byImagePadding(5)
        return UIButton.sys()
            .byConfiguration(configuration)
            .byTitle(title)
            .byTitleColor(primary ? JobsCor.white : accent)
            .byTitleColor(primary ? JobsCor.white : accent, for: .disabled)
            .byTitleColor(primary ? JobsCor.white : accent, for: .selected)
            .byTitleColor(primary ? JobsCor.white : accent, for: .highlighted)
            .byTitleFont(font(compact ? 12 : 14, weight: primary ? .semibold : .regular))
            .byNumberOfLines(0)
            .onTap { _ in
                action()
            }
    }
}
