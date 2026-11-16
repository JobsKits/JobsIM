//
//  IMAuthoritativeSyncTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import CryptoKit
import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport
import XCTest

@testable import JobsIM

@MainActor
final class IMAuthoritativeSyncTests: XCTestCase {
    @MainActor
    private final class Fixture {
        let store: IMLocalStore
        let namespace: IMNamespace
        let conversation = "11111111-1111-4111-8111-111111111111"
        let messageID = "22222222-2222-4222-8222-222222222222"
        let snapshotID = "33333333-3333-4333-8333-333333333333"
        let stream = "44444444-4444-4444-8444-444444444444"
        let permission = String(repeating: "a", count: 64)
        var calls: [(String, String, [String: IMJSON])] = []
        var hook: ((String, [String: IMJSON]) throws -> IMJSON?)?
        var live = true
        var clientNonce = "88888888-8888-4888-8888-888888888888"
        var referenceEnvelopes: [Int: (IMJSON, IMJSON)] = [:]

        init(path: String = ":memory:") throws {
            store = try IMLocalStore(path: path)
            namespace = IMNamespace(
                profile: "sync-fixture", account: "55555555-5555-4555-8555-555555555555",
                demo: false,
                restoreEpoch: "1", deploymentID: "66666666-6666-4666-8666-666666666666",
                sourceContextID: "personal", deviceID: "77777777-7777-4777-8777-777777777777")
        }

        var message: IMJSON {
            return .object([
                "id": .string(messageID), "conversationId": .string(conversation),
                "senderId": .string(namespace.account),
                "clientMsgId": .string(clientNonce),
                "type": .string("text"), "text": .string("原字节：<消息> & \"quoted\""),
                "version": .string("9007199254740993"), "contentVersion": .string("1"),
                "sequence": .string("1"),
            ])
        }

        func ref(kind: String, id: String, revision: String = "1") -> IMJSON {
            var value: [String: IMJSON] = [
                "owner": .string(kind == "message" ? "message" : "conversation"),
                "kind": .string(kind),
                "id": .string(id), "revision": .string(revision), "revisionEpoch": .string("1"),
                "state": .string("active"),
            ]
            if kind == "message" { value["conversationId"] = .string(conversation) }
            return .object(value)
        }

        func envelope(_ value: IMJSON) throws -> (IMJSON, IMJSON) {
            let bytes = try JSONEncoder.make().encode(value)
            let digest = SHA256.hash(data: bytes).map { String(format: "%02x", $0) }.joined()
            let encoded = bytes.base64EncodedString().replacingOccurrences(of: "+", with: "-")
                .replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
            return (.string(encoded), .string(digest))
        }

        func projection(_ ref: IMJSON, body: IMJSON, state: String = "available") throws -> IMJSON {
            let (encoded, digest) = try envelope(body)
            return .object([
                "ref": ref, "state": .string(state), "currentRevision": ref["revision"],
                "currentRevisionEpoch": .string("1"), "revisionMatch": .bool(true),
                "object": body, "bodyEnvelope": encoded, "bodyDigest": digest,
            ])
        }

        func binding() throws -> IMJSON {
            let state = try store.syncState(namespace: namespace)
            var fields: [String: IMJSON] = [:]
            for key in [
                "scopeId", "principalId", "sourceContextId", "deviceId", "storeGeneration",
                "restoreEpoch",
            ] {
                fields[key] = state[key]
            }
            fields["streamId"] = .string(stream)
            return .object(fields)
        }

        func page(index: Int) throws -> IMJSON {
            let ref =
                index == 0
                ? ref(kind: "conversation", id: conversation) : ref(kind: "message", id: messageID)
            let body: IMJSON =
                index == 0
                ? .object([
                    "id": .string(conversation), "kind": .string("group"), "title": .string("群"),
                    "memberIds": .array([.string(namespace.account)]),
                ]) : message
            let refs: IMJSON = .array([ref])
            let original: (IMJSON, IMJSON)
            if let stored = referenceEnvelopes[index] {
                original = stored
            } else {
                original = try envelope(refs)
                referenceEnvelopes[index] = original
            }
            let (encoded, digest) = original
            return .object([
                "snapshotId": .string(snapshotID), "page": .number(Decimal(index)), "refs": refs,
                "referenceEnvelope": encoded, "pageDigest": digest,
                "items": .array([try projection(ref, body: body)]),
                "permissionGeneration": .string(permission), "restoreEpoch": .string("1"),
                "installable": .bool(true),
                "requiresFinalValidation": .bool(true), "requiresAtomicInstall": .bool(true),
                "qualifiedAck": .bool(false), "digestScope": .string("immutable_references_only"),
            ])
        }

        func manifest() throws -> IMJSON {
            return try binding().setting("snapshotId", .string(snapshotID)).setting(
                "schemaVersion", .string("1")
            )
            .setting("boundary", .string("0")).setting("boundaryToken", .string("boundary"))
            .setting("permissionGeneration", .string(permission)).setting(
                "classificationWatermark", .string("9")
            )
            .setting("coverage", .object([:])).setting(
                "ownerWatermarks",
                .object([
                    "auth": .string("0"), "directory": .string("0"), "conversation": .string("1"),
                    "message": .string("1"),
                ])
            ).setting(
                "expiresAt",
                .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(600)))
            )
            .setting(
                "pageManifest",
                .array(
                    try (0..<2).map { index in
                        return .object([
                            "page": .number(Decimal(index)), "count": .number(1),
                            "digest": try page(index: index)["pageDigest"],
                        ])
                    }))
        }

        func changes() -> IMJSON {
            return .object([
                "items": .array([]), "cursor": .string("continuous-0"), "position": .string("0"),
                "head": .string("0"),
                "hasMore": .bool(false), "restoreEpoch": .string("1"),
                "permissionGeneration": .string(permission),
                "snapshotRequired": .bool(false), "qualifiedAck": .bool(false),
            ])
        }

        func request(_ path: String, _ method: String, _ body: [String: IMJSON]) async throws
            -> IMJSON
        {
            calls.append((path, method, body))
            if let result = try hook?(path, body) {
                return result
            }
            if path == "bootstrap" {
                return .object([
                    "deploymentId": .string(namespace.deploymentID), "restoreEpoch": .string("1"),
                    "features": .object([
                        "authoritative_sync": .bool(true), "sync_jetstream": .bool(true),
                    ]),
                ])
            }
            if path == "sync/device" {
                return try binding().setting("qualification", .string("transport_only")).setting(
                    "qualifiedAck", .bool(false))
            }
            if path == "sync/context" {
                return try binding().setting("head", .string("0")).setting(
                    "permissionGeneration", .string(permission)
                )
                .setting("qualification", .string("transport_only")).setting(
                    "qualifiedAck", .bool(false)
                )
                .setting("afterReceiptEnabled", .bool(false))
            }
            if path == "sync/snapshots" {
                return try manifest()
            }
            if path.hasSuffix("/pages/0") {
                return try page(index: 0)
            }
            if path.hasSuffix("/pages/1") {
                return try page(index: 1)
            }
            if path.hasSuffix("/validate") {
                return .object([
                    "canInstall": .bool(true), "boundaryToken": .string("boundary"),
                    "restoreEpoch": .string("1"),
                    "permissionGeneration": .string(permission),
                    "classificationWatermark": .string("10"),
                    "requiresLocalAtomicInstall": .bool(true), "qualifiedAck": .bool(false),
                ])
            }
            if path.hasPrefix("sync/changes?") {
                return changes()
            }
            if path.hasSuffix("/message-results") {
                throw IMAPIError(
                    status: 404, code: "MESSAGE_RESULT_NOT_FOUND", message: "not found")
            }
            throw IMSyncProtocol.Failure(code: "UNEXPECTED_TEST_ROUTE")
        }

        func engine() -> IMAuthoritativeSync {
            return IMAuthoritativeSync(
                store: store, namespace: namespace,
                request: { [self] path, method, body in
                    return try await request(path, method, body)
                }, isCurrent: { [self] in return live })
        }

        func original() -> IMJSON {
            return message.setting("id", message["clientMsgId"]).setting(
                "sendAdmission", .string("original-token")
            )
            .setting("restoreEpoch", .string("1")).setting("status", .string("queued"))
        }
    }

    func testCompleteSnapshotWaitsForFirstPermissionDeltaAndKeepsLargeVersions() async throws {
        let f = try Fixture()
        let original = try f.page(index: 0)
        XCTAssertEqual(try f.page(index: 0)["referenceEnvelope"], original["referenceEnvelope"])
        XCTAssertEqual(try f.page(index: 0)["pageDigest"], original["pageDigest"])
        var checked = false
        f.hook = { path, _ in
            if path.hasPrefix("sync/changes?") {
                XCTAssertTrue(
                    try f.store.list("messages:" + f.conversation, namespace: f.namespace).isEmpty)
                XCTAssertEqual(try f.store.syncState(namespace: f.namespace)["cursor"], .null)
                checked = true
            }
            return nil
        }
        try await f.engine().run()
        XCTAssertTrue(checked)
        XCTAssertEqual(
            try f.store.list("messages:" + f.conversation, namespace: f.namespace).first?["version"]
                .text, "9007199254740993")
        XCTAssertEqual(
            try f.store.syncState(namespace: f.namespace)["classificationWatermark"], .string("10"))
        XCTAssertEqual(
            try f.store.syncState(namespace: f.namespace)["cursor"], .string("continuous-0"))
    }

    func testUnknownSnapshotRetriesIdenticalDurableIntentAfterReopen() async throws {
        let path = FileManager.default.temporaryDirectory.appendingPathComponent(
            UUID().uuidString + ".sqlite"
        ).path
        let f = try Fixture(path: path)
        var failed = false
        f.hook = { path, _ in
            if path == "sync/snapshots", !failed {
                failed = true
                throw IMAPIError(status: 503, code: "UNKNOWN", message: "unknown")
            }
            return nil
        }
        do {
            try await f.engine().run()
            XCTFail("unknown must not install")
        } catch {}
        let before = try f.store.syncState(namespace: f.namespace)
        let reopened = try IMLocalStore(path: path)
        XCTAssertEqual(
            try reopened.syncState(namespace: f.namespace)["snapshotIntent"],
            before["snapshotIntent"])
        XCTAssertEqual(
            try reopened.syncState(namespace: f.namespace)["registrationRequestId"],
            before["registrationRequestId"])
        f.hook = nil
        try await f.engine().run()
        let sent = f.calls.filter { $0.0 == "sync/snapshots" }
        XCTAssertEqual(sent.count, 2)
        XCTAssertEqual(sent[0].2, sent[1].2)
    }

    func testMissingOrCorruptedSecondPageNeverInstallsFirstPage() async throws {
        let f = try Fixture()
        f.hook = { path, _ in
            if path.hasSuffix("/pages/1") {
                return try f.page(index: 1).setting("referenceEnvelope", .string("e30"))
            }
            return nil
        }
        do {
            try await f.engine().run()
            XCTFail("corrupt page must fail")
        } catch {}
        XCTAssertTrue(try f.store.list("conversations", namespace: f.namespace).isEmpty)
        XCTAssertTrue(
            try f.store.list("messages:" + f.conversation, namespace: f.namespace).isEmpty)
        XCTAssertEqual(try f.store.syncState(namespace: f.namespace)["cursor"], .null)
        XCTAssertFalse(f.calls.contains { $0.0.hasSuffix("/validate") })
    }

    func testValidationWatermarkRegressionAndPermissionABARefuseInstallation() async throws {
        for changePermission in [false, true] {
            let f = try Fixture()
            f.hook = { path, _ in
                if !changePermission, path.hasSuffix("/validate") {
                    return .object([
                        "canInstall": .bool(true), "boundaryToken": .string("boundary"),
                        "restoreEpoch": .string("1"),
                        "permissionGeneration": .string(f.permission),
                        "classificationWatermark": .string("8"),
                        "requiresLocalAtomicInstall": .bool(true), "qualifiedAck": .bool(false),
                    ])
                }
                if changePermission, path.hasPrefix("sync/changes?") {
                    return f.changes().setting(
                        "permissionGeneration", .string(String(repeating: "b", count: 64)))
                }
                return nil
            }
            do {
                try await f.engine().run()
                XCTFail("changed permission/watermark must fail")
            } catch {}
            XCTAssertTrue(
                try f.store.list("messages:" + f.conversation, namespace: f.namespace).isEmpty)
        }
    }

    func testLocalGuardChangedAfterValidatePreventsLateAtomicCommit() async throws {
        let f = try Fixture()
        f.hook = { path, _ in
            if path.hasPrefix("sync/changes?") {
                try f.store.syncRestrict(namespace: f.namespace, reason: "local revoke")
            }
            return nil
        }
        do {
            try await f.engine().run()
            XCTFail("late response must lose CAS")
        } catch {}
        XCTAssertTrue(
            try f.store.list("messages:" + f.conversation, namespace: f.namespace).isEmpty)
        XCTAssertEqual(try f.store.syncState(namespace: f.namespace)["cursor"], .null)
    }

    func testLateAccountContextCannotPublishSnapshot() async throws {
        let f = try Fixture()
        f.hook = { path, _ in
            if path.hasPrefix("sync/changes?") { f.live = false }
            return nil
        }
        do {
            try await f.engine().run()
            XCTFail("old context must fail")
        } catch {}
        XCTAssertTrue(
            try f.store.list("messages:" + f.conversation, namespace: f.namespace).isEmpty)
    }

    func testSyncMatchingClientIDIsNotOutboxProofAndNeverPostsCreate() async throws {
        let f = try Fixture()
        try f.store.save([f.message], collection: "messages:" + f.conversation, namespace: f.namespace)
        try f.store.enqueue(f.original(), conversation: f.conversation, namespace: f.namespace)
        f.hook = { path, _ in
            if path.hasSuffix("/message-results") {
                throw IMAPIError(status: 503, code: "RESULT_UNKNOWN", message: "unknown proof")
            }
            if path.hasSuffix("/pages/1") {
                let page = try f.page(index: 1)
                let tombstone = f.message.setting("deleted", .bool(true))
                return page.setting("items", .array([try f.projection(page["refs"].list[0], body: tombstone, state: "global")]))
            }
            return nil
        }
        try await f.engine().run()
        XCTAssertNotNil(
            try f.store.originalOutbox(
                clientID: f.original()["clientMsgId"].text, namespace: f.namespace))
        XCTAssertTrue(
            f.calls.contains { $0.0 == "conversations/" + f.conversation + "/message-results" })
        XCTAssertFalse(f.calls.contains { $0.1 == "POST" && $0.0.hasSuffix("/messages") })
        let submitted = try XCTUnwrap(f.calls.first { $0.0.hasSuffix("/message-results") })
        XCTAssertEqual(submitted.2, IMSession.wireFields(f.original().fields))
        XCTAssertEqual(try f.store.originalOutbox(clientID: f.original()["clientMsgId"].text, namespace: f.namespace), f.original())
        XCTAssertTrue(try f.store.syncGlobalIDs(namespace: f.namespace).contains(f.messageID))
        XCTAssertEqual(try f.store.list("retained:messages:" + f.conversation, namespace: f.namespace).first?["text"], .null)
    }

    func testOriginalReadOnlyProofClosesOutboxOnlyWithAtomicOriginalBodyMatch() async throws {
        let f = try Fixture()
        try f.store.enqueue(f.original(), conversation: f.conversation, namespace: f.namespace)
        f.hook = { path, _ in
            if path.hasSuffix("/message-results") {
                return f.message
            }
            return nil
        }
        try await f.engine().run()
        XCTAssertNil(
            try f.store.originalOutbox(
                clientID: f.original()["clientMsgId"].text, namespace: f.namespace))
        let wrong = f.original().setting("text", .string("changed original"))
        try f.store.enqueue(wrong, conversation: f.conversation, namespace: f.namespace)
        XCTAssertThrowsError(
            try f.store.acknowledgeOriginal(
                f.original(), message: f.message, namespace: f.namespace))
        XCTAssertEqual(
            try f.store.originalOutbox(clientID: wrong["clientMsgId"].text, namespace: f.namespace),
            wrong)
    }

    func testGlobalIsTerminalAcrossDeviceRetainedSavedAndUnreadHolders() async throws {
        let f = try Fixture()
        try await f.engine().run()
        let old = IMNamespace(
            profile: f.namespace.profile, account: f.namespace.account, demo: false,
            restoreEpoch: "1", deploymentID: f.namespace.deploymentID, sourceContextID: "personal",
            deviceID: "99999999-9999-4999-8999-999999999999")
        let oldState = try f.store.syncState(namespace: old)
        _ = try f.store.syncUpdate(
            namespace: old, expected: oldState, next: oldState.setting("restriction", .string("")))
        for ns in [f.namespace, old] {
            for collection in [
                "retained:messages:" + f.conversation, "saved", "unread:" + f.conversation,
            ] {
                try f.store.save([f.message], collection: collection, namespace: ns)
            }
        }
        let state = try f.store.syncState(namespace: f.namespace)
        let global = try f.projection(
            f.ref(kind: "message", id: f.messageID, revision: "2"),
            body: f.message.setting("deleted", .bool(true)), state: "global")
        try f.store.syncCommit(
            namespace: f.namespace, expected: state, next: state, projections: [global], events: [],
            proofs: [], replaceCloud: false)
        for ns in [f.namespace, old] {
            for collection in [
                "retained:messages:" + f.conversation, "saved", "unread:" + f.conversation,
            ] {
                try f.store.save(
                    [f.message.setting("version", .string("9007199254740999"))],
                    collection: collection, namespace: ns)
                let row = try XCTUnwrap(f.store.list(collection, namespace: ns).first)
                XCTAssertEqual(row["text"], .null)
                XCTAssertEqual(row["deleted"], .bool(true))
            }
        }
    }

    func testOrdinaryCopiesRetainedWhileForwardBodyAndProofNeverPersistInSyncStore() async throws {
        let f = try Fixture()
        try await f.engine().run()
        let forwarded = f.message.setting("id", .string("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"))
            .setting("forwarded", .bool(true)).setting(
                "forwardAdmission", .string("temporary-proof")
            )
            .setting("attachmentURL", .string("/api/messages/forward/attachment"))
        try f.store.save(
            [forwarded], collection: "messages:" + f.conversation, namespace: f.namespace)
        let row = try XCTUnwrap(
            f.store.list("messages:" + f.conversation, namespace: f.namespace).first {
                $0["id"] == forwarded["id"]
            })
        XCTAssertEqual(row["text"], .null)
        XCTAssertEqual(row["forwardAdmission"], .null)
        XCTAssertEqual(row["attachmentURL"], .null)
        try f.store.syncRestrict(namespace: f.namespace, reason: "temporary transport unknown")
        XCTAssertTrue(
            try f.store.list("messages:" + f.conversation, namespace: f.namespace).isEmpty)
        let retained = try f.store.list(
            "retained:messages:" + f.conversation, namespace: f.namespace)
        XCTAssertEqual(retained.count, 1)
        XCTAssertEqual(retained.first?["text"], f.message["text"])
    }

    func testAnnouncementPersistsOnlyReferenceWithoutSourceBodyOrProof() async throws {
        let f = try Fixture()
        try await f.engine().run()
        let source = f.message.setting("forwarded", .bool(true))
            .setting("forwardAdmission", .string("temporary-proof"))
        let announcement: IMJSON = .object([
            "conversationId": .string(f.conversation), "messageId": .string(f.messageID),
            "sourceVersion": .string("1"), "version": .string("1"),
            "state": .string("active"), "message": source,
        ])
        let projection = try f.projection(
            f.ref(kind: "announcement", id: f.conversation), body: announcement)
        let state = try f.store.syncState(namespace: f.namespace)
        try f.store.syncCommit(
            namespace: f.namespace, expected: state, next: state, projections: [projection],
            events: [], proofs: [], replaceCloud: false)
        let stored = try XCTUnwrap(f.store.list("announcements", namespace: f.namespace).first)
        XCTAssertEqual(stored["messageId"], .string(f.messageID))
        XCTAssertEqual(stored["sourceVersion"], .string("1"))
        XCTAssertEqual(stored["message"], .null)
    }

    func testStrictCountersOriginalBodyDigestAndContinuousRedactions() throws {
        XCTAssertEqual(
            try IMSyncProtocol.counter(.string("9007199254740993")), 9_007_199_254_740_993)
        for invalid in [
            IMJSON.number(1), .string("01"), .string("9223372036854775808"), .string("-1"),
        ] {
            XCTAssertThrowsError(try IMSyncProtocol.counter(invalid))
        }
        let f = try Fixture()
        let page = try f.page(index: 1)
        XCTAssertNoThrow(try IMSyncProtocol.page(page, manifest: f.manifest(), index: 1))
        let originalBytes = Data("{\"a\":\"x\",\"b\":\"y\"}".utf8)
        let reorderedBytes = Data("{\"b\":\"y\",\"a\":\"x\"}".utf8)
        let originalDigest = SHA256.hash(data: originalBytes).map {
            String(format: "%02x", $0)
        }.joined()
        let reorderedEnvelope = reorderedBytes.base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
        XCTAssertThrowsError(
            try IMSyncProtocol.envelope(
                .string(reorderedEnvelope), digest: .string(originalDigest)))
        let changed = page["items"].list[0].setting(
            "object", f.message.setting("text", .string("forged")))
        XCTAssertThrowsError(
            try IMSyncProtocol.page(
                page.setting("items", .array([changed])), manifest: f.manifest(), index: 1))
        let gap = f.changes().setting("position", .string("3")).setting("head", .string("3"))
            .setting(
                "items",
                .array([
                    .object([
                        "kind": .string("redacted"), "from": .string("2"), "to": .string("3"),
                        "rangeDigest": .string(f.permission),
                    ])
                ]))
        XCTAssertThrowsError(
            try IMSyncProtocol.changes(
                gap, previous: .string("0"), epoch: .string("1"), permission: .string(f.permission))
        )
    }

    func testCapabilityLossCannotReturnToLegacyPollOrCreateRoutes() async throws {
        let f = try Fixture()
        try await f.engine().run()
        f.hook = { path, _ in
            if path == "bootstrap" {
                return .object([
                    "deploymentId": .string(f.namespace.deploymentID), "restoreEpoch": .string("1"),
                    "features": .object([:]),
                ])
            }
            return nil
        }
        let count = f.calls.count
        do {
            try await f.engine().run()
            XCTFail("missing capability must fail closed")
        } catch {}
        XCTAssertEqual(f.calls.count, count + 1)
        XCTAssertTrue(try f.store.list("conversations", namespace: f.namespace).isEmpty)
        XCTAssertEqual(
            try f.store.list("retained:messages:" + f.conversation, namespace: f.namespace).first?[
                "text"], f.message["text"])
    }

    func testDeltaOriginConflictRollsBackBodyAndContinuousPosition() async throws {
        let f = try Fixture()
        try await f.engine().run()
        var position = "1"
        let origin = "abababab-abab-4bab-8bab-abababababab"
        f.hook = { path, _ in
            guard path.hasPrefix("sync/changes?") else {
                return nil
            }
            let ref = f.ref(kind: "message", id: f.messageID, revision: position)
            let original: IMJSON = .object([
                "position": .string(position), "originEventId": .string(origin), "ref": ref,
            ])
            let (envelope, digest) = try f.envelope(original)
            let message = f.message.setting("text", .string("delta " + position))
                .setting(
                    "version", .string(position == "1" ? "9007199254740994" : "9007199254740995"))
            let event = original.setting("kind", .string("reference"))
                .setting("eventEnvelope", envelope).setting("eventDigest", digest)
                .setting("projection", try f.projection(ref, body: message))
            return f.changes().setting("items", .array([event])).setting(
                "position", .string(position)
            )
            .setting("head", .string(position)).setting("cursor", .string("delta-" + position))
        }
        try await f.engine().run()
        XCTAssertEqual(try f.store.syncState(namespace: f.namespace)["position"], .string("1"))
        position = "2"
        do {
            try await f.engine().run()
            XCTFail("same origin at a different position must conflict")
        } catch {}
        XCTAssertEqual(try f.store.syncState(namespace: f.namespace)["position"], .string("1"))
        XCTAssertEqual(try f.store.syncState(namespace: f.namespace)["cursor"], .string("delta-1"))
        XCTAssertEqual(
            try f.store.list("retained:messages:" + f.conversation, namespace: f.namespace).first?[
                "text"], .string("delta 1"))
    }

    func testUnknownDeltaCannotAdvanceCursorOrEraseLawfulOrdinaryCopy() async throws {
        let f = try Fixture()
        try await f.engine().run()
        f.hook = { path, _ in
            guard path.hasPrefix("sync/changes?") else {
                return nil
            }
            let ref = f.ref(kind: "message", id: f.messageID, revision: "2")
            let original: IMJSON = .object([
                "position": .string("1"),
                "originEventId": .string("acacacac-acac-4cac-8cac-acacacacacac"), "ref": ref,
            ])
            let (envelope, digest) = try f.envelope(original)
            let event = original.setting("kind", .string("reference"))
                .setting("eventEnvelope", envelope).setting("eventDigest", digest)
                .setting(
                    "projection",
                    .object([
                        "ref": ref, "state": .string("unknown"), "revisionMatch": .bool(false),
                    ]))
            return f.changes().setting("items", .array([event])).setting("position", .string("1"))
                .setting("head", .string("1")).setting("cursor", .string("must-not-install"))
        }
        do {
            try await f.engine().run()
            XCTFail("unknown is not global and not installable")
        } catch {}
        XCTAssertEqual(try f.store.syncState(namespace: f.namespace)["position"], .string("0"))
        XCTAssertEqual(
            try f.store.retainedMessages(conversation: f.conversation, namespace: f.namespace)
                .first?["text"], f.message["text"])
        XCTAssertTrue(try f.store.syncGlobalIDs(namespace: f.namespace).isEmpty)
    }

    func testRetainedEntryRequiresPreviouslyValidatedExactBodyAndSameIdentity() async throws {
        let f = try Fixture()
        let unverified = f.message.setting("text", .string("old unverified backup"))
        try f.store.save(
            [unverified], collection: "retained:messages:" + f.conversation, namespace: f.namespace)
        XCTAssertTrue(
            try f.store.retainedMessages(conversation: f.conversation, namespace: f.namespace)
                .isEmpty)
        try await f.engine().run()
        XCTAssertTrue(
            try f.store.retainedMessages(conversation: f.conversation, namespace: f.namespace)
                .isEmpty)
        try f.store.syncRestrict(namespace: f.namespace, reason: "known local ordinary copy")
        XCTAssertEqual(try f.store.retainedConversations(namespace: f.namespace).count, 1)
        XCTAssertEqual(
            try f.store.retainedMessages(conversation: f.conversation, namespace: f.namespace)
                .first?["text"], f.message["text"])
        let otherEpoch = IMNamespace(
            profile: f.namespace.profile, account: f.namespace.account, demo: false,
            restoreEpoch: "2", deploymentID: f.namespace.deploymentID, sourceContextID: "personal",
            deviceID: f.namespace.deviceID)
        XCTAssertTrue(
            try f.store.retainedMessages(conversation: f.conversation, namespace: otherEpoch)
                .isEmpty)
        try f.store.save(
            [unverified], collection: "retained:messages:" + f.conversation, namespace: f.namespace)
        XCTAssertTrue(try f.store.retainedConversations(namespace: f.namespace).isEmpty)
    }

    func testStoreGenerationSurvivesReopenAndCannotSilentlyReplaceARegisteredStore() async throws {
        let path = FileManager.default.temporaryDirectory.appendingPathComponent(
            UUID().uuidString + ".sqlite"
        ).path
        let f = try Fixture(path: path)
        let state = try f.store.syncState(namespace: f.namespace)
        let reopened = try IMLocalStore(path: path)
        XCTAssertEqual(
            try reopened.syncState(namespace: f.namespace)["storeGeneration"],
            state["storeGeneration"])
        let other = try Fixture()
        XCTAssertNotEqual(
            try other.store.syncState(namespace: other.namespace)["storeGeneration"],
            state["storeGeneration"])
        other.hook = { path, _ in
            if path == "sync/device" {
                throw IMAPIError(
                    status: 409, code: "STORE_GENERATION_CHANGED", message: "new empty store")
            }
            return nil
        }
        do {
            try await other.engine().run()
            XCTFail("new empty store must require a new authenticated device")
        } catch {}
        XCTAssertFalse(other.calls.contains { $0.0 == "sync/snapshots" })
        XCTAssertEqual(
            try reopened.syncState(namespace: f.namespace)["registrationRequestId"],
            state["registrationRequestId"])
    }

    func testSchemaTwoNegotiationArchivesOldResponsibilityAndKeepsOriginalQueue() async throws {
        let f = try Fixture()
        try await f.engine().run()
        try f.store.enqueue(f.original(), conversation: f.conversation, namespace: f.namespace)
        let original = try f.store.syncState(namespace: f.namespace)
        let selected = try f.store.syncSelectSchema(namespace: f.namespace, expected: original, schema: "2")
        XCTAssertEqual(selected["schemaVersion"], .string("2"))
        XCTAssertEqual(selected["storeGeneration"], original["storeGeneration"])
        XCTAssertEqual(selected["deviceId"], original["deviceId"])
        XCTAssertNotEqual(selected["registrationRequestId"], original["registrationRequestId"])
        XCTAssertEqual(selected["position"], .string("0"))
        XCTAssertEqual(selected["cursor"], .null)
        XCTAssertEqual(selected["streamId"], .null)
        XCTAssertEqual(try f.store.list("sync-schema-archives", namespace: f.namespace).count, 1)
        XCTAssertEqual(try f.store.originalOutbox(clientID: f.original()["clientMsgId"].text,
                                                namespace: f.namespace), f.original())
        XCTAssertThrowsError(try f.store.syncSelectSchema(namespace: f.namespace, expected: selected, schema: "1"))
        XCTAssertThrowsError(try f.store.syncSelectSchema(namespace: f.namespace, expected: original, schema: "2"))
        XCTAssertEqual(try f.store.syncState(namespace: f.namespace), selected)
    }

    func testSchemaTwoPersonalEnvelopeIsExplicitAndOwnerBound() throws {
        let f = try Fixture()
        let ref: IMJSON = .object(["owner": .string("directory"), "kind": .string("preferences"),
                                   "id": .string(f.namespace.account), "subjectUserId": .string(f.namespace.account),
                                   "revision": .string("1"), "revisionEpoch": .string("1"), "state": .string("active")])
        let body: IMJSON = .object(["scopeId": .string(f.namespace.deploymentID), "sourceContextId": .string("personal"),
                                    "userId": .string(f.namespace.account), "revisionEpoch": .string("1"),
                                    "version": .string("9007199254740993"), "fontPreset": .string("large"), "explicit": .bool(true)])
        let projection = try f.projection(ref, body: body)
        XCTAssertThrowsError(try IMSyncProtocol.projection(projection, reference: ref))
        XCTAssertEqual(try IMSyncProtocol.projection(projection, reference: ref, schema: "2")["object"], body)
        let other = body.setting("userId", .string(f.conversation))
        XCTAssertThrowsError(try IMSyncProtocol.projection(f.projection(ref, body: other), reference: ref, schema: "2"))
        let rounded = body.setting("version", .number(9007199254740993))
        XCTAssertThrowsError(try IMSyncProtocol.projection(f.projection(ref, body: rounded), reference: ref, schema: "2"))
    }

    func testSchemaTwoPersonalStateAndCursorRollbackOnWrongScope() async throws {
        let f = try Fixture()
        try await f.engine().run()
        let original = try f.store.syncState(namespace: f.namespace)
        let selected = try f.store.syncSelectSchema(namespace: f.namespace, expected: original, schema: "2")
        let ref: IMJSON = .object(["owner": .string("directory"), "kind": .string("preferences"),
                                   "id": .string(f.namespace.account), "subjectUserId": .string(f.namespace.account),
                                   "revision": .string("1"), "revisionEpoch": .string("1"), "state": .string("active")])
        let body: IMJSON = .object(["scopeId": .string(f.namespace.deploymentID), "sourceContextId": .string("personal"),
                                    "userId": .string(f.namespace.account), "revisionEpoch": .string("1"),
                                    "version": .string("1"), "fontPreset": .string("large"), "explicit": .bool(true)])
        let good = try f.projection(ref, body: body)
        let bad = try f.projection(ref.setting("id", .string(f.messageID)),
                                   body: body.setting("scopeId", .string(f.conversation)))
        XCTAssertThrowsError(try f.store.syncCommit(namespace: f.namespace, expected: selected,
                                                   next: selected.setting("cursor", .string("uncommitted")),
                                                   projections: [good, bad], events: [], proofs: [], replaceCloud: false))
        XCTAssertEqual(try f.store.syncState(namespace: f.namespace), selected)
        XCTAssertTrue(try f.store.list("self:preferences", namespace: f.namespace).isEmpty)
        try f.store.syncCommit(namespace: f.namespace, expected: selected,
                               next: selected.setting("cursor", .string("committed")),
                               projections: [good], events: [], proofs: [], replaceCloud: false)
        XCTAssertEqual(try f.store.syncState(namespace: f.namespace)["cursor"], .string("committed"))
        XCTAssertEqual(try f.store.list("self:preferences", namespace: f.namespace).first?["fontPreset"], .string("large"))
    }

    func testOpaqueCLIClientNonceIsNotRequiredToBeAServerUUID() async throws {
        let f = try Fixture()
        f.clientNonce = "3a1542717a864d439cfe0db5da3918dd"
        try await f.engine().run()
        XCTAssertEqual(
            try f.store.list("messages:" + f.conversation, namespace: f.namespace).first?[
                "clientMsgId"], .string(f.clientNonce))
        let ref = f.ref(kind: "message", id: f.messageID)
        let bad = f.message.setting("clientMsgId", .string(String(repeating: "🙂", count: 33)))
        let projection = try f.projection(ref, body: bad)
        XCTAssertThrowsError(try IMSyncProtocol.projection(projection, reference: ref))
    }
}
