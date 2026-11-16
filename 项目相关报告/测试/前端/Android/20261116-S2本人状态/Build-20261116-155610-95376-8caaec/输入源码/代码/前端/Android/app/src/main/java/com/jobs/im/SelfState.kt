package com.jobs.im

import java.math.BigInteger
import java.time.Instant
import java.time.OffsetDateTime
import java.util.UUID
import org.json.JSONArray
import org.json.JSONObject

object SelfStateWire {
    val kinds = setOf("preferences", "conversation_view", "read_state", "read_observation", "draft")
    private val included = setOf("own_font_preference", "own_conversation_views", "own_read_points", "own_read_observation_metadata")

    fun coverage(value: JSONObject) {
        val list = value.optJSONArray("included") ?: SyncWire.fail()
        val names = (0 until list.length()).map { SyncWire.text(list.opt(it), 96) }.toSet()
        if (value.opt("schemaVersion") != "2" || !names.containsAll(included) ||
            value.opt("absence") != "unknown" || value.opt("classification") != "explicit_known_only") SyncWire.fail("SYNC_SCHEMA_CHANGED")
    }

    fun point(value: Any?): JSONObject {
        val p = value as? JSONObject ?: SyncWire.fail()
        val epoch = SyncWire.counter(p.opt("epoch")); val sequence = SyncWire.counter(p.opt("sequence"))
        if ((epoch == "0") != (sequence == "0")) SyncWire.fail()
        return p
    }

    fun validate(kind: String, value: JSONObject, identity: SyncIdentity, conversation: String = "", id: String = "") {
        if (kind !in kinds || value.opt("scopeId") != identity.deployment || value.opt("userId") != identity.user ||
            value.opt("sourceContextId") != "personal" || value.opt("revisionEpoch") != identity.epoch) SyncWire.fail("SELF_STATE_CONTEXT_CHANGED")
        if (conversation.isNotEmpty() && value.opt("conversationId") != conversation) SyncWire.fail("SELF_STATE_CONTEXT_CHANGED")
        if (kind != "preferences") {
            SyncWire.id(value.opt("conversationId")); SyncWire.id(value.opt("id"))
            if (id.isNotEmpty() && value.opt("id") != id) SyncWire.fail("SELF_STATE_CONTEXT_CHANGED")
        } else if (id.isNotEmpty() && id != identity.user) SyncWire.fail("SELF_STATE_CONTEXT_CHANGED")
        if (value.opt("removed") == true) return
        SyncWire.counter(value.opt("version"))
        when (kind) {
            "preferences" -> {
                if (value.opt("fontPreset") !in setOf("small", "standard", "large")) SyncWire.fail()
                SyncWire.flag(value.opt("explicit"))
            }
            "conversation_view" -> {
                if (listOf("pinnedRank", "mutedUntil", "manualUnreadMarkedAt", "manualUnreadFrom").any { !value.has(it) }) SyncWire.fail()
                listOf("archived", "hidden", "manualUnread").forEach { SyncWire.flag(value.opt(it)) }
                SyncWire.counter(value.opt("manualUnreadGeneration"))
                if (!value.isNull("pinnedRank") && (value.opt("pinnedRank") !is Int || value.getInt("pinnedRank") !in 0..999999)) SyncWire.fail()
                if (!value.isNull("mutedUntil")) utc(value.opt("mutedUntil"))
                if (!value.isNull("manualUnreadMarkedAt")) utc(value.opt("manualUnreadMarkedAt"))
                if (!value.isNull("manualUnreadFrom")) point(value.opt("manualUnreadFrom"))
                if (value.getBoolean("manualUnread") == value.isNull("manualUnreadMarkedAt")) SyncWire.fail()
            }
            "read_state" -> {
                SyncWire.counter(value.opt("membershipGeneration")); point(value.opt("readPoint"))
                SyncWire.flag(value.opt("writable")); SyncWire.text(value.opt("continuityStatus"), 96)
                when (value.opt("countStatus")) {
                    "not_computed" -> if (value.has("unreadCount") || value.has("countSourceWatermark")) SyncWire.fail()
                    "exact_current_source" -> { SyncWire.counter(value.opt("unreadCount")); SyncWire.counter(value.opt("countSourceWatermark")) }
                    else -> SyncWire.fail()
                }
            }
            "read_observation" -> {
                SyncWire.id(value.opt("messageId")); SyncWire.counter(value.opt("membershipGeneration"))
                SyncWire.counter(value.opt("observedInEpoch")); utc(value.opt("observedAt"))
                listOf("orderKnown", "eligibleAtObservation", "currentlyApplicable").forEach { SyncWire.flag(value.opt(it)) }
                if (value.opt("countStatus") != "not_computed") SyncWire.fail()
                if (value.getBoolean("orderKnown")) {
                    if (SyncWire.counter(value.opt("creationEpoch")) == "0" || SyncWire.counter(value.opt("creationSequence")) == "0") SyncWire.fail()
                } else if (value.opt("creationEpoch") != "unknown" || value.opt("creationSequence") != "unknown") SyncWire.fail()
            }
            "draft" -> {
                if (!value.has("ownedText") || !value.has("reply")) SyncWire.fail()
                if (value.opt("contentState") !in setOf("empty", "owned", "unavailable") || value.opt("replyState") !in setOf("none", "available", "unavailable")) SyncWire.fail()
                SyncWire.flag(value.opt("writable"))
                if (value.has("draftAdmission")) SyncWire.text(value.opt("draftAdmission"))
                if (!value.isNull("ownedText") && (value.opt("ownedText") !is String || value.getString("ownedText").toByteArray().size > 16384)) SyncWire.fail()
                value.optJSONObject("reply")?.let { SyncWire.id(it.opt("messageId")); SyncWire.counter(it.opt("sourceVersion")) }
                value.optJSONObject("recovery")?.let {
                    SyncWire.clientMessageId(it.opt("operationId"))
                    if (it.opt("ownedText") !is String || it.getString("ownedText").toByteArray().size > 16384) SyncWire.fail()
                }
            }
        }
    }

    fun utc(value: Any?): Instant {
        val text = SyncWire.text(value, 64)
        if (!text.matches(Regex("\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}(?:\\.\\d{1,9})?(?:Z|[+-]\\d{2}:\\d{2})"))) SyncWire.fail()
        return runCatching { OffsetDateTime.parse(text).toInstant() }.getOrElse { SyncWire.fail() }
    }
}

data class SelfIntent(val key: String, val value: JSONObject) {
    val kind get() = value.getString("kind")
    val conversation get() = value.optString("conversationId")
    val body get() = value.getJSONObject("body")
    val requestId get() = body.getString("requestId")
    val path get() = value.getString("path")
    val method get() = value.getString("method")
    val status get() = value.getString("status")
}

class SelfStateStore(private val database: ChatDatabase) {
    private val records get() = database.syncRecords()
    private fun key(identity: SyncIdentity, kind: String, conversation: String) = "${identity.namespace}|self|$kind|$conversation"

    private suspend fun check(identity: SyncIdentity, current: () -> Boolean) {
        identity.check()
        if (!current() || database.messages().nativeScope(identity.namespace)?.namespace != identity.namespace) SyncWire.fail("SELF_STATE_CONTEXT_CHANGED")
    }

    private suspend fun checkedIntent(identity: SyncIdentity, intent: SelfIntent): JSONObject {
        val row = records.one(intent.key) ?: SyncWire.fail("SELF_INTENT_CHANGED")
        val saved = JSONObject(row.payload)
        if (row.namespace != identity.namespace || !intent.key.startsWith(identity.namespace + "|self|") ||
            saved.opt("deviceId") != identity.device || saved.opt("familyId") != identity.family ||
            saved.getJSONObject("body").opt("revisionEpoch") != identity.epoch || !SyncWire.same(saved.opt("body"), intent.body) ||
            saved.opt("path") != intent.path || saved.opt("method") != intent.method) SyncWire.fail("SELF_INTENT_CHANGED")
        return saved
    }

    // Called by the sync installer on the same Room writer as projections and cursor.
    suspend fun acceptInTransaction(identity: SyncIdentity, kind: String, value: JSONObject) {
        SelfStateWire.validate(kind, value, identity)
        val objectKey = key(identity, kind, if (kind == "read_observation") value.getString("id") else value.optString("conversationId"))
        val old = records.one(objectKey)?.let { JSONObject(it.payload) }
        if (value.opt("removed") == true) { records.remove(objectKey); return }
        if (old != null && BigInteger(old.getString("version")) > BigInteger(value.getString("version"))) return
        val saved = JSONObject(value.toString()).put("_kind", kind)
        if (kind == "draft") {
            saved.remove("draftAdmission")
            saved.optJSONObject("reply")?.let { reply ->
                reply.remove("replyAdmission")
                if (records.one("live|${identity.profile}|${identity.deployment}|${identity.user}|personal|global|${reply.getString("messageId")}") != null)
                    saved.put("replyState", "unavailable").put("reply", JSONObject.NULL)
            }
        }
        records.put(SyncRecord(objectKey, identity.namespace, "self", saved.toString()))
    }

    suspend fun accept(identity: SyncIdentity, kind: String, value: JSONObject, conversation: String, guard: LocalProjectionGuard?, current: () -> Boolean) = database.withTransaction {
        check(identity, current)
        if (SyncStore(database).projectionGuard(identity.namespace) != guard) SyncWire.fail("SYNC_LOCAL_RESTRICTION_CHANGED")
        SelfStateWire.validate(kind, value, identity, conversation)
        acceptInTransaction(identity, kind, value)
        if (!current()) SyncWire.fail("SELF_STATE_CONTEXT_CHANGED")
    }

    suspend fun values(identity: SyncIdentity): List<JSONObject> = records.rows(identity.namespace, "self").map { JSONObject(it.payload) }
    suspend fun pending(identity: SyncIdentity): List<SelfIntent> = records.rows(identity.namespace, "self-intent").map { SelfIntent(it.key, JSONObject(it.payload)) }

    suspend fun invalidateCountInTransaction(identity: SyncIdentity, conversation: String) {
        val k = key(identity, "read_state", conversation)
        records.one(k)?.let {
            val body = JSONObject(it.payload).put("countStatus", "not_computed")
            body.remove("unreadCount"); body.remove("countSourceWatermark")
            records.put(it.copy(payload = body.toString()))
        }
    }

    suspend fun archiveRejected(identity: SyncIdentity, intent: SelfIntent, current: () -> Boolean) = database.withTransaction {
        check(identity, current)
        val saved = checkedIntent(identity, intent)
        if (!saved.getString("status").startsWith("rejected:") || !SyncWire.same(saved.opt("body"), intent.body)) SyncWire.fail("SELF_OPERATION_PENDING")
        records.put(SyncRecord(intent.key + "|" + intent.requestId, identity.namespace, "self-history", saved.toString()))
        records.remove(intent.key)
    }

    suspend fun prepare(identity: SyncIdentity, kind: String, conversation: String, path: String, method: String, body: JSONObject, current: () -> Boolean): SelfIntent = database.withTransaction {
        check(identity, current)
        if (kind !in setOf("preferences", "conversation_view", "read_state", "draft")) SyncWire.fail()
        val expectedPath = if (kind == "preferences") "/me/preferences" else "/conversations/${SyncWire.id(conversation)}/" +
            when (kind) { "conversation_view" -> "view"; "read_state" -> "read"; else -> "draft" }
        val allowedMethod = when (kind) { "read_state" -> setOf("PUT"); "draft" -> setOf("PUT", "DELETE"); else -> setOf("PATCH") }
        if (path != expectedPath || method !in allowedMethod) SyncWire.fail("SELF_INTENT_CHANGED")
        val intentKey = "${key(identity, kind, conversation)}|intent"
        records.one(intentKey)?.let {
            val original = SelfIntent(intentKey, JSONObject(it.payload))
            if (!SyncWire.same(original.body, body) || original.path != path || original.method != method) SyncWire.fail("SELF_OPERATION_PENDING")
            return@withTransaction original
        }
        if (body.opt("revisionEpoch") != identity.epoch) SyncWire.fail("RESTORE_EPOCH_CHANGED")
        SyncWire.clientMessageId(body.opt("requestId"))
        val value = JSONObject().put("kind", kind).put("conversationId", conversation).put("path", path)
            .put("method", method).put("body", JSONObject(body.toString())).put("status", "prepared")
            .put("deviceId", identity.device).put("familyId", identity.family)
        records.put(SyncRecord(intentKey, identity.namespace, "self-intent", value.toString()))
        SelfIntent(intentKey, value)
    }

    suspend fun status(identity: SyncIdentity, intent: SelfIntent, status: String, current: () -> Boolean): SelfIntent = database.withTransaction {
        check(identity, current)
        val saved = checkedIntent(identity, intent)
        if (!SyncWire.same(saved.opt("body"), intent.body) || saved.opt("path") != intent.path || saved.opt("method") != intent.method || saved.opt("deviceId") != identity.device || saved.opt("familyId") != identity.family) SyncWire.fail("SELF_INTENT_CHANGED")
        saved.put("status", status)
        records.put(SyncRecord(intent.key, identity.namespace, "self-intent", saved.toString()))
        SelfIntent(intent.key, saved)
    }

    suspend fun finish(identity: SyncIdentity, intent: SelfIntent, result: JSONObject, current: () -> Boolean) = database.withTransaction {
        check(identity, current)
        val saved = checkedIntent(identity, intent)
        val allowedStatus = if (intent.kind == "draft") setOf("applied", "retired") else setOf("applied")
        if (!SyncWire.same(saved.opt("body"), intent.body) || result.opt("status") !in allowedStatus || result.opt("current") !is Boolean || result.optJSONObject("result") == null) SyncWire.fail("SELF_RESULT_INVALID")
        if (intent.kind in setOf("preferences", "conversation_view")) {
            if (result.opt("operationId") != intent.requestId) SyncWire.fail("SELF_RESULT_INVALID")
        } else SyncWire.id(result.opt("operationId"))
        val metadata = result.getJSONObject("result")
        if (metadata.opt("revisionEpoch") != identity.epoch) SyncWire.fail("SELF_RESULT_INVALID")
        SyncWire.counter(metadata.opt("version"))
        if (intent.kind == "preferences" && metadata.opt("fontPreset") != intent.body.opt("fontPreset")) SyncWire.fail("SELF_RESULT_INVALID")
        if (intent.kind == "conversation_view") SelfStateWire.validate("conversation_view", metadata, identity, intent.conversation)
        if (intent.kind == "draft" && (metadata.opt("contentState") !in setOf("empty", "owned", "unavailable") ||
                metadata.opt("replyState") !in setOf("none", "available", "unavailable"))) SyncWire.fail("SELF_RESULT_INVALID")
        if (intent.kind == "read_state") {
            if (metadata.opt("membershipGeneration") != intent.body.opt("membershipGeneration") || metadata.opt("countStatus") != "not_computed" ||
                !SyncWire.same(metadata.opt("observedMessageIds"), intent.body.opt("observedMessageIds"))) SyncWire.fail("SELF_RESULT_INVALID")
            SelfStateWire.point(metadata.opt("readPoint"))
            SyncWire.flag(metadata.opt("hasMoreEligible")); SyncWire.flag(metadata.opt("hasUnorderedEligible"))
            if (result.getBoolean("current")) {
                val observed = intent.body.getJSONArray("observedMessageIds").let { a -> (0 until a.length()).map { a.getString(it) }.toSet() }
                val unreadKey = "${identity.namespace}|sync-unread|${intent.conversation}"
                records.one(unreadKey)?.let { entry ->
                    val held = JSONObject(entry.payload)
                    val ids = held.getJSONArray("ids").let { a -> (0 until a.length()).map { a.getString(it) }.filter { it !in observed } }
                    records.put(entry.copy(payload = held.put("ids", JSONArray(ids)).toString()))
                    database.messages().state(identity.namespace, intent.conversation)?.let {
                        database.messages().putState(it.copy(unread = held.optInt("legacy", 0) + ids.size))
                    }
                }
            }
        }
        // Never publish an old operation result as current state. Only a fresh owner GET may do that.
        saved.put("status", result.getString("status"))
        records.put(SyncRecord(intent.key + "|" + intent.requestId, identity.namespace, "self-history", saved.toString()))
        records.remove(intent.key)
        if (!current()) SyncWire.fail("SELF_STATE_CONTEXT_CHANGED")
    }
}
