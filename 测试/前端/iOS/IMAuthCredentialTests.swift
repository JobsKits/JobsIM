//
//  IMAuthCredentialTests.swift
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
final class IMAuthCredentialTests: XCTestCase {
    private let profile = "credential-test-profile"
    private let deployment = "24ac6520-f87d-4bb0-9ff7-df3b1500c0c2"
    private let userID = "e1dfe1c3-47eb-43b4-9895-9c14c3dce59a"
    private let deviceID = "39df1a0e-9fb8-439e-8d0d-eab8cbfa0ed3"

    private func response(generation: String = "1") -> IMJSON {
        return .object([
            "token": .string("access-" + generation), "refreshToken": .string("refresh-" + generation),
            "familyId": .string("76ec6b98-966c-47d2-8e59-bc6d69dd906e"),
            "refreshGeneration": .string(generation), "deviceId": .string(deviceID),
            "user": .object(["id": .string(userID), "username": .string("credential_user")]),
            "restoreEpoch": .string("1"), "sourceContextId": .string("personal"),
            "accessExpiresAt": .string("2026-11-15T12:15:00.123456Z"),
            "idleExpiresAt": .string("2026-12-15T12:00:00.123456Z"),
            "absoluteExpiresAt": .string("2027-02-13T12:00:00.123456Z"),
        ])
    }

    private func pair(_ response: IMJSON) throws -> IMCredentialPair {
        return try IMCredentialPair(
            response: response, profile: profile, deploymentID: deployment,
            requestID: "credential-request-id")
    }

    private func isolatedAuthentication() throws -> IMSession {
        let session = IMSession(store: try IMLocalStore(path: ":memory:"))
        session.profile = "mfa-credential-test:" + UUID().uuidString
        session.deployment = deployment
        session.restoreEpoch = "1"
        return session
    }

    private func authenticationRows(_ session: IMSession) throws -> [IMJSON] {
        let namespace = IMNamespace(
            profile: session.profile, account: "__auth_state__", demo: false,
            restoreEpoch: "1", deploymentID: deployment, sourceContextID: "personal")
        return try session.store.list("auth-state", namespace: namespace)
    }

    func testServerMFAChallengeUsesFreshAttemptThenPinsUnknownOTP() async throws {
        let session = try isolatedAuthentication()
        let ids = (0..<3).map { _ in UUID().uuidString }
        let password = "controlled-password-" + UUID().uuidString
        var sent: [IMJSON] = []
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { _, body, _ in
                sent.append(.object(body))
                if sent.count < 3 {
                    throw IMAPIError(
                        status: 403, code: sent.count == 1 ? "MFA_REQUIRED" : "MFA_INVALID",
                        message: "Controlled server challenge")
                }
                throw IMAPIError(status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled lost response")
            },
            secureWrite: { _, _ in
                XCTFail("A rejected or unknown request cannot install credentials")
            })
        for index in ids.indices {
            do {
                _ = try await coordinator.authenticate(
                    mode: "login", username: "credential_user", password: password,
                    otp: index == 0 ? "" : index == 1 ? "123456" : "654321",
                    proposedRequestID: ids[index], explicitNew: false)
                XCTFail("Controlled challenge and lost delivery must throw")
            } catch let error as IMAPIError {
                XCTAssertEqual(error.code, index == 0 ? "MFA_REQUIRED" : index == 1 ? "MFA_INVALID" : "TRANSPORT_UNKNOWN")
            }
            XCTAssertEqual(try coordinator.pendingInitial("credential_user")?["requestId"].text,
                           index == 2 ? ids[2] : nil)
        }
        XCTAssertEqual(sent.map { $0["requestId"].text }, ids)
        XCTAssertEqual(sent.map { $0["otp"].text }, ["", "123456", "654321"])
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "credential_user", password: password,
                otp: "999999", proposedRequestID: UUID().uuidString, explicitNew: false)
            XCTFail("An unknown request must keep its exact original OTP")
        } catch let error as IMAPIError {
            XCTAssertEqual(error.code, "ORIGINAL_AUTHENTICATION_PROOF_CHANGED")
        }
        XCTAssertEqual(sent.count, 3)
        let stored = String(decoding: try JSONEncoder.make().encode(authenticationRows(session)), as: UTF8.self)
        XCTAssertFalse(stored.contains(password))
        XCTAssertFalse(stored.contains("654321"))
    }

    func testReplayAndMissingFactorRequireConfirmed403BeforeClosingFreshIntent() async throws {
        for code in ["MFA_REPLAY", "MFA_NOT_ENROLLED"] {
            for status in [403, 0, 503] {
                let session = try isolatedAuthentication()
                let requestID = UUID().uuidString
                let coordinator = IMAuthCoordinator(
                    session: session,
                    request: { _, _, _ in
                        throw IMAPIError(status: status, code: code, message: "Controlled typed response")
                    },
                    secureWrite: { _, _ in
                        XCTFail("A rejected request cannot install credentials")
                    })
                do {
                    _ = try await coordinator.authenticate(
                        mode: "login", username: "credential_user", password: "controlled-test-password",
                        otp: "123456", proposedRequestID: requestID, explicitNew: false)
                    XCTFail("Controlled response must throw")
                } catch let error as IMAPIError {
                    XCTAssertEqual(error.status, status)
                }
                XCTAssertEqual(try coordinator.pendingInitial("credential_user")?["requestId"].text,
                               status == 403 ? nil : requestID)
            }
        }
    }

    func testRejectedMFAReauthenticationRestoresUnknownPredecessorAndProof() async throws {
        let session = try isolatedAuthentication()
        let originalID = UUID().uuidString
        let rejectedID = UUID().uuidString
        let successorID = UUID().uuidString
        let originalPassword = "original-password-" + UUID().uuidString
        let descriptor: IMJSON = .object(["features": .object(["auth_retirement_chain": .bool(true)])])
        var sent: [(String, IMJSON)] = []
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { path, body, _ in
                sent.append((path, .object(body)))
                if sent.count == 2 {
                    throw IMAPIError(status: 403, code: "MFA_REQUIRED", message: "Controlled successor rejection")
                }
                throw IMAPIError(status: 0, code: "TRANSPORT_UNKNOWN", message: "Controlled unknown delivery")
            },
            secureWrite: { _, _ in
                XCTFail("No controlled request returns credentials")
            })
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "credential_user", password: originalPassword,
                otp: "", proposedRequestID: originalID, explicitNew: false)
            XCTFail("Original delivery is unknown")
        } catch {}
        let original = try XCTUnwrap(coordinator.pendingInitial("credential_user"))
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "credential_user", password: "replacement-password",
                otp: "", proposedRequestID: rejectedID, explicitNew: true, descriptor: descriptor)
            XCTFail("Successor requires server MFA")
        } catch let error as IMAPIError {
            XCTAssertEqual(error.code, "MFA_REQUIRED")
        }
        XCTAssertEqual(try coordinator.pendingInitial("credential_user"), original)
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "credential_user", password: originalPassword + "changed",
                otp: "", proposedRequestID: UUID().uuidString, explicitNew: false)
            XCTFail("Rejected successor cannot discard the original RAM proof")
        } catch let error as IMAPIError {
            XCTAssertEqual(error.code, "ORIGINAL_AUTHENTICATION_PROOF_CHANGED")
        }
        XCTAssertEqual(sent.count, 2)
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "credential_user", password: "replacement-password",
                otp: "654321", proposedRequestID: successorID, explicitNew: true, descriptor: descriptor)
            XCTFail("Controlled new successor delivery is unknown")
        } catch let error as IMAPIError {
            XCTAssertEqual(error.code, "TRANSPORT_UNKNOWN")
        }
        XCTAssertEqual(sent.last?.0, "auth/reauthenticate")
        XCTAssertEqual(sent.last?.1["requestId"].text, successorID)
        XCTAssertEqual(sent.last?.1["previousRequestId"].text, originalID)
        XCTAssertEqual(sent.last?.1["previousRequestIds"].list, [.string(originalID)])
        XCTAssertEqual(sent.last?.1["otp"].text, "654321")
        let retained = try XCTUnwrap(authenticationRows(session).first { $0["id"].text == "request:" + originalID })
        XCTAssertEqual(retained, original)
        XCTAssertEqual(try authenticationRows(session).first { $0["id"].text == "request:" + rejectedID }?["state"].text, "failed")
    }

    func testMFAResponseOnUnknownReplayCannotDiscardOriginalNonceOrProof() async throws {
        let session = try isolatedAuthentication()
        let originalID = UUID().uuidString
        var sent: [IMJSON] = []
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { _, body, _ in
                sent.append(.object(body))
                throw IMAPIError(
                    status: sent.count == 1 ? 0 : 403,
                    code: sent.count == 1 ? "TRANSPORT_UNKNOWN" : "MFA_INVALID",
                    message: "Controlled replay response")
            },
            secureWrite: { _, _ in
                XCTFail("Unknown original cannot install credentials")
            })
        for _ in 0..<2 {
            do {
                _ = try await coordinator.authenticate(
                    mode: "login", username: "credential_user", password: "controlled-test-password",
                    otp: "123456", proposedRequestID: originalID, explicitNew: false)
                XCTFail("Controlled requests must throw")
            } catch {}
        }
        XCTAssertEqual(sent.count, 2)
        XCTAssertEqual(sent[0], sent[1])
        XCTAssertEqual(try coordinator.pendingInitial("credential_user")?["requestId"].text, originalID)
        XCTAssertEqual(try coordinator.pendingInitial("credential_user")?["state"].text, "unknown")
        do {
            _ = try await coordinator.authenticate(
                mode: "login", username: "credential_user", password: "controlled-test-password",
                otp: "654321", proposedRequestID: UUID().uuidString, explicitNew: false)
            XCTFail("A server error during replay cannot authorize changing its proof")
        } catch let error as IMAPIError {
            XCTAssertEqual(error.code, "ORIGINAL_AUTHENTICATION_PROOF_CHANGED")
        }
        XCTAssertEqual(sent.count, 2)
    }

    func testAcceptedButUnsecuredAuthenticationCannotBecomeFreshMFAIntent() async throws {
        let session = try isolatedAuthentication()
        let originalID = UUID().uuidString
        var requests = 0
        var secureWrites = 0
        let accepted = response()
        let coordinator = IMAuthCoordinator(
            session: session,
            request: { _, _, _ in
                requests += 1
                if requests == 1 {
                    return accepted
                }
                throw IMAPIError(status: 403, code: "MFA_REQUIRED", message: "Controlled later replay rejection")
            },
            secureWrite: { _, _ in
                secureWrites += 1
                throw IMAPIError(status: 0, code: "SECURE_STORAGE_UNAVAILABLE", message: "Controlled persistence failure")
            })
        for _ in 0..<2 {
            do {
                _ = try await coordinator.authenticate(
                    mode: "login", username: "credential_user", password: "controlled-test-password",
                    otp: "123456", proposedRequestID: originalID, explicitNew: false)
                XCTFail("Securing and replay responses must remain unresolved")
            } catch {}
        }
        let original = try XCTUnwrap(coordinator.pendingInitial("credential_user"))
        XCTAssertEqual(original["requestId"].text, originalID)
        XCTAssertEqual(original["state"].text, "unknown")
        XCTAssertEqual(original["resultUser"]["id"].text, userID)
        XCTAssertEqual(secureWrites, 1)
        XCTAssertNil(session.token)
    }

    func testPairStrictnessAndRotationKeepsDeviceNamespace() throws {
        let first = try pair(response())
        let next = try pair(response(generation: "2"))
        try next.validateRotation(from: first)
        XCTAssertEqual(first.namespace, next.namespace)
        XCTAssertThrowsError(try pair(response().setting("refreshToken", .string(""))))
        XCTAssertThrowsError(try pair(response().setting("refreshGeneration", .string("01"))))
        XCTAssertThrowsError(try pair(response().setting("sourceContextId", .string("other-source"))))
        XCTAssertThrowsError(
            try pair(response().setting("absoluteExpiresAt", .string("2026-11-01T00:00:00Z"))))
        let skipped = try pair(response(generation: "3"))
        XCTAssertThrowsError(try skipped.validateRotation(from: first))
        let newDevice = try pair(response(generation: "2").setting("deviceId", .string(UUID().uuidString)))
        XCTAssertNotEqual(first.namespace, newDevice.namespace)
        XCTAssertThrowsError(try newDevice.validateRotation(from: first))
        XCTAssertThrowsError(try IMCredentialPair.decode("legacy-seven-day-bearer"))
    }

    func testKeychainStoresCompletePairAsSingleItem() throws {
        let key = "credential-test:" + UUID().uuidString
        defer { try? IMKeychain.write(nil, key: key) }
        let first = try pair(response())
        try IMKeychain.write(first.serialized(), key: key)
        let firstStored = try XCTUnwrap(IMKeychain.readValidated(key))
        XCTAssertTrue(try IMCredentialPair.decode(firstStored) == first)
        let next = try pair(response(generation: "2"))
        try IMKeychain.write(next.serialized(), key: key)
        let nextStored = try XCTUnwrap(IMKeychain.readValidated(key))
        XCTAssertTrue(try IMCredentialPair.decode(nextStored) == next)
        XCTAssertFalse(nextStored.contains("access-1"))
        XCTAssertFalse(nextStored.contains("refresh-1"))
    }

    func testNamespaceHandoffFreezesOldOutboxAndRollsBackPartialCommit() throws {
        let store = try IMLocalStore(path: ":memory:")
        let legacy = IMNamespace(profile: profile, account: userID, demo: false)
        let next = try pair(response()).namespace
        let metadata = IMNamespace(profile: profile, account: "__auth_state__", demo: false)
        let intent: IMJSON = .object([
            "id": .string("original-intent"), "clientMsgId": .string("original-intent"),
            "conversationId": .string("conversation"), "type": .string("text"),
            "text": .string("Preserve old device intent"),
            "sendAdmission": .string("original-admission"), "restoreEpoch": .string("1"),
            "status": .string("queued"),
        ])
        try store.enqueue(intent, conversation: "conversation", namespace: legacy)
        XCTAssertThrowsError(
            try store.commitAuthenticationBinding(
                previousNamespaces: [legacy],
                newNamespace: next, metadata: [.object([:])], metadataNamespace: metadata))
        XCTAssertEqual(try store.pending(namespace: legacy).count, 1)
        XCTAssertTrue(try store.list("auth-state", namespace: metadata).isEmpty)
        let binding: IMJSON = .object(["id": .string("binding:" + userID), "deviceId": .string(deviceID)])
        try store.commitAuthenticationBinding(
            previousNamespaces: [legacy], newNamespace: next,
            metadata: [binding], metadataNamespace: metadata)
        XCTAssertTrue(try store.pending(namespace: legacy).isEmpty)
        XCTAssertTrue(try store.pending(namespace: next).isEmpty)
        XCTAssertTrue(try store.originalOutbox(clientID: "original-intent", namespace: legacy) == intent)
        let visible = try XCTUnwrap(store.list("messages:conversation", namespace: legacy).first)
        XCTAssertEqual(visible["status"].text, "frozen")
        XCTAssertEqual(visible["freezeReason"].text, "authentication_device_changed")
        XCTAssertEqual(try store.list("auth-state", namespace: metadata), [binding])
    }
}
