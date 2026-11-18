//
//  IMReceiptCoordinator.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import UIKit

import JobsIMCore

@MainActor
final class IMReceiptCoordinator: NSObject {

    typealias Factory = @MainActor (IMSession, @escaping @MainActor () -> Bool) throws -> IMReceiptService
    enum State: Equatable {
        case off, verifying, running, paused, blocked, unknown
    }

    static let shared = IMReceiptCoordinator()
    private let session: IMSession
    private let factory: Factory
    private let foreground: @MainActor () -> Bool
    private var consent: (namespace: IMNamespace, family: String, physical: String)?
    private var generation = UUID()
    private var service: IMReceiptService?
    private var context: IMJSON?
    private var baseline = Set<String>()
    private var cooldown: [String: Date] = [:]
    private var pendingCursor = 0
    private var targetCursor = 0
    private var enabled = false
    private var active = true
    private var dirty = false
    private var worker: Task<Void, Never>?
    private var timer: Task<Void, Never>?
    private var syncObservation: UUID?
    private var observers: [UUID: @MainActor () -> Void] = [:]
    private(set) var state: State = .off
    private(set) var unknownCount = 0

    init(session: IMSession = .shared, observeLifecycle: Bool = true,
         foreground: @escaping @MainActor () -> Bool = { UIApplication.shared.applicationState == .active },
         factory: Factory? = nil) {
        self.session = session
        self.foreground = foreground
        self.factory = factory ?? { session, current in
            return try IMReceiptService(session: session, isCurrent: current)
        }
        super.init()
        if observeLifecycle {
            NotificationCenter.default.addObserver(self, selector: #selector(pause),
                name: UIApplication.willResignActiveNotification, object: nil)
            NotificationCenter.default.addObserver(self, selector: #selector(resume),
                name: UIApplication.didBecomeActiveNotification, object: nil)
        }
    }

    deinit {
        worker?.cancel()
        timer?.cancel()
    }

    func observe(_ callback: @escaping @MainActor () -> Void) -> UUID {
        let id = UUID()
        observers[id] = callback
        return id
    }

    func removeObserver(_ id: UUID) {
        observers.removeValue(forKey: id)
    }

    private func notify() {
        for callback in Array(observers.values) {
            callback()
        }
    }

    private func bound() -> Bool {
        guard enabled, let consent, !session.demo, session.namespace == consent.namespace,
            session.currentCredentialPair?.familyID == consent.family,
            let current = try? session.store.syncStateIfPresent(namespace: consent.namespace),
            current["storeGeneration"].text == consent.physical else {
            return false
        }
        return true
    }

    private func isCurrent(_ owner: UUID) -> Bool {
        return generation == owner && active && foreground() && bound()
    }

    func isEnabled(for candidate: IMSession) -> Bool {
        return candidate === session && bound()
    }

    func isQualified(for candidate: IMSession, objects: Bool = false) -> Bool {
        guard candidate === session, bound(), active, foreground(), let context else {
            return false
        }
        return service?.locallyQualified(context, objects: objects) == true
    }

    func enable() async throws {
        disable()
        guard !session.demo, let pair = session.currentCredentialPair,
            let current = try session.store.syncStateIfPresent(namespace: session.namespace),
            !current["streamId"].text.isEmpty, current["restriction"].text.isEmpty, foreground() else {
            try IMReceiptProtocol.fail("RECEIPT_SYNC_REQUIRED")
        }
        let physical = try IMReceiptProtocol.id(current["storeGeneration"])
        consent = (session.namespace, pair.familyID, physical)
        enabled = true
        active = true
        generation = UUID()
        let owner = generation
        let next = try factory(session, { [weak self] in
            return self?.isCurrent(owner) == true
        })
        baseline = Set(try next.candidates().map { Self.key($0) })
        service = next
        state = .verifying
        syncObservation = session.observeSync { [weak self] in
            self?.wake()
        }
        timer = Task { [weak self] in
            while !Task.isCancelled {
                do {
                    try await Task.sleep(nanoseconds: 3_000_000_000)
                } catch {
                    return
                }
                self?.wake()
            }
        }
        await refresh()
    }

    func disable() {
        generation = UUID()
        enabled = false
        worker?.cancel()
        worker = nil
        timer?.cancel()
        timer = nil
        if let syncObservation {
            session.removeSyncObserver(syncObservation)
        }
        syncObservation = nil
        consent = nil
        service = nil
        context = nil
        baseline = []
        cooldown = [:]
        dirty = false
        pendingCursor = 0
        targetCursor = 0
        unknownCount = 0
        state = .off
        notify()
    }

    @objc func pause() {
        guard enabled else {
            return
        }
        active = false
        generation = UUID()
        worker?.cancel()
        worker = nil
        service = nil
        context = nil
        state = .paused
        notify()
    }

    @objc func resume() {
        guard enabled else {
            return
        }
        worker?.cancel()
        worker = nil
        dirty = false
        active = true
        generation = UUID()
        service = nil
        wake()
    }

    func refresh() async {
        wake()
        await worker?.value
    }

    private func wake() {
        guard enabled else {
            return
        }
        guard bound() else {
            disable()
            return
        }
        guard active, foreground() else {
            return
        }
        if worker != nil {
            dirty = true
            return
        }
        let owner = generation
        worker = Task { [weak self] in
            guard let self else {
                return
            }
            await self.cycle(owner)
            if self.generation == owner {
                self.worker = nil
                self.notify()
                if self.dirty {
                    self.dirty = false
                    self.wake()
                }
            }
        }
    }

    private static func key(_ message: IMJSON) -> String {
        return message["id"].text + ":" + message["contentVersion"].text
    }

    private func batch(_ rows: [IMJSON], cursor: inout Int, limit: Int) -> [IMJSON] {
        guard !rows.isEmpty else {
            cursor = 0
            return []
        }
        let result = (0..<min(limit, rows.count)).map { rows[(cursor + $0) % rows.count] }
        cursor = (cursor + result.count) % rows.count
        return result
    }

    private func cycle(_ owner: UUID) async {
        guard isCurrent(owner) else {
            return
        }
        do {
            let currentService: IMReceiptService
            if let service {
                currentService = service
            } else {
                currentService = try factory(session, { [weak self] in
                    return self?.isCurrent(owner) == true
                })
                service = currentService
            }
            let pending = try currentService.intents().filter { ["prepared", "unknown"].contains($0["state"].text) }
            unknownCount = pending.count
            for row in batch(pending, cursor: &pendingCursor, limit: 10) {
                guard isCurrent(owner) else {
                    return
                }
                // A background retry is a pure result lookup, including 404.
                _ = try? await currentService.checkReceipt(row["id"].text)
            }
            let observed = try await currentService.context()
            guard isCurrent(owner) else {
                return
            }
            context = observed
            guard observed["storage"]["qualified"] == .bool(true) else {
                state = .blocked
                return
            }
            if currentService.verification == nil {
                state = .verifying
                _ = try await currentService.verify(includeObjects: observed["storage"]["storageContract"].text == IMReceiptProtocol.contracts[1])
            } else if !currentService.locallyQualified(observed) {
                _ = try await currentService.refreshProfiles()
            }
            guard isCurrent(owner), currentService.locallyQualified(observed) else {
                state = .blocked
                return
            }
            state = .running
            let candidates = try currentService.candidates(), originals = try currentService.intents()
            let existing = Set(originals.filter { $0["kind"].text == "content" }.map { $0["messageId"].text + ":" + $0["contentVersion"].text })
            let available = candidates.filter { !existing.contains(Self.key($0)) && (cooldown[Self.key($0)] ?? .distantPast) <= Date() }
            var admitted = 0
            for message in available.filter({ !baseline.contains(Self.key($0)) }).prefix(10) {
                guard isCurrent(owner) else {
                    return
                }
                do {
                    _ = try await currentService.receiveContent(message["id"].text)
                    admitted += 1
                } catch {
                    cooldown[Self.key(message)] = Date().addingTimeInterval(30)
                }
            }
            // An existing unacknowledged frozen target must not disappear into
            // the startup baseline. Other historical messages stay opt-in.
            let historical = available.filter { baseline.contains(Self.key($0)) }
            for message in batch(historical, cursor: &targetCursor, limit: 10) {
                guard isCurrent(owner) else {
                    return
                }
                do {
                    let target = try await currentService.target(message["id"].text)
                    guard target["contentTarget"] == .bool(true), target["originalTarget"] == .bool(true),
                        target["messageCurrent"] == .bool(true), target["contentRecorded"] == .bool(false),
                        target["qualificationId"] == observed["storage"]["qualificationId"],
                        target["contentVersion"] == message["contentVersion"] else {
                        cooldown[Self.key(message)] = Date().addingTimeInterval(30)
                        continue
                    }
                    _ = try await currentService.receiveContent(message["id"].text, expectedTarget: target)
                    admitted += 1
                } catch {
                    cooldown[Self.key(message)] = Date().addingTimeInterval(30)
                }
            }
            guard isCurrent(owner) else {
                return
            }
            cooldown = cooldown.filter { $0.value > Date() }
            unknownCount = try currentService.intents().filter { ["prepared", "unknown"].contains($0["state"].text) }.count
            if unknownCount > 0 && admitted == 0 {
                state = .unknown
            }
        } catch {
            if isCurrent(owner) {
                state = .blocked
            }
        }
    }
}
