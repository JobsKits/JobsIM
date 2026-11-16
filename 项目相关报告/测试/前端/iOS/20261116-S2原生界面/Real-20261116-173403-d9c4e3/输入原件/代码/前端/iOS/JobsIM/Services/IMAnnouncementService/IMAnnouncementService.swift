//
//  IMAnnouncementService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
final class IMAnnouncementService {

    typealias Request = @MainActor (String, String, [String: IMJSON]) async throws -> IMJSON
    private let session: IMSession
    private let request: Request

    init(session: IMSession = .shared, request: Request? = nil) {
        self.session = session
        self.request =
            request ?? { path, method, body in
                try await session.action(path, method: method, body: body)
            }
    }

    func pending(_ conversation: String) throws -> IMJSON? {
        return try session.store.list("announcement-operations:" + conversation, namespace: session.namespace)
            .last { $0["state"].text == "pending" }
    }

    func load(_ conversation: String) async throws -> IMJSON {
        let namespace = session.namespace
        let value = try await request("conversations/" + conversation + "/announcement", "GET", [:])
        guard session.namespace == namespace, value["conversationId"].text == conversation else {
            throw IMSession.SessionError.noInstance
        }
        return value
    }

    func publish(_ source: IMJSON, expectedVersion: String? = nil, expiresAt: Date? = nil) async throws
        -> IMJSON
    {
        return try await change(
            source["conversationId"].text, source: source, expectedVersion: expectedVersion,
            expiresAt: expiresAt)
    }

    func clear(_ conversation: String, expectedVersion: String? = nil) async throws -> IMJSON {
        return try await change(conversation, source: nil, expectedVersion: expectedVersion, expiresAt: nil)
    }

    private func change(_ conversation: String, source: IMJSON?, expectedVersion: String?, expiresAt: Date?)
        async throws -> IMJSON
    {
        let namespace = session.namespace
        guard !session.demo, !conversation.isEmpty, try pending(conversation) == nil else {
            throw IMAPIError(
                status: 409, code: "ANNOUNCEMENT_PENDING", message: IMStrings.text("announcementPending"))
        }
        let descriptor = try await request("bootstrap", "GET", [:])
        guard session.namespace == namespace, descriptor["features"]["role_admission"] == .bool(true) else {
            throw IMAPIError(
                status: 409, code: "QUALIFICATION_CHANGED", message: IMStrings.text("sendAdmissionPaused"))
        }
        let metadata = try await IMMessageListService(session: session).conversation(conversation)
        let current = try await load(conversation)
        if let expectedVersion, current["version"].text != expectedVersion {
            throw IMAPIError(
                status: 409, code: "ANNOUNCEMENT_VERSION_CONFLICT", message: IMStrings.text("error"))
        }
        guard session.namespace == namespace, current["canManage"] == .bool(true),
            !metadata["membershipGeneration"].text.isEmpty, !metadata["roleGeneration"].text.isEmpty,
            try pending(conversation) == nil
        else {
            throw IMAPIError(status: 403, code: "ANNOUNCEMENT_FORBIDDEN", message: IMStrings.text("readOnly"))
        }
        let operation = UUID().uuidString
        var body: [String: IMJSON] = [
            "expectedVersion": current["version"],
            "membershipGeneration": metadata["membershipGeneration"],
            "roleGeneration": metadata["roleGeneration"],
            "requestId": .string(operation),
        ]
        if let source {
            guard source["conversationId"].text == conversation, !source["id"].text.isEmpty,
                source["deleted"] != .bool(true), source["contentUnavailable"] != .bool(true)
            else {
                throw IMSession.SessionError.invalidInstance
            }
            body["messageId"] = source["id"]
            body["sourceVersion"] =
                source["contentVersion"].text.isEmpty ? source["version"] : source["contentVersion"]
            if let expiresAt {
                body["expiresAt"] = .string(ISO8601DateFormatter.make().string(from: expiresAt))
            }
        }
        let method = source == nil ? "DELETE" : "PUT"
        let record = IMJSON.object([
            "id": .string(operation), "conversationId": .string(conversation), "method": .string(method),
            "payload": .object(body), "state": .string("pending"),
            "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())),
        ])
        try session.store.save(
            [record], collection: "announcement-operations:" + conversation, namespace: namespace)
        do {
            let result = try await request("conversations/" + conversation + "/announcement", method, body)
            guard session.namespace == namespace else {
                throw IMSession.SessionError.noInstance
            }
            try settle(record, result: result, namespace: namespace)
            return result
        } catch {
            if let failure = error as? IMAPIError, (400..<500).contains(failure.status) {
                try session.store.save(
                    [record.setting("state", .string("rejected"))],
                    collection: "announcement-operations:" + conversation, namespace: namespace)
            }
            throw error
        }
    }

    func checkOriginal(_ conversation: String) async throws -> IMJSON {
        let namespace = session.namespace
        guard let original = try pending(conversation) else {
            throw IMSession.SessionError.invalidInstance
        }
        let result = try await request(
            "conversations/" + conversation + "/announcement/operations/" + original["id"].text, "GET", [:])
        guard session.namespace == namespace else {
            throw IMSession.SessionError.noInstance
        }
        try settle(original, result: result, namespace: namespace)
        return result
    }

    private func settle(_ original: IMJSON, result: IMJSON, namespace: IMNamespace) throws {
        let conversation = original["conversationId"].text
        guard result["operationId"] == original["id"], result["status"].text == "applied",
            result["result"]["conversationId"].text == conversation
        else {
            throw IMSession.SessionError.invalidInstance
        }
        // Persist only completion identity; current source text always comes from a new GET.
        let completed = original.setting("state", .string("completed"))
            .setting("resultVersion", result["result"]["version"])
            .setting("current", result["current"])
        try session.store.save(
            [completed], collection: "announcement-operations:" + conversation, namespace: namespace)
    }
}
