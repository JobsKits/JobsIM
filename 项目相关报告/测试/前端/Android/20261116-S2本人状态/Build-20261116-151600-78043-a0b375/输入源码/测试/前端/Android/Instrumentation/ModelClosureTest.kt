package com.jobs.im

import android.app.Application
import android.graphics.Bitmap
import android.net.Uri
import android.util.Base64
import androidx.lifecycle.ViewModelStore
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.net.SocketTimeoutException
import java.util.UUID
import java.util.concurrent.atomic.AtomicBoolean
import kotlinx.coroutines.*
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ModelClosureTest {
    @Test
    fun realNativeModelKeepsOriginalOperationsAndRelationshipGenerations() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val args = InstrumentationRegistry.getArguments()
        val base = args.getString("liveUrl")
        val encoded = args.getString("caBase64")
        assumeTrue(!base.isNullOrEmpty() && !encoded.isNullOrEmpty())
        val ca = Base64.decode(encoded, Base64.DEFAULT)
        val anonymous = Api(base!!, ca = ca)
        val bootstrap = anonymous.request("/bootstrap")
        assertEquals(args.getString("deploymentId"), bootstrap.getString("deploymentId"))
        val app = instrumentation.targetContext.applicationContext as Application
        val prefs = app.getSharedPreferences("JobsIM.settings", 0)
        prefs
            .edit()
            .putString("deployment", "")
            .putString("profile", UUID.randomUUID().toString())
            .commit()
        val suffix = UUID.randomUUID().toString().replace("-", "").take(12)
        val password = "Model fixture " + UUID.randomUUID()
        val peer =
            anonymous.request(
                "/auth/register",
                "POST",
                JSONObject()
                    .put("username", "android_peer_" + suffix)
                    .put("password", password)
                    .put("requestId", UUID.randomUUID())
                    .put("deviceName", "Android model peer"),
            )
        val peerId = peer.getJSONObject("user").getString("id")
        val peerApi = Api(base, peer.getString("token"), ca)
        peerApi.request("/me", "PATCH", JSONObject().put("displayName", "Native model peer"))
        lateinit var model: ChatModel
        val lostMessage = AtomicBoolean(true)
        val lostAvatar = AtomicBoolean(true)
        val lostReport = AtomicBoolean(true)
        val messageIds = mutableListOf<String>()
        val avatarIds = mutableListOf<String>()
        val reportIds = mutableListOf<String>()
        val holdNextMessage = AtomicBoolean(false)
        val submitted = CompletableDeferred<Unit>()
        val releaseMessage = CompletableDeferred<Unit>()
        val factory = { address: String, token: String, trust: ByteArray? ->
            object : Api(address, token, trust) {
                override suspend fun request(
                    path: String,
                    method: String,
                    data: JSONObject?,
                ): JSONObject {
                    if (method == "POST" && path.endsWith("/messages")) {
                        val id = data!!.getString("clientMsgId")
                        messageIds.add(id)
                        val database = ChatDatabase.open(app, false)
                        try {
                            val ns =
                                Isolation.namespace(
                                    model.profile,
                                    model.deployment,
                                    model.userId,
                                    false,
                                    model.restoreEpoch,
                                )
                            assertTrue(database.messages().pending(ns).any { it.clientMsgId == id })
                        } finally {
                            database.close()
                        }
                    }
                    if (method == "POST" && path == "/reports")
                        reportIds.add(data!!.getString("requestId"))
                    val result = super.request(path, method, data)
                    if (
                        method == "POST" &&
                            path.endsWith("/messages") &&
                            holdNextMessage.compareAndSet(true, false)
                    ) {
                        submitted.complete(Unit)
                        releaseMessage.await()
                    }
                    if (
                        method == "POST" &&
                            path.endsWith("/messages") &&
                            lostMessage.compareAndSet(true, false)
                    )
                        throw SocketTimeoutException("TEST_RESPONSE_LOST")
                    if (
                        method == "POST" &&
                            path == "/reports" &&
                            lostReport.compareAndSet(true, false)
                    )
                        throw SocketTimeoutException("TEST_RESPONSE_LOST")
                    return result
                }

                override suspend fun upload(
                    name: String,
                    mime: String,
                    bytes: ByteArray,
                    path: String,
                    fields: Map<String, String>,
                ): JSONObject {
                    if (path == "/me/avatar") avatarIds.add(fields.getValue("requestId"))
                    val result = super.upload(name, mime, bytes, path, fields)
                    if (path == "/me/avatar" && lostAvatar.compareAndSet(true, false))
                        throw SocketTimeoutException("TEST_RESPONSE_LOST")
                    return result
                }
            }
        }
        val holder = ViewModelStore()
        withContext(Dispatchers.Main) {
            model = ChatModel(app, factory)
            holder.put("fixture", model)
        }
        suspend fun perform(block: () -> Job) {
            withContext(Dispatchers.Main) { block() }.join()
        }
        suspend fun success(block: () -> Job) {
            perform(block)
            assertEquals("", model.error)
        }
        var avatarSource: File? = null
        try {
            success { model.connect(base, bootstrap.getString("deploymentId"), ca) {} }
            success { model.authenticate("register", "android_model_" + suffix, password) }
            val me = model.userId
            assertTrue(me.isNotBlank())
            val request =
                peerApi.request(
                    "/contacts/requests",
                    "POST",
                    JSONObject().put("userId", me).put("requestId", UUID.randomUUID()),
                )
            success { model.refresh() }
            assertEquals("Native model peer", model.requests.single().name)
            success { model.resolveRequest(request.getString("id"), true) }
            assertTrue(model.requests.isEmpty())
            assertTrue(model.contacts.any { it.id == peerId })
            success { model.loadRelationship(peerId) }
            assertTrue(model.relationships[peerId]!!.friend)
            success { model.relationship(peerId, "remove") }
            assertFalse(model.relationships[peerId]!!.friend)
            assertEquals("open", model.relationships[peerId]!!.inboundGate)
            success { model.relationship(peerId, "block") }
            assertTrue(model.relationships[peerId]!!.blockedByMe)
            success { model.relationship(peerId, "unblock") }
            assertFalse(model.relationships[peerId]!!.blockedByMe)
            assertEquals("denied", model.relationships[peerId]!!.inboundGate)
            success { model.relationship(peerId, "reopen") }
            assertEquals("open", model.relationships[peerId]!!.inboundGate)
            assertFalse(model.relationships[peerId]!!.friend)
            val newRequest =
                peerApi.request(
                    "/contacts/requests",
                    "POST",
                    JSONObject().put("userId", me).put("requestId", UUID.randomUUID()),
                )
            success { model.refresh() }
            assertEquals(1, model.requests.size)
            success { model.resolveRequest(newRequest.getString("id"), true) }
            success { model.create("Native model group", listOf(peerId), true) }
            val group = model.selected!!
            withContext(Dispatchers.Main) { model.updateDraft("Durable original draft") }
            delay(150)
            perform { model.send("Durable original draft") }
            assertTrue(model.error.isNotEmpty())
            val database = ChatDatabase.open(app, false)
            val ns =
                Isolation.namespace(model.profile, model.deployment, me, false, model.restoreEpoch)
            val original =
                try {
                    database.messages().pending(ns).single()
                } finally {
                    database.close()
                }
            assertEquals("", model.draft)
            success { model.retry(original) }
            assertEquals(2, messageIds.size)
            assertEquals(messageIds[0], messageIds[1])
            val history =
                peerApi
                    .request("/conversations/${group.id}/messages?after=0&limit=200")
                    .getJSONArray("items")
            assertEquals(
                1,
                (0 until history.length()).count {
                    history.getJSONObject(it).getString("clientMsgId") == original.clientMsgId
                },
            )
            holdNextMessage.set(true)
            withContext(Dispatchers.Main) { model.updateDraft("Second outgoing fixture") }
            val secondSend = withContext(Dispatchers.Main) { model.send(model.draft) }
            withTimeout(10000) { submitted.await() }
            withContext(Dispatchers.Main) { model.updateDraft("New draft while sending") }
            releaseMessage.complete(Unit)
            secondSend.join()
            assertEquals("", model.error)
            assertEquals("New draft while sending", model.draft)
            repeat(4) {
                coroutineScope {
                    val pollingRefresh = launch(Dispatchers.Main) { model.reload() }
                    val opening = withContext(Dispatchers.Main) { model.open(group) }
                    opening.join()
                    pollingRefresh.join()
                }
                assertEquals(model.messages.size, model.messages.map { it.key }.toSet().size)
                assertTrue(
                    model.messages.all { it.namespace == ns && it.conversationId == group.id }
                )
                assertEquals(2, model.messages.size)
                assertEquals("New draft while sending", model.draft)
            }
            peerApi.request(
                "/conversations/${group.id}/messages",
                "POST",
                JSONObject()
                    .put(
                        "sendAdmission",
                        conversationSnapshot(peerApi, group.id).getString("sendAdmission"),
                    )
                    .put("clientMsgId", UUID.randomUUID())
                    .put("restoreEpoch", model.restoreEpoch)
                    .put("type", "text")
                    .put("text", "Background unread fixture"),
            )
            withContext(Dispatchers.Main) { model.foreground = false }
            success { model.open(group) }
            assertEquals(1, model.unread[group.id])
            withContext(Dispatchers.Main) { model.foreground = true }
            success { model.refresh() }
            assertEquals(0, model.unread[group.id])
            val before = model.messages.size
            withContext(Dispatchers.Main) {
                model.selected = model.selected!!.copy(readOnly = true)
            }
            perform { model.send("Must not queue in read-only group") }
            assertTrue(model.error.isNotEmpty())
            assertEquals(before, model.messages.size)
            withContext(Dispatchers.Main) {
                model.selected = model.selected!!.copy(readOnly = false)
            }
            success { model.loadMembers() }
            assertEquals(2, model.members.size)
            val peerMember = model.members.single { it.id == peerId }
            val oldMembership = model.members.single { it.id == me }
            success { model.groupChange("rename", "Native renamed group") }
            assertEquals("Native renamed group", model.selected!!.title)
            success { model.groupChange("transfer", member = peerMember) }
            assertEquals(peerId, model.selected!!.ownerId)
            success { model.groupChange("leave") }
            assertNull(model.selected)
            peerApi.request(
                "/conversations/${group.id}/members",
                "POST",
                JSONObject().put("userId", me),
            )
            val stale = runCatching {
                peerApi.request(
                    "/conversations/${group.id}/members/$me?membershipGeneration=${oldMembership.membershipGeneration}",
                    "DELETE",
                )
            }
                .exceptionOrNull()
            assertTrue(stale is ApiFailure && stale.status == 409)
            val file = File(app.cacheDir, "avatar-$suffix.png")
            avatarSource = file
            file.outputStream().use {
                Bitmap.createBitmap(32, 32, Bitmap.Config.ARGB_8888)
                    .apply { eraseColor(android.graphics.Color.rgb(40, 124, 102)) }
                    .compress(Bitmap.CompressFormat.PNG, 100, it)
            }
            perform { model.publishAvatar(app, Uri.fromFile(file)) }
            assertTrue(model.error.isNotEmpty())
            assertTrue(
                "Avatar intent missing after unknown result: " + model.error,
                model.avatarPending,
            )
            val originalDatabase = ChatDatabase.open(app, false)
            val oldAvatar =
                try {
                    originalDatabase.messages().operation(ns, "avatar")!!
                } finally {
                    originalDatabase.close()
                }
            success { model.publishAvatar(app) }
            assertEquals(2, avatarIds.size)
            assertEquals(avatarIds[0], avatarIds[1])
            assertFalse(model.avatarPending)
            assertTrue(model.me!!.avatar.isNotBlank())
            success { model.publishAvatar(app, Uri.fromFile(file)) }
            val currentAvatar = model.me!!.avatar
            val historical =
                Api(base, model.token, ca).request("/me/avatar/operations/" + oldAvatar.requestId)
            assertFalse(historical.getBoolean("current"))
            val historicalDatabase = ChatDatabase.open(app, false)
            try {
                historicalDatabase.messages().putOperation(oldAvatar)
            } finally {
                historicalDatabase.close()
            }
            success { model.inspectAvatar() }
            assertEquals(currentAvatar, model.me!!.avatar)
            perform { model.report("user", peerId, "Native report original operation") }
            assertTrue(model.error.isNotEmpty())
            assertTrue(model.pendingReport != null)
            success { model.report("user", peerId, "Native report original operation") }
            assertEquals(2, reportIds.size)
            assertEquals(reportIds[0], reportIds[1])
            success { model.loadReports() }
            assertEquals(1, model.reports.count { it.reason == "Native report original operation" })
            val attachmentBytes = "Native authenticated download fixture".toByteArray()
            val attachment = peerApi.upload("fixture.txt", "text/plain", attachmentBytes)
            val fileMessage =
                peerApi.request(
                    "/conversations/${group.id}/messages",
                    "POST",
                    JSONObject()
                        .put(
                            "sendAdmission",
                            conversationSnapshot(peerApi, group.id).getString("sendAdmission"),
                        )
                        .put("clientMsgId", UUID.randomUUID().toString())
                        .put("restoreEpoch", model.restoreEpoch)
                        .put("type", "file")
                        .put("text", "fixture.txt")
                        .put("attachmentId", attachment.getString("id")),
                )
            val fileRow = message(fileMessage, ns, model.restoreEpoch)
            var received: ByteArray? = null
            success {
                model.download(fileRow) { file, mime ->
                    received = file.readBytes()
                    assertTrue(mime.startsWith("text/plain"))
                }
            }
            assertArrayEquals(attachmentBytes, received)
            success { model.loadDevices() }
            assertTrue(model.devices.isNotEmpty())
            val otherSession =
                anonymous.request(
                    "/auth/login",
                    "POST",
                    JSONObject()
                        .put("username", model.username)
                        .put("password", password)
                        .put("requestId", UUID.randomUUID().toString())
                        .put("deviceName", "Other fixture device"),
                )
            success { model.loadDevices() }
            success { model.revokeDevice(otherSession.getString("deviceId")) }
            assertTrue(
                runCatching {
                    Api(base, otherSession.getString("token"), ca).request("/auth/context")
                }
                    .exceptionOrNull() is ApiFailure
            )
            withContext(Dispatchers.Main) { holder.put("fixture", ChatModel(app)) }
            withContext(Dispatchers.Main) { model = holder.get("fixture") as ChatModel }
            withTimeout(10000) { while (model.userId.isBlank()) delay(100) }
            assertEquals(me, model.userId)
            assertTrue(model.me != null)
        } finally {
            avatarSource?.delete()
            perform { model.logout() }
            withContext(Dispatchers.Main) { holder.clear() }
            peerApi.request("/auth/logout", "POST", JSONObject())
        }
    }
}
