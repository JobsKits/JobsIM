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
class RoleAdmissionLifecycleTest {
    @Test
    fun ownerRoundTripCannotReviveOldAllMentionOrAnnouncementAndMissingFlagNeverWrites() =
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
            assertTrue(boot.getJSONObject("features").getBoolean("role_admission"))
            val app = instrumentation.targetContext.applicationContext as Application
            app.getSharedPreferences("JobsIM.settings", 0)
                .edit()
                .putString("deployment", "")
                .putString("profile", UUID.randomUUID().toString())
                .commit()
            val suffix = UUID.randomUUID().toString().replace("-", "").take(10)
            val password = "Role fence fixture " + UUID.randomUUID()
            val peer =
                anonymous.request(
                    "/auth/register",
                    "POST",
                    JSONObject()
                        .put("username", "android_role_peer_" + suffix)
                        .put("password", password)
                        .put("requestId", UUID.randomUUID())
                        .put("deviceName", "Role peer"),
                )
            val peerId = peer.getJSONObject("user").getString("id")
            val peerApi = Api(base, peer.getString("token"), ca)
            val offlineAll = AtomicBoolean(true)
            val offlineAnnouncement = AtomicBoolean(true)
            val hideRoleFlag = AtomicBoolean(false)
            var allCreates = 0
            var announcementCreates = 0
            var messageLookups = 0
            var operationLookups = 0
            val allBodies = mutableListOf<String>()
            val factory = { address: String, token: String, trust: ByteArray? ->
                object : Api(address, token, trust) {
                    override suspend fun request(
                        path: String,
                        method: String,
                        data: JSONObject?,
                    ): JSONObject {
                        if (path.endsWith("/message-results")) messageLookups++
                        if (path.contains("/announcement/operations/")) operationLookups++
                        if (
                            path.endsWith("/messages") &&
                                method == "POST" &&
                                data!!.optBoolean("mentionAll")
                        ) {
                            allCreates++
                            allBodies.add(data.toString())
                            if (offlineAll.get())
                                throw SocketTimeoutException("TEST_OFFLINE_BEFORE_ALL_WRITE")
                        }
                        if (path.endsWith("/announcement") && method in listOf("PUT", "DELETE")) {
                            announcementCreates++
                            if (offlineAnnouncement.get())
                                throw SocketTimeoutException(
                                    "TEST_OFFLINE_BEFORE_ANNOUNCEMENT_WRITE"
                                )
                        }
                        val result = super.request(path, method, data)
                        if (path == "/bootstrap" && hideRoleFlag.get())
                            result.getJSONObject("features").remove("role_admission")
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
            suspend fun ann(action: () -> Job) {
                perform(action)
                assertEquals("", model.announcement.problem)
            }
            suspend fun row(id: String): MessageRow {
                val db = ChatDatabase.open(app, false)
                try {
                    return db.messages()
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
                    db.close()
                }
            }
            var api: Api? = null
            try {
                success { model.connect(base, boot.getString("deploymentId"), ca) {} }
                success { model.authenticate("register", "android_role_" + suffix, password) }
                val me = model.userId
                val currentApi = Api(base, model.token, ca)
                api = currentApi
                val invitation =
                    peerApi.request(
                        "/contacts/requests",
                        "POST",
                        JSONObject().put("userId", me).put("requestId", UUID.randomUUID()),
                    )
                success { model.resolveRequest(invitation.getString("id"), true) }
                success { model.create("Role qualification fence", listOf(peerId), true) }
                success { model.loadMembers() }
                val originalGroup = model.selected!!
                withContext(Dispatchers.Main) { model.updateDraft("Role reference source") }
                success { model.send(model.draft) }
                val source = model.messages.single { it.text == "Role reference source" }
                withContext(Dispatchers.Main) {
                    model.updateDraft("Old owner all intent")
                    model.setDraftMentions(emptyList(), true)
                }
                perform { model.send(model.draft) }
                assertTrue(model.error.isNotEmpty())
                val oldAll = model.messages.single { it.state == "pending" }
                ann { model.announcement.load(originalGroup) }
                ann { model.announcement.choose(source) }
                perform { model.announcement.publish() }
                assertTrue(model.announcement.problem.isNotEmpty())
                val oldOp = model.announcement.pending!!
                val oldBody = JSONObject(oldOp.payload).getJSONObject("body").toString()
                assertTrue(
                    JSONObject(oldBody).getString("roleGeneration") == originalGroup.roleGeneration
                )
                currentApi.request(
                    "/conversations/${originalGroup.id}",
                    "PATCH",
                    JSONObject().put("ownerId", peerId),
                )
                peerApi.request(
                    "/conversations/${originalGroup.id}",
                    "PATCH",
                    JSONObject().put("ownerId", me),
                )
                success { model.refresh() }
                val newGroup = model.selected!!
                assertEquals(originalGroup.membershipGeneration, newGroup.membershipGeneration)
                assertTrue(originalGroup.roleGeneration != newGroup.roleGeneration)
                offlineAll.set(false)
                offlineAnnouncement.set(false)
                perform { model.retry(oldAll) }
                assertEquals("frozen", row(oldAll.id).state)
                assertTrue(row(oldAll.id).sendAdmission == oldAll.sendAdmission)
                assertTrue(allBodies[0] == allBodies[1])
                perform { model.announcement.retry() }
                assertTrue(model.announcement.terminalPending)
                assertEquals(
                    "QUALIFICATION_CHANGED",
                    JSONObject(model.announcement.pending!!.payload).getString("terminalError"),
                )
                assertTrue(
                    JSONObject(model.announcement.pending!!.payload)
                        .getJSONObject("body")
                        .toString() == oldBody
                )
                ann { model.announcement.reload() }
                ann { model.announcement.choose(source) }
                ann { model.announcement.publish() }
                assertEquals("active", model.announcement.current!!.state)
                val db = ChatDatabase.open(app, false)
                val ns =
                    Isolation.namespace(
                        model.profile,
                        model.deployment,
                        me,
                        false,
                        model.restoreEpoch,
                    )
                try {
                    assertTrue(
                        JSONObject(
                                db.messages()
                                    .operation(ns, "announcement-history-" + oldOp.requestId)!!
                                    .payload
                            )
                            .getJSONObject("body")
                            .toString() == oldBody
                    )
                } finally {
                    db.close()
                }
                val legacyId = UUID.randomUUID().toString()
                val legacyBody =
                    JSONObject(oldBody)
                        .put("requestId", legacyId)
                        .put("expectedVersion", model.announcement.current!!.version)
                        .also { it.remove("roleGeneration") }
                val legacy =
                    oldOp.copy(
                        requestId = legacyId,
                        payload =
                            JSONObject()
                                .put("conversationId", originalGroup.id)
                                .put("method", "PUT")
                                .put("body", legacyBody)
                                .toString(),
                    )
                val legacyDb = ChatDatabase.open(app, false)
                try {
                    legacyDb.messages().putOperation(legacy)
                } finally {
                    legacyDb.close()
                }
                hideRoleFlag.set(true)
                ann { model.announcement.reload() }
                val beforeAnnouncement = announcementCreates
                val beforeLookup = operationLookups
                perform { model.announcement.retry() }
                assertEquals(beforeAnnouncement, announcementCreates)
                assertTrue(operationLookups > beforeLookup)
                hideRoleFlag.set(false)
                success { model.refresh() }
                ann { model.announcement.reload() }
                perform { model.announcement.retry() }
                assertTrue(model.announcement.terminalPending)
                assertFalse(
                    JSONObject(model.announcement.pending!!.payload)
                        .getJSONObject("body")
                        .has("roleGeneration")
                )
                ann { model.announcement.reload() }
                ann { model.announcement.choose(source) }
                ann { model.announcement.publish() }
                val legacyArchiveDb = ChatDatabase.open(app, false)
                try {
                    assertFalse(
                        JSONObject(
                                legacyArchiveDb
                                    .messages()
                                    .operation(ns, "announcement-history-" + legacyId)!!
                                    .payload
                            )
                            .getJSONObject("body")
                            .has("roleGeneration")
                    )
                } finally {
                    legacyArchiveDb.close()
                }
                offlineAll.set(true)
                withContext(Dispatchers.Main) {
                    model.updateDraft("Current owner intent before missing flag")
                    model.setDraftMentions(emptyList(), true)
                }
                perform { model.send(model.draft) }
                val freshPending = model.messages.single { it.state == "pending" }
                offlineAll.set(false)
                hideRoleFlag.set(true)
                val beforeAll = allCreates
                val beforeMessageLookup = messageLookups
                perform { model.retry(freshPending) }
                assertEquals(beforeAll, allCreates)
                assertTrue(messageLookups > beforeMessageLookup)
                assertEquals("frozen", row(freshPending.id).state)
                hideRoleFlag.set(false)
                success { model.refresh() }
                perform { model.retry(freshPending) }
                assertEquals(beforeAll, allCreates)
                withContext(Dispatchers.Main) {
                    model.updateDraft("New current owner all intent")
                    model.setDraftMentions(emptyList(), true)
                }
                success { model.send(model.draft) }
                assertEquals(beforeAll + 1, allCreates)
            } finally {
                hideRoleFlag.set(false)
                withContext(Dispatchers.Main) { holder.clear() }
                api?.request("/auth/logout", "POST", JSONObject())
                peerApi.request("/auth/logout", "POST", JSONObject())
            }
        }
}
