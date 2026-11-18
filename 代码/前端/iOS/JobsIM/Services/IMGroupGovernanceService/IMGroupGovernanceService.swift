//
//  IMGroupGovernanceService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
final class IMGroupGovernanceService {

    typealias Request = @MainActor (String, String, [String: IMJSON]) async throws -> IMJSON
    private let session: IMSession
    private let request: Request
    private static var working = Set<String>()

    init(session: IMSession = .shared, request: Request? = nil) {
        self.session = session
        self.request = request ?? { path, method, body in
            return try await session.action(path, method: method, body: body)
        }
    }

    private var metadata: IMNamespace {
        return IMNamespace(profile: session.profile, account: session.user["id"].text, demo: false,
                           restoreEpoch: "group-protocol", deploymentID: session.deployment, sourceContextID: "personal")
    }

    private func storage(_ row: IMJSON, original: IMNamespace) -> IMNamespace {
        return row["namespaceKey"] == .null ? original : metadata
    }

    func pending(_ conversation: String) throws -> IMJSON? {
        let current = try session.store.list("group-operations:" + conversation, namespace: metadata)
        let legacy = try session.store.list("group-operations:" + conversation, namespace: session.namespace)
        return (current + legacy)
            .first { $0["state"].text == "pending" }
    }

    func load(_ conversation: String) async throws -> IMJSON {
        return try await read(conversation, suffix: "governance")
    }

    func read(_ conversation: String, suffix: String) async throws -> IMJSON {
        let namespace = session.namespace
        guard UUID(uuidString: conversation) != nil else {
            throw IMSession.SessionError.invalidInstance
        }
        let value = try await request("conversations/" + conversation + "/" + suffix, "GET", [:])
        guard session.namespace == namespace, !Task.isCancelled else {
            throw IMSession.SessionError.noInstance
        }
        return value
    }

    func change(_ conversation: String, observed: IMJSON, action: String,
                changes: [String: IMJSON] = [:], target: IMJSON? = nil) async throws -> IMJSON {
        guard !observed["version"].text.isEmpty, !observed["membershipGeneration"].text.isEmpty,
            !observed["roleGeneration"].text.isEmpty else {
            throw IMSession.SessionError.invalidInstance
        }
        var body = changes
        body["action"] = .string(action)
        body["version"] = observed["version"]
        body["membershipGeneration"] = observed["membershipGeneration"]
        body["roleGeneration"] = observed["roleGeneration"]
        if let target {
            guard !target["membershipGeneration"].text.isEmpty, !target["roleGeneration"].text.isEmpty else {
                throw IMSession.SessionError.invalidInstance
            }
            body["targetId"] = target["id"]
            body["targetMembershipGeneration"] = target["membershipGeneration"]
            body["targetRoleGeneration"] = target["roleGeneration"]
        }
        return try await create(conversation, route: "governance", body: body)
    }

    func create(_ conversation: String, route: String, body: [String: IMJSON], runtime: [String: IMJSON] = [:], target: String = "") async throws -> IMJSON {
        let namespace = session.namespace
        guard body["token"] == nil, body["password"] == nil, body["otp"] == nil,
            runtime.keys.allSatisfy({ ["token", "otp"].contains($0) }), !session.demo, UUID(uuidString: conversation) != nil,
            session.currentCredentialPair != nil,
            ["governance", "invites", "join-requests", "invite_revoke", "join_decide"].contains(route),
            (target.isEmpty || UUID(uuidString: target) != nil),
            try pending(conversation) == nil else {
            throw IMAPIError(status: 409, code: "GROUP_OPERATION_PENDING", message: IMStrings.text("groupPending"))
        }
        let id = UUID().uuidString.lowercased()
        let original = body.merging(["requestId": .string(id)]) { _, value in value }
        let row: IMJSON = .object(["id": .string(id), "conversationId": .string(conversation),
                                  "familyId": .string(session.currentCredentialPair?.familyID ?? ""),
                                  "namespaceKey": .string(namespace.key), "restoreEpoch": .string(namespace.restoreEpoch),
                                  "route": .string(route), "targetId": .string(target), "payload": .object(original), "state": .string("pending"),
                                  "createdAt": .string(ISO8601DateFormatter.make().string(from: Date()))])
        try session.store.save([row], collection: "group-operations:" + conversation, namespace: metadata)
        return try await perform(row, namespace: namespace, retry: true, runtime: runtime)
    }

    func reconcile(_ conversation: String, retry: Bool, otp: String = "") async throws -> IMJSON? {
        guard let row = try pending(conversation) else {
            return nil
        }
        return try await perform(row, namespace: session.namespace, retry: retry, runtime: otp.isEmpty ? [:] : ["otp": .string(otp)])
    }

    private func perform(_ row: IMJSON, namespace: IMNamespace, retry: Bool, runtime: [String: IMJSON] = [:]) async throws -> IMJSON {
        let conversation = row["conversationId"].text
        let id = row["id"].text
        let key = namespace.key + ":" + conversation
        guard UUID(uuidString: conversation) != nil, UUID(uuidString: id) != nil,
            !Self.working.contains(key), ["governance", "invites", "join-requests", "invite_revoke", "join_decide"].contains(row["route"].text),
            row["payload"]["requestId"].text == id, session.namespace == namespace else {
            throw IMSession.SessionError.invalidInstance
        }
        Self.working.insert(key)
        defer {
            Self.working.remove(key)
        }
        let prefix = "conversations/" + conversation
        let management = ["invite_revoke", "join_decide"].contains(row["route"].text)
        if management && UUID(uuidString: row["targetId"].text) == nil {
            throw IMSession.SessionError.invalidInstance
        }
        let path = management
            ? prefix + (row["route"].text == "invite_revoke" ? "/invites/" : "/join-requests/") + row["targetId"].text
            : prefix + "/" + row["route"].text
        let lookup = management ? prefix + "/governance" : path
        let value: IMJSON
        do {
            value = try await request(lookup + "/operations/" + id, "GET", [:])
        } catch let error as IMAPIError where error.status == 404 && error.code == "GROUP_OPERATION_NOT_FOUND" {
            guard retry, session.namespace == namespace, !Task.isCancelled else {
                throw error
            }
            guard let pair = session.currentCredentialPair, !row["familyId"].text.isEmpty,
                pair.familyID == row["familyId"].text, pair.namespace == namespace,
                row["namespaceKey"].text == namespace.key, row["restoreEpoch"].text == namespace.restoreEpoch else {
                throw IMAPIError(status: 409, code: "GROUP_ORIGINAL_FAMILY_REQUIRED", message: IMStrings.text("groupPending"))
            }
            if row["route"].text == "join-requests" && runtime["token"]?.text.isEmpty != false {
                throw IMAPIError(status: 0, code: "GROUP_INVITATION_PROOF_REQUIRED", message: IMStrings.text("groupJoinHelp"))
            }
            var body = row["payload"].fields
            if row["payload"]["action"].text == "recover" {
                guard let otp = runtime["otp"]?.text, otp.range(of: "^[0-9]{6}$", options: .regularExpression) != nil else {
                    throw IMAPIError(status: 0, code: "MFA_REQUIRED", message: IMStrings.text("authOTP"))
                }
                let authorization = try await request("auth/step-up", "POST", [
                    "action": .string("group.recover"), "targetId": .string(conversation),
                    "requestId": .string(id), "payload": row["payload"], "otp": .string(otp),
                ])
                guard UUID(uuidString: authorization["authorizationId"].text) != nil,
                    session.namespace == namespace, !Task.isCancelled else {
                    throw IMSession.SessionError.invalidInstance
                }
                body["stepUpId"] = authorization["authorizationId"]
            } else if row["route"].text == "join-requests" {
                body["token"] = runtime["token"]
            }
            do {
                value = try await request(path, row["route"].text == "invite_revoke" ? "DELETE" : "POST", body)
            } catch let failure as IMAPIError {
                let rejected = ["GROUP_PERMISSION_CHANGED", "GROUP_TARGET_CHANGED", "GROUP_TARGET_UNAVAILABLE",
                                "GROUP_READ_ONLY", "GROUP_ROLE_BOUNDARY", "OWNER_REQUIRED", "GROUP_ADMIN_REQUIRED"]
                if session.namespace == namespace && rejected.contains(failure.code) {
                    try session.store.save([row.setting("state", .string("rejected"))],
                                           collection: "group-operations:" + conversation, namespace: storage(row, original: namespace))
                }
                throw failure
            }
        }
        guard session.namespace == namespace, !Task.isCancelled, value["requestId"].text == id else {
            throw IMSession.SessionError.invalidInstance
        }
        if management {
            let expected = row["route"].text == "invite_revoke" ? "revoked" : row["payload"]["approve"] == .bool(true) ? "approved" : "rejected"
            guard value["status"].text == "applied", value["action"] == row["route"],
                value["targetId"] == row["targetId"], value["state"].text == expected else {
                throw IMSession.SessionError.invalidInstance
            }
        } else if row["route"].text == "governance" {
            guard value["status"].text == "applied", value["action"] == row["payload"]["action"],
                ((try? IMSyncProtocol.counter(value["version"])) ?? 0) > 0 else {
                throw IMSession.SessionError.invalidInstance
            }
        } else if row["route"].text == "join-requests" {
            guard UUID(uuidString: value["id"].text) != nil,
                ["pending", "approved", "rejected"].contains(value["state"].text) else {
                throw IMSession.SessionError.invalidInstance
            }
        } else {
            guard UUID(uuidString: value["id"].text) != nil, !value["token"].text.isEmpty else {
                throw IMSession.SessionError.invalidInstance
            }
        }
        try session.store.save([row.setting("state", .string("complete"))],
                               collection: "group-operations:" + conversation, namespace: storage(row, original: namespace))
        return value
    }

    private func snapshot(_ observed: IMJSON) -> [String: IMJSON] {
        return ["version": observed["version"], "membershipGeneration": observed["membershipGeneration"],
                "roleGeneration": observed["roleGeneration"]]
    }

    func decide(_ conversation: String, observed: IMJSON, requestID: String, approve: Bool) async throws {
        var body = snapshot(observed)
        body["approve"] = .bool(approve)
        _ = try await create(conversation, route: "join_decide", body: body, target: requestID)
    }

    func revoke(_ conversation: String, observed: IMJSON, invite: String) async throws {
        _ = try await create(conversation, route: "invite_revoke", body: snapshot(observed), target: invite)
    }

    func recover(_ conversation: String, observed: IMJSON, target: IMJSON, otp: String) async throws -> IMJSON {
        var body = snapshot(observed)
        body["action"] = .string("recover")
        body["targetId"] = target["id"]
        body["targetMembershipGeneration"] = target["membershipGeneration"]
        return try await create(conversation, route: "governance", body: body, runtime: ["otp": .string(otp)])
    }

    func invitationCode(_ conversation: String, invite: IMJSON) throws -> String {
        let data = try JSONEncoder.make().encode(IMJSON.object([
            "deploymentId": .string(session.deployment), "conversationId": .string(conversation),
            "inviteId": invite["id"], "token": invite["token"],
        ]))
        return "JobsIM-Invite-1:" + data.base64EncodedString()
    }

    func join(_ code: String) async throws -> IMJSON {
        guard code.hasPrefix("JobsIM-Invite-1:"), code.utf8.count < 4096,
            let data = Data(base64Encoded: String(code.dropFirst("JobsIM-Invite-1:".count))) else {
            throw IMSession.SessionError.invalidInstance
        }
        let value = try JSONDecoder.make().decode(IMJSON.self, from: data)
        guard value["deploymentId"].text == session.deployment,
            UUID(uuidString: value["conversationId"].text) != nil,
            UUID(uuidString: value["inviteId"].text) != nil, !value["token"].text.isEmpty else {
            throw IMSession.SessionError.invalidInstance
        }
        let conversation = value["conversationId"].text
        let runtime: [String: IMJSON] = ["token": value["token"]]
        if let original = try pending(conversation) {
            guard original["route"].text == "join-requests",
                original["payload"]["inviteId"] == value["inviteId"] else {
                throw IMAPIError(status: 409, code: "GROUP_OPERATION_PENDING", message: IMStrings.text("groupPending"))
            }
            return try await perform(original, namespace: session.namespace, retry: true, runtime: runtime)
        }
        return try await create(conversation, route: "join-requests",
                                body: ["inviteId": value["inviteId"]], runtime: runtime)
    }
}
