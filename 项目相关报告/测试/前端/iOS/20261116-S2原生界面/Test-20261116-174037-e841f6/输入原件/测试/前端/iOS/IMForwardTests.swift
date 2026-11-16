//
//  IMForwardTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import CryptoKit
import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport
import XCTest

@testable import JobsIM

@MainActor
private final class IMForwardRaceGate {
    private var held = false
    private var released = false
    private var continuation: CheckedContinuation<Void, Never>?
    func hold() async {
        held = true
        if released { return }
        await withCheckedContinuation { continuation = $0 }
    }
    func waitUntilHeld() async throws {
        for _ in 0..<200 {
            if held { return }
            try await Task.sleep(for: .milliseconds(20))
        }
        throw IMAPIError(status: 0, code: "TEST_GATE_TIMEOUT", message: "Expected held response")
    }
    func release() {
        released = true
        continuation?.resume()
        continuation = nil
    }
}

@MainActor
final class IMForwardTests: XCTestCase {
    func testProjectionFencePersistsAndAcknowledgementsCannotUnmask() throws {
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(
            UUID().uuidString + ".sqlite")
        defer { try? FileManager.default.removeItem(at: file) }
        let namespace = IMNamespace(profile: "projection-race", account: "owner", demo: false)
        let message: IMJSON = .object([
            "id": .string("forward"), "clientMsgId": .string("intent"),
            "conversationId": .string("target"), "forwarded": .bool(true), "type": .string("text"),
            "version": .string("1"), "contentVersion": .string("1"), "text": .string("old body"),
            "attachmentURL": .string("/api/messages/forward/attachment"),
        ])
        do {
            let store = try IMLocalStore(path: file.path)
            try store.save([message], collection: "messages:target", namespace: namespace)
            try store.redactForward(messageID: "forward", conversation: "target", namespace: namespace)
        }
        let store = try IMLocalStore(path: file.path)
        try store.applyMessagePage([message], conversation: "target", cursor: "1:1", namespace: namespace)
        try store.acknowledge(
            clientID: "intent", message: message, conversation: "target", namespace: namespace)
        try store.applyForwardProjection(
            message, conversation: "target", namespace: namespace, expectedGeneration: "0")
        var value = try XCTUnwrap(store.list("messages:target", namespace: namespace).first)
        XCTAssertTrue(value["text"].text.isEmpty)
        XCTAssertEqual(value["forwardProjectionUnknown"], .bool(true))
        XCTAssertEqual(value["forwardProjectionGeneration"].text, "1")
        try store.applyForwardProjection(
            message, conversation: "target", namespace: namespace, expectedGeneration: "1")
        value = try XCTUnwrap(store.list("messages:target", namespace: namespace).first)
        XCTAssertEqual(value["text"].text, "old body")
        let permanent = message.setting("version", .string("2")).setting("text", .string(""))
            .setting("forwardUnavailable", .bool(true)).setting("contentUnavailable", .bool(true))
        try store.save([permanent], collection: "messages:target", namespace: namespace)
        try store.acknowledge(
            clientID: "intent", message: message, conversation: "target", namespace: namespace)
        try store.applyForwardProjection(
            message.setting("version", .string("2")), conversation: "target",
            namespace: namespace, expectedGeneration: "1")
        value = try XCTUnwrap(store.list("messages:target", namespace: namespace).first)
        XCTAssertEqual(value["version"].text, "2")
        XCTAssertEqual(value["forwardUnavailable"], .bool(true))
        XCTAssertTrue(value["text"].text.isEmpty)
    }

    func testRealHTTPSLateForwardResponsesCannotReviveProjection() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        let restore = install(api, account: a, descriptor: descriptor)
        defer { restore() }
        do {
            try await friend(a, b, api: api)
            let sourceGroup = try await group(
                "Native projection race source", owner: b, members: [a], api: api)
            let target = try await group("Native projection race target", owner: a, members: [], api: api)
            let source = try await post(
                "Native delayed projection", in: sourceGroup, account: b,
                descriptor: descriptor, api: api)
            let selected = try await IMForwardService().prepare(source)
            let namespace = IMSession.shared.namespace
            let gate = IMForwardRaceGate()
            var accepted: IMJSON = .null
            let sender = IMForwardService(sendRequest: { path, body in
                accepted = try await api.request(path, method: "POST", body: body, token: a["token"].text)
                await gate.hold()
                return accepted
            })
            let sendTask = Task { try await sender.send(selected, to: target["id"].text) }
            defer { gate.release() }
            try await gate.waitUntilHeld()
            let id = accepted["id"].text
            XCTAssertFalse(id.isEmpty)
            let history = try await api.request(
                "conversations/" + target["id"].text + "/messages", token: a["token"].text)
            try IMSession.shared.store.applyMessagePage(
                history["items"].list, conversation: target["id"].text,
                cursor: history["cursor"].text, namespace: namespace)
            let sidecar = try IMMediaCache.directory(namespace).appendingPathComponent(id + "-old-sidecar")
            try Data("old local projection".utf8).write(to: sidecar)
            let unavailable = IMForwardService(readRequest: { _ in
                throw IMAPIError(
                    status: 503, code: "FORWARD_QUALIFICATION_UNAVAILABLE",
                    message: "Controlled projection-read failure")
            })
            do {
                _ = try await unavailable.current(accepted)
                XCTFail("Expected controlled temporary read failure")
            } catch let error as IMAPIError { XCTAssertEqual(error.status, 503) }
            gate.release()
            try await sendTask.value
            XCTAssertFalse(FileManager.default.fileExists(atPath: sidecar.path))
            var value = try XCTUnwrap(
                IMSession.shared.store.list("messages:" + target["id"].text, namespace: namespace).first)
            XCTAssertTrue(value["text"].text.isEmpty)
            XCTAssertEqual(value["forwardProjectionUnknown"], .bool(true))
            // 真实旧历史及纯结果查询载荷晚到，也只能确认事实，不能解除临时占位。
            try IMSession.shared.store.applyMessagePage(
                history["items"].list, conversation: target["id"].text,
                cursor: history["cursor"].text, namespace: namespace)
            let lookup = try await api.request(
                "conversations/" + target["id"].text + "/message-results", method: "POST",
                body: [
                    "clientMsgId": accepted["clientMsgId"], "type": .string("forward"),
                    "forwardOfId": selected["id"],
                    "forwardSourceVersion": selected["contentVersion"],
                    "forwardAdmission": selected["forwardAdmission"],
                    "restoreEpoch": descriptor["restoreEpoch"], "sendAdmission": target["sendAdmission"],
                ], token: a["token"].text)
            try IMSession.shared.store.acknowledge(
                clientID: accepted["clientMsgId"].text, message: lookup,
                conversation: target["id"].text, namespace: namespace)
            value = try XCTUnwrap(
                IMSession.shared.store.list("messages:" + target["id"].text, namespace: namespace).first)
            XCTAssertTrue(value["text"].text.isEmpty)
            XCTAssertEqual(value["attachmentURL"], .null)

            let readGate = IMForwardRaceGate()
            let reader = IMForwardService(readRequest: { path in
                let response = try await api.request(path, token: a["token"].text)
                await readGate.hold()
                return response
            })
            let readTask = Task { try await reader.current(accepted) }
            defer { readGate.release() }
            try await readGate.waitUntilHeld()
            do { _ = try await unavailable.current(accepted) } catch {}
            readGate.release()
            let stale = try await readTask.value
            XCTAssertTrue(stale["text"].text.isEmpty)
            let fresh = try await IMForwardService().current(accepted)
            XCTAssertEqual(fresh["text"].text, "Native delayed projection")
            let beforeTemporary = try IMSession.shared.store.forwardProjectionGeneration(
                messageID: id,
                conversation: target["id"].text, namespace: namespace)
            let transient200 = IMForwardService(readRequest: { _ in
                return fresh.setting("forwardUnavailable", .bool(true)).setting("text", .string(""))
            })
            let temporary = try await transient200.current(fresh)
            XCTAssertTrue(temporary["text"].text.isEmpty)
            XCTAssertEqual(temporary["forwardUnavailable"], .bool(true))
            try IMSession.shared.store.save(
                [fresh], collection: "messages:" + target["id"].text, namespace: namespace)
            try IMSession.shared.store.applyForwardProjection(
                fresh, conversation: target["id"].text,
                namespace: namespace, expectedGeneration: beforeTemporary)
            value = try XCTUnwrap(
                IMSession.shared.store.list("messages:" + target["id"].text, namespace: namespace).first)
            XCTAssertTrue(value["text"].text.isEmpty)
            let sameVersionRecovered = try await IMForwardService().current(temporary)
            XCTAssertEqual(sameVersionRecovered["version"], fresh["version"])
            XCTAssertEqual(sameVersionRecovered["text"].text, "Native delayed projection")
            XCTAssertNotEqual(sameVersionRecovered["forwardUnavailable"], .bool(true))
            _ = try await api.request(
                "messages/" + source["id"].text, method: "PATCH",
                body: [
                    "text": .string("Changed race source"), "expectedVersion": source["version"],
                    "requestId": .string(UUID().uuidString),
                ], token: b["token"].text)
            let permanent = try await IMForwardService().current(accepted)
            XCTAssertEqual(permanent["forwardUnavailable"], .bool(true))
            try IMSession.shared.store.acknowledge(
                clientID: accepted["clientMsgId"].text, message: lookup,
                conversation: target["id"].text, namespace: namespace)
            value = try XCTUnwrap(
                IMSession.shared.store.list("messages:" + target["id"].text, namespace: namespace).first)
            XCTAssertTrue(value["text"].text.isEmpty)
            XCTAssertEqual(value["version"], permanent["version"])
            await logout([a, b], api: api)
        } catch {
            await logout([a, b], api: api)
            throw error
        }
    }
    func testReferenceWireAndExportNeverCopyProjection() throws {
        let fields: [String: IMJSON] = [
            "clientMsgId": .string("original"), "type": .string("forward"),
            "forwardOfId": .string("source"), "forwardSourceVersion": .string("9007199254740993123"),
            "forwardAdmission": .string("original-proof"), "restoreEpoch": .string("1"),
            "sendAdmission": .string("original-admission"), "text": .string("must not copy"),
            "attachmentId": .string("source-file"), "replyToId": .string("source-reply"),
            "mentionAll": .bool(true),
        ]
        let wire = IMSession.wireFields(fields)
        XCTAssertEqual(wire.count, 7)
        XCTAssertNil(wire["text"])
        XCTAssertNil(wire["attachmentId"])
        XCTAssertNil(wire["replyToId"])
        XCTAssertNil(wire["mentionAll"])
        XCTAssertEqual(wire["forwardSourceVersion"]?.text, "9007199254740993123")
        let store = try IMLocalStore(path: ":memory:")
        let namespace = IMNamespace(
            profile: "export-forward", account: "owner", demo: false, restoreEpoch: "1")
        try store.save(
            [.object(["id": .string("target")])], collection: "conversations", namespace: namespace)
        try store.save(
            [
                .object([
                    "id": .string("forward"), "conversationId": .string("target"),
                    "type": .string("text"), "forwarded": .bool(true), "text": .string("private projection"),
                    "forwardAdmission": .string("temporary-proof"),
                    "attachmentURL": .string("/api/messages/forward/attachment"),
                ])
            ],
            collection: "messages:target", namespace: namespace)
        let export = String(
            decoding: try IMHistoryExport.data(store: store, namespace: namespace), as: UTF8.self)
        XCTAssertFalse(export.contains("private projection"))
        XCTAssertFalse(export.contains("temporary-proof"))
        XCTAssertFalse(export.contains("attachmentURL"))
        try store.redactForward(messageID: "forward", conversation: "target", namespace: namespace)
        let redacted = try XCTUnwrap(store.list("messages:target", namespace: namespace).first)
        XCTAssertEqual(redacted["text"], .null)
        XCTAssertEqual(redacted["forwardAdmission"], .null)
        XCTAssertEqual(redacted["forwardProjectionUnknown"], .bool(true))
    }

    private func client() throws -> IMAPIClient {
        let original = try IMIntegrationFixture.certificate()
        let certificate = try IMInstanceCertificate(
            address: "https://127.0.0.1:28443",
            certificate: original.certificateDER, verifiedFingerprint: original.fingerprint)
        try IMTrustStore.shared.save(certificate)
        return IMAPIClient(baseURL: URL(string: certificate.address + "/api/")!, certificate: certificate)
    }

    func testDeploymentMismatchRejectsOriginalQueueWithoutChangingPayload() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let account = try await IMIntegrationFixture.register(api)
        let restore = install(api, account: account, descriptor: descriptor)
        defer { restore() }
        let conversation = try await group(
            "Native wrong deployment guard", owner: account, members: [], api: api)
        let namespace = IMSession.shared.namespace
        let clientID = UUID().uuidString
        let original: IMJSON = .object([
            "id": .string(clientID), "clientMsgId": .string(clientID),
            "conversationId": conversation["id"], "senderId": account["user"]["id"], "type": .string("text"),
            "text": .string("Must not cross instance identity"), "status": .string("queued"),
            "sendAdmission": conversation["sendAdmission"], "restoreEpoch": descriptor["restoreEpoch"],
        ])
        try IMSession.shared.store.enqueue(
            original, conversation: conversation["id"].text, namespace: namespace)
        IMSession.shared.deployment = UUID().uuidString
        do {
            try await IMSession.shared.reconcileOriginalMessage(clientID)
            XCTFail("Wrong instance cannot receive even a lookup with the original proof")
        } catch let error as IMAPIError {
            XCTAssertEqual(error.code, "DEPLOYMENT_CHANGED")
        }
        XCTAssertNil(IMSession.shared.token)
        XCTAssertTrue(
            try IMSession.shared.store.originalOutbox(clientID: clientID, namespace: namespace) == original)
        let history = try await api.request(
            "conversations/" + conversation["id"].text + "/messages", token: account["token"].text)
        XCTAssertTrue(history["items"].list.isEmpty)
        _ = try await api.request("auth/logout", method: "POST", token: account["token"].text)
    }

    private func install(_ api: IMAPIClient, account: IMJSON, descriptor: IMJSON) -> () -> Void {
        let session = IMSession.shared
        let old = (
            session.profile, session.user, session.token, session.demo, session.api,
            session.restoreEpoch, session.changed, session.baseURL, session.deployment
        )
        IMSyncCoordinator.shared.stop()
        session.changed = nil
        session.profile = UUID().uuidString
        session.user = account["user"]
        session.token = account["token"].text
        session.demo = false
        session.api = api
        session.baseURL = URL(string: "https://127.0.0.1:28443")
        session.deployment = descriptor["deploymentId"].text
        session.restoreEpoch = descriptor["restoreEpoch"].text
        return {
            session.profile = old.0
            session.user = old.1
            session.token = old.2
            session.demo = old.3
            session.api = old.4
            session.restoreEpoch = old.5
            session.changed = old.6
            session.baseURL = old.7
            session.deployment = old.8
            IMSyncCoordinator.shared.start()
        }
    }

    private func friend(_ a: IMJSON, _ b: IMJSON, api: IMAPIClient) async throws {
        let request = try await api.request(
            "contacts/requests", method: "POST",
            body: ["userId": b["user"]["id"], "requestId": .string(UUID().uuidString)], token: a["token"].text
        )
        _ = try await api.request(
            "contacts/requests/" + request["id"].text + "/accept", method: "POST",
            token: b["token"].text)
    }

    private func group(_ title: String, owner: IMJSON, members: [IMJSON], api: IMAPIClient) async throws
        -> IMJSON
    {
        return try await api.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("group"), "title": .string(title),
                "memberIds": .array(members.map { $0["user"]["id"] }),
            ], token: owner["token"].text)
    }

    private func post(
        _ text: String, in conversation: IMJSON, account: IMJSON,
        descriptor: IMJSON, api: IMAPIClient, attachment: String? = nil
    ) async throws -> IMJSON {
        var body: [String: IMJSON] = [
            "clientMsgId": .string(UUID().uuidString),
            "type": .string(attachment == nil ? "text" : "file"), "text": .string(text),
            "sendAdmission": conversation["sendAdmission"], "restoreEpoch": descriptor["restoreEpoch"],
        ]
        if let attachment { body["attachmentId"] = .string(attachment) }
        return try await api.request(
            "conversations/" + conversation["id"].text + "/messages", method: "POST",
            body: body, token: account["token"].text)
    }

    private func logout(_ accounts: [IMJSON], api: IMAPIClient) async {
        for account in accounts {
            _ = try? await api.request("auth/logout", method: "POST", token: account["token"].text)
        }
    }

    func testReferenceOutboxUnknownAndExplicitOriginalRetry() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        XCTAssertEqual(descriptor["features"]["forwarding"], .bool(true))
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        let c = try await IMIntegrationFixture.register(api)
        let restore = install(api, account: a, descriptor: descriptor)
        defer { restore() }
        do {
            try await friend(a, b, api: api)
            try await friend(a, c, api: api)
            let sourceGroup = try await group("Forward original", owner: b, members: [a], api: api)
            let target = try await group("Forward destination", owner: a, members: [c], api: api)
            let original = try await post(
                "Native reference-only content", in: sourceGroup, account: b,
                descriptor: descriptor, api: api)
            let source = try await IMForwardService().prepare(original)
            let firstID = UUID().uuidString
            var firstPosts = 0
            let lost = IMForwardService(sendRequest: { path, body in
                let persisted = try XCTUnwrap(
                    IMSession.shared.store.originalOutbox(
                        clientID: firstID,
                        namespace: IMSession.shared.namespace))
                XCTAssertTrue(persisted["forwardAdmission"] == source["forwardAdmission"])
                for key in ["text", "attachmentId", "replyToId", "mentionedUserIds", "mentionAll"] {
                    XCTAssertEqual(persisted[key], .null)
                    XCTAssertNil(body[key])
                }
                firstPosts += 1
                _ = try await api.request(path, method: "POST", body: body, token: a["token"].text)
                throw IMAPIError(status: 0, code: "TEST_LOST_RESPONSE", message: "Outcome unknown")
            })
            do {
                try await lost.send(source, to: target["id"].text, clientID: firstID)
                XCTFail("Successful response was deliberately lost")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.status, 0)
            }
            XCTAssertEqual(firstPosts, 1)
            XCTAssertTrue(try IMSession.shared.store.pending(namespace: IMSession.shared.namespace).isEmpty)
            try await IMForwardService().retryOriginal(firstID)
            XCTAssertEqual(firstPosts, 1)
            XCTAssertNil(
                try IMSession.shared.store.originalOutbox(
                    clientID: firstID, namespace: IMSession.shared.namespace))
            let history = try await api.request(
                "conversations/" + target["id"].text + "/messages", token: c["token"].text)
            let projected = try XCTUnwrap(history["items"].list.first { $0["clientMsgId"].text == firstID })
            XCTAssertEqual(projected["forwarded"], .bool(true))
            XCTAssertEqual(projected["text"].text, "Native reference-only content")
            do {
                _ = try await api.request("messages/" + original["id"].text, token: c["token"].text)
                XCTFail("The destination reader must not gain source membership")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.status, 403)
            }

            let secondID = UUID().uuidString
            let neverSent = IMForwardService(sendRequest: { _, _ in
                throw IMAPIError(status: 0, code: "TEST_NOT_SENT", message: "Outcome unknown")
            })
            do {
                try await neverSent.send(source, to: target["id"].text, clientID: secondID)
                XCTFail("First request was deliberately not sent")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.status, 0)
            }
            let oldBody = try IMSession.shared.store.originalOutbox(
                clientID: secondID, namespace: IMSession.shared.namespace)
            do {
                try await IMSession.shared.reconcileOriginalMessage(secondID)
                XCTFail("A pure lookup cannot create the unknown message")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.code, "MESSAGE_RESULT_NOT_FOUND")
            }
            XCTAssertTrue(
                try IMSession.shared.store.originalOutbox(
                    clientID: secondID,
                    namespace: IMSession.shared.namespace) == oldBody)
            try await IMForwardService().retryOriginal(secondID)
            let after = try await api.request(
                "conversations/" + target["id"].text + "/messages", token: c["token"].text)
            XCTAssertEqual(after["items"].list.filter { $0["clientMsgId"].text == secondID }.count, 1)
            await logout([a, b, c], api: api)
        } catch {
            await logout([a, b, c], api: api)
            throw error
        }
    }

    func testChangedSourceRequiresReselectionAndOriginalProofFreezes() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        let restore = install(api, account: a, descriptor: descriptor)
        defer { restore() }
        do {
            try await friend(a, b, api: api)
            let sourceGroup = try await group("Native changing source", owner: b, members: [a], api: api)
            let destination = try await group("Native changing target", owner: a, members: [], api: api)
            let original = try await post(
                "Original selected version", in: sourceGroup, account: b,
                descriptor: descriptor, api: api)
            let proof = try await IMForwardService().prepare(original)
            let edited = try await api.request(
                "messages/" + original["id"].text, method: "PATCH",
                body: [
                    "text": .string("Changed source version"), "expectedVersion": original["version"],
                    "requestId": .string(UUID().uuidString),
                ], token: b["token"].text)
            do {
                _ = try await IMForwardService().prepare(original)
                XCTFail("A changed source needs a new explicit selection")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.code, "FORWARD_SOURCE_RESELECT")
            }
            let clientID = UUID().uuidString
            do {
                try await IMForwardService().send(proof, to: destination["id"].text, clientID: clientID)
                XCTFail("Old source proof cannot create a forward")
            } catch let error as IMAPIError {
                XCTAssertTrue(
                    ["FORWARD_SOURCE_CHANGED", "FORWARD_ADMISSION_CHANGED", "FORWARD_SOURCE_UNAVAILABLE"]
                        .contains(error.code))
            }
            let body = try XCTUnwrap(
                IMSession.shared.store.originalOutbox(
                    clientID: clientID,
                    namespace: IMSession.shared.namespace))
            XCTAssertTrue(body["forwardAdmission"] == proof["forwardAdmission"])
            XCTAssertEqual(body["forwardSourceVersion"], proof["contentVersion"])
            let visible = try XCTUnwrap(
                IMSession.shared.store.list(
                    "messages:" + destination["id"].text,
                    namespace: IMSession.shared.namespace
                ).first { $0["clientMsgId"].text == clientID })
            XCTAssertEqual(visible["freezeReason"].text, "forward_invalid")
            do {
                try await IMForwardService().retryOriginal(clientID)
                XCTFail("Permanent failure must not be manually revived")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.code, "FORWARD_RETRY_FORBIDDEN")
            }
            let current = try await IMForwardService().prepare(edited)
            try await IMForwardService().send(current, to: destination["id"].text)
            await logout([a, b], api: api)
        } catch {
            await logout([a, b], api: api)
            throw error
        }
    }

    func testSourceRejoinHistoryDenialPermanentlyFreezesOriginalForward() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        let restore = install(api, account: a, descriptor: descriptor)
        defer { restore() }
        do {
            try await friend(a, b, api: api)
            let sourceGroup = try await group(
                "Native forward rejoin source", owner: b, members: [a], api: api)
            let target = try await group("Native forward rejoin target", owner: a, members: [], api: api)
            let source = try await post(
                "Old source membership content", in: sourceGroup, account: b,
                descriptor: descriptor, api: api)
            let selected = try await IMForwardService().prepare(source)
            let clientID = UUID().uuidString
            let notSent = IMForwardService(sendRequest: { _, _ in
                throw IMAPIError(status: 0, code: "TEST_NOT_SENT", message: "Outcome unknown")
            })
            do {
                try await notSent.send(selected, to: target["id"].text, clientID: clientID)
                XCTFail("The first POST was deliberately not sent")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.status, 0)
            }
            let original = try IMSession.shared.store.originalOutbox(
                clientID: clientID, namespace: IMSession.shared.namespace)
            let members = try await api.request(
                "conversations/" + sourceGroup["id"].text + "/members", token: b["token"].text)
            let member = try XCTUnwrap(members["items"].list.first { $0["id"] == a["user"]["id"] })
            _ = try await api.request(
                "conversations/" + sourceGroup["id"].text + "/members/" + a["user"]["id"].text
                    + "?membershipGeneration=" + member["membershipGeneration"].text, method: "DELETE",
                token: b["token"].text)
            _ = try await api.request(
                "conversations/" + sourceGroup["id"].text + "/members", method: "POST",
                body: ["userId": a["user"]["id"]], token: b["token"].text)
            do {
                try await IMForwardService().retryOriginal(clientID)
                XCTFail("A new membership period cannot revive the old source proof")
            } catch let error as IMAPIError {
                XCTAssertTrue(
                    [
                        "HISTORY_NOT_ALLOWED", "NOT_A_MEMBER", "FORWARD_SOURCE_UNAVAILABLE",
                        "FORWARD_ADMISSION_CHANGED",
                    ].contains(error.code))
            }
            let row = try XCTUnwrap(
                IMSession.shared.store.list(
                    "messages:" + target["id"].text,
                    namespace: IMSession.shared.namespace
                ).first { $0["clientMsgId"].text == clientID })
            XCTAssertEqual(row["freezeReason"].text, "forward_invalid")
            XCTAssertTrue(
                try IMSession.shared.store.originalOutbox(
                    clientID: clientID,
                    namespace: IMSession.shared.namespace) == original)
            let history = try await api.request(
                "conversations/" + target["id"].text + "/messages", token: a["token"].text)
            XCTAssertTrue(history["items"].list.isEmpty)
            await logout([a, b], api: api)
        } catch {
            await logout([a, b], api: api)
            throw error
        }
    }

    func testForwardAttachmentProjectionOfflineAndRevocationPurge() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        let restore = install(api, account: a, descriptor: descriptor)
        defer { restore() }
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".txt")
        let bytes = Data("Native forwarded attachment bytes".utf8)
        try bytes.write(to: file)
        defer { try? FileManager.default.removeItem(at: file) }
        do {
            try await friend(a, b, api: api)
            let sourceGroup = try await group("Native file source", owner: b, members: [a], api: api)
            let destination = try await group("Native file target", owner: a, members: [], api: api)
            let attachment = try await api.upload(file, mimeType: "text/plain", token: b["token"].text)
            let original = try await post(
                file.lastPathComponent, in: sourceGroup, account: b,
                descriptor: descriptor, api: api, attachment: attachment["id"].text)
            let source = try await IMForwardService().prepare(original)
            let clientID = UUID().uuidString
            try await IMForwardService().send(source, to: destination["id"].text, clientID: clientID)
            let namespace = IMSession.shared.namespace
            let projected = try XCTUnwrap(
                IMSession.shared.store.list(
                    "messages:" + destination["id"].text,
                    namespace: namespace
                ).first { $0["clientMsgId"].text == clientID })
            XCTAssertEqual(projected["attachmentId"], .null)
            XCTAssertEqual(
                projected["attachmentURL"].text, "/api/messages/" + projected["id"].text + "/attachment")
            let local = try await IMMediaCache.file(projected, namespace: namespace)
            XCTAssertEqual(SHA256.hash(data: try Data(contentsOf: local)), SHA256.hash(data: bytes))
            let exported = try IMHistoryExport.data(store: IMSession.shared.store, namespace: namespace)
            let exportText = String(decoding: exported, as: UTF8.self)
            XCTAssertFalse(exportText.contains("forwardAdmission"))
            XCTAssertFalse(exportText.contains(file.lastPathComponent))
            XCTAssertFalse(
                IMMediaCache.permitsFallback(
                    IMAPIError(status: 0, code: "OFFLINE", message: "Offline"), message: projected))
            let certificate = try IMIntegrationFixture.certificate()
            let unreachable = try IMInstanceCertificate(
                address: "https://127.0.0.1:1",
                certificate: certificate.certificateDER, verifiedFingerprint: certificate.fingerprint)
            IMSession.shared.api = IMAPIClient(
                baseURL: URL(string: unreachable.address + "/api/")!, certificate: unreachable)
            do {
                _ = try await IMMediaCache.file(projected, namespace: namespace)
                XCTFail("Forward media cannot fall back offline")
            } catch {}
            XCTAssertFalse(FileManager.default.fileExists(atPath: local.path))
            let redacted = try XCTUnwrap(
                IMSession.shared.store.list(
                    "messages:" + destination["id"].text,
                    namespace: namespace
                ).first { $0["id"] == projected["id"] })
            XCTAssertEqual(redacted["text"], .null)
            XCTAssertEqual(redacted["attachmentURL"], .null)
            IMSession.shared.api = api
            let restored = try await IMForwardService().current(projected)
            XCTAssertEqual(restored["type"].text, "file")
            let localAgain = try await IMMediaCache.file(restored, namespace: namespace)
            let members = try await api.request(
                "conversations/" + sourceGroup["id"].text + "/members", token: b["token"].text)
            let member = try XCTUnwrap(members["items"].list.first { $0["id"] == a["user"]["id"] })
            _ = try await api.request(
                "conversations/" + sourceGroup["id"].text + "/members/" + a["user"]["id"].text
                    + "?membershipGeneration=" + member["membershipGeneration"].text,
                method: "DELETE", token: b["token"].text)
            await IMForwardService().refreshCached(destination["id"].text)
            let unavailable = try XCTUnwrap(
                IMSession.shared.store.list(
                    "messages:" + destination["id"].text,
                    namespace: namespace
                ).first { $0["id"] == projected["id"] })
            XCTAssertEqual(unavailable["forwardUnavailable"], .bool(true))
            XCTAssertTrue(unavailable["text"].text.isEmpty)
            XCTAssertFalse(FileManager.default.fileExists(atPath: localAgain.path))
            await logout([a, b], api: api)
        } catch {
            await logout([a, b], api: api)
            throw error
        }
    }

    func testPrepareNativeForwardUIFixture() async throws {
        let api = try client()
        if let old = UserDefaults.standard.dictionary(forKey: "ui-fixture:forwarding"),
            let oldProfile = (old["instanceProfile"] ?? old["profile"]) as? String
        {
            if let data = old["user:" + oldProfile] as? Data,
                let user = try? JSONDecoder.make().decode(IMJSON.self, from: data),
                let token = IMKeychain.read(oldProfile + ":" + user["id"].text)
            {
                _ = try? await api.request("auth/logout", method: "POST", token: token)
                try IMKeychain.write(nil, key: oldProfile + ":" + user["id"].text)
            }
            if let token = IMKeychain.read("forward-peer:" + oldProfile) {
                _ = try? await api.request("auth/logout", method: "POST", token: token)
                try IMKeychain.write(nil, key: "forward-peer:" + oldProfile)
            }
        }
        let descriptor = try await api.request("bootstrap")
        XCTAssertEqual(descriptor["features"]["forwarding"], .bool(true))
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        try await friend(a, b, api: api)
        let source = try await group("Native forward source", owner: a, members: [b], api: api)
        let destination = try await group("Native forward target", owner: a, members: [b], api: api)
        _ = try await post(
            "Native forwarding selected source", in: source, account: a,
            descriptor: descriptor, api: api)
        let profile = UUID().uuidString
        let accountID = a["user"]["id"].text
        let userData = try JSONEncoder.make().encode(a["user"])
        UserDefaults.standard.set(
            [
                "instanceURL": "https://127.0.0.1:28443", "instanceProfile": profile,
                "instanceDeployment": descriptor["deploymentId"].text,
                "instanceTrustFingerprint": try IMIntegrationFixture.certificate().fingerprint,
                "epoch:" + profile: descriptor["restoreEpoch"].text,
                "user:" + profile: userData,
                "loggedOut:" + profile + ":" + accountID: false,
            ] as [String: Any], forKey: "ui-fixture:forwarding")
        try IMKeychain.write(a["token"].text, key: profile + ":" + accountID)
        try IMKeychain.write(b["token"].text, key: "forward-peer:" + profile)
        UserDefaults.standard.set(destination["id"].text, forKey: "forward-target:" + profile)
        UserDefaults.standard.set(source["id"].text, forKey: "forward-source:" + profile)
    }

    func testVerifyNativeForwardUIAndRecallOnServer() async throws {
        let api = try client()
        let fixture = try XCTUnwrap(UserDefaults.standard.dictionary(forKey: "ui-fixture:forwarding"))
        let profile = try XCTUnwrap(fixture["instanceProfile"] as? String)
        let peerToken = try XCTUnwrap(IMKeychain.read("forward-peer:" + profile))
        defer { try? IMKeychain.write(nil, key: "forward-peer:" + profile) }
        let target = try XCTUnwrap(UserDefaults.standard.string(forKey: "forward-target:" + profile))
        let source = try XCTUnwrap(UserDefaults.standard.string(forKey: "forward-source:" + profile))
        do {
            let history = try await api.request("conversations/" + target + "/messages", token: peerToken)
            let forwarded = history["items"].list.filter { $0["forwarded"] == .bool(true) }
            XCTAssertEqual(forwarded.count, 1)
            XCTAssertEqual(forwarded.first?["deleted"], .bool(true))
            let original = try await api.request("conversations/" + source + "/messages", token: peerToken)
            XCTAssertEqual(original["items"].list.first?["text"].text, "Native forwarding selected source")
            XCTAssertNotEqual(original["items"].list.first?["deleted"], .bool(true))
            _ = try await api.request("auth/logout", method: "POST", token: peerToken)
        } catch {
            _ = try? await api.request("auth/logout", method: "POST", token: peerToken)
            throw error
        }
    }
}
