//
//  IMMigrationServiceTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport
import XCTest

@testable import JobsIM

@MainActor
final class IMMigrationServiceTests: XCTestCase {

    private let scope = "11111111-1111-4111-8111-111111111111"
    private let user = "22222222-2222-4222-8222-222222222222"
    private let device = "33333333-3333-4333-8333-333333333333"
    private let family = "44444444-4444-4444-8444-444444444444"
    private let conversation = "55555555-5555-4555-8555-555555555555"
    private let messageID = "66666666-6666-4666-8666-666666666666"

    private func response(family: String? = nil) -> IMJSON {
        return .object([
            "token": .string("fixture-migration-access"), "refreshToken": .string("fixture-migration-refresh"),
            "familyId": .string(family ?? self.family), "refreshGeneration": .string("1"),
            "deviceId": .string(device), "restoreEpoch": .string("2"), "sourceContextId": .string("personal"),
            "user": .object(["id": .string(user), "username": .string("migration_fixture")]),
            "accessExpiresAt": .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(3600))),
            "idleExpiresAt": .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(7200))),
            "absoluteExpiresAt": .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(86400))),
        ])
    }

    private func fixture(registered: Bool = true) throws -> IMSession {
        let value = response()
        let pair = try IMCredentialPair(response: value, profile: "migration-fixture-" + UUID().uuidString,
            deploymentID: scope, requestID: "migration-fixture-auth")
        let session = IMSession(store: try IMLocalStore(path: ":memory:"))
        session.profile = pair.profile
        session.deployment = scope
        session.restoreEpoch = "2"
        session.user = value["user"]
        session.token = pair.accessToken
        session.credentialPair = pair
        if registered {
            let initial = try session.store.syncState(namespace: session.namespace)
            _ = try session.store.syncUpdate(namespace: session.namespace, expected: initial,
                next: initial.setting("restriction", .string("")).setting("streamId", .string(UUID().uuidString.lowercased())))
            try session.store.save([.object([
                "id": .string(conversation), "kind": .string("direct"), "state": .string("active"),
                "membershipGeneration": .string("1"), "readOnly": .bool(false),
            ])], collection: "conversations", namespace: session.namespace)
            try session.store.save([message()], collection: "messages:" + conversation, namespace: session.namespace)
        }
        return session
    }

    private func message() -> IMJSON {
        return .object([
            "id": .string(messageID), "conversationId": .string(conversation), "senderId": .string(user),
            "clientMsgId": .string(UUID().uuidString.lowercased()), "type": .string("text"),
            "text": .string("原文 <>& བོད་ཡིག"), "createdAt": .string("2026-11-17T00:00:00Z"),
            "updatedAt": .string("2026-11-17T00:00:00Z"), "contentVersion": .string("9007199254740993"),
            "version": .string("1"), "creationEpoch": .string("2"), "creationSequence": .string("1"),
            "deleted": .bool(false), "favorite": .bool(false), "reactions": .array([]),
        ])
    }

    private func capability() -> IMJSON {
        return .object([
            "enabled": .bool(true), "format": .string(IMMigrationProtocol.format), "sourceContextId": .string("personal"),
            "maxMessages": .number(256), "maxObjects": .number(64), "maxRequestBytes": .number(Decimal(IMMigrationProtocol.limit)),
            "maxObjectBytes": .number(64 * 1024 * 1024), "maxLifetimeSeconds": .number(600),
            "pairingLifetimeSeconds": .number(300), "installLifetimeSeconds": .number(5),
            "qualifiedAck": .bool(false), "targetQualified": .bool(false),
        ])
    }

    private func metadata(_ session: IMSession) -> IMNamespace {
        return IMNamespace(profile: session.profile, account: user, demo: false, restoreEpoch: "migration-protocol",
            deploymentID: scope, sourceContextID: "personal")
    }

    private func proof(device: String, physical: String, id: String = UUID().uuidString.lowercased()) throws -> IMJSON {
        let expires = Int64(Date().timeIntervalSince1970) + 300
        let payload: IMJSON = .object([
            "pairingId": .string(id), "scopeId": .string(scope), "userId": .string(user),
            "targetDeviceId": .string(device), "targetStoreGeneration": .string(physical), "restoreEpoch": .string("2"),
            "securityVersion": .string("1"), "sourceGeneration": .string("1"), "expires": .number(Decimal(expires)),
        ])
        let token = "mp1." + IMMigrationProtocol.base64(try JSONEncoder.make().encode(payload))
            + "." + IMMigrationProtocol.base64(Data(repeating: 0, count: 32))
        return .object([
            "pairingId": .string(id), "pairingProof": .string(token), "targetDeviceId": .string(device),
            "targetStoreGeneration": .string(physical), "restoreEpoch": .string("2"),
            "expiresAt": .string(ISO8601DateFormatter.make().string(from: Date(timeIntervalSince1970: Double(expires)))),
            "sourcePreserved": .bool(true),
        ])
    }

    func testMissingRegistrationDoesNotCreateStoreOrQualification() throws {
        let session = try fixture(registered: false)
        XCTAssertNil(try session.store.syncStateIfPresent(namespace: session.namespace))
        XCTAssertThrowsError(try IMMigrationService(session: session))
        XCTAssertNil(try session.store.syncStateIfPresent(namespace: session.namespace))
    }

    func testPairingPostsOnlyAfterDurableOriginalAndDoesNotPersistSecrets() async throws {
        let session = try fixture()
        let physical = try XCTUnwrap(session.store.syncStateIfPresent(namespace: session.namespace))["storeGeneration"].text
        let returned = try proof(device: device, physical: physical)
        var writes = 0
        let service = try IMMigrationService(session: session, request: { [self] path, _, body, _, store in
            if path == "sync/migration-pairing-results" {
                return .object(["status": .string("not_found")])
            }
            if path == "sync/migration-capabilities" {
                return capability()
            }
            XCTAssertEqual(path, "sync/migration-pairings")
            XCTAssertEqual(store, physical)
            let row = try XCTUnwrap(session.store.list("migration-intents", namespace: metadata(session)).first)
            XCTAssertEqual(row["id"], body["requestId"])
            XCTAssertEqual(row["attempts"], .number(1))
            XCTAssertEqual(row["state"], .string("unknown"))
            writes += 1
            return returned
        })
        _ = try await service.pair(password: " fixture RAM password ", otp: "123456")
        let encoded = String(decoding: try JSONEncoder.make().encode(service.rows()), as: UTF8.self)
        XCTAssertEqual(writes, 1)
        XCTAssertFalse(encoded.contains("fixture RAM password"))
        XCTAssertFalse(encoded.contains("123456"))
        XCTAssertFalse(encoded.contains(returned["pairingProof"].text))
        XCTAssertEqual(try service.rows().first?["target"]["deviceId"], .string(device))
    }

    func testUnknownLookupCreatesNothingAndExplicitRetryKeepsNonce() async throws {
        let session = try fixture()
        let physical = try XCTUnwrap(session.store.syncStateIfPresent(namespace: session.namespace))["storeGeneration"].text
        let returned = try proof(device: device, physical: physical)
        var writes: [IMJSON] = []
        let service = try IMMigrationService(session: session, request: { [self] path, _, body, _, _ in
            if path == "sync/migration-pairing-results" {
                return .object(["status": .string("not_found")])
            }
            if path == "sync/migration-capabilities" {
                return capability()
            }
            writes.append(.object(body))
            if writes.count == 1 {
                throw IMAPIError(status: 503, code: "FIXTURE_DELIVERY_LOST", message: "fixture")
            }
            return returned
        })
        do {
            _ = try await service.pair(password: "fixture password", otp: "")
            XCTFail("The first result must remain unknown")
        } catch {}
        let row = try XCTUnwrap(service.rows().first)
        let checked = try await service.check(row["id"].text)
        XCTAssertNil(checked)
        XCTAssertEqual(writes.count, 1)
        _ = try await service.pair(password: "fixture password", otp: "", originalID: row["id"].text)
        XCTAssertEqual(writes.count, 2)
        XCTAssertEqual(writes[0], writes[1])
    }

    func testSuccessorFamilyCanQueryButCannotPublishUnseenOriginal() async throws {
        let session = try fixture()
        var writes = 0
        let request: IMMigrationService.Request = { [self] path, _, _, _, _ in
            if path == "sync/migration-pairing-results" {
                return .object(["status": .string("not_found")])
            }
            if path == "sync/migration-capabilities" {
                return capability()
            }
            writes += 1
            throw IMAPIError(status: 503, code: "FIXTURE_DELIVERY_LOST", message: "fixture")
        }
        let original = try IMMigrationService(session: session, request: request)
        do {
            _ = try await original.pair(password: "fixture password", otp: "")
        } catch {}
        let row = try XCTUnwrap(original.rows().first)
        let next = try IMCredentialPair(response: response(family: UUID().uuidString.lowercased()),
            profile: session.profile, deploymentID: scope, requestID: "fixture-new-auth-request")
        session.credentialPair = next
        session.token = next.accessToken
        let successor = try IMMigrationService(session: session, request: request)
        let checked = try await successor.check(row["id"].text)
        XCTAssertNil(checked)
        do {
            _ = try await successor.pair(password: "fixture password", otp: "", originalID: row["id"].text)
            XCTFail("A new family must not first publish an old intent")
        } catch {}
        XCTAssertEqual(writes, 1)
        XCTAssertEqual(try successor.rows().first?["familyId"], .string(family))
    }

    func testSourceFreezesVersionAndDropsMutableDisplayAndAdmissionFields() async throws {
        let session = try fixture()
        let peer = try proof(device: "77777777-7777-4777-8777-777777777777", physical: UUID().uuidString.lowercased())
        var source = message().setting("forwardAdmission", .string("FORBIDDEN_ADMISSION"))
        var creates: [IMJSON] = []
        let service = try IMMigrationService(session: session, request: { [self] path, _, body, _, _ in
            if path == "sync/migration-results" {
                return .object(["status": .string("not_found")])
            }
            if path == "sync/migration-capabilities" {
                return capability()
            }
            if path == "messages/" + messageID {
                return source
            }
            creates.append(.object(body))
            throw IMAPIError(status: 503, code: "FIXTURE_DELIVERY_LOST", message: "fixture")
        })
        let row = try service.prepareSource(ids: [messageID], originals: false)
        do {
            _ = try await service.create(row["id"].text, pairingProof: peer["pairingProof"].text, password: "fixture password", otp: "")
        } catch {}
        XCTAssertEqual(creates.count, 1)
        let body = try JSONDecoder.make().decode(IMJSON.self, from: IMMigrationProtocol.decode(creates[0]["messages"].list[0]["bodyEnvelope"], maximum: 128 * 1024))
        XCTAssertEqual(body["contentVersion"], .string("9007199254740993"))
        for field in ["forwardAdmission", "replyAdmission", "updatedAt", "favorite", "reactions", "clientMsgId"] {
            XCTAssertEqual(body[field], .null)
        }
        source = source.setting("contentVersion", .string("9007199254740994"))
        do {
            _ = try await service.create(row["id"].text, pairingProof: peer["pairingProof"].text, password: "fixture password", otp: "")
        } catch {}
        XCTAssertEqual(creates.count, 1)
        let persisted = try XCTUnwrap(service.rows().first)
        XCTAssertEqual(persisted["sourceDigest"].text.count, 64)
        XCTAssertFalse(String(decoding: try JSONEncoder.make().encode(persisted), as: UTF8.self).contains(peer["pairingProof"].text))
    }

    func testIdentityChangeWhileLookupWaitsPreventsFirstCreatingPost() async throws {
        let session = try fixture()
        var current = true
        var paths: [String] = []
        let service = try IMMigrationService(session: session, isCurrent: { current }, request: { path, _, _, _, _ in
            paths.append(path)
            current = false
            return .object(["status": .string("not_found")])
        })
        do {
            _ = try await service.pair(password: "fixture password", otp: "")
            XCTFail("A late old-context lookup cannot authorize a new POST")
        } catch {}
        XCTAssertEqual(paths, ["sync/migration-pairing-results"])
    }

    func testToxicDurableMetadataRejectsBeforeAnyCreatingHttp() async throws {
        let session = try fixture()
        var paths: [String] = []
        let service = try IMMigrationService(session: session, request: { path, _, _, _, _ in
            paths.append(path)
            return .null
        })
        let row = try service.prepareSource(ids: [messageID], originals: false)
        try session.store.save([row.setting("password", .string("FORBIDDEN_STORE_SECRET"))],
            collection: "migration-intents", namespace: metadata(session))
        do {
            _ = try await service.create(row["id"].text, pairingProof: "", password: "fixture password", otp: "")
            XCTFail("Unknown persisted secret fields must fail closed")
        } catch {}
        XCTAssertTrue(paths.isEmpty)
    }

    func testCredentialNFCAndSignificantSpacesStayInRam() throws {
        let value = try IMMigrationService.credential(password: " e\u{301} fixture ", otp: "123456")
        XCTAssertEqual(value["password"], .string(" é fixture "))
        XCTAssertThrowsError(try IMMigrationService.credential(password: "", otp: ""))
        XCTAssertThrowsError(try IMMigrationService.credential(password: "fixture", otp: "12345x"))
        XCTAssertThrowsError(try IMMigrationService.credential(password: String(repeating: "界", count: 180), otp: ""))
    }

    private func applyGlobal(_ id: String, session: IMSession) throws {
        let state = try XCTUnwrap(session.store.syncStateIfPresent(namespace: session.namespace))
        let reference: IMJSON = .object([
            "owner": .string("message"), "kind": .string("message"), "id": .string(id),
            "conversationId": .string(conversation), "revision": .string("2"), "revisionEpoch": .string("2"),
        ])
        let item: IMJSON = .object([
            "ref": reference, "state": .string("global"), "currentRevision": .string("2"),
            "currentRevisionEpoch": .string("2"),
            "object": .object([
                "id": .string(id), "conversationId": .string(conversation),
                "deleted": .bool(true), "deleteScope": .string("global"),
            ]),
        ])
        try session.store.syncCommit(namespace: session.namespace, expected: state, next: state,
            projections: [item], events: [], proofs: [], replaceCloud: false)
    }

    private func installArchive(_ id: String, messages: [IMJSON], session: IMSession) throws {
        let state = try XCTUnwrap(session.store.syncStateIfPresent(namespace: session.namespace))
        let hash = String(repeating: "a", count: 64)
        let manifest: IMJSON = .object([
            "id": .string(id), "scopeId": .string(scope), "userId": .string(user),
            "targetDeviceId": .string(device), "restoreEpoch": .string("2"),
            "targetStoreGeneration": state["storeGeneration"], "objects": .array([]),
            "messages": .array(messages.map { message in
                return .object([
                    "messageId": message["id"], "conversationId": message["conversationId"],
                    "contentVersion": message["contentVersion"], "membershipGeneration": .string("1"),
                ])
            }),
        ])
        try session.store.commitMigrationArchive(namespace: session.namespace, expected: state,
            manifest: manifest, digest: hash,
            packet: .object(["manifestDigest": .string(hash), "messages": .array(messages), "objects": .array([])]),
            installBefore: Date().addingTimeInterval(5))
    }

    func testLateForwardReadCannotRefillGloballyClearedArchive() async throws {
        let session = try fixture()
        let archive = UUID().uuidString.lowercased()
        let forwarded = message().setting("forwarded", .bool(true)).setting("objectMode", .string("none"))
        let ownedID = UUID().uuidString.lowercased()
        let owned = message().setting("id", .string(ownedID)).setting("objectMode", .string("none"))
        try installArchive(archive, messages: [owned, forwarded], session: session)
        XCTAssertEqual(try XCTUnwrap(session.store.list("migration-messages:" + archive,
            namespace: session.namespace).first(where: { $0["id"] == forwarded["id"] }))["text"], forwarded["text"])
        var reads = 0
        let service = try IMMigrationService(session: session, request: { [self] path, method, _, _, _ in
            XCTAssertEqual(path, "messages/" + messageID)
            XCTAssertEqual(method, "GET")
            reads += 1
            try applyGlobal(messageID, session: session)
            try applyGlobal(ownedID, session: session)
            return forwarded
        })
        let result = try await service.messages(archive)
        XCTAssertEqual(reads, 1)
        XCTAssertEqual(result.count, 2)
        for row in result {
            XCTAssertTrue(row["text"].text.isEmpty)
            XCTAssertEqual(row["deleted"], .bool(true))
            XCTAssertEqual(row["contentUnavailable"], .bool(true))
        }
        for row in try session.store.list("migration-messages:" + archive, namespace: session.namespace) {
            XCTAssertTrue(row["text"].text.isEmpty)
        }
    }

    func testLateForwardReadCannotUndoCurrentUnavailableProjection() async throws {
        let session = try fixture()
        let archive = UUID().uuidString.lowercased()
        let forwarded = message().setting("forwarded", .bool(true)).setting("objectMode", .string("none"))
        try installArchive(archive, messages: [forwarded], session: session)
        var reads = 0
        let service = try IMMigrationService(session: session, request: { _, _, _, _, _ in
            reads += 1
            try session.store.redactMigrationForward(forwarded["id"].text, namespace: session.namespace)
            return forwarded
        })
        let result = try await service.messages(archive)
        XCTAssertEqual(reads, 1)
        XCTAssertEqual(result.count, 1)
        XCTAssertTrue(result[0]["text"].text.isEmpty)
        XCTAssertEqual(result[0]["contentUnavailable"], .bool(true))
        XCTAssertTrue(try XCTUnwrap(session.store.list("migration-messages:" + archive,
            namespace: session.namespace).first)["text"].text.isEmpty)
    }
}
