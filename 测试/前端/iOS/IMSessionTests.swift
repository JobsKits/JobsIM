//
//  IMSessionTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import XCTest
import UIKit

import JobsIMCore
import JobsIMTransport
@testable import JobsIM

@MainActor
final class IMSessionTests: XCTestCase {
    func testDirectChatTitleUsesAuthorizedPeerEvenWhenConversationHasNoTitle() {
        let current = "11111111-1111-4111-8111-111111111111"
        let peer = "22222222-2222-4222-8222-222222222222"
        let person: IMJSON = .object([
            "id": .string(peer), "displayName": .string("小林"), "username": .string("lin"),
        ])
        let conversation: IMJSON = .object([
            "id": .string("33333333-3333-4333-8333-333333333333"),
            "kind": .string("direct"), "peer": person,
        ])
        XCTAssertEqual(IMChatVC.displayTitle(conversation: conversation, members: [], currentUserID: current), "小林")
        let own: IMJSON = .object(["id": .string(current), "displayName": .string("本人")])
        let updated = person.setting("displayName", .string("小林的新昵称"))
        XCTAssertEqual(IMChatVC.displayTitle(conversation: conversation, members: [own, updated], currentUserID: current), "小林的新昵称")
        let unrelated: IMJSON = .object(["id": .string("other"), "displayName": .string("其他成员")])
        XCTAssertEqual(IMChatVC.displayTitle(conversation: conversation, members: [own, unrelated], currentUserID: current), "小林")
    }

    func testChatTitleKeepsGroupNameAndResolvesDirectMemberWithoutLeakingOwnName() {
        let current = "11111111-1111-4111-8111-111111111111"
        let own: IMJSON = .object(["id": .string(current), "displayName": .string("本人")])
        let peer: IMJSON = .object([
            "id": .string("22222222-2222-4222-8222-222222222222"), "username": .string("lin"),
        ])
        let group: IMJSON = .object([
            "id": .string("group-id"), "kind": .string("group"), "title": .string("项目群"), "peer": peer,
        ])
        XCTAssertEqual(IMChatVC.displayTitle(conversation: group, members: [own, peer], currentUserID: current), "项目群")
        let direct = group.setting("kind", .string("direct")).setting("peer", .null).setting("title", .null)
        XCTAssertEqual(IMChatVC.displayTitle(conversation: direct, members: [own, peer], currentUserID: current), "lin")
        XCTAssertEqual(IMChatVC.displayTitle(conversation: direct, members: [own], currentUserID: current), "group-id")
        let personal = direct.setting("kind", .string("self"))
        XCTAssertEqual(IMChatVC.displayTitle(conversation: personal, members: [peer, own], currentUserID: current), "本人")
    }

    func testContactsSyncNotificationDoesNotCancelItsRefreshOrRequestAgain() async throws {
        let contact: IMJSON = .object([
            "id": .string("11111111-1111-4111-8111-111111111111"),
            "displayName": .string("小林"),
        ])
        var refresh: IMContactsRefresh!
        var requests: [Bool] = []
        var displayed: [[IMJSON]] = []
        refresh = IMContactsRefresh(
            isCurrent: { true },
            load: { synchronize in
                requests.append(synchronize)
                // 模拟权威同步完成时同步发出的观察者通知，包括同轮通知风暴。
                for _ in 0..<20 {
                    XCTAssertNil(refresh.projectionChanged())
                }
                XCTAssertFalse(Task.isCancelled)
                return [contact]
            },
            apply: { displayed.append($0) })
        let first = try XCTUnwrap(refresh.reload())
        await first.value
        XCTAssertEqual(requests, [true])
        XCTAssertEqual(displayed, [[contact]])

        // 后续外部投影通知只重新读取/补齐资料，不再触发一轮同步通知。
        let external = try XCTUnwrap(refresh.projectionChanged())
        await external.value
        XCTAssertEqual(requests, [true, false])
        XCTAssertEqual(displayed, [[contact], [contact]])
    }

    func testExplicitContactsRefreshSupersedesOldResultWithoutDroppingCurrentTask() async throws {
        var entered: AsyncStream<Int>.Continuation!
        let starts = AsyncStream<Int> { entered = $0 }
        var iterator = starts.makeAsyncIterator()
        var pending: [CheckedContinuation<[IMJSON], Never>] = []
        var displayed: [[IMJSON]] = []
        var finished = 0
        let refresh = IMContactsRefresh(
            isCurrent: { true },
            load: { _ in
                return await withCheckedContinuation { continuation in
                    pending.append(continuation)
                    entered.yield(pending.count)
                }
            },
            apply: { displayed.append($0) },
            didFinish: { finished += 1 })
        let first = try XCTUnwrap(refresh.reload())
        let firstStarted = await iterator.next()
        XCTAssertEqual(firstStarted, 1)
        let second = try XCTUnwrap(refresh.reload())
        let secondStarted = await iterator.next()
        XCTAssertEqual(secondStarted, 2)
        pending[0].resume(returning: [.object(["id": .string("old")])])
        await first.value
        XCTAssertTrue(displayed.isEmpty)
        XCTAssertEqual(finished, 0)
        XCTAssertNil(refresh.projectionChanged())
        let current: IMJSON = .object(["id": .string("current")])
        pending[1].resume(returning: [current])
        await second.value
        XCTAssertEqual(displayed, [[current]])
        XCTAssertEqual(finished, 1)
        entered.finish()
    }

    func testContactsRefreshCannotPublishAfterNamespaceChangesOrScreenCancellation() async throws {
        var entered: AsyncStream<Void>.Continuation!
        let starts = AsyncStream<Void> { entered = $0 }
        var iterator = starts.makeAsyncIterator()
        var pending: CheckedContinuation<[IMJSON], Never>?
        var current = true
        var displayed: [[IMJSON]] = []
        let refresh = IMContactsRefresh(
            isCurrent: { current },
            load: { _ in
                return await withCheckedContinuation { continuation in
                    pending = continuation
                    entered.yield(())
                }
            },
            apply: { displayed.append($0) })
        let first = try XCTUnwrap(refresh.reload())
        _ = await iterator.next()
        current = false
        pending?.resume(returning: [.object(["id": .string("previous-account")])])
        await first.value
        XCTAssertTrue(displayed.isEmpty)
        XCTAssertNil(refresh.reload())

        current = true
        let second = try XCTUnwrap(refresh.reload())
        _ = await iterator.next()
        refresh.cancel()
        pending?.resume(returning: [.object(["id": .string("hidden-screen")])])
        await second.value
        XCTAssertTrue(displayed.isEmpty)
        entered.finish()
    }

    func testReadableThemeColorsKeepAccentAndWhitePrimaryTextDistinct() throws {
        func luminance(_ color: UIColor, traits: UITraitCollection) throws -> Double {
            var red: CGFloat = 0
            var green: CGFloat = 0
            var blue: CGFloat = 0
            var alpha: CGFloat = 0
            let resolved = color.resolvedColor(with: traits)
            XCTAssertTrue(resolved.getRed(&red, green: &green, blue: &blue, alpha: &alpha))
            XCTAssertEqual(alpha, 1)
            let linear = [red, green, blue].map { channel -> Double in
                let value = Double(channel)
                return value <= 0.04045 ? value / 12.92 : pow((value + 0.055) / 1.055, 2.4)
            }
            return linear[0] * 0.2126 + linear[1] * 0.7152 + linear[2] * 0.0722
        }
        func contrast(_ foreground: UIColor, _ background: UIColor, traits: UITraitCollection) throws -> Double {
            let first = try luminance(foreground, traits: traits)
            let second = try luminance(background, traits: traits)
            return (max(first, second) + 0.05) / (min(first, second) + 0.05)
        }
        for style in [UIUserInterfaceStyle.light, .dark] {
            let traits = UITraitCollection(userInterfaceStyle: style)
            for background in [IMUI.background, IMUI.panel] {
                for foreground in [IMUI.foreground, IMUI.secondary, IMUI.green] {
                    XCTAssertGreaterThanOrEqual(try contrast(foreground, background, traits: traits), 4.5)
                }
            }
            XCTAssertGreaterThanOrEqual(try contrast(.white, IMUI.greenFill, traits: traits), 4.5)
        }
    }

    func testReadOnlyDoesNotQueueOrConsumeDraft() async throws {
        let session = IMSession.shared
        let ns = session.namespace
        let id = UUID().uuidString
        let draft = IMJSON.object(["id": .string(id), "text": .string("Keep my draft")])
        try session.store.save(
            [.object(["id": .string(id), "readOnly": .bool(true)])], collection: "conversations",
            namespace: ns)
        try session.store.save([draft], collection: "drafts", namespace: ns)
        let pending = try session.store.pending(namespace: ns)
        defer {
            try? session.store.remove(id, collection: "conversations", namespace: ns)
            try? session.store.remove(id, collection: "drafts", namespace: ns)
        }
        do {
            try await session.send(
                "Keep my draft", conversation: id, didEnqueue: { XCTFail("Read-only intent committed") })
            XCTFail("Read-only send accepted")
        } catch let error as IMAPIError { XCTAssertEqual(error.code, "CONVERSATION_READ_ONLY") }
        XCTAssertEqual(try session.store.pending(namespace: ns), pending)
        XCTAssertTrue(try session.store.list("drafts", namespace: ns).contains(draft))
    }
    func testInputMayClearOnlyAfterOutboxCommit() async throws {
        let session = IMSession.shared
        let originalDemo = session.demo
        session.demo = true
        let ns = session.namespace
        let id = UUID().uuidString
        var committed = false
        defer {
            try? session.store.remove(id, collection: "messages:" + id, namespace: ns)
            session.demo = originalDemo
        }
        try await session.send(
            "Commit before clearing", conversation: id, clientID: id,
            didEnqueue: {
                committed =
                    (try? session.store.pending(namespace: ns))?.contains { $0["clientMsgId"].text == id }
                    == true
            })
        XCTAssertTrue(committed)
        XCTAssertFalse(try session.store.pending(namespace: ns).contains { $0["clientMsgId"].text == id })
        XCTAssertTrue(
            try session.store.list("messages:" + id, namespace: ns).contains { $0["clientMsgId"].text == id })
    }
}
