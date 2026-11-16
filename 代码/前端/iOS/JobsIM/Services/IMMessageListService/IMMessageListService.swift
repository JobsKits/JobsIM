//
//  IMMessageListService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore

@MainActor
final class IMMessageListService {

    private let session: IMSession

    init(session: IMSession = .shared) {
        self.session = session
    }

    func page(favorites: Bool, query: String = "", cursor: String = "0", limit: Int = 50) async throws
        -> IMMessagePage
    {
        guard (1...100).contains(limit) else {
            throw IMSession.SessionError.invalidInstance
        }
        let namespace = session.namespace
        if session.demo {
            let conversations = try session.store.list("conversations", namespace: namespace)
            var values: [IMJSON] = []
            for conversation in conversations {
                values += try session.store.list("messages:" + conversation["id"].text, namespace: namespace)
            }
            values = values.filter { item in
                let matches =
                    favorites
                    ? item["favorite"] == .bool(true)
                    : item["text"].text.localizedCaseInsensitiveContains(query)
                return item["deleted"] != .bool(true) && matches
            }
            values.sort {
                $0["createdAt"].text > $1["createdAt"].text
            }
            let start = cursor.hasPrefix("demo:") ? Int(cursor.dropFirst(5)) ?? 0 : 0
            guard start >= 0 else {
                throw IMSession.SessionError.invalidInstance
            }
            let end = min(values.count, start + limit)
            let items = start < values.count ? Array(values[start..<end]) : []
            return try IMMessagePage(
                .object([
                    "items": .array(items), "cursor": .string("demo:" + String(end)),
                    "hasMore": .bool(end < values.count),
                ]))
        }
        var path = favorites ? "favorites?" : "search?q=" + Self.escape(query) + "&"
        path += "cursor=" + Self.escape(cursor) + "&limit=" + String(limit)
        let response = try await session.action(path, method: "GET")
        guard session.namespace == namespace else {
            throw IMSession.SessionError.noInstance
        }
        return try IMMessagePage(response)
    }

    func conversation(_ id: String) async throws -> IMJSON {
        let namespace = session.namespace
        if session.demo {
            guard
                let value = try session.store.list("conversations", namespace: namespace).first(
                    where: { $0["id"].text == id })
            else {
                throw IMSession.SessionError.invalidInstance
            }
            return value
        }
        var value = try await session.action("conversations/" + Self.escape(id), method: "GET")
        guard session.namespace == namespace, value["id"].text == id else {
            throw IMSession.SessionError.noInstance
        }
        if value["kind"].text == "direct", value["title"].text.isEmpty {
            let members = try await session.action(
                "conversations/" + Self.escape(id) + "/members", method: "GET")
            guard session.namespace == namespace else {
                throw IMSession.SessionError.noInstance
            }
            if let peer = members["items"].list.first(where: { $0["id"] != session.user["id"] }) {
                value = value.setting("title", .string(peer.firstText("displayName", "username", "id")))
            }
        }
        try session.store.save([value], collection: "conversations", namespace: namespace)
        return value
    }

    func locate(_ source: IMJSON) async throws -> (conversation: IMJSON, message: IMJSON) {
        let namespace = session.namespace
        let conversationID = source["conversationId"].text
        let messageID = source["id"].text
        guard !conversationID.isEmpty, !messageID.isEmpty else {
            throw IMSession.SessionError.invalidInstance
        }
        let metadata: IMJSON
        do {
            metadata = try await conversation(conversationID)
        } catch {
            if IMForwardService.isForward(source), session.namespace == namespace {
                try? session.store.redactForward(
                    messageID: messageID, conversation: conversationID, namespace: namespace)
                IMMediaCache.purge(messageID: messageID, namespace: namespace)
            }
            throw error
        }
        let message: IMJSON
        if session.demo {
            guard
                let cached = try session.store.list("messages:" + conversationID, namespace: namespace)
                    .first(
                        where: { $0["id"].text == messageID })
            else {
                throw IMSession.SessionError.invalidInstance
            }
            message = cached
        } else {
            message =
                IMForwardService.isForward(source)
                ? try await IMForwardService(session: session).current(source)
                : try await session.action("messages/" + Self.escape(messageID), method: "GET")
        }
        guard session.namespace == namespace, message["id"].text == messageID,
            message["conversationId"].text == conversationID
        else {
            throw IMSession.SessionError.noInstance
        }
        try session.store.save(
            [message], collection: "messages:" + conversationID, namespace: namespace)
        if message["deleted"] == .bool(true) || message["contentUnavailable"] == .bool(true) {
            IMMediaCache.purge(messageID: messageID, namespace: namespace)
        }
        return (metadata, message)
    }

    private static func escape(_ value: String) -> String {
        let allowed = CharacterSet.urlQueryAllowed.subtracting(CharacterSet(charactersIn: "&+=?#/"))
        return value.addingPercentEncoding(withAllowedCharacters: allowed) ?? ""
    }
}
