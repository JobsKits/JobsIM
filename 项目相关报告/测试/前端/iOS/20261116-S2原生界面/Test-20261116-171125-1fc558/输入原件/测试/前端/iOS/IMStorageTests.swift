//
//  IMStorageTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import JobsIMCore
import JobsIMStorage
import XCTest

final class IMStorageTests: XCTestCase {

    func testNamespacesIsolateAccountsInstancesAndDemo() throws {
        let store = try IMLocalStore(path: ":memory:")
        let formal = IMNamespace(profile: "A", account: "jobs", demo: false)
        let sample = IMJSON.object(["id": .string("m1"), "text": .string("private")])
        try store.save([sample], collection: "messages:c1", namespace: formal)
        for other in [
            IMNamespace(profile: "B", account: "jobs", demo: false),
            IMNamespace(profile: "A", account: "other", demo: false),
            IMNamespace(profile: "A", account: "jobs", demo: true),
            IMNamespace(profile: "A", account: "jobs", demo: false, restoreEpoch: "2"),
        ] {
            XCTAssertEqual(try store.list("messages:c1", namespace: other), [])
        }
        XCTAssertEqual(try store.list("messages:c1", namespace: formal), [sample])
    }

    func testOutboxIdentityIsStableAndCommitIsAtomic() throws {
        let store = try IMLocalStore(path: ":memory:")
        let namespace = IMNamespace(profile: "A", account: "jobs", demo: false)
        let message = IMJSON.object([
            "id": .string("client-1"), "clientMsgId": .string("client-1"), "text": .string("hello"),
            "sendAdmission": .string("test-admission"),
        ])
        try store.enqueue(message, conversation: "c1", namespace: namespace)
        try store.enqueue(message, conversation: "c1", namespace: namespace)
        XCTAssertEqual(try store.pending(namespace: namespace).count, 1)
        XCTAssertThrowsError(
            try store.enqueue(
                message.setting("text", .string("changed")), conversation: "c1", namespace: namespace))
        XCTAssertThrowsError(
            try store.acknowledge(
                clientID: "client-1", message: .null, conversation: "c1", namespace: namespace))
        XCTAssertEqual(try store.pending(namespace: namespace).count, 1)
        let accepted = message.setting("id", .string("server-42"))
        try store.acknowledge(
            clientID: "client-1", message: accepted, conversation: "c1", namespace: namespace)
        XCTAssertEqual(try store.pending(namespace: namespace), [])
        XCTAssertEqual(try store.list("messages:c1", namespace: namespace), [accepted])
    }

    func testOutboxSurvivesReopeningStore() throws {
        let path = FileManager.default.temporaryDirectory.appendingPathComponent(
            UUID().uuidString + ".sqlite"
        ).path
        let namespace = IMNamespace(profile: "A", account: "jobs", demo: false)
        let message = IMJSON.object([
            "id": .string("kept"), "clientMsgId": .string("kept"), "text": .string("not yet confirmed"),
            "sendAdmission": .string("test-admission"),
        ])
        do {
            let store = try IMLocalStore(path: path)
            try store.enqueue(message, conversation: "c", namespace: namespace)
        }
        let reopened = try IMLocalStore(path: path)
        XCTAssertEqual(try reopened.pending(namespace: namespace), [message])
        XCTAssertEqual(try reopened.list("messages:c", namespace: namespace), [message])
    }

    func testCompleteEmptyListReplacesStaleProjection() throws {
        let store = try IMLocalStore(path: ":memory:")
        let namespace = IMNamespace(profile: "A", account: "jobs", demo: false)
        try store.save([.object(["id": .string("old")])], collection: "contacts", namespace: namespace)
        try store.replace([], collection: "contacts", namespace: namespace)
        XCTAssertEqual(try store.list("contacts", namespace: namespace), [])
    }

    func testStringIdentifiersKeepLargeValuesExact() throws {
        let value = try JSONDecoder().decode(
            IMJSON.self,
            from: Data("{\"id\":\"18446744073709551615\",\"sequence\":\"9007199254740993\"}".utf8))
        XCTAssertEqual(value["id"].text, "18446744073709551615")
        XCTAssertEqual(value["sequence"].text, "9007199254740993")
        XCTAssertEqual(try JSONDecoder().decode(IMJSON.self, from: JSONEncoder().encode(value)), value)
    }

    func testMessageCursorAndProjectionCommitTogetherWithoutVersionRegression() throws {
        let store = try IMLocalStore(path: ":memory:")
        let namespace = IMNamespace(profile: "A", account: "jobs", demo: false)
        let latest = IMJSON.object([
            "id": .string("server"), "text": .string("edited"), "version": .string("2"),
        ])
        try store.applyMessagePage([latest], conversation: "c", cursor: "1:12", namespace: namespace)
        XCTAssertThrowsError(
            try store.applyMessagePage(
                [latest, .null], conversation: "c", cursor: "1:13", namespace: namespace))
        XCTAssertEqual(try store.list("cursors", namespace: namespace).first?["cursor"].text, "1:12")
        let old = latest.setting("version", .string("1")).setting("text", .string("old"))
        try store.applyMessagePage([old], conversation: "c", cursor: "1:14", namespace: namespace)
        XCTAssertEqual(try store.list("messages:c", namespace: namespace), [latest])
        let deleted = latest.setting("version", .string("3")).setting("deleted", .bool(true)).setting(
            "text", .string(""))
        try store.save([deleted], collection: "messages:c", namespace: namespace)
        try store.save([latest], collection: "messages:c", namespace: namespace)
        XCTAssertEqual(try store.list("messages:c", namespace: namespace), [deleted])
    }

    func testEpochCursorIsExactAndRejectsWrongEpochOrRegression() throws {
        let baseline = try XCTUnwrap(IMCursor(raw: "0", expectedEpoch: "1"))
        let first = try XCTUnwrap(IMCursor(raw: "1:9007199254740993", expectedEpoch: "1"))
        let latest = try XCTUnwrap(IMCursor(raw: "1:18446744073709551615", expectedEpoch: "1"))
        XCTAssertTrue(first.isAtLeast(baseline))
        XCTAssertTrue(latest.isAtLeast(first))
        XCTAssertFalse(first.isAtLeast(latest))
        XCTAssertNil(IMCursor(raw: "2:9", expectedEpoch: "1"))
        XCTAssertNil(IMCursor(raw: "1:01", expectedEpoch: "1"))
        XCTAssertNil(IMCursor(raw: "9", expectedEpoch: "1"))
    }

    func testIncomingUnreadIsAtomicDeduplicatedAndCursorCannotRegress() throws {
        let store = try IMLocalStore(path: ":memory:")
        let ns = IMNamespace(profile: "p", account: "self", demo: false)
        let message = IMJSON.object([
            "id": .string("new"), "senderId": .string("peer"), "conversationId": .string("c"),
            "version": .string("1"),
        ])
        XCTAssertEqual(
            try store.applyMessagePage(
                [message], conversation: "c", cursor: "1:3", namespace: ns, readerID: "self"
            ).count, 1)
        XCTAssertEqual(
            try store.applyMessagePage(
                [message], conversation: "c", cursor: "1:3", namespace: ns, readerID: "self"
            ).count, 0)
        try store.applyMessagePage([], conversation: "c", cursor: "1:2", namespace: ns, readerID: "self")
        XCTAssertEqual(try store.list("cursors", namespace: ns).first?["cursor"].text, "1:3")
        XCTAssertEqual(try store.list("unread:c", namespace: ns).count, 1)
        try store.applyMessagePage(
            [], conversation: "c", cursor: "1:3", namespace: ns, readerID: "self", isVisible: true)
        XCTAssertTrue(try store.list("unread:c", namespace: ns).isEmpty)
    }

    func testUnreadAndDraftSurviveReopeningWithoutReadReceipt() throws {
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(
            UUID().uuidString + ".sqlite")
        let ns = IMNamespace(profile: "restart", account: "reader", demo: false)
        do {
            let store = try IMLocalStore(path: file.path)
            try store.save(
                [.object(["id": .string("c"), "text": .string("Persistent draft")])], collection: "drafts",
                namespace: ns)
            _ = try store.applyMessagePage(
                [
                    .object([
                        "id": .string("incoming"), "senderId": .string("peer"),
                        "text": .string("Unread ordinary message"), "version": .string("1"),
                    ])
                ], conversation: "c", cursor: "1:1", namespace: ns, readerID: "reader")
        }
        let reopened = try IMLocalStore(path: file.path)
        XCTAssertEqual(try reopened.list("unread:c", namespace: ns).count, 1)
        XCTAssertEqual(try reopened.list("drafts", namespace: ns).first?["text"].text, "Persistent draft")
        XCTAssertEqual(try reopened.list("cursors", namespace: ns).first?["cursor"].text, "1:1")
        try reopened.replace([], collection: "unread:c", namespace: ns)
        XCTAssertTrue(try reopened.list("unread:c", namespace: ns).isEmpty)
        XCTAssertFalse(try reopened.list("messages:c", namespace: ns).isEmpty)
        XCTAssertTrue(try reopened.pending(namespace: ns).isEmpty)
    }

}
