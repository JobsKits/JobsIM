package com.jobs.im

import androidx.compose.runtime.*
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONArray
import org.json.JSONObject

class SelfStateModel(
    private val database: ChatDatabase,
    private val identity: () -> SyncIdentity?,
    private val transport: () -> Api,
    private val fontChanged: (Float) -> Unit,
) {
    private val store = SelfStateStore(database)
    private val mutex = Mutex()
    val views = mutableStateMapOf<String, JSONObject>()
    val reads = mutableStateMapOf<String, JSONObject>()
    val drafts = mutableStateMapOf<String, JSONObject>()
    val observations = mutableStateMapOf<String, JSONObject>()
    val pending = mutableStateListOf<SelfIntent>()
    var enabled by mutableStateOf(false)
        private set
    var draftEnabled by mutableStateOf(false)
        private set
    var problem by mutableStateOf("")
        private set
    private var preferences: JSONObject? = null
    private var owner = ""
    private var lifecycle = 0
    private data class Bound(val identity: SyncIdentity, val lifecycle: Int) {
        val namespace get() = identity.namespace
        val deployment get() = identity.deployment
        val epoch get() = identity.epoch
    }
    private fun capture(): Bound? = identity()?.let { Bound(it, lifecycle) }
    private val observed = mutableSetOf<String>()

    fun reset() {
        lifecycle++
        views.clear(); reads.clear(); drafts.clear(); observations.clear(); pending.clear()
        preferences = null; owner = ""; observed.clear(); enabled = false; draftEnabled = false; problem = ""
    }

    private fun current(original: Bound): Boolean = identity() == original.identity && lifecycle == original.lifecycle
    private fun requireCurrent(original: Bound) { if (!current(original)) throw CancellationException("SELF_STATE_CONTEXT_CHANGED") }

    private suspend fun capability(original: Bound, draft: Boolean = false): Api {
        val api = transport()
        val bootstrap = api.request("/bootstrap")
        requireCurrent(original)
        if (bootstrap.opt("deploymentId") != original.deployment || bootstrap.opt("restoreEpoch") != original.epoch ||
            bootstrap.optJSONObject("features")?.opt("sync_self_state") != true ||
            (draft && bootstrap.optJSONObject("features")?.opt("cloud_drafts") != true)) SyncWire.fail("SELF_STATE_CAPABILITY_REQUIRED")
        return api
    }

    suspend fun restore() {
        val original = capture() ?: return
        val rows = store.values(original.identity); val intents = store.pending(original.identity)
        requireCurrent(original)
        if (owner != original.namespace) {
            views.clear(); reads.clear(); drafts.clear(); observations.clear(); pending.clear()
            preferences = null; observed.clear()
        }
        owner = original.namespace
        views.clear(); reads.clear(); drafts.clear(); observations.clear()
        for (value in rows) when (value.opt("_kind")) {
            "preferences" -> { preferences = value; applyFont(value.getString("fontPreset")) }
            "conversation_view" -> views[value.getString("conversationId")] = value
            "read_state" -> reads[value.getString("conversationId")] = value
            "read_observation" -> observations[value.getString("id")] = value
            "draft" -> drafts[value.getString("conversationId")] = value
        }
        pending.clear(); pending.addAll(intents)
        // Sparse observations are applicable only to their captured membership, not the next join.
        observations.values.filter { it.opt("currentlyApplicable") == true }.forEach {
            observed.add("${original.namespace}|${it.getString("conversationId")}|${it.getString("membershipGeneration")}|${it.getString("messageId")}")
        }
    }

    suspend fun activate(selfState: Boolean, cloudDrafts: Boolean) {
        enabled = selfState; draftEnabled = selfState && cloudDrafts
        if (selfState) restore()
    }

    private fun applyFont(preset: String) = fontChanged(when (preset) { "small" -> .85f; "large" -> 1.2f; else -> 1f })

    private suspend fun serialized(action: suspend (Bound) -> Unit) {
        val original = capture() ?: return
        mutex.withLock {
            requireCurrent(original)
            action(original)
        }
    }

    suspend fun refresh(conversation: String? = null) = serialized { original ->
        val api = capability(original)
        readCurrent(original, api, "preferences", "")
        if (conversation != null) {
            readCurrent(original, api, "conversation_view", conversation)
            readCurrent(original, api, "read_state", conversation)
            if (draftEnabled) readCurrent(original, api, "draft", conversation)
        }
        restore()
    }

    private fun path(kind: String, conversation: String): String = when (kind) {
        "preferences" -> "/me/preferences"
        "conversation_view" -> "/conversations/$conversation/view"
        "read_state" -> "/conversations/$conversation/read"
        "draft" -> "/conversations/$conversation/draft"
        else -> SyncWire.fail()
    }

    private suspend fun readCurrent(original: Bound, api: Api, kind: String, conversation: String): JSONObject {
        val guard = SyncStore(database).projectionGuard(original.namespace)
        val value = api.request(path(kind, conversation))
        requireCurrent(original)
        SelfStateWire.validate(kind, value, original.identity, conversation)
        store.accept(original.identity, kind, value, conversation, guard) { current(original) }
        return value
    }

    private suspend fun execute(original: Bound, intent: SelfIntent, api: Api, retry: Boolean = false) {
        requireCurrent(original)
        if (intent.status.startsWith("rejected:")) SyncWire.fail("SELF_OPERATION_REJECTED")
        val attempted = store.status(original.identity, intent, "unknown") { current(original) }
        try {
            suspend fun write() = api.request(attempted.path, attempted.method, JSONObject(attempted.body.toString()))
            val response = if (intent.status == "prepared") write() else try {
                api.request(attempted.path + "-results", "POST", if (attempted.kind == "draft")
                    JSONObject().put("method", attempted.method).put("body", JSONObject(attempted.body.toString())) else JSONObject(attempted.body.toString()))
            } catch (error: ApiFailure) {
                requireCurrent(original)
                val notFound = mapOf("preferences" to "PREFERENCE_OPERATION_NOT_FOUND", "conversation_view" to "VIEW_OPERATION_NOT_FOUND", "read_state" to "READ_OPERATION_NOT_FOUND", "draft" to "DRAFT_OPERATION_NOT_FOUND")
                if (retry && error.status == 404 && error.code == notFound[attempted.kind]) write() else throw error
            }
            requireCurrent(original)
            if (response.opt("status") == "prepared") {
                if (attempted.kind != "draft") SyncWire.fail("SELF_RESULT_INVALID")
                problem = "SELF_OPERATION_PENDING"
                return
            }
            store.finish(original.identity, attempted, response) { current(original) }
            problem = ""
            readCurrent(original, api, attempted.kind, attempted.conversation)
        } catch (error: Exception) {
            requireCurrent(original)
            if (error is CancellationException) throw error
            val code = if (error is ApiFailure) error.code else if (error is SyncFailure) error.reason else "SELF_OPERATION_UNKNOWN"
            if (code in setOf("PREFERENCE_VERSION_CONFLICT", "VIEW_VERSION_CONFLICT", "MANUAL_UNREAD_CHANGED", "QUALIFICATION_CHANGED", "RESTORE_EPOCH_CHANGED", "DRAFT_ADMISSION_CHANGED", "IDEMPOTENCY_CONFLICT"))
                store.status(original.identity, attempted, "rejected:$code") { current(original) }
            problem = code
            throw error
        } finally {
            if (current(original)) restore()
        }
    }

    suspend fun recover(intent: SelfIntent, retry: Boolean = false) = serialized { original ->
        val api = capability(original, intent.kind == "draft")
        execute(original, intent, api, retry)
    }

    suspend fun archiveRejected(intent: SelfIntent) = serialized { original ->
        store.archiveRejected(original.identity, intent) { current(original) }
        restore()
    }

    private suspend fun prepare(original: Bound, kind: String, conversation: String, method: String, body: JSONObject): SelfIntent {
        val prior = store.pending(original.identity).firstOrNull { it.kind == kind && it.conversation == conversation }
        if (prior != null) SyncWire.fail("SELF_OPERATION_PENDING")
        return store.prepare(original.identity, kind, conversation, path(kind, conversation), method, body) { current(original) }
    }

    suspend fun setFont(preset: String) = serialized { original ->
        val api = capability(original)
        val latest = readCurrent(original, api, "preferences", "")
        val body = JSONObject().put("requestId", UUID.randomUUID().toString()).put("revisionEpoch", original.epoch)
            .put("expectedVersion", latest.getString("version")).put("fontPreset", preset)
        execute(original, prepare(original, "preferences", "", "PATCH", body), api)
    }

    suspend fun changeView(conversation: String, changes: JSONObject) = serialized { original ->
        val api = capability(original)
        val latest = readCurrent(original, api, "conversation_view", conversation)
        val copied = JSONObject(changes.toString())
        if (copied.has("manualUnread")) copied.put("expectedManualUnreadGeneration", latest.getString("manualUnreadGeneration"))
        val body = JSONObject().put("requestId", UUID.randomUUID().toString()).put("revisionEpoch", original.epoch)
            .put("expectedVersion", latest.getString("version")).put("changes", copied)
        execute(original, prepare(original, "conversation_view", conversation, "PATCH", body), api)
    }

    suspend fun observe(conversation: Conversation, ids: List<String>) = serialized { original ->
        if (!enabled || ids.isEmpty()) return@serialized
        val api = capability(original)
        val previous = store.pending(original.identity).firstOrNull { it.kind == "read_state" && it.conversation == conversation.id }
        if (previous != null) { execute(original, previous, api); return@serialized }
        val metadata = api.request("/conversations/${conversation.id}")
        requireCurrent(original)
        if (metadata.opt("id") != conversation.id) SyncWire.fail("SELF_STATE_CONTEXT_CHANGED")
        val member = SyncWire.counter(metadata.opt("membershipGeneration"))
        if (member == "0") SyncWire.fail("QUALIFICATION_CHANGED")
        val visible = ids.distinct().filter { "${original.namespace}|${conversation.id}|$member|$it" !in observed }.take(200)
        if (visible.isEmpty()) return@serialized
        visible.forEach { SyncWire.id(it) }
        val state = readCurrent(original, api, "read_state", conversation.id)
        if (state.opt("writable") != true || state.opt("membershipGeneration") != member) SyncWire.fail("QUALIFICATION_CHANGED")
        val body = JSONObject().put("requestId", UUID.randomUUID().toString()).put("revisionEpoch", original.epoch)
            .put("membershipGeneration", member).put("observedMessageIds", JSONArray(visible))
        execute(original, prepare(original, "read_state", conversation.id, "PUT", body), api)
        visible.forEach { observed.add("${original.namespace}|${conversation.id}|$member|$it") }
    }

    suspend fun saveDraft(conversation: String, text: String, reply: MessageRow?, clear: Boolean = false) = serialized { original ->
        if (text.toByteArray().size > 16384) SyncWire.fail("DRAFT_TEXT_TOO_LARGE")
        val api = capability(original, true)
        val latest = readCurrent(original, api, "draft", conversation)
        if (latest.opt("writable") != true) SyncWire.fail("DRAFT_NOT_WRITABLE")
        val admission = SyncWire.text(latest.opt("draftAdmission"))
        val body = JSONObject().put("requestId", UUID.randomUUID().toString()).put("revisionEpoch", original.epoch)
            .put("expectedVersion", latest.getString("version")).put("draftAdmission", admission)
        if (!clear) {
            var reference: Any = JSONObject.NULL
            if (reply != null) {
                val source = api.request("/messages/${reply.id}")
                requireCurrent(original)
                if (source.opt("id") != reply.id || source.opt("conversationId") != conversation || source.opt("deleted") == true ||
                    source.opt("contentUnavailable") == true || source.opt("forwardUnavailable") == true) SyncWire.fail("DRAFT_REPLY_UNAVAILABLE")
                if (source.opt("contentVersion") != reply.contentVersion) SyncWire.fail("DRAFT_REPLY_SOURCE_CHANGED")
                reference = JSONObject().put("messageId", reply.id).put("sourceVersion", SyncWire.counter(source.opt("contentVersion")))
                    .put("replyAdmission", SyncWire.text(source.opt("replyAdmission")))
            }
            body.put("ownedText", text).put("reply", reference)
        }
        execute(original, prepare(original, "draft", conversation, if (clear) "DELETE" else "PUT", body), api)
    }

    fun isMuted(conversation: String): Boolean = views[conversation]?.optString("mutedUntil")?.let {
        runCatching { Instant.parse(it).isAfter(Instant.now()) }.getOrDefault(false)
    } ?: false
}
