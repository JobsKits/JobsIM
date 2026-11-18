//
//  IMReceiptStorageTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import CryptoKit
import Darwin
import Foundation
import JobsIMCore
import JobsIMStorage
import SQLite3
import XCTest

@MainActor
final class IMReceiptStorageTests: XCTestCase {

    private func execute(_ sql: String, path: String) throws {
        var database: OpaquePointer?
        guard sqlite3_open_v2(path, &database, SQLITE_OPEN_READWRITE | SQLITE_OPEN_FULLMUTEX, nil) == SQLITE_OK, let database else {
            throw NSError(domain: "ReceiptFixtureSQLite", code: 1)
        }
        defer {
            sqlite3_close(database)
        }
        guard sqlite3_exec(database, sql, nil, nil, nil) == SQLITE_OK else {
            throw NSError(domain: "ReceiptFixtureSQLite", code: Int(sqlite3_errcode(database)))
        }
    }

    private final class Fixture {
        let directory: URL
        let path: String
        let store: IMLocalStore
        let namespace: IMNamespace
        let conversation = "11111111-1111-4111-8111-111111111111"
        let messageID = "22222222-2222-4222-8222-222222222222"
        let qualification = "33333333-3333-4333-8333-333333333333"
        let family = "44444444-4444-4444-8444-444444444444"
        var state: IMJSON = .null
        var message: IMJSON = .null
        var content: IMJSON = .null
        var envelope = ""

        init() throws {
            directory = FileManager.default.temporaryDirectory.appendingPathComponent("JobsIMReceiptTests-" + UUID().uuidString)
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: false)
            path = directory.appendingPathComponent("client.sqlite").path
            store = try IMLocalStore(path: path)
            namespace = IMNamespace(profile: "receipt-test-" + UUID().uuidString, account: "55555555-5555-4555-8555-555555555555",
                demo: false, restoreEpoch: "1", deploymentID: "66666666-6666-4666-8666-666666666666",
                sourceContextID: "personal", deviceID: "77777777-7777-4777-8777-777777777777")
            let initial = try store.syncState(namespace: namespace)
            state = try store.syncUpdate(namespace: namespace, expected: initial,
                next: initial.setting("restriction", .string("")).setting("streamId", .string(UUID().uuidString.lowercased()))
                    .setting("permissionGeneration", .string(String(repeating: "a", count: 64))))
            try store.save([.object(["id": .string(conversation), "kind": .string("direct"), "state": .string("active"),
                "membershipGeneration": .string("7")])], collection: "conversations", namespace: namespace)
            message = .object(["id": .string(messageID), "conversationId": .string(conversation), "senderId": .string(namespace.account),
                "type": .string("text"), "text": .string("原始耐久字节 <&> བོད་"), "createdAt": .string("2026-11-17T00:00:00.123456789Z"),
                "contentVersion": .string("9007199254740993"), "version": .string("1"), "deleted": .bool(false)])
            let raw = try JSONEncoder.make().encode(message)
            envelope = raw.base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
            content = intent(kind: "content", digest: Self.hash(raw))
        }

        deinit {
            try? FileManager.default.removeItem(at: directory)
            let folder = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
                .appendingPathComponent("ReceiptObjects").appendingPathComponent(namespace.key)
            try? FileManager.default.removeItem(at: folder)
        }

        static func hash(_ data: Data) -> String {
            return SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
        }

        func intent(kind: String, digest: String) -> IMJSON {
            let id = UUID().uuidString.lowercased()
            let body: IMJSON = .object(["requestId": .string(id), "kind": .string("message_content"), "storeGeneration": state["storeGeneration"],
                "revisionEpoch": .string("1"), "qualificationId": .string(qualification), "challenge": .string("rc1.storage-fixture-only"), "bodyDigest": .string(digest)])
            return .object(["id": .string(id), "kind": .string(kind), "familyId": .string(family), "storeGeneration": state["storeGeneration"],
                "restoreEpoch": .string("1"), "schemaVersion": .string("1"), "createdAt": .string("2026-11-17T00:00:00.123456789Z"),
                "attempts": .number(0), "state": .string("prepared"), "originalBody": body, "messageId": .string(messageID),
                "conversationId": .string(conversation), "contentVersion": message["contentVersion"], "membershipGeneration": .string("7"),
                "qualificationId": .string(qualification), "permissionGeneration": state["permissionGeneration"], "deliveryGeneration": .string("3"),
                "manifestDigest": .string(String(repeating: "b", count: 64)), "bodyDigest": .string(digest), "result": .null])
        }

        func stage() throws {
            try store.commitReceiptContent(namespace: namespace, expected: state, intent: content, bodyEnvelope: envelope)
        }

        func object() throws -> (IMJSON, IMJSON, Data) {
            let objectID = "88888888-8888-4888-8888-888888888888"
            message = message.setting("type", .string("file")).setting("attachmentId", .string(objectID))
            let raw = try JSONEncoder.make().encode(message)
            envelope = raw.base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
            content = intent(kind: "content", digest: Self.hash(raw))
            try stage()
            let completed = content.setting("state", .string("completed")).setting("result", .object(["fixtureOnly": .bool(true)]))
            try store.compareReceiptIntent(namespace: namespace, expected: state, previous: content, next: completed)
            let bytes = Data("所有原件\0bytes".utf8)
            let object = "88888888-8888-4888-8888-888888888888"
            var intent = self.intent(kind: "object", digest: content["bodyDigest"].text)
            let body: IMJSON = .object(["requestId": intent["id"], "kind": .string("object_received"), "storeGeneration": state["storeGeneration"],
                "revisionEpoch": .string("1"), "qualificationId": .string(qualification), "challenge": .string("ro1.storage-fixture-only"),
                "parentReceiptId": content["id"], "objectId": .string(object), "sizeBytes": .string(String(bytes.count)), "objectDigest": .string(Self.hash(bytes))])
            intent = intent.setting("originalBody", body)
            let descriptor: IMJSON = .object(["objectId": .string(object), "messageId": .string(messageID), "sizeBytes": .string(String(bytes.count)),
                "sha256": .string(Self.hash(bytes)), "contentType": .string("application/octet-stream")])
            return (intent, descriptor, bytes)
        }
    }

    func testContentOriginalBytesAndIntentCommitWithoutCursorOrOutboxChanges() throws {
        let f = try Fixture()
        try f.stage()
        let reopened = try IMLocalStore(path: f.path)
        XCTAssertEqual(try reopened.list("receipt-bodies", namespace: f.namespace).first?["bodyEnvelope"].text, f.envelope)
        XCTAssertEqual(try reopened.list("receipt-intents", namespace: f.namespace).first, f.content)
        XCTAssertEqual(try reopened.syncStateIfPresent(namespace: f.namespace), f.state)
        XCTAssertTrue(try reopened.allOutbox(namespace: f.namespace).isEmpty)
    }

    func testGuardMemberAndDigestFailuresPublishNoPartialIntent() throws {
        let f = try Fixture()
        XCTAssertThrowsError(try f.store.commitReceiptContent(namespace: f.namespace, expected: f.state.setting("guardGeneration", .string("99")), intent: f.content, bodyEnvelope: f.envelope))
        XCTAssertThrowsError(try f.store.commitReceiptContent(namespace: f.namespace, expected: f.state, intent: f.content, bodyEnvelope: "e30"))
        try f.store.save([.object(["id": .string(f.conversation), "state": .string("active"), "membershipGeneration": .string("8")])], collection: "conversations", namespace: f.namespace)
        XCTAssertThrowsError(try f.stage())
        XCTAssertTrue(try f.store.list("receipt-intents", namespace: f.namespace).isEmpty)
        XCTAssertTrue(try f.store.list("receipt-bodies", namespace: f.namespace).isEmpty)
    }

    func testCompleteOriginalFsyncDescriptorAndNonceSurviveReopen() throws {
        let f = try Fixture()
        let (intent, descriptor, bytes) = try f.object()
        try f.store.commitReceiptObject(namespace: f.namespace, expected: f.state, intent: intent, descriptor: descriptor, bytes: bytes)
        let reopened = try IMLocalStore(path: f.path)
        XCTAssertEqual(try reopened.receiptObject(namespace: f.namespace, requestId: intent["id"].text), bytes)
        XCTAssertEqual(try reopened.list("receipt-objects", namespace: f.namespace).first?["sha256"], descriptor["sha256"])
        XCTAssertTrue(try reopened.list("receipt-file-preparations", namespace: f.namespace).isEmpty)
        XCTAssertThrowsError(try reopened.commitReceiptObject(namespace: f.namespace, expected: f.state, intent: intent, descriptor: descriptor, bytes: Data("partial".utf8)))
    }

    func testGlobalErasesRawBodyAndOriginalButPreservesFrozenOriginalQueryMetadata() throws {
        let f = try Fixture()
        let (intent, descriptor, bytes) = try f.object()
        try f.store.commitReceiptObject(namespace: f.namespace, expected: f.state, intent: intent, descriptor: descriptor, bytes: bytes)
        let ref: IMJSON = .object(["owner": .string("message"), "kind": .string("message"), "id": .string(f.messageID),
            "conversationId": .string(f.conversation), "revision": .string("2"), "revisionEpoch": .string("1")])
        let global: IMJSON = .object(["ref": ref, "state": .string("global"), "currentRevision": .string("2"),
            "currentRevisionEpoch": .string("1"), "object": f.message.setting("deleted", .bool(true)).setting("deleteScope", .string("global"))])
        try f.store.syncCommit(namespace: f.namespace, expected: f.state, next: f.state, projections: [global], events: [], proofs: [], replaceCloud: false)
        XCTAssertTrue(try f.store.list("receipt-bodies", namespace: f.namespace).isEmpty)
        XCTAssertTrue(try f.store.list("receipt-objects", namespace: f.namespace).isEmpty)
        XCTAssertThrowsError(try f.store.receiptObject(namespace: f.namespace, requestId: intent["id"].text))
        let original = try XCTUnwrap(f.store.list("receipt-intents", namespace: f.namespace).first { $0["id"] == intent["id"] })
        XCTAssertEqual(original["state"].text, "frozen")
        XCTAssertEqual(original["originalBody"], intent["originalBody"])
        XCTAssertThrowsError(try f.store.commitReceiptContent(namespace: f.namespace, expected: f.state, intent: f.content, bodyEnvelope: f.envelope))
    }

    func testReceiptIntentCannotChangeOriginalTupleOrBorrowSuccessorFamily() throws {
        let f = try Fixture()
        try f.stage()
        let retry = f.content.setting("attempts", .number(1)).setting("state", .string("unknown"))
        XCTAssertThrowsError(try f.store.compareReceiptIntent(namespace: f.namespace, expected: f.state, previous: f.content,
            next: retry.setting("familyId", .string(UUID().uuidString.lowercased()))))
        XCTAssertThrowsError(try f.store.compareReceiptIntent(namespace: f.namespace, expected: f.state, previous: f.content,
            next: retry.setting("deliveryGeneration", .string("4"))))
        try f.store.compareReceiptIntent(namespace: f.namespace, expected: f.state, previous: f.content, next: retry)
        XCTAssertEqual(try f.store.list("receipt-intents", namespace: f.namespace).first?["originalBody"], f.content["originalBody"])
    }

    func testLivePhysicalStoreProbeProducesBoundEvidenceWithoutQualification() throws {
        let f = try Fixture()
        let report = try f.store.verifyReceiptDurability(namespace: f.namespace, expected: f.state, buildDigest: String(repeating: "c", count: 64), includeObjects: true)
        XCTAssertEqual(report["storeGeneration"], f.state["storeGeneration"])
        XCTAssertEqual(report["storageContract"].text, "native_sqlite_full_file_fsync_v1")
        XCTAssertEqual(report["qualifiedAck"], .bool(false))
        XCTAssertEqual(report["checks"]["independentConnectionRead"], .bool(true))
        XCTAssertEqual(report["verificationDigest"].text.count, 64)
        XCTAssertTrue(try f.store.list("receipt-storage-probes", namespace: f.namespace).isEmpty)
        XCTAssertTrue(try f.store.list("receipt-file-preparations", namespace: f.namespace).isEmpty)
    }

    func testGlobalSQLFailureLeavesCompleteOriginalAndReceiptRowsRecoverable() throws {
        let f = try Fixture()
        let (intent, descriptor, bytes) = try f.object()
        try f.store.commitReceiptObject(namespace: f.namespace, expected: f.state, intent: intent, descriptor: descriptor, bytes: bytes)
        let next = try XCTUnwrap(Int(f.state["guardGeneration"].text)) + 1
        try execute("CREATE TRIGGER jobsim_receipt_fail_final BEFORE UPDATE ON sync_state WHEN json_extract(CAST(NEW.body AS TEXT),'$.guardGeneration')='\(next)' BEGIN SELECT RAISE(ABORT,'receipt fixture final SQL failure'); END", path: f.path)
        let ref: IMJSON = .object(["owner": .string("message"), "kind": .string("message"), "id": .string(f.messageID),
            "conversationId": .string(f.conversation), "revision": .string("2"), "revisionEpoch": .string("1")])
        let global: IMJSON = .object(["ref": ref, "state": .string("global"), "currentRevision": .string("2"),
            "currentRevisionEpoch": .string("1"), "object": f.message.setting("deleted", .bool(true)).setting("deleteScope", .string("global"))])
        XCTAssertThrowsError(try f.store.syncCommit(namespace: f.namespace, expected: f.state, next: f.state, projections: [global], events: [], proofs: [], replaceCloud: false))
        XCTAssertEqual(try f.store.receiptObject(namespace: f.namespace, requestId: intent["id"].text), bytes)
        XCTAssertEqual(try f.store.list("receipt-bodies", namespace: f.namespace).first?["bodyEnvelope"].text, f.envelope)
        XCTAssertFalse(try f.store.syncGlobalIDs(namespace: f.namespace).contains(f.messageID))
        try execute("DROP TRIGGER jobsim_receipt_fail_final", path: f.path)
        try f.store.syncCommit(namespace: f.namespace, expected: f.state, next: f.state, projections: [global], events: [], proofs: [], replaceCloud: false)
        XCTAssertThrowsError(try f.store.receiptObject(namespace: f.namespace, requestId: intent["id"].text))
    }

    func testQueuedCleanupCannotUnlinkSameNameSuccessorInode() throws {
        let f = try Fixture()
        let (intent, descriptor, bytes) = try f.object()
        try f.store.commitReceiptObject(namespace: f.namespace, expected: f.state, intent: intent, descriptor: descriptor, bytes: bytes)
        let folder = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("ReceiptObjects").appendingPathComponent(f.namespace.key)
        let file = folder.appendingPathComponent(f.messageID + "-" + intent["id"].text + ".object")
        var original = stat()
        XCTAssertEqual(lstat(file.path, &original), 0)
        try execute("INSERT INTO receipt_file_cleanup VALUES('\(f.namespace.key)','\(intent["id"].text)','\(f.messageID)',\(original.st_dev),\(original.st_ino))", path: f.path)
        try FileManager.default.moveItem(at: file, to: f.directory.appendingPathComponent("retired-original"))
        let successor = Data("a later owner of the same name".utf8)
        try successor.write(to: file)
        let reopened = try IMLocalStore(path: f.path)
        XCTAssertThrowsError(try reopened.drainReceiptFileCleanup(namespace: f.namespace))
        XCTAssertEqual(try Data(contentsOf: file), successor)
    }
}
