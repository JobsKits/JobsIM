//
//  IMAccountDeletionService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport

@MainActor
final class IMAccountDeletionService {

    enum Action { case submit, check, retry }
    struct Credential {
        let password: String
        let recoveryCode: String
        let otp: String
    }
    typealias Request = @MainActor (String, [String: IMJSON], String?) async throws -> IMJSON
    private let session: IMSession
    private let username: String
    private let profile: String
    private let deployment: String
    private let requestOverride: Request?
    private static var working = Set<String>()

    init(session: IMSession = .shared, username: String, request: Request? = nil) {
        self.session = session
        self.username = username.lowercased().trimmingCharacters(in: .whitespacesAndNewlines)
        self.profile = session.profile
        self.deployment = session.deployment
        self.requestOverride = request
    }

    static func metadata(_ session: IMSession) -> IMNamespace {
        return IMNamespace(profile: session.profile, account: "__account_deletions__", demo: false,
                           restoreEpoch: "protocol", deploymentID: session.deployment, sourceContextID: "personal")
    }

    static func blocks(_ session: IMSession, pair: IMCredentialPair) throws -> Bool {
        return try session.store.list("account-deletion-operations", namespace: metadata(session)).contains {
            $0["userId"].text == pair.userID && ($0["state"].text == "completed"
                || $0["familyId"].text == pair.familyID && !["unsent", "rejected"].contains($0["state"].text))
        }
    }

    static func credential(password: String, recoveryCode: String = "", otp: String = "") throws -> Credential {
        let normalized = password.precomposedStringWithCanonicalMapping
        let code = recoveryCode.trimmingCharacters(in: .whitespacesAndNewlines)
        guard normalized.isEmpty != code.isEmpty, normalized.utf8.count <= 512, code.utf8.count <= 128,
            otp.isEmpty || otp.range(of: "^[0-9]{6}$", options: .regularExpression) != nil else {
            throw IMAPIError(status: 0, code: "INVALID_DATA_RIGHTS_PROOF", message: IMStrings.text("deletionProofHelp"))
        }
        return Credential(password: normalized, recoveryCode: code, otp: otp)
    }

    private static func positive(_ value: IMJSON) -> Bool {
        return (try? IMSyncProtocol.counter(value)).map { $0 > 0 } == true
    }

    private func requireContext() throws {
        guard !Task.isCancelled, !session.demo, session.profile == profile, session.deployment == deployment,
            UUID(uuidString: deployment) != nil, !profile.isEmpty,
            username.range(of: "^[a-z0-9_]{3,32}$", options: .regularExpression) != nil else {
            throw IMSession.SessionError.noInstance
        }
    }

    private func request(_ path: String, body: [String: IMJSON] = [:], token: String? = nil) async throws -> IMJSON {
        try requireContext()
        let result: IMJSON
        if let requestOverride {
            result = try await requestOverride(path, body, token)
        } else {
            guard let api = session.api else {
                throw IMSession.SessionError.noInstance
            }
            if path == "bootstrap" {
                result = try await api.request(path)
            } else {
                result = try await api.accountDeletion(path, body: body, token: token)
            }
        }
        try requireContext()
        return result
    }

    private func capability(_ row: IMJSON? = nil, required: Bool) async throws -> Bool {
        let value = try await request("bootstrap")
        guard value["deploymentId"].text == deployment,
            Self.positive(value["restoreEpoch"]) else {
            throw IMSession.SessionError.invalidInstance
        }
        let allowed = value["features"]["own_account_deletion"] == .bool(true)
            && (row == nil || value["restoreEpoch"] == row?["restoreEpoch"])
        if required && !allowed {
            throw IMAPIError(status: 409, code: "ACCOUNT_DELETION_UNAVAILABLE", message: IMStrings.text("connectionNeeded"))
        }
        return allowed
    }

    func rows() throws -> [IMJSON] {
        try requireContext()
        return try session.store.list("account-deletion-operations", namespace: Self.metadata(session))
            .filter { $0["username"].text == username }
            .sorted { $0["createdAt"].text > $1["createdAt"].text }
    }

    private func original(_ id: String) throws -> IMJSON {
        guard let row = try rows().first(where: { $0["id"].text == id }),
            row["profile"].text == profile, row["deploymentId"].text == deployment else {
            throw IMSession.SessionError.invalidInstance
        }
        return row
    }

    private func save(_ row: IMJSON, previous: IMJSON?) throws {
        try requireContext()
        try session.store.compareAccountDeletionIntent(namespace: Self.metadata(session), previous: previous, next: row)
    }

    func prepare() async throws -> IMJSON {
        try requireContext()
        if requestOverride == nil {
            _ = try await session.authorized { api, token in
                return try await api.request("auth/context", token: token)
            }
        }
        _ = try await capability(required: true)
        guard let pair = session.currentCredentialPair, pair.username == username,
            pair.profile == profile, pair.deploymentID == deployment, pair.restoreEpoch == session.restoreEpoch,
            try !rows().contains(where: { $0["familyId"].text == pair.familyID && !["rejected", "cancelled", "retired"].contains($0["state"].text) }) else {
            throw IMAPIError(status: 409, code: "ACCOUNT_DELETION_PENDING", message: IMStrings.text("deletionPending"))
        }
        let row: IMJSON = .object([
            "id": .string(UUID().uuidString.lowercased()), "username": .string(username), "profile": .string(profile),
            "deploymentId": .string(deployment), "userId": .string(pair.userID), "deviceId": .string(pair.deviceID),
            "familyId": .string(pair.familyID), "restoreEpoch": .string(pair.restoreEpoch),
            "namespaceKey": .string(pair.namespace.key), "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())),
            "state": .string("unsent"), "external": .bool(false), "deletionId": .null,
            "requestVersion": .null, "result": .null, "cancelNonce": .null, "cancelVersion": .null,
            "cancelSent": .bool(false), "cleanup": .bool(false),
        ])
        try save(row, previous: nil)
        return row
    }

    private func token(_ row: IMJSON) -> String? {
        guard let pair = session.currentCredentialPair, pair.familyID == row["familyId"].text,
            pair.userID == row["userId"].text, pair.deviceID == row["deviceId"].text,
            pair.restoreEpoch == row["restoreEpoch"].text, pair.username == username,
            pair.profile == profile, pair.deploymentID == deployment else {
            return nil
        }
        return pair.accessToken
    }

    private func body(_ row: IMJSON, credential: Credential, otp: Bool) throws -> [String: IMJSON] {
        guard !credential.password.isEmpty, UUID(uuidString: row["deviceId"].text) != nil else {
            throw IMAPIError(status: 0, code: "ORIGINAL_PASSWORD_REQUIRED", message: IMStrings.text("deletionOriginalHelp"))
        }
        var value: [String: IMJSON] = ["requestId": row["id"], "username": row["username"],
                                      "deviceId": row["deviceId"], "restoreEpoch": row["restoreEpoch"],
                                      "password": .string(credential.password)]
        if otp && !credential.otp.isEmpty {
            value["otp"] = .string(credential.otp)
        }
        return value
    }

    private func lock<T>(_ action: () async throws -> T) async throws -> T {
        try requireContext()
        let key = profile + ":" + username
        guard Self.working.insert(key).inserted else {
            throw IMAPIError(status: 409, code: "ACCOUNT_DELETION_BUSY", message: IMStrings.text("deletionPending"))
        }
        defer {
            Self.working.remove(key)
        }
        return try await action()
    }

    func perform(_ id: String, credential: Credential, action: Action) async throws -> IMJSON? {
        return try await lock {
            var row = try self.original(id)
            let available = try await self.capability(row, required: action == .submit)
            let captured = self.token(row)
            if action == .submit {
                guard row["state"].text == "unsent", captured != nil else {
                    throw IMSession.SessionError.invalidInstance
                }
                let next = row.setting("state", .string("unknown"))
                try self.save(next, previous: row)
                row = next
                self.session.stopAuthoritativeSync()
            }
            var found: IMJSON?
            do {
                found = try await self.request("auth/deletion-results", body: self.body(row, credential: credential, otp: false))
            } catch let error as IMAPIError where error.status == 404 && error.code == "ACCOUNT_DELETION_NOT_FOUND" {
                if action == .check {
                    return nil
                }
            }
            if let found, found["status"].text != "pending" || action != .retry {
                return try await self.accept(found, row: row)
            }
            if found != nil, let pair = self.session.currentCredentialPair, pair.familyID != row["familyId"].text {
                throw IMAPIError(status: 409, code: "ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED", message: IMStrings.text("deletionPending"))
            }
            guard available, try await self.capability(row, required: false) else {
                throw IMAPIError(status: 409, code: "ACCOUNT_DELETION_UNAVAILABLE", message: IMStrings.text("deletionPending"))
            }
            let authorization: String?
            if found == nil {
                guard let captured, captured == self.token(row) else {
                    throw IMAPIError(status: 409, code: "ACCOUNT_DELETION_ORIGINAL_FAMILY_REQUIRED", message: IMStrings.text("deletionPending"))
                }
                authorization = captured
            } else {
                authorization = nil
            }
            let unknown = row.setting("state", .string("unknown"))
            try self.save(unknown, previous: try self.original(id))
            row = unknown
            self.session.stopAuthoritativeSync()
            do {
                let result = try await self.request("auth/deletion-requests",
                    body: self.body(row, credential: credential, otp: authorization != nil), token: authorization)
                return try await self.accept(result, row: row)
            } catch let error as IMAPIError {
                if action == .submit && (error.status == 400 && error.code == "INVALID_ACCOUNT_DELETION"
                    || error.status == 403 && error.code == "DATA_RIGHTS_REJECTED"
                    || error.status == 429 && error.code == "RATE_LIMITED") {
                    try self.save(row.setting("state", .string("rejected")), previous: try self.original(id))
                }
                throw error
            }
        }
    }

    private func proof(_ row: IMJSON, credential: Credential, purpose: String) async throws -> IMJSON {
        guard UUID(uuidString: row["deletionId"].text) != nil, !row["requestVersion"].text.isEmpty else {
            throw IMAPIError(status: 0, code: "DELETION_RESULT_REQUIRED", message: IMStrings.text("deletionOriginalHelp"))
        }
        let bootstrap = try await request("bootstrap")
        guard bootstrap["deploymentId"].text == deployment else {
            throw IMSession.SessionError.invalidInstance
        }
        var body: [String: IMJSON] = ["username": .string(username), "deletionId": row["deletionId"],
                                      "requestVersion": row["requestVersion"], "purpose": .string(purpose)]
        if !credential.password.isEmpty {
            body["password"] = .string(credential.password)
        } else {
            body["recoveryCode"] = .string(credential.recoveryCode)
        }
        if !credential.otp.isEmpty {
            body["otp"] = .string(credential.otp)
        }
        let value = try await request("auth/data-rights/proofs", body: body)
        guard value["deletionId"] == row["deletionId"], value["purpose"].text == purpose,
            value["restoreEpoch"] == bootstrap["restoreEpoch"],
            value["proof"].text.utf8.count >= 32, value["proof"].text.utf8.count <= 128,
            let expiry = IMSyncProtocol.date(value["expiresAt"].text), expiry > Date(),
            expiry < Date().addingTimeInterval(330),
            Self.positive(value["requestVersion"]) else {
            throw IMSession.SessionError.invalidInstance
        }
        return value
    }

    func query(_ id: String, credential: Credential) async throws -> IMJSON {
        return try await lock {
            let row = try self.original(id)
            let proof = try await self.proof(row, credential: credential, purpose: "query")
            let result = try await self.request("auth/data-rights/query", body: ["proof": proof["proof"]])
            return try await self.accept(result, row: row)
        }
    }

    func cancel(_ id: String, credential: Credential) async throws -> IMJSON {
        return try await lock {
            var row = try self.original(id)
            guard row["result"]["status"].text == "requested", row["cancelSent"] != .bool(true),
                let until = IMSyncProtocol.date(row["result"]["cancelUntil"].text), until > Date() else {
                throw IMAPIError(status: 409, code: "DELETION_CHECK_REQUIRED", message: IMStrings.text("deletionCancelHelp"))
            }
            if row["cancelNonce"] == .null {
                let next = row.setting("cancelNonce", .string(UUID().uuidString.lowercased()))
                    .setting("cancelVersion", row["requestVersion"])
                try self.save(next, previous: row)
                row = next
            }
            guard row["cancelVersion"] == row["requestVersion"] else {
                throw IMAPIError(status: 409, code: "DELETION_VERSION_CHANGED", message: IMStrings.text("deletionCancelHelp"))
            }
            let proof = try await self.proof(row, credential: credential, purpose: "cancel")
            guard proof["requestVersion"] == row["cancelVersion"] else {
                throw IMSession.SessionError.invalidInstance
            }
            let next = row.setting("cancelSent", .bool(true))
            try self.save(next, previous: row)
            row = next
            let result = try await self.request("auth/data-rights/cancel", body: ["proof": proof["proof"], "requestId": row["cancelNonce"]])
            guard result["cancelRequestId"] == row["cancelNonce"], ["cancel_pending", "cancelled"].contains(result["status"].text) else {
                throw IMSession.SessionError.invalidInstance
            }
            return try await self.accept(result, row: row)
        }
    }

    private func accept(_ result: IMJSON, row: IMJSON) async throws -> IMJSON {
        let publicKeys: Set<String> = ["deletionId", "requestId", "status", "requestVersion", "restoreEpoch", "cancelUntil", "cancelRequestId", "requiresNewLogin"]
        let safe: IMJSON = .object(result.fields.filter { publicKeys.contains($0.key) })
        guard safe["requestId"] == row["id"], safe["restoreEpoch"] == row["restoreEpoch"],
            UUID(uuidString: safe["deletionId"].text) != nil,
            ["pending", "requested", "cancel_pending", "processing", "completed", "cancelled", "retired"].contains(safe["status"].text),
            safe["requiresNewLogin"] == .bool(true),
            case .string(let cancelRequestID) = safe["cancelRequestId"],
            cancelRequestID.isEmpty || cancelRequestID.range(of: "^[A-Za-z0-9_-]{8,128}$", options: .regularExpression) != nil,
            Self.positive(safe["requestVersion"]),
            safe["cancelUntil"] == .null || IMSyncProtocol.date(safe["cancelUntil"].text) != nil,
            row["deletionId"] == .null || row["deletionId"] == safe["deletionId"] else {
            throw IMSession.SessionError.invalidInstance
        }
        if row["requestVersion"] != .null {
            let previous = try IMSyncProtocol.counter(row["requestVersion"])
            let next = try IMSyncProtocol.counter(safe["requestVersion"])
            guard next >= previous, row["state"].text != "completed" || safe["status"].text == "completed" else {
                throw IMSession.SessionError.invalidInstance
            }
        }
        var next = row.setting("state", safe["status"]).setting("deletionId", safe["deletionId"])
            .setting("requestVersion", safe["requestVersion"]).setting("result", safe)
        if row["cancelSent"] == .bool(true), safe["status"].text == "requested",
            safe["requestVersion"] == row["cancelVersion"], safe["cancelRequestId"].text.isEmpty {
            next = next.setting("cancelSent", .bool(false))
        }
        try save(next, previous: try original(row["id"].text))
        if let pair = session.currentCredentialPair, pair.familyID == row["familyId"].text,
            pair.userID == row["userId"].text {
            session.invalidateAuthentication(namespace: pair.namespace, token: pair.accessToken)
        }
        if safe["status"].text == "completed", !row["userId"].text.isEmpty {
            try await cleanup(row)
        }
        return safe
    }

    func receipt(_ id: String) throws -> String {
        let row = try original(id)
        guard UUID(uuidString: row["deletionId"].text) != nil else {
            throw IMSession.SessionError.invalidInstance
        }
        let data = try JSONEncoder.make().encode(IMJSON.object([
            "deploymentId": .string(deployment), "username": .string(username),
            "requestId": row["id"], "deviceId": row["deviceId"], "restoreEpoch": row["restoreEpoch"],
            "deletionId": row["deletionId"], "requestVersion": row["requestVersion"],
        ]))
        return "JobsIM-Deletion-1:" + data.base64EncodedString()
    }

    func importReceipt(_ code: String) throws {
        try requireContext()
        let prefix = "JobsIM-Deletion-1:"
        guard code.hasPrefix(prefix), code.utf8.count <= 8192,
            let data = Data(base64Encoded: String(code.dropFirst(prefix.count))) else {
            throw IMSession.SessionError.invalidInstance
        }
        let value = try JSONDecoder.make().decode(IMJSON.self, from: data)
        guard Set(value.fields.keys) == Set(["deploymentId", "username", "requestId", "deviceId", "restoreEpoch", "deletionId", "requestVersion"]),
            value["deploymentId"].text == deployment, value["username"].text == username,
            value["requestId"].text.range(of: "^[A-Za-z0-9_-]{8,128}$", options: .regularExpression) != nil,
            UUID(uuidString: value["deviceId"].text) != nil, UUID(uuidString: value["deletionId"].text) != nil,
            Self.positive(value["restoreEpoch"]), Self.positive(value["requestVersion"]) else {
            throw IMSession.SessionError.invalidInstance
        }
        if let existing = try rows().first(where: { $0["id"] == value["requestId"] }) {
            guard existing["deviceId"] == value["deviceId"], existing["restoreEpoch"] == value["restoreEpoch"],
                existing["deletionId"] == value["deletionId"] else {
                throw IMSession.SessionError.invalidInstance
            }
            return
        }
        let pair = session.currentCredentialPair
        let trustedUser = pair?.username == username && pair?.deploymentID == deployment && pair?.profile == profile
            ? pair?.userID ?? "" : ""
        let row: IMJSON = .object([
            "id": value["requestId"], "username": .string(username), "profile": .string(profile),
            "deploymentId": .string(deployment), "userId": .string(trustedUser), "deviceId": value["deviceId"],
            "familyId": .string(""), "restoreEpoch": value["restoreEpoch"], "namespaceKey": .string(""),
            "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())), "state": .string("unknown"),
            "external": .bool(true), "deletionId": value["deletionId"], "requestVersion": value["requestVersion"],
            "result": .null, "cancelNonce": .null, "cancelVersion": .null, "cancelSent": .bool(false), "cleanup": .bool(false),
        ])
        try save(row, previous: nil)
    }

    func resumeLocalCleanup() async throws {
        for row in try rows() where row["state"].text == "completed" && row["cleanup"] != .bool(true) && !row["userId"].text.isEmpty {
            try await cleanup(row)
        }
    }

    private func cleanup(_ row: IMJSON) async throws {
        let keys = try session.store.beginAccountDeletionCleanup(profile: profile, deployment: deployment,
            user: row["userId"].text, originalNamespace: row["namespaceKey"].text)
        try await IMAccountDeletionFiles.clear(namespaceKeys: keys, files: session.store.accountDeletionFiles(profile: profile, deployment: deployment, user: row["userId"].text))
        try session.store.finishAccountDeletionCleanup(profile: profile, deployment: deployment, user: row["userId"].text)
        try IMRememberedLoginStore.shared.forget(for: profile, userID: row["userId"].text,
                                                 legacyUsername: row["username"].text)
        try IMKeychain.write(nil, key: profile + ":" + row["userId"].text)
        if let data = UserDefaults.standard.data(forKey: "user:" + profile),
            let user = try? JSONDecoder.make().decode(IMJSON.self, from: data), user["id"] == row["userId"] {
            UserDefaults.standard.removeObject(forKey: "user:" + profile)
        }
        if let pair = session.currentCredentialPair, pair.userID == row["userId"].text {
            session.invalidateAuthentication(namespace: pair.namespace, token: pair.accessToken)
        }
        let current = try original(row["id"].text)
        try save(current.setting("cleanup", .bool(true)), previous: current)
    }
}
