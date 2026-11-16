//
//  IMPersonalStateTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import Foundation
import JobsIMCore
import JobsIMStorage
import XCTest

@testable import JobsIM

@MainActor
final class IMPersonalStateTests: XCTestCase {

    private let conversation = "11111111-1111-4111-8111-111111111111"
    private let message = "22222222-2222-4222-8222-222222222222"
    private var namespace: IMNamespace {
        return IMNamespace(profile: "owned-self-" + conversation, account: "55555555-5555-4555-8555-555555555555",
                           demo: false, restoreEpoch: "2", deploymentID: "66666666-6666-4666-8666-666666666666",
                           sourceContextID: "personal", deviceID: "77777777-7777-4777-8777-777777777777")
    }

    private func original(_ kind: String = "draft") -> IMJSON {
        let body: IMJSON = .object(["requestId": .string("opaque-original-request"), "revisionEpoch": .string("2"),
                                    "expectedVersion": .string("9007199254740993"), "draftAdmission": .string("da1.original"),
                                    "ownedText": .string("Independent \"text\"\n原字节"), "reply": .null])
        return .object(["id": .string("opaque-original-request"), "path": .string("conversations/" + conversation + "/" + kind),
                        "method": .string("PUT"), "conversationId": .string(conversation), "familyId": .string("owned-family"),
                        "body": body, "state": .string("pending")])
    }

    private func result(_ status: String = "applied") -> IMJSON {
        return .object(["operationId": .string("33333333-3333-4333-8333-333333333333"), "status": .string(status),
                        "current": .bool(false), "result": .object(["revisionEpoch": .string("2"), "version": .string("9007199254740994"),
                        "contentState": .string("owned"), "replyState": .string("none")])])
    }

    private func draft() -> IMJSON {
        return .object(["id": .string(message), "scopeId": .string(namespace.deploymentID), "sourceContextId": .string("personal"),
                        "userId": .string(namespace.account), "conversationId": .string(conversation), "revisionEpoch": .string("2"),
                        "version": .string("9007199254740993"), "contentState": .string("owned"), "ownedText": .string("own input"),
                        "replyState": .string("none"), "reply": .null, "draftAdmission": .string("da1.original"), "writable": .bool(true)])
    }

    func testExactDraftValidatesPrincipalEpochOriginalStringsAndSeparateAdmission() throws {
        let value = draft()
        try IMPersonalStateService.validateCurrent(value, kind: "draft", conversation: conversation, namespace: namespace)
        for wrong in [value.setting("userId", .string(message)), value.setting("version", .number(1)),
                      value.setting("draftAdmission", .string("dr1.not-a-draft-admission")), value.setting("revisionEpoch", .string("1"))] {
            XCTAssertThrowsError(try IMPersonalStateService.validateCurrent(wrong, kind: "draft", conversation: conversation, namespace: namespace))
        }
        XCTAssertEqual(value["version"].text, "9007199254740993")
    }

    func testDraftReadonlyQueryPreservesFullBodyAndMethodWithoutNewProof() {
        let pending = original()
        let query = IMPersonalStateService.resultQuery(pending)
        XCTAssertEqual(query, ["method": pending["method"], "body": pending["body"]])
        XCTAssertEqual(pending["body"]["draftAdmission"], .string("da1.original"))
        XCTAssertNil(query["requestId"])
        XCTAssertEqual(IMPersonalStateService.resultQuery(pending.setting("method", .string("DELETE")))["method"], .string("DELETE"))
    }

    func testPreparedAndRetiredAreDistinctFromAppliedAndDoNotCarryOldText() throws {
        for status in ["prepared", "applied", "retired"] {
            try IMPersonalStateService.validateResult(result(status), original: original(), namespace: namespace)
        }
        XCTAssertThrowsError(try IMPersonalStateService.validateResult(result("unknown"), original: original(), namespace: namespace))
        let leaked = result().setting("result", result()["result"].setting("ownedText", .string("old server body")))
        XCTAssertThrowsError(try IMPersonalStateService.validateResult(leaked, original: original(), namespace: namespace))
    }

    func testReadServerOperationUUIDDoesNotNeedToEqualClientNonceAndIDsStayExact() throws {
        let body: IMJSON = .object(["revisionEpoch": .string("2"), "membershipGeneration": .string("4"),
                                    "observedMessageIds": .array([.string(message)]), "requestId": .string("client-nonce-32hex")])
        let pending = original("read").setting("body", body)
        let value = result().setting("result", .object(["revisionEpoch": .string("2"), "version": .string("1"),
            "membershipGeneration": .string("4"), "readPoint": .object(["epoch": .string("2"), "sequence": .string("0")]),
            "observedMessageIds": .array([.string(message)]), "countStatus": .string("not_computed")]))
        try IMPersonalStateService.validateResult(value, original: pending, namespace: namespace)
        XCTAssertNotEqual(value["operationId"], body["requestId"])
        XCTAssertThrowsError(try IMPersonalStateService.validateResult(value.setting("result", value["result"].setting("observedMessageIds", .array([]))), original: pending, namespace: namespace))
    }

    func testEditorAdoptionRejectsLateLocalTextReplyAndMentionChanges() throws {
        let original = IMPersonalEditorSnapshot(text: "unsent input", reply: .null, mentionIDs: [], mentionAll: false)
        try original.requireUnchanged(original)
        for current in [IMPersonalEditorSnapshot(text: "new input", reply: .null, mentionIDs: [], mentionAll: false),
                        IMPersonalEditorSnapshot(text: original.text, reply: .object(["messageId": .string(message)]), mentionIDs: [], mentionAll: false),
                        IMPersonalEditorSnapshot(text: original.text, reply: .null, mentionIDs: [message], mentionAll: false)] {
            XCTAssertThrowsError(try original.requireUnchanged(current))
        }
    }

    func testRecoveryListsHideByDefaultKeepManualUnreadAndSortOriginalPinRank() {
        func row(_ id: String, _ view: [String: IMJSON], count: String = "0") -> IMJSON {
            return .object(["id": .string(id), "kind": .string("group"), "unreadCount": .string(count), "personalView": .object(view)])
        }
        let values = [row("ordinary", [:]), row("archived", ["archived": .bool(true)]), row("hidden", ["hidden": .bool(true)]),
                      row("manual", ["manualUnread": .bool(true), "pinnedRank": .number(2)])]
        XCTAssertEqual(IMPersonalStateService.displayedConversations(values, filter: 0).map { $0["id"].text }, ["manual", "ordinary"])
        XCTAssertEqual(IMPersonalStateService.displayedConversations(values, filter: 1).map { $0["id"].text }, ["manual"])
        XCTAssertEqual(IMPersonalStateService.displayedConversations(values, filter: 3).map { $0["id"].text }, ["archived"])
        XCTAssertEqual(IMPersonalStateService.displayedConversations(values, filter: 4).map { $0["id"].text }, ["hidden"])
        XCTAssertEqual(values[1]["personalView"]["archived"], .bool(true))
    }

    func testCurrentGuardCanFinishOriginalPendingAfterReopenWithoutRebindingBody() throws {
        let folder = FileManager.default.temporaryDirectory.appendingPathComponent("self-" + UUID().uuidString)
        try FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        defer {
            try? FileManager.default.removeItem(at: folder)
        }
        let file = folder.appendingPathComponent("store.sqlite").path
        let pending = original()
        var first: IMLocalStore? = try IMLocalStore(path: file)
        let initial = try first!.syncState(namespace: namespace)
        try first!.save([pending], collection: "self-state-operations", namespace: namespace)
        try first!.syncRestrict(namespace: namespace, reason: "TEST_NEW_GUARD")
        XCTAssertThrowsError(try first!.completeSelfStateOperation(namespace: namespace, expectedGuard: initial, original: pending, result: result()))
        first = nil
        let reopened = try IMLocalStore(path: file)
        let current = try XCTUnwrap(reopened.syncStateIfPresent(namespace: namespace))
        XCTAssertEqual(try reopened.list("self-state-operations", namespace: namespace).first, pending)
        try reopened.completeSelfStateOperation(namespace: namespace, expectedGuard: current, original: pending, result: result())
        let done = try XCTUnwrap(reopened.list("self-state-operations", namespace: namespace).first)
        XCTAssertEqual(done["body"], pending["body"])
        XCTAssertEqual(done["state"], .string("applied"))
    }

    func testRejectedArchiveIsOriginalBodyCASAndRetiredNeverBecomesCurrentCloudText() throws {
        let store = try IMLocalStore(path: ":memory:")
        let guardState = try store.syncState(namespace: namespace)
        let pending = original()
        try store.save([pending], collection: "self-state-operations", namespace: namespace)
        try store.transitionSelfStateOperation(namespace: namespace, expectedGuard: guardState, original: pending, state: "rejected", reason: "DRAFT_VERSION_CONFLICT")
        XCTAssertThrowsError(try store.transitionSelfStateOperation(namespace: namespace, expectedGuard: guardState, original: pending, state: "archived", reason: ""))
        let rejected = try XCTUnwrap(store.list("self-state-operations", namespace: namespace).first)
        try store.transitionSelfStateOperation(namespace: namespace, expectedGuard: guardState, original: rejected, state: "archived", reason: "DRAFT_VERSION_CONFLICT")
        XCTAssertEqual(try store.list("self-state-operations", namespace: namespace).first?["body"], pending["body"])
        let next = pending.setting("id", .string("next-original-request"))
        try store.save([next], collection: "self-state-operations", namespace: namespace)
        try store.completeSelfStateOperation(namespace: namespace, expectedGuard: guardState, original: next, result: result("retired"))
        XCTAssertTrue(try store.list("self:draft", namespace: namespace).isEmpty)
        XCTAssertEqual(try store.list("self-state-operations", namespace: namespace).first { $0["id"] == next["id"] }?["state"], .string("retired"))
    }
}
