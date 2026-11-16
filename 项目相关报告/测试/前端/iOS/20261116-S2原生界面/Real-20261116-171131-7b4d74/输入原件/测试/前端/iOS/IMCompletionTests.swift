//
//  IMCompletionTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import AVFoundation
import CryptoKit
import JobsIMCore
import JobsIMStorage
import JobsIMTransport
import UIKit
import XCTest

@testable import JobsIM

@MainActor
final class IMCompletionTests: XCTestCase {
    func testLocalExportRedactsBodiesAndSeparatesAccounts() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let store = try IMLocalStore(path: directory.appendingPathComponent("export.sqlite").path)
        let ns = IMNamespace(profile: "profile-a", account: "alice", demo: false)
        let other = IMNamespace(profile: "profile-b", account: "alice", demo: false)
        try store.save(
            [.object(["id": .string("c"), "title": .string("Local history")])], collection: "conversations",
            namespace: ns)
        try store.save(
            [
                .object([
                    "id": .string("normal"), "text": .string("Ordinary copy"),
                    "token": .string("never-export"),
                ]),
                .object([
                    "id": .string("deleted"), "text": .string("deleted-body"), "deleted": .bool(true),
                    "attachmentId": .string("deleted-media"),
                ]),
                .object([
                    "id": .string("unavailable"), "text": .string("unavailable-body"),
                    "contentUnavailable": .bool(true), "attachmentId": .string("unavailable-media"),
                ]),
                .object([
                    "id": .string("pending"), "text": .string("not-confirmed"), "status": .string("queued"),
                ]),
            ], collection: "messages:c", namespace: ns)
        let data = try IMHistoryExport.data(store: store, namespace: ns)
        let document = try JSONDecoder.make().decode(IMJSON.self, from: data)
        XCTAssertEqual(document["formatVersion"].text, "1")
        XCTAssertEqual(document["importSupported"], .bool(false))
        XCTAssertEqual(document["includesMediaBytes"], .bool(false))
        XCTAssertEqual(document["accountId"].text, "alice")
        XCTAssertEqual(document["conversations"].list.first?["messages"].list.count, 3)
        let text = String(decoding: data, as: UTF8.self)
        XCTAssertTrue(text.contains("Ordinary copy"))
        for denied in [
            "never-export", "deleted-body", "deleted-media", "unavailable-body", "unavailable-media",
            "not-confirmed",
        ] {
            XCTAssertFalse(text.contains(denied))
        }
        let isolated = try JSONDecoder.make().decode(
            IMJSON.self, from: IMHistoryExport.data(store: store, namespace: other))
        XCTAssertTrue(isolated["conversations"].list.isEmpty)
    }
    func testRealDeviceRevocationAndOneUseAccountRecovery() async throws {
        let api = try IMIntegrationFixture.client()
        let username = "recovery_" + UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(16)
        let password = UUID().uuidString + UUID().uuidString
        let account = try await api.request(
            "auth/register", method: "POST",
            body: [
                "username": .string(username), "password": .string(password),
                "requestId": .string(UUID().uuidString), "deviceName": .string("Native first device"),
            ])
        let login = try await api.request(
            "auth/login", method: "POST",
            body: [
                "username": .string(username), "password": .string(password),
                "requestId": .string(UUID().uuidString), "deviceName": .string("Native second device"),
            ])
        let sessions = try await api.request("auth/sessions", token: login["token"].text)
        XCTAssertEqual(sessions["items"].list.count, 2)
        XCTAssertTrue(sessions["items"].list.contains { $0["name"].text == "Native first device" })
        _ = try await api.request(
            "auth/sessions/" + account["deviceId"].text, method: "DELETE", token: login["token"].text)
        do {
            _ = try await api.request("me", token: account["token"].text)
            XCTFail("Revoked token accepted")
        } catch let error as IMAPIError { XCTAssertEqual(error.status, 401) }
        let request = UUID().uuidString
        let material = try await api.request(
            "auth/recovery-codes", method: "POST",
            body: ["password": .string(password), "requestId": .string(request)], token: login["token"].text)
        XCTAssertFalse(material["codes"].list.isEmpty)
        let code = try XCTUnwrap(material["codes"].list.first?.text)
        let repeated = try await api.request(
            "auth/recovery-codes", method: "POST",
            body: ["password": .string(password), "requestId": .string(request)], token: login["token"].text)
        XCTAssertTrue(repeated["codes"].list.isEmpty)
        let newPassword = UUID().uuidString + UUID().uuidString
        let body: [String: IMJSON] = [
            "username": .string(username), "code": .string(code), "password": .string(newPassword),
            "requestId": .string(UUID().uuidString),
        ]
        let recovered = try await api.request("auth/recover", method: "POST", body: body)
        XCTAssertTrue(recovered["token"].text.isEmpty)
        do {
            _ = try await api.request("me", token: login["token"].text)
            XCTFail("Recovery retained old session")
        } catch let error as IMAPIError { XCTAssertEqual(error.status, 401) }
        let fresh = try await api.request(
            "auth/login", method: "POST",
            body: [
                "username": .string(username), "password": .string(newPassword),
                "requestId": .string(UUID().uuidString), "deviceName": .string("Recovered native device"),
            ])
        XCTAssertFalse(fresh["token"].text.isEmpty)
        do {
            _ = try await api.request(
                "auth/recover", method: "POST",
                body: body.merging(["requestId": .string(UUID().uuidString)]) { _, new in new })
            XCTFail("Recovery code reused")
        } catch let error as IMAPIError { XCTAssertEqual(error.status, 403) }
    }
    func testRealImageVideoAndFileAuthenticatedRoundtrip() async throws {
        let api = try IMIntegrationFixture.client()
        let account = try await IMIntegrationFixture.register(api)
        let stranger = try await IMIntegrationFixture.register(api)
        let token = account["token"].text
        let conversation = try await api.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("group"), "title": .string("Native media formats"), "memberIds": .array([]),
            ], token: token)
        let image = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "Avatar", withExtension: "data"))
        let video = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "NativeVideo", withExtension: "data"))
        let text = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".txt")
        try Data("JobsIM native file roundtrip\n".utf8).write(to: text)
        for (source, type, mime, name) in [
            (image, "image", "image/png", "Native-image.png"),
            (video, "video", "video/mp4", "Native-video.mp4"),
            (text, "file", "text/plain", "Native-file.txt"),
        ] {
            let media = try await api.upload(source, mimeType: mime, token: token)
            _ = try await api.request(
                "conversations/" + conversation["id"].text + "/messages", method: "POST",
                body: [
                    "clientMsgId": .string(UUID().uuidString), "type": .string(type), "text": .string(name),
                    "attachmentId": media["id"], "restoreEpoch": .string("1"),
                    "sendAdmission": conversation["sendAdmission"],
                ], token: token)
            let file = FileManager.default.temporaryDirectory.appendingPathComponent(
                UUID().uuidString + "-" + name)
            _ = try await api.download(media["id"].text, to: file, token: token)
            XCTAssertEqual(
                SHA256.hash(data: try Data(contentsOf: source)), SHA256.hash(data: try Data(contentsOf: file))
            )
            if type == "image" { XCTAssertNotNil(UIImage.make(contentsOfFile: file.path)) }
            if type == "video" {
                let tracks = try await AVURLAsset(url: file).loadTracks(withMediaType: .video)
                XCTAssertFalse(tracks.isEmpty)
            }
            do {
                let denied = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
                _ = try await api.download(media["id"].text, to: denied, token: stranger["token"].text)
                XCTFail("Stranger downloaded private media")
            } catch let error as IMAPIError { XCTAssertTrue([403, 404].contains(error.status)) }
        }
    }
    func testAuthorizedMediaCacheOfflineAndRevocationGate() async throws {
        let api = try IMIntegrationFixture.client()
        let account = try await IMIntegrationFixture.register(api)
        let token = account["token"].text
        let conversation = try await api.request(
            "conversations", method: "POST",
            body: ["kind": .string("group"), "title": .string("Native cache"), "memberIds": .array([])],
            token: token)
        let source = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "Avatar", withExtension: "data"))
        let media = try await api.upload(source, mimeType: "image/png", token: token)
        let message = try await api.request(
            "conversations/" + conversation["id"].text + "/messages", method: "POST",
            body: [
                "clientMsgId": .string(UUID().uuidString), "type": .string("image"),
                "text": .string("cache.png"), "attachmentId": media["id"], "restoreEpoch": .string("1"),
                "sendAdmission": conversation["sendAdmission"],
            ], token: token)
        let session = IMSession.shared
        let original = (
            session.profile, session.user, session.token, session.demo, session.api, session.restoreEpoch,
            session.deployment
        )
        session.profile = UUID().uuidString
        session.user = account["user"]
        session.token = token
        session.demo = false
        session.api = api
        session.deployment = try await api.request("bootstrap")["deploymentId"].text
        session.restoreEpoch = "1"
        let ns = session.namespace
        defer {
            IMMediaCache.purge(messageID: message["id"].text, namespace: ns)
            session.profile = original.0
            session.user = original.1
            session.token = original.2
            session.demo = original.3
            session.api = original.4
            session.restoreEpoch = original.5
            session.deployment = original.6
        }
        try session.store.save([message], collection: "messages:" + conversation["id"].text, namespace: ns)
        let online = try await IMMediaCache.file(message, namespace: ns)
        XCTAssertEqual(try Data(contentsOf: online), try Data(contentsOf: source))
        let sourceCertificate = try IMIntegrationFixture.certificate()
        let unreachableCertificate = try IMInstanceCertificate(
            address: "https://127.0.0.1:1", certificate: sourceCertificate.certificateDER,
            verifiedFingerprint: sourceCertificate.fingerprint)
        let offline = IMAPIClient(
            baseURL: URL(string: "https://127.0.0.1:1/api/")!, certificate: unreachableCertificate)
        session.api = offline
        let cached = try await IMMediaCache.file(message, namespace: ns)
        XCTAssertEqual(cached, online)
        session.api = api
        session.token = UUID().uuidString
        do {
            _ = try await IMMediaCache.file(message, namespace: ns)
            XCTFail("401 fell back to local media")
        } catch let error as IMAPIError { XCTAssertEqual(error.status, 401) }
        session.token = token
        session.api = offline
        try session.store.save(
            [message.setting("deleted", .bool(true)).setting("version", .string("99"))],
            collection: "messages:" + conversation["id"].text, namespace: ns)
        do {
            _ = try await IMMediaCache.file(message, namespace: ns)
            XCTFail("Old row bypassed a newer tombstone")
        } catch {}
        IMMediaCache.purge(messageID: message["id"].text, namespace: ns)
        XCTAssertFalse(FileManager.default.fileExists(atPath: online.path))
    }
    func testAvatarReuseClearsOmittedProfilesAndDeniedACL() async throws {
        let api = try IMIntegrationFixture.client()
        let owner = try await IMIntegrationFixture.register(api)
        let stranger = try await IMIntegrationFixture.register(api)
        let source = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "Avatar", withExtension: "data"))
        _ = try await api.upload(
            source, mimeType: "image/png", token: owner["token"].text, path: "me/avatar",
            form: ["requestId": UUID().uuidString, "expectedVersion": "1"])
        let profile = try await api.request("me", token: owner["token"].text)
        let session = IMSession.shared
        let original = (
            session.profile, session.user, session.token, session.demo, session.api, session.deployment
        )
        session.profile = UUID().uuidString
        session.user = owner["user"]
        session.token = owner["token"].text
        session.demo = false
        session.api = api
        session.deployment = try await api.request("bootstrap")["deploymentId"].text
        defer {
            session.profile = original.0
            session.user = original.1
            session.token = original.2
            session.demo = original.3
            session.api = original.4
            session.deployment = original.5
        }
        XCTAssertNotNil(UIImage.make(named: "Logo"))
        let view = IMAvatarView.make()
        view.show(profile)
        for _ in 0..<60 where view.accessibilityIdentifier != "avatar.remote" {
            try await Task.sleep(for: .milliseconds(50))
        }
        XCTAssertEqual(view.accessibilityIdentifier, "avatar.remote")
        view.show(.null)
        XCTAssertEqual(view.accessibilityIdentifier, "avatar.fallback")
        view.show(profile)
        session.profile = UUID().uuidString
        try await Task.sleep(for: .milliseconds(200))
        XCTAssertEqual(view.accessibilityIdentifier, "avatar.fallback")
        session.user = stranger["user"]
        session.token = stranger["token"].text
        let denied = await IMAvatarLoader.image(
            profile["avatarThumbnailURL"].text, namespace: session.namespace)
        XCTAssertNil(denied)
    }
    func testRevokedSessionLeavesFormalUIAndLateReplyCannotLogoutNewIdentity() async throws {
        let api = try IMIntegrationFixture.client()
        let account = try await IMIntegrationFixture.register(api)
        let session = IMSession.shared
        let original = (
            session.profile, session.user, session.token, session.demo, session.api, session.changed,
            session.deployment
        )
        session.changed = nil
        session.profile = UUID().uuidString
        session.user = account["user"]
        session.token = account["token"].text
        session.demo = false
        session.api = api
        session.deployment = try await api.request("bootstrap")["deploymentId"].text
        let ns = session.namespace
        let token = session.token
        defer {
            session.profile = original.0
            session.user = original.1
            session.token = original.2
            session.demo = original.3
            session.api = original.4
            session.changed = original.5
            session.deployment = original.6
        }
        try session.store.save(
            [.object(["id": .string("draft"), "text": .string("Keep ordinary draft")])], collection: "drafts",
            namespace: ns)
        _ = try await api.request("auth/sessions/" + account["deviceId"].text, method: "DELETE", token: token)
        do {
            _ = try await session.action("me", method: "GET")
            XCTFail("Revoked identity remained authorized")
        } catch let error as IMAPIError { XCTAssertEqual(error.status, 401) }
        XCTAssertNil(session.token)
        XCTAssertEqual(session.user, .null)
        XCTAssertEqual(
            try session.store.list("drafts", namespace: ns).first?["text"].text, "Keep ordinary draft")
        session.profile = UUID().uuidString
        session.user = .object(["id": .string("new-identity")])
        session.token = UUID().uuidString
        let newToken = session.token
        session.invalidateAuthentication(namespace: ns, token: token)
        XCTAssertEqual(session.token, newToken)
        XCTAssertEqual(session.user["id"].text, "new-identity")
    }
    func testPrepareNativePickerSession() async throws {
        let api = try IMIntegrationFixture.client()
        let owner = try await IMIntegrationFixture.register(api)
        let peer = try await IMIntegrationFixture.register(api)
        let token = owner["token"].text
        _ = try await api.request(
            "me", method: "PATCH", body: ["displayName": .string("Native portrait peer")],
            token: peer["token"].text)
        let image = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "Avatar", withExtension: "data"))
        _ = try await api.upload(
            image, mimeType: "image/png", token: peer["token"].text, path: "me/avatar",
            form: ["requestId": UUID().uuidString, "expectedVersion": "1"])
        let request = try await api.request(
            "contacts/requests", method: "POST",
            body: ["userId": owner["user"]["id"], "requestId": .string(UUID().uuidString)],
            token: peer["token"].text)
        _ = try await api.request(
            "contacts/requests/" + request["id"].text + "/accept", method: "POST", token: token)
        let direct = try await api.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("direct"), "memberIds": .array([peer["user"]["id"]]),
                "requestId": .string(UUID().uuidString),
            ], token: token)
        let peerConversations = try await api.request("conversations", token: peer["token"].text)
        let peerAdmission =
            peerConversations["items"].list.first { $0["id"] == direct["id"] }?["sendAdmission"] ?? .null
        let media = try await api.upload(image, mimeType: "image/png", token: peer["token"].text)
        _ = try await api.request(
            "conversations/" + direct["id"].text + "/messages", method: "POST",
            body: [
                "clientMsgId": .string(UUID().uuidString), "type": .string("image"),
                "text": .string("Native image.png"), "attachmentId": media["id"],
                "restoreEpoch": .string("1"), "sendAdmission": peerAdmission,
            ], token: peer["token"].text)
        _ = try await api.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("group"), "title": .string("Native picker group"),
                "memberIds": .array([peer["user"]["id"]]),
            ], token: token)
        let profile = UUID().uuidString
        let user = try await api.request("me", token: token)
        let bootstrap = try await api.request("bootstrap")
        try IMKeychain.write(token, key: profile + ":" + user["id"].text)
        UserDefaults.standard.set("https://127.0.0.1:8443", forKey: "instanceURL")
        UserDefaults.standard.set(profile, forKey: "instanceProfile")
        UserDefaults.standard.set(bootstrap["deploymentId"].text, forKey: "instanceDeployment")
        UserDefaults.standard.set("1", forKey: "epoch:" + profile)
        UserDefaults.standard.set(try JSONEncoder.make().encode(user), forKey: "user:" + profile)
        UserDefaults.standard.set(
            try IMIntegrationFixture.certificate().fingerprint, forKey: "instanceTrustFingerprint")
        let keys = [
            "instanceURL", "instanceProfile", "instanceDeployment", "instanceTrustFingerprint",
            "user:" + profile, "epoch:" + profile,
        ]
        let snapshot = Dictionary(
            uniqueKeysWithValues: keys.compactMap { key in
                UserDefaults.standard.object(forKey: key).map { (key, $0) }
            })
        UserDefaults.standard.set(snapshot, forKey: "ui-fixture:picker")
        let documents = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
        try FileManager.default.createDirectory(at: documents, withIntermediateDirectories: true)
        try Data(contentsOf: image).write(
            to: documents.appendingPathComponent("JobsIM-Avatar.png"), options: .atomic)
        let certificate = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "IntegrationCA", withExtension: "pem"))
        try Data(contentsOf: certificate).write(
            to: documents.appendingPathComponent("JobsIM-CA.pem"), options: .atomic)
    }
}
