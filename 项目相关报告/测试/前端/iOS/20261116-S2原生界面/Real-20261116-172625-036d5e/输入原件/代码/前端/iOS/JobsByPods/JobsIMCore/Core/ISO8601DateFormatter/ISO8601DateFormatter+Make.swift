//
//  ISO8601DateFormatter+Make.swift
//  JobsIMCore
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import Foundation

extension ISO8601DateFormatter {
    public static func make(options: ISO8601DateFormatter.Options = [.withInternetDateTime])
        -> ISO8601DateFormatter
    {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = options
        return formatter
    }
}
