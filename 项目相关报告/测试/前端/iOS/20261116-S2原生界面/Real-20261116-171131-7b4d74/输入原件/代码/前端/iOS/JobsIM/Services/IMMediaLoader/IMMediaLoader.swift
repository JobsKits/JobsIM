//
//  IMMediaLoader.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import UIKit

import JobsIMCore

@MainActor
enum IMMediaLoader {
    static func image(_ message: IMJSON, namespace: IMNamespace) async -> UIImage? {
        do {
            let file = try await IMMediaCache.file(message, namespace: namespace)
            guard !Task.isCancelled, IMSession.shared.namespace == namespace else {
                return nil
            }
            return UIImage.make(contentsOfFile: file.path)
        } catch { return nil }
    }
}
