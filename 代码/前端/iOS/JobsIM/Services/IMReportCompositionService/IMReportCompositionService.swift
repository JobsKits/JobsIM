//
//  IMReportCompositionService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import CryptoKit
import Darwin
import Foundation
import JobsIMCore
import JobsIMTransport

struct IMReportEvidenceSelection {
    let url: URL
    let name: String
    let mimeType: String
}

struct IMReportComposition {
    var targetType: String
    var targetID: String
    var targetUserID: String? = nil
    var reason: String
    var messageIDs: [String]
    var evidence: [IMReportEvidenceSelection]
}

@MainActor
final class IMReportCompositionService {
    private let session: IMSession
    private let collection = "report-composition-outbox"

    init(session: IMSession) {
        self.session = session
    }

    static func digest(_ file: URL) throws -> String {
        let handle = try FileHandle(forReadingFrom: file)
        defer {
            try? handle.close()
        }
        var hash = SHA256()
        while let chunk = try handle.read(upToCount: 262_144), !chunk.isEmpty {
            hash.update(data: chunk)
        }
        return hash.finalize().map {
            String(format: "%02x", $0)
        }.joined()
    }

    private static func directory(namespace: IMNamespace, intentID: String, create: Bool) throws -> URL {
        guard namespace.key.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil,
            UUID(uuidString: intentID) != nil else {
            throw IMSession.SessionError.invalidInstance
        }
        var current = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .resolvingSymlinksInPath()
        for component in ["JobsIM", "ReportOutbox", namespace.key, intentID] {
            current.appendPathComponent(component, isDirectory: true)
            var info = stat()
            if lstat(current.path, &info) != 0 {
                guard create, errno == ENOENT else {
                    throw CocoaError(.fileReadNoSuchFile)
                }
                try FileManager.default.createDirectory(at: current, withIntermediateDirectories: false,
                    attributes: [.posixPermissions: 0o700])
                guard lstat(current.path, &info) == 0 else {
                    throw CocoaError(.fileWriteUnknown)
                }
            }
            guard info.st_mode & mode_t(S_IFMT) == mode_t(S_IFDIR) else {
                throw CocoaError(.fileReadNoPermission)
            }
        }
        return current
    }

    private static func file(_ upload: IMJSON, namespace: IMNamespace, intentID: String) throws -> URL {
        let folder = try directory(namespace: namespace, intentID: intentID, create: false)
        let requestID = upload["requestId"].text
        guard UUID(uuidString: requestID) != nil else {
            throw IMSession.SessionError.invalidInstance
        }
        let file = folder.appendingPathComponent(requestID + ".evidence")
        guard upload["file"].text == file.path else {
            throw CocoaError(.fileReadNoPermission)
        }
        var info = stat()
        guard lstat(file.path, &info) == 0, info.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG) else {
            throw CocoaError(.fileReadNoPermission)
        }
        return file
    }

    private static func syncDirectory(_ directory: URL) throws {
        let descriptor = open(directory.path, O_RDONLY | O_CLOEXEC | O_NOFOLLOW)
        guard descriptor >= 0 else {
            throw CocoaError(.fileWriteUnknown)
        }
        defer {
            close(descriptor)
        }
        guard fsync(descriptor) == 0 else {
            throw CocoaError(.fileWriteUnknown)
        }
    }

    private func checkContext(_ expected: IMNamespace, family: String?) throws {
        guard session.namespace == expected,
            session.currentCredentialPair?.familyID == family,
            session.demo || session.token != nil
        else {
            throw IMSession.SessionError.noInstance
        }
        if !session.demo, let pair = session.currentCredentialPair,
            try session.store.accountIsErased(
                profile: pair.profile, deployment: pair.deploymentID, user: pair.userID)
        {
            throw IMSession.SessionError.noInstance
        }
    }

    private func selectionMetadata(_ selection: IMReportEvidenceSelection) throws -> IMJSON {
        let attributes = try selection.url.resourceValues(forKeys: [
            .isRegularFileKey, .isSymbolicLinkKey, .fileSizeKey,
        ])
        let name = URL(fileURLWithPath: selection.name).lastPathComponent
        let size = attributes.fileSize ?? 0
        let image = selection.mimeType.hasPrefix("image/")
        let video = selection.mimeType.hasPrefix("video/")
        guard selection.url.isFileURL, attributes.isRegularFile == true,
            attributes.isSymbolicLink != true, !name.isEmpty, name.utf8.count <= 180,
            !name.unicodeScalars.contains(where: {
                CharacterSet.controlCharacters.contains($0)
            }),
            image || video, size > 0, size <= (image ? 20 : 64) * 1_024 * 1_024
        else {
            throw IMSession.SessionError.invalidInstance
        }
        return .object([
            "name": .string(name), "size": .string(String(size)), "mimeType": .string(selection.mimeType),
            "sha256": .string(try Self.digest(selection.url)),
        ])
    }

    func submit(_ composition: IMReportComposition) async throws -> IMJSON {
        let ns = session.namespace
        let family = session.currentCredentialPair?.familyID
        let reason = composition.reason.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !reason.isEmpty, reason.unicodeScalars.count <= 1_000 else {
            throw IMAPIError(status: 400, code: "REPORT_REASON_LIMIT",
                message: IMStrings.text("reportReason") + " · 1–1000")
        }
        guard ["message", "user", "conversation"].contains(composition.targetType),
            !composition.targetID.isEmpty,
            composition.evidence.count <= 10, composition.messageIDs.count <= 50,
            Set(composition.messageIDs).count == composition.messageIDs.count
        else {
            throw IMSession.SessionError.invalidInstance
        }
        try checkContext(ns, family: family)
        let metadata = try composition.evidence.map(selectionMetadata)
        let descriptor = IMJSON.object([
            "targetType": .string(composition.targetType), "targetId": .string(composition.targetID),
            "reason": .string(reason),
            "selectedMessageIds": .array(composition.messageIDs.map(IMJSON.string)),
            "evidence": .array(metadata),
        ])
        let encoder = JSONEncoder.make()
        encoder.outputFormatting = [.sortedKeys]
        let fingerprint = SHA256.hash(data: try encoder.encode(descriptor)).map {
            String(format: "%02x", $0)
        }
            .joined()
        if session.demo {
            let result = descriptor.setting("id", .string(UUID().uuidString))
                .setting("state", .string("demo")).setting("reporterId", session.user["id"])
                .setting("createdAt", .string(ISO8601DateFormatter.make().string(from: Date())))
            try session.store.save([result], collection: "reports", namespace: ns)
            return result
        }
        guard let pair = session.currentCredentialPair,
            UUID(uuidString: composition.targetID) != nil,
            composition.messageIDs.allSatisfy({
                UUID(uuidString: $0) != nil
            })
        else {
            throw IMSession.SessionError.noInstance
        }
        if let pending = try session.store.list(collection, namespace: ns).first(where: {
            $0["fingerprint"].text == fingerprint && $0["familyId"].text == pair.familyID
        }) {
            return try await resume(pending["id"].text)
        }
        let id = UUID().uuidString
        let root = try Self.directory(namespace: ns, intentID: id, create: true)
        var folder = root
        var resource = URLResourceValues()
        resource.isExcludedFromBackup = true
        try folder.setResourceValues(resource)
        var uploads: [IMJSON] = []
        var copiedFiles: [URL] = []
        do {
            for (index, selection) in composition.evidence.enumerated() {
                let item = metadata[index]
                let uploadID = UUID().uuidString
                let file = root.appendingPathComponent(uploadID + ".evidence")
                _ = try Self.directory(namespace: ns, intentID: id, create: false)
                try FileManager.default.copyItem(at: selection.url, to: file)
                copiedFiles.append(file)
                let handle = try FileHandle(forWritingTo: file)
                do {
                    try handle.synchronize()
                    try handle.close()
                } catch {
                    try? handle.close()
                    throw error
                }
                try FileManager.default.setAttributes(
                    [.protectionKey: FileProtectionType.completeUntilFirstUserAuthentication],
                    ofItemAtPath: file.path)
                guard try Self.digest(file) == item["sha256"].text,
                    try file.resourceValues(forKeys: [.fileSizeKey]).fileSize == Int(item["size"].text)
                else {
                    throw IMSession.SessionError.invalidInstance
                }
                uploads.append(
                    item.setting("requestId", .string(uploadID))
                        .setting("file", .string(file.path)).setting("uploadState", .string("prepared")))
            }
            try Self.syncDirectory(root)
            try Self.syncDirectory(root.deletingLastPathComponent())
            try Self.syncDirectory(root.deletingLastPathComponent().deletingLastPathComponent())
            try checkContext(ns, family: family)
            let intent = descriptor.setting("id", .string(id)).setting("requestId", .string(id))
                .setting("fingerprint", .string(fingerprint)).setting("uploads", .array(uploads))
                .setting("userId", .string(pair.userID)).setting("deviceId", .string(pair.deviceID))
                .setting("familyId", .string(pair.familyID)).setting(
                    "restoreEpoch", .string(pair.restoreEpoch)
                )
                .setting("state", .string("prepared"))
            try session.store.save([intent], collection: collection, namespace: ns)
        } catch {
            // 只回收本次尚未形成耐久意图、且仍位于原目录的普通文件。
            if (try? Self.directory(namespace: ns, intentID: id, create: false)) == root {
                for file in copiedFiles {
                    var info = stat()
                    if lstat(file.path, &info) == 0, info.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG) {
                        try? FileManager.default.removeItem(at: file)
                    }
                }
                _ = rmdir(root.path)
            }
            throw error
        }
        return try await resume(id)
    }

    func resume(_ id: String) async throws -> IMJSON {
        let ns = session.namespace
        guard
            let original = try session.store.list(collection, namespace: ns).first(where: {
                $0["id"].text == id
            }),
            let pair = session.currentCredentialPair, original["userId"].text == pair.userID,
            original["deviceId"].text == pair.deviceID, original["familyId"].text == pair.familyID,
            original["restoreEpoch"].text == pair.restoreEpoch
        else {
            throw IMSession.SessionError.noInstance
        }
        if original["state"].text == "completed" {
            return original["result"]
        }
        let family = pair.familyID
        var operation = original
        var uploads = operation["uploads"].list
        for index in uploads.indices {
            try checkContext(ns, family: family)
            var upload = uploads[index]
            if upload["uploadState"].text == "committed" {
                continue
            }
            let keys = [
                "requestId", "userId", "deviceId", "familyId", "restoreEpoch", "name", "size", "sha256",
            ]
            let fields = Dictionary(
                uniqueKeysWithValues: keys.map { key in
                    (
                        key,
                        (key == "requestId" || ["name", "size", "sha256"].contains(key)
                            ? upload[key] : operation[key]).text
                    )
                })
            var result: IMJSON
            if upload["uploadState"].text == "unknown" {
                let lookupBody = fields.mapValues(IMJSON.string)
                do {
                    result = try await session.action("reports/evidence/upload-results", body: lookupBody)
                } catch let error as IMAPIError where error.code == "UPLOAD_NOT_FOUND" {
                    result = .object(["uploadState": .string("missing")])
                }
            } else {
                result = .object(["uploadState": .string("missing")])
            }
            try checkContext(ns, family: family)
            if result["uploadState"].text != "committed" {
                let file = try Self.file(upload, namespace: ns, intentID: id)
                guard !upload["file"].text.isEmpty,
                    try Self.digest(file) == upload["sha256"].text,
                    try file.resourceValues(forKeys: [.fileSizeKey]).fileSize == Int(upload["size"].text)
                else {
                    throw IMSession.SessionError.invalidInstance
                }
                upload = upload.setting("uploadState", .string("unknown"))
                uploads[index] = upload
                operation = operation.setting("uploads", .array(uploads)).setting(
                    "state", .string("uploading"))
                try session.store.save([operation], collection: collection, namespace: ns)
                result = try await session.authorized { api, token in
                    return try await api.upload(
                        file, mimeType: upload["mimeType"].text, token: token,
                        path: "reports/evidence", form: fields, fileName: fields["name"])
                }
                try checkContext(ns, family: family)
            }
            guard result["protocol"].text == "stable-upload-v1", result["uploadState"].text == "committed",
                result["requestId"] == upload["requestId"], UUID(uuidString: result["id"].text) != nil,
                result["sha256"].text.lowercased() == upload["sha256"].text.lowercased(),
                result["size"].text == upload["size"].text
            else {
                throw IMSession.SessionError.invalidInstance
            }
            uploads[index] = upload.setting("uploadState", .string("committed")).setting(
                "objectId", result["id"])
            operation = operation.setting("uploads", .array(uploads))
            try session.store.save([operation], collection: collection, namespace: ns)
        }
        try checkContext(ns, family: family)
        let body: [String: IMJSON] = [
            "requestId": operation["requestId"], "targetType": operation["targetType"],
            "targetId": operation["targetId"], "reason": operation["reason"],
            "selectedMessageIds": operation["selectedMessageIds"],
            "evidenceIds": .array(uploads.map {
                $0["objectId"]
            }),
        ]
        operation = operation.setting("state", .string("unknown"))
        try session.store.save([operation], collection: collection, namespace: ns)
        let result = try await session.action("reports", body: body)
        try checkContext(ns, family: family)
        guard !result["id"].text.isEmpty, result["reporterId"].text == pair.userID,
            result["targetType"] == operation["targetType"], result["targetId"] == operation["targetId"]
        else {
            throw IMSession.SessionError.invalidInstance
        }
        try session.store.save([result], collection: "reports", namespace: ns)
        try session.store.save(
            [operation.setting("state", .string("completed")).setting("result", result)],
            collection: collection, namespace: ns)
        for upload in uploads {
            if let file = try? Self.file(upload, namespace: ns, intentID: id) {
                try? FileManager.default.removeItem(at: file)
            }
        }
        return result
    }
}

extension IMSession {
    func submitReport(_ composition: IMReportComposition) async throws -> IMJSON {
        return try await IMReportCompositionService(session: self).submit(composition)
    }

    func resumeReport(id: String) async throws -> IMJSON {
        return try await IMReportCompositionService(session: self).resume(id)
    }

    var pendingComposedReports: [IMJSON] {
        return ((try? store.list("report-composition-outbox", namespace: namespace)) ?? [])
            .filter {
                $0["state"].text != "completed"
            }
    }
}
