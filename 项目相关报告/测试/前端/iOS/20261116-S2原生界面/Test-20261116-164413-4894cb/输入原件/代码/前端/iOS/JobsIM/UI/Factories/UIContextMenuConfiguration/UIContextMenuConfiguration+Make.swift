//
//  UIContextMenuConfiguration+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

extension UIContextMenuConfiguration {
    static func make(actions: @escaping UIContextMenuActionProvider) -> UIContextMenuConfiguration {
        return UIContextMenuConfiguration(identifier: nil, previewProvider: nil, actionProvider: actions)
    }
}
