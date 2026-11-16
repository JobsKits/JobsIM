package com.jobs.im

import android.app.Application
import android.util.Base64
import androidx.lifecycle.ViewModelStore
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.net.SocketTimeoutException
import java.util.UUID
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ForwardLifecycleTest {
    private suspend fun verifyTemporary200Projection(
        model: ChatModel,
        api: Api,
        ns: String,
        target: Conversation,
        sourceId: String,
        setTemporary: (Boolean) -> Unit,
    ) {
        val app =
            InstrumentationRegistry.getInstrumentation().targetContext.applicationContext
                as Application
        suspend fun perform(action: () -> Job) {
            withContext(Dispatchers.Main) { action() }.join()
        }
        suspend fun success(action: () -> Job) {
            perform(action)
            assertEquals("", model.error)
        }
        val preciseBefore200 = message(api.request("/messages/$sourceId"), ns, model.restoreEpoch)
        setTemporary(true)
        success { model.refresh() }
        val placeholder200 = model.messages.single { it.id == sourceId }
        assertTrue(placeholder200.forwardUnavailable)
        assertEquals(preciseBefore200.version, placeholder200.version)
        for (lane in listOf("history", "ack")) {
            val database200 = ChatDatabase.open(app, false)
            try {
                if (lane == "history")
                    database200
                        .messages()
                        .page(
                            ns,
                            target.id,
                            model.restoreEpoch,
                            database200.messages().state(ns, target.id)!!.cursor,
                            listOf(preciseBefore200),
                            model.userId,
                            false,
                        )
                else database200.messages().put(preciseBefore200)
            } finally {
                database200.close()
            }
            perform { model.send("200 projection redraw barrier") }
            assertTrue(model.error.isNotBlank())
            assertTrue(
                model.messages
                    .filter { it.forwarded }
                    .all { it.forwardUnavailable && it.text.isBlank() }
            )
            val cleanup200 = ChatDatabase.open(app, false)
            try {
                cleanup200
                    .messages()
                    .pending(ns)
                    .filter { it.text == "200 projection redraw barrier" }
                    .forEach { cleanup200.messages().remove(ns, it.id) }
            } finally {
                cleanup200.close()
            }
        }
        setTemporary(false)
        success { model.refresh() }
        val restored200 = model.messages.single { it.id == sourceId }
        assertEquals(placeholder200.version, restored200.version)
        assertEquals(preciseBefore200.text, restored200.text)
        assertFalse(restored200.forwardUnavailable)
    }

    @Test
    fun originalForwardSurvivesUnknownRepliesWithoutNewProofAndScrubsLostSources() =
        runBlocking<Unit> {
            val instrumentation = InstrumentationRegistry.getInstrumentation()
            val args = InstrumentationRegistry.getArguments()
            val base = args.getString("liveUrl")
            val encoded = args.getString("caBase64")
            assumeTrue(!base.isNullOrBlank() && !encoded.isNullOrBlank())
            val ca = Base64.decode(encoded, Base64.DEFAULT)
            val anonymous = Api(base!!, ca = ca)
            val boot = anonymous.request("/bootstrap")
            assertEquals(args.getString("deploymentId"), boot.getString("deploymentId"))
            assertTrue(boot.getJSONObject("features").getBoolean("forwarding"))
            assertFalse(boot.getJSONObject("features").getBoolean("text_forward"))
            val suffix = UUID.randomUUID().toString().replace("-", "").take(10)
            val password = "Forward fixture " + UUID.randomUUID()
            val peer =
                anonymous.request(
                    "/auth/register",
                    "POST",
                    JSONObject()
                        .put("username", "android_forward_peer_" + suffix)
                        .put("password", password)
                        .put("requestId", UUID.randomUUID())
                        .put("deviceName", "Forward peer"),
                )
            val peerId = peer.getJSONObject("user").getString("id")
            val bob = Api(base, peer.getString("token"), ca)
            val app = instrumentation.targetContext.applicationContext as Application
            app.getSharedPreferences("JobsIM.settings", 0)
                .edit()
                .putString("deployment", "")
                .putString("profile", UUID.randomUUID().toString())
                .commit()
            lateinit var model: ChatModel
            var offlineBefore = false
            var loseAfter = false
            var hideFlag = false
            var temporaryProjection = false
            var temporary200Projection = false
            var denySourceConversation = false
            var assertHiddenBeforeMemberLoad = false
            var forwardPosts = 0
            var resultChecks = 0
            val bodies = mutableListOf<String>()
            var lastQueued: MessageRow? = null
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
                                data?.optString("text") == "200 projection redraw barrier"
                        ) {
                            val projections = model.messages.filter { it.forwarded }
                            assertTrue(projections.isNotEmpty())
                            assertTrue(
                                "Old same-version history/ack cannot unlock a 200 unknown projection",
                                projections.all {
                                    it.text.isBlank() &&
                                        it.attachmentURL.isBlank() &&
                                        it.forwardUnavailable
                                },
                            )
                            throw SocketTimeoutException("TEST_200_PROJECTION_REDRAW_ONLY")
                        }
                        if (
                            method == "POST" &&
                                path.endsWith("/message-results") &&
                                data?.optString("type") == "forward"
                        )
                            resultChecks++
                        if (
                            method == "POST" &&
                                path.endsWith("/messages") &&
                                data?.optString("type") == "forward"
                        ) {
                            forwardPosts++
                            bodies.add(data.toString())
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
                                lastQueued =
                                    database.messages().one(ns, data.getString("clientMsgId"))!!
                                assertTrue(
                                    "Original forward payload must be durable before HTTP",
                                    data.toString() == lastQueued!!.forwardPayload,
                                )
                                assertEquals("", lastQueued!!.text)
                                assertEquals("", lastQueued!!.attachmentId)
                                assertEquals("[]", lastQueued!!.mentionedUserIds)
                                assertFalse(lastQueued!!.mentionAll)
                                assertTrue(
                                    "Wire body must equal the original durable body",
                                    data.toString() == Forwarding.payload(lastQueued!!).toString(),
                                )
                            } finally {
                                database.close()
                            }
                            if (offlineBefore)
                                throw SocketTimeoutException("TEST_FORWARD_BEFORE_HTTP")
                            val result = super.request(path, method, data)
                            if (loseAfter)
                                throw SocketTimeoutException("TEST_FORWARD_RESPONSE_LOST")
                            return result
                        }
                        if (
                            method == "GET" &&
                                path.startsWith("/conversations/") &&
                                !path.endsWith("/members") &&
                                !path.contains('?') &&
                                denySourceConversation
                        )
                            throw ApiFailure(403, "HISTORY_NOT_ALLOWED")
                        if (
                            method == "GET" &&
                                path.endsWith("/members") &&
                                assertHiddenBeforeMemberLoad
                        ) {
                            val projected = model.messages.filter { it.forwarded }
                            assertTrue(
                                "Forwarded rows are present during the redraw",
                                projected.isNotEmpty(),
                            )
                            assertTrue(
                                "Late cached history must not revive an unknown forwarded body",
                                projected.all { it.text.isBlank() && it.attachmentURL.isBlank() },
                            )
                            assertHiddenBeforeMemberLoad = false
                        }
                        if (method == "GET" && path.startsWith("/messages/") && temporaryProjection)
                            throw ApiFailure(503, "FORWARD_QUALIFICATION_UNAVAILABLE")
                        val result = super.request(path, method, data)
                        if (
                            method == "GET" &&
                                path.startsWith("/messages/") &&
                                result.optBoolean("forwarded") &&
                                temporary200Projection
                        ) {
                            result.put("forwardUnavailable", true).put("text", "")
                            listOf("attachmentId", "attachmentURL", "forwardAdmission").forEach {
                                result.remove(it)
                            }
                        }
                        if (path == "/bootstrap" && hideFlag)
                            result.getJSONObject("features").remove("forwarding")
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
            var alice: Api? = null
            try {
                success { model.connect(base, boot.getString("deploymentId"), ca) {} }
                success { model.authenticate("register", "android_forward_" + suffix, password) }
                val me = model.userId
                val api = Api(base, model.token, ca)
                alice = api
                val invitation =
                    bob.request(
                        "/contacts/requests",
                        "POST",
                        JSONObject().put("userId", me).put("requestId", UUID.randomUUID()),
                    )
                success { model.resolveRequest(invitation.getString("id"), true) }
                success { model.create("Forward destination", listOf(peerId), true) }
                val target = model.selected!!
                val sourceGroup =
                    bob.request(
                        "/conversations",
                        "POST",
                        JSONObject()
                            .put("kind", "group")
                            .put("title", "Forward original group")
                            .put("memberIds", JSONArray(listOf(me))),
                    )
                suspend fun source(text: String, attachment: String = ""): MessageRow {
                    val snapshot = conversationSnapshot(bob, sourceGroup.getString("id"))
                    val sent =
                        bob.request(
                            "/conversations/${sourceGroup.getString("id")}/messages",
                            "POST",
                            JSONObject()
                                .put("clientMsgId", UUID.randomUUID())
                                .put("type", if (attachment.isBlank()) "text" else "file")
                                .put("text", text)
                                .put("restoreEpoch", boot.getString("restoreEpoch"))
                                .put("sendAdmission", snapshot.getString("sendAdmission"))
                                .also {
                                    if (attachment.isNotBlank()) it.put("attachmentId", attachment)
                                },
                        )
                    val ns =
                        Isolation.namespace(
                            model.profile,
                            model.deployment,
                            me,
                            false,
                            model.restoreEpoch,
                        )
                    return message(
                        api.request("/messages/${sent.getString("id")}"),
                        ns,
                        model.restoreEpoch,
                    )
                }
                val ns =
                    Isolation.namespace(
                        model.profile,
                        model.deployment,
                        me,
                        false,
                        model.restoreEpoch,
                    )
                suspend fun saved(id: String): MessageRow? {
                    val database = ChatDatabase.open(app, false)
                    return try {
                        database.messages().one(ns, id)
                    } finally {
                        database.close()
                    }
                }
                val original = source("Native referenced original")
                success { model.prepareForward(original) }
                assertFalse(model.forwardNeedsReselect)
                offlineBefore = true
                perform { model.sendForward(model.forwardSource!!, target) }
                assertTrue(model.error.isNotBlank())
                val pending = lastQueued!!
                assertEquals("pending", saved(pending.id)!!.state)
                assertEquals(7, JSONObject(pending.forwardPayload).length())
                val postsBeforeCheck = forwardPosts
                success { model.retry(pending) }
                assertTrue(pending.id in model.missingForwardResults)
                assertEquals(postsBeforeCheck, forwardPosts)
                offlineBefore = false
                success { model.retryOriginalForward(pending) }
                assertEquals(postsBeforeCheck + 1, forwardPosts)
                assertTrue("Explicit retry keeps the same original body", bodies[0] == bodies[1])
                assertNull(saved(pending.id))
                val forwarded = model.messages.single { it.clientMsgId == pending.clientMsgId }
                assertTrue(forwarded.forwarded)
                assertEquals(original.text, forwarded.text)
                assertTrue(resultChecks >= 3)

                // A real committed response loss is reconciled without another creating POST.
                success { model.prepareForward(original) }
                loseAfter = true
                perform { model.sendForward(model.forwardSource!!, target) }
                val committed = lastQueued!!
                loseAfter = false
                val afterCommitPosts = forwardPosts
                success { model.retry(committed) }
                assertEquals(afterCommitPosts, forwardPosts)
                assertTrue(
                    model.messages.any { it.clientMsgId == committed.clientMsgId && it.forwarded }
                )

                // A same-version unknown projection cannot keep showing a previously cached body.
                temporaryProjection = true
                success { model.refresh() }
                assertTrue(
                    model.messages
                        .filter { it.forwarded }
                        .all {
                            it.text.isBlank() && it.attachmentURL.isBlank() && it.forwardUnavailable
                        }
                )
                val staleProjectionDatabase = ChatDatabase.open(app, false)
                try {
                    staleProjectionDatabase.messages().put(forwarded)
                } finally {
                    staleProjectionDatabase.close()
                }
                assertHiddenBeforeMemberLoad = true
                success { model.action(forwarded, "favorite") }
                assertFalse(assertHiddenBeforeMemberLoad)
                assertTrue(
                    model.messages
                        .filter { it.forwarded }
                        .all { it.text.isBlank() && it.attachmentURL.isBlank() }
                )
                temporaryProjection = false
                success { model.refresh() }
                assertTrue(model.messages.any { it.forwarded && it.text == original.text })

                verifyTemporary200Projection(model, api, ns, target, forwarded.id) {
                    temporary200Projection = it
                }

                val staleSeen = source("Original before edit")
                bob.request(
                    "/messages/${staleSeen.id}",
                    "PATCH",
                    JSONObject()
                        .put("text", "Original after edit")
                        .put("expectedVersion", staleSeen.version)
                        .put("requestId", UUID.randomUUID()),
                )
                success { model.prepareForward(staleSeen) }
                assertTrue(model.forwardNeedsReselect)
                assertEquals("Original after edit", model.forwardSource!!.text)
                val beforeRejectedSelection = forwardPosts
                perform { model.sendForward(model.forwardSource!!, target) }
                assertEquals(beforeRejectedSelection, forwardPosts)
                withContext(Dispatchers.Main) { model.acceptUpdatedForwardSource() }
                success { model.sendForward(model.forwardSource!!, target) }
                assertTrue(model.messages.any { it.forwarded && it.text == "Original after edit" })

                val media =
                    bob.upload(
                        "forward-native.txt",
                        "text/plain",
                        "Forward attachment bytes".toByteArray(),
                    )
                val mediaSource = source("forward-native.txt", media.getString("id"))
                success { model.prepareForward(mediaSource) }
                success { model.sendForward(model.forwardSource!!, target) }
                val file = model.messages.single { it.forwarded && it.type == "file" }
                assertEquals("", file.attachmentId)
                assertEquals(
                    "/api/messages/${file.id}/attachment",
                    Forwarding.attachmentPath(base, file.id, file.attachmentURL),
                )
                var downloaded: ByteArray? = null
                var localFile: java.io.File? = null
                success {
                    model.download(file) { output, _ ->
                        localFile = output
                        downloaded = output.readBytes()
                    }
                }
                assertArrayEquals("Forward attachment bytes".toByteArray(), downloaded)
                assertThrows(IllegalArgumentException::class.java) {
                    Forwarding.attachmentPath(
                        base,
                        file.id,
                        "https://elsewhere.invalid/api/messages/${file.id}/attachment",
                    )
                }
                assertThrows(IllegalArgumentException::class.java) {
                    Forwarding.attachmentPath(
                        base,
                        file.id,
                        file.attachmentURL + "?token=not-a-real-token",
                    )
                }
                api.request("/messages/${file.id}/favorite", "PUT")
                withContext(Dispatchers.Main) { model.discovery.start("favorites") }.join()
                val favoriteFile = model.discovery.items.single { it.id == file.id }
                assertTrue(localFile!!.exists())
                denySourceConversation = true
                perform { model.openSource(favoriteFile) }
                assertTrue(
                    model.discovery.items.all {
                        it.text.isBlank() &&
                            it.attachmentURL.isBlank() &&
                            it.forwardAdmission.isBlank()
                    }
                )
                assertFalse(localFile!!.exists())
                assertTrue(saved(file.id)!!.forwardUnavailable)
                assertEquals("", saved(file.id)!!.attachmentURL)
                denySourceConversation = false
                success { model.refresh() }
                bob.request("/messages/${mediaSource.id}", "DELETE")
                success { model.refresh() }
                val unavailableFile = model.messages.single { it.id == file.id }
                assertTrue(unavailableFile.forwardUnavailable)
                assertEquals("", unavailableFile.text)
                assertEquals("", unavailableFile.attachmentURL)
                var reopened = false
                perform { model.download(file) { _, _ -> reopened = true } }
                assertFalse(reopened)

                // Persist original proof before a source membership round trip.
                success { model.prepareForward(original) }
                offlineBefore = true
                perform { model.sendForward(model.forwardSource!!, target) }
                val stale = lastQueued!!
                offlineBefore = false
                val member =
                    api.request("/conversations/${sourceGroup.getString("id")}")
                        .getString("membershipGeneration")
                bob.request(
                    "/conversations/${sourceGroup.getString("id")}/members/$me?membershipGeneration=$member",
                    "DELETE",
                )
                bob.request(
                    "/conversations/${sourceGroup.getString("id")}/members",
                    "POST",
                    JSONObject().put("userId", me),
                )
                withContext(Dispatchers.Main) {
                    model = ChatModel(app, factory)
                    holder.put("fixture", model)
                }
                withTimeout(10000) {
                    while (
                        model.userId.isBlank() || model.conversations.none { it.id == target.id }
                    ) delay(50)
                }
                success { model.refresh() }
                success { model.open(model.conversations.single { it.id == target.id }) }
                assertTrue(
                    "Source proof is never replaced",
                    stale.forwardPayload == saved(stale.id)!!.forwardPayload,
                )
                success { model.retry(stale) }
                perform { model.retryOriginalForward(stale) }
                assertEquals(
                    "Original source rejection code=" + model.errorCode,
                    "frozen",
                    saved(stale.id)!!.state,
                )
                assertTrue(
                    "Source proof is never replaced",
                    stale.forwardPayload == saved(stale.id)!!.forwardPayload,
                )
                val frozenPosts = forwardPosts
                perform { model.retry(stale) }
                assertEquals(frozenPosts, forwardPosts)

                val currentSource = source("Current member original")
                success { model.prepareForward(currentSource) }
                offlineBefore = true
                perform { model.sendForward(model.forwardSource!!, target) }
                val gated = lastQueued!!
                offlineBefore = false
                hideFlag = true
                success { model.retry(gated) }
                val beforeNoFlag = forwardPosts
                perform { model.retryOriginalForward(gated) }
                assertEquals(beforeNoFlag, forwardPosts)
                assertEquals("pending", saved(gated.id)!!.state)
                assertTrue(
                    "Capability changes do not rewrite the original body",
                    gated.forwardPayload == saved(gated.id)!!.forwardPayload,
                )
                hideFlag = false
                success { model.retryOriginalForward(gated) }
                assertTrue(
                    model.messages.any { it.clientMsgId == gated.clientMsgId && it.forwarded }
                )
                val legacyId = UUID.randomUUID().toString()
                val legacy =
                    gated.copy(
                        key = "$ns|$legacyId",
                        id = legacyId,
                        clientMsgId = legacyId,
                        forwardPayload = "",
                    )
                val legacyDatabase = ChatDatabase.open(app, false)
                try {
                    legacyDatabase.messages().enqueue(legacy)
                } finally {
                    legacyDatabase.close()
                }
                val legacyPosts = forwardPosts
                perform { model.retry(legacy) }
                assertEquals(legacyPosts, forwardPosts)
                assertEquals("frozen", saved(legacyId)!!.state)
                assertEquals("", saved(legacyId)!!.forwardPayload)
                assertThrows(IllegalArgumentException::class.java) {
                    Forwarding.attachmentPath(
                        base,
                        file.id,
                        "https://127.0.0.1:8443/api/messages/${file.id}/attachment",
                    )
                }
            } finally {
                withContext(Dispatchers.Main) { holder.clear() }
                alice?.request("/auth/logout", "POST", JSONObject())
                bob.request("/auth/logout", "POST", JSONObject())
            }
        }
}
