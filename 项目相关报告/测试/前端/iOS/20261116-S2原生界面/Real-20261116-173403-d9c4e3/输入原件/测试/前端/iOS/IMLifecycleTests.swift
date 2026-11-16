//
//  IMLifecycleTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import CryptoKit
import Security
import XCTest

import JobsIMCore
@testable import JobsIMTransport
@testable import JobsIM

@MainActor
final class IMLifecycleTests: XCTestCase {
    func testContactsAndGroupAuthorityLifecycle() async throws {
        let api = try IMIntegrationFixture.client()
        let a = try await IMIntegrationFixture.register(api)
        let b = try await IMIntegrationFixture.register(api)
        let c = try await IMIntegrationFixture.register(api)
        let ta = a["token"].text, tb = b["token"].text, tc = c["token"].text
        let request = try await api.request(
            "contacts/requests", method: "POST",
            body: ["userId": b["user"]["id"], "requestId": .string(UUID().uuidString)], token: ta)
        let inbox = try await api.request("contacts", token: tb)
        XCTAssertTrue(
            inbox["requests"].list.contains { $0["id"] == request["id"] && $0["state"].text == "pending" })
        _ = try await api.request("contacts/requests/\(request["id"].text)/accept", method: "POST", token: tb)
        let accepted = try await api.request("contacts", token: ta)
        XCTAssertTrue(accepted["items"].list.contains { $0["id"] == b["user"]["id"] })
        _ = try await api.request("contacts/\(b["user"]["id"].text)", method: "DELETE", token: ta)
        let removed = try await api.request("contacts", token: tb)
        XCTAssertFalse(removed["items"].list.contains { $0["id"] == a["user"]["id"] })
        let refused = try await api.request(
            "contacts/requests", method: "POST",
            body: ["userId": b["user"]["id"], "requestId": .string(UUID().uuidString)], token: tc)
        _ = try await api.request("contacts/requests/\(refused["id"].text)/reject", method: "POST", token: tb)
        for (peer, peerToken) in [(b, tb), (c, tc)] {
            let fresh = try await api.request(
                "contacts/requests", method: "POST",
                body: ["userId": peer["user"]["id"], "requestId": .string(UUID().uuidString)], token: ta)
            _ = try await api.request(
                "contacts/requests/" + fresh["id"].text + "/accept", method: "POST", token: peerToken)
        }
        let bc = try await api.request(
            "contacts/requests", method: "POST",
            body: ["userId": c["user"]["id"], "requestId": .string(UUID().uuidString)], token: tb)
        _ = try await api.request("contacts/requests/" + bc["id"].text + "/accept", method: "POST", token: tc)
        let group = try await api.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("group"), "title": .string("Native lifecycle"),
                "memberIds": .array([b["user"]["id"], c["user"]["id"]]),
            ], token: ta)
        let path = "conversations/" + group["id"].text
        _ = try await api.request(
            path, method: "PATCH", body: ["title": .string("Renamed natively")], token: ta)
        do {
            _ = try await api.request(
                path, method: "PATCH", body: ["title": .string("Forbidden")], token: tb);
            XCTFail("Non-owner renamed")
        } catch let error as IMAPIError { XCTAssertEqual(error.status, 403) }
        let beforeRemove = try await api.request(path + "/members", token: ta)
        let cGeneration =
            beforeRemove["items"].list.first { $0["id"] == c["user"]["id"] }?["membershipGeneration"].text
            ?? ""
        XCTAssertFalse(cGeneration.isEmpty)
        _ = try await api.request(
            path + "/members/" + c["user"]["id"].text + "?membershipGeneration=" + cGeneration,
            method: "DELETE", token: ta)
        do {
            _ = try await api.request(path + "/messages?after=0", token: tc);
            XCTFail("Removed member read cloud history")
        } catch let error as IMAPIError { XCTAssertTrue([403, 404].contains(error.status)) }
        _ = try await api.request(path, method: "PATCH", body: ["ownerId": b["user"]["id"]], token: ta)
        do {
            _ = try await api.request(
                path + "/members", method: "POST", body: ["userId": c["user"]["id"]], token: ta);
            XCTFail("Old owner invited")
        } catch let error as IMAPIError { XCTAssertEqual(error.status, 403) }
        _ = try await api.request(
            path + "/members", method: "POST", body: ["userId": c["user"]["id"]], token: tb)
        let beforeLeave = try await api.request(path + "/members", token: ta)
        let aGeneration =
            beforeLeave["items"].list.first { $0["id"] == a["user"]["id"] }?["membershipGeneration"].text
            ?? ""
        XCTAssertFalse(aGeneration.isEmpty)
        _ = try await api.request(
            path + "/members/" + a["user"]["id"].text + "?membershipGeneration=" + aGeneration,
            method: "DELETE", token: ta)
        let members = try await api.request(path + "/members", token: tb)
        XCTAssertFalse(members["items"].list.contains { $0["id"] == a["user"]["id"] })
        _ = try await api.request(
            "me", method: "PATCH", body: ["displayName": .string("Native governance peer")], token: tb)
        _ = try await api.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("group"), "title": .string("Native governance group"),
                "memberIds": .array([b["user"]["id"]]),
            ], token: ta)
        let incoming = try await IMIntegrationFixture.register(api)
        _ = try await api.request(
            "contacts/requests", method: "POST",
            body: ["userId": a["user"]["id"], "requestId": .string(UUID().uuidString)],
            token: incoming["token"].text)
        _ = try await api.request(
            "me", method: "PATCH", body: ["displayName": .string("Native incoming peer")],
            token: incoming["token"].text)
        let profile = UUID().uuidString
        let authoritative = try await api.request("me", token: ta)
        try IMKeychain.write(ta, key: profile + ":" + a["user"]["id"].text)
        UserDefaults.standard.set("https://127.0.0.1:8443", forKey: "instanceURL")
        UserDefaults.standard.set(profile, forKey: "instanceProfile")
        UserDefaults.standard.set("1", forKey: "epoch:" + profile)
        UserDefaults.standard.set(try JSONEncoder.make().encode(authoritative), forKey: "user:" + profile)
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
        UserDefaults.standard.set(snapshot, forKey: "ui-fixture:governance")
        _ = try await api.request("blocks/\(b["user"]["id"].text)", method: "PUT", token: ta)
        do {
            _ = try await api.request(
                "contacts/requests", method: "POST", body: ["userId": a["user"]["id"]], token: tb);
            XCTFail("Blocked request accepted")
        } catch let error as IMAPIError { XCTAssertTrue([403, 409].contains(error.status)) }
        _ = try await api.request("blocks/\(b["user"]["id"].text)", method: "DELETE", token: ta)
        let unblocked = try await api.request("contacts", token: ta)
        XCTAssertFalse(unblocked["items"].list.contains { $0["id"] == b["user"]["id"] })
        _ = try await api.request(
            "contacts/" + b["user"]["id"].text + "/reopen", method: "POST",
            body: ["requestId": .string(UUID().uuidString)], token: ta)
        let renewed = try await api.request(
            "contacts/requests", method: "POST",
            body: ["userId": a["user"]["id"], "requestId": .string(UUID().uuidString)], token: tb)
        _ = try await api.request(
            "contacts/requests/" + renewed["id"].text + "/accept", method: "POST", token: ta)
        do {
            _ = try await api.request(
                "contacts/requests/" + request["id"].text + "/accept", method: "POST", token: tb);
            XCTFail("Old accepted request reopened relationship")
        } catch let error as IMAPIError { XCTAssertTrue([403, 409].contains(error.status)) }

    }
    func testAvatarOperationReconciliationAndPrivateReports() async throws {
        let api = try IMIntegrationFixture.client()
        let account = try await IMIntegrationFixture.register(api)
        let token = account["token"].text
        let me = try await api.request("me", token: token)
        let file = try XCTUnwrap(Bundle(for: Self.self).url(forResource: "Avatar", withExtension: "data"))
        let input = try Data(contentsOf: file)
        XCTAssertEqual(input.count, 258366)
        XCTAssertEqual(
            SHA256.hash(data: input).map { String(format: "%02x", $0) }.joined(),
            "be2cbcc9654b544d1fc0e762afe981d696f1ed3c9b8b8379187635990a0d55c9")
        let firstID = UUID().uuidString
        let form = ["requestId": firstID, "expectedVersion": me["profileVersion"].text]
        let first = try await api.upload(
            file, mimeType: "image/png", token: token, path: "me/avatar", form: form)
        XCTAssertEqual(first["status"].text, "applied")
        let current = try await api.request("me", token: token)
        XCTAssertFalse(current["avatarURL"].text.isEmpty)
        _ = try await api.upload(
            file, mimeType: "image/png", token: token, path: "me/avatar",
            form: ["requestId": UUID().uuidString, "expectedVersion": current["profileVersion"].text])
        let latest = try await api.request("me", token: token)
        _ = try await api.upload(file, mimeType: "image/png", token: token, path: "me/avatar", form: form)
        let original = try await api.request("me/avatar/operations/" + firstID, token: token)
        XCTAssertEqual(original["current"], .bool(false))
        let reconciled = try await api.request("me", token: token)
        XCTAssertEqual(reconciled["profileVersion"], latest["profileVersion"])
        let avatarID = try XCTUnwrap(latest["avatarThumbnailURL"].text.split(separator: "/").last)
        let local = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".png")
        _ = try await api.download(String(avatarID), to: local, token: token, collection: "avatars")
        XCTAssertGreaterThan(try Data(contentsOf: local).count, 0)
        let reportID = UUID().uuidString
        let body: [String: IMJSON] = [
            "targetType": .string("user"), "targetId": account["user"]["id"],
            "reason": .string("Native verified report"), "requestId": .string(reportID),
        ]
        let report = try await api.request("reports", method: "POST", body: body, token: token)
        let repeatReport = try await api.request("reports", method: "POST", body: body, token: token)
        XCTAssertEqual(report["id"], repeatReport["id"])
        let own = try await api.request("reports", token: token)
        XCTAssertEqual(own["includesChatBodies"], .bool(false))
        XCTAssertTrue(own["items"].list.contains { $0["id"] == report["id"] })
    }
    func testStrictInstanceTrustDoesNotUseSystemRootStore() async throws {
        let strict = try IMIntegrationFixture.client()
        let bootstrap = try await strict.request("bootstrap")
        XCTAssertFalse(bootstrap["deploymentId"].text.isEmpty)
        let system = IMAPIClient(baseURL: URL(string: "https://127.0.0.1:8443/api/")!)
        do {
            _ = try await system.request("bootstrap");
            XCTFail("Test device unexpectedly trusts local CA globally")
        } catch { XCTAssertFalse(error.localizedDescription.isEmpty) }
        let certificate = try IMIntegrationFixture.certificate()
        XCTAssertThrowsError(
            try IMInstanceCertificate(
                address: certificate.address, certificate: certificate.certificateDER,
                verifiedFingerprint: String(repeating: "0", count: 64)))
        XCTAssertFalse(certificate.permits(URL(string: "https://127.0.0.1:20443/api/")!))
    }
    func testCertificateRejectsWrongHostnameAndExpiredLeaf() throws {
        let certificate = try IMIntegrationFixture.certificate()
        let file = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "IntegrationServer", withExtension: "pem"))
        let pem = try String(contentsOf: file, encoding: .utf8)
        let encoded = pem.components(separatedBy: "-----BEGIN CERTIFICATE-----")[1].components(
            separatedBy: "-----END CERTIFICATE-----")[0]
        let leaf = try XCTUnwrap(
            SecCertificateCreateWithData(
                nil, Data(base64Encoded: encoded, options: .ignoreUnknownCharacters)! as CFData))
        let root = try XCTUnwrap(SecCertificateCreateWithData(nil, certificate.certificateDER as CFData))
        func makeTrust(_ host: String, date: Date? = nil) throws -> SecTrust {
            var trust: SecTrust?
            XCTAssertEqual(
                SecTrustCreateWithCertificates(
                    [leaf, root] as CFArray, SecPolicyCreateSSL(true, host as CFString), &trust),
                errSecSuccess)
            let value = try XCTUnwrap(trust)
            if let date { SecTrustSetVerifyDate(value, date as CFDate) }
            return value
        }
        let evaluator = IMInstanceEvaluator(certificate: certificate)
        XCTAssertNoThrow(try evaluator.evaluate(makeTrust("127.0.0.1"), forHost: "127.0.0.1"))
        XCTAssertThrowsError(
            try evaluator.evaluate(makeTrust("wrong.example.invalid"), forHost: "wrong.example.invalid"))
        XCTAssertThrowsError(
            try evaluator.evaluate(
                makeTrust("127.0.0.1", date: Date(timeIntervalSince1970: 4102444800)), forHost: "127.0.0.1"))
    }

}
