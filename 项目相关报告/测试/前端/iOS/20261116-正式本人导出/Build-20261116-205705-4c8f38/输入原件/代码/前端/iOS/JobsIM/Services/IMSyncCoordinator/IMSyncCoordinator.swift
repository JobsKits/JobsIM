//
//  IMSyncCoordinator.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation

@MainActor
final class IMSyncCoordinator {
    static let shared = IMSyncCoordinator()
    private var task: Task<Void, Never>?
    func start() {
        stop()
        let session = IMSession.shared
        guard session.token != nil, !session.demo else {
            return
        }
        let namespace = session.namespace
        task = Task {
            while !Task.isCancelled, session.namespace == namespace, session.token != nil, !session.demo {
                await session.synchronizeAuthoritatively()
                try? await Task.sleep(for: .seconds(3))
            }
        }
    }
    func stop() {
        task?.cancel()
        task = nil
        IMSession.shared.stopAuthoritativeSync()
    }
}
