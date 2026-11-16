//
//  IMMessageListTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMTransport
import UIKit
import XCTest

@testable import JobsIM

@MainActor
final class IMMessageListTests: XCTestCase {

    private func client() throws -> IMAPIClient {
        let original = try IMIntegrationFixture.certificate()
        let certificate = try IMInstanceCertificate(
            address: "https://127.0.0.1:20443", certificate: original.certificateDER,
            verifiedFingerprint: original.fingerprint)
        try IMTrustStore.shared.save(certificate)
        return IMAPIClient(
            baseURL: URL(string: certificate.address + "/api/")!, certificate: certificate)
    }

    private func install(_ api: IMAPIClient, account: IMJSON, descriptor: IMJSON) -> () -> Void {
        let session = IMSession.shared
        let old = (
            session.profile, session.user, session.token, session.demo, session.api, session.restoreEpoch,
            session.changed, session.baseURL, session.deployment
        )
        IMSyncCoordinator.shared.stop()
        session.changed = nil
        session.profile = UUID().uuidString
        session.user = account["user"]
        session.token = account["token"].text
        session.demo = false
        session.api = api
        session.baseURL = URL(string: "https://127.0.0.1:20443")
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

    func testPageRejectsMalformedOrRepeatedIdentities() throws {
        XCTAssertThrowsError(try IMMessagePage(.object(["items": .array([])])))
        XCTAssertThrowsError(
            try IMMessagePage(
                .object([
                    "items": .array([]), "cursor": .string("opaque"), "hasMore": .bool(true),
                ])))
        let item = IMJSON.object(["id": .string("9007199254740993123")])
        XCTAssertThrowsError(
            try IMMessagePage(
                .object([
                    "items": .array([item, item]), "cursor": .string("opaque"), "hasMore": .bool(false),
                ])))
        let page = try IMMessagePage(
            .object([
                "items": .array([item]), "cursor": .string("opaque.signed"), "hasMore": .bool(false),
            ]))
        XCTAssertEqual(page.items.first?["id"].text, "9007199254740993123")
        XCTAssertEqual(page.cursor, "opaque.signed")
    }

    func testInputPlaceholderContrastInBothThemes() throws {
        let field = IMUI.field("Readable placeholder")
        let attributes = try XCTUnwrap(field.attributedPlaceholder?.attributes(at: 0, effectiveRange: nil))
        let foreground = try XCTUnwrap(attributes[.foregroundColor] as? UIColor)
        func luminance(_ color: UIColor) -> Double {
            var red: CGFloat = 0
            var green: CGFloat = 0
            var blue: CGFloat = 0
            var alpha: CGFloat = 0
            XCTAssertTrue(color.getRed(&red, green: &green, blue: &blue, alpha: &alpha))
            XCTAssertEqual(alpha, 1, accuracy: 0.001)
            let values = [red, green, blue].map { value -> Double in
                let component = Double(value)
                return component <= 0.04045 ? component / 12.92 : pow((component + 0.055) / 1.055, 2.4)
            }
            return 0.2126 * values[0] + 0.7152 * values[1] + 0.0722 * values[2]
        }
        for style in [UIUserInterfaceStyle.light, .dark] {
            let traits = UITraitCollection(userInterfaceStyle: style)
            let a = luminance(foreground.resolvedColor(with: traits))
            let b = luminance(IMUI.panel.resolvedColor(with: traits))
            XCTAssertGreaterThanOrEqual((max(a, b) + 0.05) / (min(a, b) + 0.05), 4.5)
        }
    }

    func testHTTPSFavoritesPaginationAndCurrentSourceACL() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        let at = a["token"].text
        let bt = b["token"].text
        let restore = install(api, account: a, descriptor: descriptor)
        defer { restore() }
        do {
            let request = try await api.request(
                "contacts/requests", method: "POST",
                body: ["userId": b["user"]["id"], "requestId": .string(UUID().uuidString)], token: at)
            _ = try await api.request(
                "contacts/requests/" + request["id"].text + "/accept", method: "POST", token: bt)
            let conversation = try await api.request(
                "conversations", method: "POST",
                body: [
                    "kind": .string("group"), "title": .string("Native list source"),
                    "memberIds": .array([b["user"]["id"]]),
                ], token: at)
            var originals: [IMJSON] = []
            for number in 0..<3 {
                let message = try await api.request(
                    "conversations/" + conversation["id"].text + "/messages", method: "POST",
                    body: [
                        "clientMsgId": .string(UUID().uuidString), "type": .string("text"),
                        "text": .string("Native source " + String(number)),
                        "restoreEpoch": descriptor["restoreEpoch"],
                        "sendAdmission": conversation["sendAdmission"],
                    ], token: at)
                originals.append(message)
                _ = try await api.request(
                    "messages/" + message["id"].text + "/favorite", method: "PUT", token: at)
            }
            let service = IMMessageListService()
            let first = try await service.page(favorites: true, limit: 2)
            XCTAssertEqual(first.items.count, 2)
            XCTAssertTrue(first.hasMore)
            let second = try await service.page(favorites: true, cursor: first.cursor, limit: 2)
            XCTAssertEqual(second.items.count, 1)
            XCTAssertFalse(second.hasMore)
            XCTAssertEqual(Set((first.items + second.items).map { $0["id"].text }).count, 3)
            let located = try await service.locate(originals[0])
            XCTAssertEqual(located.message["id"], originals[0]["id"])
            XCTAssertEqual(located.conversation["kind"].text, "group")
            let source = originals[0]
            _ = try await api.request(
                "messages/" + source["id"].text + "/favorite", method: "PUT", token: bt)
            let members = try await api.request(
                "conversations/" + conversation["id"].text + "/members", token: at)
            let generation = try XCTUnwrap(
                members["items"].list.first { $0["id"] == b["user"]["id"] }?["membershipGeneration"].text)
            _ = try await api.request(
                "conversations/" + conversation["id"].text + "/members/" + b["user"]["id"].text
                    + "?membershipGeneration=" + generation, method: "DELETE", token: at)
            IMSession.shared.user = b["user"]
            IMSession.shared.token = bt
            let inaccessible = try await service.page(favorites: true)
            XCTAssertTrue(inaccessible.items.isEmpty)
            do {
                _ = try await service.locate(source)
                XCTFail("A removed member located a former cloud source")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.status, 403)
            }
        } catch {
            _ = try? await api.request("auth/logout", method: "POST", token: at)
            _ = try? await api.request("auth/logout", method: "POST", token: bt)
            throw error
        }
        _ = try? await api.request("auth/logout", method: "POST", token: at)
        _ = try? await api.request("auth/logout", method: "POST", token: bt)
    }

    func testFrozenOriginalLookupCannotCreateWithStillValidAdmission() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let account = try await IMIntegrationFixture.register(api)
        let token = account["token"].text
        let restore = install(api, account: account, descriptor: descriptor)
        defer { restore() }
        do {
            let conversation = try await api.request(
                "conversations", method: "POST", body: ["kind": .string("self")], token: token)
            let id = UUID().uuidString
            let wire: [String: IMJSON] = [
                "clientMsgId": .string(id), "type": .string("text"),
                "text": .string("Frozen before a revocation request could reach server"),
                "restoreEpoch": descriptor["restoreEpoch"], "sendAdmission": conversation["sendAdmission"],
            ]
            let original = IMJSON.object(wire).setting("id", .string(id)).setting(
                "conversationId", conversation["id"]
            ).setting("status", .string("queued"))
            let session = IMSession.shared
            let namespace = session.namespace
            try session.store.enqueue(
                original, conversation: conversation["id"].text, namespace: namespace)
            try session.store.freezeOutbox(
                clientID: id, conversation: conversation["id"].text, namespace: namespace,
                reason: "membership_changed")
            do {
                try await session.reconcileOriginalMessage(id)
                XCTFail("Read-only lookup created a missing message")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.status, 404)
                XCTAssertEqual(error.code, "MESSAGE_RESULT_NOT_FOUND")
            }
            let history = try await api.request(
                "conversations/" + conversation["id"].text + "/messages?after=0", token: token)
            XCTAssertFalse(history["items"].list.contains { $0["clientMsgId"].text == id })
            XCTAssertEqual(try session.store.originalOutbox(clientID: id, namespace: namespace), original)
            let accepted = try await api.request(
                "conversations/" + conversation["id"].text + "/messages", method: "POST", body: wire,
                token: token)
            try await session.reconcileOriginalMessage(id)
            XCTAssertNil(try session.store.originalOutbox(clientID: id, namespace: namespace))
            XCTAssertTrue(
                try session.store.list("messages:" + conversation["id"].text, namespace: namespace).contains {
                    $0["id"] == accepted["id"]
                })
        } catch {
            _ = try? await api.request("auth/logout", method: "POST", token: token)
            throw error
        }
        _ = try? await api.request("auth/logout", method: "POST", token: token)
    }

    func testPrepareSavedSearchUIFixture() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let account = try await IMIntegrationFixture.register(api)
        let token = account["token"].text
        let suffix = String(UUID().uuidString.prefix(8))
        let conversation = try await api.request(
            "conversations", method: "POST",
            body: ["kind": .string("group"), "title": .string("Native paged source " + suffix)],
            token: token)
        let sourceText = "Native selected source " + suffix
        var selected: IMJSON = .null
        for number in 0..<55 {
            let message = try await api.request(
                "conversations/" + conversation["id"].text + "/messages", method: "POST",
                body: [
                    "clientMsgId": .string(UUID().uuidString), "type": .string("text"),
                    "text": .string(number == 0 ? sourceText : "Native saved note " + String(number)),
                    "restoreEpoch": descriptor["restoreEpoch"],
                    "sendAdmission": conversation["sendAdmission"],
                ], token: token)
            _ = try await api.request(
                "messages/" + message["id"].text + "/favorite", method: "PUT", token: token)
            if number == 0 { selected = message }
        }
        for number in 0..<101 {
            _ = try await api.request(
                "conversations", method: "POST",
                body: ["kind": .string("group"), "title": .string("Other native scope " + String(number))],
                token: token)
        }
        let profile = UUID().uuidString
        try IMKeychain.write(token, key: profile + ":" + account["user"]["id"].text)
        let snapshot: [String: Any] = [
            "instanceURL": "https://127.0.0.1:20443", "instanceProfile": profile,
            "instanceDeployment": descriptor["deploymentId"].text,
            "instanceTrustFingerprint": try IMIntegrationFixture.certificate().fingerprint,
            "user:" + profile: try JSONEncoder.make().encode(account["user"]),
            "epoch:" + profile: descriptor["restoreEpoch"].text,
        ]
        UserDefaults.standard.set(snapshot, forKey: "ui-fixture:saved-search")
        UserDefaults.standard.set(sourceText, forKey: "ui-source-text")
        UserDefaults.standard.set(selected["id"].text, forKey: "ui-source-id")
        XCTAssertFalse(selected["id"].text.isEmpty)
    }

    func testStructuredMentionIntentAndRoleRejectionRemainFrozen() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        let at = a["token"].text
        let bt = b["token"].text
        let restore = install(api, account: a, descriptor: descriptor)
        defer { restore() }
        do {
            let request = try await api.request(
                "contacts/requests", method: "POST",
                body: ["userId": b["user"]["id"], "requestId": .string(UUID().uuidString)], token: at)
            _ = try await api.request(
                "contacts/requests/" + request["id"].text + "/accept", method: "POST", token: bt)
            let group = try await api.request(
                "conversations", method: "POST",
                body: [
                    "kind": .string("group"), "title": .string("Native structured mention scope"),
                    "memberIds": .array([b["user"]["id"]]),
                ], token: at)
            let conversation = group["id"].text
            let session = IMSession.shared
            try session.store.save([group], collection: "conversations", namespace: session.namespace)
            let id = UUID().uuidString
            try await session.send(
                "Structured native mention", conversation: conversation, clientID: id,
                mentionedUserIDs: [b["user"]["id"].text])
            let history = try await api.request(
                "conversations/" + conversation + "/messages?after=0", token: bt)
            let selected = try XCTUnwrap(history["items"].list.first { $0["clientMsgId"].text == id })
            XCTAssertEqual(selected["mentionedUserIds"].list.map { $0.text }, [b["user"]["id"].text])
            XCTAssertEqual(selected["mentionedMe"], .bool(true))
            let plainID = UUID().uuidString
            try await session.send("@all is plain user text", conversation: conversation, clientID: plainID)
            let plainHistory = try await api.request(
                "conversations/" + conversation + "/messages?after=0", token: bt)
            let plain = try XCTUnwrap(plainHistory["items"].list.first { $0["clientMsgId"].text == plainID })
            XCTAssertNotEqual(plain["mentionedMe"], .bool(true))
            _ = try await api.request(
                "conversations/" + conversation, method: "PATCH", body: ["ownerId": b["user"]["id"]],
                token: at)
            _ = try await IMMessageListService().conversation(conversation)
            let forbiddenID = UUID().uuidString
            do {
                try await session.send(
                    "Original all-mention without current role", conversation: conversation,
                    clientID: forbiddenID, mentionAll: true)
                XCTFail("A former owner mentioned all")
            } catch let error as IMAPIError { XCTAssertEqual(error.code, "MENTION_ALL_FORBIDDEN") }
            let original = try XCTUnwrap(
                session.store.originalOutbox(clientID: forbiddenID, namespace: session.namespace))
            let local = try session.store.list("messages:" + conversation, namespace: session.namespace).first
            { $0["clientMsgId"].text == forbiddenID }
            XCTAssertEqual(local?["status"].text, "frozen")
            _ = try await api.request(
                "conversations/" + conversation, method: "PATCH", body: ["ownerId": a["user"]["id"]],
                token: bt)
            _ = try await IMMessageListService().conversation(conversation)
            await session.flushPending()
            XCTAssertEqual(
                try session.store.originalOutbox(clientID: forbiddenID, namespace: session.namespace),
                original)
            let current = try await api.request(
                "conversations/" + conversation + "/messages?after=0", token: bt)
            XCTAssertFalse(current["items"].list.contains { $0["clientMsgId"].text == forbiddenID })
        } catch {
            _ = try? await api.request("auth/logout", method: "POST", token: at)
            _ = try? await api.request("auth/logout", method: "POST", token: bt)
            throw error
        }
        _ = try? await api.request("auth/logout", method: "POST", token: at)
        _ = try? await api.request("auth/logout", method: "POST", token: bt)
    }

    func testAnnouncementLostResultUsesOnlyOriginalLookupAndContentVersion() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let account = try await IMIntegrationFixture.register(api)
        let token = account["token"].text
        let restore = install(api, account: account, descriptor: descriptor)
        defer { restore() }
        do {
            let group = try await api.request(
                "conversations", method: "POST",
                body: ["kind": .string("group"), "title": .string("Native announcement protocol")],
                token: token)
            let conversation = group["id"].text
            let source = try await api.request(
                "conversations/" + conversation + "/messages", method: "POST",
                body: [
                    "clientMsgId": .string(UUID().uuidString), "type": .string("text"),
                    "text": .string("Authoritative native announcement source"),
                    "restoreEpoch": descriptor["restoreEpoch"], "sendAdmission": group["sendAdmission"],
                ], token: token)
            var puts = 0
            let writer = IMAnnouncementService(request: { path, method, body in
                if method == "PUT" {
                    puts += 1
                    let pending = try XCTUnwrap(IMAnnouncementService().pending(conversation))
                    XCTAssertEqual(pending["payload"], .object(body))
                    XCTAssertEqual(body["roleGeneration"], group["roleGeneration"])
                    _ = try await api.request(path, method: method, body: body, token: token)
                    throw IMAPIError(
                        status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled response loss")
                }
                return try await api.request(path, method: method, body: body, token: token)
            })
            do {
                _ = try await writer.publish(source)
                XCTFail("Controlled missing response was treated as confirmed")
            } catch let error as IMAPIError { XCTAssertEqual(error.code, "TRANSPORT_UNKNOWN") }
            let operation = try XCTUnwrap(writer.pending(conversation))
            let resumed = IMAnnouncementService()
            let original = try await resumed.checkOriginal(conversation)
            XCTAssertEqual(original["operationId"], operation["id"])
            XCTAssertEqual(puts, 1)
            var live = try await resumed.load(conversation)
            XCTAssertEqual(live["version"].text, "1")
            XCTAssertEqual(live["state"].text, "active")
            _ = try await api.request(
                "messages/" + source["id"].text + "/favorite", method: "PUT", token: token)
            live = try await resumed.load(conversation)
            XCTAssertEqual(live["state"].text, "active")
            let current = try await api.request("messages/" + source["id"].text, token: token)
            _ = try await api.request(
                "messages/" + source["id"].text, method: "PATCH",
                body: [
                    "text": .string("New source content"), "expectedVersion": current["version"],
                    "requestId": .string(UUID().uuidString),
                ], token: token)
            live = try await resumed.load(conversation)
            XCTAssertEqual(live["state"].text, "unavailable")
            XCTAssertTrue(live["message"].fields.isEmpty)
            _ = try await resumed.clear(conversation, expectedVersion: live["version"].text)
            let empty = try await resumed.load(conversation)
            XCTAssertEqual(empty["state"].text, "empty")
            let record = try XCTUnwrap(
                IMSession.shared.store.list(
                    "announcement-operations:" + conversation, namespace: IMSession.shared.namespace
                ).first)
            XCTAssertNil(record["payload"].fields["text"])
            XCTAssertNil(record.fields["message"])
        } catch {
            _ = try? await api.request("auth/logout", method: "POST", token: token)
            throw error
        }
        _ = try? await api.request("auth/logout", method: "POST", token: token)
    }

    func testPrepareMentionAnnouncementUIFixture() async throws {
        let api = try client()
        let descriptor = try await api.request("bootstrap")
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        let at = a["token"].text
        let bt = b["token"].text
        _ = try await api.request(
            "me", method: "PATCH", body: ["displayName": .string("Native mention peer")], token: bt)
        let request = try await api.request(
            "contacts/requests", method: "POST",
            body: ["userId": b["user"]["id"], "requestId": .string(UUID().uuidString)], token: at)
        _ = try await api.request(
            "contacts/requests/" + request["id"].text + "/accept", method: "POST", token: bt)
        let group = try await api.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("group"), "title": .string("Native interactions"),
                "memberIds": .array([b["user"]["id"]]),
            ], token: at)
        let source = try await api.request(
            "conversations/" + group["id"].text + "/messages", method: "POST",
            body: [
                "clientMsgId": .string(UUID().uuidString), "type": .string("text"),
                "text": .string("Native announcement source"), "restoreEpoch": descriptor["restoreEpoch"],
                "sendAdmission": group["sendAdmission"],
            ], token: at)
        let restore = install(api, account: a, descriptor: descriptor)
        defer { restore() }
        let session = IMSession.shared
        let writer = IMAnnouncementService(request: { path, method, body in
            let result = try await api.request(path, method: method, body: body, token: at)
            if method == "PUT" {
                throw IMAPIError(
                    status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled native fixture response loss")
            }
            return result
        })
        do {
            _ = try await writer.publish(source)
            XCTFail("Expected response loss")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "TRANSPORT_UNKNOWN") }
        let profile = session.profile
        try IMKeychain.write(at, key: profile + ":" + a["user"]["id"].text)
        try IMKeychain.write(bt, key: "interaction-peer:" + profile)
        UserDefaults.standard.set(
            [
                "instanceURL": "https://127.0.0.1:20443", "instanceProfile": profile,
                "instanceDeployment": descriptor["deploymentId"].text,
                "instanceTrustFingerprint": try IMIntegrationFixture.certificate().fingerprint,
                "user:" + profile: try JSONEncoder.make().encode(a["user"]),
                "epoch:" + profile: descriptor["restoreEpoch"].text,
            ] as [String: Any], forKey: "ui-fixture:interactions")
        UserDefaults.standard.set(profile, forKey: "ui-interaction-profile")
        UserDefaults.standard.set(group["id"].text, forKey: "ui-interaction-conversation")
        XCTAssertNotNil(try writer.pending(group["id"].text))
    }

    func testVerifyNativeInteractionUIAcceptedOnServer() async throws {
        let profile = try XCTUnwrap(UserDefaults.standard.string(forKey: "ui-interaction-profile"))
        let conversation = try XCTUnwrap(UserDefaults.standard.string(forKey: "ui-interaction-conversation"))
        let token = try XCTUnwrap(IMKeychain.read("interaction-peer:" + profile))
        let api = try client()
        do {
            let history = try await api.request(
                "conversations/" + conversation + "/messages?after=0", token: token)
            let selected = try XCTUnwrap(
                history["items"].list.first { $0["text"].text == "Native structured UI mention" })
            let all = try XCTUnwrap(
                history["items"].list.first { $0["text"].text == "Native all UI mention" })
            XCTAssertEqual(selected["mentionedMe"], .bool(true))
            XCTAssertEqual(all["mentionedMe"], .bool(true))
            XCTAssertEqual(all["mentionAll"], .bool(true))
            let announcement = try await api.request(
                "conversations/" + conversation + "/announcement", token: token)
            XCTAssertEqual(announcement["state"].text, "empty")
            XCTAssertEqual(announcement["version"].text, "3")
        } catch {
            _ = try? await api.request("auth/logout", method: "POST", token: token)
            try? IMKeychain.write(nil, key: "interaction-peer:" + profile)
            throw error
        }
        _ = try? await api.request("auth/logout", method: "POST", token: token)
        try IMKeychain.write(nil, key: "interaction-peer:" + profile)
    }
}
