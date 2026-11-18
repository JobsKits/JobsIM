//
//  IMMessageEditingService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation
import JobsIMCore
import JobsIMTransport

enum IMDraftReplacementPolicy: Equatable {
    case overwrite
    case saveCurrent
}

extension IMSession {
    func recallMessage(_ message: IMJSON, conversationID: String) async throws {
        let ns = namespace
        let id = message["id"].text
        guard !id.isEmpty, message["senderId"] == user["id"],
            message["deleted"] != .bool(true), message["contentUnavailable"] != .bool(true),
            message["conversationId"].text.isEmpty || message["conversationId"].text == conversationID
        else {
            throw SessionError.invalidInstance
        }
        if demo {
            let removed = message.setting("deleted", .bool(true)).setting("text", .string(""))
                .setting("conversationId", .string(conversationID))
            try store.save([removed], collection: "messages:" + conversationID, namespace: ns)
            try store.remove(id, collection: "saved", namespace: ns)
            return
        }
        guard UUID(uuidString: id) != nil, UUID(uuidString: conversationID) != nil,
            UInt64(message["version"].text) != nil
        else {
            throw SessionError.invalidInstance
        }
        let collection = "message-retraction-outbox"
        let previous = try store.list(collection, namespace: ns).first {
            $0["id"].text == id
        }
        if let previous, previous["expectedVersion"] != message["version"] {
            throw IMAPIError(status: 409, code: "VERSION_CONFLICT", message: IMStrings.text("messageChanged"))
        }
        let intent =
            previous
            ?? .object([
                "id": .string(id), "conversationId": .string(conversationID), "senderId": user["id"],
                "requestId": .string(UUID().uuidString), "expectedVersion": message["version"],
            ])
        try store.save([intent], collection: collection, namespace: ns)
        let result = try await action(
            "messages/" + id, method: "DELETE",
            body: [
                "requestId": intent["requestId"], "expectedVersion": intent["expectedVersion"],
            ])
        guard namespace == ns, result["id"].text == id,
            result["conversationId"].text == conversationID,
            result["senderId"] == user["id"], result["deleted"] == .bool(true)
        else {
            throw SessionError.invalidInstance
        }
        try store.remove(id, collection: collection, namespace: ns)
        await synchronizeAuthoritatively()
        guard namespace == ns else {
            throw SessionError.noInstance
        }
    }

    func recallForEditing(
        _ message: IMJSON, conversationID: String,
        currentDraft: String, policy: IMDraftReplacementPolicy
    ) async throws -> String {
        let ns = namespace
        guard message["type"].text == "text", !message["text"].text.isEmpty else {
            throw SessionError.invalidInstance
        }
        let original = message["text"].text
        let draftID = UUID().uuidString
        let preserveDraft = policy == .saveCurrent && !currentDraft.isEmpty
        if preserveDraft {
            try store.save(
                [
                    .object([
                        "id": .string(draftID), "text": .string(currentDraft),
                        "conversationId": .string(conversationID), "sourceMessageId": message["id"],
                        "savedAt": .string(ISO8601DateFormatter.make().string(from: Date())),
                    ])
                ], collection: "saved-editing-drafts", namespace: ns)
        }
        // 只保存原有输入；被撤回消息不落一份绕过全局擦除的副本。
        try await recallMessage(message, conversationID: conversationID)
        guard namespace == ns else {
            throw SessionError.noInstance
        }
        return original
    }

    func savedEditingDrafts(conversationID: String) -> [IMJSON] {
        return ((try? store.list("saved-editing-drafts", namespace: namespace)) ?? [])
            .filter {
                $0["conversationId"].text == conversationID
            }
            .sorted {
                $0["savedAt"].text > $1["savedAt"].text
            }
    }

    func restoreEditingDraft(id: String, conversationID: String) throws -> String {
        guard
            let draft = try store.list("saved-editing-drafts", namespace: namespace).first(where: {
                $0["id"].text == id && $0["conversationId"].text == conversationID
            })
        else {
            throw SessionError.invalidInstance
        }
        return draft["text"].text
    }
}
