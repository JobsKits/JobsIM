//
//  IMMigrationProtocol.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二。
//

import CryptoKit
import Foundation
import JobsIMCore
import JobsIMTransport

struct IMMigrationBinding: Equatable {
    let namespace: IMNamespace
    let family: String
    let physical: String
    let guardState: IMJSON
}

enum IMMigrationProtocol {

    static let format = "jobsim-personal-migration-v1"
    static let limit = 128 * 1024 * 1024

    static func refuse(_ code: String = "MIGRATION_INVALID") throws -> Never {
        throw IMAPIError(status: 0, code: code, message: IMStrings.text("migrationInvalid"))
    }

    static func id(_ value: IMJSON) throws -> String {
        guard case .string(let text) = value, UUID(uuidString: text) != nil else {
            try refuse()
        }
        return text.lowercased()
    }

    static func positive(_ value: IMJSON) throws -> UInt64 {
        let number = try IMSyncProtocol.counter(value)
        guard number > 0 else {
            try refuse()
        }
        return number
    }

    static func hash(_ bytes: Data) -> String {
        return SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
    }

    static func digest(_ value: IMJSON) throws -> String {
        guard case .string(let text) = value,
            text.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil else {
            try refuse()
        }
        return text
    }

    static func base64(_ bytes: Data, url: Bool = true) -> String {
        let text = bytes.base64EncodedString()
        return url ? text.replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "") : text
    }

    static func decode(_ value: IMJSON, maximum: Int, url: Bool = true) throws -> Data {
        guard case .string(let text) = value, text.utf8.count <= ((maximum + 2) / 3) * 4,
            !url || text.range(of: "^[A-Za-z0-9_-]*$", options: .regularExpression) != nil else {
            try refuse()
        }
        var padded = url ? text.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/") : text
        if url {
            padded += String(repeating: "=", count: (4 - padded.utf8.count % 4) % 4)
        }
        guard let bytes = Data(base64Encoded: padded), bytes.count <= maximum, base64(bytes, url: url) == text else {
            try refuse()
        }
        return bytes
    }

    static func object(_ value: IMJSON, required: Set<String>, optional: Set<String> = []) throws {
        guard case .object(let fields) = value, required.isSubset(of: Set(fields.keys)),
            Set(fields.keys).isSubset(of: required.union(optional)) else {
            try refuse()
        }
    }

    static func array(_ value: IMJSON, maximum: Int) throws -> [IMJSON] {
        guard case .array(let values) = value, values.count <= maximum else {
            try refuse()
        }
        return values
    }

    static func view(_ value: IMJSON, binding: IMMigrationBinding, target: Bool) throws -> IMJSON {
        try object(value, required: ["id", "requestId", "status", "manifest", "manifestEnvelope", "manifestDigest",
                                     "capsuleSHA256", "sizeBytes", "expiresAt", "sourcePreserved", "qualifiedAck", "targetQualified"])
        let bytes = try decode(value["manifestEnvelope"], maximum: 1024 * 1024)
        guard String(data: bytes, encoding: .utf8) != nil, hash(bytes) == (try digest(value["manifestDigest"])) else {
            try refuse("MIGRATION_MANIFEST_CHANGED")
        }
        let manifest = try JSONDecoder.make().decode(IMJSON.self, from: bytes)
        try object(manifest, required: ["format", "id", "scopeId", "userId", "sourceContextId", "sourceDeviceId",
            "sourceStoreGeneration", "targetDeviceId", "targetStoreGeneration", "restoreEpoch", "securityVersion",
            "sourceGeneration", "messages", "objects", "expiresAt"])
        for key in ["id", "scopeId", "userId", "sourceDeviceId", "sourceStoreGeneration", "targetDeviceId", "targetStoreGeneration"] {
            _ = try id(manifest[key])
        }
        _ = try positive(manifest["securityVersion"])
        _ = try positive(manifest["sourceGeneration"])
        let ns = binding.namespace
        guard manifest == value["manifest"], manifest["format"].text == format,
            manifest["scopeId"].text == ns.deploymentID, manifest["userId"].text == ns.account,
            manifest["sourceContextId"].text == "personal", manifest["restoreEpoch"].text == ns.restoreEpoch,
            manifest[target ? "targetDeviceId" : "sourceDeviceId"].text == ns.deviceID,
            manifest[target ? "targetStoreGeneration" : "sourceStoreGeneration"].text == binding.physical,
            manifest["sourceDeviceId"] != manifest["targetDeviceId"],
            value["id"] == manifest["id"], value["expiresAt"] == manifest["expiresAt"],
            IMSyncProtocol.date(manifest["expiresAt"].text) != nil,
            value["requestId"].text.range(of: "^[A-Za-z0-9_-]{8,128}$", options: .regularExpression) != nil,
            ["building", "ready", "installed_reported", "revoking", "retired"].contains(value["status"].text),
            value["sourcePreserved"] == .bool(true), value["qualifiedAck"] == .bool(false), value["targetQualified"] == .bool(false) else {
            try refuse("MIGRATION_CONTEXT_CHANGED")
        }
        let size = try IMSyncProtocol.counter(value["sizeBytes"])
        guard size <= UInt64(limit + 4096) else {
            try refuse()
        }
        if ["ready", "installed_reported"].contains(value["status"].text) {
            _ = try digest(value["capsuleSHA256"])
        }
        let messages = try array(manifest["messages"], maximum: 256)
        let objects = try array(manifest["objects"], maximum: 64)
        guard !messages.isEmpty, Set(messages.map { $0["messageId"].text }).count == messages.count,
            Set(objects.map { $0["objectId"].text }).count == objects.count else {
            try refuse()
        }
        for ref in messages {
            try object(ref, required: ["messageId", "conversationId", "contentVersion", "membershipGeneration", "classification", "primaryDigest", "objectMode"], optional: ["objectId", "deadline"])
            _ = try id(ref["messageId"])
            _ = try id(ref["conversationId"])
            _ = try positive(ref["contentVersion"])
            _ = try positive(ref["membershipGeneration"])
            _ = try digest(ref["primaryDigest"])
            guard ["available", "cloud_only"].contains(ref["classification"].text),
                ["none", "metadata_only", "full"].contains(ref["objectMode"].text),
                ref["deadline"] == .null || IMSyncProtocol.date(ref["deadline"].text) != nil else {
                try refuse()
            }
            if ref["objectId"] != .null {
                _ = try id(ref["objectId"])
            }
            let matching = objects.filter { $0["messageId"] == ref["messageId"] }
            guard ref["objectMode"].text == "full" ? matching.count == 1 && matching[0]["objectId"] == ref["objectId"] : matching.isEmpty else {
                try refuse()
            }
        }
        for ref in objects {
            try object(ref, required: ["objectId", "messageId", "sizeBytes", "sha256", "contentType", "classification"], optional: ["deadline"])
            _ = try id(ref["objectId"])
            _ = try id(ref["messageId"])
            _ = try digest(ref["sha256"])
            let length = try IMSyncProtocol.counter(ref["sizeBytes"])
            guard length <= 64 * 1024 * 1024, ["available", "cloud_only"].contains(ref["classification"].text),
                !ref["contentType"].text.isEmpty, ref["contentType"].text.utf8.count <= 255,
                ref["deadline"] == .null || IMSyncProtocol.date(ref["deadline"].text) != nil,
                messages.contains(where: { $0["messageId"] == ref["messageId"] && $0["objectId"] == ref["objectId"] && $0["objectMode"].text == "full" }) else {
                try refuse()
            }
        }
        return value
    }

    static func open(_ value: IMJSON, capsule: Data, key: IMJSON, binding: IMMigrationBinding) throws -> IMJSON {
        let view = try view(value, binding: binding, target: true)
        let manifest = view["manifest"]
        guard ["ready", "installed_reported"].contains(view["status"].text),
            let expiry = IMSyncProtocol.date(view["expiresAt"].text), expiry > Date(),
            capsule.count >= 36, capsule.count <= limit + 4096,
            UInt64(capsule.count) == (try IMSyncProtocol.counter(view["sizeBytes"])),
            hash(capsule) == view["capsuleSHA256"].text,
            capsule.prefix(8) == Data("JIMMIGR1".utf8),
            key["id"] == view["id"], key["manifestDigest"] == view["manifestDigest"], key["algorithm"].text == "AES-256-GCM",
            key["expiresAt"] == view["expiresAt"], key["targetQualified"] == .bool(false) else {
            try refuse("MIGRATION_CAPSULE_INVALID")
        }
        var bytes = try decode(key["key"], maximum: 32)
        defer {
            bytes.resetBytes(in: 0..<bytes.count)
        }
        let aad = try decode(key["aad"], maximum: 8192)
        let expected = ["JobsIM migration v1", manifest["scopeId"].text, manifest["id"].text, manifest["userId"].text,
            manifest["sourceDeviceId"].text, manifest["sourceStoreGeneration"].text, manifest["targetDeviceId"].text,
            manifest["targetStoreGeneration"].text, manifest["restoreEpoch"].text, manifest["securityVersion"].text,
            manifest["sourceGeneration"].text, view["manifestDigest"].text].joined(separator: "\0")
        guard bytes.count == 32, aad == Data(expected.utf8) else {
            try refuse("MIGRATION_CONTEXT_CHANGED")
        }
        let box = try AES.GCM.SealedBox(nonce: AES.GCM.Nonce(data: capsule.subdata(in: 8..<20)),
            ciphertext: capsule.subdata(in: 20..<(capsule.count - 16)), tag: capsule.suffix(16))
        var plain = try AES.GCM.open(box, using: SymmetricKey(data: bytes), authenticating: aad)
        defer {
            plain.resetBytes(in: 0..<plain.count)
        }
        guard plain.count <= limit, String(data: plain, encoding: .utf8) != nil else {
            try refuse()
        }
        let packet = try JSONDecoder.make().decode(IMJSON.self, from: plain)
        try object(packet, required: ["format", "manifestDigest", "messages", "objects"])
        guard packet["format"].text == format, packet["manifestDigest"] == view["manifestDigest"] else {
            try refuse()
        }
        let messages = try array(packet["messages"], maximum: 256)
        let objects = try array(packet["objects"], maximum: 64)
        guard messages.count == manifest["messages"].list.count, objects.count == manifest["objects"].list.count,
            Set(messages.map { $0["id"].text }).count == messages.count,
            Set(objects.map { $0["objectId"].text }).count == objects.count else {
            try refuse()
        }
        let messageRanges = try IMMigrationJSON.messageRanges(plain)
        guard messageRanges.count == messages.count else {
            try refuse()
        }
        for (index, message) in messages.enumerated() {
            try object(message, required: ["id", "conversationId", "senderId", "type", "text", "createdAt", "contentVersion", "creationEpoch", "creationSequence", "objectMode"], optional: ["replyToId", "attachmentId", "forwarded"])
            _ = try id(message["senderId"])
            guard case .string = message["text"], case .string = message["type"] else {
                try refuse()
            }
            _ = try positive(message["creationEpoch"])
            _ = try positive(message["creationSequence"])
            guard let ref = manifest["messages"].list.first(where: { $0["messageId"] == message["id"] }),
                ref["conversationId"] == message["conversationId"], ref["contentVersion"] == message["contentVersion"],
                ref["objectMode"] == message["objectMode"], ref["objectId"] == message["attachmentId"],
                hash(plain.subdata(in: messageRanges[index])) == ref["primaryDigest"].text,
                IMSyncProtocol.date(message["createdAt"].text) != nil, message["text"].text.utf8.count <= 16384,
                ["text", "image", "video", "audio", "file"].contains(message["type"].text),
                message["forwarded"] == .null || message["forwarded"] == .bool(true),
                message["replyToId"] == .null || UUID(uuidString: message["replyToId"].text) != nil else {
                try refuse()
            }
            if message["type"].text == "text" {
                guard message["objectMode"].text == "none", message["attachmentId"] == .null else {
                    try refuse()
                }
            } else if message["forwarded"] == .bool(true) {
                guard message["objectMode"].text == "metadata_only", message["attachmentId"] == .null else {
                    try refuse()
                }
            } else {
                guard ["metadata_only", "full"].contains(message["objectMode"].text), UUID(uuidString: message["attachmentId"].text) != nil else {
                    try refuse()
                }
            }
        }
        for object in objects {
            try self.object(object, required: ["objectId", "messageId", "bytes"])
            guard let ref = manifest["objects"].list.first(where: { $0["objectId"] == object["objectId"] }),
                ref["messageId"] == object["messageId"] else {
                try refuse()
            }
            let body = try decode(object["bytes"], maximum: 64 * 1024 * 1024, url: false)
            guard UInt64(body.count) == (try IMSyncProtocol.counter(ref["sizeBytes"])), hash(body) == ref["sha256"].text else {
                try refuse("MIGRATION_OBJECT_INTEGRITY")
            }
        }
        return packet
    }
}
