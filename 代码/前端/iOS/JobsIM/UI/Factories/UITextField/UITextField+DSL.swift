//
//  UITextField+DSL.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import UIKit

extension UITextField {

    @discardableResult
    func bySelectedTextRange(_ range: UITextRange?) -> Self {
        selectedTextRange = range
        return self
    }
}
