//
//  IMAvatarService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import UniformTypeIdentifiers

import JobsIMCore
import JobsIMTransport

@MainActor
final class IMAvatarService {
    private let session = IMSession.shared
    private var directory: URL {
        let folder = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("AvatarOperations", isDirectory: true)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        return folder
    }
    func prepare(_ source: URL) async throws {
        guard !session.demo else {
            return
        }
        let bytes = try Data(contentsOf: source)
        guard bytes.count <= 2 * 1024 * 1024, bytes.count > 0 else {
            throw IMAPIError(status: 413, code: "AVATAR_LIMIT", message: "JPEG / PNG ≤ 2 MiB")
        }
        let type = UTType(filenameExtension: source.pathExtension)
        guard type == .jpeg || type == .png else {
            throw IMAPIError(status: 400, code: "STATIC_AVATAR_REQUIRED", message: "JPEG / PNG")
        }
        let ns = session.namespace
        if !(try session.store.list("avatar-operations", namespace: ns)).isEmpty {
            try await retry()
            guard (try session.store.list("avatar-operations", namespace: ns)).isEmpty else {
                throw IMAPIError(
                    status: 409, code: "AVATAR_OPERATION_PENDING", message: IMStrings.text("retryAvatar"))
            }
        }
        let user = try await session.action("me", method: "GET")
        guard ns == session.namespace else {
            return
        }
        let id = UUID().uuidString
        let fileName = ns.key + "-" + id + "." + source.pathExtension
        try bytes.write(to: directory.appendingPathComponent(fileName), options: .atomic)
        let operation = IMJSON.object([
            "id": .string(id), "fileName": .string(fileName), "expectedVersion": user["profileVersion"],
            "mime": .string(type == .png ? "image/png" : "image/jpeg"),
        ])
        do {
            try session.store.save([operation], collection: "avatar-operations", namespace: ns)
        } catch {
            try? FileManager.default.removeItem(at: directory.appendingPathComponent(fileName))
            throw error
        }
        try await retry()
    }
    func retry() async throws {
        try await session.verifyRestoreEpoch()
        let ns = session.namespace
        guard !session.demo, session.api != nil, session.token != nil,
            let operation = try session.store.list("avatar-operations", namespace: ns).first
        else {
            return
        }
        let id = operation["id"].text
        var result: IMJSON?
        do { result = try await session.action("me/avatar/operations/" + id, method: "GET") } catch let error
            as IMAPIError where error.status == 404
        {}
        if result == nil || result?["status"].text == "prepared" {
            do {
                result = try await session.authorized { api, token in
                    return try await api.upload(
                        self.directory.appendingPathComponent(operation["fileName"].text),
                        mimeType: operation["mime"].text, token: token, path: "me/avatar",
                        form: ["requestId": id, "expectedVersion": operation["expectedVersion"].text.isEmpty
                            ? "1" : operation["expectedVersion"].text])
                }
            } catch let failure as IMAPIError {
                if [400, 413].contains(failure.status) || failure.code == "VERSION_CONFLICT" {
                    try session.store.save(
                        [operation.setting("failureCode", .string(failure.code))],
                        collection: "avatar-history", namespace: ns)
                    try session.store.remove(id, collection: "avatar-operations", namespace: ns)
                }
                throw failure
            }
        }
        guard session.namespace == ns else {
            return
        }
        if ["applied", "cancelled", "superseded"].contains(result?["status"].text ?? "") {
            // 原操作结果只作对账；即使current=false，也只用GET me更新当前头像。
            let user = try await session.action("me", method: "GET")
            guard session.namespace == ns else {
                return
            }
            session.user = user
            UserDefaults.standard.set(try JSONEncoder.make().encode(user), forKey: "user:" + session.profile)
            try session.store.remove(id, collection: "avatar-operations", namespace: ns)
            try? FileManager.default.removeItem(
                at: directory.appendingPathComponent(operation["fileName"].text))
        }
    }
}
