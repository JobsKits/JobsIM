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

    private func deletionView(_ request: IMJSON, id: String, state: String = "requested", version: String = "1", cancel: String = "") -> IMJSON {
        return .object(["deletionId": .string(id), "requestId": request["id"], "status": .string(state),
                        "requestVersion": .string(version), "restoreEpoch": request["restoreEpoch"],
                        "cancelUntil": state == "requested" ? .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(600))) : .null,
                        "cancelRequestId": .string(cancel), "requiresNewLogin": .bool(true)])
    }

    func testDeletionUnknownDoesNotBorrowSuccessorFamilyOrStoreProof() async throws {
        let value = try session()
        defer { cleanup(value) }
        var writes = 0
        let service = IMAccountDeletionService(session: value, username: "credential_user", request: { path, _, _ in
            if path == "bootstrap" {
                return .object(["deploymentId": .string(self.deployment), "restoreEpoch": .string("1"), "features": .object(["own_account_deletion": .bool(true)])])
            }
            if path == "auth/deletion-results" {
                throw IMAPIError(status: 404, code: "ACCOUNT_DELETION_NOT_FOUND", message: "not found")
            }
            writes += 1
            throw IMAPIError(status: 403, code: "DEPENDENCY_DENIED", message: "uncertain protected operation")
        })
        let row = try await service.prepare()
        let proof = try IMAccountDeletionService.credential(password: " original secret ", otp: "123456")
        do {
            _ = try await service.perform(row["id"].text, credential: proof, action: .submit)
            XCTFail("Expected unknown dependency result")
        } catch {}
        XCTAssertEqual(try service.rows().first?["state"].text, "unknown")
        XCTAssertTrue(try value.accountOperationBlocked(XCTUnwrap(value.currentCredentialPair)))
        let bytes = try JSONEncoder.make().encode(service.rows())
        XCTAssertFalse(String(decoding: bytes, as: UTF8.self).contains("original secret"))
        XCTAssertFalse(String(decoding: bytes, as: UTF8.self).contains("123456"))
        let next = try IMCredentialPair(response: response().setting("familyId", .string(UUID().uuidString)),
                                        profile: value.profile, deploymentID: deployment, requestID: "successor-request")
        value.credentialPair = next
        value.token = next.accessToken
        XCTAssertFalse(try value.accountOperationBlocked(next))
        let unseen = try await service.perform(row["id"].text, credential: proof, action: .check)
        XCTAssertNil(unseen)
        do {
            _ = try await service.perform(row["id"].text, credential: proof, action: .retry)
            XCTFail("A successor cannot create the unseen original operation")
        } catch {}
        XCTAssertEqual(writes, 1)
        XCTAssertEqual(value.currentCredentialPair, next)
    }

    func testDeletionUnknownCancelKeepsOriginalNonceAndRejectsWrongPurpose() async throws {
        let value = try session()
        defer { cleanup(value) }
        let deletionID = UUID().uuidString.lowercased()
        var original: IMJSON = .null
        var stored: IMJSON?
        var cancelBodies: [IMJSON] = []
        var wrongPurpose = true
        let service = IMAccountDeletionService(session: value, username: "credential_user", request: { path, body, _ in
            switch path {
            case "bootstrap":
                return .object(["deploymentId": .string(self.deployment), "restoreEpoch": .string("1"), "features": .object(["own_account_deletion": .bool(true)])])
            case "auth/deletion-results":
                guard let stored else { throw IMAPIError(status: 404, code: "ACCOUNT_DELETION_NOT_FOUND", message: "not found") }
                return stored
            case "auth/deletion-requests":
                stored = self.deletionView(original, id: deletionID)
                return stored!
            case "auth/data-rights/proofs":
                return .object(["deletionId": .string(deletionID), "requestVersion": .string("1"), "restoreEpoch": .string("1"),
                                "purpose": .string(wrongPurpose ? "query" : body["purpose"]!.text),
                                "expiresAt": .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(240))),
                                "proof": .string(String(repeating: "p", count: 64))])
            case "auth/data-rights/query":
                return stored!
            case "auth/data-rights/cancel":
                cancelBodies.append(.object(body))
                if cancelBodies.count == 1 { throw IMAPIError(status: 0, code: "UNKNOWN", message: "lost response") }
                stored = self.deletionView(original, id: deletionID, state: "cancelled", version: "2", cancel: body["requestId"]!.text)
                return stored!
            default:
                throw IMSession.SessionError.invalidInstance
            }
        })
        original = try await service.prepare()
        let proof = try IMAccountDeletionService.credential(password: " proof secret ")
        _ = try await service.perform(original["id"].text, credential: proof, action: .submit)
        XCTAssertNil(value.currentCredentialPair)
        do {
            _ = try await service.cancel(original["id"].text, credential: proof)
            XCTFail("Wrong-purpose proof must be rejected before cancellation")
        } catch {}
        XCTAssertTrue(cancelBodies.isEmpty)
        wrongPurpose = false
        do { _ = try await service.cancel(original["id"].text, credential: proof) } catch {}
        XCTAssertEqual(try service.rows().first?["cancelSent"], .bool(true))
        do { _ = try await service.cancel(original["id"].text, credential: proof) } catch {}
        XCTAssertEqual(cancelBodies.count, 1)
        _ = try await service.query(original["id"].text, credential: proof)
        XCTAssertEqual(try service.rows().first?["cancelSent"], .bool(false))
        _ = try await service.cancel(original["id"].text, credential: proof)
        XCTAssertEqual(cancelBodies.count, 2)
        XCTAssertEqual(cancelBodies[0]["requestId"], cancelBodies[1]["requestId"])
        XCTAssertEqual(try service.rows().first?["state"].text, "cancelled")
        XCTAssertEqual(try service.rows().first?["proof"], .null)
    }

    func testDeletionFloorBlocksLateWritersAcrossEpochsPreservesOtherAccountsAndProfiles() throws {
        let value = try session()
        defer { cleanup(value) }
        let pair = try XCTUnwrap(value.currentCredentialPair)
        let old = pair.namespace
        let future = IMNamespace(profile: value.profile, account: userID, demo: false, restoreEpoch: "2", deploymentID: deployment, sourceContextID: "personal")
        let other = IMNamespace(profile: value.profile, account: UUID().uuidString.lowercased(), demo: false, restoreEpoch: "1", deploymentID: deployment, sourceContextID: "personal")
        let neighbor = IMNamespace(profile: "other-profile", account: userID, demo: false, restoreEpoch: "1", deploymentID: deployment, sourceContextID: "personal")
        for ns in [old, future, other, neighbor] {
            try value.store.save([.object(["id": .string("private-row"), "text": .string(ns == old || ns == future ? "erased unique private fixture" : "unrelated lawful fixture")])], collection: "messages", namespace: ns)
        }
        let owned = "JobsIMDataExport-" + UUID().uuidString
        try value.store.registerAccountPrivateFile(namespace: old, kind: "export", name: owned)
        let keys = try value.store.beginAccountDeletionCleanup(profile: value.profile, deployment: deployment, user: userID, originalNamespace: old.key)
        XCTAssertEqual(Set(keys), Set([old.key, future.key]))
        XCTAssertThrowsError(try value.store.list("messages", namespace: old))
        XCTAssertThrowsError(try value.store.save([.object(["id": .string("late")])], collection: "messages", namespace: future))
        XCTAssertThrowsError(try value.store.commitAuthenticationBinding(previousNamespaces: [], newNamespace: old, metadata: [], metadataNamespace: metadata(value)))
        XCTAssertTrue(try value.accountOperationBlocked(pair))
        XCTAssertEqual(try value.store.accountDeletionFiles(profile: value.profile, deployment: deployment, user: userID).first?["name"].text, owned)
        try value.store.finishAccountDeletionCleanup(profile: value.profile, deployment: deployment, user: userID)
        XCTAssertTrue(try value.store.accountDeletionFiles(profile: value.profile, deployment: deployment, user: userID).isEmpty)
        XCTAssertEqual(try value.store.list("messages", namespace: other).count, 1)
        XCTAssertEqual(try value.store.list("messages", namespace: neighbor).count, 1)
        let restart = try reopened(value)
        XCTAssertTrue(try restart.store.accountIsErased(profile: value.profile, deployment: deployment, user: userID))
        XCTAssertThrowsError(try restart.store.save([.object(["id": .string("resurrection")])], collection: "messages", namespace: old))
        let path = try XCTUnwrap(databasePaths[value.profile])
        let wal = URL(fileURLWithPath: path.path + "-wal")
        if FileManager.default.fileExists(atPath: wal.path) {
            let data = try Data(contentsOf: wal)
            XCTAssertFalse(String(decoding: data, as: UTF8.self).contains("erased unique private fixture"))
        }
    }

    func testGroupUnknownReusesOriginalNonceAndObservedPermission() async throws {
        let value = try session()
        defer {
            cleanup(value)
        }
        let group = UUID().uuidString.lowercased()
        let observed: IMJSON = .object(["version": .string("9007199254740993"),
                                      "membershipGeneration": .string("7"), "roleGeneration": .string("8")])
        var writes: [[String: IMJSON]] = []
        var finish = false
        let service = IMGroupGovernanceService(session: value, request: { _, method, body in
            if method == "GET" {
                throw IMAPIError(status: 404, code: "GROUP_OPERATION_NOT_FOUND", message: "not found")
            }
            writes.append(body)
            guard finish else {
                throw IMAPIError(status: 0, code: "TRANSPORT_UNKNOWN", message: "unknown")
            }
            return .object(["requestId": body["requestId"]!, "status": .string("applied"), "action": body["action"]!, "version": .string("9007199254740994")])
        })
        do {
            _ = try await service.change(group, observed: observed, action: "settings",
                changes: ["muteAll": .bool(true), "slowSeconds": .number(5), "maxMembers": .number(500)])
            XCTFail("Expected unknown original operation")
        } catch {}
        XCTAssertEqual(writes.count, 1)
        let pending = try XCTUnwrap(service.pending(group))
        do {
            _ = try await service.reconcile(group, retry: false)
            XCTFail("Expected pure unseen lookup")
        } catch {}
        XCTAssertEqual(writes.count, 1)
        XCTAssertEqual(pending["payload"]["version"].text, "9007199254740993")
        finish = true
        _ = try await service.reconcile(group, retry: true)
        XCTAssertEqual(writes.count, 2)
        XCTAssertEqual(writes[0], writes[1])
        XCTAssertNil(try service.pending(group))
    }

    func testGroupInvitationCapabilityStaysInMemoryAcrossColdResume() async throws {
        let value = try session()
        defer {
            cleanup(value)
        }
        let group = UUID().uuidString.lowercased()
        let invite = UUID().uuidString.lowercased()
        let capability = "private-invitation-fixture-" + UUID().uuidString
        var firstID = ""
        let service = IMGroupGovernanceService(session: value, request: { _, method, body in
            if method == "GET" {
                throw IMAPIError(status: 404, code: "GROUP_OPERATION_NOT_FOUND", message: "not found")
            }
            firstID = body["requestId"]!.text
            XCTAssertTrue(body["token"]?.text == capability)
            throw IMAPIError(status: 0, code: "TRANSPORT_UNKNOWN", message: "unknown")
        })
        let code = try service.invitationCode(group, invite: .object(["id": .string(invite), "token": .string(capability)]))
        do {
            _ = try await service.join(code)
            XCTFail("Expected unknown initial join")
        } catch {}
        let pending = try XCTUnwrap(service.pending(group))
        XCTAssertEqual(pending["id"].text, firstID)
        XCTAssertEqual(pending["payload"]["token"], .null)
        let bytes = try JSONEncoder.make().encode(pending)
        XCTAssertFalse(String(decoding: bytes, as: UTF8.self).contains(capability))
        let cold = try reopened(value)
        cold.user = value.user
        cold.credentialPair = value.credentialPair
        cold.token = value.token
        let restored = IMGroupGovernanceService(session: cold, request: { _, method, body in
            if method == "GET" {
                throw IMAPIError(status: 404, code: "GROUP_OPERATION_NOT_FOUND", message: "not found")
            }
            XCTAssertEqual(body["requestId"]?.text, firstID)
            XCTAssertTrue(body["token"]?.text == capability)
            return .object(["requestId": body["requestId"]!, "id": .string(UUID().uuidString.lowercased()), "state": .string("pending")])
        })
        _ = try await restored.join(code)
        XCTAssertNil(try restored.pending(group))
    }

    func testGroupSuccessorFamilyOnlyQueriesHistoricalOriginalAndTypedTargetMustMatch() async throws {
        let value = try session()
        defer { cleanup(value) }
        let group = UUID().uuidString.lowercased()
        let invite = UUID().uuidString.lowercased()
        var writes = 0
        var authoritative: IMJSON?
        let service = IMGroupGovernanceService(session: value, request: { _, method, _ in
            if method == "GET" {
                guard let authoritative else { throw IMAPIError(status: 404, code: "GROUP_OPERATION_NOT_FOUND", message: "not found") }
                return authoritative
            }
            writes += 1
            throw IMAPIError(status: 0, code: "UNKNOWN", message: "lost response")
        })
        let observed: IMJSON = .object(["version": .string("2"), "membershipGeneration": .string("3"), "roleGeneration": .string("4")])
        do { try await service.revoke(group, observed: observed, invite: invite) } catch {}
        let row = try XCTUnwrap(service.pending(group))
        let next = try IMCredentialPair(response: response().setting("deviceId", .string(UUID().uuidString))
            .setting("familyId", .string(UUID().uuidString)), profile: value.profile, deploymentID: deployment, requestID: "successor-group-request")
        value.credentialPair = next
        value.token = next.accessToken
        XCTAssertNotNil(try service.pending(group))
        do { _ = try await service.reconcile(group, retry: true) } catch {}
        XCTAssertEqual(writes, 1)
        authoritative = .object(["requestId": row["id"], "status": .string("applied"), "action": .string("invite_revoke"), "targetId": .string(UUID().uuidString), "state": .string("revoked")])
        do {
            _ = try await service.reconcile(group, retry: false)
            XCTFail("A different typed target cannot complete this original operation")
        } catch {}
        XCTAssertNotNil(try service.pending(group))
        authoritative = authoritative?.setting("targetId", .string(invite))
        _ = try await service.reconcile(group, retry: false)
        XCTAssertNil(try service.pending(group))
        XCTAssertEqual(writes, 1)
        XCTAssertEqual(value.currentCredentialPair, next)
    }

    func testGroupJoinRejectsAnotherDeploymentAndSecretMetadata() async throws {
        let value = try session()
        defer {
            cleanup(value)
        }
        var calls = 0
        let service = IMGroupGovernanceService(session: value, request: { _, _, _ in
            calls += 1
            return .null
        })
        let group = UUID().uuidString.lowercased()
        let other: IMJSON = .object(["deploymentId": .string(UUID().uuidString.lowercased()),
                                    "conversationId": .string(group), "inviteId": .string(UUID().uuidString.lowercased()),
                                    "token": .string("fixture-invitation")])
        let code = "JobsIM-Invite-1:" + (try JSONEncoder.make().encode(other)).base64EncodedString()
        do {
            _ = try await service.join(code)
            XCTFail("Cross-deployment invitation accepted")
        } catch {}
        do {
            _ = try await service.create(group, route: "join-requests", body: ["token": .string("must-not-persist")])
            XCTFail("Invitation proof entered ordinary durable metadata")
        } catch {}
        XCTAssertEqual(calls, 0)
        XCTAssertNil(try service.pending(group))
    }

    func testRealHTTPSPasswordChangeUsesPhysicalStoreAndPreservesLaterFamily() async throws {
        guard let file = Bundle(for: Self.self).url(forResource: "OwnPasswordPublic", withExtension: "json") else {
            throw XCTSkip("Explicit current canonical HTTPS fixture not installed")
        }
        let configuration = try JSONDecoder.make().decode(IMJSON.self, from: Data(contentsOf: file))
        let address = configuration["url"].text
        let certificate = try IMInstanceCertificate(address: address,
            certificate: Data(configuration["caPEM"].text.utf8), verifiedFingerprint: configuration["fingerprint"].text)
        let base = try XCTUnwrap(URL(string: address))
        let api = IMAPIClient(baseURL: IMSession.apiURL(base), certificate: certificate)
        let bootstrap = try await api.request("bootstrap")
        XCTAssertTrue(bootstrap["deploymentId"] == configuration["deploymentId"])
        XCTAssertTrue(bootstrap["features"]["own_password_change"] == .bool(true))
        let database = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".sqlite")
        let value = IMSession(store: try IMLocalStore(path: database.path))
        value.profile = "canonical-password:" + UUID().uuidString
        value.deployment = bootstrap["deploymentId"].text
        value.restoreEpoch = bootstrap["restoreEpoch"].text
        value.baseURL = base
        value.api = api
        let oldInjected = ProcessInfo.processInfo.environment["JOBS_IM_SYNC_INJECTED_TESTS"]
        setenv("JOBS_IM_SYNC_INJECTED_TESTS", "0", 1)
        defer {
            value.stopAuthoritativeSync()
            if let oldInjected {
                setenv("JOBS_IM_SYNC_INJECTED_TESTS", oldInjected, 1)
            } else {
                unsetenv("JOBS_IM_SYNC_INJECTED_TESTS")
            }
            try? IMKeychain.write(nil, key: value.profile + ":" + value.user["id"].text)
            UserDefaults.standard.removeObject(forKey: "user:" + value.profile)
        }
        let username = "ios_pw_" + UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(16).lowercased()
        let current = UUID().uuidString + UUID().uuidString
        let replacement = UUID().uuidString + UUID().uuidString
        _ = try await value.authenticate(mode: "register", username: username, password: current, requestID: UUID().uuidString)
        let pair = try XCTUnwrap(value.currentCredentialPair)
        let service = IMPasswordChangeService(session: value, username: username)
        let row = try await service.prepare()
        let proof = try IMPasswordChangeService.proof(current: current, replacement: replacement,
                                                     confirmation: replacement, otp: "")
        var result = try await service.perform(row["id"].text, proof: proof, action: .submit)
        for _ in 0..<100 {
            if result?["status"].text == "changed" {
                break
            }
            try await Task.sleep(for: .milliseconds(100))
            result = try await service.perform(row["id"].text, proof: proof, action: .check)
        }
        XCTAssertTrue(result?["status"].text == "changed")
        XCTAssertNil(value.currentCredentialPair)
        XCTAssertTrue(try IMPasswordChangeService.blocks(value, pair: pair))
        do {
            _ = try await api.request("me", token: pair.accessToken)
            XCTFail("Original access remained valid after actual password change")
        } catch let error as IMAPIError {
            XCTAssertEqual(error.status, 401)
        }
        do {
            _ = try await value.authenticate(mode: "login", username: username, password: current, requestID: UUID().uuidString)
            XCTFail("Original password remained valid after actual password change")
        } catch let error as IMAPIError {
            XCTAssertTrue([401, 403].contains(error.status))
        }
        _ = try await value.authenticate(mode: "login", username: username, password: replacement, requestID: UUID().uuidString)
        let successor = try XCTUnwrap(value.currentCredentialPair)
        XCTAssertTrue(successor.familyID != pair.familyID && successor.userID == pair.userID)
        _ = try await service.perform(row["id"].text, proof: proof, action: .check)
        XCTAssertTrue(value.currentCredentialPair?.familyID == successor.familyID)
        let metadata = try JSONEncoder.make().encode(IMJSON.array(service.rows()))
        let text = String(decoding: metadata, as: UTF8.self)
        XCTAssertFalse(text.contains(current))
        XCTAssertFalse(text.contains(replacement))
        let reopened = IMSession(store: try IMLocalStore(path: database.path))
        reopened.profile = value.profile
        reopened.deployment = value.deployment
        reopened.restoreEpoch = value.restoreEpoch
        let restored = IMPasswordChangeService(session: reopened, username: username)
        XCTAssertTrue(try restored.rows().first?["state"].text == "changed")
        _ = try await api.request("auth/logout", method: "POST", body: [:], token: successor.accessToken)
    }

    func testPasswordChangeUnknownUsesOriginalAndNeverPersistsProof() async throws {
        let value = try session()
        defer {
            cleanup(value)
        }
        let pair = try XCTUnwrap(value.currentCredentialPair)
        let old = UUID().uuidString
        let replacement = UUID().uuidString
        let proof = try IMPasswordChangeService.proof(current: old, replacement: replacement, confirmation: replacement, otp: "123456")
        var writes = 0
        var checks = 0
        var finish = false
        let service = IMPasswordChangeService(session: value, username: pair.username, request: { path, body, token in
            if path == "bootstrap" {
                return .object(["deploymentId": .string(self.deployment), "restoreEpoch": .string("1"),
                                "features": .object(["own_password_change": .bool(true)])])
            }
            if path == "auth/password-change-results" {
                checks += 1
                XCTAssertNil(token)
                XCTAssertNil(body["otp"])
                throw IMAPIError(status: 404, code: "PASSWORD_CHANGE_OPERATION_NOT_FOUND", message: "not found")
            }
            writes += 1
            XCTAssertEqual(token, pair.accessToken)
            guard finish else {
                throw IMAPIError(status: 0, code: "TRANSPORT_UNKNOWN", message: "unknown")
            }
            return .object(["requestId": body["requestId"]!, "restoreEpoch": .string("1"), "status": .string("changed"),
                            "credentialGeneration": .string("2"), "securityVersion": .string("2"),
                            "current": .bool(true), "requiresNewLogin": .bool(true)])
        })
        let row = try await service.prepare()
        do {
            _ = try await service.perform(row["id"].text, proof: proof, action: .submit)
            XCTFail("Expected unknown response")
        } catch {}
        XCTAssertEqual(writes, 1)
        XCTAssertTrue(try IMPasswordChangeService.blocks(value, pair: pair))
        let stored = try service.rows()
        let bytes = try JSONEncoder.make().encode(IMJSON.array(stored))
        let text = String(decoding: bytes, as: UTF8.self)
        XCTAssertFalse(text.contains(old))
        XCTAssertFalse(text.contains(replacement))
        XCTAssertFalse(text.contains("123456"))
        let unknown = try await service.perform(row["id"].text, proof: proof, action: .check)
        XCTAssertNil(unknown)
        XCTAssertEqual(writes, 1)
        finish = true
        let result = try await service.perform(row["id"].text, proof: proof, action: .retry)
        XCTAssertEqual(result?["status"].text, "changed")
        XCTAssertEqual(writes, 2)
        XCTAssertEqual(checks, 3)
        XCTAssertNil(value.currentCredentialPair)
        XCTAssertTrue(try IMPasswordChangeService.blocks(value, pair: pair))
        XCTAssertEqual(try service.rows().first?["id"], row["id"])
        let reopened = try reopened(value)
        let restored = IMPasswordChangeService(session: reopened, username: pair.username)
        XCTAssertEqual(try restored.rows().first?["state"].text, "changed")
    }

    func testPasswordChangeLateResultCannotSignOutSuccessorFamily() async throws {
        let value = try session()
        defer {
            cleanup(value)
        }
        let previous = try XCTUnwrap(value.currentCredentialPair)
        let replacement = UUID().uuidString
        let successor = try IMCredentialPair(response: response().setting("familyId", .string(UUID().uuidString)),
                                              profile: value.profile, deploymentID: deployment, requestID: "successor-auth-request")
        let proof = try IMPasswordChangeService.proof(current: UUID().uuidString, replacement: replacement, confirmation: replacement, otp: "")
        let service = IMPasswordChangeService(session: value, username: previous.username, request: { path, body, _ in
            if path == "bootstrap" {
                return .object(["deploymentId": .string(self.deployment), "restoreEpoch": .string("1"),
                                "features": .object(["own_password_change": .bool(true)])])
            }
            if path == "auth/password-change-results" {
                throw IMAPIError(status: 404, code: "PASSWORD_CHANGE_OPERATION_NOT_FOUND", message: "not found")
            }
            value.credentialPair = successor
            value.token = successor.accessToken
            return .object(["requestId": body["requestId"]!, "restoreEpoch": .string("1"), "status": .string("changed"),
                            "credentialGeneration": .string("2"), "securityVersion": .string("2"),
                            "current": .bool(false), "requiresNewLogin": .bool(true)])
        })
        let row = try await service.prepare()
        _ = try await service.perform(row["id"].text, proof: proof, action: .submit)
        XCTAssertEqual(value.currentCredentialPair?.familyID, successor.familyID)
        XCTAssertEqual(try service.rows().first?["state"].text, "changed")
    }

    func testPasswordChangeOriginalRemainsQueryableAfterCapabilityWithdrawalAndRestore() async throws {
        let value = try session()
        defer {
            cleanup(value)
        }
        let pair = try XCTUnwrap(value.currentCredentialPair)
        let replacement = UUID().uuidString
        let proof = try IMPasswordChangeService.proof(current: UUID().uuidString, replacement: replacement, confirmation: replacement, otp: "")
        var restored = false
        var mutationCalls = 0
        let service = IMPasswordChangeService(session: value, username: pair.username, request: { path, body, token in
            if path == "bootstrap" {
                return .object(["deploymentId": .string(self.deployment), "restoreEpoch": .string(restored ? "2" : "1"),
                                "features": .object(["own_password_change": .bool(!restored)])])
            }
            if path == "auth/password-change" {
                mutationCalls += 1
            }
            XCTAssertNil(token)
            return .object(["requestId": body["requestId"]!, "restoreEpoch": .string("1"), "status": .string("retired"),
                            "credentialGeneration": .string("2"), "securityVersion": .string("2"),
                            "current": .bool(false), "requiresNewLogin": .bool(false)])
        })
        let row = try await service.prepare()
        restored = true
        let result = try await service.perform(row["id"].text, proof: proof, action: .check)
        XCTAssertEqual(result?["status"].text, "retired")
        XCTAssertEqual(mutationCalls, 0)
    }

    func testPasswordChangeTransportRefusesPlainHTTPBeforeSendingProof() async throws {
        let api = IMAPIClient(baseURL: URL(string: "http://127.0.0.1:1/api/")!)
        do {
            _ = try await api.passwordChange("auth/password-change", body: [:], token: nil)
            XCTFail("Sensitive transport must require HTTPS")
        } catch {}
    }

    func testPasswordChangeProofValidationAndMetadataRejectSecrets() throws {
        XCTAssertThrowsError(try IMPasswordChangeService.proof(current: "same", replacement: "same", confirmation: "same", otp: ""))
        let value = try session()
        defer {
            cleanup(value)
        }
        let invalid: IMJSON = .object(["id": .string(UUID().uuidString), "currentPassword": .string(UUID().uuidString)])
        XCTAssertThrowsError(try value.store.comparePasswordChangeIntent(namespace: IMPasswordChangeService.metadata(value), previous: nil, next: invalid))
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
