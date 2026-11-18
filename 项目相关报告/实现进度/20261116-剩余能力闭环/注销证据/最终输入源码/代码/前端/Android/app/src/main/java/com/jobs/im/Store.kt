package com.jobs.im

import android.content.Context
import androidx.room.*
import androidx.sqlite.SQLiteConnection
import androidx.sqlite.driver.bundled.BundledSQLiteDriver
import java.math.BigInteger
import java.net.URI
import java.security.MessageDigest

object Isolation {
    fun namespace(
        profile: String,
        deployment: String,
        user: String,
        demo: Boolean,
        epoch: String = "1",
        sourceContext: String = "",
        device: String = "",
    ): String {
        val parts = mutableListOf(if (demo) "demo" else "live", profile, deployment, user)
        if (!demo && (sourceContext.isNotEmpty() || device.isNotEmpty())) {
            require(sourceContext == "personal" && device.isNotBlank())
            parts.add(sourceContext)
            parts.add(device)
        }
        parts.add(epoch)
        return parts.joinToString("|")
    }

    fun digest(value: String): String =
        MessageDigest.getInstance("SHA-256").digest(value.toByteArray()).joinToString("") {
            "%02x".format(it)
        }

    fun endpoint(raw: String): String {
        val uri = URI(raw.trim())
        require(uri.scheme == "https" && !uri.host.isNullOrBlank()) { "HTTPS required" }
        require(uri.userInfo == null && uri.query == null && uri.fragment == null) {
            "URL must not contain credentials or query"
        }
        return raw.trim().trimEnd('/')
    }

    fun mayTransmit(
        row: MessageRow,
        namespace: String,
        demo: Boolean,
        epoch: String = "1",
    ): Boolean =
        !demo && row.namespace == namespace && row.state == "pending" && row.restoreEpoch == epoch

    fun cursor(value: String, epoch: String): BigInteger {
        val parts = value.split(':')
        require(parts.size == 2 && parts[0] == epoch && parts[1].matches(Regex("[0-9]+"))) {
            "Invalid cursor epoch"
        }
        return BigInteger(parts[1])
    }
}

@Entity(
    tableName = "messages",
    indices = [Index(value = ["namespace", "conversationId", "id"], unique = true), Index(value = ["id"])],
)
data class MessageRow(
    @PrimaryKey val key: String,
    val namespace: String,
    val conversationId: String,
    val id: String,
    val clientMsgId: String = "",
    val senderId: String,
    val type: String = "text",
    val text: String,
    val attachmentId: String = "",
    val replyToId: String = "",
    val state: String = "received",
    val createdAt: Long = System.currentTimeMillis(),
    val pinned: Boolean = false,
    val favorite: Boolean = false,
    val reaction: String = "",
    val version: String = "1",
    val edited: Boolean = false,
    val retracted: Boolean = false,
    @ColumnInfo(defaultValue = "'1'") val restoreEpoch: String = "1",
    @ColumnInfo(defaultValue = "'0'") val sequence: String = "0",
    @ColumnInfo(defaultValue = "0") val contentUnavailable: Boolean = false,
    @ColumnInfo(defaultValue = "''") val membershipGeneration: String = "",
    @ColumnInfo(defaultValue = "''") val sendAdmission: String = "",
    @ColumnInfo(defaultValue = "'[]'") val mentionedUserIds: String = "[]",
    @ColumnInfo(defaultValue = "0") val mentionAll: Boolean = false,
    @ColumnInfo(defaultValue = "0") val mentionedMe: Boolean = false,
    @ColumnInfo(defaultValue = "''") val contentVersion: String = "",
    @ColumnInfo(defaultValue = "0") val forwarded: Boolean = false,
    @ColumnInfo(defaultValue = "0") val forwardUnavailable: Boolean = false,
    @ColumnInfo(defaultValue = "''") val forwardAdmission: String = "",
    @ColumnInfo(defaultValue = "''") val attachmentURL: String = "",
    @ColumnInfo(defaultValue = "''") val forwardPayload: String = "",
    @ColumnInfo(defaultValue = "''") val projectionGuard: String = "",
)

@Entity(tableName = "conversation_state")
data class ConversationState(
    @PrimaryKey val key: String,
    val namespace: String,
    val conversationId: String,
    val cursor: String,
    val draft: String = "",
    val unread: Int = 0,
    @ColumnInfo(defaultValue = "0") val revision: Long = 0,
    @ColumnInfo(defaultValue = "'[]'") val draftMentions: String = "[]",
    @ColumnInfo(defaultValue = "0") val draftMentionAll: Boolean = false,
)

@Entity(tableName = "operations")
data class LocalOperation(
    @PrimaryKey val key: String,
    val namespace: String,
    val purpose: String,
    val requestId: String,
    val payload: String,
    val filePath: String = "",
    val restoreEpoch: String,
)

@Entity(tableName = "native_scopes")
data class NativeScope(
    @PrimaryKey val key: String,
    val prefix: String,
    val namespace: String,
    val deviceId: String,
    val reason: String,
)

@Dao
abstract class Messages {
    @Query("SELECT namespace FROM messages WHERE substr(namespace,1,length(:prefix))=:prefix UNION SELECT namespace FROM conversation_state WHERE substr(namespace,1,length(:prefix))=:prefix UNION SELECT namespace FROM operations WHERE substr(namespace,1,length(:prefix))=:prefix UNION SELECT namespace FROM sync_records WHERE substr(namespace,1,length(:prefix))=:prefix UNION SELECT namespace FROM native_scopes WHERE substr(namespace,1,length(:prefix))=:prefix")
    abstract suspend fun accountNamespaces(prefix: String): List<String>
    @Query("DELETE FROM messages WHERE substr(namespace,1,length(:prefix))=:prefix")
    abstract suspend fun eraseAccountMessages(prefix: String)
    @Query("DELETE FROM conversation_state WHERE substr(namespace,1,length(:prefix))=:prefix")
    abstract suspend fun eraseAccountStates(prefix: String)
    @Query("DELETE FROM operations WHERE substr(namespace,1,length(:prefix))=:prefix")
    abstract suspend fun eraseAccountOperations(prefix: String)
    @Query("DELETE FROM sync_records WHERE substr(namespace,1,length(:prefix))=:prefix")
    abstract suspend fun eraseAccountSync(prefix: String)
    @Query("DELETE FROM native_scopes WHERE substr(namespace,1,length(:prefix))=:prefix")
    abstract suspend fun eraseAccountScopes(prefix: String)
    @Query("SELECT * FROM messages WHERE namespace=:namespace")
    abstract suspend fun all(namespace: String): List<MessageRow>

    @Query("SELECT * FROM messages WHERE namespace=:namespace AND state IN ('pending','frozen') ORDER BY createdAt")
    abstract suspend fun outbox(namespace: String): List<MessageRow>

    @Query("DELETE FROM messages WHERE namespace=:namespace AND state NOT IN ('pending','frozen')")
    abstract suspend fun clearCloud(namespace: String)

    @Query("SELECT * FROM messages WHERE substr(namespace,1,length(:prefix))=:prefix AND id=:id")
    abstract suspend fun across(prefix: String, id: String): List<MessageRow>

    @Query("SELECT * FROM messages WHERE substr(namespace,1,length(:prefix))=:prefix AND forwarded=1 AND state NOT IN ('pending','frozen')")
    abstract suspend fun forwardHolders(prefix: String): List<MessageRow>

    @Query("SELECT payload FROM sync_records WHERE `key`=:key LIMIT 1")
    abstract suspend fun syncPayload(key: String): String?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun putSyncRecord(row: SyncRecord)

    @Query("SELECT * FROM sync_records WHERE kind='unread' AND substr(namespace,1,length(:prefix))=:prefix")
    abstract suspend fun syncUnreadAcross(prefix: String): List<SyncRecord>

    @Query("SELECT * FROM native_scopes WHERE substr(:namespace,1,length(prefix))=prefix LIMIT 1")
    abstract suspend fun nativeScope(namespace: String): NativeScope?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun putNativeScope(scope: NativeScope)

    @Query(
        "UPDATE messages SET state='frozen' WHERE substr(namespace,1,length(:prefix))=:prefix AND namespace!=:active AND state='pending'"
    )
    abstract suspend fun retireOtherNativeScopes(prefix: String, active: String)

    @Transaction
    open suspend fun adoptNative(session: NativeSession) {
        val b = session.binding
        val prefix = "live|${b.profile}|${b.deployment}|"
        val active =
            Isolation.namespace(
                b.profile,
                b.deployment,
                session.userId,
                false,
                b.epoch,
                session.sourceContextId,
                session.deviceId,
            )
        retireOtherNativeScopes(prefix, active)
        putNativeScope(
            NativeScope(
                Isolation.digest(b.profile + "|" + b.origin + "|" + b.deployment),
                prefix,
                active,
                session.deviceId,
                "SESSION_DEVICE_CHANGED",
            )
        )
    }

    @Transaction
    open suspend fun retireNative(binding: SessionBinding) {
        val prefix = "live|${binding.profile}|${binding.deployment}|"
        retireOtherNativeScopes(prefix, "")
        putNativeScope(
            NativeScope(
                Isolation.digest(binding.profile + "|" + binding.origin + "|" + binding.deployment),
                prefix,
                "",
                "",
                "SESSION_ENDED",
            )
        )
    }

    @Query(
        "SELECT * FROM messages WHERE namespace=:namespace AND conversationId=:conversation ORDER BY createdAt, LENGTH(sequence), sequence, id"
    )
    abstract suspend fun list(namespace: String, conversation: String): List<MessageRow>

    @Query("SELECT * FROM messages WHERE namespace=:namespace AND id=:id LIMIT 1")
    abstract suspend fun one(namespace: String, id: String): MessageRow?

    @Query(
        "SELECT * FROM messages WHERE namespace=:namespace AND state='pending' ORDER BY createdAt"
    )
    abstract suspend fun pending(namespace: String): List<MessageRow>

    @Query(
        "UPDATE messages SET state='frozen' WHERE state='pending' AND substr(namespace,1,length(:prefix))=:prefix AND substr(namespace,-length(:suffix))=:suffix"
    )
    abstract suspend fun freezeScope(prefix: String, suffix: String)

    @Query(
        "UPDATE messages SET state='frozen' WHERE namespace=:namespace AND id=:id AND state='pending'"
    )
    abstract suspend fun freeze(namespace: String, id: String)

    @Insert(onConflict = OnConflictStrategy.REPLACE) abstract suspend fun put(row: MessageRow)

    @Query("DELETE FROM messages WHERE namespace=:namespace AND id=:id")
    abstract suspend fun remove(namespace: String, id: String)

    @Query(
        "SELECT * FROM conversation_state WHERE namespace=:namespace AND conversationId=:conversation LIMIT 1"
    )
    abstract suspend fun state(namespace: String, conversation: String): ConversationState?

    @Query("SELECT * FROM conversation_state WHERE namespace=:namespace")
    abstract suspend fun states(namespace: String): List<ConversationState>

    @Query(
        "UPDATE conversation_state SET unread=0 WHERE namespace=:namespace AND conversationId=:conversation"
    )
    abstract suspend fun clearUnreadCount(namespace: String, conversation: String)

    @Query("DELETE FROM sync_records WHERE `key`=:key")
    abstract suspend fun clearSyncUnread(key: String)

    @Transaction
    open suspend fun markRead(namespace: String, conversation: String) {
        clearUnreadCount(namespace, conversation)
        clearSyncUnread("$namespace|sync-unread|$conversation")
    }

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun putState(row: ConversationState)

    @Query("SELECT * FROM operations WHERE namespace=:namespace AND purpose=:purpose LIMIT 1")
    abstract suspend fun operation(namespace: String, purpose: String): LocalOperation?

    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun putOperation(row: LocalOperation)

    @Transaction
    open suspend fun prepareOperation(row: LocalOperation): LocalOperation {
        val original = operation(row.namespace, row.purpose)
        if (original != null) return original
        putOperation(row)
        return row
    }

    @Transaction
    open suspend fun archiveOperation(row: LocalOperation) {
        val current = operation(row.namespace, row.purpose)
        if (current?.requestId != row.requestId) return
        putOperation(
            row.copy(
                key = row.namespace + "|announcement-history-" + row.requestId,
                purpose = "announcement-history-" + row.requestId,
            )
        )
        clearOperation(row.namespace, row.purpose, row.requestId)
    }

    @Query(
        "DELETE FROM operations WHERE namespace=:namespace AND purpose=:purpose AND requestId=:requestId"
    )
    abstract suspend fun clearOperation(namespace: String, purpose: String, requestId: String)

    @Transaction
    open suspend fun saveDraft(
        namespace: String,
        conversation: String,
        epoch: String,
        text: String,
        revision: Long,
        mentions: String = "[]",
        mentionAll: Boolean = false,
    ) {
        val old =
            state(namespace, conversation)
                ?: ConversationState(
                    namespace + "|" + conversation,
                    namespace,
                    conversation,
                    "$epoch:0",
                )
        if (revision >= old.revision)
            putState(
                old.copy(
                    draft = text,
                    revision = revision,
                    draftMentions = mentions,
                    draftMentionAll = mentionAll,
                )
            )
    }

    @Transaction
    open suspend fun enqueue(row: MessageRow, expectedDraftRevision: Long = -1) {
        val scope = nativeScope(row.namespace)
        val retired = scope != null && scope.namespace != row.namespace
        put(if (retired) row.copy(state = "frozen") else row)
        val state =
            state(row.namespace, row.conversationId)
                ?: ConversationState(
                    row.namespace + "|" + row.conversationId,
                    row.namespace,
                    row.conversationId,
                    row.restoreEpoch + ":0",
                )
        putState(
            state.copy(
                draft =
                    if (
                        expectedDraftRevision >= 0 &&
                            !retired &&
                            state.revision == expectedDraftRevision
                    )
                        ""
                    else state.draft,
                draftMentions =
                    if (
                        expectedDraftRevision >= 0 &&
                            !retired &&
                            state.revision == expectedDraftRevision
                    )
                        "[]"
                    else state.draftMentions,
                draftMentionAll =
                    if (
                        expectedDraftRevision >= 0 &&
                            !retired &&
                            state.revision == expectedDraftRevision
                    )
                        false
                    else state.draftMentionAll,
            )
        )
    }

    @Transaction
    open suspend fun page(
        namespace: String,
        conversation: String,
        epoch: String,
        cursor: String,
        rows: List<MessageRow>,
        me: String,
        visible: Boolean,
    ): List<MessageRow> {
        val sync = syncPayload("$namespace|sync-control")?.let { org.json.JSONObject(it) }
        if (sync != null && (sync.has("restriction") || !sync.has("cursor") || rows.any { it.projectionGuard != sync.optString("guardGeneration") }))
            throw SyncFailure("SYNC_LOCAL_RESTRICTION_CHANGED")
        val old =
            state(namespace, conversation)
                ?: ConversationState(
                    namespace + "|" + conversation,
                    namespace,
                    conversation,
                    epoch + ":0",
                )
        require(Isolation.cursor(cursor, epoch) >= Isolation.cursor(old.cursor, epoch)) {
            "Cursor regression"
        }
        val fresh = mutableListOf<MessageRow>()
        val globals = mutableSetOf<String>()
        for (row in rows) {
            require(
                row.namespace == namespace &&
                    row.conversationId == conversation &&
                    row.restoreEpoch == epoch
            ) {
                "Foreign message page"
            }
            val previous = one(namespace, row.id)
            val knownGlobal = syncPayload(namespace.split('|').take(4).joinToString("|") + "|personal|global|" + row.id) != null
            if ((previous?.retracted == true || knownGlobal) && !row.retracted) continue
            if (previous != null && !row.retracted && BigInteger(previous.version) > BigInteger(row.version)) continue
            if (previous == null && row.senderId != me && !row.retracted && !row.contentUnavailable)
                fresh.add(row)
            if (row.retracted && row.state !in setOf("pending", "frozen")) globals.add(row.id)
            put(if (row.retracted) globalMessagePlaceholder(row)
                else if (row.forwarded && row.state !in setOf("pending", "frozen")) Forwarding.hidden(row) else row)
        }
        putState(old.copy(cursor = cursor, unread = if (visible) 0 else old.unread + fresh.size))
        if (sync != null && globals.isNotEmpty()) {
            val prefix = namespace.split('|').take(4).joinToString("|") + "|"
            val scope = prefix + "personal"
            for (id in globals) {
                val body = org.json.JSONObject().put("id", id).toString()
                putSyncRecord(SyncRecord("$scope|global|$id", scope, "global", body))
                putSyncRecord(SyncRecord("$scope|cleanup|$id", scope, "cleanup", body))
                across(prefix, id).filter { it.state !in setOf("pending", "frozen") }.forEach { put(globalMessagePlaceholder(it)) }
            }
            forwardHolders(prefix).forEach { put(if (it.retracted) globalMessagePlaceholder(it) else Forwarding.hidden(it)) }
            for (entry in syncUnreadAcross(prefix)) {
                val payload = org.json.JSONObject(entry.payload)
                val previousIds = payload.optJSONArray("ids") ?: continue
                val ids = (0 until previousIds.length()).map { previousIds.getString(it) }.filter { it !in globals }
                if (ids.size == previousIds.length()) continue
                putSyncRecord(entry.copy(payload = payload.put("ids", org.json.JSONArray(ids)).toString()))
                val conversationId = entry.key.substringAfterLast('|')
                state(entry.namespace, conversationId)?.let { putState(it.copy(unread = payload.optInt("legacy", 0) + ids.size)) }
            }
            sync.put("guardGeneration", java.util.UUID.randomUUID().toString())
            putSyncRecord(SyncRecord("$namespace|sync-control", namespace, "control", sync.toString()))
        }
        return fresh
    }
}

@Database(
    entities =
        [MessageRow::class, ConversationState::class, LocalOperation::class, NativeScope::class, SyncRecord::class],
    version = 9,
    exportSchema = true,
)
abstract class ChatDatabase : RoomDatabase() {
    abstract fun messages(): Messages
    abstract fun syncRecords(): SyncRecords

    // Room's legacy extension requires SupportSQLiteOpenHelper; keep every DAO on the configured driver writer.
    suspend fun <T> withTransaction(block: suspend () -> T): T =
        useWriterConnection { writer ->
            writer.withTransaction(Transactor.SQLiteTransactionType.IMMEDIATE) { block() }
        }

    companion object {
        fun open(context: Context, demo: Boolean, testName: String? = null): ChatDatabase =
            Room.databaseBuilder(
                    context,
                    ChatDatabase::class.java,
                    testName?.also { require(it.startsWith("JobsIM-test-")) }
                        ?: if (demo) "JobsIM-demo.db" else "JobsIM-live.db",
                )
                .setJournalMode(JournalMode.WRITE_AHEAD_LOGGING)
                .setDriver(BundledSQLiteDriver())
                .addMigrations(
                    object : androidx.room.migration.Migration(1, 2) {
                        override fun migrate(connection: SQLiteConnection) {
                            connection
                                .prepare(
                                    "ALTER TABLE messages ADD COLUMN restoreEpoch TEXT NOT NULL DEFAULT '1'"
                                )
                                .use { it.step() }
                        }
                    },
                    object : androidx.room.migration.Migration(2, 3) {
                        override fun migrate(connection: SQLiteConnection) {
                            listOf(
                                    "ALTER TABLE messages ADD COLUMN sequence TEXT NOT NULL DEFAULT '0'",
                                    "ALTER TABLE messages ADD COLUMN contentUnavailable INTEGER NOT NULL DEFAULT 0",
                                    "CREATE TABLE IF NOT EXISTS conversation_state (`key` TEXT NOT NULL PRIMARY KEY, namespace TEXT NOT NULL, conversationId TEXT NOT NULL, cursor TEXT NOT NULL, draft TEXT NOT NULL, unread INTEGER NOT NULL)",
                                    "CREATE TABLE IF NOT EXISTS operations (`key` TEXT NOT NULL PRIMARY KEY, namespace TEXT NOT NULL, purpose TEXT NOT NULL, requestId TEXT NOT NULL, payload TEXT NOT NULL, filePath TEXT NOT NULL, restoreEpoch TEXT NOT NULL)",
                                )
                                .forEach { sql -> connection.prepare(sql).use { it.step() } }
                        }
                    },
                    object : androidx.room.migration.Migration(3, 4) {
                        override fun migrate(connection: SQLiteConnection) {
                            connection
                                .prepare(
                                    "ALTER TABLE conversation_state ADD COLUMN revision INTEGER NOT NULL DEFAULT 0"
                                )
                                .use { it.step() }
                        }
                    },
                    object : androidx.room.migration.Migration(4, 5) {
                        override fun migrate(connection: SQLiteConnection) {
                            listOf(
                                    "ALTER TABLE messages ADD COLUMN membershipGeneration TEXT NOT NULL DEFAULT ''",
                                    "ALTER TABLE messages ADD COLUMN sendAdmission TEXT NOT NULL DEFAULT ''",
                                )
                                .forEach { sql -> connection.prepare(sql).use { it.step() } }
                        }
                    },
                    object : androidx.room.migration.Migration(5, 6) {
                        override fun migrate(connection: SQLiteConnection) {
                            listOf(
                                    "ALTER TABLE messages ADD COLUMN mentionedUserIds TEXT NOT NULL DEFAULT '[]'",
                                    "ALTER TABLE messages ADD COLUMN mentionAll INTEGER NOT NULL DEFAULT 0",
                                    "ALTER TABLE messages ADD COLUMN mentionedMe INTEGER NOT NULL DEFAULT 0",
                                    "ALTER TABLE messages ADD COLUMN contentVersion TEXT NOT NULL DEFAULT ''",
                                    "ALTER TABLE conversation_state ADD COLUMN draftMentions TEXT NOT NULL DEFAULT '[]'",
                                    "ALTER TABLE conversation_state ADD COLUMN draftMentionAll INTEGER NOT NULL DEFAULT 0",
                                )
                                .forEach { sql -> connection.prepare(sql).use { it.step() } }
                        }
                    },
                    object : androidx.room.migration.Migration(6, 7) {
                        override fun migrate(connection: SQLiteConnection) {
                            listOf(
                                    "ALTER TABLE messages ADD COLUMN forwarded INTEGER NOT NULL DEFAULT 0",
                                    "ALTER TABLE messages ADD COLUMN forwardUnavailable INTEGER NOT NULL DEFAULT 0",
                                    "ALTER TABLE messages ADD COLUMN forwardAdmission TEXT NOT NULL DEFAULT ''",
                                    "ALTER TABLE messages ADD COLUMN attachmentURL TEXT NOT NULL DEFAULT ''",
                                    "ALTER TABLE messages ADD COLUMN forwardPayload TEXT NOT NULL DEFAULT ''",
                                )
                                .forEach { sql -> connection.prepare(sql).use { it.step() } }
                        }
                    },
                    object : androidx.room.migration.Migration(7, 8) {
                        override fun migrate(connection: SQLiteConnection) {
                            connection
                                .prepare(
                                    "CREATE TABLE IF NOT EXISTS native_scopes (`key` TEXT NOT NULL PRIMARY KEY, prefix TEXT NOT NULL, namespace TEXT NOT NULL, deviceId TEXT NOT NULL, reason TEXT NOT NULL)"
                                )
                                .use { it.step() }
                        }
                    },
                    object : androidx.room.migration.Migration(8, 9) {
                        override fun migrate(connection: SQLiteConnection) {
                            connection.prepare("ALTER TABLE messages ADD COLUMN projectionGuard TEXT NOT NULL DEFAULT ''").use { it.step() }
                            connection.prepare("CREATE INDEX IF NOT EXISTS index_messages_id ON messages (id)").use { it.step() }
                            connection.prepare("UPDATE messages SET text='',attachmentId='',attachmentURL='',forwardAdmission='',replyToId='',mentionedUserIds='[]',mentionAll=0,mentionedMe=0,forwardUnavailable=1 WHERE forwarded=1 AND state NOT IN ('pending','frozen')").use { it.step() }
                            connection.prepare("CREATE TABLE IF NOT EXISTS sync_records (`key` TEXT NOT NULL PRIMARY KEY, namespace TEXT NOT NULL, kind TEXT NOT NULL, payload TEXT NOT NULL)").use { it.step() }
                            connection.prepare("CREATE INDEX IF NOT EXISTS index_sync_records_namespace_kind ON sync_records (namespace, kind)").use { it.step() }
                            connection.prepare("CREATE INDEX IF NOT EXISTS index_sync_records_kind ON sync_records (kind)").use { it.step() }
                        }
                    },
                )
                .setQueryCoroutineContext(kotlinx.coroutines.Dispatchers.IO)
                .addCallback(
                    object : Callback() {
                        override fun onOpen(connection: SQLiteConnection) {
                            connection.prepare("PRAGMA synchronous=FULL").use { it.step() }
                        }
                    }
                )
                .build()
    }
}
