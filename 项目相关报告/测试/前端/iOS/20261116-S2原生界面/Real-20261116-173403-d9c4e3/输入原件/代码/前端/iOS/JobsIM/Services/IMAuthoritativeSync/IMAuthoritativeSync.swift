//
//  IMAuthoritativeSync.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport

@MainActor
final class IMAuthoritativeSync {
    typealias Request = @MainActor (String, String, [String: IMJSON]) async throws -> IMJSON

    private let store: IMLocalStore
    let namespace: IMNamespace
    private let request: Request
    private let isCurrent: () -> Bool
    private let didRestrict: () -> Void
    private let visibleConversation: () -> String?
    private var task: Task<Void, Error>?

    init(
        store: IMLocalStore, namespace: IMNamespace,
        request: @escaping Request, isCurrent: @escaping () -> Bool,
        didRestrict: @escaping () -> Void = {},
        visibleConversation: @escaping () -> String? = { nil }
    ) {
        self.store = store
        self.namespace = namespace
        self.request = request
        self.isCurrent = isCurrent
        self.didRestrict = didRestrict
        self.visibleConversation = visibleConversation
    }

    func cancel() {
        task?.cancel()
        task = nil
        try? store.syncRestrict(namespace: namespace, reason: "SYNC_CONTEXT_CHANGED")
    }

    func run() async throws {
        if let task {
            return try await task.value
        }
        let running = Task {
            do {
                try await synchronize()
            } catch {
                let code =
                    (error as? IMSyncProtocol.Failure)?.code
                    ?? (error as? IMAPIError)?.code ?? "SYNC_UNAVAILABLE"
                if isCurrent() {
                    let obsolete = [
                        "SNAPSHOT_EXPIRED", "SNAPSHOT_INVALIDATED", "SYNC_SNAPSHOT_EXPIRED",
                        "SYNC_SNAPSHOT_INVALIDATED", "SYNC_PERMISSION_CHANGED",
                    ].contains(code)
                    try? store.syncRestrict(
                        namespace: namespace, reason: code, resetSnapshot: obsolete)
                    didRestrict()
                }
                throw error
            }
        }
        task = running
        defer {
            task = nil
        }
        try await running.value
    }

    private func current() throws {
        guard !Task.isCancelled, isCurrent() else {
            try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
        }
    }

    private func call(_ path: String, method: String = "GET", body: [String: IMJSON] = [:])
        async throws -> IMJSON
    {
        try current()
        let value = try await request(path, method, body)
        try current()
        return value
    }

    private func synchronize() async throws {
        try current()
        var state = try store.syncState(namespace: namespace)
        let bootstrap = try await call("bootstrap")
        guard bootstrap["deploymentId"] == state["scopeId"],
            bootstrap["restoreEpoch"] == state["restoreEpoch"]
        else {
            try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
        }
        guard bootstrap["features"]["authoritative_sync"] == .bool(true),
            bootstrap["features"]["sync_jetstream"] == .bool(true)
        else {
            try IMSyncProtocol.refuse("SYNC_CAPABILITY_REQUIRED")
        }
        let supportsSelfState = bootstrap["features"]["sync_self_state"] == .bool(true)
        if state["schemaVersion"] == .string("2"), !supportsSelfState {
            try IMSyncProtocol.refuse("SYNC_CAPABILITY_REQUIRED")
        }
        state = try store.syncSelectSchema(namespace: namespace, expected: state,
                                           schema: supportsSelfState ? "2" : "1")
        let schema = state["schemaVersion"].text.isEmpty ? "1" : state["schemaVersion"].text
        var identity: [String: IMJSON] = [:]
        for key in [
            "scopeId", "principalId", "sourceContextId", "deviceId", "storeGeneration",
            "restoreEpoch",
        ] {
            identity[key] = state[key]
        }
        for key in ["scopeId", "principalId", "deviceId", "storeGeneration"] {
            _ = try IMSyncProtocol.id(state[key])
        }
        guard state["sourceContextId"] == .string("personal"),
            try IMSyncProtocol.counter(state["restoreEpoch"]) > 0
        else {
            try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
        }
        var registrationBody: [String: IMJSON] = [
            "requestId": state["registrationRequestId"],
            "storeGeneration": state["storeGeneration"],
        ]
        if schema == "2" {
            registrationBody["schemaVersion"] = .string("2")
        }
        let registration = try await call(
            "sync/device", method: "PUT",
            body: registrationBody)
        try IMSyncProtocol.binding(registration, expected: .object(identity))
        guard registration["qualification"] == .string("transport_only"),
            registration["qualifiedAck"] == .bool(false)
        else {
            try IMSyncProtocol.refuse()
        }
        _ = try IMSyncProtocol.id(registration["streamId"])
        if schema == "2", registration["schemaVersion"] != .string("2") {
            try IMSyncProtocol.refuse("SYNC_UNSUPPORTED_SCHEMA")
        }
        if state["streamId"] != .null, state["streamId"] != registration["streamId"] {
            try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
        }
        state = try store.syncUpdate(
            namespace: namespace, expected: state,
            next: state.setting("streamId", registration["streamId"]))
        identity["streamId"] = registration["streamId"]
        let binding: IMJSON = .object(identity)
        let context = try await call("sync/context" + (schema == "2" ? "?schemaVersion=2" : ""))
        try IMSyncProtocol.binding(context, expected: binding)
        guard context["qualification"] == .string("transport_only"),
            context["qualifiedAck"] == .bool(false),
            context["afterReceiptEnabled"] == .bool(false)
        else {
            try IMSyncProtocol.refuse()
        }
        _ = try IMSyncProtocol.counter(context["head"])
        _ = try IMSyncProtocol.digest(context["permissionGeneration"])
        if schema == "2" {
            let included = Set(context["coverage"]["included"].list.map { $0.text })
            guard context["coverage"]["schemaVersion"] == .string("2"),
                Set(["own_font_preference", "own_conversation_views", "own_read_points", "own_read_observation_metadata"]).isSubset(of: included)
            else {
                try IMSyncProtocol.refuse("SYNC_COVERAGE_REQUIRED")
            }
        }
        if state["permissionGeneration"] != .null,
            state["permissionGeneration"] != context["permissionGeneration"]
        {
            try store.syncRestrict(
                namespace: namespace, reason: "SYNC_PERMISSION_CHANGED", resetSnapshot: true)
            didRestrict()
            state = try store.syncState(namespace: namespace)
        }
        if state["cursor"] != .null, state["restriction"].text.isEmpty,
            state["permissionGeneration"] == context["permissionGeneration"]
        {
            let raw = try await call(
                Self.changesPath(state["cursor"].text, schema: schema))
            let delta = try IMSyncProtocol.changes(
                raw, previous: state["position"], epoch: state["restoreEpoch"],
                permission: state["permissionGeneration"], schema: schema)
            let proofs = try await reconcile()
            try current()
            try store.syncCommit(
                namespace: namespace, expected: state, next: completed(state, delta: delta),
                projections: Self.projections(delta), events: delta["items"].list, proofs: proofs,
                replaceCloud: false,
                visibleConversation: schema == "2" ? nil : visibleConversation())
            return
        }
        try await snapshot(state, binding: binding)
    }

    private func snapshot(_ original: IMJSON, binding: IMJSON) async throws {
        var state = original
        let schema = state["schemaVersion"].text.isEmpty ? "1" : state["schemaVersion"].text
        if state["snapshotIntent"] == .null {
            let intent: IMJSON = .object([
                "requestId": .string(UUID().uuidString.lowercased()),
                "storeGeneration": state["storeGeneration"],
                "schemaVersion": .string(schema), "pageSize": .number(100),
            ])
            state = try store.syncUpdate(
                namespace: namespace, expected: state, next: state.setting("snapshotIntent", intent)
            )
        }
        let raw = try await call(
            "sync/snapshots", method: "POST", body: state["snapshotIntent"].fields)
        let manifest = try IMSyncProtocol.manifest(raw, binding: binding, schema: schema)
        if state["manifest"] != .null, state["manifest"] != manifest {
            try IMSyncProtocol.refuse("SYNC_SNAPSHOT_INVALIDATED")
        }
        state = try store.syncUpdate(
            namespace: namespace, expected: state, next: state.setting("manifest", manifest))
        var projections: [IMJSON] = []
        var verifiedPages: [IMJSON] = []
        var seen = Set<String>()
        for index in manifest["pageManifest"].list.indices {
            let page = try await call(
                "sync/snapshots/" + manifest["snapshotId"].text + "/pages/" + String(index))
            let values = try IMSyncProtocol.page(page, manifest: manifest, index: index)
            for value in values {
                guard seen.insert(try IMSyncProtocol.reference(value["ref"], schema: schema)).inserted else {
                    try IMSyncProtocol.refuse("SYNC_DUPLICATE_REFERENCE")
                }
            }
            let staged: IMJSON = .object([
                "snapshotId": manifest["snapshotId"], "page": .number(Decimal(index)),
                "pageDigest": page["pageDigest"], "refs": page["refs"],
                "referenceEnvelope": page["referenceEnvelope"],
            ])
            state = try store.syncUpdate(
                namespace: namespace, expected: state, next: state, page: staged)
            verifiedPages.append(staged)
            projections.append(contentsOf: values)
        }
        let validation = try await call(
            "sync/snapshots/" + manifest["snapshotId"].text + "/validate", method: "POST",
            body: [
                "pageDigests": .array(manifest["pageManifest"].list.map { $0["digest"] })
            ])
        let watermark = try IMSyncProtocol.validation(validation, manifest: manifest)
        // 最终校验后再确认当前权限；正文和连续游标仍只在随后一次GRDB事务安装。
        let rawDelta = try await call(
            Self.changesPath(manifest["boundaryToken"].text, schema: schema))
        let delta = try IMSyncProtocol.changes(
            rawDelta, previous: manifest["boundary"], epoch: manifest["restoreEpoch"],
            permission: manifest["permissionGeneration"], schema: schema)
        projections.append(contentsOf: Self.projections(delta))
        let proofs = try await reconcile()
        try current()
        _ = try IMSyncProtocol.validation(validation, manifest: manifest)
        let next = completed(state, delta: delta).setting("classificationWatermark", watermark)
        try store.syncCommit(
            namespace: namespace, expected: state, next: next,
            projections: projections, events: delta["items"].list, proofs: proofs,
            replaceCloud: true, verifiedPages: verifiedPages)
    }

    private func reconcile() async throws -> [IMJSON] {
        var proofs: [IMJSON] = []
        for original in try store.allOutbox(namespace: namespace) {
            let conversation = try IMSyncProtocol.id(original["conversationId"])
            guard original["senderId"].text == namespace.account,
                original["restoreEpoch"].text == namespace.restoreEpoch
            else {
                continue
            }
            do {
                let message = try await call(
                    "conversations/" + conversation + "/message-results", method: "POST",
                    body: IMSession.wireFields(original.fields))
                guard message["conversationId"] == original["conversationId"],
                    message["clientMsgId"] == original["clientMsgId"],
                    message["senderId"] == original["senderId"]
                else { try IMSyncProtocol.refuse() }
                proofs.append(.object(["original": original, "message": message]))
            } catch let error as IMAPIError
                where [0, 403, 404, 408, 409, 425, 429].contains(error.status)
                || error.status >= 500
            {
                // 未知、拒绝及查无原结果均不构成新发授权，也不能抹掉原队列。
                continue
            }
        }
        return proofs
    }

    private func completed(_ state: IMJSON, delta: IMJSON) -> IMJSON {
        return state.setting("cursor", delta["cursor"]).setting("position", delta["position"])
            .setting("permissionGeneration", delta["permissionGeneration"])
            .setting("snapshotIntent", .null).setting("manifest", .null)
    }

    private static func projections(_ delta: IMJSON) -> [IMJSON] {
        return delta["items"].list.filter { $0["kind"] == .string("reference") }.map {
            $0["projection"]
        }
    }

    private static func escape(_ value: String) -> String {
        let allowed = CharacterSet.urlQueryAllowed.subtracting(CharacterSet(charactersIn: "&+=?#/"))
        return value.addingPercentEncoding(withAllowedCharacters: allowed) ?? ""
    }

    private static func changesPath(_ cursor: String, schema: String) -> String {
        return "sync/changes?cursor=" + escape(cursor) + "&limit=500"
            + (schema == "2" ? "&schemaVersion=2" : "")
    }
}
