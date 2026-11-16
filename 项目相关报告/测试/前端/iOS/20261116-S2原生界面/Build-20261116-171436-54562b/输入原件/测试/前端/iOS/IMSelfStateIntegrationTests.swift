//
//  IMSelfStateIntegrationTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport
import XCTest

@testable import JobsIM

@MainActor
final class IMSelfStateIntegrationTests: XCTestCase {

    private var runID: String {
        return ProcessInfo.processInfo.environment["JOBS_IM_SELF_STATE_RUN"] ?? ""
    }

    private var journalKey: String {
        return "s2-native-journal:" + runID
    }

    private func publicConfig() throws -> IMJSON {
        guard UUID(uuidString: runID) != nil else {
            throw IMSession.SessionError.invalidInstance
        }
        let url = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "SelfStatePublic", withExtension: "json"))
        return try JSONDecoder.make().decode(IMJSON.self, from: Data(contentsOf: url))
    }

    private func journal(_ session: IMSession? = nil, username: String, requestID: String = "", closed: Bool = false) {
        var rows = UserDefaults.standard.array(forKey: journalKey) as? [[String: String]] ?? []
        let index = rows.firstIndex { $0["username"] == username } ?? rows.count
        var row = index < rows.count ? rows[index] : ["username": username, "runID": runID]
        if !requestID.isEmpty {
            row["requestID"] = requestID
        }
        if let session, let pair = session.currentCredentialPair {
            row["userID"] = pair.userID
            row["profile"] = pair.profile
            row["familyID"] = pair.familyID
            row["deviceID"] = pair.deviceID
            row["epoch"] = pair.restoreEpoch
            row["deployment"] = pair.deploymentID
            row["accessExpiresAt"] = pair.accessExpiresAt
        }
        row["closed"] = closed ? "true" : "false"
        if index < rows.count {
            rows[index] = row
        } else {
            rows.append(row)
        }
        UserDefaults.standard.set(rows, forKey: journalKey)
    }

    private func configured(path: String = ":memory:") async throws -> IMSession {
        // The host starts with restoration isolated; only this explicit real suite enables its own HTTP.
        setenv("JOBS_IM_SYNC_INJECTED_TESTS", "0", 1)
        let config = try publicConfig()
        let certificate = try IMInstanceCertificate(address: config["url"].text,
                                                     certificate: Data(config["caPEM"].text.utf8),
                                                     verifiedFingerprint: config["fingerprint"].text)
        let address = try XCTUnwrap(URL(string: config["url"].text))
        let session = IMSession(store: try IMLocalStore(path: path))
        session.api = IMAPIClient(baseURL: IMSession.apiURL(address), certificate: certificate)
        session.baseURL = address
        session.profile = "s2-ios-" + UUID().uuidString.lowercased()
        session.deployment = config["deploymentId"].text
        session.restoreEpoch = config["restoreEpoch"].text
        let bootstrap = try await session.api!.request("bootstrap")
        guard bootstrap["deploymentId"] == config["deploymentId"], bootstrap["restoreEpoch"] == config["restoreEpoch"],
            ["auth_refresh", "authoritative_sync", "sync_jetstream", "sync_self_state", "cloud_drafts"].allSatisfy({ bootstrap["features"][$0] == .bool(true) })
        else {
            throw IMSession.SessionError.invalidInstance
        }
        try IMTrustStore.shared.save(certificate)
        return session
    }

    private func register(path: String = ":memory:") async throws -> IMSession {
        let session = try await configured(path: path)
        let username = "s2_ios_" + UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(16).lowercased()
        let request = UUID().uuidString.lowercased()
        journal(username: username, requestID: request)
        _ = try await session.authenticate(mode: "register", username: username, password: UUID().uuidString + UUID().uuidString,
                                            requestID: request)
        let pair = try XCTUnwrap(session.currentCredentialPair)
        journal(session, username: username)
        try IMKeychain.write(pair.accessToken, key: "s2-control:" + runID + ":" + session.profile)
        return session
    }

    private func close(_ session: IMSession) async throws {
        let pair = try XCTUnwrap(session.currentCredentialPair)
        let api = try XCTUnwrap(session.api)
        _ = try await api.request("auth/logout", method: "POST", token: pair.accessToken)
        do {
            _ = try await api.request("me", token: pair.accessToken)
            XCTFail("Owned native family must be revoked")
            throw IMSession.SessionError.noInstance
        } catch let error as IMAPIError {
            guard error.status == 401 else {
                throw error
            }
        }
        journal(session, username: pair.username, closed: true)
        try IMKeychain.write(nil, key: pair.profile + ":" + pair.userID)
        try IMKeychain.write(nil, key: "s2-control:" + runID + ":" + pair.profile)
        session.stopAuthoritativeSync()
    }

    private func group(_ alice: IMSession, _ bob: IMSession, title: String) async throws -> IMJSON {
        let request = try await alice.action("contacts/requests", body: ["userId": bob.user["id"], "requestId": .string(UUID().uuidString)])
        _ = try await bob.action("contacts/requests/" + request["id"].text + "/accept")
        return try await alice.action("conversations", body: ["kind": .string("group"), "title": .string(title),
                                                              "memberIds": .array([bob.user["id"]]), "requestId": .string(UUID().uuidString)])
    }

    private func send(_ session: IMSession, conversation: String, text: String) async throws -> IMJSON {
        let destination = try await session.action("conversations/" + conversation, method: "GET")
        return try await session.action("conversations/" + conversation + "/messages", body: ["clientMsgId": .string(UUID().uuidString),
            "type": .string("text"), "text": .string(text), "restoreEpoch": .string(session.restoreEpoch), "sendAdmission": destination["sendAdmission"]])
    }

    func testRealSchemaTwoReadViewAndFontKeepOriginalOutbox() async throws {
        let alice = try await register()
        let bob: IMSession
        do {
            bob = try await register()
        } catch {
            try await close(alice)
            throw error
        }
        do {
            let conversation = try await group(alice, bob, title: "iOS S2 real source")
            let id = conversation["id"].text
            let first = try await send(bob, conversation: id, text: "Actually observed native source")
            _ = try await send(bob, conversation: id, text: "Native unobserved hole")
            let pending: IMJSON = .object(["id": .string(UUID().uuidString), "clientMsgId": first["clientMsgId"], "conversationId": .string(id),
                "senderId": alice.user["id"], "type": .string("text"), "text": .string("Distinct original local intent"),
                "sendAdmission": conversation["sendAdmission"], "restoreEpoch": .string(alice.restoreEpoch), "status": .string("queued")])
            try alice.store.enqueue(pending, conversation: id, namespace: alice.namespace)
            await alice.synchronizeAuthoritatively()
            let control = try XCTUnwrap(alice.store.syncStateIfPresent(namespace: alice.namespace))
            XCTAssertEqual(control["schemaVersion"], .string("2"))
            XCTAssertTrue(control["restriction"].text.isEmpty)
            XCTAssertFalse(control["cursor"].text.isEmpty)
            XCTAssertTrue(try alice.store.pending(namespace: alice.namespace).contains { $0["clientMsgId"] == first["clientMsgId"] })
            alice.activeConversation = id
            await alice.observeVisibleMessages([first["id"].text], conversation: id, membershipGeneration: conversation["membershipGeneration"].text)
            let read = try await alice.action("conversations/" + id + "/read", method: "GET")
            XCTAssertEqual(read["unreadCount"], .string("1"))
            let personal = IMPersonalStateService(session: alice)
            let view = try await personal.read("view", conversation: id)
            _ = try await personal.changeView(["manualUnread": .bool(true), "archived": .bool(true), "pinnedRank": .number(0)], conversation: id, expected: view)
            let current = try await personal.read("view", conversation: id)
            XCTAssertEqual(current["manualUnread"], .bool(true))
            XCTAssertEqual(current["archived"], .bool(true))
            try await alice.setFontPreference("large")
            let font = try await alice.action("me/preferences", method: "GET")
            XCTAssertEqual(font["fontPreset"], .string("large"))
            XCTAssertTrue(try alice.store.pending(namespace: alice.namespace).contains { $0["text"] == pending["text"] })
            try await close(bob)
            try await close(alice)
        } catch {
            try? await close(bob)
            try? await close(alice)
            throw error
        }
    }

    func testRealDraftLostDeliveryReopensOriginalAndRetiredKeepsOwnedText() async throws {
        let folder = FileManager.default.temporaryDirectory.appendingPathComponent("real-self-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        defer {
            try? FileManager.default.removeItem(at: folder)
        }
        let alice = try await register(path: folder.appendingPathComponent("history.sqlite").path)
        let bob: IMSession
        do {
            bob = try await register()
        } catch {
            try await close(alice)
            throw error
        }
        do {
            let conversation = try await group(alice, bob, title: "iOS cloud draft loss")
            let id = conversation["id"].text
            await alice.synchronizeAuthoritatively()
            let personal = IMPersonalStateService(session: alice)
            let current = try await personal.read("draft", conversation: id)
            let request = UUID().uuidString.lowercased()
            let body: IMJSON = .object(["requestId": .string(request), "revisionEpoch": .string(alice.restoreEpoch),
                "expectedVersion": current["version"], "draftAdmission": current["draftAdmission"], "ownedText": .string("Cold independent native text"), "reply": .null])
            let original: IMJSON = .object(["id": .string(request), "path": .string("conversations/" + id + "/draft"), "method": .string("PUT"),
                "conversationId": .string(id), "familyId": .string(try XCTUnwrap(alice.currentCredentialPair).familyID), "body": body, "state": .string("pending")])
            try alice.store.save([original], collection: "self-state-operations", namespace: alice.namespace)
            // Seed a real committed operation then deliberately discard its delivery; no server behavior is mocked.
            _ = try await alice.action(original["path"].text, method: "PUT", body: body.fields)
            let reopened = IMSession(store: try IMLocalStore(path: folder.appendingPathComponent("history.sqlite").path))
            reopened.profile = alice.profile
            reopened.baseURL = alice.baseURL
            reopened.api = alice.api
            reopened.deployment = alice.deployment
            reopened.restoreEpoch = alice.restoreEpoch
            reopened.user = alice.user
            reopened.credentialPair = alice.currentCredentialPair
            reopened.token = alice.token
            let resumed = IMPersonalStateService(session: reopened)
            XCTAssertTrue(try resumed.operations(conversation: id).first?["body"] == body)
            _ = try await resumed.resume(original, retry: false)
            let recovered = try await resumed.read("draft", conversation: id)
            XCTAssertEqual(recovered["ownedText"], body["ownedText"])
            let source = try await send(bob, conversation: id, text: "Original native reply source")
            let proof = try await alice.action("messages/" + source["id"].text, method: "GET")
            let seen = try await resumed.read("draft", conversation: id)
            let retiredID = UUID().uuidString.lowercased()
            let retiredBody: IMJSON = .object(["requestId": .string(retiredID), "revisionEpoch": .string(alice.restoreEpoch),
                "expectedVersion": seen["version"], "draftAdmission": seen["draftAdmission"], "ownedText": .string("Retired independent text survives"),
                "reply": .object(["messageId": source["id"], "sourceVersion": proof["contentVersion"], "replyAdmission": proof["replyAdmission"]])])
            let retired = original.setting("id", .string(retiredID)).setting("body", retiredBody)
            try reopened.store.save([retired], collection: "self-state-operations", namespace: reopened.namespace)
            _ = try await bob.action("messages/" + source["id"].text, method: "PATCH", body: ["text": .string("Source truly edited"), "expectedVersion": source["version"]])
            let result = try await alice.action(original["path"].text, method: "PUT", body: retiredBody.fields)
            XCTAssertEqual(result["status"], .string("retired"))
            _ = try await resumed.resume(retired, retry: false)
            let recovery = try await resumed.read("draft", conversation: id)
            XCTAssertEqual(recovery["recovery"]["ownedText"], retiredBody["ownedText"])
            let adopted = try await resumed.textForAdoption(conversation: id, expected: recovery, recovery: true, includeReply: false)
            XCTAssertEqual(adopted.0, retiredBody["ownedText"].text)
            XCTAssertEqual(adopted.1, .null)
            try await close(bob)
            try await close(alice)
        } catch {
            try? await close(bob)
            try? await close(alice)
            throw error
        }
    }

    func testPrepareNativeUIFixture() async throws {
        let keys = ["instanceURL", "instanceProfile", "instanceDeployment", "instanceTrustFingerprint", "language", "theme", "fontScale"]
        var previous: [String: Any] = [:]
        for key in keys {
            previous[key] = UserDefaults.standard.object(forKey: key)
        }
        UserDefaults.standard.set(previous, forKey: "s2-ui-previous:" + runID)
        let directory = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("JobsIM")
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let alice = try await register(path: directory.appendingPathComponent("history.sqlite").path)
        let bob: IMSession
        do {
            bob = try await register()
        } catch {
            try await close(alice)
            throw error
        }
        do {
            let conversation = try await group(alice, bob, title: "Native S2 controls")
            _ = try await send(bob, conversation: conversation["id"].text, text: "Visible iOS S2 source")
            await alice.synchronizeAuthoritatively()
            XCTAssertTrue(alice.personalStatesEnabled)
            guard alice.personalStatesEnabled else {
                throw IMSession.SessionError.noInstance
            }
            let config = try publicConfig()
            let snapshot: [String: Any] = ["instanceURL": config["url"].text, "instanceProfile": alice.profile,
                "instanceDeployment": alice.deployment, "instanceTrustFingerprint": config["fingerprint"].text,
                "user:" + alice.profile: try JSONEncoder.make().encode(alice.user), "epoch:" + alice.profile: alice.restoreEpoch,
                "language": "en", "theme": "light", "fontScale": 0]
            UserDefaults.standard.set(snapshot, forKey: "ui-fixture:s2-" + runID)
            UserDefaults.standard.set(alice.profile, forKey: "s2-ui-profile:" + runID)
            UserDefaults.standard.set(conversation["id"].text, forKey: "s2-ui-conversation:" + runID)
            try await close(bob)
            // Alice intentionally remains live only for the following native UI phase; AT stays in Keychain.
        } catch {
            try? await close(bob)
            try? await close(alice)
            throw error
        }
    }

    func testVerifyNativeUIFixtureClosed() async throws {
        let config = try publicConfig()
        let rows = UserDefaults.standard.array(forKey: journalKey) as? [[String: String]] ?? []
        let profile = try XCTUnwrap(UserDefaults.standard.string(forKey: "s2-ui-profile:" + runID))
        let row = try XCTUnwrap(rows.first { $0["profile"] == profile })
        let token = try XCTUnwrap(IMKeychain.readValidated("s2-control:" + runID + ":" + profile))
        let session = try await configured()
        let api = try XCTUnwrap(session.api)
        let conversation = try XCTUnwrap(UserDefaults.standard.string(forKey: "s2-ui-conversation:" + runID))
        var verificationFailure: Error?
        do {
            _ = try await api.request("me", token: token)
            let view = try await api.request("conversations/" + conversation + "/view", token: token)
            let preferences = try await api.request("me/preferences", token: token)
            let draft = try await api.request("conversations/" + conversation + "/draft", token: token)
            XCTAssertEqual(view["manualUnread"], .bool(true))
            XCTAssertEqual(view["hidden"], .bool(false))
            XCTAssertEqual(view["archived"], .bool(false))
            XCTAssertEqual(view["pinnedRank"], .number(0))
            XCTAssertNotEqual(view["mutedUntil"], .null)
            XCTAssertEqual(preferences["fontPreset"], .string("large"))
            XCTAssertEqual(draft["ownedText"], .string("Independent iOS cloud draft"))
        } catch {
            verificationFailure = error
        }
        do {
            _ = try await api.request("auth/logout", method: "POST", token: token)
        } catch let error as IMAPIError where error.status == 401 {
            let expiry = try XCTUnwrap(IMSyncProtocol.date(row["accessExpiresAt"] ?? ""))
            XCTAssertTrue(expiry > Date(), "Do not confuse natural access expiry with family closure")
        }
        do {
            _ = try await api.request("me", token: token)
            XCTFail("UI family remains live")
            throw IMSession.SessionError.noInstance
        } catch let error as IMAPIError {
            guard error.status == 401 else {
                throw error
            }
        }
        journal(username: row["username"] ?? "", closed: true)
        try IMKeychain.write(nil, key: "s2-control:" + runID + ":" + profile)
        try IMKeychain.write(nil, key: profile + ":" + (row["userID"] ?? ""))
        UserDefaults.standard.removeObject(forKey: "ui-fixture:s2-" + runID)
        let previous = UserDefaults.standard.dictionary(forKey: "s2-ui-previous:" + runID) ?? [:]
        for key in ["instanceURL", "instanceProfile", "instanceDeployment", "instanceTrustFingerprint", "language", "theme", "fontScale"] {
            if let value = previous[key] {
                UserDefaults.standard.set(value, forKey: key)
            } else {
                UserDefaults.standard.removeObject(forKey: key)
            }
        }
        let final = UserDefaults.standard.array(forKey: journalKey) as? [[String: String]] ?? []
        XCTAssertEqual(final.count, 6)
        XCTAssertTrue(final.allSatisfy { $0["closed"] == "true" })
        guard final.count == 6, final.allSatisfy({ $0["closed"] == "true" }) else {
            throw IMSession.SessionError.noInstance
        }
        let evidence = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("NativeSelfStateEvidence")
        try FileManager.default.createDirectory(at: evidence, withIntermediateDirectories: true)
        try JSONSerialization.data(withJSONObject: final, options: [.sortedKeys])
            .write(to: evidence.appendingPathComponent(runID + ".json"), options: [.atomic, .completeFileProtection])
        XCTAssertEqual(config["restoreEpoch"], .string("2"))
        if let verificationFailure {
            throw verificationFailure
        }
    }
}
