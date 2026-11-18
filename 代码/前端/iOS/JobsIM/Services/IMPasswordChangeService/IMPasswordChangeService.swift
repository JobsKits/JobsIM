//
//  IMPasswordChangeService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
final class IMPasswordChangeService {

    enum Action { case submit, check, retry }
    typealias Request = @MainActor (String, [String: IMJSON], String?) async throws -> IMJSON
    private let session: IMSession
    private let username: String
    private let profile: String
    private let deployment: String
    private let current: @MainActor () -> Bool
    private let requestOverride: Request?
    private static var working = Set<String>()

    init(session: IMSession = .shared, username: String,
         isCurrent: @escaping @MainActor () -> Bool = { true }, request: Request? = nil) {
        self.session = session
        self.username = username.lowercased().trimmingCharacters(in: .whitespacesAndNewlines)
        profile = session.profile
        deployment = session.deployment
        current = isCurrent
        requestOverride = request
    }

    static func metadata(_ session: IMSession) -> IMNamespace {
        return IMNamespace(profile: session.profile, account: "__password_changes__", demo: false,
                           restoreEpoch: "protocol", deploymentID: session.deployment, sourceContextID: "personal")
    }

    static func blocks(_ session: IMSession, pair: IMCredentialPair) throws -> Bool {
        return try session.store.list("password-change-operations", namespace: metadata(session)).contains {
            $0["familyId"].text == pair.familyID && $0["userId"].text == pair.userID
                && ["unknown", "pending", "changed"].contains($0["state"].text)
        }
    }

    private func requireContext() throws {
        guard !Task.isCancelled, current(), !session.demo, !username.isEmpty,
            !profile.isEmpty, UUID(uuidString: deployment) != nil,
            session.profile == profile, session.deployment == deployment else {
            throw IMAPIError(status: 0, code: "PASSWORD_CHANGE_CONTEXT_CHANGED", message: IMStrings.text("passwordChangeUnknown"))
        }
    }

    private func request(_ path: String, body: [String: IMJSON] = [:], token: String? = nil) async throws -> IMJSON {
        try requireContext()
        let value: IMJSON
        if let requestOverride {
            value = try await requestOverride(path, body, token)
        } else {
            guard let api = session.api else {
                throw IMSession.SessionError.noInstance
            }
            if path == "bootstrap" {
                value = try await api.request(path)
            } else {
                value = try await api.passwordChange(path, body: body, token: token)
            }
        }
        try requireContext()
        return value
    }

    private func capability(_ original: IMJSON? = nil, required: Bool) async throws -> Bool {
        let value = try await request("bootstrap")
        guard value["deploymentId"].text == deployment,
            Self.counter(value["restoreEpoch"].text) else {
            throw IMAPIError(status: 409, code: "RESTORE_EPOCH_CHANGED", message: IMStrings.text("passwordChangeUnknown"))
        }
        let available = value["features"]["own_password_change"] == .bool(true)
            && (original == nil || value["restoreEpoch"] == original?["restoreEpoch"])
        if required && !available {
            throw IMAPIError(status: 409, code: "PASSWORD_CHANGE_UNAVAILABLE", message: IMStrings.text("connectionNeeded"))
        }
        return available
    }

    func rows() throws -> [IMJSON] {
        try requireContext()
        return try session.store.list("password-change-operations", namespace: Self.metadata(session))
            .filter { $0["username"].text == username }
            .sorted { $0["createdAt"].text > $1["createdAt"].text }
    }

    private func original(_ id: String) throws -> IMJSON {
        guard let value = try rows().first(where: { $0["id"].text == id }),
            value["profile"].text == profile, value["deploymentId"].text == deployment,
            UUID(uuidString: value["id"].text) != nil,
            UUID(uuidString: value["userId"].text) != nil,
            UUID(uuidString: value["deviceId"].text) != nil,
            UUID(uuidString: value["familyId"].text) != nil, Self.counter(value["restoreEpoch"].text) else {
            throw IMAPIError(status: 0, code: "PASSWORD_CHANGE_ORIGINAL_INVALID", message: IMStrings.text("passwordChangeUnknown"))
        }
        return value
    }

    private func save(_ next: IMJSON, previous: IMJSON?) throws {
        try requireContext()
        try session.store.comparePasswordChangeIntent(namespace: Self.metadata(session), previous: previous, next: next)
    }

    func prepare() async throws -> IMJSON {
        try requireContext()
        // First confirm a usable current family before an unknown change can freeze refresh.
        if requestOverride == nil {
            _ = try await session.authorized { api, token in
                return try await api.request("auth/context", token: token)
            }
        }
        _ = try await capability(required: true)
        guard let pair = session.currentCredentialPair, pair.username == username,
            pair.profile == profile, pair.deploymentID == deployment, pair.sourceContextID == "personal",
            pair.restoreEpoch == session.restoreEpoch,
            try !rows().contains(where: { $0["familyId"].text == pair.familyID && ["unsent", "unknown", "pending"].contains($0["state"].text) }) else {
            throw IMAPIError(status: 409, code: "PASSWORD_CHANGE_ORIGINAL_PENDING", message: IMStrings.text("passwordChangeUnknown"))
        }
        let row: IMJSON = .object([
            "id": .string(UUID().uuidString.lowercased()), "username": .string(username),
            "profile": .string(profile), "deploymentId": .string(deployment),
            "userId": .string(pair.userID), "deviceId": .string(pair.deviceID),
            "familyId": .string(pair.familyID), "restoreEpoch": .string(pair.restoreEpoch),
            "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())),
            "state": .string("unsent"),
        ])
        try save(row, previous: nil)
        return row
    }

    static func proof(current: String, replacement: String, confirmation: String, otp: String) throws -> [String: IMJSON] {
        let before = current.precomposedStringWithCanonicalMapping
        let after = replacement.precomposedStringWithCanonicalMapping
        guard !before.isEmpty, before.utf8.count <= 512, (15...128).contains(after.unicodeScalars.count),
            after.utf8.count <= 512, before != after, after == confirmation.precomposedStringWithCanonicalMapping,
            otp.isEmpty || otp.range(of: "^[0-9]{6}$", options: .regularExpression) != nil else {
            throw IMAPIError(status: 0, code: "PASSWORD_CHANGE_INPUT_INVALID", message: IMStrings.text("passwordChangeInvalid"))
        }
        var fields: [String: IMJSON] = ["currentPassword": .string(before), "newPassword": .string(after)]
        if !otp.isEmpty {
            fields["otp"] = .string(otp)
        }
        return fields
    }

    private func body(_ row: IMJSON, proof: [String: IMJSON], includeOTP: Bool) -> [String: IMJSON] {
        var fields = proof
        if !includeOTP {
            fields.removeValue(forKey: "otp")
        }
        fields["requestId"] = row["id"]
        fields["username"] = row["username"]
        fields["deviceId"] = row["deviceId"]
        fields["restoreEpoch"] = row["restoreEpoch"]
        return fields
    }

    private func originalToken(_ row: IMJSON) -> String? {
        guard let pair = session.currentCredentialPair,
            pair.familyID == row["familyId"].text, pair.userID == row["userId"].text,
            pair.deviceID == row["deviceId"].text, pair.username == username,
            pair.profile == profile, pair.deploymentID == deployment,
            pair.restoreEpoch == row["restoreEpoch"].text else {
            return nil
        }
        return pair.accessToken
    }

    private static func counter(_ value: String) -> Bool {
        return value.range(of: "^[1-9][0-9]{0,19}$", options: .regularExpression) != nil && UInt64(value) != nil
    }

    private func accept(_ result: IMJSON, row: IMJSON) throws -> IMJSON {
        guard result["requestId"] == row["id"], result["restoreEpoch"] == row["restoreEpoch"],
            ["pending", "changed", "retired"].contains(result["status"].text),
            Self.counter(result["credentialGeneration"].text), Self.counter(result["securityVersion"].text),
            [.bool(true), .bool(false)].contains(result["current"]),
            [.bool(true), .bool(false)].contains(result["requiresNewLogin"]),
            result["current"] != .bool(true) || result["status"].text == "changed",
            result["status"].text != "changed" || result["requiresNewLogin"] == .bool(true) else {
            throw IMAPIError(status: 0, code: "PASSWORD_CHANGE_RESULT_UNKNOWN", message: IMStrings.text("passwordChangeUnknown"))
        }
        let latest = try original(row["id"].text)
        guard !["changed", "retired"].contains(latest["state"].text) || latest["state"] == result["status"] else {
            throw IMAPIError(status: 0, code: "PASSWORD_CHANGE_RESULT_UNKNOWN", message: IMStrings.text("passwordChangeUnknown"))
        }
        let next = latest.setting("state", result["status"])
        try save(next, previous: latest)
        if result["requiresNewLogin"] == .bool(true), originalToken(row) != nil {
            session.invalidateAuthentication(namespace: session.namespace, token: session.token)
        }
        return result
    }

    func perform(_ id: String, proof: [String: IMJSON], action: Action) async throws -> IMJSON? {
        try requireContext()
        let lock = profile + ":" + username
        guard Self.working.insert(lock).inserted else {
            throw IMAPIError(status: 409, code: "PASSWORD_CHANGE_BUSY", message: IMStrings.text("pending"))
        }
        defer {
            Self.working.remove(lock)
        }
        let row = try original(id)
        guard Set(proof.keys).isSubset(of: ["currentPassword", "newPassword", "otp"]),
            proof["currentPassword"]?.text.isEmpty == false, proof["newPassword"]?.text.isEmpty == false else {
            throw IMAPIError(status: 0, code: "PASSWORD_CHANGE_INPUT_INVALID", message: IMStrings.text("passwordChangeInvalid"))
        }
        let capturedToken = originalToken(row)
        if action == .submit {
            guard row["state"].text == "unsent", capturedToken != nil else {
                throw IMAPIError(status: 409, code: "PASSWORD_CHANGE_FAMILY_REQUIRED", message: IMStrings.text("connectionNeeded"))
            }
        }
        let available = try await capability(row, required: action == .submit)
        if action != .check && ["unsent", "unknown", "pending"].contains(row["state"].text) {
            let latest = try original(id)
            try save(latest.setting("state", .string("unknown")), previous: latest)
            session.stopAuthoritativeSync()
        }
        do {
            let value = try await request("auth/password-change-results", body: body(row, proof: proof, includeOTP: false))
            let accepted = try accept(value, row: row)
            if action != .retry || accepted["status"].text != "pending" {
                return accepted
            }
            guard available else {
                throw IMAPIError(status: 409, code: "PASSWORD_CHANGE_UNAVAILABLE", message: IMStrings.text("passwordChangeUnknown"))
            }
            // A prepared server responsibility is resumed anonymously; no successor family is borrowed.
            return try accept(await request("auth/password-change", body: body(row, proof: proof, includeOTP: false)), row: row)
        } catch let error as IMAPIError where error.status == 404 && error.code == "PASSWORD_CHANGE_OPERATION_NOT_FOUND" {
            guard action != .check else {
                return nil
            }
            guard available, let token = capturedToken, token == originalToken(row),
                ["unsent", "unknown"].contains(row["state"].text) else {
                throw IMAPIError(status: 409, code: "PASSWORD_CHANGE_FAMILY_REQUIRED", message: IMStrings.text("passwordChangeUnknown"))
            }
            return try accept(await request("auth/password-change", body: body(row, proof: proof, includeOTP: true), token: token), row: row)
        }
    }
}
