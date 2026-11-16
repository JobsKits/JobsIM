//
//  IMSyncProtocol.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import CryptoKit
import Foundation
import JobsIMCore

enum IMSyncProtocol {
    struct Failure: LocalizedError {
        let code: String

        var errorDescription: String? {
            return code
        }
    }

    static func refuse(_ code: String = "SYNC_INVALID_RESPONSE") throws -> Never {
        throw Failure(code: code)
    }

    static func text(_ value: IMJSON, limit: Int = 4096) throws -> String {
        guard case .string(let result) = value, !result.isEmpty, result.utf8.count <= limit else {
            try refuse()
        }
        return result
    }

    static func counter(_ value: IMJSON) throws -> UInt64 {
        let raw = try text(value, limit: 19)
        guard raw.range(of: "^(0|[1-9][0-9]*)$", options: .regularExpression) != nil,
            let number = UInt64(raw), number <= UInt64(Int64.max)
        else {
            try refuse()
        }
        return number
    }

    static func id(_ value: IMJSON) throws -> String {
        let raw = try text(value, limit: 36)
        guard
            raw.range(
                of: "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$",
                options: .regularExpression) != nil
        else {
            try refuse()
        }
        return raw
    }

    static func digest(_ value: IMJSON) throws -> String {
        let raw = try text(value, limit: 64)
        guard raw.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil else {
            try refuse()
        }
        return raw
    }

    static func array(_ value: IMJSON) throws -> [IMJSON] {
        guard case .array(let result) = value else {
            try refuse()
        }
        return result
    }

    static func object(_ value: IMJSON) throws {
        guard case .object = value else {
            try refuse()
        }
    }

    static func binding(_ value: IMJSON, expected: IMJSON) throws {
        for (key, field) in expected.fields where value[key] != field {
            try refuse("SYNC_CONTEXT_CHANGED")
        }
    }

    // 校验服务端原字节，不能用Swift重新编码后的JSON代替原摘要输入。
    static func envelope(_ value: IMJSON, digest expected: IMJSON) throws -> IMJSON {
        let raw = try text(value, limit: 16 * 1024 * 1024)
        guard raw.range(of: "^[A-Za-z0-9_-]+$", options: .regularExpression) != nil,
            raw.count % 4 != 1
        else {
            try refuse()
        }
        let base64 = raw.replacingOccurrences(of: "-", with: "+")
            .replacingOccurrences(of: "_", with: "/")
        let padded = base64 + String(repeating: "=", count: (4 - base64.count % 4) % 4)
        guard let bytes = Data(base64Encoded: padded), String(data: bytes, encoding: .utf8) != nil
        else {
            try refuse()
        }
        let hash = SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
        guard hash == (try digest(expected)) else {
            try refuse("SYNC_DIGEST_MISMATCH")
        }
        return try JSONDecoder.make().decode(IMJSON.self, from: bytes)
    }

    static func reference(_ value: IMJSON, schema: String = "1") throws -> String {
        try object(value)
        var allowed = [
            "auth": ["account", "devices"],
            "directory": ["profile", "contacts", "relationship", "contact_request"],
            "conversation": ["conversation", "announcement"], "message": ["message"],
        ]
        if schema == "2" {
            allowed["directory", default: []].append("preferences")
            allowed["sync"] = ["conversation_view", "read_state", "read_observation", "draft"]
        } else if schema != "1" {
            try refuse("SYNC_UNSUPPORTED_SCHEMA")
        }
        let owner = try text(value["owner"], limit: 32)
        let kind = try text(value["kind"], limit: 32)
        guard allowed[owner]?.contains(kind) == true,
            ["active", "removed"].contains(value["state"].text),
            Set(value.fields.keys).isSubset(of: [
                "owner", "kind", "id", "revision", "revisionEpoch", "state",
                "subjectUserId", "conversationId", "targetUserId",
            ])
        else {
            try refuse()
        }
        let key = try id(value["id"])
        _ = try counter(value["revision"])
        guard try counter(value["revisionEpoch"]) > 0 else {
            try refuse()
        }
        for field in ["subjectUserId", "conversationId", "targetUserId"] where value[field] != .null
        {
            _ = try id(value[field])
        }
        return owner + "|" + kind + "|" + key
    }

    static func projection(_ value: IMJSON, reference ref: IMJSON, schema: String = "1") throws -> IMJSON {
        _ = try reference(value["ref"], schema: schema)
        let state = value["state"].text
        guard value["ref"] == ref,
            ["available", "global", "unavailable", "unknown", "redacted"].contains(state),
            case .bool(let match) = value["revisionMatch"]
        else {
            try refuse()
        }
        let revision = value["currentRevision"]
        let epoch = value["currentRevisionEpoch"]
        if revision != .null || epoch != .null {
            _ = try counter(revision)
            guard try counter(epoch) > 0,
                match == (revision == ref["revision"] && epoch == ref["revisionEpoch"])
            else {
                try refuse("SYNC_REVISION_MISMATCH")
            }
        } else if match || ["available", "global", "unavailable"].contains(state) {
            try refuse("SYNC_REVISION_MISMATCH")
        }
        if value["object"] != .null {
            let decoded = try envelope(value["bodyEnvelope"], digest: value["bodyDigest"])
            guard decoded == value["object"],
                ["available", "global", "unavailable"].contains(state)
            else {
                try refuse("SYNC_DIGEST_MISMATCH")
            }
            try body(decoded, reference: ref, state: state)
        } else if state == "available" || value["bodyEnvelope"] != .null
            || value["bodyDigest"] != .null
        {
            try refuse()
        }
        return value
    }

    private static func body(_ value: IMJSON, reference ref: IMJSON, state: String) throws {
        try object(value)
        let kind = ref["kind"].text
        if ["preferences", "conversation_view", "read_state", "read_observation", "draft"].contains(kind) {
            try personalBody(value, reference: ref)
        }
        if ["message", "conversation", "account", "contact_request"].contains(kind) {
            guard try id(value["id"]) == ref["id"].text else {
                try refuse("SYNC_OBJECT_ID_MISMATCH")
            }
        }
        if kind == "message" {
            _ = try counter(value["version"])
            for key in ["contentVersion", "sequence"] where value[key] != .null {
                _ = try counter(value[key])
            }
            for key in ["senderId", "conversationId"] where value[key] != .null {
                _ = try id(value[key])
            }
            if ref["conversationId"] != .null, value["conversationId"] != .null,
                ref["conversationId"] != value["conversationId"]
            {
                try refuse("SYNC_OBJECT_ID_MISMATCH")
            }
            if value["clientMsgId"] != .null {
                let nonce = try text(value["clientMsgId"], limit: 128)
                guard nonce.utf8.count >= 8 else {
                    try refuse()
                }
            }
            if value["creationEpoch"] == .string("unknown")
                || value["creationSequence"] == .string("unknown")
            {
                guard value["creationEpoch"] == .string("unknown"),
                    value["creationSequence"] == .string("unknown")
                else {
                    try refuse()
                }
            } else {
                for key in ["creationEpoch", "creationSequence"] where value[key] != .null {
                    _ = try counter(value[key])
                }
            }
            if state == "global", value["deleted"] != .bool(true) {
                try refuse("SYNC_INVALID_GLOBAL")
            }
        } else if kind == "conversation" {
            guard ["direct", "group", "self"].contains(value["kind"].text) else {
                try refuse()
            }
            for member in try array(value["memberIds"]) {
                _ = try id(member)
            }
            for key in ["membershipGeneration", "roleGeneration"] where value[key] != .null {
                _ = try counter(value[key])
            }
        }
    }

    static func manifest(_ value: IMJSON, binding expected: IMJSON, now: Date = Date(), schema: String = "1") throws
        -> IMJSON
    {
        try binding(value, expected: expected)
        _ = try id(value["snapshotId"])
        _ = try counter(value["boundary"])
        _ = try text(value["boundaryToken"])
        _ = try counter(value["classificationWatermark"])
        _ = try digest(value["permissionGeneration"])
        try object(value["coverage"])
        for owner in ["auth", "directory", "conversation", "message"] {
            _ = try counter(value["ownerWatermarks"][owner])
        }
        if schema == "2" {
            _ = try counter(value["ownerWatermarks"]["sync"])
            guard value["coverage"]["schemaVersion"] == .string("2") else {
                try refuse("SYNC_UNSUPPORTED_SCHEMA")
            }
        }
        let pages = try array(value["pageManifest"])
        guard ["1", "2"].contains(schema), value["schemaVersion"] == .string(schema), !pages.isEmpty, pages.count <= 200_000 else {
            try refuse()
        }
        var total = 0
        for (index, page) in pages.enumerated() {
            guard page["page"] == .number(Decimal(index)),
                case .number(let count) = page["count"], count >= 0, count <= 100,
                Decimal(NSDecimalNumber.make(decimal: count).intValue) == count
            else {
                try refuse()
            }
            total += NSDecimalNumber.make(decimal: count).intValue
            _ = try digest(page["digest"])
        }
        guard total <= 200_000, let expiry = date(value["expiresAt"].text), expiry > now else {
            try refuse("SYNC_SNAPSHOT_EXPIRED")
        }
        return value
    }

    static func date(_ raw: String) -> Date? {
        let formatter = ISO8601DateFormatter.make()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter.date(from: raw) ?? ISO8601DateFormatter.make().date(from: raw)
    }

    static func page(_ value: IMJSON, manifest: IMJSON, index: Int) throws -> [IMJSON] {
        guard manifest["pageManifest"].list.indices.contains(index) else {
            try refuse("SYNC_MANIFEST_INCOMPLETE")
        }
        let expected = manifest["pageManifest"].list[index]
        guard value["snapshotId"] == manifest["snapshotId"],
            value["page"] == .number(Decimal(index)),
            value["restoreEpoch"] == manifest["restoreEpoch"],
            value["permissionGeneration"] == manifest["permissionGeneration"]
        else {
            try refuse("SYNC_CONTEXT_CHANGED")
        }
        guard value["installable"] == .bool(true), value["requiresFinalValidation"] == .bool(true),
            value["requiresAtomicInstall"] == .bool(true), value["qualifiedAck"] == .bool(false),
            value["digestScope"] == .string("immutable_references_only")
        else {
            try refuse("SYNC_PAGE_NOT_INSTALLABLE")
        }
        guard value["pageDigest"] == expected["digest"] else {
            try refuse("SYNC_DIGEST_MISMATCH")
        }
        let original = try envelope(value["referenceEnvelope"], digest: expected["digest"])
        let refs = try array(original)
        let items = try array(value["items"])
        guard original == value["refs"], .number(Decimal(refs.count)) == expected["count"],
            items.count == refs.count
        else {
            try refuse()
        }
        var keys = Set<String>()
        return try zip(refs, items).map { ref, item in
            guard keys.insert(try reference(ref, schema: manifest["schemaVersion"].text)).inserted else {
                try refuse("SYNC_DUPLICATE_REFERENCE")
            }
            let checked = try projection(item, reference: ref, schema: manifest["schemaVersion"].text)
            guard checked["revisionMatch"] == .bool(true),
                checked["currentRevisionEpoch"] == manifest["restoreEpoch"],
                !["redacted", "unknown"].contains(checked["state"].text)
            else {
                try refuse("SYNC_PAGE_NOT_INSTALLABLE")
            }
            return checked
        }
    }

    static func validation(_ value: IMJSON, manifest: IMJSON, now: Date = Date()) throws -> IMJSON {
        guard value["canInstall"] == .bool(true),
            value["boundaryToken"] == manifest["boundaryToken"],
            value["restoreEpoch"] == manifest["restoreEpoch"],
            value["permissionGeneration"] == manifest["permissionGeneration"],
            value["requiresLocalAtomicInstall"] == .bool(true),
            value["qualifiedAck"] == .bool(false),
            try counter(value["classificationWatermark"])
                >= counter(manifest["classificationWatermark"]),
            let expiry = date(manifest["expiresAt"].text), expiry > now
        else {
            try refuse("SYNC_SNAPSHOT_INVALIDATED")
        }
        return value["classificationWatermark"]
    }

    static func changes(_ value: IMJSON, previous: IMJSON, epoch: IMJSON, permission: IMJSON, schema: String = "1") throws
        -> IMJSON
    {
        guard value["restoreEpoch"] == epoch else {
            try refuse("RESTORE_EPOCH_CHANGED")
        }
        _ = try digest(value["permissionGeneration"])
        guard value["permissionGeneration"] == permission, value["snapshotRequired"] == .bool(false)
        else {
            try refuse("SYNC_PERMISSION_CHANGED")
        }
        let items = try array(value["items"])
        guard value["qualifiedAck"] == .bool(false), items.count <= 500 else {
            try refuse()
        }
        var next = try counter(previous) + 1
        var origins = Set<String>()
        for event in items {
            if event["kind"] == .string("redacted") {
                guard try counter(event["from"]) == next, try counter(event["to"]) >= next else {
                    try refuse("SYNC_CONTINUITY_GAP")
                }
                _ = try digest(event["rangeDigest"])
                next = try counter(event["to"]) + 1
            } else if event["kind"] == .string("reference") {
                guard try counter(event["position"]) == next,
                    origins.insert(try id(event["originEventId"])).inserted
                else {
                    try refuse("SYNC_CONTINUITY_GAP")
                }
                _ = try reference(event["ref"], schema: schema)
                let raw = try envelope(event["eventEnvelope"], digest: event["eventDigest"])
                guard
                    raw
                        == .object([
                            "position": event["position"], "originEventId": event["originEventId"],
                            "ref": event["ref"],
                        ])
                else {
                    try refuse("SYNC_DIGEST_MISMATCH")
                }
                let item = try projection(event["projection"], reference: event["ref"], schema: schema)
                guard event["ref"]["revisionEpoch"] == epoch,
                    item["currentRevisionEpoch"] == .null || item["currentRevisionEpoch"] == epoch,
                    !["unknown", "redacted"].contains(item["state"].text)
                else {
                    try refuse("SYNC_PROJECTION_UNKNOWN")
                }
                next += 1
            } else {
                try refuse()
            }
        }
        let at = try counter(value["position"])
        let head = try counter(value["head"])
        guard at == next - 1, head >= at, value["hasMore"] == .bool(at < head) else {
            try refuse("SYNC_CONTINUITY_GAP")
        }
        _ = try text(value["cursor"])
        return value
    }

    static func personalBody(_ value: IMJSON, reference ref: IMJSON) throws {
        guard value["sourceContextId"] == .string("personal"),
            try id(value["scopeId"]) != "",
            try id(value["userId"]) == ref["subjectUserId"].text,
            try counter(value["revisionEpoch"]) > 0
        else {
            try refuse("SYNC_PERSONAL_IDENTITY_MISMATCH")
        }
        let kind = ref["kind"].text
        if kind != "preferences" {
            guard try id(value["id"]) == ref["id"].text,
                try id(value["conversationId"]) == ref["conversationId"].text
            else {
                try refuse("SYNC_PERSONAL_IDENTITY_MISMATCH")
            }
        } else if ref["id"] != value["userId"] {
            try refuse("SYNC_PERSONAL_IDENTITY_MISMATCH")
        }
        if value["removed"] == .bool(true) {
            guard ref["state"] == .string("removed") else {
                try refuse()
            }
            return
        }
        _ = try counter(value["version"])
        switch kind {
        case "preferences":
            guard ["small", "standard", "large"].contains(value["fontPreset"].text),
                case .bool = value["explicit"]
            else {
                try refuse()
            }
        case "conversation_view":
            for field in ["hidden", "archived", "manualUnread"] {
                guard case .bool = value[field] else {
                    try refuse()
                }
            }
            _ = try counter(value["manualUnreadGeneration"])
            if value["pinnedRank"] != .null {
                guard case .number(let rank) = value["pinnedRank"], rank >= 0, rank <= 999999,
                    Decimal(NSDecimalNumber.make(decimal: rank).intValue) == rank
                else {
                    try refuse()
                }
            }
            if value["mutedUntil"] != .null, date(value["mutedUntil"].text) == nil {
                try refuse()
            }
        case "read_state":
            guard value["countStatus"] == .string("not_computed"),
                case .bool = value["writable"],
                try counter(value["membershipGeneration"]) > 0
            else {
                try refuse()
            }
            _ = try counter(value["readPoint"]["epoch"])
            _ = try counter(value["readPoint"]["sequence"])
        case "read_observation":
            _ = try id(value["messageId"])
            guard value["countStatus"] == .string("not_computed"),
                case .bool(let known) = value["orderKnown"],
                case .bool = value["eligibleAtObservation"],
                case .bool = value["currentlyApplicable"],
                try counter(value["membershipGeneration"]) > 0
            else {
                try refuse()
            }
            if known {
                guard try counter(value["creationEpoch"]) > 0,
                    try counter(value["creationSequence"]) > 0
                else {
                    try refuse()
                }
            } else if value["creationEpoch"] != .string("unknown") || value["creationSequence"] != .string("unknown") {
                try refuse()
            }
        case "draft":
            guard ["empty", "owned", "unavailable"].contains(value["contentState"].text),
                ["none", "available", "unavailable"].contains(value["replyState"].text),
                value["draftAdmission"] == .null, value["recovery"] == .null,
                Set(value.fields.keys).isSubset(of: ["id", "scopeId", "sourceContextId", "userId", "conversationId",
                                                     "revisionEpoch", "version", "contentState", "ownedText", "replyState", "reply", "writable", "pendingOperationId"])
            else {
                try refuse()
            }
            if value["ownedText"] != .null {
                guard case .string(let owned) = value["ownedText"], owned.utf8.count <= 16384 else {
                    try refuse()
                }
            }
            if value["reply"] != .null {
                try object(value["reply"])
                guard value["replyState"] == .string("available"),
                    Set(value["reply"].fields.keys) == Set(["messageId", "sourceVersion"]),
                    try counter(value["reply"]["sourceVersion"]) > 0
                else {
                    try refuse()
                }
                _ = try id(value["reply"]["messageId"])
            }
            if value["pendingOperationId"] != .null {
                _ = try id(value["pendingOperationId"])
            }
        default:
            try refuse()
        }
    }
}
