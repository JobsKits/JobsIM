//
//  IMAuthCredentialTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMStorage
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
