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
        if session.demo {
            return
        }
        try await session.verifyRestoreEpoch()
        let ns = session.namespace
        let key = kind + ":" + userID
        let pending = try session.store.list("contact-outbox", namespace: ns).first { $0["id"].text == key }
        let requestID = pending?["requestId"].text ?? UUID().uuidString
        let operation = IMJSON.object([
            "id": .string(key), "requestId": .string(requestID), "userId": .string(userID),
            "kind": .string(kind),
        ])
        try session.store.save([operation], collection: "contact-outbox", namespace: ns)
        let path = kind == "reopen" ? "contacts/" + userID + "/reopen" : "contacts/requests"
        var body: [String: IMJSON] = ["requestId": .string(requestID)]
        if kind == "request" { body["userId"] = .string(userID) }
        do {
            _ = try await session.action(path, body: body)
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
