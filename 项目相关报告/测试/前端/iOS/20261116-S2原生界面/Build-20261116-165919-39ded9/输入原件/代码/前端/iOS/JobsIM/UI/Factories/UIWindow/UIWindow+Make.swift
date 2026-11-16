//
//  UIWindow+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsSwiftBaseDefines
import JobsSwiftDSL

extension UIWindow {

    static func make(scene: UIWindowScene) -> UIWindow {
        let window = UIWindow(windowScene: scene)
        window.tintColor = IMUI.green
        return window
    }
}
