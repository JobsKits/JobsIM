//
//  IMPersonalStateService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import Foundation
import JobsIMCore
import JobsIMTransport

struct IMPersonalEditorSnapshot: Equatable {

    let text: String
    let reply: IMJSON
    let mentionIDs: [String]
    let mentionAll: Bool

    func requireUnchanged(_ current: IMPersonalEditorSnapshot) throws {
        guard self == current else {
            try IMSyncProtocol.refuse("LOCAL_DRAFT_CHANGED")
        }
    }
}

@MainActor
final class IMPersonalStateService {

    private let session: IMSession
    let namespace: IMNamespace
    private let family: String?

    init(session: IMSession = .shared) {
        self.session = session
        namespace = session.namespace
        family = session.currentCredentialPair?.familyID
    }

    func requireCurrent() throws {
        guard !Task.isCancelled, session.namespace == namespace,
            family != nil, session.currentCredentialPair?.familyID == family,
            session.personalStatesEnabled
        else {
            try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
        }
    }

    func operations(conversation: String? = nil) throws -> [IMJSON] {
        try requireCurrent()
        return try session.store.list("self-state-operations", namespace: namespace).filter {
            $0["familyId"].text == family && ["pending", "rejected"].contains($0["state"].text)
                && (conversation == nil || $0["conversationId"].text == conversation)
        }
    }

    private func requireNoPending(_ path: String) throws {
        guard try !operations().contains(where: { $0["path"].text == path && $0["state"] == .string("pending") }) else {
            try IMSyncProtocol.refuse("SELF_OPERATION_PENDING")
        }
    }

    func read(_ kind: String, conversation: String) async throws -> IMJSON {
        try requireCurrent()
        _ = try IMSyncProtocol.id(.string(conversation))
        let value = try await session.action("conversations/" + conversation + "/" + kind, method: "GET")
        try requireCurrent()
        try Self.validateCurrent(value, kind: kind, conversation: conversation, namespace: namespace)
        return value
    }

    static func validateCurrent(_ value: IMJSON, kind: String, conversation: String,
                                namespace: IMNamespace) throws {
        guard value["scopeId"].text == namespace.deploymentID,
            value["sourceContextId"] == .string("personal"), value["userId"].text == namespace.account,
            value["revisionEpoch"].text == namespace.restoreEpoch,
            value["conversationId"].text == conversation
        else {
            try IMSyncProtocol.refuse("SYNC_PERSONAL_IDENTITY_MISMATCH")
        }
        _ = try IMSyncProtocol.id(value["id"])
        _ = try IMSyncProtocol.counter(value["version"])
        if kind == "view" {
            let ref: IMJSON = .object(["id": value["id"], "subjectUserId": .string(namespace.account),
                                      "conversationId": .string(conversation), "kind": .string("conversation_view")])
            try IMSyncProtocol.personalBody(value, reference: ref)
        } else if kind == "read" {
            _ = try IMSyncProtocol.counter(value["readPoint"]["epoch"])
            _ = try IMSyncProtocol.counter(value["readPoint"]["sequence"])
            _ = try IMSyncProtocol.counter(value["membershipGeneration"])
            guard case .bool = value["writable"],
                ["exact_current_source", "not_computed"].contains(value["countStatus"].text)
            else {
                try IMSyncProtocol.refuse()
            }
            if value["countStatus"] == .string("exact_current_source") {
                _ = try IMSyncProtocol.counter(value["unreadCount"])
                _ = try IMSyncProtocol.counter(value["countSourceWatermark"])
            }
        } else if kind == "draft" {
            var projection = value.fields
            projection.removeValue(forKey: "draftAdmission")
            projection.removeValue(forKey: "recovery")
            let ref: IMJSON = .object(["id": value["id"], "subjectUserId": .string(namespace.account),
                                      "conversationId": .string(conversation), "kind": .string("draft")])
            try IMSyncProtocol.personalBody(.object(projection), reference: ref)
            guard case .bool = value["writable"],
                try IMSyncProtocol.text(value["draftAdmission"], limit: 16384).hasPrefix("da1.")
            else {
                try IMSyncProtocol.refuse()
            }
            if value["recovery"] != .null {
                _ = try IMSyncProtocol.id(value["recovery"]["operationId"])
                guard case .string(let text) = value["recovery"]["ownedText"], text.utf8.count <= 16384 else {
                    try IMSyncProtocol.refuse()
                }
            }
        } else {
            try IMSyncProtocol.refuse()
        }
    }

    func changeView(_ changes: [String: IMJSON], conversation: String, expected: IMJSON) async throws -> IMJSON {
        let path = "conversations/" + conversation + "/view"
        try requireNoPending(path)
        let current = try await read("view", conversation: conversation)
        guard current["version"] == expected["version"],
            current["manualUnreadGeneration"] == expected["manualUnreadGeneration"]
        else {
            try IMSyncProtocol.refuse("VIEW_CHANGED_RESELECT")
        }
        var patch = changes
        if patch["manualUnread"] != nil {
            patch["expectedManualUnreadGeneration"] = current["manualUnreadGeneration"]
        }
        try requireNoPending(path)
        return try await session.writePersonalState(path: path, method: "PATCH", conversation: conversation,
                                                    body: ["revisionEpoch": .string(namespace.restoreEpoch),
                                                           "expectedVersion": current["version"], "changes": .object(patch)])
    }

    private func verifiedReply(_ seen: IMJSON, conversation: String) async throws -> IMJSON {
        let id = try IMSyncProtocol.id(seen["messageId"])
        guard try IMSyncProtocol.counter(seen["sourceVersion"]) > 0 else {
            try IMSyncProtocol.refuse()
        }
        let source = try await session.action("messages/" + id, method: "GET")
        try requireCurrent()
        guard source["id"].text == id, source["conversationId"].text == conversation,
            source["contentVersion"] == seen["sourceVersion"],
            source["deleted"] != .bool(true), source["contentUnavailable"] != .bool(true),
            source["forwardUnavailable"] != .bool(true),
            try IMSyncProtocol.text(source["replyAdmission"], limit: 16384).hasPrefix("dr1.")
        else {
            try IMSyncProtocol.refuse("DRAFT_SOURCE_CHANGED")
        }
        _ = try IMSyncProtocol.id(source["senderId"])
        _ = try IMSyncProtocol.counter(source["version"])
        return seen.setting("replyAdmission", source["replyAdmission"])
    }

    func saveDraft(_ editor: IMPersonalEditorSnapshot, conversation: String, expected: IMJSON,
                   clear: Bool = false) async throws -> IMJSON {
        let path = "conversations/" + conversation + "/draft"
        try requireNoPending(path)
        let current = try await read("draft", conversation: conversation)
        guard current["version"] == expected["version"] else {
            try IMSyncProtocol.refuse("DRAFT_CHANGED_RESELECT")
        }
        var body = ["revisionEpoch": IMJSON.string(namespace.restoreEpoch), "expectedVersion": current["version"],
                    "draftAdmission": current["draftAdmission"]]
        if !clear {
            guard current["writable"] == .bool(true), editor.text.utf8.count <= 16384 else {
                try IMSyncProtocol.refuse("DRAFT_NOT_WRITABLE")
            }
            body["ownedText"] = .string(editor.text)
            body["reply"] = editor.reply == .null ? .null : try await verifiedReply(editor.reply, conversation: conversation)
        }
        try requireCurrent()
        try requireNoPending(path)
        return try await session.writePersonalState(path: path, method: clear ? "DELETE" : "PUT",
                                                    conversation: conversation, body: body)
    }

    func textForAdoption(conversation: String, expected: IMJSON, recovery: Bool,
                         includeReply: Bool) async throws -> (String, IMJSON) {
        let latest = try await read("draft", conversation: conversation)
        guard latest["version"] == expected["version"],
            !recovery || latest["recovery"]["operationId"] == expected["recovery"]["operationId"]
        else {
            try IMSyncProtocol.refuse("DRAFT_CHANGED_RESELECT")
        }
        let owned = recovery ? latest["recovery"]["ownedText"] : latest["ownedText"]
        guard case .string(let text) = owned, !text.isEmpty else {
            try IMSyncProtocol.refuse("DRAFT_NO_OWNED_TEXT")
        }
        var reply = IMJSON.null
        if includeReply, !recovery, latest["replyState"] == .string("available"), latest["reply"] != .null {
            let verified = try await verifiedReply(latest["reply"], conversation: conversation)
            reply = .object(["messageId": verified["messageId"], "sourceVersion": verified["sourceVersion"]])
        }
        try requireCurrent()
        return (text, reply)
    }

    func resume(_ original: IMJSON, retry: Bool) async throws -> IMJSON {
        try requireCurrent()
        guard original["state"] == .string("pending"), original["familyId"].text == family,
            try operations().contains(original)
        else {
            try IMSyncProtocol.refuse("SELF_ORIGINAL_OPERATION_MISSING")
        }
        // The current local guard is captured by writePersonalState; never rewrite the original body.
        return try await session.writePersonalState(path: original["path"].text, method: original["method"].text,
                                                    conversation: original["conversationId"].text,
                                                    body: original["body"].fields, retryOriginal: retry)
    }

    func archiveRejected(_ original: IMJSON) throws {
        try requireCurrent()
        guard original["familyId"].text == family,
            let guardState = try session.store.syncStateIfPresent(namespace: namespace)
        else {
            try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
        }
        try session.store.transitionSelfStateOperation(namespace: namespace, expectedGuard: guardState,
                                                       original: original, state: "archived", reason: original["reason"].text)
    }

    static func displayedConversations(_ values: [IMJSON], filter: Int) -> [IMJSON] {
        return values.filter { value in
            let view = value["personalView"]
            if filter == 3 {
                return view["archived"] == .bool(true)
            }
            if filter == 4 {
                return view["hidden"] == .bool(true)
            }
            guard view["hidden"] != .bool(true), view["archived"] != .bool(true) else {
                return false
            }
            if filter == 1 {
                return view["manualUnread"] == .bool(true)
                    || (!value["unreadCount"].text.isEmpty && value["unreadCount"].text != "0")
            }
            return filter != 2 || value["kind"] == .string("group")
        }.sorted { left, right in
            let a = Int(left["personalView"]["pinnedRank"].text) ?? Int.max
            let b = Int(right["personalView"]["pinnedRank"].text) ?? Int.max
            if a != b {
                return a < b
            }
            if left["updatedAt"].text != right["updatedAt"].text {
                return left["updatedAt"].text > right["updatedAt"].text
            }
            return left["id"].text < right["id"].text
        }
    }

    static func requireObservedMembership(_ read: IMJSON, original: String) throws {
        guard try IMSyncProtocol.counter(.string(original)) > 0,
            read["membershipGeneration"] == .string(original), read["writable"] == .bool(true),
            read["countStatus"] == .string("exact_current_source")
        else {
            try IMSyncProtocol.refuse("QUALIFICATION_CHANGED")
        }
    }

    static func validateResult(_ result: IMJSON, original: IMJSON, namespace: IMNamespace) throws {
        let draft = original["path"].text.hasSuffix("/draft")
        _ = try IMSyncProtocol.id(result["operationId"])
        _ = try IMSyncProtocol.counter(result["result"]["version"])
        guard result["result"]["revisionEpoch"].text == namespace.restoreEpoch,
            case .bool = result["current"],
            (draft ? ["prepared", "applied", "retired"] : ["applied"]).contains(result["status"].text)
        else {
            try IMSyncProtocol.refuse("SYNC_ORIGINAL_RESULT_MISMATCH")
        }
        if draft {
            guard ["empty", "owned", "unavailable"].contains(result["result"]["contentState"].text),
                ["none", "available", "unavailable"].contains(result["result"]["replyState"].text),
                result["result"]["ownedText"] == .null, result["result"]["reply"] == .null
            else {
                try IMSyncProtocol.refuse()
            }
        } else if original["path"] == .string("me/preferences") {
            guard result["result"]["fontPreset"] == original["body"]["fontPreset"] else {
                try IMSyncProtocol.refuse("SYNC_ORIGINAL_RESULT_MISMATCH")
            }
        } else if original["path"].text.hasSuffix("/view") {
            try validateCurrent(result["result"], kind: "view", conversation: original["conversationId"].text, namespace: namespace)
        } else if original["path"].text.hasSuffix("/read") {
            _ = try IMSyncProtocol.counter(result["result"]["readPoint"]["epoch"])
            _ = try IMSyncProtocol.counter(result["result"]["readPoint"]["sequence"])
            let ids = try IMSyncProtocol.array(result["result"]["observedMessageIds"])
            for id in ids {
                _ = try IMSyncProtocol.id(id)
            }
            guard result["result"]["membershipGeneration"] == original["body"]["membershipGeneration"],
                Set(ids.map { $0.text }) == Set(original["body"]["observedMessageIds"].list.map { $0.text }),
                ids.count == Set(ids.map { $0.text }).count, result["result"]["countStatus"] == .string("not_computed")
            else {
                try IMSyncProtocol.refuse("SYNC_ORIGINAL_RESULT_MISMATCH")
            }
        } else {
            try IMSyncProtocol.refuse()
        }
    }

    static func resultQuery(_ original: IMJSON) -> [String: IMJSON] {
        return original["path"].text.hasSuffix("/draft")
            ? ["method": original["method"], "body": original["body"]] : original["body"].fields
    }
}
