//
//  NSDecimalNumber+Make.swift
//  JobsIMCore
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import Foundation

public extension NSDecimalNumber {
    static func make(decimal: Decimal) -> NSDecimalNumber {
        return NSDecimalNumber(decimal: decimal)
    }
}
