//
//  IMForwardService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
final class IMForwardService {
    private let session: IMSession
    private let sendRequest: ((String, [String: IMJSON]) async throws -> IMJSON)?
    private let readRequest: ((String) async throws -> IMJSON)?

    init(
        session: IMSession = .shared,
        sendRequest: ((String, [String: IMJSON]) async throws -> IMJSON)? = nil,
        readRequest: ((String) async throws -> IMJSON)? = nil
    ) {
        self.session = session
        self.sendRequest = sendRequest
        self.readRequest = readRequest
    }

    static func isForward(_ message: IMJSON) -> Bool {
        return message["forwarded"] == .bool(true) || message["type"].text == "forward"
    }

    func requireProtocol() async throws {
        let descriptor = try await session.verifyRestoreEpoch()
        guard descriptor["features"]["send_admission"] == .bool(true),
            descriptor["features"]["forwarding"] == .bool(true)
        else {
            throw failure("FORWARD_PROTOCOL_UNSUPPORTED", key: "forwardUnavailable")
        }
    }

    func current(_ message: IMJSON) async throws -> IMJSON {
        let namespace = session.namespace
        let id = message["id"].text
        let conversation = message["conversationId"].text
        guard id.range(of: "^[A-Za-z0-9_-]+$", options: .regularExpression) != nil,
            !conversation.isEmpty
        else {
            throw IMSession.SessionError.invalidInstance
        }
        let generation = try session.store.forwardProjectionGeneration(
            messageID: id, conversation: conversation, namespace: namespace)
        do {
            let latest: IMJSON
            if let readRequest {
                latest = try await readRequest("messages/" + id)
            } else {
                latest = try await session.action("messages/" + id, method: "GET")
            }
            guard session.namespace == namespace, latest["id"].text == id,
                latest["conversationId"].text == conversation
            else {
                throw IMSession.SessionError.noInstance
            }
            if Self.isForward(latest) || Self.isForward(message) {
                try session.store.applyForwardProjection(
                    latest, conversation: conversation, namespace: namespace,
                    expectedGeneration: generation)
            } else {
                try session.store.save([latest], collection: "messages:" + conversation, namespace: namespace)
            }
            if latest["deleted"] == .bool(true) || latest["contentUnavailable"] == .bool(true)
                || latest["forwardUnavailable"] == .bool(true)
            {
                IMMediaCache.purge(messageID: id, namespace: namespace)
                session.removeForwardView(id, namespace: namespace)
            }
            if Self.isForward(latest), latest["deleted"] != .bool(true),
                latest["contentUnavailable"] != .bool(true), latest["forwardUnavailable"] != .bool(true)
            {
                let view = latest.setting("forwardProjectionGeneration", .string(generation))
                return try session.acceptForwardView(view, namespace: namespace, generation: generation)
            }
            if !Self.isForward(latest), latest["deleted"] != .bool(true),
                let stored = try session.store.list("messages:" + conversation, namespace: namespace)
                    .first(where: { $0["id"].text == id }),
                stored["deleted"] != .bool(true), stored["version"] == latest["version"]
            {
                // 源转发许可只交给本次显式操作，不从持久历史回读临时proof。
                return latest
            }
            return try session.store.list("messages:" + conversation, namespace: namespace)
                .first(where: { $0["id"].text == id }) ?? latest
        } catch {
            if Self.isForward(message), session.namespace == namespace {
                session.removeForwardView(id, namespace: namespace)
                try? session.store.redactForward(
                    messageID: id, conversation: conversation, namespace: namespace)
                IMMediaCache.purge(messageID: id, namespace: namespace)
            }
            throw error
        }
    }

    func prepare(_ displayed: IMJSON) async throws -> IMJSON {
        try await requireProtocol()
        let latest = try await current(displayed)
        guard latest["deleted"] != .bool(true), latest["contentUnavailable"] != .bool(true),
            latest["forwardUnavailable"] != .bool(true), !latest["forwardAdmission"].text.isEmpty
        else {
            throw failure("FORWARD_SOURCE_UNAVAILABLE", key: "forwardUnavailable")
        }
        guard !displayed["contentVersion"].text.isEmpty,
            latest["contentVersion"] == displayed["contentVersion"]
        else {
            throw failure("FORWARD_SOURCE_RESELECT", key: "forwardChanged")
        }
        return latest
    }

    func send(_ source: IMJSON, to target: String, clientID: String = UUID().uuidString) async throws {
        let namespace = session.namespace
        try await requireProtocol()
        let destination = try await IMMessageListService(session: session).conversation(target)
        guard session.namespace == namespace else {
            throw IMSession.SessionError.noInstance
        }
        let admission = try session.admissionSnapshot(target, supplied: destination["sendAdmission"].text)
        guard !source["forwardAdmission"].text.isEmpty, !source["contentVersion"].text.isEmpty else {
            throw failure("FORWARD_SOURCE_UNAVAILABLE", key: "forwardUnavailable")
        }
        // 原引用与两个资格快照先持久化；待发记录不包含投影正文或附件。
        let body: IMJSON = .object([
            "id": .string(clientID), "clientMsgId": .string(clientID),
            "conversationId": .string(target), "senderId": session.user["id"],
            "type": .string("forward"), "forwardOfId": source["id"],
            "forwardSourceVersion": source["contentVersion"], "forwardAdmission": source["forwardAdmission"],
            "restoreEpoch": .string(session.restoreEpoch), "sendAdmission": .string(admission),
            "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())),
            "status": .string("queued"),
        ])
        try session.store.enqueue(body, conversation: target, namespace: namespace)
        try await submit(body, namespace: namespace)
    }

    func retryOriginal(_ clientID: String) async throws {
        let namespace = session.namespace
        guard let original = try session.store.originalOutbox(clientID: clientID, namespace: namespace),
            original["type"].text == "forward",
            let visible = try session.store.list(
                "messages:" + original["conversationId"].text, namespace: namespace
            )
            .first(where: { $0["clientMsgId"].text == clientID }),
            visible["status"].text == "frozen", visible["freezeReason"].text == "forward_unknown"
        else {
            throw failure("FORWARD_RETRY_FORBIDDEN", key: "forwardUnavailable")
        }
        do {
            try await session.reconcileOriginalMessage(clientID)
            return
        } catch let error as IMAPIError where error.status == 404 && error.code == "MESSAGE_RESULT_NOT_FOUND"
        {
            // 仅用户明确选择重试后，原查询无事实才提交完全相同的原引用。
        }
        try await requireProtocol()
        guard session.namespace == namespace else {
            throw IMSession.SessionError.noInstance
        }
        try await submit(original, namespace: namespace)
    }

    private func submit(_ original: IMJSON, namespace: IMNamespace) async throws {
        let clientID = original["clientMsgId"].text
        let conversation = original["conversationId"].text
        do {
            guard session.namespace == namespace else {
                throw IMSession.SessionError.noInstance
            }
            let path = "conversations/" + conversation + "/messages"
            let payload = IMSession.wireFields(original.fields)
            let result: IMJSON
            if let sendRequest {
                result = try await sendRequest(path, payload)
            } else {
                result = try await session.action(path, body: payload)
            }
            guard session.namespace == namespace, result["clientMsgId"].text == clientID,
                result["conversationId"].text == conversation, result["senderId"] == session.user["id"]
            else {
                throw IMSession.SessionError.invalidInstance
            }
            try await session.reconcileOriginalMessage(clientID)
        } catch {
            let apiError = error as? IMAPIError
            let unknown =
                apiError == nil || [0, 401, 408, 425, 429].contains(apiError?.status ?? 0)
                || (apiError?.status ?? 0) >= 500
            try session.store.freezeOutbox(
                clientID: clientID, conversation: conversation, namespace: namespace,
                reason: unknown ? "forward_unknown" : "forward_invalid")
            throw error
        }
    }

    func refreshCached(_ conversation: String) async {
        guard !session.demo else {
            return
        }
        let namespace = session.namespace
        let values = (try? session.store.list("messages:" + conversation, namespace: namespace)) ?? []
        for value in values where Self.isForward(value) && value["status"].text.isEmpty {
            guard session.namespace == namespace else {
                return
            }
            _ = try? await current(value)
        }
    }

    private func failure(_ code: String, key: String) -> IMAPIError {
        return IMAPIError(status: 409, code: code, message: IMStrings.text(key))
    }
}
