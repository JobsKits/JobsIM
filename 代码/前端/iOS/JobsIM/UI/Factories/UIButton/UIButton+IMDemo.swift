//
//  UIButton+IMDemo.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import UIKit

extension UIButton {
    @MainActor
    @discardableResult
    func byIMDemoAccessibility() -> Self {
        accessibilityLabel = IMStrings.text("demoEnvironment")
        accessibilityHint = IMStrings.text("demoEnvironmentHint")
        return self
    }
}
