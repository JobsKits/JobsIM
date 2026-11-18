//
//  IMAccountDeletionFiles.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import Darwin
import Foundation
import JobsIMCore

enum IMAccountDeletionFiles {

    private enum CleanupError: Error { case unsafeResource, unavailable }

    static func clear(namespaceKeys: [String], files: [IMJSON]) async throws {
        let worker = Task.detached(priority: .utility) {
            try clearFiles(namespaceKeys: namespaceKeys, files: files)
        }
        try await withTaskCancellationHandler {
            try await worker.value
        } onCancel: {
            worker.cancel()
        }
    }

    private static func kind(_ url: URL) throws -> mode_t? {
        var info = stat()
        guard lstat(url.path, &info) == 0 else {
            if errno == ENOENT {
                return nil
            }
            throw CleanupError.unavailable
        }
        return info.st_mode & mode_t(S_IFMT)
    }

    private static func sync(_ directory: URL) throws {
        let descriptor = open(directory.path, O_RDONLY | O_CLOEXEC)
        guard descriptor >= 0 else {
            throw CleanupError.unavailable
        }
        defer {
            close(descriptor)
        }
        guard fsync(descriptor) == 0 else {
            throw CleanupError.unavailable
        }
    }

    private static func clearFlatFolder(_ directory: URL, removeFolder: Bool, prefix: String? = nil) throws {
        guard let directoryKind = try kind(directory) else {
            return
        }
        guard directoryKind == mode_t(S_IFDIR) else {
            throw CleanupError.unsafeResource
        }
        let entries = try FileManager.default.contentsOfDirectory(at: directory, includingPropertiesForKeys: nil)
        for file in entries {
            if let prefix, !file.lastPathComponent.hasPrefix(prefix) {
                continue
            }
            try Task.checkCancellation()
            guard try kind(file) == mode_t(S_IFREG) else {
                throw CleanupError.unsafeResource
            }
            try FileManager.default.removeItem(at: file)
        }
        try sync(directory)
        if removeFolder {
            try FileManager.default.removeItem(at: directory)
            try sync(directory.deletingLastPathComponent())
        }
    }

    private static func clearFiles(namespaceKeys: [String], files: [IMJSON]) throws {
        let manager = FileManager.default
        let media = manager.urls(for: .cachesDirectory, in: .userDomainMask)[0].appendingPathComponent("Media")
        let avatars = manager.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("AvatarOperations")
        let receipts = manager.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0].appendingPathComponent("ReceiptObjects")
        if let mediaKind = try kind(media), mediaKind != mode_t(S_IFDIR) {
            throw CleanupError.unsafeResource
        }
        if let receiptKind = try kind(receipts), receiptKind != mode_t(S_IFDIR) {
            throw CleanupError.unsafeResource
        }
        for key in Set(namespaceKeys) {
            guard key.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil else {
                throw CleanupError.unsafeResource
            }
            try clearFlatFolder(media.appendingPathComponent(key), removeFolder: true)
            try clearFlatFolder(receipts.appendingPathComponent(key), removeFolder: true)
            try clearFlatFolder(avatars, removeFolder: false, prefix: key + "-")
        }
        for file in files {
            guard file["kind"].text == "export",
                file["name"].text.range(of: "^JobsIMDataExport-[0-9A-Fa-f-]{36}$", options: .regularExpression) != nil else {
                throw CleanupError.unsafeResource
            }
            try clearFlatFolder(manager.temporaryDirectory.appendingPathComponent(file["name"].text), removeFolder: true)
        }
    }
}
