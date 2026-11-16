//
//  IMLocalStore.swift
//  JobsIMStorage
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import CryptoKit
import Foundation
import GRDB
import JobsIMCore

public final class IMLocalStore: @unchecked Sendable {

    private let database: DatabaseQueue

    public init(path: String) throws {
        var configuration = Configuration()
        configuration.prepareDatabase { db in
            try db.execute(sql: "PRAGMA journal_mode=WAL")
            try db.execute(sql: "PRAGMA synchronous=FULL")
            try db.execute(sql: "PRAGMA foreign_keys=ON")
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
        try migrations.migrate(database)
    }

    public func list(_ collection: String, namespace: IMNamespace) throws -> [IMJSON] {
        try database.read { db in
            try Row.fetchAll(
                db, sql: "SELECT body FROM records WHERE namespace=? AND collection=? ORDER BY ordering,id",
                arguments: [namespace.key, collection]
            ).map {
                try JSONDecoder.make().decode(IMJSON.self, from: $0["body"] as Data)
            }
        }
    }

    public func save(_ values: [IMJSON], collection: String, namespace: IMNamespace) throws {
        try database.write { db in
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
            try Row.fetchAll(
                db, sql: "SELECT body FROM outbox WHERE namespace=? AND status='queued' ORDER BY rowid",
                arguments: [namespace.key]
            ).map {
                try JSONDecoder.make().decode(IMJSON.self, from: $0["body"] as Data)
            }
        }
    }

    public func allOutbox(namespace: IMNamespace) throws -> [IMJSON] {
        return try database.read { db in
            return try Row.fetchAll(db, sql: "SELECT body FROM outbox WHERE namespace=? ORDER BY rowid", arguments: [namespace.key]).map { row in
                return try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
            }
        }
    }

    public func originalOutbox(clientID: String, namespace: IMNamespace) throws -> IMJSON? {
        return try database.read { db in
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
            try db.execute(
                sql: "DELETE FROM records WHERE namespace=? AND collection=? AND id=?",
                arguments: [namespace.key, collection, id])
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
            return try Self.syncRead(namespace.key, db: db)
        }
    }

    public func syncSelectSchema(namespace: IMNamespace, expected: IMJSON, schema: String) throws -> IMJSON {
        guard ["1", "2"].contains(schema) else {
            throw StoreError.invalidIdentity
        }
        return try database.write { db in
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
            try Self.syncCompare(expectedGuard, namespace: namespace.key, db: db)
            guard let raw = try Data.fetchOne(db,
                                             sql: "SELECT body FROM records WHERE namespace=? AND collection='self-state-operations' AND id=?",
                                             arguments: [namespace.key, original["id"].text]),
                try JSONDecoder.make().decode(IMJSON.self, from: raw) == original,
                original["body"]["revisionEpoch"].text == namespace.restoreEpoch,
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
            try Self.restrictSync(
                namespace.key, reason: reason, resetSnapshot: resetSnapshot, db: db)
        }
    }

    public func syncNamespaces(namespace: IMNamespace) throws -> [String] {
        return try database.read { db in
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
    }

    public func acknowledgeOriginal(_ original: IMJSON, message: IMJSON, namespace: IMNamespace)
        throws
    {
        try database.write { db in
            try Self.syncAcknowledge(
                .object(["original": original, "message": message]), namespace: namespace, db: db)
        }
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
            try db.execute(
                sql: "DELETE FROM sync_eligibility WHERE namespace=? AND id=?",
                arguments: [holder, id])
            for row in try Row.fetchAll(
                db, sql: "SELECT collection,id,body FROM records WHERE namespace=?",
                arguments: [holder])
            {
                let body = try JSONDecoder.make().decode(IMJSON.self, from: row["body"] as Data)
                let rowID: String = row["id"]
                let collection: String = row["collection"]
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

    public enum StoreError: Error {
        case invalidIdentity, conflictingIdentity
    }
}
