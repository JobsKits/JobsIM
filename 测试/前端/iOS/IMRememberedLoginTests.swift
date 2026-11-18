//
//  IMRememberedLoginTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation
import CryptoKit
import JobsIMCore
import XCTest

@testable import JobsIM

@MainActor
final class IMRememberedLoginTests: XCTestCase {

    func testRememberIsOptInAndUsesOnlyThisProfile() async throws {
        try await withFixture { session, profile in
            let store = IMRememberedLoginStore.shared
            let other = "remember-other-" + UUID().uuidString
            XCTAssertNil(try store.restore(for: profile))
            try store.persistAfterVerifiedAuthentication(for: profile,
                                                         username: "remember_fixture",
                                                         password: "unique-fixture-secret-42",
                                                         enabled: false)
            XCTAssertNil(try store.restore(for: profile))
            try store.persistAfterVerifiedAuthentication(for: profile,
                                                         username: "remember_fixture",
                                                         password: "unique-fixture-secret-42",
                                                         enabled: true)
            XCTAssertEqual(try store.restore(for: profile)?.username, "remember_fixture")
            XCTAssertEqual(try store.restore(for: profile)?.password, "unique-fixture-secret-42")
            XCTAssertNil(try store.restore(for: other))
            XCTAssertNil(try store.restore(for: profile, username: "some_other_user"))
            XCTAssertNotNil(session.currentCredentialPair)
        }
    }

    func testExplicitOptOutErasesSavedPassword() async throws {
        try await withFixture { _, profile in
            let store = IMRememberedLoginStore.shared
            try store.persistAfterVerifiedAuthentication(for: profile,
                                                         username: "remember_fixture",
                                                         password: "fixture-before-opt-out",
                                                         enabled: true)
            try store.forget(for: profile, username: "remember_fixture")
            XCTAssertNil(try store.restore(for: profile))
        }
    }

    func testDemoAndUnverifiedIdentityNeverSavePassword() async throws {
        try await withFixture { session, profile in
            let store = IMRememberedLoginStore.shared
            session.demo = true
            XCTAssertThrowsError(try store.persistAfterVerifiedAuthentication(for: profile,
                                                                               username: "remember_fixture",
                                                                               password: "not-to-save",
                                                                               enabled: true))
            session.demo = false
            session.token = nil
            XCTAssertThrowsError(try store.persistAfterVerifiedAuthentication(for: profile,
                                                                               username: "remember_fixture",
                                                                               password: "not-to-save",
                                                                               enabled: true))
            XCTAssertNil(try store.restore(for: profile))
        }
    }

    func testCompletedErasureClearsRenamedAccountAndPreservesOtherAccount() async throws {
        try await withFixture { session, profile in
            let store = IMRememberedLoginStore.shared
            let deletedID = session.user["id"].text
            try store.persistAfterVerifiedAuthentication(for: profile,
                                                         username: "remember_fixture",
                                                         password: "fixture-before-rename",
                                                         enabled: true)
            session.user = session.user.setting("username", .string("remember_renamed"))
            session.credentialPair = try session.credentialPair?.replacingUsername("remember_renamed")
            try store.persistAfterVerifiedAuthentication(for: profile,
                                                         username: "remember_renamed",
                                                         password: "fixture-after-rename",
                                                         enabled: true)
            let deletedUser = session.user
            let deletedPair = session.credentialPair
            try installVerifiedUser(session, profile: profile, userID: UUID().uuidString,
                                    username: "remember_other")
            try store.persistAfterVerifiedAuthentication(for: profile,
                                                         username: "remember_other",
                                                         password: "other-account-must-survive",
                                                         enabled: true)
            session.user = deletedUser
            session.credentialPair = deletedPair
            let service = IMAccountDeletionService(session: session, username: "remember_renamed") {
                path, _, _ in
                XCTAssertEqual(path, "bootstrap")
                return .object([
                    "deploymentId": .string(session.deployment), "restoreEpoch": .string("1"),
                    "features": .object(["own_account_deletion": .bool(true)]),
                ])
            }
            let original = try await service.prepare()
            let completed = original.setting("state", .string("completed"))
            try session.store.save([completed], collection: "account-deletion-operations",
                                   namespace: IMAccountDeletionService.metadata(session))
            try await service.resumeLocalCleanup()
            XCTAssertNil(try store.restore(for: profile, username: "remember_fixture"))
            XCTAssertNil(try store.restore(for: profile, username: "remember_renamed"))
            XCTAssertEqual(try store.restore(for: profile, username: "remember_other")?.password,
                           "other-account-must-survive")
            XCTAssertNil(session.token)
            let receipt = try service.rows().first {
                $0["userId"].text == deletedID
            }
            XCTAssertEqual(receipt?["cleanup"], .bool(true))
        }
    }

    func testLegacyPasswordRecordDecodesAndErasureOnlyRemovesVerifiedUsername() async throws {
        try await withFixture { session, profile in
            let legacy = """
            {"selected":"remember_fixture","candidates":{
              "remember_fixture":{"username":"remember_fixture","password":"legacy-target"},
              "remember_other":{"username":"remember_other","password":"legacy-other"}}}
            """
            let digest = SHA256.hash(data: Data(profile.utf8)).map {
                String(format: "%02x", $0)
            }.joined()
            try IMKeychain.write(legacy, key: "remembered-login-v1:" + digest)
            let store = IMRememberedLoginStore.shared
            XCTAssertEqual(try store.restore(for: profile)?.password, "legacy-target")
            XCTAssertNil(try store.restore(for: profile)?.userID)
            try store.forget(for: profile, userID: session.user["id"].text,
                             legacyUsername: "remember_fixture")
            XCTAssertNil(try store.restore(for: profile, username: "remember_fixture"))
            XCTAssertEqual(try store.restore(for: profile)?.username, "remember_other")
            XCTAssertEqual(try store.restore(for: profile)?.password, "legacy-other")
        }
    }

    private func withFixture(_ body: (IMSession, String) async throws -> Void) async throws {
        guard ProcessInfo.processInfo.environment["JOBS_IM_SYNC_INJECTED_TESTS"] == "1" else {
            throw XCTSkip("Requires the isolated in-memory test host")
        }
        let session = IMSession.shared
        let previous = (session.profile, session.deployment, session.restoreEpoch,
                        session.user, session.demo, session.token, session.credentialPair)
        let profile = "remember-test-" + UUID().uuidString
        defer {
            try? IMRememberedLoginStore.shared.forget(for: profile)
            session.profile = previous.0
            session.deployment = previous.1
            session.restoreEpoch = previous.2
            session.user = previous.3
            session.demo = previous.4
            session.token = previous.5
            session.credentialPair = previous.6
        }
        session.profile = profile
        session.deployment = UUID().uuidString
        session.restoreEpoch = "1"
        session.demo = false
        try installVerifiedUser(session, profile: profile, userID: UUID().uuidString,
                                username: "remember_fixture")
        try await body(session, profile)
    }

    private func installVerifiedUser(_ session: IMSession,
                                     profile: String,
                                     userID: String,
                                     username: String) throws {
        let user = IMJSON.object(["id": .string(userID), "username": .string(username)])
        let response = IMJSON.object([
            "user": user, "token": .string("remember-fixture-access"),
            "refreshToken": .string("remember-fixture-refresh"),
            "familyId": .string(UUID().uuidString), "deviceId": .string(UUID().uuidString),
            "restoreEpoch": .string("1"), "sourceContextId": .string("personal"),
            "refreshGeneration": .string("1"), "accessExpiresAt": .string("2027-01-01T00:00:00Z"),
            "idleExpiresAt": .string("2027-02-01T00:00:00Z"),
            "absoluteExpiresAt": .string("2027-03-01T00:00:00Z"),
        ])
        session.user = user
        session.credentialPair = try IMCredentialPair(response: response,
                                                      profile: profile,
                                                      deploymentID: session.deployment,
                                                      requestID: UUID().uuidString)
        session.token = "remember-fixture-access"
    }
}
