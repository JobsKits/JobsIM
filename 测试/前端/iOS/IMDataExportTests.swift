//
//  IMDataExportTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport
import XCTest

@testable import JobsIM

@MainActor
final class IMDataExportTests: XCTestCase {

    private let jobID = "11111111-1111-4111-8111-111111111111"
    private let family = "22222222-2222-4222-8222-222222222222"
    private let scope = "33333333-3333-4333-8333-333333333333"

    private func session(path: String = ":memory:") throws -> IMSession {
        let response: IMJSON = .object(["token": .string("injected-access"), "refreshToken": .string("injected-refresh"),
            "familyId": .string(family), "refreshGeneration": .string("1"),
            "deviceId": .string("44444444-4444-4444-8444-444444444444"),
            "restoreEpoch": .string("2"), "sourceContextId": .string("personal"),
            "user": .object(["id": .string("55555555-5555-4555-8555-555555555555"), "username": .string("injected_export")]),
            "accessExpiresAt": .string("2026-11-16T20:15:00Z"), "idleExpiresAt": .string("2026-12-16T20:00:00Z"),
            "absoluteExpiresAt": .string("2027-02-14T20:00:00Z")])
        let pair = try IMCredentialPair(response: response, profile: "injected-export-" + UUID().uuidString,
                                        deploymentID: scope, requestID: "injected-auth-intent")
        let session = IMSession(store: try IMLocalStore(path: path))
        session.credentialPair = pair
        session.profile = pair.profile
        session.deployment = pair.deploymentID
        session.restoreEpoch = pair.restoreEpoch
        session.user = response["user"]
        session.token = pair.accessToken
        return session
    }

    private func bootstrap(_ enabled: Bool = true) -> IMJSON {
        return .object(["deploymentId": .string(scope), "restoreEpoch": .string("2"),
                         "features": .object(["chat_export": .bool(enabled)])])
    }

    private func view() -> IMJSON {
        return .object(["format": .string("zip"), "scope": .string("personal_current"),
            "coverage": .array([.string("messages")]), "exclusions": .array([.string("secrets")]),
            "job": .object(["id": .string(jobID), "artifactId": .string("66666666-6666-4666-8666-666666666666"),
                "restoreEpoch": .string("2"), "buildGeneration": .string("9007199254740993"),
                "state": .string("ready"), "expiresAt": .string("2026-11-17T20:00:00Z"), "cleanupState": .string("published")])])
    }

    private func row(_ session: IMSession, bound: Bool = false) -> IMJSON {
        let raw = "{\"restoreEpoch\":\"2\", \"requestId\":\"opaque-32hex-client-nonce\"}"
        var value: IMJSON = .object(["id": .string("opaque-32hex-client-nonce"), "familyId": .string(family),
            "originalBody": .string(raw), "bodySHA256": .string(IMDataExportProtocol.digest(raw)),
            "createdAt": .string("2026-11-16T20:00:00Z")])
        if bound {
            value = value.setting("jobId", .string(jobID))
        }
        return value
    }

    func testViewStrictCountersAndMetadataDoesNotRetainUnknownSecretFields() throws {
        let source = view().setting("decryptionKey", .string("forbidden-response-field"))
        let parsed = try IMDataExportProtocol.view(source, expectedJob: jobID)
        XCTAssertEqual(parsed["decryptionKey"], .null)
        XCTAssertEqual(parsed["job"]["buildGeneration"], .string("9007199254740993"))
        for job in [view()["job"].setting("restoreEpoch", .number(2)), view()["job"].setting("buildGeneration", .string("01")),
                    view()["job"].setting("state", .string("success")), view()["job"].setting("id", .string(UUID().uuidString))] {
            XCTAssertThrowsError(try IMDataExportProtocol.view(view().setting("job", job), expectedJob: jobID))
        }
    }

    func testOriginalNonceIsOpaqueAndRawWhitespaceHashCannotBeRewritten() throws {
        let session = try session()
        let original = row(session)
        try IMDataExportProtocol.original(original, namespace: session.namespace, family: family)
        let changed = original.setting("originalBody", .string(original["originalBody"].text.replacingOccurrences(of: " ", with: "")))
        XCTAssertThrowsError(try IMDataExportProtocol.original(changed, namespace: session.namespace, family: family))
        XCTAssertThrowsError(try IMDataExportProtocol.original(original.setting("ticket", .string("not-allowed")), namespace: session.namespace, family: family))
    }

    func testCASPreservesFirstCancelBodyAndOriginalAgainstLateWriter() throws {
        let session = try session()
        let original = row(session, bound: true)
        try session.store.compareDataExportIntent(namespace: session.namespace, previous: nil, next: original)
        let raw = "{\"requestId\":\"original-cancel-nonce\"}"
        let cancelled = original.setting("cancelBody", .string(raw)).setting("cancelSHA256", .string(IMDataExportProtocol.digest(raw)))
        try session.store.compareDataExportIntent(namespace: session.namespace, previous: original, next: cancelled)
        XCTAssertThrowsError(try session.store.compareDataExportIntent(namespace: session.namespace, previous: original, next: original))
        XCTAssertThrowsError(try session.store.compareDataExportIntent(namespace: session.namespace, previous: cancelled,
            next: cancelled.setting("cancelBody", .string("{\"requestId\":\"new-cancel-nonce\"}"))))
        XCTAssertEqual(try session.store.list("data-export-operations", namespace: session.namespace), [cancelled])
    }

    func testUnknownCreationOnlyChecksOriginalUntilExplicitSameBodyRetry() async throws {
        let session = try session()
        var writes: [String] = []
        var succeed = false
        let exports = IMDataExportService(session: session, request: { [self] path, _, raw in
            if path == "bootstrap" {
                return bootstrap()
            }
            if path == "me/data-export-results" {
                throw IMAPIError(status: 404, code: "EXPORT_OPERATION_NOT_FOUND", message: "injected absence")
            }
            XCTAssertEqual(path, "me/data-exports")
            writes.append(try XCTUnwrap(raw))
            if !succeed {
                throw IMAPIError(status: 0, code: "TRANSPORT_UNKNOWN", message: "injected response loss")
            }
            return view()
        })
        do {
            _ = try await exports.create()
            XCTFail("Creation must remain unknown")
        } catch {}
        let original = try XCTUnwrap(exports.rows().first)
        let checked = try await exports.check(original["id"].text)
        XCTAssertNil(checked)
        XCTAssertEqual(writes.count, 1)
        succeed = true
        _ = try await exports.check(original["id"].text, retry: true)
        XCTAssertEqual(writes, [original["originalBody"].text, original["originalBody"].text])
        XCTAssertEqual(try exports.rows().first?["jobId"], .string(jobID))
    }

    func testCapabilityWithdrawalStillAllowsOriginalLookupAndSameCancelBody() async throws {
        let session = try session()
        let original = row(session, bound: true)
        try session.store.compareDataExportIntent(namespace: session.namespace, previous: nil, next: original)
        var cancellations: [String] = []
        let exports = IMDataExportService(session: session, request: { [self] path, _, raw in
            if path == "bootstrap" {
                return bootstrap(false)
            }
            if path.hasSuffix("/cancel") {
                cancellations.append(try XCTUnwrap(raw))
            } else {
                XCTAssertEqual(path, "me/data-export-results")
            }
            return view()
        })
        _ = try await exports.check(original["id"].text)
        _ = try await exports.cancel(original["id"].text)
        _ = try await exports.cancel(original["id"].text)
        XCTAssertEqual(cancellations.count, 2)
        XCTAssertEqual(cancellations[0], cancellations[1])
        let body = try JSONDecoder.make().decode(IMJSON.self, from: Data(cancellations[0].utf8))
        XCTAssertEqual(Set(body.fields.keys), ["requestId"])
        do {
            _ = try await exports.create()
            XCTFail("Withdrawn capability must refuse new creation")
        } catch {}
        XCTAssertEqual(try exports.rows().count, 1)
    }

    func testLateIdentityResponseDoesNotBindOriginalJob() async throws {
        let session = try session()
        let original = row(session)
        let ns = session.namespace
        try session.store.compareDataExportIntent(namespace: ns, previous: nil, next: original)
        let exports = IMDataExportService(session: session, request: { [self] path, _, _ in
            if path == "bootstrap" {
                return bootstrap()
            }
            session.credentialPair = nil
            return view()
        })
        do {
            _ = try await exports.check(original["id"].text)
            XCTFail("Late response must be fenced")
        } catch {}
        XCTAssertEqual(try session.store.list("data-export-operations", namespace: ns), [original])
    }

    func testTicketDeadlineAndTypeAreStrictWithoutPersistingTheToken() throws {
        let now = Date(timeIntervalSince1970: 1_794_780_000)
        let expiry = ISO8601DateFormatter.make().string(from: now.addingTimeInterval(60))
        let job = view()["job"].setting("expiresAt", .string(expiry))
        let ticket: IMJSON = .object(["format": .string("zip"), "expiresAt": .string(expiry),
                                     "downloadToken": .string("injected-ticket-32characters-long")])
        XCTAssertFalse(try IMDataExportProtocol.ticket(ticket, job: job, now: now).isEmpty)
        XCTAssertThrowsError(try IMDataExportProtocol.ticket(ticket.setting("downloadToken", .number(1234567890123456)), job: job, now: now))
        XCTAssertThrowsError(try IMDataExportProtocol.ticket(ticket, job: job, now: now.addingTimeInterval(61)))
        XCTAssertThrowsError(try IMDataExportProtocol.ticket(ticket.setting("expiresAt", .string("2027-01-01T00:00:00Z")), job: job, now: now))
    }

    private func zip(zip64: Bool = false, names: [String] = ["manifest.json", "account-export.json"]) -> Data {
        func integer(_ value: UInt64, width: Int) -> Data {
            return Data((0..<width).map { UInt8(truncatingIfNeeded: value >> (8 * $0)) })
        }
        var bytes = Data()
        var directory = Data()
        for name in names {
            let offset = UInt64(bytes.count)
            let title = Data(name.utf8)
            bytes += integer(0x04034b50, width: 4) + integer(20, width: 2) + Data(repeating: 0, count: 20)
            bytes += integer(UInt64(title.count), width: 2) + Data(repeating: 0, count: 2) + title
            directory += integer(0x02014b50, width: 4) + integer(20, width: 2) + integer(20, width: 2)
            directory += Data(repeating: 0, count: 20) + integer(UInt64(title.count), width: 2)
            directory += Data(repeating: 0, count: 12) + integer(offset, width: 4) + title
        }
        let offset = UInt64(bytes.count)
        bytes += directory
        if zip64 {
            let record = UInt64(bytes.count)
            bytes += integer(0x06064b50, width: 4) + integer(44, width: 8) + integer(45, width: 2) + integer(45, width: 2)
            bytes += Data(repeating: 0, count: 8) + integer(UInt64(names.count), width: 8) + integer(UInt64(names.count), width: 8)
            bytes += integer(UInt64(directory.count), width: 8) + integer(offset, width: 8)
            bytes += integer(0x07064b50, width: 4) + Data(repeating: 0, count: 4) + integer(record, width: 8) + integer(1, width: 4)
        }
        bytes += integer(0x06054b50, width: 4) + Data(repeating: 0, count: 4)
        bytes += integer(zip64 ? 65535 : UInt64(names.count), width: 2) + integer(zip64 ? 65535 : UInt64(names.count), width: 2)
        bytes += integer(UInt64(directory.count), width: 4) + integer(offset, width: 4) + Data(repeating: 0, count: 2)
        return bytes
    }

    func testCompleteZIPAndZIP64DirectoryAreRequiredBeforeSystemSave() throws {
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".zip")
        defer {
            try? FileManager.default.removeItem(at: file)
        }
        for bytes in [zip(), zip(zip64: true)] {
            try bytes.write(to: file)
            try IMDataExportArchive.validate(file)
            for broken in [Data(bytes.dropLast()), bytes + Data("trailing".utf8)] {
                try broken.write(to: file)
                XCTAssertThrowsError(try IMDataExportArchive.validate(file))
            }
        }
    }

    func testUnsafeOrMissingExportDirectoryEntriesAreRejected() throws {
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".zip")
        defer {
            try? FileManager.default.removeItem(at: file)
        }
        for names in [["manifest.json"], ["manifest.json", "../account-export.json"], ["manifest.json", "manifest.json", "account-export.json"]] {
            try zip(names: names).write(to: file)
            XCTAssertThrowsError(try IMDataExportArchive.validate(file))
        }
    }

    func testReopenKeepsOnlyOriginalPublicMetadataAndDoesNotCreateSyncResponsibility() throws {
        let folder = FileManager.default.temporaryDirectory.appendingPathComponent("export-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        defer {
            try? FileManager.default.removeItem(at: folder)
        }
        let file = folder.appendingPathComponent("store.sqlite").path
        let session = try session(path: file)
        let original = row(session)
        try session.store.compareDataExportIntent(namespace: session.namespace, previous: nil, next: original)
        let reopened = try IMLocalStore(path: file)
        XCTAssertEqual(try reopened.list("data-export-operations", namespace: session.namespace), [original])
        XCTAssertNil(try reopened.syncStateIfPresent(namespace: session.namespace))
        let raw = try XCTUnwrap(reopened.list("data-export-operations", namespace: session.namespace).first)["originalBody"].text
        XCTAssertEqual(Data(raw.utf8), Data(original["originalBody"].text.utf8))
        XCTAssertFalse(raw.contains("injected-access"))
        XCTAssertFalse(raw.contains("injected-refresh"))
    }
}
