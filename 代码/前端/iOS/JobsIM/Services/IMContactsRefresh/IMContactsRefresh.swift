//
//  IMContactsRefresh.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation
import JobsIMCore

@MainActor
final class IMContactsRefresh {
    private let isCurrent: () -> Bool
    private let load: (Bool) async -> [IMJSON]
    private let apply: ([IMJSON]) -> Void
    private let didFinish: () -> Void
    private var generation = UUID()
    private var task: Task<Void, Never>?

    init(
        isCurrent: @escaping () -> Bool,
        load: @escaping (Bool) async -> [IMJSON],
        apply: @escaping ([IMJSON]) -> Void,
        didFinish: @escaping () -> Void = {}
    ) {
        self.isCurrent = isCurrent
        self.load = load
        self.apply = apply
        self.didFinish = didFinish
    }

    @discardableResult
    func reload() -> Task<Void, Never>? {
        return start(synchronize: true)
    }

    @discardableResult
    func projectionChanged() -> Task<Void, Never>? {
        // 本次同步也会通知观察者；在途读取会使用最新投影，不能取消自身再同步。
        guard task == nil else {
            return nil
        }
        return start(synchronize: false)
    }

    func cancel() {
        generation = UUID()
        task?.cancel()
        task = nil
        didFinish()
    }

    private func start(synchronize: Bool) -> Task<Void, Never>? {
        generation = UUID()
        task?.cancel()
        task = nil
        guard isCurrent() else {
            didFinish()
            return nil
        }
        let original = generation
        let running = Task { [weak self] in
            guard let self else {
                return
            }
            defer {
                if self.generation == original {
                    self.task = nil
                    self.didFinish()
                }
            }
            let rows = await self.load(synchronize)
            guard !Task.isCancelled, self.generation == original, self.isCurrent() else {
                return
            }
            self.apply(rows)
        }
        task = running
        return running
    }
}
