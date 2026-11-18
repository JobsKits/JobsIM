package com.jobs.im

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.time.Instant
import java.util.Base64
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class AuthoritativeSyncTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private fun uuid() = UUID.randomUUID().toString()

    private class Fixture(val database: ChatDatabase) {
        private fun uuid() = UUID.randomUUID().toString()
        val profile = uuid()
        val deployment = uuid()
        val user = uuid()
        val device = uuid()
        val conversation = uuid()
        val messageId = uuid()
        val stream = uuid()
        val permission = "a".repeat(64)
        val ns = Isolation.namespace(profile, deployment, user, false, "1", "personal", device)
        val identity = SyncIdentity(ns, profile, deployment, user, "1", device, uuid())
        val store = SyncStore(database)
        var control = JSONObject()

        suspend fun start() {
            database.messages().putNativeScope(NativeScope(uuid(), "live|$profile|$deployment|", ns, device, "test"))
            control = store.bind(identity, store.ensure(identity) { true }, stream) { true }
        }

        fun ref(kind: String, id: String, revision: String = "1") = SyncRef(JSONObject()
            .put("owner", if (kind == "message") "message" else "conversation")
            .put("kind", kind).put("id", id).put("revision", revision).put("revisionEpoch", "1")
            .put("state", "active").also { if (kind == "message") it.put("conversationId", conversation) })

        fun body(id: String = messageId, text: String = "source") = JSONObject()
            .put("id", id).put("conversationId", conversation).put("senderId", user)
            .put("clientMsgId", "0123456789abcdef0123456789abcdef").put("type", "text")
            .put("text", text).put("version", "1").put("contentVersion", "1")
            .put("sequence", "1").put("createdAt", "2026-11-16T01:00:00Z")

        fun projection(ref: SyncRef, state: String, body: JSONObject?) =
            SyncProjection(ref, state, ref.revision, "1", body, body?.toString()?.toByteArray()?.let(SyncWire::hash).orEmpty())

        fun wireProjection(value: SyncProjection): JSONObject = value.metadata().put("revisionMatch", true).also {
            value.body?.let { body ->
                val bytes = body.toString().toByteArray()
                it.put("object", body).put("bodyDigest", SyncWire.hash(bytes))
                    .put("bodyEnvelope", Base64.getUrlEncoder().withoutPadding().encodeToString(bytes))
            }
        }

        fun conversationProjection() = projection(ref("conversation", conversation), "available", JSONObject()
            .put("id", conversation).put("kind", "direct").put("title", "fixture").put("memberIds", JSONArray(listOf(user))))

        fun delta(position: String = "0", values: List<SyncProjection> = emptyList()) =
            SyncDelta("opaque-$position", position, position, false, values, JSONArray())

        suspend fun snapshot(values: List<SyncProjection>, stagePage: Boolean = true): Pair<SyncManifest, SyncPage> {
            control = store.begin(identity) { true }
            val refs = JSONArray().also { array -> values.forEach { array.put(it.ref.raw) } }
            val bytes = refs.toString().toByteArray()
            val digest = SyncWire.hash(bytes)
            val manifest = SyncManifest(identity.binding(control.getString("storeGeneration"), stream)
                .put("snapshotId", uuid()).put("schemaVersion", "1").put("boundary", "0")
                .put("boundaryToken", "original-boundary-token").put("permissionGeneration", permission)
                .put("classificationWatermark", "0").put("expiresAt", Instant.now().plusSeconds(900).toString())
                .put("ownerWatermarks", JSONObject().put("auth", "0").put("directory", "0").put("conversation", "0").put("message", "0"))
                .put("coverage", JSONObject()).put("pageManifest", JSONArray().put(JSONObject().put("page", 0).put("count", values.size).put("digest", digest))))
            manifest.check(identity, control.getString("storeGeneration"), stream)
            control = store.manifest(identity, manifest, control.getJSONObject("snapshotRequest").getString("requestId")) { true }
            val page = SyncPage(0, digest, Base64.getUrlEncoder().withoutPadding().encodeToString(bytes), values.map { it.ref }, values)
            if (stagePage) store.page(identity, manifest, page) { true }
            return manifest to page
        }

        suspend fun install(values: List<SyncProjection>) {
            val (manifest, page) = snapshot(values)
            store.commit(identity, control, manifest, listOf(page), delta(), values, "0", emptyList(), null) { true }
            control = store.ensure(identity) { true }
        }
    }

    private suspend fun fixture(test: suspend (Fixture) -> Unit) {
        val name = "JobsIM-test-authoritative-${uuid()}.db"
        val database = ChatDatabase.open(context, false, name)
        try { test(Fixture(database).also { it.start() }) }
        finally { database.close(); context.deleteDatabase(name) }
    }

    @Test
    fun incompleteStageNeverInstallsOrAdvancesCursorAndOriginalNonceSurvivesRestart() = runBlocking {
        val name = "JobsIM-test-authoritative-${uuid()}.db"
        var database = ChatDatabase.open(context, false, name)
        try {
            val f = Fixture(database); f.start()
            val original = f.store.begin(f.identity) { true }.getJSONObject("snapshotRequest").toString()
            val values = listOf(f.conversationProjection(), f.projection(f.ref("message", f.messageId), "available", f.body()))
            val (manifest, page) = f.snapshot(values, false)
            assertTrue(runCatching { f.store.commit(f.identity, f.control, manifest, listOf(page), f.delta(), values, "0", emptyList(), null) { true } }.exceptionOrNull() is SyncFailure)
            assertFalse(f.store.ensure(f.identity) { true }.has("cursor"))
            database.close()
            database = ChatDatabase.open(context, false, name)
            assertEquals(original, SyncStore(database).begin(f.identity) { true }.getJSONObject("snapshotRequest").toString())
            assertTrue(database.messages().all(f.ns).isEmpty())
        } finally { database.close(); context.deleteDatabase(name) }
    }

    @Test
    fun sameOpaqueClientIdNeverAcknowledgesWithoutOriginalRouteAndBodyProof() = runBlocking { fixture { f ->
        val original = MessageRow("${f.ns}|local", f.ns, f.conversation, "local", "0123456789abcdef0123456789abcdef", f.user, text = "original", state = "pending", sendAdmission = "original-admission")
        f.database.messages().enqueue(original)
        f.install(listOf(f.conversationProjection(), f.projection(f.ref("message", f.messageId), "available", f.body(text = "different submitted body"))))
        assertEquals(original, f.database.messages().outbox(f.ns).single())
        val wrongBody = SyncOutboxProof(original.clientMsgId, f.conversation, originalMessageBody(original).put("text", "different").toString())
        val wrongRoute = wrongBody.copy(conversationId = UUID.randomUUID().toString(), originalBody = originalMessageBody(original).toString())
        f.store.commit(f.identity, f.control, null, emptyList(), f.delta(), emptyList(), "0", listOf(wrongBody, wrongRoute), null) { true }
        assertEquals(original, f.database.messages().outbox(f.ns).single())
        f.control = f.store.ensure(f.identity) { true }
        f.store.commit(f.identity, f.control, null, emptyList(), f.delta(), emptyList(), "0", listOf(wrongBody.copy(originalBody = originalMessageBody(original).toString())), null) { true }
        assertTrue(f.database.messages().outbox(f.ns).isEmpty())
    } }

    @Test
    fun localRestrictionRejectsLateCommitAndRetainsOrdinaryButNeverForwardBody() = runBlocking { fixture { f ->
        val normal = f.projection(f.ref("message", f.messageId), "available", f.body())
        val forwardedId = uuid()
        val forwarded = f.projection(f.ref("message", forwardedId), "available", f.body(forwardedId, "controlled-body").put("forwarded", true).put("forwardAdmission", "temporary-proof"))
        f.install(listOf(f.conversationProjection(), normal, forwarded))
        val stale = JSONObject(f.control.toString())
        f.store.restrict(f.identity, "SYNC_UNAVAILABLE", false) { true }
        assertTrue(f.database.messages().all(f.ns).isEmpty())
        val retained = f.database.messages().all("${f.ns}|sync-retained")
        assertEquals("source", retained.single { it.id == f.messageId }.text)
        val heldForward = retained.single { it.id == forwardedId }
        assertEquals("", heldForward.text); assertEquals("", heldForward.forwardAdmission)
        assertTrue(heldForward.forwardUnavailable)
        assertTrue(runCatching { f.store.commit(f.identity, stale, null, emptyList(), f.delta(), emptyList(), "0", emptyList(), null) { true } }.exceptionOrNull() is SyncFailure)
        assertEquals("0", f.store.ensure(f.identity) { true }.getString("position"))
    } }

    @Test
    fun globalMinimalTombstoneErasesAllDeviceAndRetainedHoldersAndCannotRevive() = runBlocking { fixture { f ->
        f.install(listOf(f.conversationProjection(), f.projection(f.ref("message", f.messageId), "available", f.body())))
        val older = f.ns.replace(f.device, uuid())
        val source = f.database.messages().one(f.ns, f.messageId)!!
        f.database.messages().put(source.copy(key = "$older|${source.id}", namespace = older, text = "old-device-body"))
        f.database.messages().put(source.copy(key = "${f.ns}|sync-retained|${source.id}", namespace = "${f.ns}|sync-retained"))
        val global = f.projection(f.ref("message", f.messageId, "2"), "global", JSONObject().put("id", f.messageId))
        f.store.commit(f.identity, f.control, null, emptyList(), f.delta("1", listOf(global)), listOf(global), "0", emptyList(), null) { true }
        f.database.messages().across("live|${f.profile}|${f.deployment}|${f.user}|", f.messageId).forEach {
            assertTrue(it.retracted); assertEquals("", it.text); assertEquals("", it.attachmentURL)
        }
        f.control = f.store.ensure(f.identity) { true }
        val late = f.projection(f.ref("message", f.messageId, "999"), "available", f.body(text = "must not revive").put("version", "999"))
        f.store.commit(f.identity, f.control, null, emptyList(), f.delta("2", listOf(late)), listOf(late), "0", emptyList(), null) { true }
        assertTrue(f.database.messages().one(f.ns, f.messageId)!!.retracted)
        assertEquals("", f.database.messages().one(f.ns, f.messageId)!!.text)
    } }

    @Test
    fun changedCurrentIdentityOrCapabilityCannotUseLegacyMessagesAndLateApiResult() = runBlocking {
        var ready = false
        var current = true
        var guard = "first"
        var calls = 0
        val raw = object : Api("https://localhost") {
            override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                calls++
                if (path == "/messages/late") guard = "second"
                return JSONObject()
            }
        }
        val api = SyncGuardApi("https://localhost", raw, { current }) { LocalProjectionGuard(guard, ready) }
        assertTrue(runCatching { api.request("/conversations/room/messages") }.exceptionOrNull() is SyncFailure)
        assertEquals(0, calls)
        api.request("/conversations/room/message-results", "POST", JSONObject().put("clientMsgId", "original-id"))
        assertEquals(1, calls)
        ready = true
        assertTrue(runCatching { api.request("/messages/late") }.exceptionOrNull() is kotlinx.coroutines.CancellationException)
        current = false
        assertTrue(runCatching { api.request("/sync/context") }.exceptionOrNull() is kotlinx.coroutines.CancellationException)
        assertEquals(2, calls)
    }

    @Test
    fun exactDeletedResultProtectsEveryHolderBeforeItsSyncEventArrives() = runBlocking { fixture { f ->
        f.install(listOf(f.conversationProjection(), f.projection(f.ref("message", f.messageId), "available", f.body())))
        val older = f.ns.replace(f.device, uuid()) + "|sync-retained"
        val original = f.database.messages().one(f.ns, f.messageId)!!
        f.database.messages().put(original.copy(key = "$older|${original.id}", namespace = older))
        val originalGuard = f.control.getString("guardGeneration")
        val deleted = original.copy(retracted = true, text = "must be erased despite malformed leftover", version = "2", projectionGuard = originalGuard)
        f.database.messages().page(f.ns, f.conversation, "1", "1:0", listOf(deleted), f.user, true)
        f.database.messages().across("live|${f.profile}|${f.deployment}|${f.user}|", f.messageId).forEach {
            assertTrue(it.retracted); assertEquals("", it.text)
        }
        assertEquals(1, f.store.pendingCleanup(f.profile, f.deployment).size)
        assertNotEquals(originalGuard, f.store.ensure(f.identity) { true }.getString("guardGeneration"))
        assertTrue(runCatching {
            f.store.commit(f.identity, f.control, null, emptyList(), f.delta(), emptyList(), "0", emptyList(), null) { true }
        }.exceptionOrNull() is SyncFailure)
    } }

    @Test
    fun auxiliaryResultUnknownCannotBlockVerifiedGlobalAndNeverCreatesMessage() = runBlocking {
        listOf<Exception?>(ApiFailure(503, "DEPENDENCY_UNAVAILABLE"), java.io.IOException("injected transport unknown"), null).forEach { unknown ->
            fixture { f ->
                f.install(listOf(f.conversationProjection(), f.projection(f.ref("message", f.messageId), "available", f.body())))
                val pending = MessageRow("${f.ns}|original", f.ns, f.conversation, "original", "opaque-original-client", f.user,
                    text = "unrelated immutable original", state = "pending", sendAdmission = "original-admission")
                f.database.messages().enqueue(pending)
                val global = f.projection(f.ref("message", f.messageId, "2"), "global", JSONObject().put("id", f.messageId))
                val origin = uuid()
                val event = JSONObject().put("position", "1").put("originEventId", origin).put("ref", global.ref.raw)
                val bytes = event.toString().toByteArray()
                val delta = JSONObject().put("restoreEpoch", "1").put("permissionGeneration", f.permission)
                    .put("snapshotRequired", false).put("qualifiedAck", false).put("position", "1").put("head", "1")
                    .put("cursor", "opaque-one").put("hasMore", false)
                    .put("items", JSONArray().put(JSONObject(event.toString()).put("kind", "reference")
                        .put("eventEnvelope", Base64.getUrlEncoder().withoutPadding().encodeToString(bytes))
                        .put("eventDigest", SyncWire.hash(bytes)).put("projection", f.wireProjection(global))))
                var probes = 0
                var creates = 0
                val raw = object : Api("https://localhost") {
                    override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject = when {
                        path == "/bootstrap" -> JSONObject().put("deploymentId", f.deployment).put("restoreEpoch", "1")
                            .put("features", JSONObject().put("authoritative_sync", true))
                        path == "/sync/device" -> f.identity.binding(f.control.getString("storeGeneration"), f.stream)
                            .put("qualification", "transport_only").put("qualifiedAck", false)
                        path == "/sync/context" -> f.identity.binding(f.control.getString("storeGeneration"), f.stream)
                            .put("qualification", "transport_only").put("qualifiedAck", false).put("afterReceiptEnabled", false)
                            .put("head", "1").put("permissionGeneration", f.permission)
                        path.startsWith("/sync/changes?") -> delta
                        path.endsWith("/message-results") -> {
                            probes++
                            assertTrue(SyncWire.same(originalMessageBody(pending), data))
                            if (unknown != null) throw unknown
                            // A 200 triple (even with an ID) is not a complete committed Message.
                            JSONObject().put("id", uuid()).put("senderId", f.user)
                                .put("conversationId", f.conversation).put("clientMsgId", pending.clientMsgId)
                        }
                        else -> { creates++; error("unexpected request: $method $path") }
                    }
                }
                val installed = NativeSyncEngine(f.database).run(f.identity, raw, null, { true })
                assertEquals(setOf(f.messageId), installed.globalIds)
                assertEquals("1", f.store.ensure(f.identity) { true }.getString("position"))
                assertTrue(f.database.messages().one(f.ns, f.messageId)!!.retracted)
                assertEquals("", f.database.messages().one(f.ns, f.messageId)!!.text)
                assertEquals(pending, f.database.messages().outbox(f.ns).single())
                assertEquals(1, probes); assertEquals(0, creates)
            }
        }
    }

    @Test
    fun globalCleanupSurvivesCommitCrashAndEmptyDeltaAndRetriesFailedSidecarRemoval() = runBlocking {
        val name = "JobsIM-test-authoritative-${uuid()}.db"
        val cache = java.io.File(context.cacheDir, "SyncCleanup-test-${uuid()}")
        var database = ChatDatabase.open(context, false, name)
        try {
            val f = Fixture(database); f.start()
            f.install(listOf(f.conversationProjection(), f.projection(f.ref("message", f.messageId), "available", f.body())))
            val oldNamespace = f.ns.replace(f.device, uuid())
            val held = database.messages().one(f.ns, f.messageId)!!
            val retainedNamespace = "$oldNamespace|sync-retained"
            database.messages().put(held.copy(key = "$retainedNamespace|${f.messageId}", namespace = retainedNamespace))
            val oldFile = SyncMediaCleanup.sidecar(cache, oldNamespace, f.messageId)
            oldFile.parentFile!!.mkdirs(); oldFile.writeText("old device media")
            val global = f.projection(f.ref("message", f.messageId, "2"), "global", JSONObject().put("id", f.messageId))
            f.store.commit(f.identity, f.control, null, emptyList(), f.delta("1", listOf(global)), listOf(global), "0", emptyList(), null) { true }
            assertEquals(1, f.store.pendingCleanup(f.profile, f.deployment).size)
            // Stop before the UI/filesystem cleanup, then reopen and receive no new global event.
            database.close()
            database = ChatDatabase.open(context, false, name)
            val store = SyncStore(database)
            val control = store.ensure(f.identity) { true }
            val installed = store.commit(f.identity, control, null, emptyList(), f.delta("1"), emptyList(), "0", emptyList(), null) { true }
            assertEquals(setOf(f.messageId), installed.globalIds)
            assertTrue(oldFile.exists())
            val blocked = SyncMediaCleanup.sidecar(cache, f.ns, f.messageId)
            blocked.mkdirs()
            val marker = java.io.File(blocked, "owned-deletion-failure")
            marker.writeText("controlled nonempty directory")
            SyncMediaCleanup(database, cache).drain(store.pendingCleanup(f.profile, f.deployment))
            assertFalse(oldFile.exists())
            assertEquals(1, store.pendingCleanup(f.profile, f.deployment).size)
            assertTrue(marker.delete())
            SyncMediaCleanup(database, cache).drain(store.pendingCleanup(f.profile, f.deployment))
            assertFalse(blocked.exists())
            assertTrue(store.pendingCleanup(f.profile, f.deployment).isEmpty())
        } finally {
            database.close(); context.deleteDatabase(name); cache.deleteRecursively()
        }
    }
}
