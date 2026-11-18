//
//  IMReceiptProtocol.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import Foundation
import JobsIMCore
import JobsIMTransport

enum IMReceiptProtocol {

    static let version = "qualified-receipts-v1"
    static let protection = [
        "transportConfigured", "publishConnected", "consumeConnected", "messageDurable",
        "journalDurable", "journalIndependent", "epochMatches",
    ]
    static let contracts = ["native_sqlite_full_v1", "native_sqlite_full_file_fsync_v1"]

    static func fail(_ code: String = "RECEIPT_INVALID_RESPONSE") throws -> Never {
        throw IMAPIError(status: 0, code: code, message: "Receipt responsibility retained")
    }

    static func exact(_ value: IMJSON, _ keys: Set<String>) throws {
        guard Set(value.fields.keys) == keys else {
            try fail()
        }
    }

    static func id(_ value: IMJSON) throws -> String {
        return try IMMigrationProtocol.id(value)
    }

    static func counter(_ value: IMJSON) throws -> String {
        return String(try IMMigrationProtocol.positive(value))
    }

    static func hash(_ value: IMJSON) throws -> String {
        guard case .string = value, value.text.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil else {
            try fail()
        }
        return value.text
    }

    static func flag(_ value: IMJSON) throws -> Bool {
        guard case .bool(let result) = value else {
            try fail()
        }
        return result
    }

    static func tuple(_ value: IMJSON) throws {
        let generation = value["deliveryGeneration"].text
        let digest = value["manifestDigest"].text
        guard generation.isEmpty == digest.isEmpty else {
            try fail()
        }
        if !generation.isEmpty {
            _ = try counter(value["deliveryGeneration"])
            _ = try hash(value["manifestDigest"])
        }
    }

    static func storage(_ value: IMJSON, namespace: IMNamespace, physical: String) throws {
        try exact(
            value,
            [
                "qualificationId", "storeGeneration", "restoreEpoch", "profileId", "buildDigest",
                "verificationDigest", "storageContract", "qualified", "objectQualified",
            ])
        guard value["storeGeneration"].text == physical, value["restoreEpoch"].text == namespace.restoreEpoch else {
            try fail("RECEIPT_CONTEXT_CHANGED")
        }
        let qualified = try flag(value["qualified"])
        let objects = try flag(value["objectQualified"])
        guard !objects || qualified else {
            try fail()
        }
        for field in ["qualificationId", "profileId", "buildDigest", "verificationDigest", "storageContract"] {
            guard case .string = value[field] else {
                try fail()
            }
        }
        if qualified {
            _ = try id(value["qualificationId"])
            _ = try hash(value["buildDigest"])
            _ = try hash(value["verificationDigest"])
            guard contracts.contains(value["storageContract"].text),
                !objects || value["storageContract"].text == contracts[1]
            else {
                try fail()
            }
        }
    }

    static func context(_ value: IMJSON, namespace: IMNamespace, physical: String, schema: String) throws -> IMJSON {
        guard value["protocol"].text == version, value["scopeId"].text == namespace.deploymentID,
            value["sourceContextId"].text == "personal", value["principalId"].text == namespace.account,
            value["deviceId"].text == namespace.deviceID, value["storeGeneration"].text == physical,
            value["restoreEpoch"].text == namespace.restoreEpoch, value["schemaVersion"].text == schema,
            value["defaultRetentionMode"].text == "cloud_history", value["deletionEligible"] == .bool(false),
            value["targetRangeComplete"] == .bool(false)
        else {
            try fail("RECEIPT_CONTEXT_CHANGED")
        }
        try storage(value["storage"], namespace: namespace, physical: physical)
        let readiness = value["readiness"]
        try exact(readiness, Set(protection + ["ready", "blockers"]))
        for field in protection {
            _ = try flag(readiness[field])
        }
        let ready = try flag(readiness["ready"])
        let qualified = value["storage"]["qualified"] == .bool(true)
        let objects = value["storage"]["objectQualified"] == .bool(true)
        guard case .array(let blockers) = readiness["blockers"], blockers.count <= 32,
            Set(blockers.map(\.text)).count == blockers.count,
            blockers.allSatisfy({ $0.text.range(of: "^[a-z0-9_]{1,80}$", options: .regularExpression) != nil }),
            ready == protection.allSatisfy({ readiness[$0] == .bool(true) }), !ready || blockers.isEmpty,
            value["qualification"].text
                == (objects ? "qualified_content_and_objects" : qualified ? "qualified_content" : "transport_only"),
            value["qualifiedAck"] == .bool(qualified), value["contentReceiptEnabled"] == .bool(qualified),
            value["objectReceiptEnabled"] == .bool(objects), value["afterReceiptEnabled"] == .bool(qualified && ready),
            value["afterObjectReceiptEnabled"] == .bool(objects && ready)
        else {
            try fail()
        }
        return value
    }

    static func profiles(_ value: IMJSON, report: IMJSON) throws -> [IMJSON] {
        guard value["protocol"].text == version, value["qualifiedAck"] == .bool(false),
            value["declarationRequired"] == .bool(true), value["localDurabilityVerificationRequired"] == .bool(true),
            case .array(let profiles) = value["profiles"], profiles.count <= 64
        else {
            try fail()
        }
        var identifiers = Set<String>()
        for profile in profiles {
            try exact(profile, ["id", "buildDigest", "verificationDigest", "storageContract"])
            guard profile["id"].text.range(of: "^[a-z0-9_.-]{8,128}$", options: .regularExpression) != nil,
                profile["buildDigest"] == report["buildDigest"], identifiers.insert(profile["id"].text).inserted
            else {
                try fail()
            }
            _ = try hash(profile["verificationDigest"])
            guard contracts.contains(profile["storageContract"].text) else {
                try fail()
            }
        }
        return profiles.filter {
            $0["verificationDigest"] == report["verificationDigest"]
                && $0["storageContract"] == report["storageContract"]
        }
    }

    static func proof(
        _ token: IMJSON, object: Bool, namespace: IMNamespace, physical: String,
        requireUnexpired: Bool = true
    ) throws -> IMJSON {
        let parts = token.text.split(separator: ".", omittingEmptySubsequences: false)
        guard case .string = token, token.text.utf8.count <= (object ? 8192 : 4096), parts.count == 3,
            parts[0] == (object ? "ro1" : "rc1"),
            try IMMigrationProtocol.decode(.string(String(parts[2])), maximum: 32).count == 32
        else {
            try fail()
        }
        let bytes = try IMMigrationProtocol.decode(.string(String(parts[1])), maximum: 8192)
        var wrapped = Data("{\"messages\":[".utf8)
        wrapped.append(bytes)
        wrapped.append(Data("]}".utf8))
        _ = try IMMigrationJSON.messageRanges(wrapped)
        let value = try JSONDecoder.make().decode(IMJSON.self, from: bytes)
        let common: Set<String> = [
            "scope", "user", "device", "store", "epoch", "qualification", "message",
            "conversation", "contentVersion", "bodyDigest", "permission", "expires",
        ]
        let optional: Set<String> = ["deliveryGeneration", "manifestDigest"]
        let required = object ? common.union(["parentReceiptId", "objectId", "sizeBytes", "objectDigest"]) : common
        guard required.isSubset(of: Set(value.fields.keys)),
            Set(value.fields.keys).isSubset(of: required.union(optional)),
            value["scope"].text == namespace.deploymentID, value["user"].text == namespace.account,
            value["device"].text == namespace.deviceID, value["store"].text == physical,
            value["epoch"].text == namespace.restoreEpoch, case .number = value["expires"],
            let expires = Int64(value["expires"].text),
            !requireUnexpired || Double(expires) > Date().timeIntervalSince1970,
            !requireUnexpired || Double(expires) <= Date().timeIntervalSince1970 + 3660
        else {
            try fail("RECEIPT_PROOF_CHANGED")
        }
        for field in ["qualification", "message", "conversation"] {
            _ = try id(value[field])
        }
        _ = try counter(value["contentVersion"])
        _ = try hash(value["bodyDigest"])
        _ = try hash(value["permission"])
        try tuple(value)
        if object {
            _ = try id(value["parentReceiptId"])
            _ = try id(value["objectId"])
            _ = try hash(value["objectDigest"])
            guard let size = Int(value["sizeBytes"].text), (0...64 * 1024 * 1024).contains(size),
                String(size) == value["sizeBytes"].text
            else {
                try fail()
            }
        }
        return value
    }

    static func recorded(_ value: IMJSON, intent: IMJSON, namespace: IMNamespace) throws -> IMJSON {
        let object = intent["kind"].text == "object"
        let body = intent["originalBody"]
        var fields: Set<String> = [
            "operationId", "status", "receiptKind", "messageId", "conversationId", "contentVersion",
            "restoreEpoch", "deviceId", "storeGeneration", "qualificationId", "bodyDigest", "storageQualifiedAtCommit",
            "objectComplete", "targetRangeComplete", "deletionEligible", "deliveryGeneration", "manifestDigest",
        ]
        if object, intent["deliveryGeneration"].text.isEmpty,
            value["deliveryGeneration"] == .null, value["manifestDigest"] == .null
        {
            fields.subtract(["deliveryGeneration", "manifestDigest"])
        }
        try exact(value, object ? fields.union(["parentReceiptId", "objectId", "sizeBytes", "objectDigest"]) : fields)
        guard value["operationId"] == intent["id"], value["status"].text == "recorded",
            value["receiptKind"].text == (object ? "object_received" : "message_content"),
            value["messageId"] == intent["messageId"], value["conversationId"] == intent["conversationId"],
            value["contentVersion"] == intent["contentVersion"], value["restoreEpoch"] == intent["restoreEpoch"],
            value["deviceId"].text == namespace.deviceID, value["storeGeneration"] == intent["storeGeneration"],
            value["qualificationId"] == intent["qualificationId"], value["bodyDigest"] == intent["bodyDigest"],
            value["deliveryGeneration"].text == intent["deliveryGeneration"].text,
            value["manifestDigest"].text == intent["manifestDigest"].text,
            value["storageQualifiedAtCommit"] == .bool(true), value["objectComplete"] == .bool(object),
            value["targetRangeComplete"] == .bool(false), value["deletionEligible"] == .bool(false)
        else {
            try fail()
        }
        if object {
            for field in ["parentReceiptId", "objectId", "sizeBytes", "objectDigest"] where value[field] != body[field]
            {
                try fail()
            }
        }
        return value
    }

    static func declarationFact(_ value: IMJSON) throws -> IMJSON {
        let base: Set<String> = ["qualificationId", "storeGeneration", "restoreEpoch", "profileId", "buildDigest",
            "verificationDigest", "storageContract"]
        let snapshots: Set<String> = ["qualified", "objectQualified"]
        let fields = Set(value.fields.keys)
        guard base.isSubset(of: fields), fields.isSubset(of: base.union(snapshots)),
            fields.intersection(snapshots).isEmpty || snapshots.isSubset(of: fields),
            contracts.contains(value["storageContract"].text) else {
            try fail()
        }
        _ = try id(value["qualificationId"])
        _ = try id(value["storeGeneration"])
        _ = try counter(value["restoreEpoch"])
        _ = try hash(value["buildDigest"])
        _ = try hash(value["verificationDigest"])
        guard value["profileId"].text.range(of: "^[a-z0-9_.-]{8,128}$", options: .regularExpression) != nil else {
            try fail()
        }
        if snapshots.isSubset(of: fields) {
            guard try flag(value["qualified"]),
                try flag(value["objectQualified"]) == (value["storageContract"].text == contracts[1]) else {
                try fail()
            }
        }
        return .object(value.fields.filter { base.contains($0.key) })
    }

    static func declaration(_ row: IMJSON, namespace: IMNamespace, physical: String) throws {
        try exact(row, ["id", "familyId", "createdAt", "attempts", "state", "originalBody", "result"])
        _ = try id(row["id"])
        _ = try id(row["familyId"])
        guard IMSyncProtocol.date(row["createdAt"].text) != nil, case .number = row["attempts"],
            let attempts = Int(row["attempts"].text), (0...10000).contains(attempts),
            ["prepared", "unknown", "completed", "frozen"].contains(row["state"].text)
        else {
            try fail()
        }
        guard row["state"].text != "completed" || row["result"] != .null,
            !["prepared", "unknown"].contains(row["state"].text) || row["result"] == .null else {
            try fail()
        }
        let body = row["originalBody"]
        try exact(
            body, ["requestId", "storeGeneration", "revisionEpoch", "profileId", "buildDigest", "verificationDigest"])
        guard body["requestId"] == row["id"], body["storeGeneration"].text == physical,
            body["revisionEpoch"].text == namespace.restoreEpoch,
            body["profileId"].text.range(of: "^[a-z0-9_.-]{8,128}$", options: .regularExpression) != nil
        else {
            try fail()
        }
        _ = try hash(body["buildDigest"])
        _ = try hash(body["verificationDigest"])
        if row["result"] != .null {
            let result = try declarationFact(row["result"])
            guard result == row["result"] else {
                try fail()
            }
            for field in ["storeGeneration", "profileId", "buildDigest", "verificationDigest"]
            where result[field] != body[field] {
                try fail()
            }
            guard result["restoreEpoch"] == body["revisionEpoch"], contracts.contains(result["storageContract"].text)
            else {
                try fail()
            }
        }
    }

    static func localIntent(_ row: IMJSON, namespace: IMNamespace, physical: String) throws {
        try exact(
            row,
            [
                "id", "kind", "familyId", "storeGeneration", "restoreEpoch", "schemaVersion", "createdAt",
                "attempts", "state", "originalBody", "messageId", "conversationId", "contentVersion",
                "membershipGeneration",
                "qualificationId", "permissionGeneration", "deliveryGeneration", "manifestDigest", "bodyDigest",
                "result",
            ])
        for field in ["id", "familyId", "messageId", "conversationId", "qualificationId"] {
            _ = try id(row[field])
        }
        guard ["content", "object"].contains(row["kind"].text), row["storeGeneration"].text == physical,
            row["restoreEpoch"].text == namespace.restoreEpoch, ["1", "2"].contains(row["schemaVersion"].text),
            IMSyncProtocol.date(row["createdAt"].text) != nil, case .number = row["attempts"],
            let attempts = Int(row["attempts"].text), (0...10000).contains(attempts),
            ["prepared", "unknown", "completed", "frozen"].contains(row["state"].text)
        else {
            try fail()
        }
        guard row["state"].text != "completed" || row["result"] != .null,
            !["prepared", "unknown"].contains(row["state"].text) || row["result"] == .null else {
            try fail()
        }
        _ = try counter(row["contentVersion"])
        _ = try counter(row["membershipGeneration"])
        _ = try hash(row["permissionGeneration"])
        _ = try hash(row["bodyDigest"])
        try tuple(row)
        let object = row["kind"].text == "object"
        let body = row["originalBody"]
        let common: Set<String> = [
            "requestId", "storeGeneration", "revisionEpoch", "qualificationId", "kind", "challenge",
        ]
        try exact(
            body,
            object
                ? common.union(["parentReceiptId", "objectId", "sizeBytes", "objectDigest"])
                : common.union(["bodyDigest"]))
        guard body["requestId"] == row["id"], body["storeGeneration"] == row["storeGeneration"],
            body["revisionEpoch"] == row["restoreEpoch"], body["qualificationId"] == row["qualificationId"],
            body["kind"].text == (object ? "object_received" : "message_content")
        else {
            try fail()
        }
        let verified = try proof(
            body["challenge"], object: object, namespace: namespace, physical: physical, requireUnexpired: false)
        for (outer, inner) in [
            ("messageId", "message"), ("conversationId", "conversation"), ("contentVersion", "contentVersion"),
            ("qualificationId", "qualification"), ("permissionGeneration", "permission"), ("bodyDigest", "bodyDigest"),
        ]
        where row[outer] != verified[inner] {
            try fail()
        }
        guard row["deliveryGeneration"].text == verified["deliveryGeneration"].text,
            row["manifestDigest"].text == verified["manifestDigest"].text
        else {
            try fail()
        }
        for field in object ? ["parentReceiptId", "objectId", "sizeBytes", "objectDigest"] : ["bodyDigest"]
        where body[field] != verified[field] {
            try fail()
        }
        if row["result"] != .null {
            _ = try recorded(row["result"], intent: row, namespace: namespace)
        }
    }
}
