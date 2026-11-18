package com.jobs.im

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class StorageLifecycleTest {
    @Test
    fun versionTwoUpgradePreservesOriginalPendingIdentity() = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val name = "JobsIM-test-" + UUID.randomUUID() + ".db"
        val file = context.getDatabasePath(name)
        file.parentFile!!.mkdirs()
        android.database.sqlite.SQLiteDatabase.openOrCreateDatabase(file, null).use { old ->
            old.execSQL(
                "CREATE TABLE IF NOT EXISTS `messages` (`key` TEXT NOT NULL, `namespace` TEXT NOT NULL, `conversationId` TEXT NOT NULL, `id` TEXT NOT NULL, `clientMsgId` TEXT NOT NULL, `senderId` TEXT NOT NULL, `type` TEXT NOT NULL, `text` TEXT NOT NULL, `attachmentId` TEXT NOT NULL, `replyToId` TEXT NOT NULL, `state` TEXT NOT NULL, `createdAt` INTEGER NOT NULL, `pinned` INTEGER NOT NULL, `favorite` INTEGER NOT NULL, `reaction` TEXT NOT NULL, `version` TEXT NOT NULL, `edited` INTEGER NOT NULL, `retracted` INTEGER NOT NULL, `restoreEpoch` TEXT NOT NULL DEFAULT '1', PRIMARY KEY(`key`))"
            )
            old.execSQL(
                "CREATE UNIQUE INDEX IF NOT EXISTS `index_messages_namespace_conversationId_id` ON `messages` (`namespace`, `conversationId`, `id`)"
            )
            old.execSQL(
                "INSERT INTO messages VALUES('original-key','legacy','room','original-id','original-client','me','text','retained original','','','pending',1,0,0,'','1',0,0,'1')"
            )
            old.version = 2
        }
        val upgraded = ChatDatabase.open(context, false, name)
        try {
            val pending = upgraded.messages().pending("legacy").single()
            assertEquals("original-client", pending.clientMsgId)
            assertEquals("retained original", pending.text)
            assertEquals("1", pending.restoreEpoch)
            assertEquals("0", pending.sequence)
            assertFalse(pending.contentUnavailable)
            assertEquals("", pending.sendAdmission)
            assertEquals("", pending.membershipGeneration)
            upgraded.messages().saveDraft("legacy", "room", "1", "new draft", 1)
            assertEquals("new draft", upgraded.messages().state("legacy", "room")!!.draft)
        } finally {
            upgraded.close()
            context.deleteDatabase(name)
        }
    }

    @Test
    fun anonymousReconnectFreezesEveryAccountOfOnlyTheOldInstanceEpoch() = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val name = "JobsIM-test-" + UUID.randomUUID() + ".db"
        val database = ChatDatabase.open(context, false, name)
        try {
            val dao = database.messages()
            val scopes =
                listOf(
                    "live|profile|deployment|alice|1",
                    "live|profile|deployment|bob|1",
                    "live|profile|deployment|alice|2",
                    "live|other-profile|deployment|alice|1",
                )
            scopes.forEachIndexed { index, ns ->
                dao.put(
                    MessageRow(
                        ns + "|m",
                        ns,
                        "room",
                        "m",
                        clientMsgId = "original-$index",
                        senderId = "alice",
                        text = "retained",
                        state = "pending",
                    )
                )
            }
            dao.freezeScope("live|profile|deployment|", "|1")
            assertEquals("frozen", dao.one(scopes[0], "m")!!.state)
            assertEquals("frozen", dao.one(scopes[1], "m")!!.state)
            assertEquals("original-0", dao.one(scopes[0], "m")!!.clientMsgId)
            assertEquals("retained", dao.one(scopes[0], "m")!!.text)
            assertEquals("pending", dao.one(scopes[2], "m")!!.state)
            assertEquals("pending", dao.one(scopes[3], "m")!!.state)
        } finally {
            database.close()
            context.deleteDatabase(name)
        }
    }

    @Test
    fun durableQueueDraftRevisionAndCursorRemainAtomic() = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val database = ChatDatabase.open(context, false)
        val ns = "storage-" + UUID.randomUUID()
        val dao = database.messages()
        val epoch = "1"
        val conversation = "room"
        try {
            dao.saveDraft(ns, conversation, epoch, "first draft", 1)
            dao.saveDraft(ns, conversation, epoch, "newer draft", 2)
            val pending =
                MessageRow(
                    "$ns|original",
                    ns,
                    conversation,
                    "original",
                    clientMsgId = "original",
                    senderId = "me",
                    text = "first draft",
                    state = "pending",
                )
            dao.enqueue(pending, 1)
            assertEquals("newer draft", dao.state(ns, conversation)!!.draft)
            assertEquals(1, dao.pending(ns).size)
            val current =
                pending.copy(
                    key = "$ns|accepted",
                    id = "accepted",
                    state = "received",
                    version = "9007199254740993000",
                    sequence = "9007199254740993000",
                )
            dao.page(ns, conversation, epoch, "1:9007199254740993000", listOf(current), "me", false)
            // Sync/history does not prove the original body or admission was accepted.
            assertEquals(pending, dao.pending(ns).single())
            assertEquals("newer draft", dao.state(ns, conversation)!!.draft)
            val bad = current.copy(key = "$ns|foreign", id = "foreign", conversationId = "foreign")
            try {
                dao.page(
                    ns,
                    conversation,
                    epoch,
                    "1:9007199254740993001",
                    listOf(current.copy(version = "9007199254740993001"), bad),
                    "me",
                    false,
                )
                fail("Foreign page accepted")
            } catch (_: IllegalArgumentException) {}
            assertEquals("9007199254740993000", dao.one(ns, "accepted")!!.version)
            assertEquals("1:9007199254740993000", dao.state(ns, conversation)!!.cursor)
            val fresh =
                MessageRow(
                    "$ns|other",
                    ns,
                    conversation,
                    "other",
                    senderId = "peer",
                    text = "new",
                    version = "1",
                    sequence = "9007199254740993001",
                )
            dao.page(ns, conversation, epoch, "1:9007199254740993001", listOf(fresh), "me", false)
            assertEquals(1, dao.state(ns, conversation)!!.unread)
            dao.page(ns, conversation, epoch, "1:9007199254740993001", listOf(fresh), "me", false)
            assertEquals(1, dao.state(ns, conversation)!!.unread)
            assertTrue(dao.list(ns + "-another-account", conversation).isEmpty())
            dao.page(ns, conversation, epoch, "1:9007199254740993001", emptyList(), "me", true)
            assertEquals(0, dao.state(ns, conversation)!!.unread)
        } finally {
            database.close()
        }
        val reopened = ChatDatabase.open(context, false)
        try {
            assertEquals("newer draft", reopened.messages().state(ns, conversation)!!.draft)
            assertEquals(3, reopened.messages().list(ns, conversation).size)
        } finally {
            reopened.close()
        }
    }
}
