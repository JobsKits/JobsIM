//
//  UIAlertController+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsSwiftBaseDefines
import JobsSwiftDSL

extension UIAlertController {

    static func make(title: String?, message: String?, style: UIAlertController.Style) -> UIAlertController {
        UIAlertController(title: title, message: message, preferredStyle: style)
    }
}
