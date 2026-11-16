//
//  IMCursor.swift
//  JobsIMCore
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation

public struct IMCursor: Sendable, Equatable {

    public let raw: String
    public let epoch: String
    public let sequence: String

    public init?(raw: String, expectedEpoch: String) {
        guard Self.isDecimal(expectedEpoch) else {
            return nil
        }
        let parts = raw.split(separator: ":", omittingEmptySubsequences: false)
        if raw == "0" {
            epoch = expectedEpoch
            sequence = "0"
        } else {
            guard parts.count == 2, String(parts[0]) == expectedEpoch,
                Self.isDecimal(String(parts[1]))
            else {
                return nil
            }
            epoch = expectedEpoch
            sequence = String(parts[1])
        }
        self.raw = raw
    }

    public func isAtLeast(_ previous: IMCursor) -> Bool {
        guard epoch == previous.epoch else {
            return false
        }
        return sequence.count > previous.sequence.count
            || (sequence.count == previous.sequence.count && sequence >= previous.sequence)
    }

    private static func isDecimal(_ value: String) -> Bool {
        return !value.isEmpty && (value == "0" || value.first != "0")
            && value.utf8.allSatisfy { $0 >= 48 && $0 <= 57 }
    }
}
