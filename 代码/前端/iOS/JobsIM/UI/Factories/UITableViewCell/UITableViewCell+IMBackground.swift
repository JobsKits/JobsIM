//
//  UITableViewCell+IMBackground.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import UIKit
import JobsSwiftBaseDefines

extension UITableViewCell {
    @MainActor
    @discardableResult
    func byIMThemeAccessoryType(_ value: AccessoryType) -> Self {
        // 模板箭头绑定会把整行锁成默认浅色；本App的行应继承所在window的有效主题。
        JobsThemeCenter.shared.unbind(self, slot: "UITableViewCell.disclosureAppearance")
        accessoryType = value
        overrideUserInterfaceStyle = .unspecified
        setNeedsUpdateConfiguration()
        return self
    }

    @MainActor
    @discardableResult
    func byIMThemeBackground(normal: UIColor, selected: UIColor) -> Self {
        // UIKit's default list background is separate from UIView.backgroundColor.
        automaticallyUpdatesBackgroundConfiguration = false
        configurationUpdateHandler = { cell, state in
            var background = UIBackgroundConfiguration.clear()
            background.backgroundColor = state.isSelected || state.isHighlighted ? selected : normal
            cell.backgroundConfiguration = background
        }
        var background = UIBackgroundConfiguration.clear()
        background.backgroundColor = isSelected || isHighlighted ? selected : normal
        backgroundConfiguration = background
        return self
    }
}
