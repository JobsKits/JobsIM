//
//  IMJSON.swift
//  JobsIMCore
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import Foundation

public enum IMJSON: Codable, Sendable, Equatable {

    case object([String: IMJSON])
    case array([IMJSON])
    case string(String)
    case number(Decimal)
    case bool(Bool)
    case null

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() {
            self = .null
        } else if let v = try? c.decode(Bool.self) {
            self = .bool(v)
        } else if let v = try? c.decode(String.self) {
            self = .string(v)
        } else if let v = try? c.decode(Decimal.self) {
            self = .number(v)
        } else if let v = try? c.decode([IMJSON].self) {
            self = .array(v)
        } else {
            self = .object(try c.decode([String: IMJSON].self))
        }
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        /// 保存对象。
        case .object(let v):
            try c.encode(v)
        /// 保存列表。
        case .array(let v):
            try c.encode(v)
        /// 保持字符串ID精度。
        case .string(let v):
            try c.encode(v)
        /// 保持JSON数值精度。
        case .number(let v):
            try c.encode(v)
        /// 保存开关。
        case .bool(let v):
            try c.encode(v)
        /// 明确空值。
        case .null:
            try c.encodeNil()
        }
    }

    public subscript(_ key: String) -> IMJSON {
        if case .object(let value) = self {
            return value[key] ?? .null
        }
        return .null
    }

    public var text: String {
        if case .string(let value) = self {
            return value
        }
        if case .number(let value) = self {
            return NSDecimalNumber.make(decimal: value).stringValue
        }
        return ""
    }

    public var list: [IMJSON] {
        if case .array(let value) = self {
            return value
        }
        return []
    }

    public var fields: [String: IMJSON] {
        if case .object(let value) = self {
            return value
        }
        return [:]
    }

    public func setting(_ key: String, _ value: IMJSON) -> IMJSON {
        var result = fields
        result[key] = value
        return .object(result)
    }

    public func firstText(_ keys: String...) -> String {
        for key in keys {
            if !self[key].text.isEmpty {
                return self[key].text
            }
        }
        return ""
    }
}
