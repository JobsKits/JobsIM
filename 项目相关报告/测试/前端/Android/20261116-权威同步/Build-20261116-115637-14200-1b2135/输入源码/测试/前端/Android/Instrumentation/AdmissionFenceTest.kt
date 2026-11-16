package com.jobs.im

import android.app.Application
import android.util.Base64
import androidx.lifecycle.ViewModelStore
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
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
class AdmissionFenceTest {
    @Test
    fun missedMembershipCycleCannotBorrowNewAdmissionForOriginalOutbox() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val args = InstrumentationRegistry.getArguments()
        val base = args.getString("liveUrl")
        val encoded = args.getString("caBase64")
        assumeTrue(!base.isNullOrBlank() && !encoded.isNullOrBlank())
        val ca = Base64.decode(encoded, Base64.DEFAULT)
        val anonymous = Api(base!!, ca = ca)
        val boot = anonymous.request("/bootstrap")
        assertTrue(boot.getJSONObject("features").getBoolean("send_admission"))
        val app = instrumentation.targetContext.applicationContext as Application
        app.getSharedPreferences("JobsIM.settings", 0)
            .edit()
            .putString("deployment", "")
            .putString("profile", UUID.randomUUID().toString())
            .commit()
        val suffix = UUID.randomUUID().toString().replace("-", "").take(12)
        val password = "Admission fixture " + UUID.randomUUID()
        val peer =
            anonymous.request(
                "/auth/register",
                "POST",
                JSONObject()
                    .put("username", "android_admit_peer_" + suffix)
                    .put("password", password)
                    .put("requestId", UUID.randomUUID())
                    .put("deviceName", "Admission peer"),
            )
        val peerId = peer.getJSONObject("user").getString("id")
        val peerApi = Api(base, peer.getString("token"), ca)
        val offline = AtomicBoolean(true)
        val oldServer = AtomicBoolean(false)
        val posted = mutableListOf<JSONObject>()
        val factory = { address: String, token: String, trust: ByteArray? ->
            object : Api(address, token, trust) {
                override suspend fun request(
                    path: String,
                    method: String,
                    data: JSONObject?,
                ): JSONObject {
                    if (method == "POST" && path.endsWith("/messages")) {
                        posted.add(JSONObject(data.toString()))
                        if (offline.get()) throw SocketTimeoutException("TEST_OFFLINE_BEFORE_HTTP")
                    }
                    val result = super.request(path, method, data)
                    if (path == "/bootstrap" && oldServer.get())
                        result.getJSONObject("features").remove("send_admission")
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
        suspend fun stored(id: String): MessageRow {
            val database = ChatDatabase.open(app, false)
            try {
                return database
                    .messages()
                    .one(
                        Isolation.namespace(
                            model.profile,
                            model.deployment,
                            model.userId,
                            false,
                            model.restoreEpoch,
                        ),
                        id,
                    )!!
            } finally {
                database.close()
            }
        }
        suspend fun put(row: MessageRow) {
            val database = ChatDatabase.open(app, false)
            try {
                database.messages().put(row)
            } finally {
                database.close()
            }
        }
        try {
            success { model.connect(base, boot.getString("deploymentId"), ca) {} }
            success { model.authenticate("register", "android_admit_" + suffix, password) }
            val me = model.userId
            val request =
                peerApi.request(
                    "/contacts/requests",
                    "POST",
                    JSONObject().put("userId", me).put("requestId", UUID.randomUUID()),
                )
            success { model.resolveRequest(request.getString("id"), true) }
            success { model.create("Admission fence group", listOf(peerId), true) }
            success { model.loadMembers() }
            val membership = model.members.single { it.id == me }.membershipGeneration
            success {
                model.groupChange("transfer", member = model.members.single { it.id == peerId })
            }
            val originalConversation = model.selected!!
            withContext(Dispatchers.Main) { model.updateDraft("Original offline intent") }
            perform { model.send(model.draft) }
            assertTrue(model.offline)
            val pending = model.messages.single { it.state == "pending" }
            val original = stored(pending.id)
            assertTrue(original.sendAdmission.isNotBlank())
            assertTrue(original.sendAdmission == originalConversation.sendAdmission)
            assertTrue(original.membershipGeneration == membership)
            peerApi.request(
                "/conversations/${originalConversation.id}/members/$me?membershipGeneration=$membership",
                "DELETE",
            )
            peerApi.request(
                "/conversations/${originalConversation.id}/members",
                "POST",
                JSONObject().put("userId", me),
            )
            // The client deliberately misses both events; cold restoration sees only the new
            // membership.
            withContext(Dispatchers.Main) {
                model = ChatModel(app, factory)
                holder.put("fixture", model)
            }
            withTimeout(10000) { while (model.userId.isBlank()) delay(50) }
            success { model.refresh() }
            success { model.open(model.conversations.single { it.id == originalConversation.id }) }
            assertTrue(model.selected!!.sendAdmission != original.sendAdmission)
            assertTrue(model.selected!!.membershipGeneration != original.membershipGeneration)
            offline.set(false)
            val reopenedPending = stored(original.id)
            perform { model.retry(reopenedPending) }
            assertEquals("SEND_ADMISSION_CHANGED", model.errorCode)
            val frozen = stored(original.id)
            assertEquals("frozen", frozen.state)
            assertTrue(frozen.sendAdmission == original.sendAdmission)
            assertEquals(original.clientMsgId, frozen.clientMsgId)
            assertTrue(posted.last().getString("sendAdmission") == original.sendAdmission)
            val attempts = posted.size
            perform { model.retry(frozen) }
            assertEquals(attempts, posted.size)
            perform { model.retry(original) }
            assertEquals(attempts, posted.size)
            val legacyId = UUID.randomUUID().toString()
            val legacy =
                original.copy(
                    key = original.namespace + "|" + legacyId,
                    id = legacyId,
                    clientMsgId = legacyId,
                    text = "Legacy unknown intent",
                    state = "pending",
                    sendAdmission = "",
                    membershipGeneration = "",
                )
            put(legacy)
            perform { model.retry(legacy) }
            assertEquals("SEND_ADMISSION_CHANGED", model.errorCode)
            assertEquals("frozen", stored(legacy.id).state)
            assertTrue(posted.last().optString("sendAdmission").isEmpty())
            val oldId = UUID.randomUUID().toString()
            val oldQueue =
                legacy.copy(key = legacy.namespace + "|" + oldId, id = oldId, clientMsgId = oldId)
            put(oldQueue)
            oldServer.set(true)
            val beforeOldServer = posted.size
            perform { model.retry(oldQueue) }
            assertEquals("SEND_ADMISSION_UNSUPPORTED", model.errorCode)
            assertEquals(beforeOldServer, posted.size)
            assertEquals("pending", stored(oldId).state)
            oldServer.set(false)
            success { model.refresh() }
            withContext(Dispatchers.Main) { model.updateDraft("New membership new intent") }
            success { model.send(model.draft) }
            val payload = posted.last()
            assertTrue(payload.getString("sendAdmission") == model.selected!!.sendAdmission)
            val api = Api(base, model.token, ca)
            val replay =
                api.request("/conversations/${originalConversation.id}/messages", "POST", payload)
            assertEquals(1, model.messages.count { it.id == replay.getString("id") })
            val conflict = runCatching {
                api.request(
                    "/conversations/${originalConversation.id}/messages",
                    "POST",
                    JSONObject(payload.toString()).put("sendAdmission", "changed"),
                )
            }
                .exceptionOrNull()
            assertTrue(conflict is ApiFailure && conflict.code == "IDEMPOTENCY_CONFLICT")
            success { model.create("Direct admission fence", listOf(peerId), false) }
            val directConversation = model.selected!!
            offline.set(true)
            withContext(Dispatchers.Main) { model.updateDraft("Original direct offline intent") }
            perform { model.send(model.draft) }
            val directPending = model.messages.single { it.state == "pending" }
            peerApi.request("/contacts/$me", "DELETE")
            val replacement =
                peerApi.request(
                    "/contacts/requests",
                    "POST",
                    JSONObject().put("userId", me).put("requestId", UUID.randomUUID()),
                )
            api.request(
                "/contacts/requests/${replacement.getString("id")}/accept",
                "POST",
                JSONObject(),
            )
            success { model.refresh() }
            assertTrue(model.selected!!.sendAdmission != directConversation.sendAdmission)
            offline.set(false)
            perform { model.retry(directPending) }
            assertEquals("SEND_ADMISSION_CHANGED", model.errorCode)
            val directFrozen = stored(directPending.id)
            assertEquals("frozen", directFrozen.state)
            assertTrue(directFrozen.sendAdmission == directPending.sendAdmission)
            assertEquals(directPending.clientMsgId, directFrozen.clientMsgId)
        } finally {
            oldServer.set(false)
            perform { model.logout() }
            withContext(Dispatchers.Main) { holder.clear() }
            peerApi.request("/auth/logout", "POST", JSONObject())
        }
    }
}
