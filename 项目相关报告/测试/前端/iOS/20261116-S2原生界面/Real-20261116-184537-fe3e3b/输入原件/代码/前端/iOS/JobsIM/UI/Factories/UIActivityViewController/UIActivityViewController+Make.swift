//
//  UIActivityViewController+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsSwiftBaseDefines
import JobsSwiftDSL

extension UIActivityViewController {

    static func make(_ items: [Any]) -> UIActivityViewController {
        UIActivityViewController(activityItems: items, applicationActivities: nil)
    }
}
