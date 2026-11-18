//
//  IMMigrationTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月17日，星期二。
//

import CryptoKit
import Foundation
import JobsIMCore
import JobsIMStorage
import XCTest

@testable import JobsIM

private func migrationFixtureBytes(_ value: IMJSON) throws -> Data {
    let encoder = JSONEncoder.make()
    encoder.outputFormatting = [.sortedKeys, .withoutEscapingSlashes]
    return try encoder.encode(value)
}

@MainActor
final class IMMigrationTests: XCTestCase {

    @MainActor
    private final class Fixture {
        let store: IMLocalStore
        let namespace: IMNamespace
        let group = "11111111-1111-4111-8111-111111111111"
        let messageID = "22222222-2222-4222-8222-222222222222"
        let objectID = "33333333-3333-4333-8333-333333333333"
        let migrationID = "44444444-4444-4444-8444-444444444444"
        let keyBytes = Data(repeating: 0x2A, count: 32)
        var state: IMJSON = .null
        var manifest: IMJSON = .null
        var packet: IMJSON = .null
        var view: IMJSON = .null
        var key: IMJSON = .null
        var capsule = Data()

        init(full: Bool = false, forwarded: Bool = false, fractional: Bool = false) throws {
            store = try IMLocalStore(path: ":memory:")
            namespace = IMNamespace(profile: "migration-test", account: "55555555-5555-4555-8555-555555555555",
                demo: false, restoreEpoch: "1", deploymentID: "66666666-6666-4666-8666-666666666666",
                sourceContextID: "personal", deviceID: "77777777-7777-4777-8777-777777777777")
            let initial = try store.syncState(namespace: namespace)
            state = try store.syncUpdate(namespace: namespace, expected: initial, next: initial.setting("restriction", .string("")))
            try store.save([.object(["id": .string(group), "kind": .string("group"), "state": .string("active"),
                "membershipGeneration": .string("7"), "readOnly": .bool(false)])], collection: "conversations", namespace: namespace)
            let bytes = Data("private original object".utf8)
            var message: IMJSON = .object(["id": .string(messageID), "conversationId": .string(group),
                "senderId": .string(namespace.account), "type": .string(full ? "file" : "text"),
                "text": .string("原字节 <标题> & བོད་ཡིག\nsecond line"),
                "createdAt": .string("2026-11-17T00:00:00Z"), "contentVersion": .string("9007199254740993"),
                "creationEpoch": .string("1"), "creationSequence": .string("11"), "objectMode": .string(full ? "full" : "none")])
            if full { message = message.setting("attachmentId", .string(objectID)) }
            if forwarded { message = message.setting("forwarded", .bool(true)) }
            var ref: IMJSON = .object(["messageId": .string(messageID), "conversationId": .string(group),
                "contentVersion": message["contentVersion"], "membershipGeneration": .string("7"),
                "classification": .string("available"), "primaryDigest": .string(IMMigrationProtocol.hash(try migrationFixtureBytes(message))),
                "objectMode": message["objectMode"]])
            if full { ref = ref.setting("objectId", .string(objectID)) }
            let objects: [IMJSON] = full ? [.object(["objectId": .string(objectID), "messageId": .string(messageID),
                "sizeBytes": .string(String(bytes.count)), "sha256": .string(IMMigrationProtocol.hash(bytes)),
                "contentType": .string("application/octet-stream"), "classification": .string("available")])] : []
            let formatter = ISO8601DateFormatter.make()
            if fractional {
                formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
            }
            var expiry = formatter.string(from: Date().addingTimeInterval(240))
            if fractional {
                expiry = String(expiry.dropLast()) + "123456Z"
            }
            manifest = .object(["format": .string(IMMigrationProtocol.format), "id": .string(migrationID),
                "scopeId": .string(namespace.deploymentID), "userId": .string(namespace.account), "sourceContextId": .string("personal"),
                "sourceDeviceId": .string("88888888-8888-4888-8888-888888888888"),
                "sourceStoreGeneration": .string("99999999-9999-4999-8999-999999999999"),
                "targetDeviceId": .string(namespace.deviceID), "targetStoreGeneration": state["storeGeneration"],
                "restoreEpoch": .string("1"), "securityVersion": .string("2"), "sourceGeneration": .string("1"),
                "messages": .array([ref]), "objects": .array(objects),
                "expiresAt": .string(expiry)])
            let envelope = try migrationFixtureBytes(manifest)
            let digest = IMMigrationProtocol.hash(envelope)
            packet = .object(["format": .string(IMMigrationProtocol.format), "manifestDigest": .string(digest),
                "messages": .array([message]), "objects": .array(full ? [.object(["objectId": .string(objectID),
                    "messageId": .string(messageID), "bytes": .string(bytes.base64EncodedString())])] : [])])
            view = .object(["id": .string(migrationID), "requestId": .string("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"),
                "status": .string("ready"), "manifest": manifest, "manifestEnvelope": .string(IMMigrationProtocol.base64(envelope)),
                "manifestDigest": .string(digest), "capsuleSHA256": .string(""), "sizeBytes": .string("0"),
                "expiresAt": manifest["expiresAt"], "sourcePreserved": .bool(true), "qualifiedAck": .bool(false), "targetQualified": .bool(false)])
            let aad = ["JobsIM migration v1", namespace.deploymentID, migrationID, namespace.account,
                manifest["sourceDeviceId"].text, manifest["sourceStoreGeneration"].text, namespace.deviceID,
                state["storeGeneration"].text, "1", "2", "1", digest].joined(separator: "\0")
            key = .object(["id": .string(migrationID), "manifestDigest": .string(digest), "algorithm": .string("AES-256-GCM"),
                "key": .string(IMMigrationProtocol.base64(keyBytes)), "aad": .string(IMMigrationProtocol.base64(Data(aad.utf8))),
                "expiresAt": manifest["expiresAt"], "targetQualified": .bool(false)])
            try repack(packet)
        }

        var binding: IMMigrationBinding {
            return IMMigrationBinding(namespace: namespace, family: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
                                      physical: state["storeGeneration"].text, guardState: state)
        }

        func repack(_ value: IMJSON) throws {
            let plain = try migrationFixtureBytes(value)
            let nonce = Data(repeating: 0x31, count: 12)
            let box = try AES.GCM.seal(plain, using: SymmetricKey(data: keyBytes), nonce: AES.GCM.Nonce(data: nonce),
                                      authenticating: IMMigrationProtocol.decode(key["aad"], maximum: 8192))
            capsule = Data("JIMMIGR1".utf8)
            capsule.append(nonce)
            capsule.append(box.ciphertext)
            capsule.append(box.tag)
            view = view.setting("capsuleSHA256", .string(IMMigrationProtocol.hash(capsule))).setting("sizeBytes", .string(String(capsule.count)))
        }

        func publish(before: Date = Date().addingTimeInterval(5)) throws {
            let decoded = try IMMigrationProtocol.open(view, capsule: capsule, key: key, binding: binding)
            try store.commitMigrationArchive(namespace: namespace, expected: state, manifest: manifest,
                                             digest: view["manifestDigest"].text, packet: decoded, installBefore: before)
        }

        func global() throws {
            let state = try XCTUnwrap(store.syncStateIfPresent(namespace: namespace))
            let ref: IMJSON = .object(["owner": .string("message"), "kind": .string("message"), "id": .string(messageID),
                "conversationId": .string(group), "revision": .string("2"), "revisionEpoch": .string("1")])
            let item: IMJSON = .object(["ref": ref, "state": .string("global"), "currentRevision": .string("2"),
                "currentRevisionEpoch": .string("1"), "object": .object(["id": .string(messageID), "conversationId": .string(group),
                    "deleted": .bool(true), "deleteScope": .string("global")])])
            try store.syncCommit(namespace: namespace, expected: state, next: state, projections: [item], events: [], proofs: [], replaceCloud: false)
        }
    }

    func testEncryptedPacketAndFullOriginalPreserveExactCountersWithoutQualification() throws {
        let f = try Fixture(full: true)
        let packet = try IMMigrationProtocol.open(f.view, capsule: f.capsule, key: f.key, binding: f.binding)
        XCTAssertEqual(packet, f.packet)
        try f.publish()
        let messages = try f.store.list("migration-messages:" + f.migrationID, namespace: f.namespace)
        XCTAssertEqual(messages.first?["contentVersion"].text, "9007199254740993")
        XCTAssertEqual(messages.first?["text"], f.packet["messages"].list[0]["text"])
        XCTAssertEqual(try f.store.list("migration-archives", namespace: f.namespace).first?["targetQualified"], .bool(false))
        XCTAssertEqual(try f.store.syncStateIfPresent(namespace: f.namespace), f.state)
        XCTAssertTrue(try f.store.pending(namespace: f.namespace).isEmpty)
    }

    func testWrongAADOrTamperedCapsuleNeverOpens() throws {
        let f = try Fixture()
        let wrong = f.key.setting("aad", .string(IMMigrationProtocol.base64(Data("another-context".utf8))))
        XCTAssertThrowsError(try IMMigrationProtocol.open(f.view, capsule: f.capsule, key: wrong, binding: f.binding))
        var tampered = f.capsule
        tampered[tampered.count - 1] ^= 1
        XCTAssertThrowsError(try IMMigrationProtocol.open(f.view, capsule: tampered, key: f.key, binding: f.binding))
        XCTAssertTrue(try f.store.list("migration-archives", namespace: f.namespace).isEmpty)
    }

    func testManifestEnvelopeIsHashedAsOriginalBytesAndCrossTargetRejected() throws {
        let f = try Fixture()
        let wrong = f.view.setting("manifest", f.manifest.setting("securityVersion", .string("3")))
        XCTAssertThrowsError(try IMMigrationProtocol.view(wrong, binding: f.binding, target: true))
        let binding = IMMigrationBinding(namespace: f.namespace, family: f.binding.family,
                                         physical: "cccccccc-cccc-4ccc-8ccc-cccccccccccc", guardState: f.state)
        XCTAssertThrowsError(try IMMigrationProtocol.view(f.view, binding: binding, target: true))
        XCTAssertEqual(try IMMigrationProtocol.view(f.view, binding: f.binding, target: true), f.view)
    }

    func testUnknownSecretFieldsAndOriginalObjectDigestRejectWholePacket() throws {
        let f = try Fixture(full: true)
        let bad = f.packet.setting("messages", .array([f.packet["messages"].list[0].setting("password", .string("synthetic-not-to-persist"))]))
        try f.repack(bad)
        XCTAssertThrowsError(try IMMigrationProtocol.open(f.view, capsule: f.capsule, key: f.key, binding: f.binding))
        let wrong = f.packet["objects"].list[0].setting("bytes", .string(Data("changed source".utf8).base64EncodedString()))
        try f.repack(f.packet.setting("objects", .array([wrong])))
        XCTAssertThrowsError(try IMMigrationProtocol.open(f.view, capsule: f.capsule, key: f.key, binding: f.binding))
        XCTAssertTrue(try f.store.list("migration-archives", namespace: f.namespace).isEmpty)
    }

    func testExpiredPublishAndChangedGuardLeaveWholeArchiveUnpublished() throws {
        let f = try Fixture(full: true)
        XCTAssertThrowsError(try f.publish(before: Date().addingTimeInterval(-1)))
        _ = try f.store.syncUpdate(namespace: f.namespace, expected: f.state, next: f.state)
        XCTAssertThrowsError(try f.publish())
        XCTAssertTrue(try f.store.list("migration-archives", namespace: f.namespace).isEmpty)
        XCTAssertTrue(try f.store.list("migration-messages:" + f.migrationID, namespace: f.namespace).isEmpty)
        XCTAssertTrue(try f.store.list("migration-objects:" + f.migrationID, namespace: f.namespace).isEmpty)
    }

    func testNewerLocalContentAndMembershipChangeDenyStaleWholeImport() throws {
        let f = try Fixture()
        try f.store.save([f.packet["messages"].list[0].setting("contentVersion", .string("9007199254740994"))],
                         collection: "messages:" + f.group, namespace: f.namespace)
        XCTAssertThrowsError(try f.publish())
        XCTAssertTrue(try f.store.list("migration-archives", namespace: f.namespace).isEmpty)
        let g = try Fixture()
        try g.store.save([.object(["id": .string(g.group), "state": .string("active"), "membershipGeneration": .string("8")])], collection: "conversations", namespace: g.namespace)
        XCTAssertThrowsError(try g.publish())
    }

    func testGlobalDeniesInitialPublishAndClearsCommittedOriginalBytes() throws {
        let f = try Fixture(full: true)
        try f.publish()
        try f.global()
        let messages = try f.store.list("migration-messages:" + f.migrationID, namespace: f.namespace)
        let objects = try f.store.list("migration-objects:" + f.migrationID, namespace: f.namespace)
        XCTAssertTrue(messages.first?["text"].text.isEmpty == true)
        XCTAssertTrue(objects.first?["bytes"].text.isEmpty == true)
        let next = try Fixture(full: true)
        try next.global()
        next.state = try XCTUnwrap(next.store.syncStateIfPresent(namespace: next.namespace))
        XCTAssertThrowsError(try next.publish())
        XCTAssertTrue(try next.store.list("migration-archives", namespace: next.namespace).isEmpty)
    }

    func testForwardReobservationInvalidatesIndependentProjectionCopy() throws {
        let f = try Fixture(forwarded: true)
        try f.publish()
        XCTAssertFalse(try f.store.list("migration-messages:" + f.migrationID, namespace: f.namespace)[0]["text"].text.isEmpty)
        try f.store.save([f.packet["messages"].list[0].setting("forwardUnavailable", .bool(true))],
                         collection: "messages:" + f.group, namespace: f.namespace)
        XCTAssertTrue(try f.store.list("migration-messages:" + f.migrationID, namespace: f.namespace)[0]["text"].text.isEmpty)
    }
    func testEncryptedStageIsClearedByGlobalAndLateStageCannotReturn() throws {
        let f = try Fixture(full: true)
        try f.store.stageMigrationCapsule(namespace: f.namespace, expected: f.state, id: f.migrationID,
            manifestDigest: f.view["manifestDigest"].text, capsuleSHA256: f.view["capsuleSHA256"].text,
            expiresAt: f.view["expiresAt"].text, messageIDs: [f.messageID], capsule: f.capsule)
        let staged = try XCTUnwrap(f.store.list("migration-stages", namespace: f.namespace).first)
        XCTAssertEqual(try IMMigrationProtocol.decode(staged["capsule"], maximum: IMMigrationProtocol.limit + 4096), f.capsule)
        XCTAssertEqual(staged["key"], .null)
        try f.global()
        XCTAssertTrue(try f.store.list("migration-stages", namespace: f.namespace).isEmpty)
        f.state = try XCTUnwrap(f.store.syncStateIfPresent(namespace: f.namespace))
        XCTAssertThrowsError(try f.store.stageMigrationCapsule(namespace: f.namespace, expected: f.state, id: f.migrationID,
            manifestDigest: f.view["manifestDigest"].text, capsuleSHA256: f.view["capsuleSHA256"].text,
            expiresAt: f.view["expiresAt"].text, messageIDs: [f.messageID], capsule: f.capsule))
        XCTAssertTrue(try f.store.list("migration-stages", namespace: f.namespace).isEmpty)
        let g = try Fixture()
        try g.store.stageMigrationCapsule(namespace: g.namespace, expected: g.state, id: g.migrationID,
            manifestDigest: g.view["manifestDigest"].text, capsuleSHA256: g.view["capsuleSHA256"].text,
            expiresAt: g.view["expiresAt"].text, messageIDs: [g.messageID], capsule: g.capsule)
        try g.publish()
        XCTAssertTrue(try g.store.list("migration-stages", namespace: g.namespace).isEmpty)
    }

    func testPrimaryMessageDigestUsesAuthenticatedRawObjectBytes() throws {
        let f = try Fixture()
        let changed = f.packet["messages"].list[0].setting("text", .string("changed after original manifest"))
        try f.repack(f.packet.setting("messages", .array([changed])))
        XCTAssertThrowsError(try IMMigrationProtocol.open(f.view, capsule: f.capsule, key: f.key, binding: f.binding))
        let data = try migrationFixtureBytes(f.packet)
        let ranges = try IMMigrationJSON.messageRanges(data)
        XCTAssertEqual(ranges.count, 1)
        XCTAssertEqual(IMMigrationProtocol.hash(data.subdata(in: ranges[0])), f.manifest["messages"].list[0]["primaryDigest"].text)
        XCTAssertThrowsError(try IMMigrationJSON.messageRanges(Data("{\"messages\":[],\"messages\":[]}".utf8)))
    }

    func testGoFractionalNanosecondExpiryStagesAndPublishes() throws {
        let f = try Fixture(full: true, fractional: true)
        XCTAssertTrue(f.view["expiresAt"].text.contains("."))
        try f.store.stageMigrationCapsule(namespace: f.namespace, expected: f.state, id: f.migrationID,
            manifestDigest: f.view["manifestDigest"].text, capsuleSHA256: f.view["capsuleSHA256"].text,
            expiresAt: f.view["expiresAt"].text, messageIDs: [f.messageID], capsule: f.capsule)
        try f.publish()
        XCTAssertEqual(try f.store.list("migration-archives", namespace: f.namespace).count, 1)
        XCTAssertTrue(try f.store.list("migration-stages", namespace: f.namespace).isEmpty)
    }


    func testOwnerUnavailableReadOnlyHistoryRemainsArchivable() throws {
        let f = try Fixture()
        try f.store.save([.object(["id": .string(f.group), "kind": .string("group"), "state": .string("owner_unavailable"),
            "membershipGeneration": .string("7"), "readOnly": .bool(true)])], collection: "conversations", namespace: f.namespace)
        try f.publish()
        XCTAssertEqual(try f.store.list("migration-messages:" + f.migrationID, namespace: f.namespace).count, 1)
    }

}
