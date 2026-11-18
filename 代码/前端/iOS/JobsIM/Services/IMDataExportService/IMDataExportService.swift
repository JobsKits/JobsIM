//
//  IMDataExportService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
final class IMDataExportService {

    typealias Request = @MainActor (String, String, String?) async throws -> IMJSON
    private let session: IMSession
    private let requestOverride: Request?
    private let current: @MainActor () -> Bool
    let namespace: IMNamespace
    private let family: String
    private static var working = Set<String>()

    init(session: IMSession = .shared, isCurrent: @escaping @MainActor () -> Bool = { true },
         request: Request? = nil) {
        self.session = session
        namespace = session.namespace
        family = session.currentCredentialPair?.familyID ?? ""
        current = isCurrent
        requestOverride = request
    }

    func requireCurrent() throws {
        guard !Task.isCancelled, current(), !session.demo, !family.isEmpty, namespace.sourceContextID == "personal",
            !namespace.deviceID.isEmpty, session.namespace == namespace,
            session.currentCredentialPair?.familyID == family, session.token != nil else {
            try IMDataExportProtocol.fail("DATA_EXPORT_CONTEXT_CHANGED")
        }
    }

    private func request(_ path: String, method: String = "GET", original: String? = nil) async throws -> IMJSON {
        try requireCurrent()
        let result: IMJSON
        if let requestOverride {
            result = try await requestOverride(path, method, original)
        } else if let original {
            result = try await session.authorized { api, token in
                try self.requireCurrent()
                return try await api.originalJSON(path, body: original, token: token)
            }
        } else {
            result = try await session.action(path, method: method)
        }
        try requireCurrent()
        return result
    }

    func capability(required: Bool = false) async throws -> Bool {
        let descriptor = try await request("bootstrap")
        guard descriptor["deploymentId"].text == namespace.deploymentID,
            descriptor["restoreEpoch"].text == namespace.restoreEpoch else {
            try IMDataExportProtocol.fail("DATA_EXPORT_CONTEXT_CHANGED")
        }
        let enabled = descriptor["features"]["chat_export"] == .bool(true)
        if required && !enabled {
            try IMDataExportProtocol.fail("DATA_EXPORT_UNAVAILABLE")
        }
        return enabled
    }

    func rows() throws -> [IMJSON] {
        try requireCurrent()
        let rows = try session.store.list("data-export-operations", namespace: namespace)
        guard rows.count <= 10000 else {
            try IMDataExportProtocol.fail("DATA_EXPORT_STORE_INVALID")
        }
        for row in rows {
            try IMDataExportProtocol.original(row, namespace: namespace, family: family)
        }
        return rows.sorted { $0["createdAt"].text > $1["createdAt"].text }
    }

    private func original(_ id: String) throws -> IMJSON {
        guard let row = try rows().first(where: { $0["id"].text == id }) else {
            try IMDataExportProtocol.fail("DATA_EXPORT_STORE_INVALID")
        }
        return row
    }

    private func save(_ next: IMJSON, previous: IMJSON?) throws {
        try requireCurrent()
        try IMDataExportProtocol.original(next, namespace: namespace, family: family)
        // MainActor在这个同步FULL事务期间不可切换账号；原record仍由SQLite逐字节语义CAS。
        try session.store.compareDataExportIntent(namespace: namespace, previous: previous, next: next)
    }

    private func bind(_ row: IMJSON, value: IMJSON) throws {
        try requireCurrent()
        let current = try original(row["id"].text)
        guard current["originalBody"] == row["originalBody"],
            current["bodySHA256"] == row["bodySHA256"],
            current["jobId"] == .null || current["jobId"] == value["job"]["id"] else {
            try IMDataExportProtocol.fail("DATA_EXPORT_CONTEXT_CHANGED")
        }
        try save(current.setting("jobId", value["job"]["id"]), previous: current)
    }

    private func lookup(_ row: IMJSON) async throws -> IMJSON? {
        do {
            let value = try IMDataExportProtocol.view(
                await request("me/data-export-results", method: "POST", original: row["originalBody"].text),
                expectedJob: row["jobId"] == .null ? nil : row["jobId"].text)
            try bind(row, value: value)
            return value
        } catch let error as IMAPIError where error.status == 404 && error.code == "EXPORT_OPERATION_NOT_FOUND" {
            try requireCurrent()
            return nil
        }
    }

    private func exclusive<T>(_ operation: @MainActor () async throws -> T) async throws -> T {
        try requireCurrent()
        guard Self.working.insert(namespace.key).inserted else {
            try IMDataExportProtocol.fail("DATA_EXPORT_BUSY")
        }
        defer {
            Self.working.remove(namespace.key)
        }
        return try await operation()
    }

    func create() async throws -> IMJSON? {
        return try await exclusive {
            _ = try await self.capability(required: true)
            guard try !self.rows().contains(where: { $0["jobId"] == .null }) else {
                try IMDataExportProtocol.fail("DATA_EXPORT_ORIGINAL_PENDING")
            }
            let id = UUID().uuidString.lowercased()
            let raw = "{\"requestId\":\"" + id + "\",\"restoreEpoch\":\"" + self.namespace.restoreEpoch + "\"}"
            let row: IMJSON = .object(["id": .string(id), "familyId": .string(self.family),
                "originalBody": .string(raw), "bodySHA256": .string(IMDataExportProtocol.digest(raw)),
                "createdAt": .string(ISO8601DateFormatter.make().string(from: Date()))])
            try self.save(row, previous: nil)
            if let existing = try await self.lookup(row) {
                return existing
            }
            let result = try IMDataExportProtocol.view(await self.request("me/data-exports", method: "POST", original: raw))
            guard result["job"]["restoreEpoch"].text == self.namespace.restoreEpoch else {
                try IMDataExportProtocol.fail("DATA_EXPORT_CONTEXT_CHANGED")
            }
            try self.bind(row, value: result)
            return result
        }
    }

    func check(_ id: String, retry: Bool = false) async throws -> IMJSON? {
        return try await exclusive {
            _ = try await self.capability(required: retry)
            let row = try self.original(id)
            if let existing = try await self.lookup(row) {
                return existing
            }
            guard retry, row["jobId"] == .null else {
                return nil
            }
            let value = try IMDataExportProtocol.view(await self.request("me/data-exports", method: "POST", original: row["originalBody"].text))
            guard value["job"]["restoreEpoch"].text == self.namespace.restoreEpoch else {
                try IMDataExportProtocol.fail("DATA_EXPORT_CONTEXT_CHANGED")
            }
            try self.bind(row, value: value)
            return value
        }
    }

    func status(_ id: String) async throws -> IMJSON? {
        let row = try original(id)
        if row["jobId"] == .null {
            return try await check(id)
        }
        return try IMDataExportProtocol.view(await request("me/data-exports/" + row["jobId"].text), expectedJob: row["jobId"].text)
    }

    func cancel(_ id: String) async throws -> IMJSON {
        return try await exclusive {
            _ = try await self.capability()
            var row = try self.original(id)
            _ = try IMSyncProtocol.id(row["jobId"])
            if row["cancelBody"] == .null {
                let raw = "{\"requestId\":\"" + UUID().uuidString.lowercased() + "\"}"
                let next = row.setting("cancelBody", .string(raw)).setting("cancelSHA256", .string(IMDataExportProtocol.digest(raw)))
                try self.save(next, previous: row)
                row = next
            }
            return try IMDataExportProtocol.view(await self.request("me/data-exports/" + row["jobId"].text + "/cancel",
                method: "POST", original: row["cancelBody"].text), expectedJob: row["jobId"].text)
        }
    }

    func download(_ id: String) async throws -> URL {
        return try await exclusive {
            _ = try await self.capability(required: true)
            let row = try self.original(id)
            guard row["jobId"] != .null, row["cancelBody"] == .null else {
                try IMDataExportProtocol.fail("DATA_EXPORT_UNAVAILABLE")
            }
            let path = "me/data-exports/" + row["jobId"].text
            let view = try IMDataExportProtocol.view(await self.request(path), expectedJob: row["jobId"].text)
            let job = view["job"]
            guard job["restoreEpoch"].text == self.namespace.restoreEpoch, job["state"] == .string("ready"),
                job["cleanupState"] == .string("published"),
                let expiry = IMSyncProtocol.date(job["expiresAt"].text), expiry > Date() else {
                try IMDataExportProtocol.fail("DATA_EXPORT_UNAVAILABLE")
            }
            let ticket = try IMDataExportProtocol.ticket(await self.request(path + "/download-ticket", method: "POST", original: "{}"), job: job)
            try self.requireCurrent()
            let folder = FileManager.default.temporaryDirectory.appendingPathComponent("JobsIMDataExport-" + UUID().uuidString)
            try self.session.store.registerAccountPrivateFile(namespace: self.namespace, kind: "export", name: folder.lastPathComponent)
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
            let file = folder.appendingPathComponent("jobsim-personal-data.zip")
            do {
                _ = try await self.session.authorized(requiredFeature: "chat_export") { api, token in
                    try self.requireCurrent()
                    return try await api.downloadPersonalExport(row["jobId"].text, ticket: ticket, to: file, token: token)
                }
                try self.requireCurrent()
                let validation = Task.detached(priority: .utility) {
                    try IMDataExportArchive.validate(file)
                }
                try await withTaskCancellationHandler {
                    try await validation.value
                } onCancel: {
                    validation.cancel()
                }
                try self.requireCurrent()
                return file
            } catch {
                try? FileManager.default.removeItem(at: folder)
                throw error
            }
        }
    }
}
