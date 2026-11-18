//
//  IMLocalStore.swift
//  JobsIMStorage
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import CryptoKit
import Darwin
import Foundation
import GRDB
import JobsIMCore

public final class IMLocalStore: @unchecked Sendable {

    private let database: DatabaseQueue
    private let databasePath: String
    private var databaseFileIdentity: (device: dev_t, inode: ino_t)?

    public init(path: String) throws {
        databasePath = path
        var configuration = Configuration()
        configuration.prepareDatabase { db in
            try db.execute(sql: "PRAGMA journal_mode=WAL")
            try db.execute(sql: "PRAGMA synchronous=FULL")
            try db.execute(sql: "PRAGMA foreign_keys=ON")
            try db.execute(sql: "PRAGMA secure_delete=ON")
            try db.execute(sql: "PRAGMA busy_timeout=5000")
            try db.execute(sql: "PRAGMA fullfsync=ON")
            try db.execute(sql: "PRAGMA checkpoint_fullfsync=ON")
        }
        database = try DatabaseQueue(path: path, configuration: configuration)
        var migrations = DatabaseMigrator()
        migrations.registerMigration("initial-durable-outbox") { db in
            try db.execute(
                sql:
                    "CREATE TABLE records(namespace TEXT NOT NULL, collection TEXT NOT NULL, id TEXT NOT NULL, body BLOB NOT NULL, ordering TEXT NOT NULL, PRIMARY KEY(namespace,collection,id))"
            )
            try db.execute(
                sql:
                    "CREATE TABLE outbox(namespace TEXT NOT NULL, id TEXT NOT NULL, conversation TEXT NOT NULL, body BLOB NOT NULL, status TEXT NOT NULL, PRIMARY KEY(namespace,id))"
            )
        }
        migrations.registerMigration("send-admission-freeze") { db in
            try db.execute(sql: "ALTER TABLE outbox ADD COLUMN freeze_reason TEXT NOT NULL DEFAULT ''")
            for row in try Row.fetchAll(
                db, sql: "SELECT namespace,id,conversation,body FROM outbox WHERE status='queued'")
            {
                let message = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
                guard case .string(let snapshot) = message["sendAdmission"], !snapshot.isEmpty else {
                    try Self.freeze(
                        row["id"], conversation: row["conversation"], namespace: row["namespace"],
                        reason: "missing_snapshot", db: db)
                    continue
                }
            }
        }
        migrations.registerMigration("authoritative-sync-transport-only") { db in
            try db.execute(sql: "CREATE TABLE sync_state(namespace TEXT PRIMARY KEY, body BLOB NOT NULL)")
            try db.execute(sql: "CREATE TABLE sync_metadata(id TEXT PRIMARY KEY, value TEXT NOT NULL)")
            try db.execute(sql: "INSERT INTO sync_metadata VALUES('storeGeneration',?)", arguments: [UUID().uuidString.lowercased()])
            try db.execute(sql: "CREATE TABLE sync_scopes(namespace TEXT PRIMARY KEY, profile TEXT NOT NULL, scope TEXT NOT NULL, principal TEXT NOT NULL, source TEXT NOT NULL)")
            try db.execute(sql: "CREATE TABLE sync_global(profile TEXT NOT NULL, scope TEXT NOT NULL, principal TEXT NOT NULL, source TEXT NOT NULL, id TEXT NOT NULL, PRIMARY KEY(profile,scope,principal,source,id))")
            try db.execute(sql: "CREATE TABLE sync_origins(namespace TEXT NOT NULL, id TEXT NOT NULL, fact BLOB NOT NULL, PRIMARY KEY(namespace,id))")
            try db.execute(sql: "CREATE TABLE sync_events(namespace TEXT NOT NULL, position TEXT NOT NULL, fact BLOB NOT NULL, PRIMARY KEY(namespace,position))")
            try db.execute(sql: "CREATE TABLE sync_eligibility(namespace TEXT NOT NULL, id TEXT NOT NULL, metadata BLOB NOT NULL, PRIMARY KEY(namespace,id))")
            try db.execute(sql: "CREATE TABLE sync_refs(namespace TEXT NOT NULL, id TEXT NOT NULL, body BLOB NOT NULL, PRIMARY KEY(namespace,id))")
            try db.execute(sql: "CREATE TABLE sync_pages(namespace TEXT NOT NULL, snapshot TEXT NOT NULL, page INTEGER NOT NULL, body BLOB NOT NULL, PRIMARY KEY(namespace,snapshot,page))")
        }
        migrations.registerMigration("account-erasure-denial-and-file-owners") { db in
            try db.execute(sql: """
                CREATE TABLE account_namespace_registry(namespace TEXT PRIMARY KEY,profile TEXT NOT NULL,scope TEXT NOT NULL,principal TEXT NOT NULL,source TEXT NOT NULL);
                CREATE TABLE local_account_erasures(profile TEXT NOT NULL,scope TEXT NOT NULL,principal TEXT NOT NULL,completed INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(profile,scope,principal));
                CREATE TABLE local_account_erasure_namespaces(profile TEXT NOT NULL,scope TEXT NOT NULL,principal TEXT NOT NULL,namespace TEXT NOT NULL,PRIMARY KEY(profile,scope,principal,namespace));
                CREATE TABLE local_account_private_files(profile TEXT NOT NULL,scope TEXT NOT NULL,principal TEXT NOT NULL,namespace TEXT NOT NULL,kind TEXT NOT NULL,name TEXT NOT NULL,PRIMARY KEY(profile,scope,principal,kind,name));
                """)
        }
        migrations.registerMigration("receipt-global-file-cleanup") { db in
            try db.execute(sql: "CREATE TABLE receipt_file_cleanup(namespace TEXT NOT NULL,request_id TEXT NOT NULL,message_id TEXT NOT NULL,device INTEGER NOT NULL,inode INTEGER NOT NULL,PRIMARY KEY(namespace,request_id))")
        }
        try migrations.migrate(database)
        var identity = stat()
        if path != ":memory:", lstat(path, &identity) == 0, identity.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG) {
            databaseFileIdentity = (identity.st_dev, identity.st_ino)
        }
    }

    public func list(_ collection: String, namespace: IMNamespace) throws -> [IMJSON] {
        try database.read { db in
            try Self.checkDeletionFloor(namespace, db: db)
            return try Row.fetchAll(
                db, sql: "SELECT body FROM records WHERE namespace=? AND collection=? ORDER BY ordering,id",
                arguments: [namespace.key, collection]
            ).map {
                try JSONDecoder.make().decode(IMJSON.self, from: $0["body"] as Data)
            }
        }
    }

    public func save(_ values: [IMJSON], collection: String, namespace: IMNamespace) throws {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            for value in values {
                let id = value.firstText("id", "clientMsgId")
                guard !id.isEmpty else {
                    continue
                }
                try Self.save(value, id: id, collection: collection, namespace: namespace.key, db: db)
            }
        }
    }

    public func enqueue(_ message: IMJSON, conversation: String, namespace: IMNamespace) throws {
        let id = message["clientMsgId"].text
        guard !id.isEmpty else {
            throw StoreError.invalidIdentity
        }
        let body = try JSONEncoder.make().encode(message)
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            if let existing = try Data.fetchOne(
                db, sql: "SELECT body FROM outbox WHERE namespace=? AND id=?", arguments: [namespace.key, id])
            {
                let original = try JSONDecoder.make().decode(IMJSON.self, from: existing)
                guard original == message else {
                    throw StoreError.conflictingIdentity
                }
                return
            }
            try db.execute(
                sql:
                    "INSERT INTO outbox(namespace,id,conversation,body,status) VALUES(?,?,?,?,?) ON CONFLICT(namespace,id) DO NOTHING",
                arguments: [namespace.key, id, conversation, body, "queued"])
            try Self.save(
                message, id: id, collection: "messages:" + conversation, namespace: namespace.key, db: db)
            if !namespace.demo {
                guard case .string(let snapshot) = message["sendAdmission"], !snapshot.isEmpty else {
                    try Self.freeze(
                        id, conversation: conversation, namespace: namespace.key, reason: "missing_snapshot",
                        db: db)
                    return
                }
            }
        }
    }

    public func pending(namespace: IMNamespace) throws -> [IMJSON] {
        try database.read { db in
            try Self.checkDeletionFloor(namespace, db: db)
            return try Row.fetchAll(
                db, sql: "SELECT body FROM outbox WHERE namespace=? AND status='queued' ORDER BY rowid",
                arguments: [namespace.key]
            ).map {
                try JSONDecoder.make().decode(IMJSON.self, from: $0["body"] as Data)
            }
        }
    }

    public func allOutbox(namespace: IMNamespace) throws -> [IMJSON] {
        return try database.read { db in
            try Self.checkDeletionFloor(namespace, db: db)
            return try Row.fetchAll(db, sql: "SELECT body FROM outbox WHERE namespace=? ORDER BY rowid", arguments: [namespace.key]).map { row in
                return try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
            }
        }
    }

    public func originalOutbox(clientID: String, namespace: IMNamespace) throws -> IMJSON? {
        return try database.read { db in
            try Self.checkDeletionFloor(namespace, db: db)
            guard
                let data = try Data.fetchOne(
                    db, sql: "SELECT body FROM outbox WHERE namespace=? AND id=?",
                    arguments: [namespace.key, clientID])
            else {
                return nil
            }
            return try JSONDecoder.make().decode(IMJSON.self, from: data)
        }
    }

    public func freezeOutbox(clientID: String, conversation: String, namespace: IMNamespace, reason: String)
        throws
    {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.freeze(
                clientID, conversation: conversation, namespace: namespace.key, reason: reason, db: db)
        }
    }

    private static func freeze(
        _ id: String, conversation: String, namespace: String, reason: String, db: Database
    ) throws {
        try db.execute(
            sql: "UPDATE outbox SET status='frozen',freeze_reason=? WHERE namespace=? AND id=?",
            arguments: [reason, namespace, id])
        guard db.changesCount > 0,
            let data = try Data.fetchOne(
                db, sql: "SELECT body FROM records WHERE namespace=? AND collection=? AND id=?",
                arguments: [namespace, "messages:" + conversation, id])
        else {
            return
        }
        let value = try JSONDecoder.make().decode(IMJSON.self, from: data)
            .setting("status", .string("frozen")).setting("freezeReason", .string(reason))
        try Self.save(value, id: id, collection: "messages:" + conversation, namespace: namespace, db: db)
    }

    public func acknowledge(clientID: String, message: IMJSON, conversation: String, namespace: IMNamespace)
        throws
    {
        guard !message["id"].text.isEmpty, message["clientMsgId"].text == clientID else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try db.execute(
                sql: "DELETE FROM outbox WHERE namespace=? AND id=?", arguments: [namespace.key, clientID])
            if message["id"].text != clientID {
                try db.execute(
                    sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                    arguments: [namespace.key, "messages:" + conversation, clientID])
            }
            try Self.save(
                message, id: message.firstText("id", "clientMsgId"), collection: "messages:" + conversation,
                namespace: namespace.key, db: db)
        }
    }

    public func replace(_ values: [IMJSON], collection: String, namespace: IMNamespace) throws {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try db.execute(
                sql: "DELETE FROM records WHERE namespace=? AND collection=?",
                arguments: [namespace.key, collection])
            for value in values {
                let id = value.firstText("id", "clientMsgId")
                guard !id.isEmpty else {
                    throw StoreError.invalidIdentity
                }
                try Self.save(value, id: id, collection: collection, namespace: namespace.key, db: db)
            }
        }
    }

    public func commitAuthenticationBinding(
        previousNamespaces: [IMNamespace], newNamespace: IMNamespace,
        metadata: [IMJSON], metadataNamespace: IMNamespace
    ) throws {
        try database.write { db in
            try Self.registerDeletionNamespace(newNamespace, db: db)
            try Self.indexSyncNamespace(newNamespace, fallback: newNamespace, db: db)
            for previous in Set(previousNamespaces) where previous != newNamespace {
                try Self.indexSyncNamespace(previous, fallback: newNamespace, db: db)
                try Self.restrictSync(previous.key, reason: "authentication_device_changed", resetSnapshot: false, db: db)
                let rows = try Row.fetchAll(
                    db,
                    sql: "SELECT id,conversation FROM outbox WHERE namespace=? AND status='queued'",
                    arguments: [previous.key])
                for row in rows {
                    try Self.freeze(
                        row["id"], conversation: row["conversation"], namespace: previous.key,
                        reason: "authentication_device_changed", db: db)
                }
            }
            for value in metadata {
                let id = value["id"].text
                guard !id.isEmpty else {
                    throw StoreError.invalidIdentity
                }
                try Self.save(
                    value, id: id, collection: "auth-state", namespace: metadataNamespace.key, db: db)
            }
        }
    }

    public func redactForward(messageID: String, conversation: String, namespace: IMNamespace) throws {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            let data = try Data.fetchOne(
                db,
                sql: "SELECT body FROM records WHERE namespace=? AND collection=? AND id=?",
                arguments: [namespace.key, "messages:" + conversation, messageID])
            let value =
                try data.map { try JSONDecoder.make().decode(IMJSON.self, from: $0) }
                ?? .object([
                    "id": .string(messageID), "conversationId": .string(conversation),
                    "forwarded": .bool(true),
                ])
            guard value["forwarded"] == .bool(true), case .object(var fields) = value else {
                return
            }
            for key in ["text", "attachmentId", "attachmentURL", "forwardAdmission", "replyToId"] {
                fields.removeValue(forKey: key)
            }
            fields["contentUnavailable"] = .bool(true)
            fields["forwardProjectionUnknown"] = .bool(true)
            let generation = UInt64(value["forwardProjectionGeneration"].text) ?? 0
            guard generation < UInt64.max else {
                throw StoreError.invalidIdentity
            }
            fields["forwardProjectionGeneration"] = .string(String(generation + 1))
            try Self.save(
                .object(fields), id: messageID, collection: "messages:" + conversation,
                namespace: namespace.key, db: db, verifiedForwardProjection: true,
                observeForwardUnavailable: false)
        }
    }

    public func forwardProjectionGeneration(messageID: String, conversation: String, namespace: IMNamespace)
        throws -> String
    {
        let value =
            try list("messages:" + conversation, namespace: namespace)
            .first(where: { $0["id"].text == messageID })?["forwardProjectionGeneration"].text ?? ""
        return value.isEmpty ? "0" : value
    }

    public func applyForwardProjection(
        _ message: IMJSON, conversation: String, namespace: IMNamespace,
        expectedGeneration: String
    ) throws {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            let id = message["id"].text
            guard !id.isEmpty else {
                throw StoreError.invalidIdentity
            }
            let data = try Data.fetchOne(
                db, sql: "SELECT body FROM records WHERE namespace=? AND collection=? AND id=?",
                arguments: [namespace.key, "messages:" + conversation, id])
            let old = try data.map { try JSONDecoder.make().decode(IMJSON.self, from: $0) }
            let saved = old?["forwardProjectionGeneration"].text ?? ""
            let generation = saved.isEmpty ? "0" : saved
            guard generation == expectedGeneration else {
                return
            }
            let verified = message.setting("forwardProjectionGeneration", .string(generation))
                .setting("forwardProjectionUnknown", .bool(false))
            try Self.save(
                verified, id: id, collection: "messages:" + conversation, namespace: namespace.key,
                db: db, verifiedForwardProjection: true)
        }
    }

    @discardableResult
    public func applyMessagePage(
        _ values: [IMJSON], conversation: String, cursor: String, namespace: IMNamespace,
        readerID: String? = nil, isVisible: Bool = false
    ) throws -> [IMJSON] {
        return try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            guard let next = IMCursor(raw: cursor, expectedEpoch: namespace.restoreEpoch) else {
                throw StoreError.invalidIdentity
            }
            if let data = try Data.fetchOne(
                db, sql: "SELECT body FROM records WHERE namespace=? AND collection='cursors' AND id=?",
                arguments: [namespace.key, conversation])
            {
                let previous = try JSONDecoder.make().decode(IMJSON.self, from: data)
                guard let old = IMCursor(raw: previous["cursor"].text, expectedEpoch: namespace.restoreEpoch)
                else {
                    throw StoreError.invalidIdentity
                }
                if !next.isAtLeast(old) {
                    return []
                }
            }
            var incoming: [IMJSON] = []
            for value in values {
                let id = value["id"].text
                guard !id.isEmpty else {
                    throw StoreError.invalidIdentity
                }
                let exists =
                    try Bool.fetchOne(
                        db,
                        sql:
                            "SELECT EXISTS(SELECT 1 FROM records WHERE namespace=? AND collection=? AND id=?)",
                        arguments: [namespace.key, "messages:" + conversation, id]) ?? false
                try Self.save(
                    value, id: id, collection: "messages:" + conversation, namespace: namespace.key, db: db)
                if let readerID, !exists, value["senderId"].text != readerID, value["deleted"] != .bool(true)
                {
                    incoming.append(value)
                    if !isVisible {
                        try Self.save(
                            value, id: id, collection: "unread:" + conversation, namespace: namespace.key,
                            db: db)
                    }
                }
                if value["deleted"] == .bool(true) {
                    try db.execute(
                        sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                        arguments: [namespace.key, "unread:" + conversation, id])
                }
            }
            if isVisible {
                try db.execute(
                    sql: "DELETE FROM records WHERE namespace=? AND collection=?",
                    arguments: [namespace.key, "unread:" + conversation])
            }
            try Self.save(
                .object(["id": .string(conversation), "cursor": .string(cursor)]), id: conversation,
                collection: "cursors", namespace: namespace.key, db: db)
            return incoming
        }
    }

    public func reject(clientID: String, conversation: String, namespace: IMNamespace) throws {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try db.execute(
                sql: "UPDATE outbox SET status='rejected' WHERE namespace=? AND id=?",
                arguments: [namespace.key, clientID])
            if let data = try Data.fetchOne(
                db, sql: "SELECT body FROM records WHERE namespace=? AND collection=? AND id=?",
                arguments: [namespace.key, "messages:" + conversation, clientID])
            {
                let value = try JSONDecoder.make().decode(IMJSON.self, from: data).setting(
                    "status", .string("rejected"))
                try Self.save(
                    value, id: clientID, collection: "messages:" + conversation, namespace: namespace.key,
                    db: db)
            }
        }
    }

    public func remove(_ id: String, collection: String, namespace: IMNamespace) throws {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try db.execute(
                sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                arguments: [namespace.key, collection, id])
        }
    }

    /// 导出原意图只以完整原record作CAS，不覆盖并发取消nonce或既成job绑定。
    public func comparePasswordChangeIntent(namespace: IMNamespace, previous: IMJSON?, next: IMJSON) throws {
        let fields: Set<String> = ["id", "username", "profile", "deploymentId", "userId", "deviceId", "familyId", "restoreEpoch", "createdAt", "state"]
        guard case .object(let values) = next, Set(values.keys) == fields,
            namespace.account == "__password_changes__", !namespace.demo else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            let bytes = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='password-change-operations' AND id=?",
                                         arguments: [namespace.key, next["id"].text])
            let current = try bytes.map { try JSONDecoder.make().decode(IMJSON.self, from: $0) }
            guard current == previous else {
                throw StoreError.conflictingIdentity
            }
            if let previous {
                for key in fields where key != "state" {
                    guard previous[key] == next[key] else {
                        throw StoreError.conflictingIdentity
                    }
                }
            } else {
                let rows = try Data.fetchAll(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='password-change-operations'",
                                             arguments: [namespace.key])
                for bytes in rows {
                    let row = try JSONDecoder.make().decode(IMJSON.self, from: bytes)
                    guard row["familyId"] != next["familyId"] || !["unsent", "unknown", "pending"].contains(row["state"].text) else {
                        throw StoreError.conflictingIdentity
                    }
                }
            }
            try Self.save(next, id: next["id"].text, collection: "password-change-operations", namespace: namespace.key, db: db)
        }
    }

    public func compareDataExportIntent(namespace: IMNamespace, previous: IMJSON?, next: IMJSON) throws {
        guard !namespace.demo, namespace.sourceContextID == "personal", !namespace.deviceID.isEmpty,
            !next["id"].text.isEmpty
        else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            let bytes = try Data.fetchOne(db,
                sql: "SELECT body FROM records WHERE namespace=? AND collection='data-export-operations' AND id=?",
                arguments: [namespace.key, next["id"].text])
            let current = try bytes.map { try JSONDecoder.make().decode(IMJSON.self, from: $0) }
            guard current == previous else {
                throw StoreError.conflictingIdentity
            }
            if let previous {
                for key in ["id", "familyId", "originalBody", "bodySHA256", "createdAt"] {
                    guard previous[key] == next[key] else {
                        throw StoreError.conflictingIdentity
                    }
                }
                for key in ["jobId", "cancelBody", "cancelSHA256"] where previous[key] != .null {
                    guard previous[key] == next[key] else {
                        throw StoreError.conflictingIdentity
                    }
                }
            } else {
                let originals = try Data.fetchAll(db,
                    sql: "SELECT body FROM records WHERE namespace=? AND collection='data-export-operations'",
                    arguments: [namespace.key])
                guard originals.count < 10000 else {
                    throw StoreError.conflictingIdentity
                }
                for original in originals {
                    let value = try JSONDecoder.make().decode(IMJSON.self, from: original)
                    guard value["jobId"] != .null else {
                        throw StoreError.conflictingIdentity
                    }
                }
            }
            try Self.save(next, id: next["id"].text, collection: "data-export-operations", namespace: namespace.key, db: db)
        }
    }

    /// 注册原意图先进入FULL事务，重启后沿同一nonce和物理数据库代际续跑。
    public func syncState(namespace: IMNamespace) throws -> IMJSON {
        guard !namespace.demo, !namespace.deploymentID.isEmpty, !namespace.deviceID.isEmpty,
            namespace.sourceContextID == "personal"
        else {
            throw StoreError.invalidIdentity
        }
        return try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            if let state = try Self.syncRead(namespace.key, db: db) {
                return state
            }
            try Self.indexSyncNamespace(namespace, fallback: namespace, db: db)
            let generation = try String.fetchOne(
                db, sql: "SELECT value FROM sync_metadata WHERE id='storeGeneration'")
            guard let generation else {
                throw StoreError.invalidIdentity
            }
            let value: IMJSON = .object([
                "scopeId": .string(namespace.deploymentID),
                "principalId": .string(namespace.account),
                "sourceContextId": .string(namespace.sourceContextID),
                "deviceId": .string(namespace.deviceID),
                "restoreEpoch": .string(namespace.restoreEpoch),
                "storeGeneration": .string(generation),
                "registrationRequestId": .string(UUID().uuidString.lowercased()),
                "guardGeneration": .string("0"), "position": .string("0"),
                "restriction": .string("SYNC_BASELINE_REQUIRED"),
            ])
            try Self.retainSyncBodies(namespace.key, db: db)
            try Self.syncWrite(value, namespace: namespace.key, db: db)
            return value
        }
    }

    public func syncStateIfPresent(namespace: IMNamespace) throws -> IMJSON? {
        return try database.read { db in
            try Self.checkDeletionFloor(namespace, db: db)
            return try Self.syncRead(namespace.key, db: db)
        }
    }

    public func syncSelectSchema(namespace: IMNamespace, expected: IMJSON, schema: String) throws -> IMJSON {
        guard ["1", "2"].contains(schema) else {
            throw StoreError.invalidIdentity
        }
        return try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.syncCompare(expected, namespace: namespace.key, db: db)
            let previous = expected["schemaVersion"].text.isEmpty ? "1" : expected["schemaVersion"].text
            if previous == schema {
                return expected
            }
            guard previous == "1", schema == "2" else {
                throw StoreError.invalidIdentity
            }
            try Self.save(expected.setting("id", expected["registrationRequestId"]),
                          id: expected["registrationRequestId"].text,
                          collection: "sync-schema-archives", namespace: namespace.key, db: db)
            try Self.retainSyncBodies(namespace.key, db: db)
            let archive = namespace.key + ":sync-schema1:" + expected["registrationRequestId"].text
            for table in ["sync_origins", "sync_events", "sync_refs", "sync_pages"] {
                try db.execute(sql: "UPDATE " + table + " SET namespace=? WHERE namespace=?",
                               arguments: [archive, namespace.key])
            }
            var next: IMJSON = .object([:])
            for field in ["scopeId", "principalId", "sourceContextId", "deviceId", "restoreEpoch", "storeGeneration"] {
                next = next.setting(field, expected[field])
            }
            next = next.setting("schemaVersion", .string("2"))
                .setting("previousSchemaArchive", .string(archive))
                .setting("registrationRequestId", .string(UUID().uuidString.lowercased()))
                .setting("position", .string("0"))
                .setting("restriction", .string("SYNC_BASELINE_REQUIRED"))
            guard let generation = UInt64(expected["guardGeneration"].text),
                generation < UInt64(Int64.max)
            else {
                throw StoreError.invalidIdentity
            }
            if expected["classificationWatermark"] != .null {
                next = next.setting("classificationWatermark", expected["classificationWatermark"])
            }
            // 新stream可重置位置和登记nonce，普通syncAdvance仍禁止这两项回退。
            let advanced = next.setting("guardGeneration", .string(String(generation + 1)))
            try Self.syncWrite(advanced, namespace: namespace.key, db: db)
            return advanced
        }
    }

    public func completeSelfStateOperation(namespace: IMNamespace, expectedGuard: IMJSON,
                                           original: IMJSON, result: IMJSON) throws {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.syncCompare(expectedGuard, namespace: namespace.key, db: db)
            guard let raw = try Data.fetchOne(db,
                                             sql: "SELECT body FROM records WHERE namespace=? AND collection='self-state-operations' AND id=?",
                                             arguments: [namespace.key, original["id"].text]),
                try JSONDecoder.make().decode(IMJSON.self, from: raw) == original,
                original["body"]["revisionEpoch"].text == namespace.restoreEpoch,
                result["result"]["revisionEpoch"].text == namespace.restoreEpoch
            else {
                throw StoreError.invalidIdentity
            }
            guard ["applied", "retired"].contains(result["status"].text) else {
                throw StoreError.invalidIdentity
            }
            let completed = original.setting("state", result["status"]).setting("result", result)
            try Self.save(completed, id: original["id"].text, collection: "self-state-operations",
                          namespace: namespace.key, db: db)
            if original["method"] == .string("PUT"), original["path"].text.hasSuffix("/read"),
                result["current"] == .bool(true) {
                let conversation = original["conversationId"].text
                guard !conversation.isEmpty,
                    result["result"]["membershipGeneration"] == original["body"]["membershipGeneration"],
                    Set(result["result"]["observedMessageIds"].list.map { $0.text }) == Set(original["body"]["observedMessageIds"].list.map { $0.text })
                else {
                    throw StoreError.invalidIdentity
                }
                for id in original["body"]["observedMessageIds"].list {
                    try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                                   arguments: [namespace.key, "unread:" + conversation, id.text])
                }
            }
        }
    }

    public func transitionSelfStateOperation(namespace: IMNamespace, expectedGuard: IMJSON,
                                             original: IMJSON, state: String, reason: String) throws {
        guard ["rejected", "archived"].contains(state) else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.syncCompare(expectedGuard, namespace: namespace.key, db: db)
            guard let raw = try Data.fetchOne(db,
                                             sql: "SELECT body FROM records WHERE namespace=? AND collection='self-state-operations' AND id=?",
                                             arguments: [namespace.key, original["id"].text]),
                try JSONDecoder.make().decode(IMJSON.self, from: raw) == original,
                original["body"]["revisionEpoch"].text == namespace.restoreEpoch,
                state != "rejected" || original["state"] == .string("pending"),
                state != "archived" || original["state"] == .string("rejected")
            else {
                throw StoreError.invalidIdentity
            }
            try Self.save(original.setting("state", .string(state)).setting("reason", .string(reason)),
                          id: original["id"].text, collection: "self-state-operations", namespace: namespace.key, db: db)
        }
    }

    @discardableResult
    public func syncUpdate(
        namespace: IMNamespace, expected: IMJSON, next: IMJSON, page: IMJSON? = nil
    ) throws -> IMJSON {
        return try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.syncCompare(expected, namespace: namespace.key, db: db)
            let value = try Self.syncAdvance(next, previous: expected)
            if let page {
                // 此表只允许原refs与摘要；云端正文不能借阶段表提前落盘。
                guard page["items"] == .null, page["object"] == .null,
                    page["bodyEnvelope"] == .null,
                    !page["snapshotId"].text.isEmpty, case .number = page["page"]
                else {
                    throw StoreError.invalidIdentity
                }
                try db.execute(
                    sql:
                        "INSERT INTO sync_pages VALUES(?,?,?,?) ON CONFLICT(namespace,snapshot,page) DO UPDATE SET body=excluded.body",
                    arguments: [
                        namespace.key, page["snapshotId"].text, Int(page["page"].text),
                        try JSONEncoder.make().encode(page),
                    ])
            }
            try Self.syncWrite(value, namespace: namespace.key, db: db)
            return value
        }
    }

    public func syncRestrict(namespace: IMNamespace, reason: String, resetSnapshot: Bool = false)
        throws
    {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.restrictSync(
                namespace.key, reason: reason, resetSnapshot: resetSnapshot, db: db)
        }
    }

    public func syncNamespaces(namespace: IMNamespace) throws -> [String] {
        return try database.read { db in
            try Self.checkDeletionFloor(namespace, db: db)
            return try String.fetchAll(
                db,
                sql:
                    "SELECT namespace FROM sync_scopes WHERE profile=? AND scope=? AND principal=? AND source=?",
                arguments: [
                    namespace.profile, namespace.deploymentID, namespace.account,
                    namespace.sourceContextID,
                ])
        }
    }

    public func syncGlobalIDs(namespace: IMNamespace) throws -> [String] {
        return try database.read { db in
            try Self.checkDeletionFloor(namespace, db: db)
            return try String.fetchAll(
                db,
                sql:
                    "SELECT id FROM sync_global WHERE profile=? AND scope=? AND principal=? AND source=?",
                arguments: [
                    namespace.profile, namespace.deploymentID, namespace.account,
                    namespace.sourceContextID,
                ])
        }
    }

    public func retainedMessages(conversation: String, namespace: IMNamespace) throws -> [IMJSON] {
        return try database.read { db in
            try Self.checkDeletionFloor(namespace, db: db)
            let rows = try Row.fetchAll(
                db,
                sql:
                    "SELECT r.body,e.metadata FROM records r JOIN sync_eligibility e ON e.namespace=r.namespace AND e.id=r.id WHERE r.namespace=? AND r.collection=? ORDER BY r.ordering,r.id",
                arguments: [namespace.key, "retained:messages:" + conversation])
            return try rows.compactMap { row in
                let value = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
                let eligibility = try JSONDecoder.make().decode(
                    IMJSON.self, from: row["metadata"] as Data)
                guard eligibility["restoreEpoch"].text == namespace.restoreEpoch,
                    value["version"] == eligibility["version"],
                    try Self.syncLocalDigest(value) == eligibility["bodyDigest"].text,
                    value["deleted"] != .bool(true), value["forwarded"] != .bool(true),
                    value["contentUnavailable"] != .bool(true)
                else {
                    return nil
                }
                return value
            }
        }
    }

    public func retainedConversations(namespace: IMNamespace) throws -> [IMJSON] {
        let values = try list("retained:conversations", namespace: namespace)
        return try values.filter { value in
            return try !retainedMessages(conversation: value["id"].text, namespace: namespace)
                .isEmpty
        }
    }

    /// body、origin去重、全局删除、原Outbox证明和连续游标在同一次数据库提交中生效。
    public func syncCommit(
        namespace: IMNamespace, expected: IMJSON, next: IMJSON,
        projections: [IMJSON], events: [IMJSON], proofs: [IMJSON], replaceCloud: Bool,
        visibleConversation: String? = nil, verifiedPages: [IMJSON] = []
    ) throws {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.syncCompare(expected, namespace: namespace.key, db: db)
            // 安装期只在本事务内解除限制；其他连接看不到正文先于游标的中间状态。
            try Self.syncWrite(
                next.setting("restriction", .string("")), namespace: namespace.key, db: db)
            if replaceCloud {
                let manifest = expected["manifest"]
                let pages = try Row.fetchAll(
                    db,
                    sql:
                        "SELECT body FROM sync_pages WHERE namespace=? AND snapshot=? ORDER BY page",
                    arguments: [namespace.key, manifest["snapshotId"].text])
                guard pages.count == manifest["pageManifest"].list.count,
                    pages.count == verifiedPages.count, !pages.isEmpty
                else {
                    throw StoreError.invalidIdentity
                }
                for (index, row) in pages.enumerated() {
                    let page = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
                    guard page["page"] == .number(Decimal(index)),
                        page["pageDigest"] == manifest["pageManifest"].list[index]["digest"],
                        page == verifiedPages[index]
                    else {
                        throw StoreError.invalidIdentity
                    }
                }
                try Self.retainSyncBodies(namespace.key, db: db)
                try db.execute(
                    sql: "DELETE FROM sync_refs WHERE namespace=?", arguments: [namespace.key])
            }
            for event in events {
                let position =
                    event["kind"] == .string("reference") ? event["position"] : event["from"]
                let metadata =
                    event["kind"] == .string("reference")
                    ? IMJSON.object([
                        "kind": event["kind"], "position": position,
                        "originEventId": event["originEventId"],
                        "eventDigest": event["eventDigest"],
                        "eventEnvelope": event["eventEnvelope"], "ref": event["ref"],
                    ])
                    : event
                if let existing = try Data.fetchOne(
                    db, sql: "SELECT fact FROM sync_events WHERE namespace=? AND position=?",
                    arguments: [namespace.key, position.text])
                {
                    guard try JSONDecoder.make().decode(IMJSON.self, from: existing) == metadata
                    else {
                        throw StoreError.conflictingIdentity
                    }
                } else {
                    try db.execute(
                        sql: "INSERT INTO sync_events VALUES(?,?,?)",
                        arguments: [
                            namespace.key, position.text, try JSONEncoder.make().encode(metadata),
                        ])
                }
            }
            for event in events where event["kind"] == .string("reference") {
                let id = event["originEventId"].text
                let fact: IMJSON = .object([
                    "position": event["position"], "originEventId": event["originEventId"],
                    "eventDigest": event["eventDigest"], "eventEnvelope": event["eventEnvelope"],
                    "ref": event["ref"],
                ])
                if let bytes = try Data.fetchOne(
                    db,
                    sql: "SELECT fact FROM sync_origins WHERE namespace=? AND id=?",
                    arguments: [namespace.key, id])
                {
                    guard try JSONDecoder.make().decode(IMJSON.self, from: bytes) == fact else {
                        throw StoreError.conflictingIdentity
                    }
                } else {
                    try db.execute(
                        sql: "INSERT INTO sync_origins VALUES(?,?,?)",
                        arguments: [namespace.key, id, try JSONEncoder.make().encode(fact)])
                }
            }
            for item in projections {
                let message = item["object"]
                if !replaceCloud, item["ref"]["kind"] == .string("message"),
                    item["state"] == .string("available"),
                    message["senderId"].text != namespace.account,
                    message["conversationId"].text != visibleConversation,
                    try Bool.fetchOne(
                        db,
                        sql:
                            "SELECT EXISTS(SELECT 1 FROM records WHERE namespace=? AND collection=? AND id=?)",
                        arguments: [
                            namespace.key, "messages:" + message["conversationId"].text,
                            message["id"].text,
                        ]) == false
                {
                    try Self.save(
                        .object(["id": message["id"], "conversationId": message["conversationId"]]),
                        id: message["id"].text,
                        collection: "unread:" + message["conversationId"].text,
                        namespace: namespace.key, db: db)
                }
                try Self.syncApply(item, namespace: namespace, db: db)
            }
            if next["schemaVersion"] == .string("2") {
                let states = try Row.fetchAll(db,
                                             sql: "SELECT collection,body FROM records WHERE namespace=? AND collection IN('self:read_state','self:read_observation')",
                                             arguments: [namespace.key])
                for state in states {
                    let value = try JSONDecoder.make().decode(IMJSON.self, from: state["body"] as Data)
                    let collection: String = state["collection"]
                    try Self.applyPersonalRead(value, kind: String(collection.dropFirst(5)),
                                               namespace: namespace, db: db)
                }
            }
            for proof in proofs {
                try Self.syncAcknowledge(proof, namespace: namespace, db: db)
            }
            // 失权会话的原待发不可因之后重新加入而复活；原body保持不变。
            for row in try Row.fetchAll(
                db, sql: "SELECT id,conversation FROM outbox WHERE namespace=? AND status='queued'",
                arguments: [namespace.key])
            {
                let exists =
                    try Bool.fetchOne(
                        db,
                        sql:
                            "SELECT EXISTS(SELECT 1 FROM records WHERE namespace=? AND collection='conversations' AND id=?)",
                        arguments: [namespace.key, row["conversation"] as String]) ?? false
                if !exists {
                    try Self.freeze(
                        row["id"], conversation: row["conversation"], namespace: namespace.key,
                        reason: "sync_scope_unavailable", db: db)
                }
            }
            let value = try Self.syncAdvance(next, previous: expected)
                .setting("restriction", .string(""))
            try Self.syncWrite(value, namespace: namespace.key, db: db)
            try db.execute(
                sql: "DELETE FROM sync_pages WHERE namespace=?", arguments: [namespace.key])
        }
        try drainReceiptFileCleanup(namespace: namespace)
    }

    public func acknowledgeOriginal(_ original: IMJSON, message: IMJSON, namespace: IMNamespace)
        throws
    {
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.syncAcknowledge(
                .object(["original": original, "message": message]), namespace: namespace, db: db)
        }
        try drainReceiptFileCleanup(namespace: namespace)
    }

    private static func syncAcknowledge(_ proof: IMJSON, namespace: IMNamespace, db: Database)
        throws
    {
        let original = proof["original"]
        let message = proof["message"]
        let id = original["clientMsgId"].text
        let conversation = original["conversationId"].text
        guard !id.isEmpty, !conversation.isEmpty, !message["id"].text.isEmpty,
            message["clientMsgId"].text == id, message["conversationId"].text == conversation,
            message["senderId"] == original["senderId"],
            message["senderId"].text == namespace.account
        else {
            throw StoreError.invalidIdentity
        }
        guard
            let bytes = try Data.fetchOne(
                db,
                sql: "SELECT body FROM outbox WHERE namespace=? AND id=? AND conversation=?",
                arguments: [namespace.key, id, conversation])
        else {
            return
        }
        guard try JSONDecoder.make().decode(IMJSON.self, from: bytes) == original else {
            throw StoreError.conflictingIdentity
        }
        try db.execute(
            sql: "DELETE FROM outbox WHERE namespace=? AND id=?", arguments: [namespace.key, id])
        try db.execute(
            sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
            arguments: [namespace.key, "messages:" + conversation, id])
        if message["deleted"] == .bool(true) {
            try syncGlobal(message["id"].text, value: message, namespace: namespace, db: db)
        }
        try Self.save(
            message, id: message["id"].text, collection: "messages:" + conversation,
            namespace: namespace.key, db: db)
    }

    private static func indexSyncNamespace(
        _ namespace: IMNamespace, fallback: IMNamespace, db: Database
    ) throws {
        guard !namespace.demo else {
            return
        }
        let scope = namespace.deploymentID.isEmpty ? fallback.deploymentID : namespace.deploymentID
        let source =
            namespace.sourceContextID.isEmpty ? fallback.sourceContextID : namespace.sourceContextID
        guard !scope.isEmpty, !source.isEmpty else {
            return
        }
        try db.execute(
            sql: "INSERT INTO sync_scopes VALUES(?,?,?,?,?) ON CONFLICT(namespace) DO NOTHING",
            arguments: [namespace.key, namespace.profile, scope, namespace.account, source])
    }

    private static func syncRead(_ namespace: String, db: Database) throws -> IMJSON? {
        guard
            let bytes = try Data.fetchOne(
                db, sql: "SELECT body FROM sync_state WHERE namespace=?", arguments: [namespace])
        else {
            return nil
        }
        return try JSONDecoder.make().decode(IMJSON.self, from: bytes)
    }

    private static func syncWrite(_ value: IMJSON, namespace: String, db: Database) throws {
        try db.execute(
            sql:
                "INSERT INTO sync_state VALUES(?,?) ON CONFLICT(namespace) DO UPDATE SET body=excluded.body",
            arguments: [namespace, try JSONEncoder.make().encode(value)])
    }

    private static func syncCompare(_ expected: IMJSON, namespace: String, db: Database) throws {
        guard try syncRead(namespace, db: db) == expected else {
            throw StoreError.conflictingIdentity
        }
    }

    private static func syncAdvance(_ next: IMJSON, previous: IMJSON) throws -> IMJSON {
        guard let generation = UInt64(previous["guardGeneration"].text),
            generation < UInt64(Int64.max)
        else {
            throw StoreError.invalidIdentity
        }
        for key in [
            "scopeId", "principalId", "sourceContextId", "deviceId", "restoreEpoch",
            "storeGeneration", "registrationRequestId",
        ] {
            guard next[key] == previous[key] else {
                throw StoreError.invalidIdentity
            }
        }
        for key in ["position", "classificationWatermark"] where previous[key] != .null {
            guard let before = UInt64(previous[key].text), let after = UInt64(next[key].text),
                after >= before
            else {
                throw StoreError.invalidIdentity
            }
        }
        return next.setting("guardGeneration", .string(String(generation + 1)))
    }

    private static func restrictSync(
        _ namespace: String, reason: String, resetSnapshot: Bool, db: Database
    ) throws {
        guard let previous = try syncRead(namespace, db: db) else {
            return
        }
        var next = previous.setting("restriction", .string(reason))
        if resetSnapshot {
            next = next.setting("snapshotIntent", .null).setting("manifest", .null)
            try db.execute(sql: "DELETE FROM sync_pages WHERE namespace=?", arguments: [namespace])
        }
        try retainSyncBodies(namespace, db: db)
        try syncWrite(syncAdvance(next, previous: previous), namespace: namespace, db: db)
    }

    private static func retainSyncBodies(_ namespace: String, db: Database) throws {
        for row in try Row.fetchAll(
            db, sql: "SELECT id,body FROM records WHERE namespace=? AND collection='conversations'",
            arguments: [namespace])
        {
            let value = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
            let id: String = row["id"]
            try save(
                .object([
                    "id": .string(id), "title": value["title"], "kind": value["kind"],
                    "readOnly": .bool(true), "localHistory": .bool(true),
                ]),
                id: id, collection: "retained:conversations", namespace: namespace, db: db)
        }
        let rows = try Row.fetchAll(
            db,
            sql:
                "SELECT collection,id,body FROM records WHERE namespace=? AND (collection LIKE 'messages:%' OR collection IN('saved','search'))",
            arguments: [namespace])
        for row in rows {
            let value = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
            guard value["status"].text.isEmpty else {
                continue
            }
            let collection: String = row["collection"]
            let id: String = row["id"]
            if value["deleted"] != .bool(true), value["forwarded"] != .bool(true),
                value["contentUnavailable"] != .bool(true)
            {
                try save(
                    value, id: id, collection: "retained:" + collection, namespace: namespace,
                    db: db)
            }
            try db.execute(
                sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                arguments: [namespace, collection, id])
        }
        try db.execute(
            sql:
                "DELETE FROM records WHERE namespace=? AND collection IN('conversations','contacts','contact-requests','announcements','cursors')",
            arguments: [namespace])
    }

    private static func syncPlaceholder(_ value: IMJSON, id: String, global: Bool) -> IMJSON {
        var fields: [String: IMJSON] = [
            "id": .string(id), "deleted": .bool(global), "contentUnavailable": .bool(true),
        ]
        for key in [
            "conversationId", "senderId", "clientMsgId", "version", "contentVersion", "sequence",
            "createdAt", "forwarded", "forwardProjectionGeneration", "forwardProjectionUnknown",
            "forwardUnavailable",
        ] where value[key] != .null {
            fields[key] = value[key]
        }
        return .object(fields)
    }

    private static func syncApply(_ item: IMJSON, namespace: IMNamespace, db: Database) throws {
        let ref = item["ref"]
        let kind = ref["kind"].text
        let id = ref["id"].text
        let key = ref["owner"].text + "|" + kind + "|" + id
        let revision = item["currentRevision"]
        let epoch = item["currentRevisionEpoch"]
        if let old = try Data.fetchOne(
            db, sql: "SELECT body FROM sync_refs WHERE namespace=? AND id=?",
            arguments: [namespace.key, key])
        {
            let previous = try JSONDecoder.make().decode(IMJSON.self, from: old)
            guard let beforeEpoch = UInt64(previous["epoch"].text),
                let before = UInt64(previous["revision"].text),
                let afterEpoch = UInt64(epoch.text), let after = UInt64(revision.text)
            else {
                throw StoreError.invalidIdentity
            }
            if afterEpoch < beforeEpoch || (afterEpoch == beforeEpoch && after < before) {
                return
            }
        }
        if revision != .null, epoch != .null {
            let metadata: IMJSON = .object([
                "ref": ref, "revision": revision, "epoch": epoch, "state": item["state"],
            ])
            try db.execute(
                sql:
                    "INSERT INTO sync_refs VALUES(?,?,?) ON CONFLICT(namespace,id) DO UPDATE SET body=excluded.body",
                arguments: [namespace.key, key, try JSONEncoder.make().encode(metadata)])
        }
        if kind == "message", item["state"] == .string("global") {
            try syncGlobal(id, value: item["object"], namespace: namespace, db: db)
            return
        }
        let value = item["object"]
        if ["preferences", "conversation_view", "read_state", "read_observation", "draft"].contains(kind) {
            guard ref["subjectUserId"].text == namespace.account,
                value["scopeId"].text == namespace.deploymentID,
                value["userId"].text == namespace.account,
                value["sourceContextId"] == .string(namespace.sourceContextID),
                value["revisionEpoch"].text == namespace.restoreEpoch
            else {
                throw StoreError.invalidIdentity
            }
            let collection = "self:" + kind
            if value["removed"] == .bool(true) {
                try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                               arguments: [namespace.key, collection, id])
            } else if item["state"] == .string("available") {
                try Self.save(value.setting("id", .string(id)), id: id,
                              collection: collection, namespace: namespace.key, db: db)
                if kind == "read_state" || kind == "read_observation" {
                    try Self.applyPersonalRead(value, kind: kind, namespace: namespace, db: db)
                }
            } else if item["state"] == .string("unknown") || item["state"] == .string("redacted") {
                throw StoreError.invalidIdentity
            }
            return
        }
        guard item["state"] == .string("available") || item["state"] == .string("unavailable")
        else {
            if kind == "conversation" {
                try db.execute(
                    sql:
                        "DELETE FROM records WHERE namespace=? AND collection='conversations' AND id=?",
                    arguments: [namespace.key, id])
            }
            return
        }
        if kind == "message" {
            let conversation =
                value["conversationId"].text.isEmpty
                ? ref["conversationId"].text : value["conversationId"].text
            guard value["id"].text == id, !conversation.isEmpty,
                ref["conversationId"] == .null || value["conversationId"] == ref["conversationId"]
            else {
                throw StoreError.invalidIdentity
            }
            let body =
                item["state"] == .string("unavailable")
                ? syncPlaceholder(value, id: id, global: false) : value
            try save(
                body, id: id, collection: "messages:" + conversation, namespace: namespace.key,
                db: db)
            if item["state"] == .string("available"), value["forwarded"] != .bool(true),
                value["deleted"] != .bool(true),
                let installed = try Data.fetchOne(
                    db, sql: "SELECT body FROM records WHERE namespace=? AND collection=? AND id=?",
                    arguments: [namespace.key, "messages:" + conversation, id]),
                try syncLocalDigest(JSONDecoder.make().decode(IMJSON.self, from: installed))
                    == syncLocalDigest(value)
            {
                let state = try syncRead(namespace.key, db: db) ?? .null
                let eligibility: IMJSON = .object([
                    "version": value["version"], "restoreEpoch": .string(namespace.restoreEpoch),
                    "permissionGeneration": state["permissionGeneration"],
                    "classificationWatermark": state["classificationWatermark"],
                    "bodyDigest": .string(try syncLocalDigest(value)),
                ])
                try db.execute(
                    sql:
                        "INSERT INTO sync_eligibility VALUES(?,?,?) ON CONFLICT(namespace,id) DO UPDATE SET metadata=excluded.metadata",
                    arguments: [namespace.key, id, try JSONEncoder.make().encode(eligibility)])
            }
        } else if kind == "contacts" || kind == "relationship" {
            let collection = kind == "contacts" ? "contacts" : "relationships"
            try db.execute(
                sql: "DELETE FROM records WHERE namespace=? AND collection=?",
                arguments: [namespace.key, collection])
            for peer in value["items"].list {
                let peerID = peer["peerId"].text
                guard !peerID.isEmpty else {
                    throw StoreError.invalidIdentity
                }
                if kind == "contacts", peer["state"].text != "friend" {
                    continue
                }
                try save(
                    peer.setting("id", .string(peerID)), id: peerID, collection: collection,
                    namespace: namespace.key, db: db)
            }
        } else {
            let collections = [
                "conversation": "conversations", "contact_request": "contact-requests",
                "announcement": "announcements", "account": "account", "devices": "devices",
                "profile": "profile",
            ]
            guard let collection = collections[kind] else {
                throw StoreError.invalidIdentity
            }
            if kind == "conversation" || kind == "contact_request" || kind == "account" {
                guard value["id"].text == id else {
                    throw StoreError.invalidIdentity
                }
            }
            try save(
                value.setting("id", .string(id)), id: id, collection: collection,
                namespace: namespace.key, db: db)
        }
    }

    private static func applyPersonalRead(_ value: IMJSON, kind: String,
                                           namespace: IMNamespace, db: Database) throws {
        let conversation = value["conversationId"].text
        guard let rawConversation = try Data.fetchOne(db,
                                                      sql: "SELECT body FROM records WHERE namespace=? AND collection='conversations' AND id=?",
                                                      arguments: [namespace.key, conversation]) else {
            return
        }
        let current = try JSONDecoder.make().decode(IMJSON.self, from: rawConversation)
        guard current["membershipGeneration"] == value["membershipGeneration"] else {
            return
        }
        if kind == "read_observation" {
            if value["currentlyApplicable"] == .bool(true), value["eligibleAtObservation"] == .bool(true) {
                try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                               arguments: [namespace.key, "unread:" + conversation, value["messageId"].text])
            }
            return
        }
        guard value["writable"] == .bool(true),
            let readEpoch = UInt64(value["readPoint"]["epoch"].text),
            let readSequence = UInt64(value["readPoint"]["sequence"].text)
        else {
            return
        }
        let rows = try Row.fetchAll(db,
                                    sql: "SELECT r.id,m.body FROM records r JOIN records m ON m.namespace=r.namespace AND m.collection=? AND m.id=r.id WHERE r.namespace=? AND r.collection=?",
                                    arguments: ["messages:" + conversation, namespace.key, "unread:" + conversation])
        for row in rows {
            let message = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
            guard let epoch = UInt64(message["creationEpoch"].text),
                let sequence = UInt64(message["creationSequence"].text),
                epoch > 0, sequence > 0,
                epoch < readEpoch || epoch == readEpoch && sequence <= readSequence
            else {
                continue
            }
            try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                           arguments: [namespace.key, "unread:" + conversation, row["id"] as String])
        }
    }

    private static func syncGlobal(
        _ id: String, value: IMJSON, namespace: IMNamespace, db: Database
    ) throws {
        try db.execute(
            sql: "INSERT OR IGNORE INTO sync_global VALUES(?,?,?,?,?)",
            arguments: [
                namespace.profile, namespace.deploymentID, namespace.account,
                namespace.sourceContextID, id,
            ])
        let holders = try String.fetchAll(
            db,
            sql:
                "SELECT namespace FROM sync_scopes WHERE profile=? AND scope=? AND principal=? AND source=?",
            arguments: [
                namespace.profile, namespace.deploymentID, namespace.account,
                namespace.sourceContextID,
            ])
        for holder in holders {
            try receiptEraseMessage(id, namespace: holder, db: db)
            try db.execute(sql: """
                DELETE FROM records WHERE namespace=? AND collection='migration-stages'
                AND EXISTS(SELECT 1 FROM json_each(CAST(records.body AS TEXT),'$.messageIds') WHERE value=?)
                """, arguments: [holder, id])
            try db.execute(
                sql: "DELETE FROM sync_eligibility WHERE namespace=? AND id=?",
                arguments: [holder, id])
            for row in try Row.fetchAll(
                db, sql: "SELECT collection,id,body FROM records WHERE namespace=? AND collection<>'migration-stages'",
                arguments: [holder])
            {
                let body = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
                let rowID: String = row["id"]
                let collection: String = row["collection"]
                if collection == "receipt-intents" {
                    continue
                }
                if rowID == id || body["message"]["id"].text == id || body["messageId"].text == id {
                    guard collection != "auth-state", collection != "drafts" else {
                        continue
                    }
                    try save(
                        syncPlaceholder(body, id: rowID, global: true), id: rowID,
                        collection: collection, namespace: holder, db: db)
                }
            }
        }
        if !value["conversationId"].text.isEmpty {
            try save(
                syncPlaceholder(value, id: id, global: true), id: id,
                collection: "messages:" + value["conversationId"].text, namespace: namespace.key,
                db: db)
        }
    }

    private static func invalidateMigrationForward(_ id: String, namespace: String, db: Database) throws {
        let rows = try Row.fetchAll(db, sql: """
            SELECT r.namespace,r.collection,r.id,r.body FROM records r JOIN sync_scopes s ON s.namespace=r.namespace
            JOIN sync_scopes current ON current.namespace=? AND s.profile=current.profile AND s.scope=current.scope
                AND s.principal=current.principal AND s.source=current.source
            WHERE r.id=? AND r.collection LIKE 'migration-messages:%'
            """, arguments: [namespace, id])
        for row in rows {
            let value = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
            guard value["forwarded"] == .bool(true) else {
                continue
            }
            let placeholder = syncPlaceholder(value, id: id, global: value["deleted"] == .bool(true))
            try db.execute(sql: "UPDATE records SET body=? WHERE namespace=? AND collection=? AND id=?",
                arguments: [try JSONEncoder.make().encode(placeholder), row["namespace"] as String, row["collection"] as String, id])
        }
    }

    private static func syncLocalDigest(_ value: IMJSON) throws -> String {
        var fields = value.fields
        fields.removeValue(forKey: "forwardAdmission")
        let encoder = JSONEncoder.make()
        encoder.outputFormatting = [.sortedKeys]
        return SHA256.hash(data: try encoder.encode(IMJSON.object(fields))).map {
            String(format: "%02x", $0)
        }.joined()
    }

    private static func save(
        _ value: IMJSON, id: String, collection: String, namespace: String, db: Database,
        verifiedForwardProjection: Bool = false, observeForwardUnavailable: Bool = true
    )
        throws
    {
        var value = value
        if collection == "announcements" {
            var fields = value.fields
            fields.removeValue(forKey: "message")
            value = .object(fields)
        }
        let global =
            try Bool.fetchOne(
                db,
                sql:
                    "SELECT EXISTS(SELECT 1 FROM sync_scopes s JOIN sync_global g ON s.profile=g.profile AND s.scope=g.scope AND s.principal=g.principal AND s.source=g.source WHERE s.namespace=? AND g.id=?)",
                arguments: [namespace, id]) ?? false
        if global, collection != "auth-state", collection != "drafts" {
            value = Self.syncPlaceholder(value, id: id, global: true)
        }
        let authoritative =
            try Bool.fetchOne(
                db, sql: "SELECT EXISTS(SELECT 1 FROM sync_state WHERE namespace=?)",
                arguments: [namespace]) ?? false
        if authoritative, let state = try syncRead(namespace, db: db),
            !state["restriction"].text.isEmpty,
            collection.hasPrefix("messages:") || collection == "saved" || collection == "search"
        {
            if value["status"].text.isEmpty, value["forwarded"] != .bool(true),
                value["deleted"] != .bool(true)
            {
                try Self.save(
                    value, id: id, collection: "retained:" + collection, namespace: namespace,
                    db: db)
            }
            return
        }
        if authoritative, value["forwarded"] == .bool(true) {
            try Self.invalidateMigrationForward(id, namespace: namespace, db: db)
            value = Self.syncPlaceholder(value, id: id, global: value["deleted"] == .bool(true))
                .setting("forwarded", .bool(true))
        }
        if authoritative {
            var fields = value.fields
            fields.removeValue(forKey: "forwardAdmission")
            value = .object(fields)
        }
        var previousProjectionGeneration: UInt64 = 0
        if collection.hasPrefix("messages:"),
            let previous = try Data.fetchOne(
                db,
                sql:
                    "SELECT body FROM records WHERE namespace=? AND collection=? AND id=? UNION ALL SELECT body FROM records WHERE namespace=? AND collection=? AND id=? LIMIT 1",
                arguments: [namespace, collection, id, namespace, "retained:" + collection, id])
        {
            let old = try JSONDecoder.make().decode(IMJSON.self, from: previous)
            previousProjectionGeneration = UInt64(old["forwardProjectionGeneration"].text) ?? 0
            if let oldVersion = UInt64(old["version"].text),
                let newVersion = UInt64(value["version"].text),
                oldVersion > newVersion
            {
                return
            }
            if old["deleted"] == .bool(true) && value["deleted"] != .bool(true) {
                return
            }
            if old["forwarded"] == .bool(true) {
                value = value.setting("forwarded", .bool(true))
                if old["forwardUnavailable"] == .bool(true),
                    value["forwardUnavailable"] != .bool(true),
                    !verifiedForwardProjection
                {
                    guard value["deleted"] == .bool(true) else {
                        return
                    }
                    value = value.setting("forwardUnavailable", .bool(true))
                }
                if !verifiedForwardProjection {
                    value = value.setting(
                        "forwardProjectionGeneration", old["forwardProjectionGeneration"])
                    if old["forwardProjectionUnknown"] == .bool(true),
                        value["forwardUnavailable"] != .bool(true),
                        value["deleted"] != .bool(true)
                    {
                        var fields = value.fields
                        for key in [
                            "text", "attachmentId", "attachmentURL", "forwardAdmission",
                            "replyToId",
                        ] {
                            fields.removeValue(forKey: key)
                        }
                        fields["contentUnavailable"] = .bool(true)
                        fields["forwardProjectionUnknown"] = .bool(true)
                        value = .object(fields)
                    }
                }
            }
        }
        // 200不可用投影也可能只是暂时未知；它建立本地围栏，只有之后发起的精确GET能解除。
        if collection.hasPrefix("messages:"), value["forwarded"] == .bool(true),
            value["forwardUnavailable"] == .bool(true), observeForwardUnavailable
        {
            guard previousProjectionGeneration < UInt64.max else {
                throw StoreError.invalidIdentity
            }
            var fields = value.fields
            for key in ["text", "attachmentId", "attachmentURL", "forwardAdmission", "replyToId"] {
                fields.removeValue(forKey: key)
            }
            fields["contentUnavailable"] = .bool(true)
            fields["forwardProjectionUnknown"] = .bool(true)
            fields["forwardProjectionGeneration"] = .string(
                String(previousProjectionGeneration + 1))
            value = .object(fields)
        }
        let bytes = try JSONEncoder.make().encode(value)
        let order = value.firstText("createdAt", "serverTime", "id")
        try db.execute(
            sql:
                "INSERT INTO records(namespace,collection,id,body,ordering) VALUES(?,?,?,?,?) ON CONFLICT(namespace,collection,id) DO UPDATE SET body=excluded.body,ordering=excluded.ordering",
            arguments: [namespace, collection, id, bytes, order])
    }

    private static func checkDeletionFloor(_ namespace: IMNamespace, db: Database) throws {
        guard !namespace.demo, !namespace.account.hasPrefix("__") else {
            return
        }
        let denied = try Bool.fetchOne(db, sql: "SELECT EXISTS(SELECT 1 FROM local_account_erasures WHERE profile=? AND scope=? AND principal=?) OR EXISTS(SELECT 1 FROM local_account_erasure_namespaces WHERE namespace=?)",
                                      arguments: [namespace.profile, namespace.deploymentID, namespace.account, namespace.key]) ?? false
        guard !denied else {
            throw StoreError.invalidIdentity
        }
    }

    private static func registerDeletionNamespace(_ namespace: IMNamespace, db: Database) throws {
        try checkDeletionFloor(namespace, db: db)
        guard !namespace.demo, !namespace.account.hasPrefix("__"), !namespace.profile.isEmpty,
            !namespace.deploymentID.isEmpty, !namespace.account.isEmpty else {
            return
        }
        try db.execute(sql: "INSERT OR IGNORE INTO account_namespace_registry VALUES(?,?,?,?,?)",
                       arguments: [namespace.key, namespace.profile, namespace.deploymentID, namespace.account, namespace.sourceContextID])
    }

    public func accountIsErased(profile: String, deployment: String, user: String) throws -> Bool {
        return try database.read { db in
            return try Bool.fetchOne(db, sql: "SELECT EXISTS(SELECT 1 FROM local_account_erasures WHERE profile=? AND scope=? AND principal=?)",
                                     arguments: [profile, deployment, user]) ?? false
        }
    }

    public func compareAccountDeletionIntent(namespace: IMNamespace, previous: IMJSON?, next: IMJSON) throws {
        let fields: Set<String> = ["id", "username", "profile", "deploymentId", "userId", "deviceId", "familyId", "restoreEpoch",
                                   "namespaceKey", "createdAt", "state", "external", "deletionId", "requestVersion", "result",
                                   "cancelNonce", "cancelVersion", "cancelSent", "cleanup"]
        let mutable: Set<String> = ["state", "deletionId", "requestVersion", "result", "cancelNonce", "cancelVersion", "cancelSent", "cleanup"]
        let publicResult: Set<String> = ["deletionId", "requestId", "status", "requestVersion", "restoreEpoch", "cancelUntil", "cancelRequestId", "requiresNewLogin"]
        guard namespace.account == "__account_deletions__", !namespace.demo,
            case .object(let values) = next, Set(values.keys) == fields,
            next["result"] == .null || (Set(next["result"].fields.keys).isSubset(of: publicResult)
                && next["result"].fields.values.allSatisfy { value in
                    switch value {
                    case .null, .string, .bool: return true
                    default: return false
                    }
                }),
            !next["id"].text.isEmpty else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='account-deletion-operations' AND id=?",
                                       arguments: [namespace.key, next["id"].text])
            let current = try raw.map { try JSONDecoder.make().decode(IMJSON.self, from: $0) }
            guard current == previous else {
                throw StoreError.conflictingIdentity
            }
            if let previous {
                for key in fields.subtracting(mutable) where previous[key] != next[key] {
                    throw StoreError.conflictingIdentity
                }
                for key in ["deletionId", "cancelNonce", "cancelVersion"] where previous[key] != .null && previous[key] != next[key] {
                    throw StoreError.conflictingIdentity
                }
            } else {
                for raw in try Data.fetchAll(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='account-deletion-operations'", arguments: [namespace.key]) {
                    let row = try JSONDecoder.make().decode(IMJSON.self, from: raw)
                    if !next["familyId"].text.isEmpty, row["familyId"] == next["familyId"],
                        !["rejected", "cancelled", "retired"].contains(row["state"].text) {
                        throw StoreError.conflictingIdentity
                    }
                }
            }
            try Self.save(next, id: next["id"].text, collection: "account-deletion-operations", namespace: namespace.key, db: db)
        }
    }

    public func registerAccountPrivateFile(namespace: IMNamespace, kind: String, name: String) throws {
        guard kind == "export", name.range(of: "^JobsIMDataExport-[0-9A-Fa-f-]{36}$", options: .regularExpression) != nil,
            !namespace.demo, !namespace.account.hasPrefix("__") else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try db.execute(sql: "INSERT OR IGNORE INTO local_account_private_files VALUES(?,?,?,?,?,?)",
                           arguments: [namespace.profile, namespace.deploymentID, namespace.account, namespace.key, kind, name])
        }
    }

    public func beginAccountDeletionCleanup(profile: String, deployment: String, user: String, originalNamespace: String) throws -> [String] {
        guard !profile.isEmpty, UUID(uuidString: deployment) != nil, UUID(uuidString: user) != nil,
            originalNamespace.isEmpty || originalNamespace.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil else {
            throw StoreError.invalidIdentity
        }
        return try database.write { db in
            try db.execute(sql: "INSERT OR IGNORE INTO local_account_erasures(profile,scope,principal) VALUES(?,?,?)", arguments: [profile, deployment, user])
            var keys = Set(try String.fetchAll(db, sql: """
                SELECT namespace FROM account_namespace_registry WHERE profile=? AND scope=? AND principal=?
                UNION SELECT namespace FROM sync_scopes WHERE profile=? AND scope=? AND principal=? AND source='personal'
                UNION SELECT namespace FROM local_account_erasure_namespaces WHERE profile=? AND scope=? AND principal=?
                """, arguments: [profile, deployment, user, profile, deployment, user, profile, deployment, user]))
            if !originalNamespace.isEmpty {
                keys.insert(originalNamespace)
            }
            for key in keys {
                guard key.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil else {
                    throw StoreError.invalidIdentity
                }
                try db.execute(sql: "INSERT OR IGNORE INTO local_account_erasure_namespaces VALUES(?,?,?,?)", arguments: [profile, deployment, user, key])
            }
            return keys.sorted()
        }
    }

    public func accountDeletionFiles(profile: String, deployment: String, user: String) throws -> [IMJSON] {
        return try database.read { db in
            return try Row.fetchAll(db, sql: "SELECT kind,name FROM local_account_private_files WHERE profile=? AND scope=? AND principal=?",
                                    arguments: [profile, deployment, user]).map { row in
                return .object(["kind": .string(row["kind"] as String), "name": .string(row["name"] as String)])
            }
        }
    }

    public func finishAccountDeletionCleanup(profile: String, deployment: String, user: String) throws {
        try database.write { db in
            let keys = try String.fetchAll(db, sql: "SELECT namespace FROM local_account_erasure_namespaces WHERE profile=? AND scope=? AND principal=?",
                                           arguments: [profile, deployment, user])
            let tables = ["records", "outbox", "sync_state", "sync_scopes", "sync_origins", "sync_events", "sync_eligibility", "sync_refs", "sync_pages", "account_namespace_registry", "receipt_file_cleanup"]
            for key in keys {
                for table in tables {
                    try db.execute(sql: "DELETE FROM " + table + " WHERE namespace=?", arguments: [key])
                }
            }
            let rows = try Row.fetchAll(db, sql: "SELECT namespace,collection,id,body FROM records WHERE collection IN('auth-state','password-change-operations')")
            var ownedMetadata = Set<String>()
            for row in rows {
                let value = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
                if value["profile"].text == profile && value["deploymentId"].text == deployment && value["userId"].text == user {
                    ownedMetadata.insert(row["namespace"] as String)
                }
            }
            for row in rows where ownedMetadata.contains(row["namespace"] as String) {
                let value = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
                if value["userId"].text == user || value["user"]["id"].text == user || value["resultUser"]["id"].text == user {
                    try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                                   arguments: [row["namespace"] as String, row["collection"] as String, row["id"] as String])
                }
            }
            try db.execute(sql: "DELETE FROM local_account_private_files WHERE profile=? AND scope=? AND principal=?", arguments: [profile, deployment, user])
        }
        try database.writeWithoutTransaction { db in
            guard try Int.fetchOne(db, sql: "PRAGMA wal_checkpoint(TRUNCATE)") == 0 else {
                throw StoreError.conflictingIdentity
            }
        }
        try database.write { db in
            try db.execute(sql: "UPDATE local_account_erasures SET completed=1 WHERE profile=? AND scope=? AND principal=?", arguments: [profile, deployment, user])
        }
    }

    private static func migrationDate(_ value: String) -> Date? {
        let fractional = ISO8601DateFormatter()
        fractional.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return fractional.date(from: value) ?? ISO8601DateFormatter.make().date(from: value)
    }

    public func redactMigrationForward(_ id: String, namespace: IMNamespace) throws {
        guard UUID(uuidString: id) != nil else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.invalidateMigrationForward(id, namespace: namespace.key, db: db)
        }
    }

    public func stageMigrationCapsule(namespace: IMNamespace, expected: IMJSON, id: String,
                                      manifestDigest: String, capsuleSHA256: String, expiresAt: String,
                                      messageIDs: [String], capsule: Data) throws {
        guard !namespace.demo, namespace.sourceContextID == "personal", UUID(uuidString: id) != nil,
            manifestDigest.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil,
            capsuleSHA256.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil,
            !messageIDs.isEmpty, messageIDs.count <= 256, Set(messageIDs).count == messageIDs.count,
            messageIDs.allSatisfy({ UUID(uuidString: $0) != nil }), capsule.count >= 36,
            capsule.count <= 128 * 1024 * 1024 + 4096, capsule.prefix(8) == Data("JIMMIGR1".utf8),
            SHA256.hash(data: capsule).map({ String(format: "%02x", $0) }).joined() == capsuleSHA256,
            let expiry = Self.migrationDate(expiresAt), expiry > Date() else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.syncCompare(expected, namespace: namespace.key, db: db)
            guard expected["restriction"].text.isEmpty,
                expected["scopeId"].text == namespace.deploymentID, expected["principalId"].text == namespace.account,
                expected["deviceId"].text == namespace.deviceID, expected["restoreEpoch"].text == namespace.restoreEpoch,
                UUID(uuidString: expected["storeGeneration"].text) != nil else {
                throw StoreError.conflictingIdentity
            }
            for message in messageIDs {
                let denied = try Bool.fetchOne(db, sql: "SELECT EXISTS(SELECT 1 FROM sync_global WHERE profile=? AND scope=? AND principal=? AND source=? AND id=?)",
                    arguments: [namespace.profile, namespace.deploymentID, namespace.account, namespace.sourceContextID, message]) ?? true
                guard !denied else {
                    throw StoreError.conflictingIdentity
                }
            }
            if let bytes = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='migration-stages' AND id=?", arguments: [namespace.key, id]) {
                let old = try JSONDecoder.make().decode(IMJSON.self, from: bytes)
                guard old["manifestDigest"].text == manifestDigest, old["capsuleSHA256"].text == capsuleSHA256,
                    old["storeGeneration"] == expected["storeGeneration"] else {
                    throw StoreError.conflictingIdentity
                }
                return
            }
            let encoded = capsule.base64EncodedString().replacingOccurrences(of: "+", with: "-")
                .replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "")
            try Self.save(.object(["id": .string(id), "manifestDigest": .string(manifestDigest),
                "capsuleSHA256": .string(capsuleSHA256), "expiresAt": .string(expiresAt),
                "messageIds": .array(messageIDs.map(IMJSON.string)), "capsule": .string(encoded),
                "storeGeneration": expected["storeGeneration"]]), id: id, collection: "migration-stages", namespace: namespace.key, db: db)
        }
    }

    public func compareMigrationIntent(namespace: IMNamespace, previous: IMJSON?, next: IMJSON) throws {
        let fields: Set<String> = ["id", "kind", "createdAt", "profile", "deploymentId", "userId", "deviceId", "familyId",
                                   "restoreEpoch", "namespaceKey", "storeGeneration", "state", "attempts", "selection",
                                   "sourceDigest", "target", "migrationId", "manifestDigest", "lastStatus"]
        let mutable: Set<String> = ["state", "attempts", "sourceDigest", "target", "migrationId", "manifestDigest", "lastStatus"]
        guard !namespace.demo, namespace.restoreEpoch == "migration-protocol", namespace.sourceContextID == "personal",
            case .object(let values) = next, Set(values.keys) == fields,
            next["profile"].text == namespace.profile, next["deploymentId"].text == namespace.deploymentID,
            next["userId"].text == namespace.account, UUID(uuidString: next["id"].text) != nil,
            Self.migrationDate(next["createdAt"].text) != nil,
            case .string = next["restoreEpoch"], UInt64(next["restoreEpoch"].text) != nil,
            UUID(uuidString: next["deviceId"].text) != nil, UUID(uuidString: next["familyId"].text) != nil,
            UUID(uuidString: next["storeGeneration"].text) != nil,
            next["namespaceKey"].text.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil,
            next["restoreEpoch"].text.range(of: "^[1-9][0-9]*$", options: .regularExpression) != nil,
            ["pairing", "create", "target"].contains(next["kind"].text),
            ["unsent", "unknown", "ready", "installed", "blocked"].contains(next["state"].text),
            case .number = next["attempts"], let attempts = Int(next["attempts"].text), (0...10000).contains(attempts),
            case .array(let selection) = next["selection"], selection.count <= 256,
            next["kind"].text != "create" || !selection.isEmpty,
            next["kind"].text == "create" || selection.isEmpty,
            Set(selection.map { $0["id"].text }).count == selection.count else {
            throw StoreError.invalidIdentity
        }
        for ref in selection {
            guard case .object(let item) = ref,
                Set(item.keys).isSubset(of: ["id", "conversationId", "contentVersion", "type", "attachmentId", "forwarded", "objectMode"]),
                UUID(uuidString: ref["id"].text) != nil, UUID(uuidString: ref["conversationId"].text) != nil,
                case .string = ref["contentVersion"], UInt64(ref["contentVersion"].text) != nil,
                ref["contentVersion"].text.range(of: "^[1-9][0-9]*$", options: .regularExpression) != nil,
                ["text", "image", "video", "audio", "file"].contains(ref["type"].text),
                ["none", "metadata_only", "full"].contains(ref["objectMode"].text),
                ref["attachmentId"] == .null || UUID(uuidString: ref["attachmentId"].text) != nil,
                ref["forwarded"] == .null || ref["forwarded"] == .bool(true) else {
                throw StoreError.invalidIdentity
            }
        }
        guard selection.filter({ $0["objectMode"].text == "full" }).count <= 64 else {
            throw StoreError.invalidIdentity
        }
        for key in ["sourceDigest", "manifestDigest"] where next[key] != .null {
            guard case .string = next[key], next[key].text.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil else {
                throw StoreError.invalidIdentity
            }
        }
        if next["migrationId"] != .null, UUID(uuidString: next["migrationId"].text) == nil {
            throw StoreError.invalidIdentity
        }
        if next["target"] != .null {
            guard Set(next["target"].fields.keys) == Set(["pairingId", "deviceId", "storeGeneration"]),
                next["target"].fields.values.allSatisfy({ UUID(uuidString: $0.text) != nil }) else {
                throw StoreError.invalidIdentity
            }
        }
        guard next["lastStatus"] == .null || ["building", "ready", "installed_reported", "revoking", "retired"].contains(next["lastStatus"].text) else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='migration-intents' AND id=?", arguments: [namespace.key, next["id"].text])
            let current = try raw.map { try JSONDecoder.make().decode(IMJSON.self, from: $0) }
            guard current == previous else {
                throw StoreError.conflictingIdentity
            }
            if let previous {
                for key in fields.subtracting(mutable) where previous[key] != next[key] {
                    throw StoreError.conflictingIdentity
                }
                for key in ["sourceDigest", "target", "migrationId", "manifestDigest"] where previous[key] != .null && previous[key] != next[key] {
                    throw StoreError.conflictingIdentity
                }
                guard let before = Int(previous["attempts"].text), attempts == before || attempts == before + 1,
                    previous["state"].text != "installed" || next["state"].text == "installed" else {
                    throw StoreError.conflictingIdentity
                }
            } else if attempts != 0 || next["state"].text != "unsent" {
                throw StoreError.invalidIdentity
            }
            try Self.save(next, id: next["id"].text, collection: "migration-intents", namespace: namespace.key, db: db)
        }
    }

    /// A personal migration is a separate archive. Its transaction never advances
    /// a sync cursor or changes the original outbox/receipt qualification.
    public func commitMigrationArchive(namespace: IMNamespace, expected: IMJSON, manifest: IMJSON,
                                       digest: String, packet: IMJSON, installBefore: Date) throws {
        guard !namespace.demo, namespace.sourceContextID == "personal",
            UUID(uuidString: manifest["id"].text) != nil, digest.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil,
            manifest["scopeId"].text == namespace.deploymentID, manifest["userId"].text == namespace.account,
            manifest["targetDeviceId"].text == namespace.deviceID, manifest["restoreEpoch"].text == namespace.restoreEpoch,
            manifest["targetStoreGeneration"] == expected["storeGeneration"], packet["manifestDigest"].text == digest,
            !packet["messages"].list.isEmpty, packet["messages"].list.count == manifest["messages"].list.count,
            packet["objects"].list.count == manifest["objects"].list.count,
            packet["messages"].list.count <= 256, packet["objects"].list.count <= 64 else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try Self.registerDeletionNamespace(namespace, db: db)
            try Self.syncCompare(expected, namespace: namespace.key, db: db)
            guard expected["restriction"].text.isEmpty, installBefore > Date() else {
                throw StoreError.conflictingIdentity
            }
            let identifier = manifest["id"].text
            if let data = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='migration-archives' AND id=?", arguments: [namespace.key, identifier]) {
                let previous = try JSONDecoder.make().decode(IMJSON.self, from: data)
                guard previous["manifestDigest"].text == digest else {
                    throw StoreError.conflictingIdentity
                }
                return
            }
            for ref in manifest["messages"].list {
                let denied = try Bool.fetchOne(db, sql: "SELECT EXISTS(SELECT 1 FROM sync_global WHERE profile=? AND scope=? AND principal=? AND source=? AND (id=? OR id=?))",
                    arguments: [namespace.profile, namespace.deploymentID, namespace.account, namespace.sourceContextID, ref["messageId"].text, ref["objectId"].text]) ?? true
                guard !denied,
                    let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='conversations' AND id=?", arguments: [namespace.key, ref["conversationId"].text]) else {
                    throw StoreError.conflictingIdentity
                }
                let conversation = try JSONDecoder.make().decode(IMJSON.self, from: raw)
                guard conversation["membershipGeneration"] == ref["membershipGeneration"],
                    ["active", "owner_unavailable"].contains(conversation["state"].text),
                    let content = UInt64(ref["contentVersion"].text) else {
                    throw StoreError.conflictingIdentity
                }
                let rows = try Data.fetchAll(db, sql: "SELECT body FROM records WHERE namespace=? AND id=? AND (collection LIKE 'messages:%' OR collection LIKE 'migration-messages:%')", arguments: [namespace.key, ref["messageId"].text])
                for body in rows {
                    let local = try JSONDecoder.make().decode(IMJSON.self, from: body)
                    guard local["deleted"] != .bool(true), local["deleteScope"].text != "global",
                        let version = UInt64(local["contentVersion"].text), version <= content else {
                        throw StoreError.conflictingIdentity
                    }
                    if version == content {
                        guard local["conversationId"] == ref["conversationId"] else {
                            throw StoreError.conflictingIdentity
                        }
                    }
                }
            }
            for message in packet["messages"].list {
                // The verified encrypted package is an independent historical copy,
                // not an ordinary forward projection or a sendable message.
                try db.execute(sql: "INSERT INTO records(namespace,collection,id,body,ordering) VALUES(?,?,?,?,?)",
                    arguments: [namespace.key, "migration-messages:" + identifier, message["id"].text,
                                try JSONEncoder.make().encode(message), message["createdAt"].text])
            }
            for object in packet["objects"].list {
                let row = object.setting("id", object["objectId"])
                try Self.save(row, id: object["objectId"].text, collection: "migration-objects:" + identifier,
                              namespace: namespace.key, db: db)
            }
            try Self.save(.object(["id": .string(identifier), "manifestDigest": .string(digest), "manifest": manifest,
                                   "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())),
                                   "messageCount": .number(Decimal(packet["messages"].list.count)),
                                   "objectCount": .number(Decimal(packet["objects"].list.count)),
                                   "targetQualified": .bool(false), "qualifiedAck": .bool(false)]),
                          id: identifier, collection: "migration-archives", namespace: namespace.key, db: db)
            guard installBefore > Date() else {
                throw StoreError.conflictingIdentity
            }
            try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection='migration-stages' AND id=?", arguments: [namespace.key, identifier])
        }
    }

    public enum StoreError: Error {
        case invalidIdentity, conflictingIdentity
    }
}

extension IMLocalStore {

    private static let receiptIntentFields: Set<String> = [
        "id", "kind", "familyId", "storeGeneration", "restoreEpoch", "schemaVersion", "createdAt", "attempts", "state",
        "originalBody", "messageId", "conversationId", "contentVersion", "membershipGeneration", "qualificationId",
        "permissionGeneration", "deliveryGeneration", "manifestDigest", "bodyDigest", "result",
    ]

    private func receiptStoreGuard(namespace: IMNamespace, expected: IMJSON, db: Database, writable: Bool = true) throws {
        var current = stat()
        guard let identity = databaseFileIdentity, lstat(databasePath, &current) == 0,
            current.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG), current.st_dev == identity.device, current.st_ino == identity.inode else {
            throw StoreError.conflictingIdentity
        }
        try Self.receiptGuard(namespace: namespace, expected: expected, db: db, writable: writable)
    }

    private static func receiptEnvelopeBytes(_ value: String) throws -> Data {
        let standard = value.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/")
        let padded = standard + String(repeating: "=", count: (4 - standard.count % 4) % 4)
        guard value.utf8.count <= 2 * 1024 * 1024, let bytes = Data(base64Encoded: padded),
            bytes.base64EncodedString().replacingOccurrences(of: "+", with: "-").replacingOccurrences(of: "/", with: "_").replacingOccurrences(of: "=", with: "") == value else {
            throw StoreError.invalidIdentity
        }
        return bytes
    }

    private static func receiptHash(_ data: Data) -> String {
        return SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
    }

    private static func receiptUUID(_ value: IMJSON) -> Bool {
        guard case .string(let text) = value else {
            return false
        }
        return text.range(of: "^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$", options: .regularExpression) != nil
    }

    private static func receiptCounter(_ value: IMJSON, zero: Bool = false) -> Bool {
        guard case .string = value, let number = UInt64(value.text), number <= UInt64(Int64.max), zero || number > 0 else {
            return false
        }
        return value.text == String(number)
    }

    private static func receiptDigest(_ value: IMJSON) -> Bool {
        guard case .string = value else {
            return false
        }
        return value.text.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil
    }

    private static func receiptDurability(_ db: Database) throws {
        guard try String.fetchOne(db, sql: "PRAGMA journal_mode")?.lowercased() == "wal",
            try Int.fetchOne(db, sql: "PRAGMA synchronous") == 2,
            try Int.fetchOne(db, sql: "PRAGMA fullfsync") == 1,
            try Int.fetchOne(db, sql: "PRAGMA checkpoint_fullfsync") == 1 else {
            throw StoreError.conflictingIdentity
        }
    }

    private static func receiptGuard(namespace: IMNamespace, expected: IMJSON, db: Database, writable: Bool = true) throws {
        try registerDeletionNamespace(namespace, db: db)
        try syncCompare(expected, namespace: namespace.key, db: db)
        guard !namespace.demo, namespace.sourceContextID == "personal", receiptUUID(.string(namespace.deviceID)),
            expected["scopeId"].text == namespace.deploymentID, expected["principalId"].text == namespace.account,
            expected["deviceId"].text == namespace.deviceID, expected["restoreEpoch"].text == namespace.restoreEpoch,
            expected["sourceContextId"].text == "personal", receiptUUID(expected["storeGeneration"]),
            receiptUUID(expected["streamId"]), !writable || expected["restriction"].text.isEmpty,
            try String.fetchOne(db, sql: "SELECT value FROM sync_metadata WHERE id='storeGeneration'") == expected["storeGeneration"].text else {
            throw StoreError.conflictingIdentity
        }
        try receiptDurability(db)
    }

    private static func receiptValidateIntent(_ intent: IMJSON, namespace: IMNamespace, expected: IMJSON, requireCurrentPermission: Bool = true) throws {
        guard Set(intent.fields.keys) == receiptIntentFields,
            ["content", "object"].contains(intent["kind"].text), receiptUUID(intent["id"]), receiptUUID(intent["familyId"]),
            intent["storeGeneration"] == expected["storeGeneration"], intent["restoreEpoch"].text == namespace.restoreEpoch,
            intent["schemaVersion"] == expected["schemaVersion"] || intent["schemaVersion"].text == "1" && expected["schemaVersion"] == .null,
            migrationDate(intent["createdAt"].text) != nil, case .number = intent["attempts"],
            let attempts = Int(intent["attempts"].text), (0...10000).contains(attempts),
            ["prepared", "unknown", "completed", "frozen"].contains(intent["state"].text),
            receiptUUID(intent["messageId"]), receiptUUID(intent["conversationId"]), receiptUUID(intent["qualificationId"]),
            receiptCounter(intent["contentVersion"]), receiptCounter(intent["membershipGeneration"]),
            receiptDigest(intent["bodyDigest"]), receiptDigest(intent["permissionGeneration"]),
            !requireCurrentPermission || intent["permissionGeneration"] == expected["permissionGeneration"],
            case .string = intent["deliveryGeneration"], case .string = intent["manifestDigest"],
            intent["deliveryGeneration"].text.isEmpty && intent["manifestDigest"].text.isEmpty
                || receiptCounter(intent["deliveryGeneration"]) && receiptDigest(intent["manifestDigest"]),
            intent["result"] == .null || !intent["result"].fields.isEmpty else {
            throw StoreError.invalidIdentity
        }
        let body = intent["originalBody"]
        let common: Set<String> = ["requestId", "storeGeneration", "revisionEpoch", "qualificationId", "kind", "challenge"]
        let fields = intent["kind"].text == "content" ? common.union(["bodyDigest"]) : common.union(["parentReceiptId", "objectId", "sizeBytes", "objectDigest"])
        guard Set(body.fields.keys) == fields, body["requestId"] == intent["id"], body["storeGeneration"] == intent["storeGeneration"],
            body["revisionEpoch"] == intent["restoreEpoch"], body["qualificationId"] == intent["qualificationId"],
            case .string = body["challenge"], !body["challenge"].text.isEmpty, body["challenge"].text.utf8.count <= 8192 else {
            throw StoreError.invalidIdentity
        }
        if intent["kind"].text == "content" {
            guard body["kind"].text == "message_content", body["bodyDigest"] == intent["bodyDigest"], body["challenge"].text.hasPrefix("rc1.") else {
                throw StoreError.invalidIdentity
            }
        } else {
            guard body["kind"].text == "object_received", body["challenge"].text.hasPrefix("ro1."), receiptUUID(body["parentReceiptId"]),
                receiptUUID(body["objectId"]), receiptCounter(body["sizeBytes"], zero: true),
                let length = Int(body["sizeBytes"].text), length <= 64 * 1024 * 1024, receiptDigest(body["objectDigest"]) else {
                throw StoreError.invalidIdentity
            }
        }
    }

    private static func receiptMessageGuard(_ intent: IMJSON, namespace: IMNamespace, db: Database) throws {
        let denied = try Bool.fetchOne(db, sql: "SELECT EXISTS(SELECT 1 FROM sync_global WHERE profile=? AND scope=? AND principal=? AND source=? AND (id=? OR id=?))",
            arguments: [namespace.profile, namespace.deploymentID, namespace.account, namespace.sourceContextID,
                        intent["messageId"].text, intent["originalBody"]["objectId"].text]) ?? true
        guard !denied,
            let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='conversations' AND id=?",
                                      arguments: [namespace.key, intent["conversationId"].text]) else {
            throw StoreError.conflictingIdentity
        }
        let conversation = try JSONDecoder.make().decode(IMJSON.self, from: raw)
        guard conversation["membershipGeneration"] == intent["membershipGeneration"], ["active", "owner_unavailable"].contains(conversation["state"].text) else {
            throw StoreError.conflictingIdentity
        }
        if let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection=? AND id=?",
                                      arguments: [namespace.key, "messages:" + intent["conversationId"].text, intent["messageId"].text]) {
            let local = try JSONDecoder.make().decode(IMJSON.self, from: raw)
            guard local["deleted"] != .bool(true), local["forwarded"] != .bool(true),
                let version = UInt64(local["contentVersion"].text), let received = UInt64(intent["contentVersion"].text), version <= received else {
                throw StoreError.conflictingIdentity
            }
        }
    }

    public func compareReceiptDeclaration(namespace: IMNamespace, expected: IMJSON, previous: IMJSON?, next: IMJSON) throws {
        let fields: Set<String> = ["id", "familyId", "createdAt", "attempts", "state", "originalBody", "result"]
        let body = next["originalBody"]
        guard Set(next.fields.keys) == fields, Self.receiptUUID(next["id"]), Self.receiptUUID(next["familyId"]),
            Self.migrationDate(next["createdAt"].text) != nil, case .number = next["attempts"], let attempts = Int(next["attempts"].text),
            (0...10000).contains(attempts), ["prepared", "unknown", "completed", "frozen"].contains(next["state"].text),
            Set(body.fields.keys) == Set(["requestId", "storeGeneration", "revisionEpoch", "profileId", "buildDigest", "verificationDigest"]),
            body["requestId"] == next["id"], body["storeGeneration"] == expected["storeGeneration"], body["revisionEpoch"].text == namespace.restoreEpoch,
            body["profileId"].text.range(of: "^[a-z0-9_.-]{8,128}$", options: .regularExpression) != nil,
            Self.receiptDigest(body["buildDigest"]), Self.receiptDigest(body["verificationDigest"]),
            next["result"] == .null || !next["result"].fields.isEmpty else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try receiptStoreGuard(namespace: namespace, expected: expected, db: db, writable: previous == nil || next["attempts"] != previous?["attempts"])
            let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-declarations' AND id=?",
                                       arguments: [namespace.key, next["id"].text])
            let current = try raw.map { try JSONDecoder.make().decode(IMJSON.self, from: $0) }
            guard current == previous else {
                throw StoreError.conflictingIdentity
            }
            try Self.receiptMutation(previous: previous, next: next, mutable: ["attempts", "state", "result"])
            try Self.save(next, id: next["id"].text, collection: "receipt-declarations", namespace: namespace.key, db: db)
        }
    }

    private static func receiptMutation(previous: IMJSON?, next: IMJSON, mutable: Set<String>) throws {
        guard let previous else {
            guard next["attempts"] == .number(0), next["state"].text == "prepared", next["result"] == .null else {
                throw StoreError.invalidIdentity
            }
            return
        }
        for field in Set(next.fields.keys).subtracting(mutable) where previous[field] != next[field] {
            throw StoreError.conflictingIdentity
        }
        guard let before = Int(previous["attempts"].text), let after = Int(next["attempts"].text), after == before || after == before + 1,
            previous["state"].text != "completed" || next["state"].text == "completed",
            previous["state"].text != "frozen" || next["state"].text == "frozen",
            previous["result"] == .null || previous["result"] == next["result"] else {
            throw StoreError.conflictingIdentity
        }
    }

    public func compareReceiptIntent(namespace: IMNamespace, expected: IMJSON, previous: IMJSON, next: IMJSON) throws {
        try Self.receiptValidateIntent(next, namespace: namespace, expected: expected, requireCurrentPermission: next["attempts"] != previous["attempts"])
        try database.write { db in
            try receiptStoreGuard(namespace: namespace, expected: expected, db: db, writable: next["attempts"] != previous["attempts"])
            guard let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-intents' AND id=?",
                                             arguments: [namespace.key, next["id"].text]),
                try JSONDecoder.make().decode(IMJSON.self, from: raw) == previous else {
                throw StoreError.conflictingIdentity
            }
            try Self.receiptMutation(previous: previous, next: next, mutable: ["attempts", "state", "result"])
            if next["attempts"] != previous["attempts"] {
                try Self.receiptMessageGuard(next, namespace: namespace, db: db)
                if next["kind"].text == "content" {
                    guard let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-bodies' AND id=?",
                                                     arguments: [namespace.key, next["id"].text]) else {
                        throw StoreError.conflictingIdentity
                    }
                    let stored = try JSONDecoder.make().decode(IMJSON.self, from: raw)
                    let bytes = try Self.receiptEnvelopeBytes(stored["bodyEnvelope"].text)
                    guard stored["messageId"] == next["messageId"], stored["bodyDigest"] == next["bodyDigest"],
                        Self.receiptHash(bytes) == next["bodyDigest"].text else {
                        throw StoreError.conflictingIdentity
                    }
                } else {
                    try Self.receiptObjectParent(next, namespace: namespace, db: db)
                    let body = next["originalBody"]
                    guard let size = Int(body["sizeBytes"].text) else {
                        throw StoreError.invalidIdentity
                    }
                    _ = try Self.receiptReadFile(Self.receiptFile(namespace: namespace.key, message: next["messageId"].text,
                        request: next["id"].text, create: false), size: size, digest: body["objectDigest"].text)
                }
            }
            try Self.save(next, id: next["id"].text, collection: "receipt-intents", namespace: namespace.key, db: db)
        }
    }

    public func commitReceiptContent(namespace: IMNamespace, expected: IMJSON, intent: IMJSON, bodyEnvelope: String) throws {
        try Self.receiptValidateIntent(intent, namespace: namespace, expected: expected)
        let body = try Self.receiptEnvelopeBytes(bodyEnvelope)
        guard intent["kind"].text == "content", Self.receiptHash(body) == intent["bodyDigest"].text else {
            throw StoreError.invalidIdentity
        }
        let message = try JSONDecoder.make().decode(IMJSON.self, from: body)
        guard message["id"] == intent["messageId"], message["conversationId"] == intent["conversationId"],
            message["contentVersion"] == intent["contentVersion"], message["deleted"] != .bool(true),
            message["forwarded"] != .bool(true), message["contentUnavailable"] != .bool(true), message["forwardUnavailable"] != .bool(true) else {
            throw StoreError.invalidIdentity
        }
        try database.write { db in
            try receiptStoreGuard(namespace: namespace, expected: expected, db: db)
            try Self.receiptMessageGuard(intent, namespace: namespace, db: db)
            if let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-intents' AND id=?",
                                          arguments: [namespace.key, intent["id"].text]) {
                guard try JSONDecoder.make().decode(IMJSON.self, from: raw) == intent else {
                    throw StoreError.conflictingIdentity
                }
                return
            }
            try Self.receiptMutation(previous: nil, next: intent, mutable: [])
            try Self.save(.object(["id": intent["id"], "messageId": intent["messageId"], "bodyDigest": intent["bodyDigest"],
                                   "bodyEnvelope": .string(bodyEnvelope)]), id: intent["id"].text, collection: "receipt-bodies", namespace: namespace.key, db: db)
            try Self.save(message, id: message["id"].text, collection: "messages:" + intent["conversationId"].text, namespace: namespace.key, db: db)
            try Self.save(intent, id: intent["id"].text, collection: "receipt-intents", namespace: namespace.key, db: db)
        }
    }

    private static func receiptDirectory(_ namespace: String, create: Bool) throws -> URL {
        guard namespace.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil else {
            throw StoreError.invalidIdentity
        }
        let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        if create, !FileManager.default.fileExists(atPath: support.path) {
            try FileManager.default.createDirectory(at: support, withIntermediateDirectories: true, attributes: [.posixPermissions: 0o700])
        }
        let root = support.appendingPathComponent("ReceiptObjects", isDirectory: true)
        let folder = root.appendingPathComponent(namespace, isDirectory: true)
        for directory in [root, folder] {
            var info = stat()
            if lstat(directory.path, &info) != 0 {
                guard errno == ENOENT, create else {
                    throw StoreError.conflictingIdentity
                }
                try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
                try receiptSyncDirectory(directory.deletingLastPathComponent())
                guard lstat(directory.path, &info) == 0 else {
                    throw StoreError.conflictingIdentity
                }
            }
            guard info.st_mode & mode_t(S_IFMT) == mode_t(S_IFDIR), info.st_uid == geteuid(), info.st_mode & 0o077 == 0 else {
                throw StoreError.conflictingIdentity
            }
        }
        return folder
    }

    private static func receiptSyncDirectory(_ directory: URL) throws {
        let fd = open(directory.path, O_RDONLY | O_CLOEXEC | O_NOFOLLOW)
        guard fd >= 0 else {
            throw StoreError.conflictingIdentity
        }
        defer {
            close(fd)
        }
        var info = stat()
        guard fstat(fd, &info) == 0, info.st_mode & mode_t(S_IFMT) == mode_t(S_IFDIR), fsync(fd) == 0 else {
            throw StoreError.conflictingIdentity
        }
    }

    private static func receiptFile(namespace: String, message: String, request: String, create: Bool) throws -> URL {
        guard receiptUUID(.string(message)), receiptUUID(.string(request)) else {
            throw StoreError.invalidIdentity
        }
        return try receiptDirectory(namespace, create: create).appendingPathComponent(message + "-" + request + ".object")
    }

    private static func receiptUnlink(_ file: URL, expected: (device: dev_t, inode: ino_t)? = nil) throws {
        var info = stat()
        if lstat(file.path, &info) != 0 {
            guard errno == ENOENT else {
                throw StoreError.conflictingIdentity
            }
            let parent = file.deletingLastPathComponent()
            var directory = stat()
            if lstat(parent.path, &directory) == 0 {
                try receiptSyncDirectory(parent)
            } else if errno == ENOENT {
                let root = parent.deletingLastPathComponent()
                if FileManager.default.fileExists(atPath: root.path) {
                    try receiptSyncDirectory(root)
                }
            } else {
                throw StoreError.conflictingIdentity
            }
            return
        }
        guard expected == nil || info.st_dev == expected?.device && info.st_ino == expected?.inode,
            info.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG), info.st_uid == geteuid(), info.st_nlink == 1, unlink(file.path) == 0 else {
            throw StoreError.conflictingIdentity
        }
        try receiptSyncDirectory(file.deletingLastPathComponent())
    }

    private static func receiptWriteFile(_ data: Data, file: URL) throws -> (device: dev_t, inode: ino_t) {
        let fd = open(file.path, O_WRONLY | O_CREAT | O_EXCL | O_CLOEXEC | O_NOFOLLOW, 0o600)
        guard fd >= 0 else {
            throw StoreError.conflictingIdentity
        }
        var complete = false
        defer {
            if !complete {
                var opened = stat()
                var named = stat()
                if fstat(fd, &opened) == 0, lstat(file.path, &named) == 0,
                    opened.st_ino == named.st_ino, opened.st_dev == named.st_dev {
                    _ = unlink(file.path)
                    try? receiptSyncDirectory(file.deletingLastPathComponent())
                }
            }
            close(fd)
        }
        try data.withUnsafeBytes { buffer in
            var written = 0
            while written < buffer.count {
                guard let address = buffer.baseAddress else {
                    throw StoreError.conflictingIdentity
                }
                let count = Darwin.write(fd, address.advanced(by: written), buffer.count - written)
                if count < 0, errno == EINTR {
                    continue
                }
                guard count > 0 else {
                    throw StoreError.conflictingIdentity
                }
                written += count
            }
        }
        guard fsync(fd) == 0 else {
            throw StoreError.conflictingIdentity
        }
        try receiptSyncDirectory(file.deletingLastPathComponent())
        var identity = stat()
        guard fstat(fd, &identity) == 0 else {
            throw StoreError.conflictingIdentity
        }
        complete = true
        return (identity.st_dev, identity.st_ino)
    }

    private static func receiptReadFile(_ file: URL, size: Int, digest: String) throws -> Data {
        let fd = open(file.path, O_RDWR | O_CLOEXEC | O_NOFOLLOW)
        guard fd >= 0 else {
            throw StoreError.conflictingIdentity
        }
        let handle = FileHandle(fileDescriptor: fd, closeOnDealloc: true)
        defer {
            try? handle.close()
        }
        var before = stat()
        guard fstat(fd, &before) == 0, before.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG), before.st_uid == geteuid(),
            before.st_nlink == 1, before.st_size == off_t(size), size >= 0, size <= 64 * 1024 * 1024 else {
            throw StoreError.conflictingIdentity
        }
        let data = try handle.read(upToCount: size + 1) ?? Data()
        var after = stat()
        var named = stat()
        guard data.count == size, receiptHash(data) == digest, fstat(fd, &after) == 0, before.st_ino == after.st_ino,
            before.st_dev == after.st_dev, after.st_size == before.st_size, after.st_nlink == 1,
            lstat(file.path, &named) == 0, named.st_ino == before.st_ino, named.st_dev == before.st_dev, fsync(fd) == 0 else {
            throw StoreError.conflictingIdentity
        }
        try receiptSyncDirectory(file.deletingLastPathComponent())
        return data
    }

    private static func receiptObjectParent(_ intent: IMJSON, namespace: IMNamespace, db: Database) throws {
        let parentID = intent["originalBody"]["parentReceiptId"].text
        guard let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-intents' AND id=?",
                                         arguments: [namespace.key, parentID]) else {
            throw StoreError.conflictingIdentity
        }
        let parent = try JSONDecoder.make().decode(IMJSON.self, from: raw)
        guard parent["kind"].text == "content", parent["state"].text == "completed" else {
            throw StoreError.conflictingIdentity
        }
        for field in ["familyId", "storeGeneration", "restoreEpoch", "schemaVersion", "messageId", "conversationId", "contentVersion",
                      "membershipGeneration", "qualificationId", "permissionGeneration", "deliveryGeneration", "manifestDigest", "bodyDigest"] {
            guard parent[field] == intent[field] else {
                throw StoreError.conflictingIdentity
            }
        }
        guard let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-bodies' AND id=?",
                                         arguments: [namespace.key, parentID]) else {
            throw StoreError.conflictingIdentity
        }
        let record = try JSONDecoder.make().decode(IMJSON.self, from: raw)
        let bytes = try receiptEnvelopeBytes(record["bodyEnvelope"].text)
        guard receiptHash(bytes) == intent["bodyDigest"].text else {
            throw StoreError.conflictingIdentity
        }
        let message = try JSONDecoder.make().decode(IMJSON.self, from: bytes)
        guard message["id"] == intent["messageId"], message["attachmentId"] == intent["originalBody"]["objectId"],
            message["contentVersion"] == intent["contentVersion"], ["image", "audio", "video", "file"].contains(message["type"].text),
            message["forwarded"] != .bool(true) else {
            throw StoreError.conflictingIdentity
        }
    }

    public func commitReceiptObject(namespace: IMNamespace, expected: IMJSON, intent: IMJSON, descriptor: IMJSON, bytes: Data) throws {
        try Self.receiptValidateIntent(intent, namespace: namespace, expected: expected)
        let body = intent["originalBody"]
        guard intent["kind"].text == "object", bytes.count <= 64 * 1024 * 1024,
            Set(descriptor.fields.keys) == Set(["objectId", "messageId", "sizeBytes", "sha256", "contentType"]),
            descriptor["objectId"] == body["objectId"], descriptor["messageId"] == intent["messageId"],
            descriptor["sizeBytes"] == body["sizeBytes"], body["sizeBytes"].text == String(bytes.count),
            descriptor["sha256"] == body["objectDigest"], Self.receiptHash(bytes) == descriptor["sha256"].text,
            case .string = descriptor["contentType"], !descriptor["contentType"].text.isEmpty,
            descriptor["contentType"].text.utf8.count <= 256 else {
            throw StoreError.invalidIdentity
        }
        // A committed preparation names every possible file before its first
        // byte exists. Recovery and global/account cleanup can therefore find it.
        let preparation: IMJSON = .object(["id": intent["id"], "messageId": intent["messageId"], "descriptor": descriptor])
        try database.write { db in
            try receiptStoreGuard(namespace: namespace, expected: expected, db: db)
            try Self.receiptMessageGuard(intent, namespace: namespace, db: db)
            try Self.receiptObjectParent(intent, namespace: namespace, db: db)
            if let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-file-preparations' AND id=?",
                                          arguments: [namespace.key, intent["id"].text]) {
                guard try JSONDecoder.make().decode(IMJSON.self, from: raw) == preparation else {
                    throw StoreError.conflictingIdentity
                }
            } else {
                try Self.save(preparation, id: intent["id"].text, collection: "receipt-file-preparations", namespace: namespace.key, db: db)
            }
        }
        var published = false
        var created: (file: URL, device: dev_t, inode: ino_t)?
        defer {
            if !published, let created {
                try? database.write { db in
                    try db.execute(sql: "UPDATE sync_metadata SET value=value WHERE id='storeGeneration'")
                    try Self.receiptUnlink(created.file, expected: (created.device, created.inode))
                }
            }
        }
        try database.write { db in
            try receiptStoreGuard(namespace: namespace, expected: expected, db: db)
            try Self.receiptMessageGuard(intent, namespace: namespace, db: db)
            try Self.receiptObjectParent(intent, namespace: namespace, db: db)
            let file = try Self.receiptFile(namespace: namespace.key, message: intent["messageId"].text, request: intent["id"].text, create: true)
            if let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-intents' AND id=?",
                                          arguments: [namespace.key, intent["id"].text]) {
                guard try JSONDecoder.make().decode(IMJSON.self, from: raw) == intent else {
                    throw StoreError.conflictingIdentity
                }
                _ = try Self.receiptReadFile(file, size: bytes.count, digest: descriptor["sha256"].text)
                return
            }
            try Self.receiptMutation(previous: nil, next: intent, mutable: [])
            let owned = try Self.receiptWriteFile(bytes, file: file)
            created = (file, owned.device, owned.inode)
            _ = try Self.receiptReadFile(file, size: bytes.count, digest: descriptor["sha256"].text)
            try Self.save(descriptor.setting("id", intent["id"]), id: intent["id"].text, collection: "receipt-objects", namespace: namespace.key, db: db)
            try Self.save(intent, id: intent["id"].text, collection: "receipt-intents", namespace: namespace.key, db: db)
            try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection='receipt-file-preparations' AND id=?", arguments: [namespace.key, intent["id"].text])
        }
        published = true
    }

    public func receiptObject(namespace: IMNamespace, requestId: String) throws -> Data {
        return try database.read { db in
            try Self.checkDeletionFloor(namespace, db: db)
            guard let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-objects' AND id=?",
                                             arguments: [namespace.key, requestId]) else {
                throw StoreError.invalidIdentity
            }
            let descriptor = try JSONDecoder.make().decode(IMJSON.self, from: raw)
            guard let size = Int(descriptor["sizeBytes"].text) else {
                throw StoreError.invalidIdentity
            }
            let denied = try Bool.fetchOne(db, sql: "SELECT EXISTS(SELECT 1 FROM sync_global WHERE profile=? AND scope=? AND principal=? AND source=? AND (id=? OR id=?))",
                arguments: [namespace.profile, namespace.deploymentID, namespace.account, namespace.sourceContextID, descriptor["messageId"].text, descriptor["objectId"].text]) ?? true
            guard !denied else {
                throw StoreError.conflictingIdentity
            }
            return try Self.receiptReadFile(Self.receiptFile(namespace: namespace.key, message: descriptor["messageId"].text, request: requestId, create: false),
                                            size: size, digest: descriptor["sha256"].text)
        }
    }

    public func recoverReceiptFiles(namespace: IMNamespace) throws {
        try drainReceiptFileCleanup(namespace: namespace)
        try database.write { db in
            try db.execute(sql: "UPDATE sync_metadata SET value=value WHERE id='storeGeneration'")
            try Self.checkDeletionFloor(namespace, db: db)
            for row in try Row.fetchAll(db, sql: "SELECT id,body FROM records WHERE namespace=? AND collection='receipt-file-preparations'", arguments: [namespace.key]) {
                let value = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
                let request: String = row["id"]
                let committed = try Bool.fetchOne(db, sql: "SELECT EXISTS(SELECT 1 FROM records WHERE namespace=? AND collection='receipt-objects' AND id=?)", arguments: [namespace.key, request]) ?? true
                guard Self.receiptUUID(value["messageId"]), Self.receiptUUID(.string(request)) else {
                    throw StoreError.invalidIdentity
                }
                if !committed {
                    let folder = try Self.receiptDirectory(namespace.key, create: true)
                    try Self.receiptUnlink(folder.appendingPathComponent(value["messageId"].text + "-" + request + ".object"))
                }
                try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection='receipt-file-preparations' AND id=?", arguments: [namespace.key, request])
            }
        }
    }

    private static func receiptLocation(namespace: String, message: String, request: String) throws -> URL {
        guard namespace.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil,
            receiptUUID(.string(message)), receiptUUID(.string(request)) else {
            throw StoreError.invalidIdentity
        }
        return FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("ReceiptObjects").appendingPathComponent(namespace)
            .appendingPathComponent(message + "-" + request + ".object")
    }

    private static func receiptQueueFile(namespace: String, message: String, request: String, db: Database) throws {
        let file = try receiptLocation(namespace: namespace, message: message, request: request)
        var info = stat()
        if lstat(file.path, &info) != 0 {
            guard errno == ENOENT else {
                throw StoreError.conflictingIdentity
            }
            return
        }
        _ = try receiptDirectory(namespace, create: false)
        guard info.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG), info.st_uid == geteuid(), info.st_nlink == 1,
            let inode = Int64(exactly: info.st_ino) else {
            throw StoreError.conflictingIdentity
        }
        try db.execute(sql: "INSERT OR IGNORE INTO receipt_file_cleanup VALUES(?,?,?,?,?)",
                       arguments: [namespace, request, message, Int64(info.st_dev), inode])
        guard let row = try Row.fetchOne(db, sql: "SELECT message_id,device,inode FROM receipt_file_cleanup WHERE namespace=? AND request_id=?",
                                        arguments: [namespace, request]), row["message_id"] as String == message,
            row["device"] as Int64 == Int64(info.st_dev), row["inode"] as Int64 == inode else {
            throw StoreError.conflictingIdentity
        }
    }

    public func drainReceiptFileCleanup(namespace: IMNamespace) throws {
        if namespace.demo {
            return
        }
        guard namespace.sourceContextID == "personal" else {
            throw StoreError.invalidIdentity
        }
        let rows = try database.read { db in
            return try Row.fetchAll(db, sql: """
                SELECT q.* FROM receipt_file_cleanup q JOIN account_namespace_registry n ON n.namespace=q.namespace
                WHERE n.profile=? AND n.scope=? AND n.principal=? AND n.source='personal'
                ORDER BY q.namespace,q.request_id
                """, arguments: [namespace.profile, namespace.deploymentID, namespace.account])
        }
        for row in rows {
            let key: String = row["namespace"]
            let request: String = row["request_id"]
            let message: String = row["message_id"]
            let device: Int64 = row["device"]
            let inode: Int64 = row["inode"]
            guard let originalDevice = dev_t(exactly: device), let originalInode = ino_t(exactly: inode) else {
                throw StoreError.invalidIdentity
            }
            try database.write { db in
                try db.execute(sql: "UPDATE sync_metadata SET value=value WHERE id='storeGeneration'")
                guard try Bool.fetchOne(db, sql: "SELECT EXISTS(SELECT 1 FROM receipt_file_cleanup WHERE namespace=? AND request_id=? AND message_id=? AND device=? AND inode=?)",
                    arguments: [key, request, message, device, inode]) == true else {
                    return
                }
                let file = try Self.receiptLocation(namespace: key, message: message, request: request)
                var current = stat()
                if lstat(file.path, &current) == 0 {
                    _ = try Self.receiptDirectory(key, create: false)
                } else if errno != ENOENT {
                    throw StoreError.conflictingIdentity
                }
                // The floor/task committed earlier. A writer fence prevents
                // managed retry/recovery from replacing this name mid-unlink.
                try Self.receiptUnlink(file, expected: (originalDevice, originalInode))
                try db.execute(sql: "DELETE FROM receipt_file_cleanup WHERE namespace=? AND request_id=? AND message_id=? AND device=? AND inode=?",
                               arguments: [key, request, message, device, inode])
            }
        }
    }

    private static func receiptEraseMessage(_ id: String, namespace: String, db: Database) throws {
        for collection in ["receipt-objects", "receipt-file-preparations"] {
            let rows = try Row.fetchAll(db, sql: "SELECT id,body FROM records WHERE namespace=? AND collection=? AND (json_extract(CAST(body AS TEXT),'$.messageId')=? OR json_extract(CAST(body AS TEXT),'$.objectId')=? OR json_extract(CAST(body AS TEXT),'$.descriptor.objectId')=?)", arguments: [namespace, collection, id, id, id])
            for row in rows {
                let value = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
                let request: String = row["id"]
                guard receiptUUID(value["messageId"]), receiptUUID(.string(request)) else {
                    throw StoreError.invalidIdentity
                }
                try receiptQueueFile(namespace: namespace, message: value["messageId"].text, request: request, db: db)
                try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?", arguments: [namespace, collection, request])
            }
        }
        try db.execute(sql: "DELETE FROM records WHERE namespace=? AND collection='receipt-bodies' AND json_extract(CAST(body AS TEXT),'$.messageId')=?", arguments: [namespace, id])
        for row in try Row.fetchAll(db, sql: "SELECT id,body FROM records WHERE namespace=? AND collection='receipt-intents' AND (json_extract(CAST(body AS TEXT),'$.messageId')=? OR json_extract(CAST(body AS TEXT),'$.originalBody.objectId')=?)", arguments: [namespace, id, id]) {
            let value = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
            try save(value.setting("state", .string("frozen")), id: row["id"], collection: "receipt-intents", namespace: namespace, db: db)
        }
    }

    public func verifyReceiptDurability(namespace: IMNamespace, expected: IMJSON, buildDigest: String, includeObjects: Bool) throws -> IMJSON {
        guard databasePath != ":memory:", !databasePath.contains("mode=memory"), Self.receiptDigest(.string(buildDigest)) else {
            throw StoreError.invalidIdentity
        }
        var information = stat()
        guard lstat(databasePath, &information) == 0, information.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG) else {
            throw StoreError.conflictingIdentity
        }
        let identifier = UUID().uuidString.lowercased()
        let probe: IMJSON = .object(["id": .string(identifier), "storeGeneration": expected["storeGeneration"],
                                    "bytes": .string(UUID().uuidString + "<&>བོད་"), "buildDigest": .string(buildDigest)])
        let encoded = try JSONEncoder.make().encode(probe)
        var file: URL?
        defer {
            if let file {
                try? Self.receiptUnlink(file)
            }
            try? database.write { db in
                try db.execute(sql: "DELETE FROM records WHERE namespace=? AND id=? AND collection='receipt-storage-probes'",
                               arguments: [namespace.key, identifier])
            }
        }
        try database.write { db in
            try receiptStoreGuard(namespace: namespace, expected: expected, db: db)
            try db.execute(sql: "INSERT INTO records(namespace,collection,id,body,ordering) VALUES(?,?,?,?,?)",
                           arguments: [namespace.key, "receipt-storage-probes", identifier, encoded, identifier])
            if includeObjects {
                try Self.save(.object(["id": .string(identifier), "messageId": .string(identifier)]), id: identifier,
                              collection: "receipt-file-preparations", namespace: namespace.key, db: db)
            }
        }
        // A separate connection reads the committed WAL bytes. This tests the
        // live storage path; it is not a power-loss or remote disk attestation.
        let independent = try DatabaseQueue(path: databasePath)
        try independent.read { db in
            guard let raw = try Data.fetchOne(db, sql: "SELECT body FROM records WHERE namespace=? AND collection='receipt-storage-probes' AND id=?",
                                             arguments: [namespace.key, identifier]), raw == encoded else {
                throw StoreError.conflictingIdentity
            }
        }
        if includeObjects {
            try database.write { db in
                try receiptStoreGuard(namespace: namespace, expected: expected, db: db)
                let destination = try Self.receiptFile(namespace: namespace.key, message: identifier, request: identifier, create: true)
                _ = try Self.receiptWriteFile(encoded, file: destination)
                file = destination
                guard try Self.receiptReadFile(destination, size: encoded.count, digest: Self.receiptHash(encoded)) == encoded else {
                    throw StoreError.conflictingIdentity
                }
            }
        }
        let checks: IMJSON = .object([
            "wal": .bool(true), "synchronousFull": .bool(true), "fullfsync": .bool(true), "checkpointFullfsync": .bool(true),
            "atomicCommit": .bool(true), "independentConnectionRead": .bool(true),
            "completeOriginalRead": .bool(includeObjects), "fileFsync": .bool(includeObjects), "directoryFsync": .bool(includeObjects),
        ])
        let contract = includeObjects ? "native_sqlite_full_file_fsync_v1" : "native_sqlite_full_v1"
        let stable: IMJSON = .object(["verificationVersion": .string("ios-receipt-durability-v1"), "buildDigest": .string(buildDigest),
                                     "storageContract": .string(contract), "checks": checks])
        let encoder = JSONEncoder.make()
        encoder.outputFormatting = [.sortedKeys]
        let digest = Self.receiptHash(try encoder.encode(stable))
        try database.write { db in
            try receiptStoreGuard(namespace: namespace, expected: expected, db: db)
        }
        if let ownedFile = file {
            try Self.receiptUnlink(ownedFile)
            file = nil
        }
        try database.write { db in
            try db.execute(sql: "DELETE FROM records WHERE namespace=? AND id=? AND collection IN('receipt-storage-probes','receipt-file-preparations')",
                           arguments: [namespace.key, identifier])
        }
        return stable.setting("verificationDigest", .string(digest)).setting("storeGeneration", expected["storeGeneration"])
            .setting("restoreEpoch", .string(namespace.restoreEpoch)).setting("guardGeneration", expected["guardGeneration"])
            .setting("verifiedAt", .string(ISO8601DateFormatter.make().string(from: Date())))
            .setting("qualifiedAck", .bool(false))
    }
}
