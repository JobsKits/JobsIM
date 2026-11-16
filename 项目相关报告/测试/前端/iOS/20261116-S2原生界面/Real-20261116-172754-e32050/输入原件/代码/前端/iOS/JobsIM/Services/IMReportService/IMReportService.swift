//
//  IMReportService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation

import JobsIMCore

@MainActor
final class IMReportService {
    private let session = IMSession.shared
    func submit(type: String, id: String, reason: String) async throws {
        let ns = session.namespace
        if session.demo {
            return
        }
        let existing = try session.store.list("report-outbox", namespace: ns).first {
            $0["targetType"].text == type && $0["targetId"].text == id && $0["reason"].text == reason
        }
        let requestID = existing?["id"].text ?? UUID().uuidString
        let operation = IMJSON.object([
            "id": .string(requestID), "requestId": .string(requestID), "targetType": .string(type),
            "targetId": .string(id), "reason": .string(reason),
        ])
        try session.store.save([operation], collection: "report-outbox", namespace: ns)
        try await retry(operation)
    }
    func retry(_ operation: IMJSON) async throws {
        try await session.verifyRestoreEpoch()
        let ns = session.namespace
        var body = operation.fields
        body = body.filter { ["requestId", "targetType", "targetId", "reason"].contains($0.key) }
        let result = try await session.action("reports", body: body)
        guard ns == session.namespace else {
            return
        }
        try session.store.save([result], collection: "reports", namespace: ns)
        try session.store.remove(operation["id"].text, collection: "report-outbox", namespace: ns)
    }
}
