package com.jobs.im

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.IOException
import java.util.UUID
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.async
import kotlinx.coroutines.CoroutineStart
import kotlinx.coroutines.CompletableDeferred
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class SelfStateTest {
    private fun uuid() = UUID.randomUUID().toString()
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext

    private class Fixture(val database: ChatDatabase) {
        private fun uuid() = UUID.randomUUID().toString()
        val profile = uuid(); val deployment = uuid(); val user = uuid(); val device = uuid(); val family = uuid()
        val conversation = uuid(); val message = uuid()
        val ns = Isolation.namespace(profile, deployment, user, false, "1", "personal", device)
        val identity = SyncIdentity(ns, profile, deployment, user, "1", device, family)
        var live: SyncIdentity? = identity
        var font = 1f
        val writes = mutableListOf<Pair<String, JSONObject>>()
        val resultBodies = mutableListOf<JSONObject>()
        val requests = mutableListOf<String>()
        var dropPreference = false
        var notFound = false
        var preferenceVersion = "0"
        var preset = "standard"
        var currentResult = true
        var sourceVersion = "5"
        var member = "1"
        var manual = true
        var manualGeneration = "4"
        var conflict = false
        var lastPreference = JSONObject()
        var beforePreferenceResponse: (() -> Unit)? = null
        var waitPreferenceResponse: (suspend () -> Unit)? = null

        fun base() = JSONObject().put("scopeId", deployment).put("sourceContextId", "personal").put("userId", user)
            .put("revisionEpoch", "1").put("version", "0")
        fun preference() = base().put("version", preferenceVersion).put("fontPreset", preset).put("explicit", preferenceVersion != "0")
        fun view() = base().put("id", conversation).put("conversationId", conversation).put("pinnedRank", JSONObject.NULL)
            .put("mutedUntil", JSONObject.NULL).put("archived", false).put("hidden", false).put("manualUnread", manual)
            .put("manualUnreadGeneration", manualGeneration).put("manualUnreadMarkedAt", if (manual) "2026-11-16T00:00:00Z" else JSONObject.NULL)
            .put("manualUnreadFrom", JSONObject.NULL)
        fun read() = base().put("id", message).put("conversationId", conversation).put("membershipGeneration", member)
            .put("readPoint", JSONObject().put("epoch", "0").put("sequence", "0")).put("writable", true)
            .put("countStatus", "exact_current_source").put("unreadCount", "9007199254740993000")
            .put("countSourceWatermark", "9007199254740993001").put("continuityStatus", "not_evaluated")
        fun draft() = base().put("id", conversation).put("conversationId", conversation).put("contentState", "owned")
            .put("ownedText", "owned independent text").put("replyState", "none").put("reply", JSONObject.NULL)
            .put("writable", true).put("draftAdmission", "da1.original")
        fun source() = JSONObject().put("id", message).put("conversationId", conversation).put("contentVersion", sourceVersion)
            .put("replyAdmission", "dr1.original")

        val api = object : Api("https://android-self-state.invalid") {
            override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                requests.add(method + " " + path)
                if (path == "/bootstrap") return JSONObject().put("deploymentId", deployment).put("restoreEpoch", "1")
                    .put("features", JSONObject().put("sync_self_state", true).put("cloud_drafts", true))
                if (method == "GET") return when (path) {
                    "/me/preferences" -> { waitPreferenceResponse?.invoke(); beforePreferenceResponse?.invoke(); preference() }
                    "/conversations/$conversation/view" -> view()
                    "/conversations/$conversation/read" -> read()
                    "/conversations/$conversation/draft" -> draft()
                    "/conversations/$conversation" -> JSONObject().put("id", conversation).put("membershipGeneration", member)
                    "/messages/$message" -> source()
                    else -> throw AssertionError("Unexpected GET path")
                }
                val body = JSONObject((data ?: throw AssertionError("Missing original body")).toString())
                if (path.endsWith("-results")) {
                    resultBodies.add(body)
                    if (notFound) throw ApiFailure(404, "PREFERENCE_OPERATION_NOT_FOUND")
                    return JSONObject().put("operationId", body.getString("requestId")).put("status", "applied").put("current", currentResult)
                        .put("result", JSONObject().put("revisionEpoch", "1").put("version", "1").put("fontPreset", body.getString("fontPreset")))
                }
                writes.add(path to body)
                return when (path) {
                    "/me/preferences" -> {
                        lastPreference = body; preset = body.getString("fontPreset"); preferenceVersion = "1"
                        if (dropPreference) throw IOException("Injected lost response")
                        JSONObject().put("operationId", body.getString("requestId")).put("status", "applied").put("current", true)
                            .put("result", JSONObject().put("revisionEpoch", "1").put("version", "1").put("fontPreset", preset))
                    }
                    "/conversations/$conversation/read" -> JSONObject().put("operationId", uuid()).put("status", "applied").put("current", true)
                        .put("result", JSONObject().put("revisionEpoch", "1").put("version", "1").put("membershipGeneration", member)
                            .put("readPoint", JSONObject().put("epoch", "0").put("sequence", "0"))
                            .put("observedMessageIds", body.getJSONArray("observedMessageIds")).put("countStatus", "not_computed")
                            .put("continuityStatus", "gap").put("hasMoreEligible", true).put("hasUnorderedEligible", false))
                    "/conversations/$conversation/view" -> {
                        if (conflict) throw ApiFailure(409, "MANUAL_UNREAD_CHANGED")
                        manual = body.getJSONObject("changes").optBoolean("manualUnread", manual)
                        manualGeneration = "5"
                        JSONObject().put("operationId", body.getString("requestId")).put("status", "applied").put("current", true).put("result", view())
                    }
                    "/conversations/$conversation/draft" -> throw IOException("Injected draft result loss")
                    else -> throw AssertionError("A creation message route must never be called")
                }
            }
        }
        fun model(database: ChatDatabase = this.database) = SelfStateModel(database, { live }, { api }) { font = it }
        suspend fun start() {
            database.messages().putNativeScope(NativeScope(uuid(), "live|$profile|$deployment|", ns, device, "owned test"))
            SyncStore(database).ensure(identity) { true }
        }
    }

    private fun fixture(block: suspend (Fixture, String) -> Unit) = runBlocking {
        val name = "JobsIM-test-self-state-${uuid()}.db"
        val database = ChatDatabase.open(context, false, name)
        try { val f = Fixture(database); f.start(); block(f, name) }
        finally { database.close(); context.deleteDatabase(name) }
    }

    @Test fun schemaUpgradeRetainsPhysicalRegistrationAndSeparatesOriginalSchemaCursor() = fixture { f, _ ->
        val store = SyncStore(f.database)
        var old = store.bind(f.identity, store.ensure(f.identity) { true }, uuid()) { true }
        old.put("cursor", "original-schema1-cursor").put("position", "9007199254740993000")
        f.database.syncRecords().put(SyncRecord("${f.ns}|sync-control", f.ns, "control", old.toString()))
        val upgraded = store.selectSchema(f.identity, true) { true }
        assertEquals(old.getString("storeGeneration"), upgraded.getString("storeGeneration"))
        assertNotEquals(old.getString("registrationRequestId"), upgraded.getString("registrationRequestId"))
        assertFalse(upgraded.has("cursor")); assertEquals("0", upgraded.getString("position"))
        val archived = f.database.syncRecords().rows(f.ns, "schema-history").single()
        assertEquals("original-schema1-cursor", JSONObject(archived.payload).getString("cursor"))
        try { store.selectSchema(f.identity, false) { true }; fail("S2 responsibility must not downgrade") } catch (expected: SyncFailure) { assertEquals("SYNC_SELF_STATE_REQUIRED", expected.reason) }
        val request = store.begin(f.identity, upgraded) { true }.getJSONObject("snapshotRequest")
        assertEquals("2", request.getString("schemaVersion"))
    }

    @Test fun wrongPrincipalOrNumericCountersRollbackSelfProjectionAndCursorTogether() = fixture { f, _ ->
        val store = SelfStateStore(f.database)
        try {
            f.database.withTransaction {
                f.database.syncRecords().put(SyncRecord("atomic-position", f.ns, "test", "1"))
                store.acceptInTransaction(f.identity, "preferences", f.preference().put("version", 1))
            }
            fail("Numeric version must not be accepted")
        } catch (_: SyncFailure) { }
        assertNull(f.database.syncRecords().one("atomic-position"))
        assertTrue(store.values(f.identity).isEmpty())
        assertThrows(SyncFailure::class.java) { SelfStateWire.validate("preferences", f.preference().put("userId", uuid()), f.identity) }
        assertThrows(SyncFailure::class.java) { SelfStateWire.validate("read_state", f.read().put("unreadCount", 1), f.identity, f.conversation) }
        assertEquals("9007199254740993000", SyncWire.counter(f.read().opt("unreadCount")))
    }

    @Test fun unknownPreferenceSurvivesColdReopenAndReadonlyResultNeverReplaysWrite() = fixture { f, name ->
        f.dropPreference = true
        try { f.model().setFont("large"); fail("Injected result loss") } catch (_: IOException) { }
        val original = SelfStateStore(f.database).pending(f.identity).single()
        val originalBytes = original.body.toString()
        f.database.close()
        val reopened = ChatDatabase.open(context, false, name)
        try {
            val model = f.model(reopened)
            model.activate(true, true)
            assertEquals(originalBytes, model.pending.single().body.toString())
            f.preset = "small"; f.preferenceVersion = "3"; f.currentResult = false
            model.recover(model.pending.single())
            assertEquals(1, f.writes.size)
            assertTrue(SyncWire.same(JSONObject(originalBytes), f.resultBodies.single()))
            assertEquals(.85f, f.font, .001f)
            assertTrue(model.pending.isEmpty())
        } finally { reopened.close() }
    }

    @Test fun unknownNotFoundRequiresExplicitRetryAndUsesIdenticalOriginalRequest() = fixture { f, _ ->
        f.dropPreference = true
        val model = f.model()
        try { model.setFont("large") } catch (_: IOException) { }
        val original = model.pending.single()
        f.notFound = true
        try { model.recover(original); fail("404 remains unknown") } catch (expected: ApiFailure) { assertEquals(404, expected.status) }
        assertEquals(1, f.writes.size)
        f.dropPreference = false
        model.recover(model.pending.single(), true)
        assertEquals(2, f.writes.size)
        assertTrue(SyncWire.same(f.writes[0].second, f.writes[1].second))
        assertEquals(2, f.resultBodies.size)
    }

    @Test fun sparseVisibleObservationDoesNotClearIndependentManualUnreadAndConflictFreezesOriginal() = fixture { f, _ ->
        val model = f.model(); model.activate(true, true); model.refresh(f.conversation)
        val unseen = uuid()
        f.database.syncRecords().put(SyncRecord("${f.ns}|sync-unread|${f.conversation}", f.ns, "unread", JSONObject().put("legacy", 0).put("ids", JSONArray(listOf(f.message, unseen))).toString()))
        f.database.messages().putState(ConversationState("${f.ns}|${f.conversation}", f.ns, f.conversation, "1:0", unread = 2))
        val c = Conversation(f.conversation, "fixture", membershipGeneration = "1")
        model.observe(c, listOf(f.message))
        assertEquals(listOf(f.message), f.writes.single().second.getJSONArray("observedMessageIds").let { (0 until it.length()).map(it::getString) })
        assertTrue(model.views.getValue(f.conversation).getBoolean("manualUnread"))
        assertEquals(1, f.database.messages().state(f.ns, f.conversation)?.unread)
        assertEquals("9007199254740993000", model.reads.getValue(f.conversation).getString("unreadCount"))
        f.conflict = true
        try { model.changeView(f.conversation, JSONObject().put("manualUnread", false)); fail("Old generation must reject") } catch (_: ApiFailure) { }
        val pending = model.pending.single()
        assertEquals("4", pending.body.getJSONObject("changes").getString("expectedManualUnreadGeneration"))
        assertTrue(pending.status.startsWith("rejected:"))
        assertTrue(model.views.getValue(f.conversation).getBoolean("manualUnread"))
    }

    @Test fun draftCapturesIndependentTextAndDistinctOriginalProofsWithoutChangingLocalMessageOutbox() = fixture { f, _ ->
        val queue = MessageRow("${f.ns}|local", f.ns, f.conversation, "local", "original-client-id", f.user,
            text = "original message intent", state = "pending", sendAdmission = "sa1.original")
        f.database.messages().put(queue)
        val model = f.model(); model.activate(true, true)
        val reply = queue.copy(id = f.message, state = "received", contentVersion = "5")
        try { model.saveDraft(f.conversation, "new independent text", reply); fail("Injected response loss") } catch (_: IOException) { }
        val original = model.pending.single().body
        assertEquals("da1.original", original.getString("draftAdmission"))
        assertEquals("dr1.original", original.getJSONObject("reply").getString("replyAdmission"))
        assertEquals("5", original.getJSONObject("reply").getString("sourceVersion"))
        assertEquals("new independent text", original.getString("ownedText"))
        assertEquals(queue, f.database.messages().outbox(f.ns).single())
        assertFalse(SelfStateStore(f.database).values(f.identity).single { it.opt("_kind") == "draft" }.has("draftAdmission"))
    }

    @Test fun changedDraftReplyRequiresReselectionAndCreatesNoCloudWrite() = fixture { f, _ ->
        f.sourceVersion = "6"
        val model = f.model(); model.activate(true, true)
        val reply = MessageRow("reply", f.ns, f.conversation, f.message, senderId = f.user, text = "old displayed body", contentVersion = "5")
        try { model.saveDraft(f.conversation, "independent", reply); fail("Changed source must require reselection") }
        catch (expected: SyncFailure) { assertEquals("DRAFT_REPLY_SOURCE_CHANGED", expected.reason) }
        assertTrue(f.writes.isEmpty()); assertTrue(model.pending.isEmpty())
    }

    @Test fun lateIdentityOrChangedProjectionGuardCannotPublishSelfState() = fixture { f, _ ->
        val guard = SyncStore(f.database).projectionGuard(f.ns)
        val original = f.preference()
        SyncStore(f.database).restrict(f.identity, "test restriction", false) { true }
        try { SelfStateStore(f.database).accept(f.identity, "preferences", original, "", guard) { true }; fail("Late guard must reject") }
        catch (expected: SyncFailure) { assertEquals("SYNC_LOCAL_RESTRICTION_CHANGED", expected.reason) }
        f.live = null
        try { SelfStateStore(f.database).prepare(f.identity, "preferences", "", "/me/preferences", "PATCH", JSONObject().put("requestId", uuid()).put("revisionEpoch", "1")) { false }; fail("Old identity must not prepare") }
        catch (expected: SyncFailure) { assertEquals("SELF_STATE_CONTEXT_CHANGED", expected.reason) }
        assertTrue(SelfStateStore(f.database).values(f.identity).isEmpty())
    }

    @Test fun firstActivationRemainsEnabledAndSameIdentityResetRejectsLateResponse() = fixture { f, _ ->
        val model = f.model()
        model.activate(true, true)
        assertTrue(model.enabled); assertTrue(model.draftEnabled)
        f.beforePreferenceResponse = { model.reset() }
        try { model.refresh(); fail("Same-identity reset must reject the earlier request") }
        catch (_: kotlinx.coroutines.CancellationException) { }
        assertFalse(model.enabled); assertFalse(model.draftEnabled)
        assertTrue(SelfStateStore(f.database).values(f.identity).isEmpty())
    }

    @Test fun s2ProjectionAndCursorShareTransactionAndWrongSubjectCannotPartiallyPublish() = fixture { f, _ ->
        val store = SyncStore(f.database)
        var control = store.bind(f.identity, store.selectSchema(f.identity, true) { true }, uuid()) { true }
        control.put("cursor", "s2-original").put("permissionGeneration", "a".repeat(64))
        control.remove("restriction")
        f.database.syncRecords().put(SyncRecord("${f.ns}|sync-control", f.ns, "control", control.toString()))
        fun projection(subject: String, preset: String): SyncProjection {
            val ref = SyncRef(JSONObject().put("owner", "directory").put("kind", "preferences").put("id", f.user)
                .put("subjectUserId", subject).put("revision", "1").put("revisionEpoch", "1").put("state", "active"))
            val body = f.preference().put("fontPreset", preset).put("version", "1")
            return SyncProjection(ref, "available", "1", "1", body, SyncWire.hash(body.toString().toByteArray()))
        }
        val good = projection(f.user, "large")
        store.commit(f.identity, control, null, emptyList(), SyncDelta("s2-position-1", "1", "1", false, listOf(good), JSONArray()), listOf(good), "0", emptyList(), null) { true }
        control = store.ensure(f.identity) { true }
        assertEquals("s2-position-1", control.getString("cursor"))
        assertEquals("large", SelfStateStore(f.database).values(f.identity).single().getString("fontPreset"))
        val bad = projection(uuid(), "small")
        try { store.commit(f.identity, control, null, emptyList(), SyncDelta("s2-position-2", "2", "2", false, listOf(bad), JSONArray()), listOf(bad), "0", emptyList(), null) { true }; fail("Wrong subject must rollback") }
        catch (expected: SyncFailure) { assertEquals("SELF_STATE_CONTEXT_CHANGED", expected.reason) }
        assertEquals("s2-position-1", store.ensure(f.identity) { true }.getString("cursor"))
        assertEquals("large", SelfStateStore(f.database).values(f.identity).single().getString("fontPreset"))
    }

    @Test fun queuedUiIntentCapturesLifecycleBeforeMutexAndCannotAdoptResetIdentity() = fixture { f, _ ->
        kotlinx.coroutines.coroutineScope {
            val entered = CompletableDeferred<Unit>(); val release = CompletableDeferred<Unit>()
            val model = f.model(); model.activate(true, true)
            f.waitPreferenceResponse = { entered.complete(Unit); release.await() }
            val first = async(start = CoroutineStart.UNDISPATCHED) { runCatching { model.setFont("large") }.exceptionOrNull() }
            entered.await()
            val queued = async(start = CoroutineStart.UNDISPATCHED) { runCatching { model.changeView(f.conversation, JSONObject().put("archived", true)) }.exceptionOrNull() }
            val originalRequestCount = f.requests.size
            model.reset()
            release.complete(Unit)
            assertTrue(first.await() is kotlinx.coroutines.CancellationException)
            assertTrue(queued.await() is kotlinx.coroutines.CancellationException)
            assertEquals(originalRequestCount, f.requests.size)
            assertTrue(f.writes.isEmpty())
            assertTrue(SelfStateStore(f.database).pending(f.identity).isEmpty())
        }
    }
}
