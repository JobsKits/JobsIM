//
//  IMSessionTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import XCTest

import JobsIMCore
import JobsIMTransport
@testable import JobsIM

@MainActor
final class IMSessionTests: XCTestCase {
    func testReadOnlyDoesNotQueueOrConsumeDraft() async throws {
        let session = IMSession.shared
        let ns = session.namespace
        let id = UUID().uuidString
        let draft = IMJSON.object(["id": .string(id), "text": .string("Keep my draft")])
        try session.store.save(
            [.object(["id": .string(id), "readOnly": .bool(true)])], collection: "conversations",
            namespace: ns)
        try session.store.save([draft], collection: "drafts", namespace: ns)
        let pending = try session.store.pending(namespace: ns)
        defer {
            try? session.store.remove(id, collection: "conversations", namespace: ns)
            try? session.store.remove(id, collection: "drafts", namespace: ns)
        }
        do {
            try await session.send(
                "Keep my draft", conversation: id, didEnqueue: { XCTFail("Read-only intent committed") })
            XCTFail("Read-only send accepted")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "CONVERSATION_READ_ONLY") }
        XCTAssertEqual(try session.store.pending(namespace: ns), pending)
        XCTAssertTrue(try session.store.list("drafts", namespace: ns).contains(draft))
    }
    func testInputMayClearOnlyAfterOutboxCommit() async throws {
        let session = IMSession.shared
        let originalDemo = session.demo
        session.demo = true
        let ns = session.namespace
        let id = UUID().uuidString
        var committed = false
        defer {
            try? session.store.remove(id, collection: "messages:" + id, namespace: ns)
            session.demo = originalDemo
        }
        try await session.send(
            "Commit before clearing", conversation: id, clientID: id,
            didEnqueue: {
                committed =
                    (try? session.store.pending(namespace: ns))?.contains { $0["clientMsgId"].text == id }
                    == true
            })
        XCTAssertTrue(committed)
        XCTAssertFalse(try session.store.pending(namespace: ns).contains { $0["clientMsgId"].text == id })
        XCTAssertTrue(
            try session.store.list("messages:" + id, namespace: ns).contains { $0["clientMsgId"].text == id })
    }
}
