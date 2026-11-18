//
//  UIContextualAction+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import UIKit

extension UIContextualAction {
    @MainActor
    static func makeContactRemoval(_ action: @escaping (@escaping (Bool) -> Void) -> Void)
        -> UIContextualAction
    {
        return UIContextualAction(style: .destructive, title: IMStrings.text("deleteAction")) {
            _, _, completion in
            action(completion)
        }
    }
}
