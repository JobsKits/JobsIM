//
//  IMReceiptServiceTests.swift
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
final class IMReceiptServiceTests: XCTestCase {

    @MainActor
    private final class Fixture {
        let folder: URL
        let session: IMSession
        let user = UUID().uuidString.lowercased()
        let device = UUID().uuidString.lowercased()
        let family = UUID().uuidString.lowercased()
        let scope = UUID().uuidString.lowercased()
        let conversation = UUID().uuidString.lowercased()
        let messageID = UUID().uuidString.lowercased()
        let qualificationID = UUID().uuidString.lowercased()
        var state: IMJSON = .null
        var message: IMJSON = .null
        var storage: IMJSON = .null
        var service: IMReceiptService!
        var log: [String] = []
        var saved: [String: IMJSON] = [:]
        var dropSubmit = false
        var corruptManifest = false
        var corruptPermission = false
        var frozenTarget = false
        var targetRecorded = true
        var declarations: [String: IMJSON] = [:]
        var legacyDeclaration = false
        var beforeResult: (() throws -> Void)?

        init() throws {
            folder = FileManager.default.temporaryDirectory.appendingPathComponent(
                "JobsIM-ReceiptService-" + UUID().uuidString)
            try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: false)
            session = IMSession(store: try IMLocalStore(path: folder.appendingPathComponent("client.sqlite").path))
            let response = auth(family)
            let pair = try IMCredentialPair(
                response: response, profile: "receipt-service-" + UUID().uuidString,
                deploymentID: scope, requestID: "receipt-service-fixture-auth")
            session.profile = pair.profile
            session.deployment = scope
            session.restoreEpoch = "1"
            session.user = response["user"]
            session.credentialPair = pair
            session.token = pair.accessToken
            let initial = try session.store.syncState(namespace: session.namespace)
            state = try session.store.syncUpdate(
                namespace: session.namespace, expected: initial,
                next: initial.setting("restriction", .string("")).setting(
                    "streamId", .string(UUID().uuidString.lowercased())
                ).setting("permissionGeneration", .string(String(repeating: "a", count: 64))))
            try session.store.save(
                [
                    .object([
                        "id": .string(conversation), "kind": .string("direct"),
                        "state": .string("active"), "membershipGeneration": .string("7"),
                    ])
                ], collection: "conversations", namespace: session.namespace)
            message = .object([
                "id": .string(messageID), "conversationId": .string(conversation), "senderId": .string(user),
                "type": .string("text"), "text": .string("原文 <&> བོད་"), "createdAt": .string("2026-11-17T00:00:00Z"),
                "contentVersion": .string("9007199254740993"), "creationEpoch": .string("1"),
                "creationSequence": .string("3"),
                "version": .string("1"), "deleted": .bool(false),
            ])
            try session.store.save([message], collection: "messages:" + conversation, namespace: session.namespace)
            storage = .object([
                "qualificationId": .string(""), "storeGeneration": state["storeGeneration"],
                "restoreEpoch": .string("1"), "profileId": .string(""), "buildDigest": .string(""),
                "verificationDigest": .string(""), "storageContract": .string(""), "qualified": .bool(false),
                "objectQualified": .bool(false),
            ])
            service = try IMReceiptService(
                session: session,
                request: { [weak self] path, method, body, _, _ in
                    guard let self else {
                        throw IMAPIError(status: 0, code: "FIXTURE_GONE", message: "fixture")
                    }
                    return try self.request(path, method: method, body: body)
                })
        }

        deinit {
            try? FileManager.default.removeItem(at: folder)
        }

        func auth(_ family: String) -> IMJSON {
            return .object([
                "token": .string("fixture-access"), "refreshToken": .string("fixture-refresh"),
                "familyId": .string(family), "refreshGeneration": .string("1"), "deviceId": .string(device),
                "sourceContextId": .string("personal"), "restoreEpoch": .string("1"),
                "user": .object(["id": .string(user), "username": .string("receipt_fixture")]),
                "accessExpiresAt": .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(3600))),
                "idleExpiresAt": .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(7200))),
                "absoluteExpiresAt": .string(
                    ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(86400))),
            ])
        }

        func context() -> IMJSON {
            let qualified = storage["qualified"] == .bool(true)
            var readiness = Dictionary(
                uniqueKeysWithValues: IMReceiptProtocol.protection.map { ($0, IMJSON.bool(true)) })
            readiness["ready"] = .bool(true)
            readiness["blockers"] = .array([])
            return .object([
                "protocol": .string(IMReceiptProtocol.version), "scopeId": .string(scope), "principalId": .string(user),
                "sourceContextId": .string("personal"), "deviceId": .string(device),
                "storeGeneration": state["storeGeneration"],
                "restoreEpoch": .string("1"), "schemaVersion": .string("1"), "storage": storage,
                "qualification": .string(qualified ? "qualified_content" : "transport_only"),
                "qualifiedAck": .bool(qualified), "contentReceiptEnabled": .bool(qualified),
                "objectReceiptEnabled": .bool(false),
                "afterReceiptEnabled": .bool(qualified), "afterObjectReceiptEnabled": .bool(false),
                "readiness": .object(readiness), "defaultRetentionMode": .string("cloud_history"),
                "deletionEligible": .bool(false), "targetRangeComplete": .bool(false),
            ])
        }

        func request(_ path: String, method: String, body: [String: IMJSON]) throws -> IMJSON {
            log.append(method + " " + path)
            if path.hasPrefix("sync/receipt-context?") {
                return context()
            }
            if path.hasPrefix("sync/receipt-profiles?") {
                let report = try XCTUnwrap(service.verification)
                return .object([
                    "protocol": .string(IMReceiptProtocol.version), "qualifiedAck": .bool(false),
                    "declarationRequired": .bool(true), "localDurabilityVerificationRequired": .bool(true),
                    "profiles": .array([
                        .object([
                            "id": .string("fixture-native-build"), "buildDigest": report["buildDigest"],
                            "verificationDigest": report["verificationDigest"],
                            "storageContract": report["storageContract"],
                        ])
                    ]),
                ])
            }
            if path == "sync/storage-results" {
                if let result = declarations[body["requestId"]?.text ?? ""] {
                    return .object([
                        "result": result, "current": .bool(storage["qualified"] == .bool(true)),
                        "storage": storage, "deletionEligible": .bool(false), "afterReceiptEnabled": .bool(false),
                    ])
                }
                throw IMAPIError(status: 404, code: "STORAGE_DECLARATION_NOT_FOUND", message: "fixture")
            }
            if path == "sync/storage", method == "PUT" {
                let row = try XCTUnwrap(session.store.list("receipt-declarations", namespace: session.namespace).first)
                XCTAssertEqual(row["originalBody"], .object(body))
                XCTAssertEqual(row["attempts"], .number(1))
                XCTAssertEqual(row["state"], .string("unknown"))
                let result: IMJSON = .object([
                    "qualificationId": .string(qualificationID), "storeGeneration": state["storeGeneration"],
                    "restoreEpoch": .string("1"), "profileId": body["profileId"] ?? .null,
                    "buildDigest": body["buildDigest"] ?? .null,
                    "verificationDigest": body["verificationDigest"] ?? .null,
                    "storageContract": .string("native_sqlite_full_v1"),
                ])
                storage = result.setting("qualified", .bool(true)).setting("objectQualified", .bool(false))
                let original = legacyDeclaration ? result : result.setting("qualified", .bool(true)).setting("objectQualified", .bool(false))
                declarations[body["requestId"]?.text ?? ""] = original
                return .object([
                    "result": original, "current": .bool(true), "storage": storage,
                    "deletionEligible": .bool(false), "afterReceiptEnabled": .bool(false),
                ])
            }
            if path == "sync/receipt-challenges" {
                let raw = try JSONEncoder.make().encode(message)
                let digest = IMMigrationProtocol.hash(raw)
                let expires = Int64(Date().timeIntervalSince1970) + 3600
                var proof: IMJSON = .object([
                    "scope": .string(scope), "user": .string(user), "device": .string(device),
                    "store": state["storeGeneration"], "epoch": .string("1"), "qualification": .string(qualificationID),
                    "message": .string(messageID), "conversation": .string(conversation),
                    "contentVersion": message["contentVersion"],
                    "bodyDigest": .string(digest), "permission": .string(String(repeating: "a", count: 64)),
                    "expires": .number(Decimal(expires)), "deliveryGeneration": .string("3"),
                    "manifestDigest": .string(String(repeating: "b", count: 64)),
                ])
                if corruptManifest {
                    var fields = proof.fields
                    fields.removeValue(forKey: "manifestDigest")
                    proof = .object(fields)
                }
                if corruptPermission {
                    proof = proof.setting("permission", .string(String(repeating: "c", count: 64)))
                }
                let token =
                    "rc1." + IMMigrationProtocol.base64(try JSONEncoder.make().encode(proof))
                    + "." + IMMigrationProtocol.base64(Data(repeating: 0, count: 32))
                return .object([
                    "challenge": .string(token), "messageId": .string(messageID),
                    "conversationId": .string(conversation),
                    "contentVersion": message["contentVersion"], "restoreEpoch": .string("1"),
                    "storeGeneration": state["storeGeneration"],
                    "qualificationId": .string(qualificationID),
                    "bodyEnvelope": .string(IMMigrationProtocol.base64(raw)),
                    "bodyDigest": .string(digest),
                    "expiresAt": .string(
                        ISO8601DateFormatter.make().string(from: Date(timeIntervalSince1970: Double(expires)))),
                    "requiresAtomicLocalCommit": .bool(true), "objectComplete": .bool(false),
                    "deletionEligible": .bool(false),
                ])
            }
            if path.hasPrefix("sync/receipt-targets/") {
                return .object([
                    "protocol": .string(IMReceiptProtocol.version), "messageId": .string(messageID),
                    "contentVersion": message["contentVersion"], "restoreEpoch": .string("1"),
                    "deliveryGeneration": .string("3"),
                    "manifestDigest": .string(String(repeating: "b", count: 64)),
                    "receiptScope": .string("all_target_devices"),
                    "originalTarget": .bool(frozenTarget), "contentTarget": .bool(frozenTarget), "objectTarget": .bool(false),
                    "contentRecorded": .bool(targetRecorded), "objectRecorded": .bool(false), "contentCompleteAt": .null,
                    "objectCompleteAt": .null, "current": .bool(frozenTarget), "messageCurrent": .bool(true),
                    "qualificationId": .string(qualificationID), "deletionEligible": .bool(false),
                ])
            }
            if path == "sync/receipt-results" {
                try beforeResult?()
                if let result = saved[body["requestId"]?.text ?? ""] {
                    return result
                }
                throw IMAPIError(status: 404, code: "RECEIPT_OPERATION_NOT_FOUND", message: "fixture")
            }
            if path == "sync/receipts" {
                let row = try XCTUnwrap(session.store.list("receipt-intents", namespace: session.namespace).first)
                XCTAssertEqual(row["state"], .string("unknown"))
                XCTAssertEqual(row["originalBody"], .object(body))
                XCTAssertEqual(
                    try session.store.list("receipt-bodies", namespace: session.namespace).first?["bodyDigest"],
                    row["bodyDigest"])
                let result: IMJSON = .object([
                    "operationId": row["id"], "status": .string("recorded"),
                    "receiptKind": .string("message_content"), "messageId": row["messageId"],
                    "conversationId": row["conversationId"],
                    "contentVersion": row["contentVersion"], "restoreEpoch": row["restoreEpoch"],
                    "deviceId": .string(device),
                    "storeGeneration": row["storeGeneration"], "qualificationId": row["qualificationId"],
                    "bodyDigest": row["bodyDigest"],
                    "deliveryGeneration": row["deliveryGeneration"], "manifestDigest": row["manifestDigest"],
                    "storageQualifiedAtCommit": .bool(true), "objectComplete": .bool(false),
                    "targetRangeComplete": .bool(false), "deletionEligible": .bool(false),
                ])
                saved[row["id"].text] = result
                if dropSubmit {
                    throw IMAPIError(status: 503, code: "FIXTURE_RESPONSE_LOST", message: "fixture")
                }
                return result
            }
            throw IMAPIError(status: 0, code: "UNEXPECTED_FIXTURE_ROUTE", message: "fixture")
        }

        func qualify() async throws {
            _ = try await service.verify(includeObjects: false)
            _ = try await service.declare("fixture-native-build")
        }
    }

    func testApprovedServerQualificationDoesNotReplaceLocalProbe() async throws {
        let f = try Fixture()
        f.storage = f.storage.setting("qualified", .bool(true)).setting("qualificationId", .string(f.qualificationID))
            .setting("profileId", .string("fixture-native-build")).setting(
                "buildDigest", .string(String(repeating: "a", count: 64))
            )
            .setting("verificationDigest", .string(String(repeating: "b", count: 64))).setting(
                "storageContract", .string("native_sqlite_full_v1"))
        do {
            _ = try await f.service.receiveContent(f.messageID)
            XCTFail("A remotely approved profile does not prove the running build or local store")
        } catch {}
        XCTAssertFalse(f.log.contains("POST sync/receipt-challenges"))
        XCTAssertTrue(try f.session.store.list("receipt-intents", namespace: f.session.namespace).isEmpty)
    }

    func testRealLocalProbeExplicitDeclarationAndContentAtomicReceipt() async throws {
        for legacy in [true, false] {
            let f = try Fixture()
            f.legacyDeclaration = legacy
            try await f.qualify()
            let report = try XCTUnwrap(f.service.verification)
            XCTAssertNotNil(UInt64(report["guardGeneration"].text))
            XCTAssertNil(UUID(uuidString: report["guardGeneration"].text))
            let result = try await f.service.receiveContent(f.messageID)
            XCTAssertEqual(result?["status"], .string("recorded"))
            XCTAssertEqual(result?["deliveryGeneration"], .string("3"))
            XCTAssertEqual(result?["contentVersion"], .string("9007199254740993"))
            XCTAssertEqual(try f.session.store.list("receipt-intents", namespace: f.session.namespace).first?["state"], .string("completed"))
            XCTAssertEqual(try f.service.intents().first?["permissionGeneration"], f.state["permissionGeneration"])
            let original = try XCTUnwrap(f.service.declarations().first)
            XCTAssertEqual(original["result"].fields.count, 7)
            XCTAssertEqual(original["result"]["qualified"], .null)
            let first = try await f.service.checkDeclaration(original["id"].text)
            XCTAssertEqual(first?["result"].fields.count, legacy ? 7 : 9)
            if !legacy {
                let snapshot = try XCTUnwrap(first?["result"])
                XCTAssertEqual(snapshot["qualified"], .bool(true))
                var partial = snapshot.fields
                partial.removeValue(forKey: "objectQualified")
                XCTAssertThrowsError(try IMReceiptProtocol.declarationFact(.object(partial)))
                XCTAssertThrowsError(try IMReceiptProtocol.declarationFact(snapshot.setting("qualified", .bool(false))))
                XCTAssertThrowsError(try IMReceiptProtocol.declarationFact(snapshot.setting("objectQualified", .bool(true))))
                XCTAssertThrowsError(try IMReceiptProtocol.declarationFact(snapshot.setting("token", .string("FORBIDDEN_FIXTURE_FIELD"))))
            }
            f.storage = f.storage.setting("qualified", .bool(false))
            let historical = try await f.service.checkDeclaration(original["id"].text)
            XCTAssertEqual(historical?["current"], .bool(false))
            XCTAssertEqual(try f.service.declarations().first?["result"], original["result"])
            XCTAssertEqual(try f.session.store.syncStateIfPresent(namespace: f.session.namespace), f.state)
            XCTAssertTrue(try f.session.store.allOutbox(namespace: f.session.namespace).isEmpty)
        }
    }

    func testOrdinarySyncAdvanceDoesNotInvalidateSameStoreMeasuredReport() async throws {
        let f = try Fixture()
        let report = try await f.service.verify(includeObjects: false)
        f.state = try f.session.store.syncUpdate(namespace: f.session.namespace, expected: f.state,
            next: f.state.setting("position", .string("1")))
        XCTAssertNotEqual(report["guardGeneration"], f.state["guardGeneration"])
        _ = try await f.service.declare("fixture-native-build")
        let result = try await f.service.receiveContent(f.messageID)
        XCTAssertEqual(result?["status"], .string("recorded"))
        XCTAssertEqual(f.service.verification?["verificationDigest"], report["verificationDigest"])
        XCTAssertEqual(f.service.verification?["guardGeneration"], report["guardGeneration"])
    }

    func testLostReceiptResultOnlyQueriesOriginalNonceAndRetainsRawBytes() async throws {
        let f = try Fixture()
        try await f.qualify()
        f.dropSubmit = true
        do {
            _ = try await f.service.receiveContent(f.messageID)
        } catch {}
        let original = try XCTUnwrap(f.service.intents().first)
        XCTAssertEqual(original["state"], .string("unknown"))
        let writes = f.log.filter { $0 == "POST sync/receipts" }.count
        let result = try await f.service.checkReceipt(original["id"].text)
        XCTAssertEqual(result?["operationId"], original["id"])
        XCTAssertEqual(f.log.filter { $0 == "POST sync/receipts" }.count, writes)
        XCTAssertEqual(try f.service.intents().first?["originalBody"], original["originalBody"])
        XCTAssertFalse(
            try XCTUnwrap(f.session.store.list("receipt-bodies", namespace: f.session.namespace).first)["bodyEnvelope"]
                .text.isEmpty)
    }

    func testUnpairedDeliveryManifestRefusesBeforeLocalStageOrAck() async throws {
        for invalid in ["manifest", "permission"] {
            let f = try Fixture()
            try await f.qualify()
            f.corruptManifest = invalid == "manifest"
            f.corruptPermission = invalid == "permission"
            do {
                _ = try await f.service.receiveContent(f.messageID)
                XCTFail("An altered delivery manifest or current permission must not stage or ACK")
            } catch {}
            XCTAssertTrue(try f.service.intents().isEmpty)
            XCTAssertFalse(f.log.contains("POST sync/receipts"))
            XCTAssertTrue(try f.session.store.list("receipt-bodies", namespace: f.session.namespace).isEmpty)
        }
    }

    func testSuccessorFamilyCannotFirstPublishUnknownOriginal() async throws {
        let f = try Fixture()
        try await f.qualify()
        f.dropSubmit = true
        do {
            _ = try await f.service.receiveContent(f.messageID)
        } catch {}
        let original = try XCTUnwrap(f.service.intents().first)
        f.saved = [:]
        let pair = try IMCredentialPair(
            response: f.auth(UUID().uuidString.lowercased()), profile: f.session.profile,
            deploymentID: f.scope, requestID: "receipt-fixture-successor-auth")
        f.session.credentialPair = pair
        f.session.token = pair.accessToken
        let next = try IMReceiptService(
            session: f.session,
            request: { path, method, body, _, _ in
                return try f.request(path, method: method, body: body)
            })
        let writes = f.log.filter { $0 == "POST sync/receipts" }.count
        do {
            _ = try await next.checkReceipt(original["id"].text, retry: true)
            XCTFail("A new family may only query old receipt responsibility")
        } catch {}
        XCTAssertEqual(f.log.last, "POST sync/receipt-results")
        XCTAssertEqual(f.log.filter { $0 == "POST sync/receipts" }.count, writes)
    }

    func testSecretDurableBodyIsRejectedBeforeQueryHttp() async throws {
        let f = try Fixture()
        try await f.qualify()
        _ = try await f.service.receiveContent(f.messageID)
        let original = try XCTUnwrap(f.service.intents().first)
        let toxic = original.setting(
            "originalBody", original["originalBody"].setting("password", .string("FORBIDDEN_FIXTURE_SECRET")))
        try f.session.store.save([toxic], collection: "receipt-intents", namespace: f.session.namespace)
        let before = f.log.count
        do {
            _ = try await f.service.checkReceipt(original["id"].text)
            XCTFail("Unknown persisted secret fields must not reach HTTP")
        } catch {}
        XCTAssertEqual(f.log.count, before)
    }

    private func pairing(_ fixture: Fixture) throws -> String {
        let payload: IMJSON = .object([
            "pairingId": .string(UUID().uuidString.lowercased()), "scopeId": .string(fixture.scope),
            "userId": .string(fixture.user), "targetDeviceId": .string(UUID().uuidString.lowercased()),
            "targetStoreGeneration": .string(UUID().uuidString.lowercased()), "restoreEpoch": .string("1"),
            "securityVersion": .string("1"), "sourceGeneration": .string("1"),
            "expires": .number(Decimal(Int64(Date().timeIntervalSince1970) + 300)),
        ])
        return "mp1." + IMMigrationProtocol.base64(try JSONEncoder.make().encode(payload))
            + "." + IMMigrationProtocol.base64(Data(repeating: 0, count: 32))
    }

    private func migrationCapability() -> IMJSON {
        return .object([
            "enabled": .bool(true), "format": .string(IMMigrationProtocol.format),
            "sourceContextId": .string("personal"),
            "maxMessages": .number(256), "maxObjects": .number(64),
            "maxRequestBytes": .number(Decimal(IMMigrationProtocol.limit)),
            "maxObjectBytes": .number(64 * 1024 * 1024), "maxLifetimeSeconds": .number(600),
            "pairingLifetimeSeconds": .number(300), "installLifetimeSeconds": .number(5),
            "qualifiedAck": .bool(false), "targetQualified": .bool(false),
        ])
    }

    func testCloudOnlyMigrationUsesExactQualifiedRawBytesWithoutExpandingOldTarget() async throws {
        let f = try Fixture()
        try await f.qualify()
        _ = try await f.service.receiveContent(f.messageID)
        let content = try XCTUnwrap(f.service.intents().first)
        let original = try XCTUnwrap(f.session.store.list("receipt-bodies", namespace: f.session.namespace).first)
        f.storage = f.storage.setting("qualified", .bool(false))
        let cloudOnly = f.message.setting("text", .string("")).setting("contentUnavailable", .bool(true))
            .setting("deleteScope", .string("cloud_only"))
        try f.session.store.save([cloudOnly], collection: "messages:" + f.conversation, namespace: f.session.namespace)
        var creates: [IMJSON] = []
        let source = try IMMigrationService(
            session: f.session,
            request: { [self] path, method, body, _, _ in
                if path == "sync/migration-capabilities" {
                    return migrationCapability()
                }
                if path == "sync/migration-results" {
                    return .object(["status": .string("not_found")])
                }
                if path == "messages/" + f.messageID {
                    return cloudOnly
                }
                if path == "sync/migrations" {
                    creates.append(.object(body))
                    throw IMAPIError(status: 503, code: "FIXTURE_MIGRATION_UNKNOWN", message: "fixture")
                }
                return try f.request(path, method: method, body: body)
            })
        let row = try source.prepareSource(ids: [f.messageID], originals: false)
        let proof = try pairing(f)
        do {
            _ = try await source.create(row["id"].text, pairingProof: proof, password: "fixture password", otp: "")
        } catch {}
        XCTAssertEqual(creates.count, 1)
        let message = try XCTUnwrap(creates.first?["messages"].list.first)
        XCTAssertEqual(message["bodyEnvelope"], original["bodyEnvelope"])
        XCTAssertEqual(message["sourceReceiptId"], content["id"])
        XCTAssertEqual(message["objectMode"], .string("none"))
        XCTAssertTrue(creates[0]["objects"].list.isEmpty)
        try f.session.store.save(
            [original.setting("bodyEnvelope", .string(IMMigrationProtocol.base64(Data("tampered".utf8))))],
            collection: "receipt-bodies", namespace: f.session.namespace)
        do {
            _ = try await source.create(row["id"].text, pairingProof: proof, password: "fixture password", otp: "")
        } catch {}
        XCTAssertEqual(creates.count, 1)
    }

    func testCloudOnlyFullMigrationCannotBorrowContentReceiptAsOriginalObjectProof() async throws {
        let f = try Fixture()
        f.message = f.message.setting("type", .string("file")).setting(
            "attachmentId", .string(UUID().uuidString.lowercased()))
        try f.session.store.save([f.message], collection: "messages:" + f.conversation, namespace: f.session.namespace)
        try await f.qualify()
        _ = try await f.service.receiveContent(f.messageID)
        let cloudOnly = f.message.setting("text", .string("")).setting("contentUnavailable", .bool(true))
            .setting("deleteScope", .string("cloud_only"))
        try f.session.store.save([cloudOnly], collection: "messages:" + f.conversation, namespace: f.session.namespace)
        var creates = 0
        let source = try IMMigrationService(
            session: f.session,
            request: { [self] path, method, body, _, _ in
                if path == "sync/migration-capabilities" {
                    return migrationCapability()
                }
                if path == "sync/migration-results" {
                    return .object(["status": .string("not_found")])
                }
                if path == "messages/" + f.messageID {
                    return cloudOnly
                }
                if path == "sync/migrations" {
                    creates += 1
                    return .null
                }
                return try f.request(path, method: method, body: body)
            })
        let row = try source.prepareSource(ids: [f.messageID], originals: true)
        do {
            _ = try await source.create(row["id"].text, pairingProof: pairing(f), password: "fixture password", otp: "")
            XCTFail("A content receipt must not stand in for original bytes or an object receipt")
        } catch {}
        XCTAssertEqual(creates, 0)
    }

    private func coordinator(_ fixture: Fixture) -> IMReceiptCoordinator {
        return IMReceiptCoordinator(session: fixture.session, observeLifecycle: false, foreground: { true },
            factory: { session, current in
                return try IMReceiptService(session: session, isCurrent: current,
                    request: { path, method, body, _, _ in
                        return try fixture.request(path, method: method, body: body)
                    })
            })
    }

    func testAutomaticConsentRecoversExistingUnacknowledgedFrozenTargetWithoutNewDeclaration() async throws {
        let f = try Fixture()
        try await f.qualify()
        f.frozenTarget = true
        f.targetRecorded = false
        let coordinator = coordinator(f)
        defer {
            coordinator.disable()
        }
        let declarations = f.log.filter { $0 == "PUT sync/storage" }.count
        XCTAssertFalse(coordinator.isEnabled(for: f.session))
        try await coordinator.enable()
        XCTAssertEqual(coordinator.state, .running)
        XCTAssertEqual(f.log.filter { $0 == "POST sync/receipts" }.count, 1)
        XCTAssertEqual(f.log.filter { $0 == "PUT sync/storage" }.count, declarations)
        let original = try XCTUnwrap(f.service.intents().first)
        XCTAssertEqual(original["state"], .string("completed"))
        XCTAssertEqual(original["deliveryGeneration"], .string("3"))
    }

    func testAutomaticUnknownOnlyQueriesAndSuccessorDoesNotInheritConsent() async throws {
        let f = try Fixture()
        try await f.qualify()
        f.targetRecorded = false
        let coordinator = coordinator(f)
        defer {
            coordinator.disable()
        }
        try await coordinator.enable()
        XCTAssertFalse(f.log.contains("POST sync/receipts"))
        f.message = f.message.setting("contentVersion", .string("9007199254740994"))
        try f.session.store.save([f.message], collection: "messages:" + f.conversation, namespace: f.session.namespace)
        f.dropSubmit = true
        await coordinator.refresh()
        let original = try XCTUnwrap(f.service.intents().first)
        XCTAssertEqual(original["state"], .string("unknown"))
        XCTAssertEqual(f.log.filter { $0 == "POST sync/receipts" }.count, 1)
        f.saved = [:]
        await coordinator.refresh()
        XCTAssertEqual(f.log.filter { $0 == "POST sync/receipts" }.count, 1)
        XCTAssertEqual(try f.service.intents().first?["originalBody"], original["originalBody"])
        let pair = try IMCredentialPair(response: f.auth(UUID().uuidString.lowercased()), profile: f.session.profile,
            deploymentID: f.scope, requestID: "automatic-receipt-successor-auth")
        f.session.credentialPair = pair
        f.session.token = pair.accessToken
        let requests = f.log.count
        await coordinator.refresh()
        XCTAssertFalse(coordinator.isEnabled(for: f.session))
        XCTAssertEqual(coordinator.state, .off)
        XCTAssertEqual(f.log.count, requests)
    }

    func testAutomaticPauseBlocksNewWorkAndSameFamilyResumeReprobesWithoutDeclaring() async throws {
        let f = try Fixture()
        try await f.qualify()
        let coordinator = coordinator(f)
        defer {
            coordinator.disable()
        }
        try await coordinator.enable()
        let declarations = f.log.filter { $0 == "PUT sync/storage" }.count
        let probes = f.log.filter { $0.hasPrefix("GET sync/receipt-profiles?") }.count
        coordinator.pause()
        f.message = f.message.setting("contentVersion", .string("9007199254740994"))
        try f.session.store.save([f.message], collection: "messages:" + f.conversation, namespace: f.session.namespace)
        let requests = f.log.count
        await coordinator.refresh()
        XCTAssertEqual(coordinator.state, .paused)
        XCTAssertEqual(f.log.count, requests)
        coordinator.resume()
        await coordinator.refresh()
        XCTAssertEqual(f.log.filter { $0 == "POST sync/receipts" }.count, 1)
        XCTAssertGreaterThan(f.log.filter { $0.hasPrefix("GET sync/receipt-profiles?") }.count, probes)
        XCTAssertEqual(f.log.filter { $0 == "PUT sync/storage" }.count, declarations)
    }

    private func codeBundle() throws -> (bundle: URL, executable: URL, debug: URL, storage: URL) {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("JobsIM-BuildMeasure-" + UUID().uuidString)
        let framework = root.appendingPathComponent("Frameworks/JobsIMStorage.framework")
        try FileManager.default.createDirectory(at: framework, withIntermediateDirectories: true)
        let executable = root.appendingPathComponent("JobsIM"), debug = root.appendingPathComponent("JobsIM.debug.dylib")
        let storage = framework.appendingPathComponent("JobsIMStorage")
        let header = Data([0xCF, 0xFA, 0xED, 0xFE])
        // These are inert magic-tagged bytes for the hashing contract, not runnable code.
        try (header + Data("launcher".utf8)).write(to: executable)
        try (header + Data("app implementation".utf8)).write(to: debug)
        try (header + Data("storage implementation".utf8)).write(to: storage)
        try PropertyListSerialization.data(fromPropertyList: ["CFBundleExecutable": "JobsIMStorage"],
            format: .binary, options: 0).write(to: framework.appendingPathComponent("Info.plist"))
        return (root, executable, debug, storage)
    }

    func testBuildMeasurementIncludesDebugAndStorageCodeChanges() throws {
        let f = try codeBundle()
        defer {
            try? FileManager.default.removeItem(at: f.bundle)
        }
        let first = try IMReceiptBuild.measure(bundle: f.bundle, executable: f.executable)
        XCTAssertEqual(first.components.count, 3)
        XCTAssertTrue(first.components.contains(where: { $0["path"].text == "JobsIM.debug.dylib" }))
        XCTAssertTrue(first.components.contains(where: { $0["path"].text == "Frameworks/JobsIMStorage.framework/JobsIMStorage" }))
        let original = try Data(contentsOf: f.debug)
        try (Data([0xCF, 0xFA, 0xED, 0xFE]) + Data("changed app code".utf8)).write(to: f.debug)
        let debug = try IMReceiptBuild.measure(bundle: f.bundle, executable: f.executable)
        XCTAssertNotEqual(first.digest, debug.digest)
        try original.write(to: f.debug)
        try (Data([0xCF, 0xFA, 0xED, 0xFE]) + Data("changed storage code".utf8)).write(to: f.storage)
        let storage = try IMReceiptBuild.measure(bundle: f.bundle, executable: f.executable)
        XCTAssertNotEqual(first.digest, storage.digest)
    }

    func testBuildMeasurementRejectsUnverifiedCodeAndExternalSymlink() throws {
        let f = try codeBundle()
        let outside = f.bundle.deletingLastPathComponent().appendingPathComponent(f.bundle.lastPathComponent + "-outside")
        defer {
            try? FileManager.default.removeItem(at: f.bundle)
            try? FileManager.default.removeItem(at: outside)
        }
        try Data("not native code".utf8).write(to: f.debug)
        XCTAssertThrowsError(try IMReceiptBuild.measure(bundle: f.bundle, executable: f.executable))
        try (Data([0xCF, 0xFA, 0xED, 0xFE]) + Data("external".utf8)).write(to: outside)
        try FileManager.default.removeItem(at: f.debug)
        try FileManager.default.createSymbolicLink(at: f.debug, withDestinationURL: outside)
        XCTAssertThrowsError(try IMReceiptBuild.measure(bundle: f.bundle, executable: f.executable))
    }
}
