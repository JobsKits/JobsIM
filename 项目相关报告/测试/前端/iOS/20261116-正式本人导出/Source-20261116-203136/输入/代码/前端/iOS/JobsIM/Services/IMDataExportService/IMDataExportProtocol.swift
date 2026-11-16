//
//  IMDataExportProtocol.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import CryptoKit
import Foundation
import JobsIMCore

enum IMDataExportProtocol {

    static func fail(_ code: String = "DATA_EXPORT_PROTOCOL_INVALID") throws -> Never {
        try IMSyncProtocol.refuse(code)
    }

    static func digest(_ raw: String) -> String {
        return SHA256.hash(data: Data(raw.utf8)).map { String(format: "%02x", $0) }.joined()
    }

    static func body(_ raw: IMJSON, keys: Set<String>) throws -> [String: IMJSON] {
        guard case .string(let text) = raw, text.utf8.count <= 2048,
            let decoded = try? JSONDecoder.make().decode(IMJSON.self, from: Data(text.utf8)),
            case .object(let values) = decoded, Set(values.keys) == keys
        else {
            try fail("DATA_EXPORT_STORE_INVALID")
        }
        guard case .string(let nonce) = values["requestId"] ?? .null, (8...128).contains(nonce.utf8.count), !nonce.contains("\0") else {
            try fail("DATA_EXPORT_STORE_INVALID")
        }
        return values
    }

    static func original(_ row: IMJSON, namespace: IMNamespace, family: String) throws {
        let allowed: Set<String> = ["id", "familyId", "originalBody", "bodySHA256", "createdAt", "jobId", "cancelBody", "cancelSHA256"]
        guard case .object(let fields) = row, Set(fields.keys).isSubset(of: allowed),
            row["familyId"].text == family, !family.isEmpty,
            IMSyncProtocol.date(row["createdAt"].text) != nil,
            digest(row["originalBody"].text) == row["bodySHA256"].text
        else {
            try fail("DATA_EXPORT_STORE_INVALID")
        }
        let creation = try body(row["originalBody"], keys: ["requestId", "restoreEpoch"])
        guard creation["requestId"] == row["id"], creation["restoreEpoch"] == .string(namespace.restoreEpoch) else {
            try fail("DATA_EXPORT_CONTEXT_CHANGED")
        }
        _ = try IMSyncProtocol.counter(creation["restoreEpoch"] ?? .null)
        if row["jobId"] != .null {
            _ = try IMSyncProtocol.id(row["jobId"])
        }
        if row["cancelBody"] != .null || row["cancelSHA256"] != .null {
            _ = try IMSyncProtocol.id(row["jobId"])
            _ = try body(row["cancelBody"], keys: ["requestId"])
            guard digest(row["cancelBody"].text) == row["cancelSHA256"].text else {
                try fail("DATA_EXPORT_STORE_INVALID")
            }
        }
    }

    static func view(_ value: IMJSON, expectedJob: String? = nil) throws -> IMJSON {
        guard value["format"] == .string("zip"), value["scope"] == .string("personal_current") else {
            try fail()
        }
        let job = value["job"]
        _ = try IMSyncProtocol.id(job["id"])
        _ = try IMSyncProtocol.id(job["artifactId"])
        let epoch = try IMSyncProtocol.counter(job["restoreEpoch"])
        let build = try IMSyncProtocol.counter(job["buildGeneration"])
        guard epoch != 0, build != 0, expectedJob == nil || job["id"].text == expectedJob,
            ["requested", "building", "ready", "cancelled", "expired", "failed"].contains(job["state"].text),
            IMSyncProtocol.date(job["expiresAt"].text) != nil,
            job["cleanupState"].text.range(of: "^[a-z_]{1,48}$", options: .regularExpression) != nil
        else {
            try fail()
        }
        var metadata: [String: IMJSON] = [:]
        for key in ["id", "artifactId", "restoreEpoch", "buildGeneration", "state", "expiresAt", "cleanupState"] {
            metadata[key] = job[key]
        }
        if job["failureCode"] != .null {
            guard job["failureCode"].text.range(of: "^[A-Z0-9_]{1,64}$", options: .regularExpression) != nil else {
                try fail()
            }
            metadata["failureCode"] = job["failureCode"]
        }
        var result: [String: IMJSON] = ["job": .object(metadata), "format": .string("zip"), "scope": .string("personal_current")]
        for key in ["coverage", "exclusions"] {
            guard case .array(let values) = value[key], values.count <= 64 else {
                try fail()
            }
            for item in values {
                guard case .string(let text) = item, text.range(of: "^[a-z_]{1,100}$", options: .regularExpression) != nil else {
                    try fail()
                }
            }
            result[key] = .array(values)
        }
        if value["capturedAt"] != .null {
            guard IMSyncProtocol.date(value["capturedAt"].text) != nil else {
                try fail()
            }
            result["capturedAt"] = value["capturedAt"]
        }
        return .object(result)
    }

    static func ticket(_ value: IMJSON, job: IMJSON, now: Date = Date()) throws -> String {
        guard value["format"] == .string("zip"), case .string(let token) = value["downloadToken"],
            let deadline = IMSyncProtocol.date(value["expiresAt"].text),
            let expiry = IMSyncProtocol.date(job["expiresAt"].text),
            deadline > now, deadline <= now.addingTimeInterval(65), deadline <= expiry,
            token.range(of: "^[A-Za-z0-9_-]{16,256}$", options: .regularExpression) != nil
        else {
            try fail()
        }
        return value["downloadToken"].text
    }
}
