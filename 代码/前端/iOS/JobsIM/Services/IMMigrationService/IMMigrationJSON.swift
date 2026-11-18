//
//  IMMigrationJSON.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二。
//

import Foundation

// Hash the actual Go message objects inside the authenticated UTF-8 packet.
// Re-encoding a Swift dictionary cannot preserve Go's escaping and field order.
enum IMMigrationJSON {

    private enum Invalid: Error { case structure }

    static func messageRanges(_ data: Data) throws -> [Range<Int>] {
        return try data.withUnsafeBytes { raw in
            var parser = Reader(bytes: raw.bindMemory(to: UInt8.self))
            return try parser.root()
        }
    }

    private struct Reader {
        let bytes: UnsafeBufferPointer<UInt8>
        var index = 0
        var tokens = 0

        mutating func whitespace() {
            while index < bytes.count && [UInt8(32), 9, 10, 13].contains(bytes[index]) {
                index += 1
            }
        }

        mutating func take(_ byte: UInt8) throws {
            whitespace()
            guard index < bytes.count, bytes[index] == byte else {
                throw Invalid.structure
            }
            index += 1
        }

        mutating func string() throws -> Range<Int> {
            whitespace()
            let start = index
            try take(34)
            while index < bytes.count {
                let byte = bytes[index]
                index += 1
                if byte == 34 {
                    return start..<index
                }
                if byte == 92 {
                    guard index < bytes.count else {
                        throw Invalid.structure
                    }
                    let escape = bytes[index]
                    index += 1
                    if escape == 117 {
                        guard index + 4 <= bytes.count else {
                            throw Invalid.structure
                        }
                        for digit in bytes[index..<(index + 4)] {
                            guard (48...57).contains(digit) || (65...70).contains(digit) || (97...102).contains(digit) else {
                                throw Invalid.structure
                            }
                        }
                        index += 4
                    } else if ![UInt8(34), 92, 47, 98, 102, 110, 114, 116].contains(escape) {
                        throw Invalid.structure
                    }
                } else if byte < 32 {
                    throw Invalid.structure
                }
            }
            throw Invalid.structure
        }

        mutating func key() throws -> String {
            let range = try string()
            guard range.count <= 1024 else {
                throw Invalid.structure
            }
            guard let address = bytes.baseAddress else {
                throw Invalid.structure
            }
            let data = Data(bytes: address.advanced(by: range.lowerBound), count: range.count)
            return try JSONDecoder().decode(String.self, from: data)
        }

        mutating func number() throws {
            if index < bytes.count && bytes[index] == 45 {
                index += 1
            }
            guard index < bytes.count else {
                throw Invalid.structure
            }
            if bytes[index] == 48 {
                index += 1
            } else {
                guard (49...57).contains(bytes[index]) else {
                    throw Invalid.structure
                }
                repeat { index += 1 } while index < bytes.count && (48...57).contains(bytes[index])
            }
            if index < bytes.count && bytes[index] == 46 {
                index += 1
                let start = index
                while index < bytes.count && (48...57).contains(bytes[index]) {
                    index += 1
                }
                guard index > start else {
                    throw Invalid.structure
                }
            }
            if index < bytes.count && [UInt8(101), 69].contains(bytes[index]) {
                index += 1
                if index < bytes.count && [UInt8(43), 45].contains(bytes[index]) {
                    index += 1
                }
                let start = index
                while index < bytes.count && (48...57).contains(bytes[index]) {
                    index += 1
                }
                guard index > start else {
                    throw Invalid.structure
                }
            }
        }

        mutating func value(depth: Int) throws {
            whitespace()
            tokens += 1
            guard depth <= 24, tokens <= 65536, index < bytes.count else {
                throw Invalid.structure
            }
            switch bytes[index] {
            case 34:
                _ = try string()
            case 123:
                index += 1
                whitespace()
                if index < bytes.count && bytes[index] == 125 {
                    index += 1
                    return
                }
                var keys = Set<String>()
                while true {
                    guard keys.insert(try key()).inserted else {
                        throw Invalid.structure
                    }
                    try take(58)
                    try value(depth: depth + 1)
                    whitespace()
                    if index < bytes.count && bytes[index] == 125 {
                        index += 1
                        return
                    }
                    try take(44)
                }
            case 91:
                index += 1
                whitespace()
                if index < bytes.count && bytes[index] == 93 {
                    index += 1
                    return
                }
                while true {
                    try value(depth: depth + 1)
                    whitespace()
                    if index < bytes.count && bytes[index] == 93 {
                        index += 1
                        return
                    }
                    try take(44)
                }
            case 116, 102, 110:
                let literal = Array((bytes[index] == 116 ? "true" : bytes[index] == 102 ? "false" : "null").utf8)
                guard index + literal.count <= bytes.count,
                    Array(bytes[index..<(index + literal.count)]) == literal else {
                    throw Invalid.structure
                }
                index += literal.count
            default:
                try number()
            }
        }

        mutating func messages() throws -> [Range<Int>] {
            try take(91)
            var ranges: [Range<Int>] = []
            whitespace()
            if index < bytes.count && bytes[index] == 93 {
                index += 1
                return ranges
            }
            while true {
                whitespace()
                let start = index
                guard index < bytes.count, bytes[index] == 123, ranges.count < 256 else {
                    throw Invalid.structure
                }
                try value(depth: 1)
                ranges.append(start..<index)
                whitespace()
                if index < bytes.count && bytes[index] == 93 {
                    index += 1
                    return ranges
                }
                try take(44)
            }
        }

        mutating func root() throws -> [Range<Int>] {
            try take(123)
            var keys = Set<String>()
            var ranges: [Range<Int>]?
            while true {
                let name = try key()
                guard keys.insert(name).inserted else {
                    throw Invalid.structure
                }
                try take(58)
                if name == "messages" {
                    ranges = try messages()
                } else {
                    try value(depth: 1)
                }
                whitespace()
                if index < bytes.count && bytes[index] == 125 {
                    index += 1
                    break
                }
                try take(44)
            }
            whitespace()
            guard index == bytes.count, let ranges else {
                throw Invalid.structure
            }
            return ranges
        }
    }
}
