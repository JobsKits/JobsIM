package com.jobs.im

import android.app.Application
import android.util.Base64
import androidx.lifecycle.ViewModelStore
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
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
class DiscoveryLifecycleTest {
    @Test
    fun realLookupPagesKeepOpaqueCursorsAndReauthorizeAnUnlistedSource() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val args = InstrumentationRegistry.getArguments()
        val base = args.getString("liveUrl")
        val encoded = args.getString("caBase64")
        assumeTrue(!base.isNullOrBlank() && !encoded.isNullOrBlank())
        val ca = Base64.decode(encoded, Base64.DEFAULT)
        val anonymous = Api(base!!, ca = ca)
        val boot = anonymous.request("/bootstrap")
        assertEquals(args.getString("deploymentId"), boot.getString("deploymentId"))
        val app = instrumentation.targetContext.applicationContext as Application
        app.getSharedPreferences("JobsIM.settings", 0)
            .edit()
            .putString("deployment", "")
            .putString("profile", UUID.randomUUID().toString())
            .commit()
        val suffix = UUID.randomUUID().toString().replace("-", "").take(10)
        val password = "Discovery fixture " + UUID.randomUUID()
        val peer =
            anonymous.request(
                "/auth/register",
                "POST",
                JSONObject()
                    .put("username", "android_lookup_peer_" + suffix)
                    .put("password", password)
                    .put("requestId", UUID.randomUUID())
                    .put("deviceName", "Discovery peer"),
            )
        val peerApi = Api(base, peer.getString("token"), ca)
        val hideTarget = AtomicBoolean(false)
        val delayLookup = AtomicBoolean(false)
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        var target = ""
        var cursorSent = ""
        var sourceMessageReads = 0
        var sourceConversationReads = 0
        val factory = { address: String, token: String, trust: ByteArray? ->
            object : Api(address, token, trust) {
                override suspend fun request(
                    path: String,
                    method: String,
                    data: JSONObject?,
                ): JSONObject {
                    if (path == "/conversations/$target") sourceConversationReads++
                    if (path.startsWith("/messages/") && method == "GET") sourceMessageReads++
                    if (path.startsWith("/search?") && path.contains("&cursor="))
                        cursorSent =
                            java.net.URLDecoder.decode(path.substringAfter("&cursor="), "UTF-8")
                    val result = super.request(path, method, data)
                    if (path == "/conversations" && hideTarget.get()) {
                        val items = result.getJSONArray("items")
                        result.put(
                            "items",
                            JSONArray(
                                (0 until items.length())
                                    .map { items.getJSONObject(it) }
                                    .filter { it.getString("id") != target }
                            ),
                        )
                    }
                    if (path.startsWith("/search?") && delayLookup.compareAndSet(true, false)) {
                        entered.complete(Unit)
                        withContext(NonCancellable) { release.await() }
                    }
                    return result
                }
            }
        }
        val holder = ViewModelStore()
        lateinit var model: ChatModel
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
        var selfApi: Api? = null
        try {
            success { model.connect(base, boot.getString("deploymentId"), ca) {} }
            success { model.authenticate("register", "android_lookup_" + suffix, password) }
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
            success {
                model.create(
                    "Discovery exact source",
                    listOf(peer.getJSONObject("user").getString("id")),
                    true,
                )
            }
            target = model.selected!!.id
            val admission = model.selected!!.sendAdmission
            val keyword = "native_lookup_" + suffix
            repeat(51) { index ->
                val sent =
                    api.request(
                        "/conversations/$target/messages",
                        "POST",
                        JSONObject()
                            .put("clientMsgId", UUID.randomUUID())
                            .put("restoreEpoch", model.restoreEpoch)
                            .put("sendAdmission", admission)
                            .put("type", "text")
                            .put("text", "$keyword item $index"),
                    )
                api.request("/messages/${sent.getString("id")}/favorite", "PUT")
            }
            perform { model.discovery.start("search", keyword, target) }
            assertEquals("", model.discovery.problem)
            assertEquals(50, model.discovery.items.size)
            assertTrue(model.discovery.hasMore)
            val opaque = model.discovery.cursor
            perform { model.discovery.more() }
            assertTrue(cursorSent == opaque)
            assertEquals(51, model.discovery.items.size)
            assertFalse(model.discovery.hasMore)
            perform { model.discovery.start("favorites", conversationId = target) }
            assertEquals(50, model.discovery.items.size)
            perform { model.discovery.more() }
            assertEquals(51, model.discovery.items.size)
            val source = model.discovery.items.first()
            api.request(
                "/messages/${source.id}",
                "PATCH",
                JSONObject()
                    .put("text", "Authorized current source")
                    .put("expectedVersion", source.version)
                    .put("requestId", UUID.randomUUID()),
            )
            withContext(Dispatchers.Main) {
                model.closeConversation()
                hideTarget.set(true)
            }
            success { model.refresh() }
            assertTrue(model.conversations.none { it.id == target })
            success { model.openSource(source) }
            assertTrue(sourceConversationReads > 0 && sourceMessageReads > 0)
            assertEquals(target, model.selected!!.id)
            assertEquals(source.id, model.focusedMessage)
            assertEquals(
                "Authorized current source",
                model.messages.single { it.id == source.id }.text,
            )
            success { model.refresh() }
            assertEquals(target, model.selected!!.id)
            val members = api.request("/conversations/$target/members").getJSONArray("items")
            val membership =
                (0 until members.length())
                    .map { members.getJSONObject(it) }
                    .single { it.getString("id") == me }
                    .getString("membershipGeneration")
            api.request(
                "/conversations/$target",
                "PATCH",
                JSONObject().put("ownerId", peer.getJSONObject("user").getString("id")),
            )
            peerApi.request(
                "/conversations/$target/members/$me?membershipGeneration=$membership",
                "DELETE",
            )
            peerApi.request(
                "/conversations/$target/members",
                "POST",
                JSONObject().put("userId", me),
            )
            withContext(Dispatchers.Main) { model.closeConversation() }
            perform { model.openSource(source) }
            assertTrue(model.error.isNotEmpty())
            assertNull(model.selected)
            val unavailable = model.discovery.items.single { it.id == source.id }
            assertTrue(
                unavailable.contentUnavailable &&
                    unavailable.text.isEmpty() &&
                    unavailable.attachmentId.isEmpty()
            )
            assertTrue(
                model.discovery.items.none { it.conversationId == target && it.text.isNotEmpty() }
            )
            perform { model.discovery.start("search", "界".repeat(86)) }
            assertEquals("searchLimit", model.discovery.problem)
            delayLookup.set(true)
            val pendingLookup =
                withContext(Dispatchers.Main) { model.discovery.start("search", keyword) }
            withTimeout(10000) { entered.await() }
            success { model.preview() }
            release.complete(Unit)
            pendingLookup.join()
            assertTrue(model.discovery.items.isEmpty())
            assertFalse(model.discovery.hasMore)
        } finally {
            release.complete(Unit)
            withContext(Dispatchers.Main) { holder.clear() }
            selfApi?.request("/auth/logout", "POST", JSONObject())
            peerApi.request("/auth/logout", "POST", JSONObject())
        }
    }
}
