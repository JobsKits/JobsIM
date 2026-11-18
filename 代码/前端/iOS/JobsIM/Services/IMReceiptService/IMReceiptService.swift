//
//  IMReceiptService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
final class IMReceiptService {

    typealias Request = @MainActor (String, String, [String: IMJSON], String, String) async throws -> IMJSON
    typealias Binary = @MainActor (String, String, String) async throws -> Data
    private let session: IMSession
    private let current: @MainActor () -> Bool
    private let requestOverride: Request?
    private let binaryOverride: Binary?
    private static var working = Set<String>()
    let namespace: IMNamespace
    let family: String
    let physical: String
    private(set) var verification: IMJSON?
    private(set) var approvedProfiles: [IMJSON] = []

    init(
        session: IMSession = .shared, isCurrent: @escaping @MainActor () -> Bool = { true },
        request: Request? = nil, binary: Binary? = nil
    ) throws {
        self.session = session
        current = isCurrent
        requestOverride = request
        binaryOverride = binary
        namespace = session.namespace
        family = session.currentCredentialPair?.familyID ?? ""
        guard let state = try session.store.syncStateIfPresent(namespace: namespace), !state["streamId"].text.isEmpty
        else {
            try IMReceiptProtocol.fail("RECEIPT_SYNC_REQUIRED")
        }
        physical = try IMReceiptProtocol.id(state["storeGeneration"])
        _ = try stateNow()
        try session.store.recoverReceiptFiles(namespace: namespace)
    }

    func stateNow(writable: Bool = false) throws -> IMJSON {
        guard !Task.isCancelled, current(), !session.demo, session.namespace == namespace,
            namespace.sourceContextID == "personal", !family.isEmpty,
            session.currentCredentialPair?.familyID == family,
            session.currentCredentialPair?.userID == namespace.account,
            session.currentCredentialPair?.deviceID == namespace.deviceID,
            let state = try session.store.syncStateIfPresent(namespace: namespace),
            state["storeGeneration"].text == physical, !state["streamId"].text.isEmpty,
            state["scopeId"].text == namespace.deploymentID, state["principalId"].text == namespace.account,
            state["deviceId"].text == namespace.deviceID, state["restoreEpoch"].text == namespace.restoreEpoch,
            !writable || state["restriction"].text.isEmpty
        else {
            try IMReceiptProtocol.fail("RECEIPT_CONTEXT_CHANGED")
        }
        return state
    }

    private func schema(_ state: IMJSON) throws -> String {
        let value = state["schemaVersion"] == .null ? "1" : state["schemaVersion"].text
        guard ["1", "2"].contains(value) else {
            try IMReceiptProtocol.fail()
        }
        return value
    }

    private func request(_ path: String, method: String = "POST", body: [String: IMJSON] = [:]) async throws -> IMJSON {
        _ = try stateNow()
        let value: IMJSON
        if let requestOverride, let token = session.token {
            value = try await requestOverride(path, method, body, token, physical)
        } else {
            value = try await session.authorized { api, token in
                _ = try self.stateNow()
                return try await api.receipt(path, method: method, body: body, token: token, store: self.physical)
            }
        }
        _ = try stateNow()
        return value
    }

    private func exclusive<T>(_ action: @MainActor () async throws -> T) async throws -> T {
        _ = try stateNow()
        guard Self.working.insert(namespace.key).inserted else {
            try IMReceiptProtocol.fail("RECEIPT_BUSY")
        }
        defer {
            Self.working.remove(namespace.key)
        }
        return try await action()
    }

    func context() async throws -> IMJSON {
        let state = try stateNow()
        let version = try schema(state)
        return try IMReceiptProtocol.context(
            await request("sync/receipt-context?schemaVersion=" + version, method: "GET"),
            namespace: namespace, physical: physical, schema: version)
    }

    func target(_ messageID: String, qualificationID: String? = nil) async throws -> IMJSON {
        _ = try IMReceiptProtocol.id(.string(messageID))
        let suffix: String
        if let qualificationID {
            suffix = "&qualificationId=" + (try IMReceiptProtocol.id(.string(qualificationID)))
        } else {
            suffix = ""
        }
        let value = try await request(
            "sync/receipt-targets/" + messageID + "?schemaVersion=" + schema(stateNow()) + suffix, method: "GET")
        guard value["protocol"].text == IMReceiptProtocol.version, value["messageId"].text == messageID,
            value["restoreEpoch"].text == namespace.restoreEpoch, value["deletionEligible"] == .bool(false),
            ["all_target_devices", "per_user_any_device"].contains(value["receiptScope"].text)
        else {
            try IMReceiptProtocol.fail()
        }
        guard case .string = value["qualificationId"],
            qualificationID == nil || value["qualificationId"].text == qualificationID
        else {
            try IMReceiptProtocol.fail()
        }
        if !value["qualificationId"].text.isEmpty {
            _ = try IMReceiptProtocol.id(value["qualificationId"])
        }
        _ = try IMReceiptProtocol.counter(value["contentVersion"])
        _ = try IMReceiptProtocol.counter(value["deliveryGeneration"])
        _ = try IMReceiptProtocol.hash(value["manifestDigest"])
        for field in [
            "originalTarget", "contentTarget", "objectTarget", "contentRecorded", "objectRecorded", "current",
            "messageCurrent",
        ] {
            _ = try IMReceiptProtocol.flag(value[field])
        }
        guard
            value["contentTarget"] != .bool(true)
                || value["originalTarget"] == .bool(true) && value["current"] == .bool(true),
            value["objectTarget"] != .bool(true) || value["contentTarget"] == .bool(true)
        else {
            try IMReceiptProtocol.fail()
        }
        for field in ["contentCompleteAt", "objectCompleteAt"]
        where value[field] != .null && IMSyncProtocol.date(value[field].text) == nil {
            try IMReceiptProtocol.fail()
        }
        return value
    }

    func verify(includeObjects: Bool) async throws -> IMJSON {
        return try await exclusive {
            self.verification = nil
            self.approvedProfiles = []
            _ = try self.stateNow(writable: true)
            guard let executable = Bundle.main.executableURL else {
                try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
            }
            let measurement = try await Task.detached(priority: .utility) {
                return try IMReceiptBuild.measure(bundle: Bundle.main.bundleURL, executable: executable)
            }.value
            let build = measurement.digest
            let expected = try self.stateNow(writable: true)
            let report = try self.session.store.verifyReceiptDurability(
                namespace: self.namespace, expected: expected, buildDigest: build, includeObjects: includeObjects)
                .setting("buildMeasurement", .string("ios-bundle-code-v1"))
                .setting("buildComponents", .array(measurement.components))
            try self.validateReport(report, currentBuild: build)
            self.verification = report
            let profiles = try IMReceiptProtocol.profiles(
                await self.request(
                    "sync/receipt-profiles?schemaVersion=" + self.schema(expected) + "&buildDigest=" + build,
                    method: "GET"), report: report)
            try self.validateReport(report, currentBuild: build)
            self.verification = report
            self.approvedProfiles = profiles
            return report
        }
    }

    private func validateReport(_ report: IMJSON, currentBuild: String? = nil) throws {
        _ = try stateNow(writable: true)
        guard report["verificationVersion"].text == "ios-receipt-durability-v1",
            report["storeGeneration"].text == physical, report["restoreEpoch"].text == namespace.restoreEpoch,
            report["qualifiedAck"] == .bool(false),
            currentBuild == nil || report["buildDigest"].text == currentBuild,
            let time = IMSyncProtocol.date(report["verifiedAt"].text), time <= Date(),
            Date().timeIntervalSince(time) <= 3600,
            IMReceiptProtocol.contracts.contains(report["storageContract"].text)
        else {
            try IMReceiptProtocol.fail("RECEIPT_LOCAL_VERIFICATION_REQUIRED")
        }
        _ = try IMReceiptProtocol.hash(report["buildDigest"])
        _ = try IMReceiptProtocol.counter(report["guardGeneration"])
        let objects = report["storageContract"].text == IMReceiptProtocol.contracts[1]
        let checks = report["checks"]
        try IMReceiptProtocol.exact(
            checks,
            [
                "wal", "synchronousFull", "fullfsync", "checkpointFullfsync", "atomicCommit",
                "independentConnectionRead", "completeOriginalRead", "fileFsync", "directoryFsync",
            ])
        for field in [
            "wal", "synchronousFull", "fullfsync", "checkpointFullfsync", "atomicCommit", "independentConnectionRead",
        ]
        where checks[field] != .bool(true) {
            try IMReceiptProtocol.fail()
        }
        for field in ["completeOriginalRead", "fileFsync", "directoryFsync"] where checks[field] != .bool(objects) {
            try IMReceiptProtocol.fail()
        }
        let stable: IMJSON = .object([
            "verificationVersion": report["verificationVersion"], "buildDigest": report["buildDigest"],
            "storageContract": report["storageContract"], "checks": checks,
        ])
        let encoder = JSONEncoder.make()
        encoder.outputFormatting = [.sortedKeys]
        guard IMMigrationProtocol.hash(try encoder.encode(stable)) == report["verificationDigest"].text else {
            try IMReceiptProtocol.fail()
        }
    }

    func refreshProfiles() async throws -> [IMJSON] {
        guard let report = verification else {
            return []
        }
        try validateReport(report)
        approvedProfiles = []
        let values = try IMReceiptProtocol.profiles(
            await request(
                "sync/receipt-profiles?schemaVersion=" + schema(stateNow()) + "&buildDigest="
                    + report["buildDigest"].text,
                method: "GET"), report: report)
        try validateReport(report)
        approvedProfiles = values
        return values
    }

    func locallyQualified(_ context: IMJSON, objects: Bool = false) -> Bool {
        return (try? requireLocal(context, objects: objects)) != nil
    }

    private func requireLocal(_ context: IMJSON, objects: Bool) throws {
        guard let report = verification else {
            try IMReceiptProtocol.fail("RECEIPT_LOCAL_VERIFICATION_REQUIRED")
        }
        try validateReport(report)
        let storage = context["storage"]
        guard storage[objects ? "objectQualified" : "qualified"] == .bool(true),
            storage["buildDigest"] == report["buildDigest"],
            storage["verificationDigest"] == report["verificationDigest"],
            storage["storageContract"] == report["storageContract"],
            !objects || report["storageContract"].text == IMReceiptProtocol.contracts[1],
            approvedProfiles.contains(where: {
                $0["id"] == storage["profileId"] && $0["buildDigest"] == report["buildDigest"]
                    && $0["verificationDigest"] == report["verificationDigest"]
                    && $0["storageContract"] == report["storageContract"]
            })
        else {
            try IMReceiptProtocol.fail("RECEIPT_LOCAL_VERIFICATION_REQUIRED")
        }
    }

    func declarations() throws -> [IMJSON] {
        _ = try stateNow()
        let rows = try session.store.list("receipt-declarations", namespace: namespace)
        for row in rows {
            try IMReceiptProtocol.declaration(row, namespace: namespace, physical: physical)
        }
        return rows.sorted { $0["createdAt"].text > $1["createdAt"].text }
    }

    func intents() throws -> [IMJSON] {
        _ = try stateNow()
        let rows = try session.store.list("receipt-intents", namespace: namespace)
        for row in rows {
            try IMReceiptProtocol.localIntent(row, namespace: namespace, physical: physical)
        }
        return rows.sorted { $0["createdAt"].text > $1["createdAt"].text }
    }

    func candidates() throws -> [IMJSON] {
        _ = try stateNow(writable: true)
        let denied = Set(try session.store.syncGlobalIDs(namespace: namespace))
        let queued = Set(
            try session.store.allOutbox(namespace: namespace).map { $0["clientMsgId"].text }.filter { !$0.isEmpty })
        return try session.store.list("conversations", namespace: namespace)
            .filter { ["active", "owner_unavailable"].contains($0["state"].text) }
            .flatMap { conversation in
                return try session.store.list("messages:" + conversation["id"].text, namespace: namespace)
            }.filter {
                !$0["id"].text.isEmpty && !denied.contains($0["id"].text) && $0["deleted"] != .bool(true)
                    && $0["contentUnavailable"] != .bool(true) && $0["forwarded"] != .bool(true)
                    && $0["state"].text != "pending" && $0["pending"] != .bool(true) && $0["frozen"] != .bool(true)
                    && !queued.contains($0["clientMsgId"].text)
                    && ["text", "image", "audio", "video", "file"].contains($0["type"].text)
                    && (try? IMReceiptProtocol.counter($0["contentVersion"])) != nil
            }
    }

    func declare(_ profileID: String) async throws -> IMJSON? {
        return try await exclusive {
            guard let report = self.verification,
                let profile = self.approvedProfiles.first(where: { $0["id"].text == profileID })
            else {
                try IMReceiptProtocol.fail("RECEIPT_LOCAL_VERIFICATION_REQUIRED")
            }
            try self.validateReport(report)
            if let pending = try self.declarations().first(where: {
                ["prepared", "unknown"].contains($0["state"].text)
            }) {
                return try await self.declaration(pending, retry: true)
            }
            let identifier = UUID().uuidString.lowercased()
            let body: IMJSON = .object([
                "requestId": .string(identifier), "storeGeneration": .string(self.physical),
                "revisionEpoch": .string(self.namespace.restoreEpoch), "profileId": profile["id"],
                "buildDigest": report["buildDigest"], "verificationDigest": report["verificationDigest"],
            ])
            let row: IMJSON = .object([
                "id": .string(identifier), "familyId": .string(self.family),
                "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())), "attempts": .number(0),
                "state": .string("prepared"), "originalBody": body, "result": .null,
            ])
            try self.session.store.compareReceiptDeclaration(
                namespace: self.namespace,
                expected: self.stateNow(writable: true), previous: nil, next: row)
            return try await self.declaration(row, retry: true)
        }
    }

    func checkDeclaration(_ id: String, retry: Bool = false) async throws -> IMJSON? {
        return try await exclusive {
            guard let row = try self.declarations().first(where: { $0["id"].text == id }) else {
                try IMReceiptProtocol.fail()
            }
            return try await self.declaration(row, retry: retry)
        }
    }

    private func declaration(_ original: IMJSON, retry: Bool) async throws -> IMJSON? {
        var row = original
        let body = row["originalBody"]
        var response: IMJSON?
        do {
            response = try await request("sync/storage-results", body: body.fields)
        } catch let error as IMAPIError where error.status == 404 && error.code == "STORAGE_DECLARATION_NOT_FOUND" {
            if !retry {
                return nil
            }
        }
        if response == nil {
            guard retry, row["familyId"].text == family, !["frozen", "completed"].contains(row["state"].text),
                let report = verification
            else {
                try IMReceiptProtocol.fail("RECEIPT_ORIGINAL_SESSION_REQUIRED")
            }
            try validateReport(report)
            guard
                approvedProfiles.contains(where: {
                    $0["id"] == body["profileId"] && $0["buildDigest"] == body["buildDigest"]
                        && $0["verificationDigest"] == body["verificationDigest"]
                        && $0["storageContract"] == report["storageContract"]
                })
            else {
                try IMReceiptProtocol.fail("RECEIPT_LOCAL_VERIFICATION_REQUIRED")
            }
            let attempted = row.setting("attempts", .number(Decimal((Int(row["attempts"].text) ?? 0) + 1)))
                .setting("state", .string("unknown"))
            try session.store.compareReceiptDeclaration(
                namespace: namespace,
                expected: stateNow(writable: true), previous: row, next: attempted)
            row = attempted
            response = try await request("sync/storage", method: "PUT", body: body.fields)
        }
        guard let value = response, value["afterReceiptEnabled"] == .bool(false),
            value["deletionEligible"] == .bool(false)
        else {
            try IMReceiptProtocol.fail()
        }
        try IMReceiptProtocol.exact(value, ["result", "current", "storage", "deletionEligible", "afterReceiptEnabled"])
        let fact = try IMReceiptProtocol.declarationFact(value["result"])
        for field in ["storeGeneration", "profileId", "buildDigest", "verificationDigest"]
        where fact[field] != body[field] {
            try IMReceiptProtocol.fail()
        }
        guard fact["restoreEpoch"] == body["revisionEpoch"],
            IMReceiptProtocol.contracts.contains(fact["storageContract"].text)
        else {
            try IMReceiptProtocol.fail()
        }
        try IMReceiptProtocol.storage(value["storage"], namespace: namespace, physical: physical)
        guard
            try IMReceiptProtocol.flag(value["current"])
                == (value["storage"]["qualified"] == .bool(true)
                    && value["storage"]["qualificationId"] == fact["qualificationId"])
        else {
            try IMReceiptProtocol.fail()
        }
        let completed = row.setting("state", .string(row["state"].text == "frozen" ? "frozen" : "completed"))
            .setting("result", fact)
        try session.store.compareReceiptDeclaration(
            namespace: namespace, expected: stateNow(), previous: row, next: completed)
        return value
    }

    private func intent(_ proof: IMJSON, originalBody: IMJSON, membership: IMJSON, kind: String) throws -> IMJSON {
        let version = try schema(stateNow(writable: true))
        return .object([
            "id": originalBody["requestId"], "kind": .string(kind), "familyId": .string(family),
            "storeGeneration": .string(physical), "restoreEpoch": .string(namespace.restoreEpoch),
            "schemaVersion": .string(version),
            "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())), "attempts": .number(0),
            "state": .string("prepared"), "originalBody": originalBody, "messageId": proof["message"],
            "conversationId": proof["conversation"], "contentVersion": proof["contentVersion"],
            "membershipGeneration": membership, "qualificationId": proof["qualification"],
            "permissionGeneration": proof["permission"],
            "deliveryGeneration": .string(proof["deliveryGeneration"].text),
            "manifestDigest": .string(proof["manifestDigest"].text), "bodyDigest": proof["bodyDigest"], "result": .null,
        ])
    }

    private func membership(_ conversation: IMJSON) throws -> IMJSON {
        guard
            let row = try session.store.list("conversations", namespace: namespace)
                .first(where: { $0["id"] == conversation }),
            ["active", "owner_unavailable"].contains(row["state"].text)
        else {
            try IMReceiptProtocol.fail("RECEIPT_MEMBERSHIP_CHANGED")
        }
        _ = try IMReceiptProtocol.counter(row["membershipGeneration"])
        return row["membershipGeneration"]
    }

    func receiveContent(_ messageID: String, expectedTarget: IMJSON? = nil) async throws -> IMJSON? {
        return try await exclusive {
            _ = try IMReceiptProtocol.id(.string(messageID))
            guard let local = try self.candidates().first(where: { $0["id"].text == messageID }) else {
                try IMReceiptProtocol.fail("RECEIPT_CONTENT_UNAVAILABLE")
            }
            if let previous = try self.intents().first(where: {
                $0["kind"].text == "content" && $0["messageId"].text == messageID
                    && $0["contentVersion"] == local["contentVersion"]
            }) {
                return try await self.receipt(previous, retry: false)
            }
            let eligibility = try await self.context()
            guard eligibility["contentReceiptEnabled"] == .bool(true) else {
                try IMReceiptProtocol.fail("STORAGE_NOT_QUALIFIED")
            }
            try self.requireLocal(eligibility, objects: false)
            let state = try self.stateNow(writable: true)
            let challenge = try await self.request(
                "sync/receipt-challenges",
                body: [
                    "messageId": .string(messageID),
                    "schemaVersion": .string(try self.schema(state)),
                ])
            let proof = try IMReceiptProtocol.proof(
                challenge["challenge"], object: false, namespace: self.namespace, physical: self.physical)
            if let expectedTarget {
                guard expectedTarget["contentTarget"] == .bool(true), expectedTarget["originalTarget"] == .bool(true),
                    expectedTarget["contentRecorded"] == .bool(false), expectedTarget["messageCurrent"] == .bool(true),
                    expectedTarget["messageId"] == proof["message"], expectedTarget["contentVersion"] == proof["contentVersion"],
                    expectedTarget["qualificationId"] == proof["qualification"],
                    expectedTarget["deliveryGeneration"].text == proof["deliveryGeneration"].text,
                    expectedTarget["manifestDigest"].text == proof["manifestDigest"].text else {
                    try IMReceiptProtocol.fail("RECEIPT_TARGET_CHANGED")
                }
            }
            guard proof["message"].text == messageID, proof["contentVersion"] == local["contentVersion"],
                proof["conversation"] == local["conversationId"],
                proof["qualification"] == eligibility["storage"]["qualificationId"],
                challenge["requiresAtomicLocalCommit"] == .bool(true), challenge["objectComplete"] == .bool(false),
                challenge["deletionEligible"] == .bool(false),
                IMSyncProtocol.date(challenge["expiresAt"].text)?.timeIntervalSince1970 == Double(proof["expires"].text)
            else {
                try IMReceiptProtocol.fail()
            }
            for (outer, inner) in [
                ("messageId", "message"), ("conversationId", "conversation"), ("contentVersion", "contentVersion"),
                ("restoreEpoch", "epoch"), ("storeGeneration", "store"), ("qualificationId", "qualification"),
                ("bodyDigest", "bodyDigest"),
            ]
            where challenge[outer] != proof[inner] {
                try IMReceiptProtocol.fail()
            }
            let bytes = try IMMigrationProtocol.decode(challenge["bodyEnvelope"], maximum: 2 * 1024 * 1024)
            guard IMMigrationProtocol.hash(bytes) == proof["bodyDigest"].text else {
                try IMReceiptProtocol.fail()
            }
            var wrapped = Data("{\"messages\":[".utf8)
            wrapped.append(bytes)
            wrapped.append(Data("]}".utf8))
            _ = try IMMigrationJSON.messageRanges(wrapped)
            let body: IMJSON = .object([
                "requestId": .string(UUID().uuidString.lowercased()), "storeGeneration": .string(self.physical),
                "revisionEpoch": .string(self.namespace.restoreEpoch), "qualificationId": proof["qualification"],
                "kind": .string("message_content"), "challenge": challenge["challenge"],
                "bodyDigest": proof["bodyDigest"],
            ])
            let row = try self.intent(
                proof, originalBody: body, membership: self.membership(proof["conversation"]), kind: "content")
            try self.session.store.commitReceiptContent(
                namespace: self.namespace, expected: self.stateNow(writable: true), intent: row,
                bodyEnvelope: challenge["bodyEnvelope"].text)
            return try await self.receipt(row, retry: true)
        }
    }

    func receiveObject(_ parentID: String) async throws -> IMJSON? {
        return try await exclusive {
            guard
                let parent = try self.intents().first(where: {
                    $0["id"].text == parentID && $0["kind"].text == "content"
                }),
                parent["state"].text == "completed"
            else {
                try IMReceiptProtocol.fail("CONTENT_RECEIPT_REQUIRED")
            }
            _ = try IMReceiptProtocol.recorded(parent["result"], intent: parent, namespace: self.namespace)
            if let previous = try self.intents().first(where: {
                $0["kind"].text == "object" && $0["originalBody"]["parentReceiptId"].text == parentID
            }) {
                return try await self.receipt(previous, retry: false)
            }
            let eligibility = try await self.context()
            guard eligibility["objectReceiptEnabled"] == .bool(true),
                eligibility["storage"]["qualificationId"] == parent["qualificationId"]
            else {
                try IMReceiptProtocol.fail("STORAGE_NOT_QUALIFIED")
            }
            try self.requireLocal(eligibility, objects: true)
            let state = try self.stateNow(writable: true)
            let challenge = try await self.request(
                "sync/object-receipt-challenges",
                body: [
                    "parentReceiptId": .string(parentID),
                    "schemaVersion": .string(try self.schema(state)),
                ])
            let proof = try IMReceiptProtocol.proof(
                challenge["challenge"], object: true, namespace: self.namespace, physical: self.physical)
            guard proof["parentReceiptId"].text == parentID, proof["message"] == parent["messageId"],
                proof["conversation"] == parent["conversationId"], proof["contentVersion"] == parent["contentVersion"],
                proof["bodyDigest"] == parent["bodyDigest"], proof["qualification"] == parent["qualificationId"],
                proof["permission"] == parent["permissionGeneration"],
                proof["deliveryGeneration"].text == parent["deliveryGeneration"].text,
                proof["manifestDigest"].text == parent["manifestDigest"].text,
                challenge["requiresDurableOriginalObject"] == .bool(true), challenge["objectComplete"] == .bool(false),
                challenge["targetRangeComplete"] == .bool(false), challenge["deletionEligible"] == .bool(false),
                IMSyncProtocol.date(challenge["expiresAt"].text)?.timeIntervalSince1970 == Double(proof["expires"].text)
            else {
                try IMReceiptProtocol.fail()
            }
            for (outer, inner) in [
                ("parentReceiptId", "parentReceiptId"), ("messageId", "message"),
                ("conversationId", "conversation"), ("contentVersion", "contentVersion"), ("restoreEpoch", "epoch"),
                ("storeGeneration", "store"), ("qualificationId", "qualification"), ("bodyDigest", "bodyDigest"),
            ]
            where challenge[outer] != proof[inner] {
                try IMReceiptProtocol.fail()
            }
            let object = challenge["object"]
            try IMReceiptProtocol.exact(object, ["objectId", "sizeBytes", "sha256", "contentType"])
            guard object["objectId"] == proof["objectId"], object["sizeBytes"] == proof["sizeBytes"],
                object["sha256"] == proof["objectDigest"], !object["contentType"].text.isEmpty,
                object["contentType"].text.utf8.count <= 200
            else {
                try IMReceiptProtocol.fail()
            }
            var bytes: Data
            if let binary = self.binaryOverride, let token = self.session.token {
                bytes = try await binary(proof["objectId"].text, token, self.physical)
            } else {
                bytes = try await self.session.authorized { api, token in
                    _ = try self.stateNow()
                    return try await api.receiptOriginalObject(
                        proof["objectId"].text, token: token, store: self.physical)
                }
            }
            defer {
                bytes.resetBytes(in: 0..<bytes.count)
            }
            _ = try self.stateNow(writable: true)
            guard bytes.count == Int(proof["sizeBytes"].text), bytes.count <= 64 * 1024 * 1024,
                IMMigrationProtocol.hash(bytes) == proof["objectDigest"].text
            else {
                try IMReceiptProtocol.fail("RECEIPT_OBJECT_INVALID")
            }
            let body: IMJSON = .object([
                "requestId": .string(UUID().uuidString.lowercased()), "storeGeneration": .string(self.physical),
                "revisionEpoch": .string(self.namespace.restoreEpoch), "qualificationId": proof["qualification"],
                "kind": .string("object_received"), "parentReceiptId": .string(parentID),
                "challenge": challenge["challenge"],
                "objectId": proof["objectId"], "sizeBytes": proof["sizeBytes"], "objectDigest": proof["objectDigest"],
            ])
            let row = try self.intent(
                proof, originalBody: body, membership: self.membership(proof["conversation"]), kind: "object")
            try self.session.store.commitReceiptObject(
                namespace: self.namespace, expected: self.stateNow(writable: true), intent: row,
                descriptor: object.setting("messageId", proof["message"]), bytes: bytes)
            return try await self.receipt(row, retry: true)
        }
    }

    func checkReceipt(_ id: String, retry: Bool = false) async throws -> IMJSON? {
        return try await exclusive {
            guard let row = try self.intents().first(where: { $0["id"].text == id }) else {
                try IMReceiptProtocol.fail()
            }
            return try await self.receipt(row, retry: retry)
        }
    }

    private func receipt(_ original: IMJSON, retry: Bool) async throws -> IMJSON? {
        var row = original
        let object = row["kind"].text == "object"
        let path = object ? "sync/object-receipt" : "sync/receipt"
        let body = row["originalBody"]
        var response: IMJSON?
        do {
            response = try await request(path + "-results", body: body.fields)
        } catch let error as IMAPIError
            where error.status == 404
            && error.code == (object ? "OBJECT_RECEIPT_OPERATION_NOT_FOUND" : "RECEIPT_OPERATION_NOT_FOUND")
        {
            if !retry {
                return nil
            }
        }
        if response == nil {
            guard retry, row["familyId"].text == family, ["prepared", "unknown"].contains(row["state"].text) else {
                try IMReceiptProtocol.fail("RECEIPT_ORIGINAL_SESSION_REQUIRED")
            }
            _ = try IMReceiptProtocol.proof(body["challenge"], object: object, namespace: namespace, physical: physical)
            let eligibility = try await context()
            guard eligibility["storage"]["qualificationId"] == row["qualificationId"],
                eligibility[object ? "objectReceiptEnabled" : "contentReceiptEnabled"] == .bool(true)
            else {
                try IMReceiptProtocol.fail("STORAGE_NOT_QUALIFIED")
            }
            try requireLocal(eligibility, objects: object)
            if object {
                _ = try session.store.receiptObject(namespace: namespace, requestId: row["id"].text)
            }
            let latest = try intents().first(where: { $0["id"] == row["id"] })
            guard latest == row else {
                try IMReceiptProtocol.fail("RECEIPT_ORIGINAL_CHANGED")
            }
            _ = try IMReceiptProtocol.proof(body["challenge"], object: object, namespace: namespace, physical: physical)
            let attempted = row.setting("state", .string("unknown"))
                .setting("attempts", .number(Decimal((Int(row["attempts"].text) ?? 0) + 1)))
            try session.store.compareReceiptIntent(
                namespace: namespace, expected: stateNow(writable: true), previous: row, next: attempted)
            row = attempted
            response = try await request(path + "s", body: body.fields)
        }
        guard let response else {
            try IMReceiptProtocol.fail()
        }
        let result = try IMReceiptProtocol.recorded(response, intent: row, namespace: namespace)
        guard let latest = try intents().first(where: { $0["id"] == row["id"] }), latest["originalBody"] == body else {
            try IMReceiptProtocol.fail("RECEIPT_ORIGINAL_CHANGED")
        }
        let completed = latest.setting("state", .string(latest["state"].text == "frozen" ? "frozen" : "completed"))
            .setting("result", result)
        try session.store.compareReceiptIntent(
            namespace: namespace, expected: stateNow(), previous: latest, next: completed)
        return result
    }
}
