//
//  IMHistoryExport.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMStorage

@MainActor
enum IMHistoryExport {
    static func data(store: IMLocalStore, namespace: IMNamespace) throws -> Data {
        let conversations = try store.list("conversations", namespace: namespace)
        var exported: [IMJSON] = []
        let allowed: Set<String> = [
            "id", "conversationId", "senderId", "type", "text", "createdAt", "updatedAt", "version",
            "deleted", "contentUnavailable", "replyToId", "attachmentId",
        ]
        for conversation in conversations {
            let messages = try store.list("messages:" + conversation["id"].text, namespace: namespace)
                .filter { !["queued", "rejected", "frozen"].contains($0["status"].text) }
                .map { message -> IMJSON in
                    guard case .object(let fields) = message else {
                        return .null
                    }
                    var safe = fields.filter {
                        allowed.contains($0.key)
                    }
                    if message["deleted"] == .bool(true) || message["contentUnavailable"] == .bool(true)
                        || IMForwardService.isForward(message)
                    {
                        safe["text"] = .string("")
                        safe["contentUnavailable"] = .bool(true)
                        safe.removeValue(forKey: "attachmentId")
                        safe.removeValue(forKey: "replyToId")
                    }
                    return .object(safe)
                }
            exported.append(
                .object([
                    "id": conversation["id"], "title": conversation["title"], "messages": .array(messages),
                ]))
        }
        // 普通本地副本导出：不包含凭据、恢复码、Outbox，也不宣称服务端备份或可导入迁移。
        return try JSONEncoder.make().encode(
            IMJSON.object([
                "format": .string("JobsIM-local-history"), "formatVersion": .string("1"),
                "accountId": .string(namespace.account), "connectionProfile": .string(namespace.profile),
                "restoreEpoch": .string(namespace.restoreEpoch), "demo": .bool(namespace.demo),
                "includesMediaBytes": .bool(false), "importSupported": .bool(false),
                "conversations": .array(exported),
            ]))
    }
}
