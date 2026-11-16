//
//  IMMediaCache.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
enum IMMediaCache {
    static func directory(_ namespace: IMNamespace) throws -> URL {
        let folder = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Media", isDirectory: true).appendingPathComponent(
                namespace.key, isDirectory: true)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        return folder
    }
    static func purge(messageID: String, namespace: IMNamespace) {
        guard !messageID.isEmpty, let folder = try? directory(namespace),
            let files = try? FileManager.default.contentsOfDirectory(
                at: folder, includingPropertiesForKeys: nil)
        else {
            return
        }
        for file in files where file.lastPathComponent.hasPrefix(messageID + "-") {
            try? FileManager.default.removeItem(at: file)
        }
    }
    static func purge(messageID: String, namespaceKeys: [String]) {
        let root = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Media", isDirectory: true)
        for key in namespaceKeys {
            let folder = root.appendingPathComponent(key, isDirectory: true)
            guard let files = try? FileManager.default.contentsOfDirectory(at: folder, includingPropertiesForKeys: nil) else {
                continue
            }
            for file in files where file.lastPathComponent.hasPrefix(messageID + "-") {
                try? FileManager.default.removeItem(at: file)
            }
        }
    }
    static func purgeForward(namespaceKeys: [String]) {
        let root = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Media", isDirectory: true)
        for key in namespaceKeys {
            let folder = root.appendingPathComponent(key, isDirectory: true)
            guard let files = try? FileManager.default.contentsOfDirectory(at: folder, includingPropertiesForKeys: nil) else {
                continue
            }
            for file in files where file.lastPathComponent.contains("-forward-") {
                try? FileManager.default.removeItem(at: file)
            }
        }
    }
    static func permitsFallback(_ error: IMAPIError, message: IMJSON) -> Bool {
        return !IMForwardService.isForward(message) && error.status == 0 && message["deleted"] != .bool(true)
            && message["contentUnavailable"] != .bool(true)
    }
    static func file(_ message: IMJSON, namespace: IMNamespace) async throws -> URL {
        let session = IMSession.shared
        if IMForwardService.isForward(message) {
            return try await forwardFile(message, namespace: namespace)
        }
        let id = message["attachmentId"].text
        let messageID = message["id"].text
        guard session.namespace == namespace, message["deleted"] != .bool(true),
            message["contentUnavailable"] != .bool(true),
            messageID.range(of: "^[A-Za-z0-9_-]+$", options: .regularExpression) != nil,
            id.range(of: "^[A-Za-z0-9_-]+$", options: .regularExpression) != nil
        else {
            throw IMAPIError(
                status: 403, code: "MEDIA_UNAVAILABLE", message: IMStrings.text("contentUnavailable"))
        }
        let folder = try directory(namespace)
        let file = folder.appendingPathComponent(
            messageID + "-" + id + "-" + URL(fileURLWithPath: message["text"].text).lastPathComponent)
        guard !session.demo, session.api != nil, session.token != nil else {
            throw IMSession.SessionError.noInstance
        }
        let pending = folder.appendingPathComponent(UUID().uuidString + ".download")
        defer {
            try? FileManager.default.removeItem(at: pending)
        }
        do {
            _ = try await session.verifyRestoreEpoch()
            guard session.namespace == namespace else { throw IMSession.SessionError.noInstance }
            _ = try await session.authorized { api, token in
                return try await api.download(id, to: pending, token: token)
            }
            guard session.namespace == namespace else {
                throw IMSession.SessionError.noInstance
            }
            let latest =
                (try session.store.list("messages:" + message["conversationId"].text, namespace: namespace))
                .first { $0["id"] == message["id"] }
            if latest?["deleted"] == .bool(true) || latest?["contentUnavailable"] == .bool(true) {
                try? FileManager.default.removeItem(at: file)
                throw IMAPIError(
                    status: 403, code: "MEDIA_UNAVAILABLE", message: IMStrings.text("contentUnavailable"))
            }
            if FileManager.default.fileExists(atPath: file.path) {
                _ = try FileManager.default.replaceItemAt(file, withItemAt: pending)
            } else {
                try FileManager.default.moveItem(at: pending, to: file)
            }
            try FileManager.default.setAttributes(
                [.protectionKey: FileProtectionType.completeUntilFirstUserAuthentication],
                ofItemAtPath: file.path)
            return file
        } catch let error as IMAPIError {
            // 只对传输失败读已鉴权的普通副本；服务端401/403/404不会伪装为离线成功。
            guard session.namespace == namespace, session.token != nil,
                permitsFallback(error, message: message),
                FileManager.default.fileExists(atPath: file.path)
            else {
                throw error
            }
            let latest =
                (try session.store.list("messages:" + message["conversationId"].text, namespace: namespace))
                .first { $0["id"] == message["id"] }
            guard latest?["attachmentId"].text == id, latest?["deleted"] != .bool(true),
                latest?["contentUnavailable"] != .bool(true)
            else {
                throw error
            }
            return file
        }
    }

    private static func forwardFile(_ message: IMJSON, namespace: IMNamespace) async throws -> URL {
        let session = IMSession.shared
        let service = IMForwardService()
        let latest = try await service.current(message)
        let id = latest["id"].text
        guard session.namespace == namespace, latest["deleted"] != .bool(true),
            latest["contentUnavailable"] != .bool(true), latest["forwardUnavailable"] != .bool(true),
            latest["attachmentURL"].text == "/api/messages/" + id + "/attachment",
            id.range(of: "^[A-Za-z0-9_-]+$", options: .regularExpression) != nil,
            session.api != nil, session.token != nil
        else {
            purge(messageID: message["id"].text, namespace: namespace)
            throw IMAPIError(
                status: 403, code: "MEDIA_UNAVAILABLE", message: IMStrings.text("forwardUnavailable"))
        }
        // 转发附件不使用源附件ID；每次重新鉴权，失联也不回读旧副本。
        purge(messageID: id, namespace: namespace)
        let folder = try directory(namespace)
        let pending = folder.appendingPathComponent(UUID().uuidString + ".download")
        defer { try? FileManager.default.removeItem(at: pending) }
        do {
            _ = try await session.authorized { api, token in
                return try await api.download(id + "/attachment", to: pending, token: token, collection: "messages")
            }
            let after = try await service.current(latest)
            guard session.namespace == namespace, after["contentVersion"] == latest["contentVersion"],
                after["deleted"] != .bool(true), after["contentUnavailable"] != .bool(true),
                after["forwardUnavailable"] != .bool(true)
            else { throw IMSession.SessionError.noInstance }
            let name = URL(fileURLWithPath: latest["text"].text).lastPathComponent
            let file = folder.appendingPathComponent(
                id + "-forward-" + UUID().uuidString + "-" + (name.isEmpty ? "attachment" : name))
            try FileManager.default.moveItem(at: pending, to: file)
            try FileManager.default.setAttributes(
                [.protectionKey: FileProtectionType.completeUntilFirstUserAuthentication],
                ofItemAtPath: file.path)
            return file
        } catch {
            purge(messageID: id, namespace: namespace)
            throw error
        }
    }
}
