//
//  UITableView+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsSwiftBaseDefines
import JobsSwiftDSL

extension UITableView {

    static func make(style: UITableView.Style = .plain) -> UITableView {
        UITableView(frame: .zero, style: style)
    }
}
