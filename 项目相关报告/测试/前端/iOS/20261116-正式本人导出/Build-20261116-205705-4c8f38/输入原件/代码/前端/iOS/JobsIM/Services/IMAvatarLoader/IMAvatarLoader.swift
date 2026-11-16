//
//  IMAvatarLoader.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import JobsIMCore
import UIKit

@MainActor
enum IMAvatarLoader {
    static func image(_ path: String, namespace: IMNamespace) async -> UIImage? {
        let session = IMSession.shared
        guard session.namespace == namespace, !session.demo,
            session.api != nil, session.token != nil,
            path.hasPrefix("/api/avatars/")
        else {
            return nil
        }
        let id = String(path.dropFirst("/api/avatars/".count))
        guard !id.isEmpty, id.range(of: "^[A-Za-z0-9_-]+$", options: .regularExpression) != nil else {
            return nil
        }
        // 每次显示都重新经过服务端ACL；不以旧的本地成功绕过失权或资料隐藏。
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer {
            try? FileManager.default.removeItem(at: file)
        }
        do {
            _ = try await session.verifyRestoreEpoch()
            guard session.namespace == namespace else { return nil }
            _ = try await session.authorized { api, token in
                return try await api.download(id, to: file, token: token, collection: "avatars")
            }
            guard !Task.isCancelled, session.namespace == namespace else {
                return nil
            }
            return UIImage.make(contentsOfFile: file.path)
        } catch {
            return nil
        }
    }
}
