//
//  UITabBarItem+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsSwiftBaseDefines
import JobsSwiftDSL

extension UITabBarItem {

    static func make(_ title: String, symbol: String, tag: Int) -> UITabBarItem {
        UITabBarItem(title: title, image: symbol.sysImg, tag: tag)
    }
}
