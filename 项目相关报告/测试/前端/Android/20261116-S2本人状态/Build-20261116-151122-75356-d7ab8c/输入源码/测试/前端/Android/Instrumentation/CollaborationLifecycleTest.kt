package com.jobs.im

import android.app.Application
import android.util.Base64
import androidx.lifecycle.ViewModelStore
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.math.BigInteger
import java.net.SocketTimeoutException
import java.util.UUID
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class CollaborationLifecycleTest {
    @Test
    fun realMentionAndAnnouncementOperationsKeepOriginalMetadataAndCurrentContentReference() =
        runBlocking {
            val instrumentation = InstrumentationRegistry.getInstrumentation()
            val args = InstrumentationRegistry.getArguments()
            val base = args.getString("liveUrl")
            val encoded = args.getString("caBase64")
            assumeTrue(!base.isNullOrBlank() && !encoded.isNullOrBlank())
            val ca = Base64.decode(encoded, Base64.DEFAULT)
            val anonymous = Api(base!!, ca = ca)
            val boot = anonymous.request("/bootstrap")
            assertEquals(args.getString("deploymentId"), boot.getString("deploymentId"))
            assertTrue(boot.getJSONObject("features").getBoolean("mentions"))
            assertTrue(boot.getJSONObject("features").getBoolean("announcements"))
            assertTrue(boot.getJSONObject("features").getBoolean("role_admission"))
            val app = instrumentation.targetContext.applicationContext as Application
            app.getSharedPreferences("JobsIM.settings", 0)
                .edit()
                .putString("deployment", "")
                .putString("profile", UUID.randomUUID().toString())
                .commit()
            val suffix = UUID.randomUUID().toString().replace("-", "").take(10)
            val password = "Collaboration fixture " + UUID.randomUUID()
            val peer =
                anonymous.request(
                    "/auth/register",
                    "POST",
                    JSONObject()
                        .put("username", "android_collab_peer_" + suffix)
                        .put("password", password)
                        .put("requestId", UUID.randomUUID())
                        .put("deviceName", "Collaboration peer"),
                )
            val peerId = peer.getJSONObject("user").getString("id")
            val peerApi = Api(base, peer.getString("token"), ca)
            lateinit var model: ChatModel
            val lostMention = AtomicBoolean(true)
            val lostAnnouncement = AtomicBoolean(true)
            val mentionBodies = mutableListOf<String>()
            val announcementBodies = mutableListOf<String>()
            val factory = { address: String, token: String, trust: ByteArray? ->
                object : Api(address, token, trust) {
                    override suspend fun request(
                        path: String,
                        method: String,
                        data: JSONObject?,
                    ): JSONObject {
                        if (
                            method == "POST" &&
                                path.endsWith("/messages") &&
                                data!!.has("mentionedUserIds")
                        ) {
                            mentionBodies.add(data.toString())
                            val db = ChatDatabase.open(app, false)
                            try {
                                val ns =
                                    Isolation.namespace(
                                        model.profile,
                                        model.deployment,
                                        model.userId,
                                        false,
                                        model.restoreEpoch,
                                    )
                                val row = db.messages().one(ns, data.getString("clientMsgId"))!!
                                assertTrue(
                                    row.mentionedUserIds ==
                                        data.getJSONArray("mentionedUserIds").toString()
                                )
                            } finally {
                                db.close()
                            }
                        }
                        if (path.endsWith("/announcement") && method == "PUT") {
                            announcementBodies.add(data.toString())
                            val db = ChatDatabase.open(app, false)
                            try {
                                val ns =
                                    Isolation.namespace(
                                        model.profile,
                                        model.deployment,
                                        model.userId,
                                        false,
                                        model.restoreEpoch,
                                    )
                                val op =
                                    db.messages()
                                        .operation(
                                            ns,
                                            "announcement-" +
                                                path
                                                    .substringAfter("/conversations/")
                                                    .substringBefore('/'),
                                        )!!
                                assertTrue(
                                    JSONObject(op.payload).getJSONObject("body").toString() ==
                                        data!!.toString()
                                )
                            } finally {
                                db.close()
                            }
                        }
                        val result = super.request(path, method, data)
                        if (
                            method == "POST" &&
                                path.endsWith("/messages") &&
                                data!!.has("mentionedUserIds") &&
                                lostMention.compareAndSet(true, false)
                        )
                            throw SocketTimeoutException("TEST_MENTION_RESPONSE_LOST")
                        if (
                            method == "PUT" &&
                                path.endsWith("/announcement") &&
                                lostAnnouncement.compareAndSet(true, false)
                        )
                            throw SocketTimeoutException("TEST_ANNOUNCEMENT_RESPONSE_LOST")
                        return result
                    }
                }
            }
            val holder = ViewModelStore()
            withContext(Dispatchers.Main) {
                model = ChatModel(app, factory)
                holder.put("fixture", model)
            }
            suspend fun perform(action: () -> Job) {
                withContext(Dispatchers.Main) { action() }.join()
            }
            suspend fun success(action: () -> Job) {
                perform(action)
                assertEquals("", model.error)
            }
            suspend fun announcement(action: () -> Job) {
                perform(action)
                assertEquals("", model.announcement.problem)
            }
            var selfApi: Api? = null
            try {
                success { model.connect(base, boot.getString("deploymentId"), ca) {} }
                success { model.authenticate("register", "android_collab_" + suffix, password) }
                val me = model.userId
                val api = Api(base, model.token, ca)
                selfApi = api
                val request =
                    peerApi.request(
                        "/contacts/requests",
                        "POST",
                        JSONObject().put("userId", me).put("requestId", UUID.randomUUID()),
                    )
                success { model.resolveRequest(request.getString("id"), true) }
                success { model.create("Native collaboration", listOf(peerId), true) }
                val group = model.selected!!
                val ns =
                    Isolation.namespace(
                        model.profile,
                        model.deployment,
                        me,
                        false,
                        model.restoreEpoch,
                    )
                success { model.loadMembers() }
                withContext(Dispatchers.Main) {
                    model.updateDraft("Directed native mention")
                    model.setDraftMentions(listOf(peerId), false)
                }
                perform { model.send(model.draft) }
                assertTrue(model.error.isNotEmpty())
                val database = ChatDatabase.open(app, false)
                val pending =
                    try {
                        database.messages().pending(ns).single()
                    } finally {
                        database.close()
                    }
                assertTrue(pending.mentionedUserIds == JSONArray(listOf(peerId)).toString())
                success { model.retry(pending) }
                assertEquals(2, mentionBodies.size)
                assertTrue(mentionBodies[0] == mentionBodies[1])
                val history =
                    peerApi
                        .request("/conversations/${group.id}/messages?after=0&limit=200")
                        .getJSONArray("items")
                val mentioned =
                    (0 until history.length())
                        .map { history.getJSONObject(it) }
                        .single { it.getString("clientMsgId") == pending.clientMsgId }
                assertTrue(mentioned.getBoolean("mentionedMe"))
                assertFalse(model.draftMentionAll)
                assertTrue(model.draftMentionIds.isEmpty())
                withContext(Dispatchers.Main) {
                    model.updateDraft("Durable mention draft")
                    model.setDraftMentions(listOf(peerId), false)
                }
                val draftDb = ChatDatabase.open(app, false)
                try {
                    withTimeout(5000) {
                        while (
                            draftDb.messages().state(ns, group.id)?.draftMentions !=
                                JSONArray(listOf(peerId)).toString()
                        ) delay(20)
                    }
                } finally {
                    draftDb.close()
                }
                withContext(Dispatchers.Main) {
                    model = ChatModel(app, factory)
                    holder.put("fixture", model)
                }
                withTimeout(10000) { while (model.userId.isBlank()) delay(50) }
                success { model.refresh() }
                success { model.open(model.conversations.single { it.id == group.id }) }
                success { model.loadMembers() }
                assertEquals("Durable mention draft", model.draft)
                assertTrue(model.draftMentionIds == listOf(peerId))
                withContext(Dispatchers.Main) {
                    model.setDraftMentions(emptyList(), false)
                    model.updateDraft("Native announcement source")
                }
                success { model.send(model.draft) }
                val originalSource =
                    model.messages.single { it.text == "Native announcement source" }
                api.request("/messages/${originalSource.id}/favorite", "PUT")
                success { model.refresh() }
                val source = model.messages.single { it.id == originalSource.id }
                assertTrue(BigInteger(source.version) > BigInteger(source.contentVersion))
                announcement { model.announcement.load(model.selected!!) }
                assertEquals("empty", model.announcement.current!!.state)
                announcement { model.announcement.choose(source) }
                perform { model.announcement.publish(1) }
                assertTrue(model.announcement.problem.isNotEmpty())
                assertTrue(model.announcement.pending != null)
                val oldOp = model.announcement.pending!!
                val oldBody = JSONObject(oldOp.payload).getJSONObject("body")
                assertTrue(oldBody.getString("sourceVersion") == source.contentVersion)
                val active = api.request("/conversations/${group.id}/announcement")
                assertEquals("active", active.getString("state"))
                assertTrue(active.getJSONObject("message").getString("id") == source.id)
                val latestGroup = conversationSnapshot(api, group.id)
                val nextSource =
                    api.request(
                        "/conversations/${group.id}/messages",
                        "POST",
                        JSONObject()
                            .put("clientMsgId", UUID.randomUUID())
                            .put("restoreEpoch", model.restoreEpoch)
                            .put("sendAdmission", latestGroup.getString("sendAdmission"))
                            .put("type", "text")
                            .put("text", "Later announcement current source"),
                    )
                api.request(
                    "/conversations/${group.id}/announcement",
                    "PUT",
                    JSONObject()
                        .put("messageId", nextSource.getString("id"))
                        .put("sourceVersion", nextSource.getString("contentVersion"))
                        .put("expectedVersion", active.getString("version"))
                        .put("membershipGeneration", latestGroup.getString("membershipGeneration"))
                        .put("roleGeneration", latestGroup.getString("roleGeneration"))
                        .put("requestId", UUID.randomUUID()),
                )
                announcement { model.announcement.retry() }
                assertEquals(2, announcementBodies.size)
                assertTrue(announcementBodies[0] == announcementBodies[1])
                assertEquals(nextSource.getString("id"), model.announcement.current!!.source!!.id)
                assertNull(model.announcement.pending)
                val inspectDb = ChatDatabase.open(app, false)
                try {
                    inspectDb.messages().putOperation(oldOp)
                } finally {
                    inspectDb.close()
                }
                announcement { model.announcement.reload() }
                val beforeInspect = announcementBodies.size
                announcement { model.announcement.inspect() }
                assertEquals(beforeInspect, announcementBodies.size)
                assertEquals(nextSource.getString("id"), model.announcement.current!!.source!!.id)
                api.request("/messages/${nextSource.getString("id")}/favorite", "PUT")
                announcement { model.announcement.reload() }
                assertEquals("active", model.announcement.current!!.state)
                val currentSource = api.request("/messages/${nextSource.getString("id")}")
                val edited =
                    api.request(
                        "/messages/${nextSource.getString("id")}",
                        "PATCH",
                        JSONObject()
                            .put("text", "Edited announcement source")
                            .put("expectedVersion", currentSource.getString("version"))
                            .put("requestId", UUID.randomUUID()),
                    )
                announcement { model.announcement.reload() }
                assertEquals("unavailable", model.announcement.current!!.state)
                assertNull(model.announcement.current!!.source)
                announcement { model.announcement.choose(message(edited, ns, model.restoreEpoch)) }
                announcement { model.announcement.publish() }
                assertEquals(
                    "Edited announcement source",
                    model.announcement.current!!.source!!.text,
                )
                announcement { model.announcement.clear() }
                assertEquals("empty", model.announcement.current!!.state)
                assertNull(model.announcement.current!!.source)
                api.request(
                    "/conversations/${group.id}",
                    "PATCH",
                    JSONObject().put("ownerId", peerId),
                )
                success { model.refresh() }
                var denied = false
                withContext(Dispatchers.Main) {
                    try {
                        model.setDraftMentions(emptyList(), true)
                    } catch (e: IllegalArgumentException) {
                        denied = true
                    }
                }
                assertTrue(denied)
                val peerGroup = conversationSnapshot(peerApi, group.id)
                peerApi.request(
                    "/conversations/${group.id}/messages",
                    "POST",
                    JSONObject()
                        .put("clientMsgId", UUID.randomUUID())
                        .put("restoreEpoch", model.restoreEpoch)
                        .put("sendAdmission", peerGroup.getString("sendAdmission"))
                        .put("type", "text")
                        .put("text", "Owner mentions everyone")
                        .put("mentionAll", true),
                )
                success { model.refresh() }
                assertTrue(
                    model.messages.single { it.text == "Owner mentions everyone" }.mentionedMe
                )
            } finally {
                withContext(Dispatchers.Main) { holder.clear() }
                selfApi?.request("/auth/logout", "POST", JSONObject())
                peerApi.request("/auth/logout", "POST", JSONObject())
            }
        }
}
