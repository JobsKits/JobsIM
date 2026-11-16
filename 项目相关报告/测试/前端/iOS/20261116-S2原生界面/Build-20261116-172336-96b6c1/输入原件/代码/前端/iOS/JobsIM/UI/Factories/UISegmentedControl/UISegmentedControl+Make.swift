//
//  UISegmentedControl+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsSwiftBaseDefines
import JobsSwiftDSL

extension UISegmentedControl {

    static func make(_ values: [String]) -> UISegmentedControl {
        let control = UISegmentedControl(items: values)
        control.selectedSegmentIndex = 0
        return control
    }
}
