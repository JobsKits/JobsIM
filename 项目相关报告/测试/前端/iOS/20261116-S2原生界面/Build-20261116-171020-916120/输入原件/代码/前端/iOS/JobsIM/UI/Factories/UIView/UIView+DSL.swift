//
//  UIView+DSL.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

extension UIView {

    @discardableResult
    func byEndEditing(_ force: Bool) -> Self {
        endEditing(force)
        return self
    }
}
