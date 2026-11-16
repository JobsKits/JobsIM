//
//  IMAuthRefreshIntegrationTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport
import XCTest

@testable import JobsIM

@MainActor
final class IMAuthRefreshIntegrationTests: XCTestCase {

    private func configured() async throws -> IMSession {
        guard let file = Bundle(for: Self.self).url(forResource: "AuthRefreshPublic", withExtension: "json")
        else {
            throw XCTSkip("New refresh realm public CA fixture not installed")
        }
        let config = try JSONDecoder.make().decode(IMJSON.self, from: Data(contentsOf: file))
        let address = config["url"].text
        let certificate = try IMInstanceCertificate(
            address: address, certificate: Data(config["caPEM"].text.utf8),
            verifiedFingerprint: config["fingerprint"].text)
        let url = try XCTUnwrap(URL(string: address))
        let api = IMAPIClient(baseURL: IMSession.apiURL(url), certificate: certificate)
        let descriptor = try await api.request("bootstrap")
        XCTAssertEqual(descriptor["deploymentId"], config["deploymentId"])
        guard descriptor["deploymentId"] == config["deploymentId"],
            descriptor["features"]["auth_refresh"] == .bool(true)
        else {
            throw IMAPIError(
                status: 0, code: "REFRESH_REALM_MISMATCH", message: "Explicit new refresh realm required")
        }
        let session = IMSession(store: try IMLocalStore(path: ":memory:"))
        session.profile = "native-refresh:" + UUID().uuidString
        session.baseURL = url
        session.api = api
        session.deployment = descriptor["deploymentId"].text
        session.restoreEpoch = descriptor["restoreEpoch"].text
        return session
    }

    private func register(_ session: IMSession) async throws -> (String, String) {
        let name = "ios_rf_" + UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(16)
        let password = UUID().uuidString + UUID().uuidString
        _ = try await session.authenticate(
            mode: "register", username: name, password: password,
            requestID: UUID().uuidString)
        return (name, password)
    }

    private func expectRevoked(_ api: IMAPIClient, token: String) async throws {
        do {
            _ = try await api.request("me", token: token)
            XCTFail("Expected revoked access credential")
        } catch let error as IMAPIError { XCTAssertEqual(error.status, 401) }
    }

    func testRealHTTPSConcurrentAccessExpiryCoordinatesOneRotation() async throws {
        let session = try await configured()
        _ = try await register(session)
        let api = try XCTUnwrap(session.api)
        let original = try XCTUnwrap(session.currentCredentialPair)
        let namespace = session.namespace
        var refreshCalls = 0
        session.authentication = IMAuthCoordinator(
            session: session,
            request: { path, body, token in
                if path == "auth/refresh" { refreshCalls += 1 }
                return try await api.request(path, method: "POST", body: body, token: token)
            })
        do {
            // 只注入业务层ACCESS_EXPIRED；实际refresh和重试GET仍使用严格CA的真实HTTPS。
            func read() async throws -> IMJSON {
                return try await session.authorized { client, token in
                    if token == original.accessToken {
                        throw IMAPIError(
                            status: 401, code: "ACCESS_EXPIRED", message: "Controlled expiry response")
                    }
                    return try await client.request("me", token: token)
                }
            }
            async let first = read()
            async let second = read()
            async let third = read()
            let users = try await [first, second, third]
            XCTAssertTrue(users.allSatisfy { $0["id"].text == original.userID })
            XCTAssertEqual(refreshCalls, 1)
            let next = try XCTUnwrap(session.currentCredentialPair)
            try next.validateRotation(from: original)
            XCTAssertEqual(session.namespace, namespace)
            XCTAssertEqual(
                UInt64(next.refreshGeneration), try XCTUnwrap(UInt64(original.refreshGeneration)) + 1)
            try await expectRevoked(api, token: original.accessToken)
            let source = FileManager.default.temporaryDirectory.appendingPathComponent(
                UUID().uuidString + ".txt")
            let destination = FileManager.default.temporaryDirectory.appendingPathComponent(
                UUID().uuidString + ".download")
            defer {
                try? FileManager.default.removeItem(at: source)
                try? FileManager.default.removeItem(at: destination)
            }
            let bytes = Data("Native authenticated media family boundary".utf8)
            try bytes.write(to: source)
            let upload = try await session.authorized { client, token in
                if token == next.accessToken {
                    throw IMAPIError(
                        status: 401, code: "ACCESS_EXPIRED", message: "Controlled media expiry response")
                }
                return try await client.upload(source, mimeType: "text/plain", token: token)
            }
            let group = try await session.action(
                "conversations",
                body: [
                    "kind": .string("group"),
                    "title": .string("Native media refresh"), "memberIds": .array([]),
                ])
            try await session.send(
                source.lastPathComponent, conversation: group["id"].text, type: "file",
                attachment: upload["id"].text, admission: group["sendAdmission"].text)
            _ = try await session.authorized { client, token in
                return try await client.download(upload["id"].text, to: destination, token: token)
            }
            XCTAssertTrue(try Data(contentsOf: destination) == bytes)
            XCTAssertEqual(refreshCalls, 2)
            XCTAssertEqual(session.currentCredentialPair?.familyID, original.familyID)
            XCTAssertEqual(session.namespace, namespace)
            await session.logout()
        } catch {
            await session.logout()
            throw error
        }
    }

    func testRealHTTPSLostRefreshRequiresReauthAndPreservesIndependentFamily() async throws {
        let session = try await configured()
        let (name, password) = try await register(session)
        let api = try XCTUnwrap(session.api)
        let original = try XCTUnwrap(session.currentCredentialPair)
        let oldNamespace = session.namespace
        let group = try await session.action(
            "conversations",
            body: [
                "kind": .string("group"),
                "title": .string("Native refresh boundary"), "memberIds": .array([]),
            ])
        let clientID = UUID().uuidString
        let queued: IMJSON = .object([
            "id": .string(clientID), "clientMsgId": .string(clientID),
            "conversationId": group["id"], "type": .string("text"), "text": .string("Keep old device intent"),
            "sendAdmission": group["sendAdmission"], "restoreEpoch": .string(session.restoreEpoch),
            "status": .string("queued"),
        ])
        try session.store.enqueue(queued, conversation: group["id"].text, namespace: oldNamespace)
        let independent = try await api.request(
            "auth/login", method: "POST",
            body: [
                "username": .string(name),
                "password": .string(password), "requestId": .string(UUID().uuidString),
                "deviceName": .string("Independent native test device"),
            ])
        var refreshCalls = 0
        var lost: IMJSON = .null
        session.authentication = IMAuthCoordinator(
            session: session,
            request: { path, body, token in
                let result = try await api.request(path, method: "POST", body: body, token: token)
                if path == "auth/refresh" {
                    refreshCalls += 1
                    lost = result
                    throw IMAPIError(
                        status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled post-commit response loss")
                }
                return result
            })
        do {
            do {
                _ = try await session.authentication.refresh(rejectedToken: original.accessToken)
                XCTFail("Unknown refresh must not publish received secrets")
            } catch let error as IMAPIError { XCTAssertEqual(error.code, "REFRESH_REAUTH_REQUIRED") }
            XCTAssertNil(session.token)
            do {
                _ = try await session.action("me", method: "GET")
                XCTFail("Business must stop")
            } catch {}
            XCTAssertEqual(refreshCalls, 1)
            _ = try await session.authenticate(
                mode: "login", username: name, password: password,
                requestID: UUID().uuidString)
            let next = try XCTUnwrap(session.currentCredentialPair)
            XCTAssertNotEqual(next.deviceID, original.deviceID)
            XCTAssertNotEqual(next.familyID, original.familyID)
            XCTAssertNotEqual(session.namespace, oldNamespace)
            XCTAssertTrue(
                try session.store.originalOutbox(clientID: clientID, namespace: oldNamespace) == queued)
            XCTAssertTrue(try session.store.pending(namespace: oldNamespace).isEmpty)
            XCTAssertTrue(try session.store.pending(namespace: session.namespace).isEmpty)
            let visible = try XCTUnwrap(
                session.store.list("messages:" + group["id"].text, namespace: oldNamespace).first)
            XCTAssertEqual(visible["freezeReason"].text, "authentication_device_changed")
            try await expectRevoked(api, token: lost["token"].text)
            let stillValid = try await api.request("me", token: independent["token"].text)
            XCTAssertEqual(stillValid["id"].text, original.userID)
            _ = try await api.request("auth/logout", method: "POST", token: independent["token"].text)
            await session.logout()
        } catch {
            _ = try? await api.request("auth/logout", method: "POST", token: independent["token"].text)
            await session.logout()
            throw error
        }
    }

    func testRealHTTPSInitialLostDeliveryReusesOriginalBodyAndDevice() async throws {
        let session = try await configured()
        let (name, password) = try await register(session)
        let api = try XCTUnwrap(session.api)
        let original = try XCTUnwrap(session.currentCredentialPair)
        let requestID = UUID().uuidString
        var attempts: [IMJSON] = []
        var lost: IMJSON = .null
        session.authentication = IMAuthCoordinator(
            session: session,
            request: { path, body, token in
                let result = try await api.request(path, method: "POST", body: body, token: token)
                if path == "auth/login" {
                    attempts.append(.object(body))
                    if attempts.count == 1 {
                        lost = result
                        throw IMAPIError(
                            status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled initial response loss")
                    }
                }
                return result
            })
        do {
            do {
                _ = try await session.authenticate(
                    mode: "login", username: name, password: password, requestID: requestID)
                XCTFail("Expected unresolved initial authentication")
            } catch let error as IMAPIError { XCTAssertEqual(error.code, "TRANSPORT_UNKNOWN") }
            _ = try await session.authenticate(
                mode: "login", username: name, password: password,
                requestID: UUID().uuidString)
            XCTAssertEqual(attempts.count, 2)
            XCTAssertTrue(attempts[0] == attempts[1])
            XCTAssertEqual(session.currentCredentialPair?.deviceID, lost["deviceId"].text)
            XCTAssertEqual(session.currentCredentialPair?.familyID, lost["familyId"].text)
            XCTAssertEqual(session.currentCredentialPair?.credentialRequestID, requestID)
            _ = try await api.request("auth/logout", method: "POST", token: original.accessToken)
            await session.logout()
        } catch {
            _ = try? await api.request("auth/logout", method: "POST", token: original.accessToken)
            await session.logout()
            throw error
        }
    }

    func testRealHTTPSUnknownSuccessorsRetireCompleteChain() async throws {
        let session = try await configured()
        let (name, password) = try await register(session)
        let api = try XCTUnwrap(session.api)
        let independent = try XCTUnwrap(session.currentCredentialPair)
        let ids = (0..<3).map { _ in UUID().uuidString }
        var bodies: [IMJSON] = []
        var lost: [IMJSON] = []
        session.authentication = IMAuthCoordinator(
            session: session,
            request: { path, body, token in
                let result = try await api.request(path, method: "POST", body: body, token: token)
                if path == "auth/login" || path == "auth/reauthenticate" {
                    bodies.append(.object(body))
                    if bodies.count < 3 {
                        lost.append(result)
                        throw IMAPIError(
                            status: 0, code: "TRANSPORT_UNKNOWN",
                            message: "Controlled unknown successor response")
                    }
                }
                return result
            })
        do {
            for index in 0..<3 {
                do {
                    _ = try await session.authenticate(
                        mode: "login", username: name, password: password,
                        requestID: ids[index], explicitReauthentication: index > 0)
                    XCTAssertEqual(index, 2)
                } catch let error as IMAPIError {
                    XCTAssertLessThan(index, 2)
                    XCTAssertEqual(error.code, "TRANSPORT_UNKNOWN")
                }
            }
            XCTAssertEqual(bodies.count, 3)
            XCTAssertEqual(bodies[0]["previousRequestIds"], .null)
            XCTAssertEqual(bodies[1]["previousRequestIds"].list, [.string(ids[0])])
            XCTAssertEqual(bodies[2]["previousRequestIds"].list, [.string(ids[0]), .string(ids[1])])
            XCTAssertEqual(bodies[2]["previousRequestId"].text, ids[1])
            for response in lost { try await expectRevoked(api, token: response["token"].text) }
            let stillIndependent = try await api.request("me", token: independent.accessToken)
            XCTAssertEqual(stillIndependent["id"].text, independent.userID)
            XCTAssertEqual(session.currentCredentialPair?.credentialRequestID, ids[2])
            _ = try await api.request("auth/logout", method: "POST", token: independent.accessToken)
            await session.logout()
        } catch {
            _ = try? await api.request("auth/logout", method: "POST", token: independent.accessToken)
            await session.logout()
            throw error
        }
    }
}
