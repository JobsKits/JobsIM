//
//  NSAttributedString+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation

extension NSAttributedString {

    static func make(string: String, attributes: [NSAttributedString.Key: Any]) -> NSAttributedString {
        return NSAttributedString(string: string, attributes: attributes)
    }
}
