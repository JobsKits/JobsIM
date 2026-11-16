//
//  IMSendAdmissionTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport
import SQLite3
import UIKit
import XCTest

@testable import JobsIM

@MainActor
final class IMSendAdmissionTests: XCTestCase {
    func testPrepareLegacyAcceptedMessageBeforeUpgrade() async throws {
        let api = try IMIntegrationFixture.client()
        let descriptor = try await api.request("bootstrap")
        guard descriptor["features"]["send_admission"] != .bool(true) else {
            throw XCTSkip("Legacy preparation requires the pre-upgrade binary")
        }
        let account = try await IMIntegrationFixture.register(api)
        let conversation = try await api.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("group"), "title": .string("Native legacy admission proof"),
                "memberIds": .array([]),
            ], token: account["token"].text)
        let body: [String: IMJSON] = [
            "clientMsgId": .string(UUID().uuidString), "type": .string("text"),
            "text": .string("Original committed legacy message"), "restoreEpoch": .string("1"),
        ]
        let accepted = try await api.request(
            "conversations/" + conversation["id"].text + "/messages", method: "POST", body: body,
            token: account["token"].text)
        XCTAssertEqual(accepted["clientMsgId"], body["clientMsgId"])
        let profile = UUID().uuidString
        try IMKeychain.write(account["token"].text, key: profile + ":" + account["user"]["id"].text)
        let fixture = IMJSON.object([
            "profile": .string(profile), "user": account["user"], "conversation": conversation,
            "body": .object(body), "accepted": accepted,
        ])
        UserDefaults.standard.set(try JSONEncoder.make().encode(fixture), forKey: "admissionLegacyFixture")
    }
    private func installSession(_ api: IMAPIClient?, user: IMJSON, conversation: IMJSON) async throws
        -> @MainActor () -> Void
    {
        let descriptor = try await api?.request("bootstrap")
        let session = IMSession.shared
        let old = (
            session.profile, session.user, session.token, session.demo, session.api, session.restoreEpoch,
            session.changed, session.deployment
        )
        session.changed = nil
        session.profile = UUID().uuidString
        session.user = user["user"].fields.isEmpty ? user : user["user"]
        session.token = user["token"].text.isEmpty ? nil : user["token"].text
        session.demo = false
        session.api = api
        session.restoreEpoch = "1"
        session.deployment = descriptor?["deploymentId"].text ?? "no-api-fixture"
        try session.store.save([conversation], collection: "conversations", namespace: session.namespace)
        return {
            session.profile = old.0
            session.user = old.1
            session.token = old.2
            session.demo = old.3
            session.api = old.4
            session.restoreEpoch = old.5
            session.changed = old.6
            session.deployment = old.7
        }
    }
    private func original(
        _ id: String = UUID().uuidString, conversation: String = "c",
        admission: String? = "original-admission"
    ) -> IMJSON {
        var value = IMJSON.object([
            "id": .string(id), "clientMsgId": .string(id), "conversationId": .string(conversation),
            "type": .string("text"), "text": .string("Original user intent"), "restoreEpoch": .string("1"),
            "status": .string("queued"),
        ])
        if let admission { value = value.setting("sendAdmission", .string(admission)) }
        return value
    }
    func testLegacyDatabaseUpgradeFreezesWithoutRewritingPayload() throws {
        let path = FileManager.default.temporaryDirectory.appendingPathComponent(
            UUID().uuidString + ".sqlite"
        ).path
        let namespace = IMNamespace(profile: "old-profile", account: "same-account", demo: false)
        let legacy = original("legacy-message", admission: nil)
        let bytes = try JSONEncoder.make().encode(legacy)
        let hex = bytes.map { String(format: "%02x", $0) }.joined()
        var connection: OpaquePointer?
        XCTAssertEqual(sqlite3_open(path, &connection), SQLITE_OK)
        let sql = """
            CREATE TABLE grdb_migrations(identifier TEXT NOT NULL PRIMARY KEY);
            INSERT INTO grdb_migrations VALUES('initial-durable-outbox');
            CREATE TABLE records(namespace TEXT NOT NULL,collection TEXT NOT NULL,id TEXT NOT NULL,body BLOB NOT NULL,ordering TEXT NOT NULL,PRIMARY KEY(namespace,collection,id));
            CREATE TABLE outbox(namespace TEXT NOT NULL,id TEXT NOT NULL,conversation TEXT NOT NULL,body BLOB NOT NULL,status TEXT NOT NULL,PRIMARY KEY(namespace,id));
            INSERT INTO records VALUES('\(namespace.key)','messages:c','legacy-message',X'\(hex)','original-order');
            INSERT INTO outbox VALUES('\(namespace.key)','legacy-message','c',X'\(hex)','queued');
            """
        XCTAssertEqual(sqlite3_exec(connection, sql, nil, nil, nil), SQLITE_OK)
        sqlite3_close(connection)
        let upgraded = try IMLocalStore(path: path)
        XCTAssertTrue(try upgraded.pending(namespace: namespace).isEmpty)
        XCTAssertEqual(try upgraded.originalOutbox(clientID: "legacy-message", namespace: namespace), legacy)
        XCTAssertEqual(try upgraded.list("messages:c", namespace: namespace).first?["status"].text, "frozen")
        XCTAssertThrowsError(
            try upgraded.enqueue(
                legacy.setting("sendAdmission", .string("new-admission")), conversation: "c",
                namespace: namespace))
        XCTAssertEqual(sqlite3_open(path, &connection), SQLITE_OK)
        defer { sqlite3_close(connection) }
        var statement: OpaquePointer?
        XCTAssertEqual(
            sqlite3_prepare_v2(
                connection, "SELECT hex(body) FROM outbox WHERE id='legacy-message'", -1, &statement, nil),
            SQLITE_OK)
        defer { sqlite3_finalize(statement) }
        XCTAssertEqual(sqlite3_step(statement), SQLITE_ROW)
        XCTAssertEqual(String(cString: sqlite3_column_text(statement, 0)).lowercased(), hex)
        let reopened = try IMLocalStore(path: path)
        XCTAssertTrue(try reopened.pending(namespace: namespace).isEmpty)
        XCTAssertEqual(try reopened.originalOutbox(clientID: "legacy-message", namespace: namespace), legacy)
    }
    func testFrozenSnapshotCannotBeReplacedOrAutomaticallyQueued() throws {
        let store = try IMLocalStore(path: ":memory:")
        let namespace = IMNamespace(profile: "profile", account: "user", demo: false)
        let body = original("immutable-client-id")
        try store.enqueue(body, conversation: "c", namespace: namespace)
        try store.save(
            [.object(["id": .string("c"), "sendAdmission": .string("latest-admission")])],
            collection: "conversations", namespace: namespace)
        XCTAssertEqual(
            try store.pending(namespace: namespace).first?["sendAdmission"].text, "original-admission")
        try store.freezeOutbox(
            clientID: "immutable-client-id", conversation: "c", namespace: namespace,
            reason: "admission_changed")
        XCTAssertTrue(try store.pending(namespace: namespace).isEmpty)
        try store.enqueue(body, conversation: "c", namespace: namespace)
        XCTAssertTrue(try store.pending(namespace: namespace).isEmpty)
        XCTAssertThrowsError(
            try store.enqueue(
                body.setting("sendAdmission", .string("latest-admission")), conversation: "c",
                namespace: namespace))
        XCTAssertEqual(try store.originalOutbox(clientID: "immutable-client-id", namespace: namespace), body)
        let accepted = body.setting("id", .string("server-message")).setting("status", .string("accepted"))
        try store.acknowledge(
            clientID: "immutable-client-id", message: accepted, conversation: "c", namespace: namespace)
        XCTAssertNil(try store.originalOutbox(clientID: "immutable-client-id", namespace: namespace))
        XCTAssertEqual(try store.list("messages:c", namespace: namespace), [accepted])
    }
    func testMissingSnapshotIsFrozenInFormalSpaceWithoutAffectingDemo() throws {
        let store = try IMLocalStore(path: ":memory:")
        let formal = IMNamespace(profile: "profile", account: "user", demo: false)
        let demo = IMNamespace(profile: "profile", account: "user", demo: true)
        let body = original("missing-client-id", admission: nil)
        try store.enqueue(body, conversation: "c", namespace: formal)
        try store.enqueue(body, conversation: "c", namespace: demo)
        XCTAssertTrue(try store.pending(namespace: formal).isEmpty)
        XCTAssertEqual(try store.pending(namespace: demo), [body])
        XCTAssertEqual(try store.originalOutbox(clientID: "missing-client-id", namespace: formal), body)
        XCTAssertTrue(
            try store.originalOutbox(clientID: "missing-client-id", namespace: formal)?["sendAdmission"].text
                .isEmpty == true)
    }
    func testMissingAdmissionDoesNotConsumeDraftAndUploadSnapshotRemainsOriginal() async throws {
        let restore = try await installSession(
            nil, user: .object(["id": .string("snapshot-user")]),
            conversation: .object(["id": .string("c"), "readOnly": .bool(false)]))
        defer { restore() }
        let session = IMSession.shared
        let namespace = session.namespace
        let draft = IMJSON.object(["id": .string("c"), "text": .string("Keep this draft")])
        try session.store.save([draft], collection: "drafts", namespace: namespace)
        do {
            try await session.send(
                "Keep this draft", conversation: "c",
                didEnqueue: { XCTFail("Missing snapshot committed a new intent") })
            XCTFail("Missing admission accepted")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "SEND_ADMISSION_UNAVAILABLE") }
        XCTAssertTrue(try session.store.pending(namespace: namespace).isEmpty)
        XCTAssertEqual(try session.store.list("drafts", namespace: namespace), [draft])
        try session.store.save(
            [.object(["id": .string("c"), "sendAdmission": .string("new-after-upload")])],
            collection: "conversations", namespace: namespace)
        XCTAssertEqual(
            try session.admissionSnapshot("c", supplied: "captured-before-upload"), "captured-before-upload")
        try session.store.save(
            [.object(["id": .string("c"), "sendAdmission": .number(99)])], collection: "conversations",
            namespace: namespace)
        XCTAssertThrowsError(try session.admissionSnapshot("c"))
    }
    func testFrozenMessageDoesNotClaimServerAcceptance() {
        let cell = IMMessageCell(style: .default, reuseIdentifier: nil)
        cell.configure(original("frozen-bubble").setting("status", .string("frozen")), own: true)
        func texts(_ view: UIView) -> [String] {
            return (view as? UILabel).flatMap { $0.text }.map { [$0] } ?? view.subviews.flatMap(texts)
        }
        let labels = texts(cell)
        XCTAssertTrue(labels.contains { $0.contains(IMStrings.text("sendAdmissionPaused")) })
        XCTAssertFalse(labels.contains { $0.contains(IMStrings.text("accepted")) })
    }

    private func strictClient() async throws -> IMAPIClient {
        let api = try IMIntegrationFixture.client()
        let bootstrap = try await api.request("bootstrap")
        XCTAssertEqual(bootstrap["features"]["send_admission"], .bool(true))
        guard bootstrap["features"]["send_admission"] == .bool(true) else {
            throw IMSession.SessionError.invalidInstance
        }
        return api
    }
    private func friends(_ first: IMJSON, _ second: IMJSON, api: IMAPIClient) async throws {
        let request = try await api.request(
            "contacts/requests", method: "POST",
            body: ["userId": second["user"]["id"], "requestId": .string(UUID().uuidString)],
            token: first["token"].text)
        _ = try await api.request(
            "contacts/requests/" + request["id"].text + "/accept", method: "POST", token: second["token"].text
        )
    }
    private func conversation(_ id: String, account: IMJSON, api: IMAPIClient) async throws -> IMJSON {
        let list = try await api.request("conversations", token: account["token"].text)
        return try XCTUnwrap(list["items"].list.first { $0["id"].text == id })
    }
    func testRealGroupRejoinFreezesUnobservedOriginalIntent() async throws {
        let api = try await strictClient()
        let owner = try await IMIntegrationFixture.register(api)
        let member = try await IMIntegrationFixture.register(api)
        try await friends(owner, member, api: api)
        let created = try await api.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("group"), "title": .string("Native admission group"),
                "memberIds": .array([member["user"]["id"]]),
            ], token: owner["token"].text)
        let id = created["id"].text
        let before = try await conversation(id, account: member, api: api)
        XCTAssertFalse(before["sendAdmission"].text.isEmpty)
        let restore = try await installSession(api, user: member, conversation: before)
        defer { restore() }
        let session = IMSession.shared
        let namespace = session.namespace
        let pending = original(conversation: id, admission: before["sendAdmission"].text)
        try session.store.enqueue(pending, conversation: id, namespace: namespace)
        let members = try await api.request("conversations/" + id + "/members", token: owner["token"].text)
        let generation = try XCTUnwrap(
            members["items"].list.first { $0["id"] == member["user"]["id"] }?["membershipGeneration"].text)
        _ = try await api.request(
            "conversations/" + id + "/members/" + member["user"]["id"].text + "?membershipGeneration="
                + generation, method: "DELETE", token: owner["token"].text)
        _ = try await api.request(
            "conversations/" + id + "/members", method: "POST", body: ["userId": member["user"]["id"]],
            token: owner["token"].text)
        let after = try await conversation(id, account: member, api: api)
        XCTAssertNotEqual(after["membershipGeneration"], before["membershipGeneration"])
        XCTAssertTrue(after["sendAdmission"] != before["sendAdmission"])
        try session.store.save([after], collection: "conversations", namespace: namespace)
        await session.flushPending()
        XCTAssertTrue(try session.store.pending(namespace: namespace).isEmpty)
        XCTAssertTrue(
            try session.store.originalOutbox(clientID: pending["clientMsgId"].text, namespace: namespace)
                == pending)
        XCTAssertEqual(
            try session.store.list("messages:" + id, namespace: namespace).first?["status"].text, "frozen")
        do {
            try await session.reconcileOriginalMessage(pending["clientMsgId"].text)
            XCTFail("Old uncommitted group intent was admitted")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "MESSAGE_RESULT_NOT_FOUND") }
        try await session.send("New explicit authorized group intent", conversation: id)
        let history = try await api.request(
            "conversations/" + id + "/messages?after=0", token: member["token"].text)
        XCTAssertFalse(history["items"].list.contains { $0["clientMsgId"] == pending["clientMsgId"] })
        XCTAssertTrue(
            history["items"].list.contains { $0["text"].text == "New explicit authorized group intent" })
    }
    func testRealDirectRemovalAndBlockCyclesDoNotRevivePending() async throws {
        let api = try await strictClient()
        let first = try await IMIntegrationFixture.register(api)
        let second = try await IMIntegrationFixture.register(api)
        try await friends(first, second, api: api)
        let created = try await api.request(
            "conversations", method: "POST",
            body: ["kind": .string("direct"), "memberIds": .array([second["user"]["id"]])],
            token: first["token"].text)
        let id = created["id"].text
        let restore = try await installSession(api, user: first, conversation: created)
        defer { restore() }
        let session = IMSession.shared
        let namespace = session.namespace
        for cycle in ["remove", "block"] {
            let before = try await conversation(id, account: first, api: api)
            let committed = original(conversation: id, admission: before["sendAdmission"].text).setting(
                "text", .string("Committed before " + cycle))
            let body = committed.fields.filter {
                ["clientMsgId", "type", "text", "restoreEpoch", "sendAdmission"].contains($0.key)
            }
            let accepted = try await api.request(
                "conversations/" + id + "/messages", method: "POST", body: body, token: first["token"].text)
            try session.store.enqueue(committed, conversation: id, namespace: namespace)
            let neverCommitted = original(conversation: id, admission: before["sendAdmission"].text)
            try session.store.enqueue(neverCommitted, conversation: id, namespace: namespace)
            if cycle == "block" {
                _ = try await api.request(
                    "blocks/" + first["user"]["id"].text, method: "PUT", token: second["token"].text)
                _ = try await api.request(
                    "blocks/" + first["user"]["id"].text, method: "DELETE", token: second["token"].text)
                _ = try await api.request(
                    "contacts/" + first["user"]["id"].text + "/reopen", method: "POST",
                    body: ["requestId": .string(UUID().uuidString)], token: second["token"].text)
            } else {
                _ = try await api.request(
                    "contacts/" + first["user"]["id"].text, method: "DELETE", token: second["token"].text)
            }
            // 已提交事实走当前read ACL，不要求此时仍有新的direct发送许可。
            try await session.reconcileOriginalMessage(committed["clientMsgId"].text)
            XCTAssertNil(
                try session.store.originalOutbox(
                    clientID: committed["clientMsgId"].text, namespace: namespace))
            XCTAssertTrue(
                try session.store.list("messages:" + id, namespace: namespace).contains {
                    $0["id"] == accepted["id"]
                })
            try await friends(first, second, api: api)
            let after = try await conversation(id, account: first, api: api)
            XCTAssertTrue(after["sendAdmission"] != before["sendAdmission"])
            try session.store.save([after], collection: "conversations", namespace: namespace)
            await session.flushPending()
            XCTAssertTrue(try session.store.pending(namespace: namespace).isEmpty)
            XCTAssertTrue(
                try session.store.originalOutbox(
                    clientID: neverCommitted["clientMsgId"].text, namespace: namespace) == neverCommitted)
            XCTAssertTrue(
                try session.store.list("messages:" + id, namespace: namespace).contains {
                    $0["clientMsgId"] == neverCommitted["clientMsgId"] && $0["status"].text == "frozen"
                })
            let history = try await api.request(
                "conversations/" + id + "/messages?after=0", token: first["token"].text)
            XCTAssertTrue(history["items"].list.contains { $0["id"] == accepted["id"] })
            XCTAssertFalse(
                history["items"].list.contains { $0["clientMsgId"] == neverCommitted["clientMsgId"] })
        }
    }
    func testRealLegacySuccessReconcilesButMissingOriginalCannotCreate() async throws {
        let api = try await strictClient()
        let data = try XCTUnwrap(UserDefaults.standard.data(forKey: "admissionLegacyFixture"))
        let fixture = try JSONDecoder.make().decode(IMJSON.self, from: data)
        let token = try XCTUnwrap(IMKeychain.read(fixture["profile"].text + ":" + fixture["user"]["id"].text))
        let account = IMJSON.object(["user": fixture["user"], "token": .string(token)])
        let id = fixture["conversation"]["id"].text
        let current = try await conversation(id, account: account, api: api)
        let restore = try await installSession(api, user: account, conversation: current)
        defer { restore() }
        let session = IMSession.shared
        let namespace = session.namespace
        let prior = fixture["body"].setting("id", fixture["body"]["clientMsgId"]).setting(
            "conversationId", .string(id)
        ).setting("status", .string("queued"))
        try session.store.enqueue(prior, conversation: id, namespace: namespace)
        XCTAssertTrue(try session.store.pending(namespace: namespace).isEmpty)
        try await session.reconcileOriginalMessage(prior["clientMsgId"].text)
        XCTAssertNil(
            try session.store.originalOutbox(clientID: prior["clientMsgId"].text, namespace: namespace))
        XCTAssertTrue(
            try session.store.list("messages:" + id, namespace: namespace).contains {
                $0["id"] == fixture["accepted"]["id"]
            })
        let absent = original(conversation: id, admission: nil)
        try session.store.enqueue(absent, conversation: id, namespace: namespace)
        do {
            try await session.reconcileOriginalMessage(absent["clientMsgId"].text)
            XCTFail("Legacy missing snapshot created a message")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "MESSAGE_RESULT_NOT_FOUND") }
        XCTAssertEqual(
            try session.store.originalOutbox(clientID: absent["clientMsgId"].text, namespace: namespace),
            absent)
        XCTAssertTrue(try session.store.pending(namespace: namespace).isEmpty)
        let history = try await api.request("conversations/" + id + "/messages?after=0", token: token)
        XCTAssertFalse(history["items"].list.contains { $0["clientMsgId"] == absent["clientMsgId"] })
    }

}
