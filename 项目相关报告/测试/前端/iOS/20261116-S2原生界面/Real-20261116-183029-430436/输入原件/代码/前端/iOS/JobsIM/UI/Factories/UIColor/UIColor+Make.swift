//
//  UIColor+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsSwiftBaseDefines
import JobsSwiftDSL

extension UIColor {

    static func make(hex: UInt32) -> UIColor {
        return UIColor(hex: hex)
    }

    static func make(light: UInt32, dark: UInt32) -> UIColor {
        UIColor { traits in
            UIColor.make(hex: traits.userInterfaceStyle == .dark ? dark : light)
        }
    }
}
