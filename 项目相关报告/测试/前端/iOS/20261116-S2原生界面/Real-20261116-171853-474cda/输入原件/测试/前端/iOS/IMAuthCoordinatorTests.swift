//
//  IMAuthCoordinatorTests.swift
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
final class IMAuthCoordinatorTests: XCTestCase {
    private var databasePaths: [String: URL] = [:]
    private let deployment = "24ac6520-f87d-4bb0-9ff7-df3b1500c0c2"
    private let userID = "e1dfe1c3-47eb-43b4-9895-9c14c3dce59a"
    private let deviceID = "39df1a0e-9fb8-439e-8d0d-eab8cbfa0ed3"
    private let familyID = "76ec6b98-966c-47d2-8e59-bc6d69dd906e"

    private func response(generation: String = "1") -> IMJSON {
        return .object([
            "token": .string("fixture-access-" + generation),
            "refreshToken": .string("fixture-refresh-" + generation),
            "familyId": .string(familyID), "refreshGeneration": .string(generation),
            "deviceId": .string(deviceID), "restoreEpoch": .string("1"),
            "sourceContextId": .string("personal"),
            "user": .object(["id": .string(userID), "username": .string("credential_user")]),
            "accessExpiresAt": .string("2026-11-15T12:15:00Z"),
            "idleExpiresAt": .string("2026-12-15T12:00:00Z"),
            "absoluteExpiresAt": .string("2027-02-13T12:00:00Z"),
        ])
    }

    private func session() throws -> IMSession {
        let path = FileManager.default.temporaryDirectory.appendingPathComponent(
            UUID().uuidString + ".sqlite")
        let value = IMSession(store: try IMLocalStore(path: path.path))
        value.profile = "auth-coordinator-test:" + UUID().uuidString
        databasePaths[value.profile] = path
        value.deployment = deployment
        value.restoreEpoch = "1"
        value.user = response()["user"]
        let pair = try IMCredentialPair(
            response: response(), profile: value.profile, deploymentID: deployment,
            requestID: "original-auth-request")
        value.credentialPair = pair
        value.token = pair.accessToken
        return value
    }

    private func reopened(_ original: IMSession) throws -> IMSession {
        let path = try XCTUnwrap(databasePaths[original.profile])
        let value = IMSession(store: try IMLocalStore(path: path.path))
        value.profile = original.profile
        value.deployment = original.deployment
        value.restoreEpoch = original.restoreEpoch
        return value
    }

    private func metadata(_ session: IMSession) -> IMNamespace {
        return IMNamespace(
            profile: session.profile, account: "__auth_state__", demo: false,
            restoreEpoch: "1", deploymentID: deployment, sourceContextID: "personal")
    }

    private func cleanup(_ session: IMSession) {
        try? IMKeychain.write(nil, key: session.profile + ":" + userID)
        UserDefaults.standard.removeObject(forKey: "user:" + session.profile)
        UserDefaults.standard.removeObject(forKey: "loggedOut:" + session.profile + ":" + userID)
    }

    func testConcurrentExpiredRequestsConsumeRefreshOnceAndPreserveNamespace() async throws {
        let session = try session()
        defer { cleanup(session) }
        let original = try XCTUnwrap(session.currentCredentialPair)
        let namespace = session.namespace
        var calls = 0
        let next = response(generation: "2")
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { path, body, token in
                XCTAssertEqual(path, "auth/refresh")
                XCTAssertNil(token)
                XCTAssertEqual(body["familyId"]?.text, original.familyID)
                XCTAssertEqual(body["refreshGeneration"]?.text, "1")
                calls += 1
                try await Task.sleep(for: .milliseconds(25))
                return next
            })
        async let first = coordinator.refresh(rejectedToken: original.accessToken)
        async let second = coordinator.refresh(rejectedToken: original.accessToken)
        let tokens = try await [first, second]
        XCTAssertEqual(calls, 1)
        XCTAssertEqual(tokens, [next["token"].text, next["token"].text])
        XCTAssertEqual(session.namespace, namespace)
        XCTAssertEqual(session.currentCredentialPair?.familyID, original.familyID)
        XCTAssertEqual(session.currentCredentialPair?.refreshGeneration, "2")
        let secure = try XCTUnwrap(IMKeychain.readValidated(session.profile + ":" + userID))
        XCTAssertEqual(try IMCredentialPair.decode(secure), session.currentCredentialPair)
        let records = try JSONEncoder.make().encode(
            session.store.list("auth-state", namespace: metadata(session)))
        let publicText = String(decoding: records, as: UTF8.self)
        XCTAssertFalse(publicText.contains(next["refreshToken"].text))
        XCTAssertFalse(publicText.contains(next["token"].text))
    }

    func testUnknownRefreshPersistsBarrierAndNeverReusesOriginalSecret() async throws {
        let session = try session()
        defer { cleanup(session) }
        let original = try XCTUnwrap(session.currentCredentialPair)
        var calls = 0
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { _, _, _ in
                calls += 1
                throw IMAPIError(status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled lost response")
            })
        do {
            _ = try await coordinator.refresh(rejectedToken: original.accessToken)
            XCTFail("Unknown refresh must block")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "REFRESH_REAUTH_REQUIRED") }
        XCTAssertNil(session.token)
        let marker = try XCTUnwrap(
            session.store.list("auth-state", namespace: metadata(session)).first {
                $0["id"].text == "refresh:" + original.familyID
            })
        XCTAssertEqual(marker["state"].text, "unknown")
        // 模拟旧回调仍持有整对，持久围栏仍在HTTP之前拒绝再次消费。
        let reopened = try reopened(session)
        reopened.user = response()["user"]
        reopened.credentialPair = original
        reopened.token = original.accessToken
        let restarted = IMAuthCoordinator(
            session: reopened,
            request: { _, _, _ in
                calls += 1
                return self.response(generation: "2")
            })
        do {
            _ = try await restarted.refresh(rejectedToken: original.accessToken)
            XCTFail("Original RT reused")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "REFRESH_REAUTH_REQUIRED") }
        XCTAssertEqual(calls, 1)
    }

    func testSecurePairFailureStopsPublishingAndRetainsNonsecretFamily() async throws {
        let session = try session()
        defer { cleanup(session) }
        let original = try XCTUnwrap(session.currentCredentialPair)
        let next = response(generation: "2")
        let coordinator = IMAuthCoordinator(
            session: session, request: { _, _, _ in next },
            secureWrite: { _, _ in
                throw IMAPIError(
                    status: 0, code: "CONTROLLED_KEYCHAIN_FAILURE",
                    message: "Controlled secure storage failure")
            })
        do {
            _ = try await coordinator.refresh(rejectedToken: original.accessToken)
            XCTFail("Unsecured pair published")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "REFRESH_REAUTH_REQUIRED") }
        XCTAssertNil(session.token)
        XCTAssertNil(session.currentCredentialPair)
        let marker = try XCTUnwrap(session.store.list("auth-state", namespace: metadata(session)).first)
        XCTAssertEqual(marker["familyId"].text, original.familyID)
        XCTAssertEqual(marker["state"].text, "unknown")
    }

    func testUnknownInitialUsesOriginalIDThenExplicitReauthLinksAndFreezesOldDevice() async throws {
        let session = try session()
        defer { cleanup(session) }
        let oldNamespace = session.namespace
        let intent: IMJSON = .object([
            "id": .string("keep-original"), "clientMsgId": .string("keep-original"),
            "conversationId": .string("conversation"), "type": .string("text"),
            "text": .string("Keep original bytes"),
            "sendAdmission": .string("original-admission"), "restoreEpoch": .string("1"),
            "status": .string("queued"),
        ])
        try session.store.enqueue(intent, conversation: "conversation", namespace: oldNamespace)
        let password = UUID().uuidString + UUID().uuidString
        let originalID = UUID().uuidString
        let successorID = UUID().uuidString
        var seen: [(String, IMJSON)] = []
        let result = response().setting("deviceId", .string(UUID().uuidString)).setting(
            "familyId", .string(UUID().uuidString))
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { path, fields, _ in
                seen.append((path, .object(fields)))
                if path == "auth/reauthenticate" { return result }
                throw IMAPIError(
                    status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled unknown authentication")
            })
        for proposed in [originalID, UUID().uuidString] {
            do {
                _ = try await coordinator.authenticate(
                    mode: "login", username: "credential_user", password: password,
                    otp: "", proposedRequestID: proposed, explicitNew: false)
                XCTFail("Unknown authentication must remain unresolved")
            } catch {}
        }
        XCTAssertEqual(seen.count, 2)
        XCTAssertTrue(seen[0].1 == seen[1].1)
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "credential_user", password: password + "changed",
                otp: "", proposedRequestID: UUID().uuidString, explicitNew: false)
            XCTFail("Changed proof must not be placed into the original unknown request")
        } catch let error as IMAPIError {
            XCTAssertEqual(error.code, "ORIGINAL_AUTHENTICATION_PROOF_CHANGED")
        }
        XCTAssertEqual(seen.count, 2)
        let records = try JSONEncoder.make().encode(
            session.store.list("auth-state", namespace: metadata(session)))
        XCTAssertFalse(String(decoding: records, as: UTF8.self).contains(password))
        _ = try await coordinator.authenticate(
            mode: "login", username: "credential_user", password: password,
            otp: "", proposedRequestID: successorID, explicitNew: true,
            descriptor: .object(["features": .object(["auth_retirement_chain": .bool(true)])]))
        XCTAssertEqual(seen.last?.0, "auth/reauthenticate")
        XCTAssertEqual(seen.last?.1["previousRequestId"].text, originalID)
        XCTAssertEqual(seen.last?.1["requestId"].text, successorID)
        XCTAssertNotEqual(session.namespace, oldNamespace)
        XCTAssertTrue(try session.store.pending(namespace: oldNamespace).isEmpty)
        XCTAssertTrue(try session.store.pending(namespace: session.namespace).isEmpty)
        XCTAssertEqual(
            try session.store.originalOutbox(clientID: "keep-original", namespace: oldNamespace), intent)
        let visible = try XCTUnwrap(
            session.store.list("messages:conversation", namespace: oldNamespace).first)
        XCTAssertEqual(visible["freezeReason"].text, "authentication_device_changed")
    }

    func testStaleOldFamilyCannotInvalidateReplacementDevice() throws {
        let session = try session()
        defer { cleanup(session) }
        let old = try XCTUnwrap(session.currentCredentialPair)
        let next = try IMCredentialPair(
            response: response().setting("deviceId", .string(UUID().uuidString))
                .setting("familyId", .string(UUID().uuidString)).setting(
                    "token", .string("replacement-fixture-access")),
            profile: session.profile, deploymentID: deployment, requestID: "replacement-auth-request")
        session.credentialPair = next
        session.token = next.accessToken
        session.invalidateAuthentication(namespace: old.namespace, token: old.accessToken)
        XCTAssertEqual(session.currentCredentialPair, next)
        XCTAssertEqual(session.token, next.accessToken)
    }

    func testRestartFinishesSecuredInitialPairBeforeExposingNewNamespace() throws {
        let session = try session()
        defer { cleanup(session) }
        let original = try XCTUnwrap(session.currentCredentialPair)
        let oldNamespace = original.namespace
        let requestID = UUID().uuidString
        let next = try IMCredentialPair(
            response: response().setting("deviceId", .string(UUID().uuidString))
                .setting("familyId", .string(UUID().uuidString)),
            profile: session.profile, deploymentID: deployment, requestID: requestID)
        let intent: IMJSON = .object([
            "id": .string("crash-preserved"), "clientMsgId": .string("crash-preserved"),
            "conversationId": .string("conversation"), "type": .string("text"),
            "text": .string("Before device handoff"), "sendAdmission": .string("old-admission"),
            "restoreEpoch": .string("1"), "status": .string("queued"),
        ])
        try session.store.enqueue(intent, conversation: "conversation", namespace: oldNamespace)
        try session.store.save(
            [
                .object([
                    "id": .string("binding:" + userID), "state": .string("active"),
                    "profile": .string(session.profile), "deploymentId": .string(deployment),
                    "userId": .string(userID), "username": .string(original.username),
                    "user": response()["user"], "deviceId": .string(original.deviceID),
                    "sourceContextId": .string("personal"), "restoreEpoch": .string("1"),
                    "familyId": .string(original.familyID), "refreshGeneration": .string("1"),
                    "credentialRequestId": .string(original.credentialRequestID),
                ]),
                .object([
                    "id": .string("request:" + requestID), "requestId": .string(requestID),
                    "username": .string(original.username), "purpose": .string("reauthenticate"),
                    "state": .string("securing"), "resultUser": response()["user"],
                    "resultFamilyId": .string(next.familyID),
                ]),
                .object([
                    "id": .string("initial:" + original.username), "requestId": .string(requestID),
                    "state": .string("pending"),
                ]),
                .object(["id": .string("active-auth-intent"), "requestId": .string(requestID)]),
            ], collection: "auth-state", namespace: metadata(session))
        try IMKeychain.write(next.serialized(), key: session.profile + ":" + userID)
        let reopened = try reopened(session)
        let restarted = IMAuthCoordinator(
            session: reopened,
            request: { _, _, _ in
                XCTFail("A secured local handoff must not request another credential pair")
                return .null
            })
        try restarted.restore()
        XCTAssertEqual(reopened.currentCredentialPair, next)
        XCTAssertEqual(reopened.namespace, next.namespace)
        XCTAssertTrue(try reopened.store.pending(namespace: oldNamespace).isEmpty)
        XCTAssertTrue(try reopened.store.pending(namespace: next.namespace).isEmpty)
        XCTAssertEqual(
            try reopened.store.originalOutbox(clientID: "crash-preserved", namespace: oldNamespace), intent)
    }

    func testUnknownRegistrationCannotBecomeUnlinkedFreshAuthentication() async throws {
        let session = try session()
        defer { cleanup(session) }
        var calls = 0
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { _, _, _ in
                calls += 1
                throw IMAPIError(
                    status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled request not observed")
            })
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "new_account", password: UUID().uuidString,
                otp: "", proposedRequestID: UUID().uuidString, explicitNew: true)
            XCTFail("Explicit recovery cannot silently create an unrelated login")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "PREVIOUS_AUTHENTICATION_REQUIRED") }
        XCTAssertEqual(calls, 0)
        do {
            _ = try await coordinator.authenticate(
                mode: "register", username: "new_account", password: UUID().uuidString,
                otp: "", proposedRequestID: UUID().uuidString, explicitNew: false)
            XCTFail("Expected unresolved registration")
        } catch {}
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "new_account", password: UUID().uuidString,
                otp: "", proposedRequestID: UUID().uuidString, explicitNew: true,
                descriptor: .object(["features": .object(["auth_retirement_chain": .bool(true)])]))
            XCTFail("Cannot assume that unresolved registration created an account")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "ORIGINAL_REGISTRATION_REQUIRED") }
        XCTAssertEqual(calls, 1)
    }

    func testFreshProofRetiresCompleteSavedChainWithoutChangingOldBodies() async throws {
        let session = try session()
        defer { cleanup(session) }
        let ids = (0..<3).map { _ in UUID().uuidString }
        let descriptor: IMJSON = .object(["features": .object(["auth_retirement_chain": .bool(true)])])
        var bodies: [IMJSON] = []
        let result = response().setting("deviceId", .string(UUID().uuidString)).setting(
            "familyId", .string(UUID().uuidString))
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { _, body, _ in
                bodies.append(.object(body))
                if bodies.count < 3 {
                    throw IMAPIError(
                        status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled unknown successor")
                }
                return result
            })
        for index in 0..<3 {
            do {
                _ = try await coordinator.authenticate(
                    mode: "login", username: "credential_user", password: UUID().uuidString,
                    otp: String(index), proposedRequestID: ids[index], explicitNew: index > 0,
                    descriptor: descriptor)
                XCTAssertEqual(index, 2)
            } catch { XCTAssertLessThan(index, 2) }
        }
        XCTAssertEqual(bodies.count, 3)
        XCTAssertEqual(bodies[1]["previousRequestIds"].list, [.string(ids[0])])
        XCTAssertEqual(bodies[2]["previousRequestIds"].list, [.string(ids[0]), .string(ids[1])])
        XCTAssertEqual(bodies[2]["previousRequestId"].text, ids[1])
        let retained = try XCTUnwrap(
            session.store.list("auth-state", namespace: metadata(session)).first {
                $0["id"].text == "request:" + ids[0]
            })
        XCTAssertEqual(retained["body"]["requestId"].text, ids[0])
        XCTAssertEqual(retained["body"]["previousRequestIds"], .null)
        XCTAssertEqual(retained["body"]["password"], .null)
        XCTAssertEqual(retained["body"]["otp"], .null)
    }

    func testMissingRetirementCapabilityKeepsOriginalIntentAndMakesNoRequest() async throws {
        let session = try session()
        defer { cleanup(session) }
        let originalID = UUID().uuidString
        var calls = 0
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { _, _, _ in
                calls += 1
                throw IMAPIError(status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled unknown initial")
            })
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "credential_user", password: UUID().uuidString,
                otp: "", proposedRequestID: originalID, explicitNew: false)
            XCTFail("Expected unresolved initial request")
        } catch {}
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "credential_user", password: UUID().uuidString,
                otp: "", proposedRequestID: UUID().uuidString, explicitNew: true)
            XCTFail("Missing retirement capability must block new successor")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "AUTH_RETIREMENT_CHAIN_UNSUPPORTED") }
        XCTAssertEqual(calls, 1)
        XCTAssertEqual(try coordinator.pendingInitial("credential_user")?["requestId"].text, originalID)
    }
}
