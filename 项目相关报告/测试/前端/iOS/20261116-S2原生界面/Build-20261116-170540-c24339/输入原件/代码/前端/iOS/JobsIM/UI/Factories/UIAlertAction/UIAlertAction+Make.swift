//
//  UIAlertAction+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsSwiftBaseDefines
import JobsSwiftDSL

extension UIAlertAction {

    static func make(_ title: String, style: UIAlertAction.Style = .default, handler: @escaping () -> Void)
        -> UIAlertAction
    {
        UIAlertAction(title: title, style: style) { _ in
            handler()
        }
    }
}
