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
    indices = [Index(value = ["namespace", "conversationId", "id"], unique = true)],
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
        "DELETE FROM messages WHERE namespace=:namespace AND clientMsgId=:client AND state IN ('pending','frozen')"
    )
    abstract suspend fun acknowledge(namespace: String, client: String)

    @Query(
        "SELECT * FROM conversation_state WHERE namespace=:namespace AND conversationId=:conversation LIMIT 1"
    )
    abstract suspend fun state(namespace: String, conversation: String): ConversationState?

    @Query("SELECT * FROM conversation_state WHERE namespace=:namespace")
    abstract suspend fun states(namespace: String): List<ConversationState>

    @Query(
        "UPDATE conversation_state SET unread=0 WHERE namespace=:namespace AND conversationId=:conversation"
    )
    abstract suspend fun markRead(namespace: String, conversation: String)

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
        for (row in rows) {
            require(
                row.namespace == namespace &&
                    row.conversationId == conversation &&
                    row.restoreEpoch == epoch
            ) {
                "Foreign message page"
            }
            val previous = one(namespace, row.id)
            if (previous != null && BigInteger(previous.version) > BigInteger(row.version)) continue
            if (previous == null && row.senderId != me && !row.retracted && !row.contentUnavailable)
                fresh.add(row)
            if (row.clientMsgId.isNotBlank()) acknowledge(namespace, row.clientMsgId)
            put(row)
        }
        putState(old.copy(cursor = cursor, unread = if (visible) 0 else old.unread + fresh.size))
        return fresh
    }
}

@Database(
    entities =
        [MessageRow::class, ConversationState::class, LocalOperation::class, NativeScope::class],
    version = 8,
    exportSchema = true,
)
abstract class ChatDatabase : RoomDatabase() {
    abstract fun messages(): Messages

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
