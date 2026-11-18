//
//  IMContactService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
final class IMContactService {
    private let session = IMSession.shared
    func perform(_ kind: String, userID: String) async throws {
        guard ["request", "remove", "block", "unblock", "reopen"].contains(kind) else {
            try IMSyncProtocol.refuse("CONTACT_ACTION_UNSUPPORTED")
            return
        }
        if session.demo {
            let ns = session.namespace
            if kind == "remove" {
                try session.store.remove(userID, collection: "contacts", namespace: ns)
            } else if kind == "block" || kind == "unblock" {
                if let person = try session.store.list("contacts", namespace: ns).first(where: {
                    $0["id"].text == userID
                }) {
                    try session.store.save(
                        [person.setting("blockedByMe", .bool(kind == "block"))], collection: "contacts",
                        namespace: ns)
                }
            } else if kind == "request" {
                try session.store.save(
                    [
                        .object([
                            "id": .string(UUID().uuidString), "requesterId": session.user["id"],
                            "recipientId": .string(userID), "requester": session.user,
                            "state": .string("pending"),
                        ])
                    ], collection: "contact-requests", namespace: ns)
            }
            return
        }
        try await session.verifyRestoreEpoch()
        let ns = session.namespace
        let key = kind + ":" + userID
        let pending = try session.store.list("contact-outbox", namespace: ns).first {
            $0["id"].text == key
        }
        let requestID = pending?["requestId"].text ?? UUID().uuidString
        let operation = IMJSON.object([
            "id": .string(key), "requestId": .string(requestID), "userId": .string(userID),
            "kind": .string(kind),
        ])
        try session.store.save([operation], collection: "contact-outbox", namespace: ns)
        let path: String
        let method: String
        switch kind {
        /// 删除联系人关系，保留合法会话历史。
        case "remove":
            path = "contacts/" + userID
            method = "DELETE"
        /// 拉黑双方消息，不删除联系人关系。
        case "block":
            path = "blocks/" + userID
            method = "PUT"
        /// 解除拉黑，重新采用服务端关系权限。
        case "unblock":
            path = "blocks/" + userID
            method = "DELETE"
        /// 兼容旧耐久请求；新界面不再暴露此开关。
        case "reopen":
            path = "contacts/" + userID + "/reopen"
            method = "POST"
        /// 创建新的好友申请。
        default:
            path = "contacts/requests"
            method = "POST"
        }
        var body: [String: IMJSON] = ["requestId": .string(requestID)]
        if kind == "request" {
            body["userId"] = .string(userID)
        }
        do {
            _ = try await session.action(path, method: method, body: body)
            guard session.namespace == ns else {
                return
            }
            try session.store.remove(key, collection: "contact-outbox", namespace: ns)
        } catch {
            if let failure = error as? IMAPIError, [400, 403, 409].contains(failure.status) {
                try session.store.save(
                    [operation.setting("failureCode", .string(failure.code))], collection: "contact-history",
                    namespace: ns)
                try session.store.remove(key, collection: "contact-outbox", namespace: ns)
            }
            throw error
        }
    }
}
