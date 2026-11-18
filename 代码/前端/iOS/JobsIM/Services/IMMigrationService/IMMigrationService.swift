//
//  IMMigrationService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
final class IMMigrationService {

    typealias Request = @MainActor (String, String, [String: IMJSON], String, String) async throws -> IMJSON
    typealias Binary = @MainActor (String, Bool, String, String) async throws -> Data

    private let session: IMSession
    private let current: @MainActor () -> Bool
    private let requestOverride: Request?
    private let binaryOverride: Binary?
    let namespace: IMNamespace
    let family: String
    let physical: String
    private static var working = Set<String>()

    init(
        session: IMSession = .shared,
        isCurrent: @escaping @MainActor () -> Bool = { true },
        request: Request? = nil,
        binary: Binary? = nil
    ) throws {
        self.session = session
        current = isCurrent
        requestOverride = request
        binaryOverride = binary
        namespace = session.namespace
        family = session.currentCredentialPair?.familyID ?? ""
        guard let state = try session.store.syncStateIfPresent(namespace: namespace),
            !state["streamId"].text.isEmpty, state["restriction"].text.isEmpty
        else {
            try IMMigrationProtocol.refuse("MIGRATION_SYNC_REQUIRED")
        }
        physical = try IMMigrationProtocol.id(state["storeGeneration"])
        try requireCurrent()
    }

    private var metadata: IMNamespace {
        return IMNamespace(
            profile: namespace.profile,
            account: namespace.account,
            demo: false,
            restoreEpoch: "migration-protocol",
            deploymentID: namespace.deploymentID,
            sourceContextID: "personal")
    }

    func requireCurrent() throws {
        guard !Task.isCancelled, current(), !session.demo, session.namespace == namespace,
            namespace.sourceContextID == "personal", !namespace.deviceID.isEmpty,
            !family.isEmpty, session.currentCredentialPair?.familyID == family,
            session.currentCredentialPair?.userID == namespace.account,
            session.currentCredentialPair?.deviceID == namespace.deviceID,
            let state = try session.store.syncStateIfPresent(namespace: namespace),
            state["storeGeneration"].text == physical, state["restriction"].text.isEmpty,
            state["scopeId"].text == namespace.deploymentID, state["principalId"].text == namespace.account,
            state["deviceId"].text == namespace.deviceID, state["restoreEpoch"].text == namespace.restoreEpoch,
            !state["streamId"].text.isEmpty
        else {
            try IMMigrationProtocol.refuse("MIGRATION_CONTEXT_CHANGED")
        }
    }

    private func binding() throws -> IMMigrationBinding {
        try requireCurrent()
        guard let state = try session.store.syncStateIfPresent(namespace: namespace) else {
            try IMMigrationProtocol.refuse("MIGRATION_SYNC_REQUIRED")
        }
        return IMMigrationBinding(namespace: namespace, family: family, physical: physical, guardState: state)
    }

    private func request(
        _ path: String,
        method: String = "POST",
        body: [String: IMJSON] = [:]
    ) async throws -> IMJSON {
        try requireCurrent()
        let result: IMJSON
        if let requestOverride, let token = session.token {
            result = try await requestOverride(path, method, body, token, physical)
        } else {
            result = try await session.authorized { api, token in
                try self.requireCurrent()
                if path.hasPrefix("messages/"), method == "GET" {
                    return try await api.request(path, token: token)
                }
                return try await api.migration(path, method: method, body: body, token: token, store: self.physical)
            }
        }
        try requireCurrent()
        return result
    }

    private func bytes(_ id: String, capsule: Bool) async throws -> Data {
        _ = try IMMigrationProtocol.id(.string(id))
        try requireCurrent()
        let result: Data
        if let binaryOverride, let token = session.token {
            result = try await binaryOverride(id, capsule, token, physical)
        } else {
            result = try await session.authorized { api, token in
                try self.requireCurrent()
                if capsule {
                    return try await api.migrationCapsule(id, token: token, store: self.physical)
                }
                return try await api.migrationOriginalObject(id, token: token, store: self.physical)
            }
        }
        try requireCurrent()
        guard result.count <= (capsule ? IMMigrationProtocol.limit + 4096 : 64 * 1024 * 1024) else {
            try IMMigrationProtocol.refuse("MIGRATION_LIMIT_EXCEEDED")
        }
        return result
    }

    private func exclusive<T>(_ operation: @MainActor () async throws -> T) async throws -> T {
        try requireCurrent()
        guard Self.working.insert(metadata.key).inserted else {
            try IMMigrationProtocol.refuse("MIGRATION_BUSY")
        }
        defer {
            Self.working.remove(metadata.key)
        }
        return try await operation()
    }

    func capability() async throws -> Bool {
        let value = try await request("sync/migration-capabilities", method: "GET")
        guard value["enabled"] == .bool(true), value["format"].text == IMMigrationProtocol.format,
            value["sourceContextId"].text == "personal", value["qualifiedAck"] == .bool(false),
            value["targetQualified"] == .bool(false), value["maxMessages"] == .number(256),
            value["maxObjects"] == .number(64), value["maxRequestBytes"] == .number(Decimal(IMMigrationProtocol.limit)),
            value["maxObjectBytes"] == .number(64 * 1024 * 1024), value["installLifetimeSeconds"] == .number(5)
        else {
            try IMMigrationProtocol.refuse("MIGRATION_UNAVAILABLE")
        }
        return true
    }

    func rows() throws -> [IMJSON] {
        try requireCurrent()
        let rows = try session.store.list("migration-intents", namespace: metadata)
        guard rows.count <= 1000 else {
            try IMMigrationProtocol.refuse("MIGRATION_STORE_INVALID")
        }
        let fields: Set<String> = [
            "id", "kind", "createdAt", "profile", "deploymentId", "userId", "deviceId", "familyId",
            "restoreEpoch", "namespaceKey", "storeGeneration", "state", "attempts", "selection", "sourceDigest",
            "target", "migrationId", "manifestDigest", "lastStatus",
        ]
        for row in rows {
            try IMMigrationProtocol.object(row, required: fields)
            guard row["profile"].text == namespace.profile, row["deploymentId"].text == namespace.deploymentID,
                row["userId"].text == namespace.account,
                ["pairing", "create", "target"].contains(row["kind"].text),
                ["unsent", "unknown", "ready", "installed", "blocked"].contains(row["state"].text),
                IMSyncProtocol.date(row["createdAt"].text) != nil
            else {
                try IMMigrationProtocol.refuse("MIGRATION_STORE_INVALID")
            }
            for key in ["id", "deviceId", "familyId", "storeGeneration"] {
                _ = try IMMigrationProtocol.id(row[key])
            }
        }
        return rows.sorted { $0["createdAt"].text > $1["createdAt"].text }
    }

    private func original(_ id: String) throws -> IMJSON {
        guard let row = try rows().first(where: { $0["id"].text == id }) else {
            try IMMigrationProtocol.refuse("MIGRATION_ORIGINAL_MISSING")
        }
        return row
    }

    private func save(_ next: IMJSON, previous: IMJSON?) throws {
        try requireCurrent()
        try session.store.compareMigrationIntent(namespace: metadata, previous: previous, next: next)
    }

    func mayWrite(_ row: IMJSON) -> Bool {
        return row["familyId"].text == family && row["namespaceKey"].text == namespace.key
            && row["restoreEpoch"].text == namespace.restoreEpoch && row["storeGeneration"].text == physical
    }

    private func requireOriginal(_ row: IMJSON) throws {
        try requireCurrent()
        guard mayWrite(row) else {
            try IMMigrationProtocol.refuse("MIGRATION_ORIGINAL_SESSION_REQUIRED")
        }
    }

    private func prepare(_ kind: String, selection: [IMJSON] = []) throws -> IMJSON {
        try requireCurrent()
        guard try rows().count < 1000 else {
            try IMMigrationProtocol.refuse("MIGRATION_STORE_INVALID")
        }
        let row: IMJSON = .object([
            "id": .string(UUID().uuidString.lowercased()), "kind": .string(kind),
            "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())),
            "profile": .string(namespace.profile), "deploymentId": .string(namespace.deploymentID),
            "userId": .string(namespace.account), "deviceId": .string(namespace.deviceID),
            "familyId": .string(family), "restoreEpoch": .string(namespace.restoreEpoch),
            "namespaceKey": .string(namespace.key), "storeGeneration": .string(physical),
            "state": .string("unsent"), "attempts": .number(0), "selection": .array(selection),
            "sourceDigest": .null, "target": .null, "migrationId": .null, "manifestDigest": .null, "lastStatus": .null,
        ])
        try save(row, previous: nil)
        return row
    }

    private func attempted(_ row: IMJSON) throws -> IMJSON {
        try requireOriginal(row)
        let count = Int(row["attempts"].text) ?? 10000
        let next = row.setting("attempts", .number(Decimal(count + 1))).setting("state", .string("unknown"))
        try save(next, previous: row)
        return next
    }

    static func credential(password: String, otp: String) throws -> [String: IMJSON] {
        let normalized = password.precomposedStringWithCanonicalMapping
        guard !normalized.isEmpty, normalized.utf8.count <= 512,
            otp.isEmpty || otp.range(of: "^[0-9]{6}$", options: .regularExpression) != nil
        else {
            try IMMigrationProtocol.refuse("MIGRATION_PROOF_INVALID")
        }
        var value: [String: IMJSON] = ["password": .string(normalized)]
        if !otp.isEmpty {
            value["otp"] = .string(otp)
        }
        return value
    }

    private static func canonical(_ value: IMJSON) throws -> Data {
        let first = try JSONEncoder.make().encode(value)
        return try JSONSerialization.data(
            withJSONObject: JSONSerialization.jsonObject(with: first),
            options: [.sortedKeys, .withoutEscapingSlashes])
    }

    private func peer(_ proof: String) throws -> IMJSON {
        let parts = proof.split(separator: ".", omittingEmptySubsequences: false)
        guard proof.utf8.count <= 16384, parts.count == 3, parts[0] == "mp1",
            try IMMigrationProtocol.decode(.string(String(parts[2])), maximum: 32).count == 32
        else {
            try IMMigrationProtocol.refuse()
        }
        let raw = try IMMigrationProtocol.decode(.string(String(parts[1])), maximum: 8192)
        let value = try JSONDecoder.make().decode(IMJSON.self, from: raw)
        try IMMigrationProtocol.object(
            value,
            required: [
                "pairingId", "scopeId", "userId", "targetDeviceId", "targetStoreGeneration", "restoreEpoch",
                "securityVersion", "sourceGeneration", "expires",
            ])
        guard value["scopeId"].text == namespace.deploymentID, value["userId"].text == namespace.account,
            value["restoreEpoch"].text == namespace.restoreEpoch,
            case .number = value["expires"], Int64(value["expires"].text) != nil
        else {
            try IMMigrationProtocol.refuse("MIGRATION_TARGET_CHANGED")
        }
        for key in ["pairingId", "targetDeviceId", "targetStoreGeneration"] {
            _ = try IMMigrationProtocol.id(value[key])
        }
        _ = try IMMigrationProtocol.positive(value["securityVersion"])
        _ = try IMMigrationProtocol.positive(value["sourceGeneration"])
        return value
    }

    private func pairing(_ value: IMJSON, row: IMJSON) throws -> IMJSON {
        try IMMigrationProtocol.object(
            value,
            required: [
                "pairingId", "pairingProof", "targetDeviceId", "targetStoreGeneration", "restoreEpoch", "expiresAt",
                "sourcePreserved",
            ])
        let proof = try peer(value["pairingProof"].text)
        guard value["sourcePreserved"] == .bool(true), value["pairingId"] == proof["pairingId"],
            value["targetDeviceId"] == row["deviceId"], value["targetStoreGeneration"] == row["storeGeneration"],
            value["targetDeviceId"] == proof["targetDeviceId"],
            value["targetStoreGeneration"] == proof["targetStoreGeneration"],
            value["restoreEpoch"] == row["restoreEpoch"],
            let expiry = IMSyncProtocol.date(value["expiresAt"].text),
            Int64(expiry.timeIntervalSince1970) == Int64(proof["expires"].text)
        else {
            try IMMigrationProtocol.refuse("MIGRATION_ORIGINAL_CHANGED")
        }
        let target: IMJSON = .object([
            "pairingId": value["pairingId"], "deviceId": value["targetDeviceId"],
            "storeGeneration": value["targetStoreGeneration"],
        ])
        try save(row.setting("state", .string("ready")).setting("target", target), previous: row)
        return value
    }

    private func observed(_ value: IMJSON, row: IMJSON, target: Bool) throws -> IMJSON {
        let originalNamespace = IMNamespace(
            profile: row["profile"].text, account: row["userId"].text,
            demo: false, restoreEpoch: row["restoreEpoch"].text, deploymentID: row["deploymentId"].text,
            sourceContextID: "personal", deviceID: row["deviceId"].text)
        guard originalNamespace.key == row["namespaceKey"].text else {
            try IMMigrationProtocol.refuse("MIGRATION_ORIGINAL_CHANGED")
        }
        let captured = IMMigrationBinding(
            namespace: originalNamespace, family: row["familyId"].text,
            physical: row["storeGeneration"].text, guardState: try binding().guardState)
        let view = try IMMigrationProtocol.view(value, binding: captured, target: target)
        if row["kind"].text == "create" {
            let refs = view["manifest"]["messages"].list
            guard view["requestId"] == row["id"], refs.count == row["selection"].list.count else {
                try IMMigrationProtocol.refuse("MIGRATION_ORIGINAL_CHANGED")
            }
            for (ref, selected) in zip(refs, row["selection"].list) {
                guard ref["messageId"] == selected["id"], ref["contentVersion"] == selected["contentVersion"],
                    ref["objectMode"] == selected["objectMode"], ref["objectId"] == selected["attachmentId"]
                else {
                    try IMMigrationProtocol.refuse("MIGRATION_ORIGINAL_CHANGED")
                }
            }
            if row["target"] != .null {
                guard view["manifest"]["targetDeviceId"] == row["target"]["deviceId"],
                    view["manifest"]["targetStoreGeneration"] == row["target"]["storeGeneration"]
                else {
                    try IMMigrationProtocol.refuse("MIGRATION_ORIGINAL_CHANGED")
                }
            }
        }
        let status = view["status"].text
        let state =
            row["state"].text == "installed"
            ? "installed"
            : ["revoking", "retired"].contains(status) ? "blocked" : status == "building" ? "unknown" : "ready"
        let next = row.setting("state", .string(state)).setting("migrationId", view["id"])
            .setting("manifestDigest", view["manifestDigest"]).setting("lastStatus", view["status"])
        try save(next, previous: row)
        if ["revoking", "retired"].contains(status) {
            try session.store.remove(view["id"].text, collection: "migration-stages", namespace: namespace)
        }
        return view
    }

    private func lookup(_ row: IMJSON) async throws -> IMJSON? {
        if row["kind"].text == "target" {
            guard row["migrationId"] != .null else {
                return nil
            }
            return try observed(await status(row["migrationId"].text), row: row, target: true)
        }
        let pair = row["kind"].text == "pairing"
        let value = try await request(
            pair ? "sync/migration-pairing-results" : "sync/migration-results",
            body: ["requestId": row["id"], pair ? "storeGeneration" : "sourceStoreGeneration": row["storeGeneration"]])
        if value == .object(["status": .string("not_found")]) {
            return nil
        }
        return try pair ? pairing(value, row: row) : observed(value, row: row, target: false)
    }

    func check(_ id: String) async throws -> IMJSON? {
        return try await exclusive {
            return try await self.lookup(self.original(id))
        }
    }

    func pair(password: String, otp: String, originalID: String? = nil) async throws -> IMJSON {
        return try await exclusive {
            var row = try originalID.map(self.original) ?? self.prepare("pairing")
            guard row["kind"].text == "pairing" else {
                try IMMigrationProtocol.refuse()
            }
            if let prior = try await self.lookup(row) {
                return prior
            }
            try self.requireOriginal(row)
            _ = try await self.capability()
            var body = try Self.credential(password: password, otp: otp)
            body["requestId"] = row["id"]
            body["storeGeneration"] = row["storeGeneration"]
            body["restoreEpoch"] = row["restoreEpoch"]
            row = try self.attempted(row)
            return try self.pairing(await self.request("sync/migration-pairings", body: body), row: row)
        }
    }

    func candidates() throws -> [IMJSON] {
        try requireCurrent()
        let queued = Set(
            try session.store.allOutbox(namespace: namespace).map { $0["clientMsgId"].text }.filter { !$0.isEmpty })
        var messages: [IMJSON] = []
        for conversation in try session.store.list("conversations", namespace: namespace) {
            for message in try session.store.list("messages:" + conversation["id"].text, namespace: namespace) {
                guard message["deleted"] != .bool(true),
                    message["contentUnavailable"] != .bool(true) || message["forwarded"] == .bool(true),
                    message["state"].text != "pending", message["pending"] != .bool(true),
                    message["frozen"] != .bool(true), !queued.contains(message["clientMsgId"].text),
                    ["text", "image", "audio", "video", "file"].contains(message["type"].text),
                    (try? IMMigrationProtocol.positive(message["contentVersion"])) != nil
                else {
                    continue
                }
                messages.append(message)
            }
        }
        for source in try qualifiedSources() where !messages.contains(where: { $0["id"] == source.message["id"] }) {
            messages.append(source.message)
        }
        return messages.sorted { $0["createdAt"].text > $1["createdAt"].text }
    }

    private func qualifiedSources() throws -> [(message: IMJSON, receipt: IMJSON, envelope: String)] {
        try requireCurrent()
        let denied = Set(try session.store.syncGlobalIDs(namespace: namespace))
        let conversations = try session.store.list("conversations", namespace: namespace)
        let bodies = try session.store.list("receipt-bodies", namespace: namespace)
        var result: [(message: IMJSON, receipt: IMJSON, envelope: String)] = []
        for receipt in try session.store.list("receipt-intents", namespace: namespace)
        where receipt["kind"].text == "content" && receipt["state"].text == "completed"
        {
            try IMReceiptProtocol.localIntent(receipt, namespace: namespace, physical: physical)
            guard !denied.contains(receipt["messageId"].text),
                let conversation = conversations.first(where: { $0["id"] == receipt["conversationId"] }),
                ["active", "owner_unavailable"].contains(conversation["state"].text),
                conversation["membershipGeneration"] == receipt["membershipGeneration"],
                let body = bodies.first(where: { $0["id"] == receipt["id"] }), body["messageId"] == receipt["messageId"],
                body["bodyDigest"] == receipt["bodyDigest"] else {
                continue
            }
            let bytes = try IMMigrationProtocol.decode(body["bodyEnvelope"], maximum: 2 * 1024 * 1024)
            guard IMMigrationProtocol.hash(bytes) == receipt["bodyDigest"].text else {
                try IMMigrationProtocol.refuse("MIGRATION_SOURCE_CHANGED")
            }
            var wrapped = Data("{\"messages\":[".utf8)
            wrapped.append(bytes)
            wrapped.append(Data("]}".utf8))
            _ = try IMMigrationJSON.messageRanges(wrapped)
            let message = try JSONDecoder.make().decode(IMJSON.self, from: bytes)
            guard message["id"] == receipt["messageId"], message["conversationId"] == receipt["conversationId"],
                message["contentVersion"] == receipt["contentVersion"], message["deleted"] != .bool(true),
                message["contentUnavailable"] != .bool(true), message["forwarded"] != .bool(true),
                message["forwardUnavailable"] != .bool(true),
                ["text", "image", "audio", "video", "file"].contains(message["type"].text) else {
                try IMMigrationProtocol.refuse("MIGRATION_SOURCE_CHANGED")
            }
            if let current = try session.store.list("messages:" + receipt["conversationId"].text, namespace: namespace)
                .first(where: { $0["id"] == message["id"] })
            {
                guard current["contentVersion"] == message["contentVersion"], current["deleteScope"].text != "global" else {
                    continue
                }
            }
            result.append((message, receipt, body["bodyEnvelope"].text))
        }
        return result
    }

    private func qualifiedSource(_ selected: IMJSON, original: IMJSON) async throws -> (IMJSON, IMJSON?) {
        guard selected["forwarded"] != .bool(true),
            let source = try qualifiedSources().first(where: {
                $0.message["id"] == selected["id"] && $0.message["contentVersion"] == selected["contentVersion"]
                    && $0.message["conversationId"] == selected["conversationId"] && $0.message["type"] == selected["type"]
                    && $0.message["attachmentId"] == selected["attachmentId"]
            }) else {
            try IMMigrationProtocol.refuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
        }
        let receipts = try IMReceiptService(session: session, isCurrent: current, request: requestOverride)
        do {
            let target = try await receipts.target(selected["id"].text, qualificationID: source.receipt["qualificationId"].text)
            try requireOriginal(original)
            guard target["messageCurrent"] == .bool(true), target["contentRecorded"] == .bool(true),
                target["qualificationId"] == source.receipt["qualificationId"],
                target["contentVersion"] == selected["contentVersion"],
                target["deliveryGeneration"].text == source.receipt["deliveryGeneration"].text,
                target["manifestDigest"].text == source.receipt["manifestDigest"].text,
                selected["objectMode"].text != "full" || target["objectRecorded"] == .bool(true) else {
                try IMMigrationProtocol.refuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
            }
        } catch let error as IMAPIError where error.status == 404 && error.code == "RECEIPT_TARGET_NOT_FOUND"
            && source.receipt["deliveryGeneration"].text.isEmpty && source.receipt["manifestDigest"].text.isEmpty {
            // Legacy receipts have no invented delivery tuple. Only their exact
            // still-current permission view can authorize the original bytes.
            let state = try binding().guardState
            let schema = state["schemaVersion"].text == "2" ? "2" : "1"
            let path = "sync/context?schemaVersion=" + schema
            let view: IMJSON
            if let requestOverride, let token = session.token {
                view = try await requestOverride(path, "GET", [:], token, physical)
            } else {
                view = try await session.authorized { api, token in
                    try self.requireOriginal(original)
                    return try await api.request(path, token: token)
                }
            }
            try requireOriginal(original)
            guard view["scopeId"].text == namespace.deploymentID, view["principalId"].text == namespace.account,
                view["deviceId"].text == namespace.deviceID, view["restoreEpoch"].text == namespace.restoreEpoch,
                view["storeGeneration"].text == physical, view["permissionGeneration"] == source.receipt["permissionGeneration"] else {
                try IMMigrationProtocol.refuse("MIGRATION_QUALIFIED_SOURCE_REQUIRED")
            }
        }
        let current = try qualifiedSources().first(where: { $0.receipt["id"] == source.receipt["id"] })
        guard current?.receipt == source.receipt, current?.envelope == source.envelope else {
            try IMMigrationProtocol.refuse("MIGRATION_SOURCE_CHANGED")
        }
        let message: IMJSON = .object(["bodyEnvelope": .string(source.envelope), "objectMode": selected["objectMode"],
            "sourceReceiptId": source.receipt["id"]])
        guard selected["objectMode"].text == "full" else {
            return (message, nil)
        }
        guard let object = try session.store.list("receipt-intents", namespace: namespace).first(where: {
                $0["kind"].text == "object" && $0["state"].text == "completed"
                    && $0["originalBody"]["parentReceiptId"] == source.receipt["id"]
                    && $0["originalBody"]["objectId"] == selected["attachmentId"]
            }) else {
            try IMMigrationProtocol.refuse("MIGRATION_QUALIFIED_OBJECT_REQUIRED")
        }
        try IMReceiptProtocol.localIntent(object, namespace: namespace, physical: physical)
        guard object["messageId"] == source.receipt["messageId"], object["contentVersion"] == source.receipt["contentVersion"],
            object["qualificationId"] == source.receipt["qualificationId"], object["bodyDigest"] == source.receipt["bodyDigest"],
            object["membershipGeneration"] == source.receipt["membershipGeneration"],
            object["permissionGeneration"] == source.receipt["permissionGeneration"],
            object["deliveryGeneration"] == source.receipt["deliveryGeneration"], object["manifestDigest"] == source.receipt["manifestDigest"] else {
            try IMMigrationProtocol.refuse("MIGRATION_QUALIFIED_OBJECT_REQUIRED")
        }
        var bytes = try session.store.receiptObject(namespace: namespace, requestId: object["id"].text)
        defer {
            bytes.resetBytes(in: 0..<bytes.count)
        }
        guard String(bytes.count) == object["originalBody"]["sizeBytes"].text,
            IMMigrationProtocol.hash(bytes) == object["originalBody"]["objectDigest"].text else {
            try IMMigrationProtocol.refuse("MIGRATION_QUALIFIED_OBJECT_REQUIRED")
        }
        return (message, .object(["objectId": selected["attachmentId"], "messageId": selected["id"],
            "bytes": .string(IMMigrationProtocol.base64(bytes, url: false)), "sourceObjectReceiptId": object["id"]]))
    }

    private static func selection(_ message: IMJSON, originals: Bool) throws -> IMJSON {
        let forwarded = message["forwarded"] == .bool(true)
        let media = message["type"].text != "text"
        _ = try IMMigrationProtocol.positive(message["contentVersion"])
        var value: [String: IMJSON] = [
            "id": .string(try IMMigrationProtocol.id(message["id"])),
            "conversationId": .string(try IMMigrationProtocol.id(message["conversationId"])),
            "contentVersion": message["contentVersion"], "type": message["type"],
            "objectMode": .string(!media ? "none" : originals && !forwarded ? "full" : "metadata_only"),
        ]
        if media && !forwarded {
            value["attachmentId"] = .string(try IMMigrationProtocol.id(message["attachmentId"]))
        }
        if forwarded {
            value["forwarded"] = .bool(true)
        }
        return .object(value)
    }

    func prepareSource(ids: Set<String>, originals: Bool) throws -> IMJSON {
        guard !ids.isEmpty, ids.count <= 256 else {
            try IMMigrationProtocol.refuse("MIGRATION_LIMIT_EXCEEDED")
        }
        let messages = try candidates().filter { ids.contains($0["id"].text) }
        guard messages.count == ids.count else {
            try IMMigrationProtocol.refuse("MIGRATION_SOURCE_CHANGED")
        }
        return try prepare("create", selection: messages.map { try Self.selection($0, originals: originals) })
    }

    private func sourceData(_ row: IMJSON) async throws -> IMJSON {
        var messages: [IMJSON] = []
        var objects: [IMJSON] = []
        var size = 0
        for selected in row["selection"].list {
            try requireOriginal(row)
            let message = try await request("messages/" + IMMigrationProtocol.id(selected["id"]), method: "GET")
            if message["deleteScope"].text == "cloud_only", message["id"] == selected["id"],
                message["conversationId"] == selected["conversationId"], message["contentVersion"] == selected["contentVersion"] {
                let (body, object) = try await qualifiedSource(selected, original: row)
                messages.append(body)
                size += body["bodyEnvelope"].text.utf8.count
                if let object {
                    objects.append(object)
                    size += object["bytes"].text.utf8.count
                }
                guard objects.count <= 64, size <= IMMigrationProtocol.limit - 32768 else {
                    try IMMigrationProtocol.refuse("MIGRATION_LIMIT_EXCEEDED")
                }
                continue
            }
            guard message["id"] == selected["id"], message["conversationId"] == selected["conversationId"],
                message["contentVersion"] == selected["contentVersion"], message["type"] == selected["type"],
                message["deleted"] == .bool(false), message["contentUnavailable"] != .bool(true),
                message["forwardUnavailable"] != .bool(true),
                (message["forwarded"] == .bool(true)) == (selected["forwarded"] == .bool(true)),
                selected["forwarded"] == .bool(true) || message["attachmentId"] == selected["attachmentId"],
                case .string = message["text"], message["text"].text.utf8.count <= 16384,
                IMSyncProtocol.date(message["createdAt"].text) != nil
            else {
                try IMMigrationProtocol.refuse("MIGRATION_SOURCE_CHANGED")
            }
            var original: [String: IMJSON] = [:]
            for field in [
                "id", "conversationId", "senderId", "type", "text", "createdAt", "contentVersion", "creationEpoch",
                "creationSequence",
            ] {
                original[field] = message[field]
            }
            _ = try IMMigrationProtocol.id(message["senderId"])
            _ = try IMMigrationProtocol.positive(message["creationEpoch"])
            _ = try IMMigrationProtocol.positive(message["creationSequence"])
            if message["replyToId"] != .null {
                original["replyToId"] = .string(try IMMigrationProtocol.id(message["replyToId"]))
            }
            if selected["attachmentId"] != .null {
                original["attachmentId"] = selected["attachmentId"]
            }
            if selected["forwarded"] == .bool(true) {
                original["forwarded"] = .bool(true)
            }
            let envelope = IMMigrationProtocol.base64(try Self.canonical(.object(original)))
            size += envelope.utf8.count
            messages.append(.object(["bodyEnvelope": .string(envelope), "objectMode": selected["objectMode"]]))
            if selected["objectMode"].text == "full" {
                let body = try await bytes(selected["attachmentId"].text, capsule: false)
                let encoded = IMMigrationProtocol.base64(body, url: false)
                size += encoded.utf8.count
                objects.append(
                    .object([
                        "objectId": selected["attachmentId"], "messageId": selected["id"], "bytes": .string(encoded),
                    ]))
            }
            guard objects.count <= 64, size <= IMMigrationProtocol.limit - 32768 else {
                try IMMigrationProtocol.refuse("MIGRATION_LIMIT_EXCEEDED")
            }
        }
        return .object(["messages": .array(messages), "objects": .array(objects)])
    }

    func create(_ id: String, pairingProof: String, password: String, otp: String) async throws -> IMJSON {
        return try await exclusive {
            var row = try self.original(id)
            guard row["kind"].text == "create" else {
                try IMMigrationProtocol.refuse()
            }
            if let prior = try await self.lookup(row) {
                return prior
            }
            try self.requireOriginal(row)
            guard row["state"].text != "blocked" else {
                try IMMigrationProtocol.refuse("MIGRATION_ORIGINAL_CHANGED")
            }
            _ = try await self.capability()
            let peer = try self.peer(pairingProof)
            guard let expires = Int64(peer["expires"].text), peer["targetDeviceId"].text != self.namespace.deviceID,
                Double(expires) > Date().timeIntervalSince1970, Double(expires) <= Date().timeIntervalSince1970 + 301
            else {
                try IMMigrationProtocol.refuse("MIGRATION_PAIRING_EXPIRED")
            }
            let target: IMJSON = .object([
                "pairingId": peer["pairingId"], "deviceId": peer["targetDeviceId"],
                "storeGeneration": peer["targetStoreGeneration"],
            ])
            var next = row.setting("target", target)
            try self.save(next, previous: row)
            row = next
            let data = try await self.sourceData(row)
            next = row.setting("sourceDigest", .string(IMMigrationProtocol.hash(try Self.canonical(data))))
            try self.save(next, previous: row)
            row = next
            var body = data.fields
            for (key, value) in try Self.credential(password: password, otp: otp) {
                body[key] = value
            }
            body["requestId"] = row["id"]
            body["sourceContextId"] = .string("personal")
            body["sourceStoreGeneration"] = row["storeGeneration"]
            body["restoreEpoch"] = row["restoreEpoch"]
            body["targetPairingProof"] = .string(pairingProof)
            guard try Self.canonical(.object(body)).count <= IMMigrationProtocol.limit else {
                try IMMigrationProtocol.refuse("MIGRATION_LIMIT_EXCEEDED")
            }
            row = try self.attempted(row)
            return try self.observed(await self.request("sync/migrations", body: body), row: row, target: false)
        }
    }

    func status(_ id: String) async throws -> IMJSON {
        let valid = try IMMigrationProtocol.id(.string(id))
        return try await request("sync/migrations/" + valid, method: "GET")
    }

    private func observeTarget(_ id: String) async throws -> (IMJSON, IMJSON) {
        let value = try IMMigrationProtocol.view(await status(id), binding: binding(), target: true)
        let previous = try rows().first {
            $0["kind"].text == "target" && $0["migrationId"] == value["id"] && mayWrite($0)
        }
        let row = try previous ?? prepare("target")
        let view = try observed(value, row: row, target: true)
        return (try original(row["id"].text), view)
    }

    func target(_ id: String) async throws -> IMJSON {
        return try await exclusive {
            let (_, view) = try await self.observeTarget(id)
            return view
        }
    }

    func capsule(_ id: String) async throws -> Data {
        let view = try IMMigrationProtocol.view(await status(id), binding: binding(), target: true)
        guard view["status"].text == "ready", let expiry = IMSyncProtocol.date(view["expiresAt"].text), expiry > Date()
        else {
            try IMMigrationProtocol.refuse("MIGRATION_DELIVERY_CLOSED")
        }
        let data = try await bytes(id, capsule: true)
        guard data.count == Int(view["sizeBytes"].text), IMMigrationProtocol.hash(data) == view["capsuleSHA256"].text
        else {
            try IMMigrationProtocol.refuse("MIGRATION_CAPSULE_INVALID")
        }
        return data
    }

    func install(_ id: String, password: String, otp: String, file: Data? = nil) async throws -> Bool {
        return try await exclusive {
            _ = try await self.capability()
            let (row, view) = try await self.observeTarget(id)
            if view["status"].text == "installed_reported" {
                guard
                    try self.archives().contains(where: {
                        $0["id"] == view["id"] && $0["manifestDigest"] == view["manifestDigest"]
                    })
                else {
                    try IMMigrationProtocol.refuse("MIGRATION_DELIVERY_CLOSED")
                }
                return true
            }
            guard view["status"].text == "ready" else {
                try IMMigrationProtocol.refuse("MIGRATION_DELIVERY_CLOSED")
            }
            let path = "sync/migrations/" + view["id"].text
            var capsule: Data
            if let file {
                capsule = file
            } else if let cached = try self.session.store.list("migration-stages", namespace: self.namespace)
                .first(where: { $0["id"] == view["id"] })
            {
                guard cached["manifestDigest"] == view["manifestDigest"],
                    cached["capsuleSHA256"] == view["capsuleSHA256"],
                    cached["storeGeneration"].text == self.physical, cached["expiresAt"] == view["expiresAt"],
                    cached["messageIds"].list == view["manifest"]["messages"].list.map({ $0["messageId"] })
                else {
                    try IMMigrationProtocol.refuse("MIGRATION_STAGE_INVALID")
                }
                capsule = try IMMigrationProtocol.decode(cached["capsule"], maximum: IMMigrationProtocol.limit + 4096)
            } else {
                capsule = try await self.capsule(id)
            }
            defer {
                capsule.resetBytes(in: 0..<capsule.count)
            }
            guard capsule.count == Int(view["sizeBytes"].text), capsule.count <= IMMigrationProtocol.limit + 4096,
                IMMigrationProtocol.hash(capsule) == view["capsuleSHA256"].text
            else {
                try IMMigrationProtocol.refuse("MIGRATION_CAPSULE_INVALID")
            }
            let staging = try self.binding()
            try self.session.store.stageMigrationCapsule(
                namespace: self.namespace, expected: staging.guardState,
                id: view["id"].text, manifestDigest: view["manifestDigest"].text,
                capsuleSHA256: view["capsuleSHA256"].text,
                expiresAt: view["expiresAt"].text,
                messageIDs: view["manifest"]["messages"].list.map({ $0["messageId"].text }), capsule: capsule)
            var proof = try Self.credential(password: password, otp: otp)
            proof["manifestDigest"] = view["manifestDigest"]
            let key = try await self.request(path + "/key", body: proof)
            let captured = try self.binding()
            let packet = try IMMigrationProtocol.open(view, capsule: capsule, key: key, binding: captured)
            let validation = try await self.request(
                path + "/validate", body: ["manifestDigest": view["manifestDigest"]])
            guard validation["id"] == view["id"], validation["manifestDigest"] == view["manifestDigest"],
                validation["status"].text == "validated", validation["requiresAtomicLocalCommit"] == .bool(true),
                validation["targetQualified"] == .bool(false), validation["qualifiedAck"] == .bool(false),
                let before = IMSyncProtocol.date(validation["installBefore"].text), before > Date(),
                before.timeIntervalSinceNow <= 5, let expiry = IMSyncProtocol.date(view["expiresAt"].text),
                before <= expiry
            else {
                try IMMigrationProtocol.refuse("MIGRATION_INSTALL_EXPIRED")
            }
            let parts = validation["installToken"].text.split(separator: ".", omittingEmptySubsequences: false)
            guard parts.count == 3, parts[0] == "mi1",
                try IMMigrationProtocol.decode(.string(String(parts[2])), maximum: 32).count == 32
            else {
                try IMMigrationProtocol.refuse()
            }
            let token = try JSONDecoder.make().decode(
                IMJSON.self,
                from: IMMigrationProtocol.decode(.string(String(parts[1])), maximum: 8192))
            guard token["id"] == view["id"], token["manifestDigest"] == view["manifestDigest"],
                token["userId"].text == self.namespace.account, token["deviceId"].text == self.namespace.deviceID,
                token["storeGeneration"].text == self.physical,
                token["restoreEpoch"].text == self.namespace.restoreEpoch,
                Int64(token["expires"].text) == Int64(before.timeIntervalSince1970)
            else {
                try IMMigrationProtocol.refuse("MIGRATION_CONTEXT_CHANGED")
            }
            try self.requireCurrent()
            // No suspension is allowed between final authorization and the FULL local commit.
            try self.session.store.commitMigrationArchive(
                namespace: self.namespace, expected: captured.guardState,
                manifest: view["manifest"], digest: view["manifestDigest"].text, packet: packet, installBefore: before)
            let latest = try self.original(row["id"].text)
            try self.save(latest.setting("state", .string("installed")), previous: latest)
            do {
                let response = try await self.request(
                    path + "/installed",
                    body: ["manifestDigest": view["manifestDigest"], "installToken": validation["installToken"]])
                guard response["id"] == view["id"], response["manifestDigest"] == view["manifestDigest"],
                    response["status"].text == "installed_reported", response["sourcePreserved"] == .bool(true),
                    response["qualifiedAck"] == .bool(false), response["targetQualified"] == .bool(false),
                    response["serverVerifiedLocalDurability"] == .bool(false)
                else {
                    try IMMigrationProtocol.refuse()
                }
                return true
            } catch {
                try self.requireCurrent()
                return false
            }
        }
    }

    func cancel(_ id: String, target: Bool) async throws -> IMJSON {
        return try await exclusive {
            let view = try IMMigrationProtocol.view(await self.status(id), binding: self.binding(), target: target)
            let value = try IMMigrationProtocol.view(
                await self.request(
                    "sync/migrations/" + view["id"].text + "/cancel",
                    body: ["manifestDigest": view["manifestDigest"]]), binding: self.binding(), target: target)
            for row in try self.rows() where row["migrationId"] == value["id"] {
                _ = try self.observed(value, row: row, target: row["kind"].text == "target")
            }
            return value
        }
    }

    func archives() throws -> [IMJSON] {
        try requireCurrent()
        return try session.store.list("migration-archives", namespace: namespace)
            .sorted { $0["createdAt"].text > $1["createdAt"].text }
    }

    func messages(_ archive: String) async throws -> [IMJSON] {
        _ = try IMMigrationProtocol.id(.string(archive))
        try requireCurrent()
        let collection = "migration-messages:" + archive
        var verified: [String: IMJSON] = [:]
        for message in try session.store.list(collection, namespace: namespace)
        where message["forwarded"] == .bool(true) && message["contentUnavailable"] != .bool(true)
            && message["deleted"] != .bool(true)
        {
            do {
                let value = try await request("messages/" + IMMigrationProtocol.id(message["id"]), method: "GET")
                guard value["id"] == message["id"], value["conversationId"] == message["conversationId"],
                    value["senderId"] == message["senderId"], value["type"] == message["type"],
                    value["forwarded"] == .bool(true), value["contentVersion"] == message["contentVersion"],
                    value["text"] == message["text"],
                    value["deleted"] != .bool(true), value["contentUnavailable"] != .bool(true),
                    value["forwardUnavailable"] != .bool(true)
                else {
                    try IMMigrationProtocol.refuse("MIGRATION_SOURCE_CHANGED")
                }
                verified[message["id"].text] = message
            } catch {
                try requireCurrent()
                try session.store.redactMigrationForward(message["id"].text, namespace: namespace)
            }
        }
        try requireCurrent()
        let global = Set(try session.store.syncGlobalIDs(namespace: namespace))
        // A lookup may suspend while another projection clears this independent holder.
        return try session.store.list(collection, namespace: namespace).map { message in
            if global.contains(message["id"].text) {
                return message.setting("text", .string("")).setting("attachmentId", .null)
                    .setting("deleted", .bool(true)).setting("deleteScope", .string("global"))
                    .setting("contentUnavailable", .bool(true)).setting("objectMode", .string("metadata_only"))
            }
            guard message["forwarded"] != .bool(true) || verified[message["id"].text] == message else {
                return message.setting("text", .string("")).setting("attachmentId", .null)
                    .setting("contentUnavailable", .bool(true))
            }
            return message
        }
    }

    func object(_ id: String, archive: String) throws -> Data {
        try requireCurrent()
        _ = try IMMigrationProtocol.id(.string(id))
        _ = try IMMigrationProtocol.id(.string(archive))
        guard
            let row = try session.store.list("migration-objects:" + archive, namespace: namespace)
                .first(where: { $0["objectId"].text == id })
        else {
            try IMMigrationProtocol.refuse("MIGRATION_OBJECT_UNAVAILABLE")
        }
        return try IMMigrationProtocol.decode(row["bytes"], maximum: 64 * 1024 * 1024, url: false)
    }
}
