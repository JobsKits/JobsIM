package com.jobs.im

import androidx.room.Dao
import androidx.room.Entity
import androidx.room.Index
import androidx.room.Insert
import androidx.room.OnConflictStrategy
import androidx.room.PrimaryKey
import androidx.room.Query
import java.math.BigInteger
import java.time.Instant
import java.util.UUID
import org.json.JSONArray
import org.json.JSONObject

@Entity(tableName = "sync_records", indices = [Index(value = ["namespace", "kind"]), Index(value = ["kind"])])
data class SyncRecord(@PrimaryKey val key: String, val namespace: String, val kind: String, val payload: String)

@Dao
abstract class SyncRecords {
    @Query("SELECT EXISTS(SELECT 1 FROM sync_records WHERE namespace=:namespace)")
    abstract suspend fun hasNamespace(namespace: String): Boolean
    @Query("SELECT * FROM sync_records WHERE `key`=:key LIMIT 1")
    abstract suspend fun one(key: String): SyncRecord?
    @Query("SELECT * FROM sync_records WHERE namespace=:namespace AND kind=:kind")
    abstract suspend fun rows(namespace: String, kind: String): List<SyncRecord>
    @Insert(onConflict = OnConflictStrategy.REPLACE)
    abstract suspend fun put(row: SyncRecord)
    @Query("DELETE FROM sync_records WHERE namespace=:namespace AND kind=:kind")
    abstract suspend fun clear(namespace: String, kind: String)
    @Query("DELETE FROM sync_records WHERE `key`=:key")
    abstract suspend fun remove(key: String)
    @Query("SELECT * FROM sync_records WHERE kind=:kind AND substr(namespace,1,length(:prefix))=:prefix")
    abstract suspend fun across(prefix: String, kind: String): List<SyncRecord>
}

data class SyncOutboxProof(val clientId: String, val conversationId: String, val originalBody: String)
data class SyncCleanupPlan(val key: String, val scope: String, val id: String)
data class SyncInstall(val messages: List<MessageRow>, val objects: List<JSONObject>, val globalIds: Set<String>, val position: String, val newIncoming: List<MessageRow>)

fun originalMessageBody(row: MessageRow): JSONObject {
    if (row.type == "forward") return Forwarding.payload(row)
    return JSONObject().put("clientMsgId", row.clientMsgId).put("restoreEpoch", row.restoreEpoch)
        .put("type", row.type).put("text", row.text).put("sendAdmission", row.sendAdmission).also {
            val mentions = JSONArray(row.mentionedUserIds)
            if (mentions.length() > 0) it.put("mentionedUserIds", mentions)
            if (row.mentionAll) it.put("mentionAll", true)
            if (row.replyToId.isNotBlank()) it.put("replyToId", row.replyToId)
            if (row.attachmentId.isNotBlank()) it.put("attachmentId", row.attachmentId)
        }
}

class SyncStore(private val database: ChatDatabase) {
    private val records get() = database.syncRecords()
    private val messages get() = database.messages()
    private fun stateKey(ns: String) = "$ns|sync-control"
    private fun retained(ns: String) = "$ns|sync-retained"
    private fun scope(identity: SyncIdentity) = "live|${identity.profile}|${identity.deployment}|${identity.user}|personal"
    private fun holderPrefix(identity: SyncIdentity) = "live|${identity.profile}|${identity.deployment}|${identity.user}|"
    private fun stageKey(ns: String, id: String) = "$ns|sync-stage|$id"
    private fun record(ns: String, kind: String, id: String, body: JSONObject) = SyncRecord("$ns|sync-$kind|$id", ns, kind, body.toString())

    suspend fun responsibility(identity: SyncIdentity): Boolean =
        records.hasNamespace(identity.namespace) || messages.all(retained(identity.namespace)).isNotEmpty()

    suspend fun projectionGuard(namespace: String): LocalProjectionGuard? =
        records.one(stateKey(namespace))?.let {
            val control = JSONObject(it.payload)
            LocalProjectionGuard(control.getString("guardGeneration"), control.has("cursor") && !control.has("restriction"))
        }

    private suspend fun checked(identity: SyncIdentity, current: () -> Boolean): JSONObject? {
        identity.check()
        if (!current() || messages.nativeScope(identity.namespace)?.namespace != identity.namespace) SyncWire.fail("SYNC_CONTEXT_CHANGED")
        return records.one(stateKey(identity.namespace))?.let {
            JSONObject(it.payload).also { value ->
                if (value.opt("familyId") != identity.family || value.opt("profile") != identity.profile) SyncWire.fail("SYNC_CONTEXT_CHANGED")
                identity.checkBinding(value, SyncWire.id(value.opt("storeGeneration")), value.optString("streamId"))
                SyncWire.id(value.opt("guardGeneration"))
            }
        }
    }

    suspend fun pendingCleanup(profile: String? = null, deployment: String? = null): List<SyncCleanupPlan> =
        records.across(if (profile != null && deployment != null) "live|$profile|$deployment|" else "live|", "cleanup").map {
            val parts = it.namespace.split('|')
            if (parts.size != 5 || parts[0] != "live" || parts[1].isBlank() ||
                (profile != null && parts[1] != profile) || (deployment != null && parts[2] != deployment) || parts[4] != "personal") SyncWire.fail("SYNC_STORE_INVALID")
            SyncWire.id(parts[2]); SyncWire.id(parts[3])
            val id = SyncWire.id(JSONObject(it.payload).opt("id"))
            if (it.key != "${it.namespace}|cleanup|$id") SyncWire.fail("SYNC_STORE_INVALID")
            SyncCleanupPlan(it.key, it.namespace, id)
        }

    suspend fun completeCleanup(plan: SyncCleanupPlan) = database.withTransaction {
        val original = records.one(plan.key) ?: return@withTransaction
        if (original.namespace != plan.scope || JSONObject(original.payload).opt("id") != plan.id) SyncWire.fail("SYNC_STORE_INVALID")
        records.remove(plan.key)
    }

    private suspend fun save(identity: SyncIdentity, control: JSONObject) =
        records.put(SyncRecord(stateKey(identity.namespace), identity.namespace, "control", control.toString()))

    private fun sameGuard(control: JSONObject, expected: JSONObject?) {
        if (expected != null && (control.opt("guardGeneration") != expected.opt("guardGeneration") ||
                control.opt("storeGeneration") != expected.opt("storeGeneration") || control.opt("streamId") != expected.opt("streamId")))
            SyncWire.fail("SYNC_LOCAL_RESTRICTION_CHANGED")
    }

    suspend fun assertGuard(identity: SyncIdentity, expected: JSONObject, current: () -> Boolean) = database.withTransaction {
        sameGuard(checked(identity, current) ?: SyncWire.fail(), expected)
    }

    suspend fun ensure(identity: SyncIdentity, current: () -> Boolean): JSONObject = database.withTransaction {
        checked(identity, current) ?: identity.binding(UUID.randomUUID().toString(), "")
            .put("profile", identity.profile).put("familyId", identity.family)
            .put("registrationRequestId", UUID.randomUUID().toString()).put("guardGeneration", UUID.randomUUID().toString())
            .put("position", "0").put("classificationWatermark", "0").also { save(identity, it) }
    }

    suspend fun selectSchema(identity: SyncIdentity, s2: Boolean, current: () -> Boolean): JSONObject = database.withTransaction {
        val original = checked(identity, current) ?: SyncWire.fail()
        val schema = original.optString("schemaVersion", "1")
        if (schema == "2" && !s2) SyncWire.fail("SYNC_SELF_STATE_REQUIRED")
        if (schema == "2" || !s2) return@withTransaction original
        // Schema selects another stream, never another physical database or authentication device.
        records.put(record(identity.namespace, "schema-history", "1", JSONObject(original.toString())))
        retain(identity)
        val next = identity.binding(original.getString("storeGeneration"), "")
            .put("profile", identity.profile).put("familyId", identity.family).put("schemaVersion", "2")
            .put("registrationRequestId", UUID.randomUUID().toString()).put("guardGeneration", UUID.randomUUID().toString())
            .put("position", "0").put("classificationWatermark", "0").put("restriction", "SYNC_SCHEMA_CHANGED")
        save(identity, next)
        next
    }

    suspend fun bind(identity: SyncIdentity, expected: JSONObject, stream: String, current: () -> Boolean): JSONObject = database.withTransaction {
        val control = checked(identity, current) ?: SyncWire.fail()
        sameGuard(control, expected)
        SyncWire.id(stream)
        if (control.opt("storeGeneration") != expected.opt("storeGeneration") || control.optString("streamId").let { it.isNotEmpty() && it != stream }) SyncWire.fail("SYNC_CONTEXT_CHANGED")
        control.put("streamId", stream); save(identity, control); control
    }

    suspend fun begin(identity: SyncIdentity, expected: JSONObject? = null, current: () -> Boolean): JSONObject = database.withTransaction {
        val control = checked(identity, current) ?: SyncWire.fail()
        sameGuard(control, expected)
        if (!control.has("snapshotRequest")) control.put("snapshotRequest", JSONObject()
            .put("requestId", UUID.randomUUID().toString()).put("storeGeneration", control.getString("storeGeneration"))
            .put("schemaVersion", control.optString("schemaVersion", "1")).put("pageSize", 100))
        save(identity, control); control
    }

    suspend fun manifest(identity: SyncIdentity, manifest: SyncManifest, requestId: String, expected: JSONObject? = null, current: () -> Boolean): JSONObject = database.withTransaction {
        val control = checked(identity, current) ?: SyncWire.fail()
        sameGuard(control, expected)
        if (control.getJSONObject("snapshotRequest").opt("requestId") != requestId) SyncWire.fail("SYNC_CONTEXT_CHANGED")
        val old = records.one(stageKey(identity.namespace, manifest.id))?.let { JSONObject(it.payload) }
        if (old != null && (old.has("obsolete") || !SyncWire.same(old.getJSONObject("manifest"), manifest.raw))) SyncWire.fail("SYNC_STAGE_CHANGED")
        records.put(SyncRecord(stageKey(identity.namespace, manifest.id), identity.namespace, "stage", JSONObject().put("manifest", manifest.raw).put("pages", old?.optJSONObject("pages") ?: JSONObject()).toString()))
        control.put("manifestId", manifest.id); save(identity, control); control
    }

    suspend fun page(identity: SyncIdentity, manifest: SyncManifest, page: SyncPage, expected: JSONObject? = null, current: () -> Boolean) = database.withTransaction {
        val control = checked(identity, current) ?: SyncWire.fail()
        sameGuard(control, expected)
        if (control.opt("manifestId") != manifest.id) SyncWire.fail("SYNC_CONTEXT_CHANGED")
        val stage = JSONObject(records.one(stageKey(identity.namespace, manifest.id))?.payload ?: SyncWire.fail())
        val metadata = JSONObject().put("digest", page.digest).put("referenceEnvelope", page.envelope)
            .put("refs", JSONArray().also { array -> page.refs.forEach { array.put(it.raw) } })
            .put("projections", JSONArray().also { array -> page.items.forEach { array.put(it.metadata()) } })
        stage.getJSONObject("pages").put(page.index.toString(), metadata)
        records.put(SyncRecord(stageKey(identity.namespace, manifest.id), identity.namespace, "stage", stage.toString()))
    }

    private fun erased(row: MessageRow) = globalMessagePlaceholder(row)
    private fun hidden(row: MessageRow) = if (row.retracted) erased(row) else if (row.forwarded) Forwarding.hidden(row) else row

    private suspend fun retain(identity: SyncIdentity) {
        val ns = identity.namespace; val target = retained(ns)
        for (row in messages.all(ns).filter { it.state !in setOf("pending", "frozen") }) {
            val previous = messages.one(target, row.id)
            if (previous?.retracted == true && !row.retracted) continue
            if (previous != null && !previous.retracted && !row.retracted && row.contentUnavailable) continue
            if (previous != null && !row.retracted && BigInteger(previous.version) > BigInteger(row.version)) continue
            messages.put(hidden(row).copy(key = "$target|${row.id}", namespace = target))
        }
    }

    private suspend fun localUnread(identity: SyncIdentity, row: MessageRow, fresh: Boolean, reading: String?) {
        val key = "${identity.namespace}|sync-unread|${row.conversationId}"
        val old = messages.state(identity.namespace, row.conversationId) ?: ConversationState("${identity.namespace}|${row.conversationId}", identity.namespace, row.conversationId, "${identity.epoch}:0")
        val saved = records.one(key)?.let { JSONObject(it.payload) }
        val ids = saved?.optJSONArray("ids")?.let { a -> (0 until a.length()).map { a.getString(it) }.toMutableSet() } ?: mutableSetOf()
        var legacy = saved?.optInt("legacy", 0) ?: old.unread
        if (row.retracted || row.contentUnavailable) ids.remove(row.id)
        if (fresh && reading != row.conversationId) ids.add(row.id)
        if (reading == row.conversationId) { ids.clear(); legacy = 0 }
        records.put(SyncRecord(key, identity.namespace, "unread", JSONObject().put("legacy", legacy).put("ids", JSONArray(ids.toList())).toString()))
        messages.putState(old.copy(unread = legacy + ids.size))
    }

    private suspend fun eraseUnread(identity: SyncIdentity, removed: Set<String>) {
        for (entry in records.across(holderPrefix(identity), "unread")) {
            val saved = JSONObject(entry.payload); val original = saved.optJSONArray("ids") ?: continue
            val ids = (0 until original.length()).map { original.getString(it) }.filter { it !in removed }
            if (ids.size == original.length()) continue
            records.put(entry.copy(payload = saved.put("ids", JSONArray(ids)).toString()))
            val conversation = entry.key.substringAfterLast('|')
            messages.state(entry.namespace, conversation)?.let { messages.putState(it.copy(unread = saved.optInt("legacy", 0) + ids.size)) }
        }
    }

    suspend fun restrict(identity: SyncIdentity, reason: String, obsolete: Boolean, current: () -> Boolean) = database.withTransaction {
        val control = checked(identity, current) ?: return@withTransaction
        retain(identity)
        messages.clearCloud(identity.namespace)
        if (obsolete) {
            control.optString("manifestId").takeIf { it.isNotEmpty() }?.let { id ->
                records.one(stageKey(identity.namespace, id))?.let { original -> records.put(original.copy(payload = JSONObject(original.payload).put("obsolete", reason).toString())) }
            }
            control.remove("snapshotRequest"); control.remove("manifestId")
        }
        control.put("restriction", reason).put("guardGeneration", UUID.randomUUID().toString())
        save(identity, control)
    }

    private fun projected(identity: SyncIdentity, value: SyncProjection, previous: MessageRow?): MessageRow? {
        val restored = previous?.copy(key = "${identity.namespace}|${previous.id}", namespace = identity.namespace, restoreEpoch = identity.epoch)
        if (restored?.retracted == true) return erased(restored)
        val body = value.body?.let { JSONObject(it.toString()) } ?: JSONObject().put("id", value.ref.id)
        if (body.opt("id") != value.ref.id) SyncWire.fail("SYNC_INVALID_MESSAGE")
        if (body.has("clientMsgId")) SyncWire.clientMessageId(body.opt("clientMsgId"))
        val conversationId = body.optString("conversationId").ifBlank { value.ref.raw.optString("conversationId").ifBlank { previous?.conversationId.orEmpty() } }
        SyncWire.id(conversationId)
        if (value.ref.raw.has("conversationId") && value.ref.raw.opt("conversationId") != conversationId) SyncWire.fail("SYNC_INVALID_MESSAGE")
        body.put("conversationId", conversationId)
        if (!body.has("senderId")) body.put("senderId", value.ref.raw.optString("subjectUserId").ifBlank { previous?.senderId.orEmpty() })
        if (!body.has("version")) body.put("version", previous?.version ?: "0")
        if (!body.has("sequence")) body.put("sequence", previous?.sequence ?: "0")
        SyncWire.counter(body.opt("version")); SyncWire.counter(body.opt("sequence"))
        if (!body.has("createdAt") && previous != null) body.put("createdAt", Instant.ofEpochMilli(previous.createdAt).toString())
        val row = message(body, identity.namespace, identity.epoch).copy(state = "received")
        if (value.state == "available" && restored != null && BigInteger(restored.version) > BigInteger(row.version)) return restored
        return when (value.state) {
            "global" -> erased(row)
            "unavailable", "unknown", "redacted" -> row.copy(text = "", attachmentId = "", attachmentURL = "",
                forwardAdmission = "", replyToId = "", reaction = "", mentionedUserIds = "[]", mentionAll = false,
                mentionedMe = false, forwardUnavailable = row.forwarded, contentUnavailable = true, retracted = false)
            "available" -> row
            else -> SyncWire.fail()
        }
    }

    suspend fun commit(
        identity: SyncIdentity, expected: JSONObject, manifest: SyncManifest?, pages: List<SyncPage>, delta: SyncDelta,
        values: List<SyncProjection>, classification: String, proofs: List<SyncOutboxProof>, reading: String?, current: () -> Boolean,
    ): SyncInstall = database.withTransaction {
        val control = checked(identity, current) ?: SyncWire.fail()
        if (control.opt("guardGeneration") != expected.opt("guardGeneration") || control.opt("storeGeneration") != expected.opt("storeGeneration") || control.opt("streamId") != expected.opt("streamId")) SyncWire.fail("SYNC_LOCAL_RESTRICTION_CHANGED")
        if (manifest == null && (control.optString("cursor") != expected.optString("cursor") || control.has("restriction"))) SyncWire.fail("SYNC_CONTEXT_CHANGED")
        if (BigInteger(classification) < BigInteger(control.optString("classificationWatermark", "0"))) SyncWire.fail("SYNC_REVISION_REGRESSION")
        if (manifest != null) {
            if (!manifest.expires.isAfter(Instant.now())) SyncWire.fail("SYNC_SNAPSHOT_EXPIRED")
            if (control.opt("manifestId") != manifest.id) SyncWire.fail("SYNC_CONTEXT_CHANGED")
            val stage = JSONObject(records.one(stageKey(identity.namespace, manifest.id))?.payload ?: SyncWire.fail("SYNC_STAGE_INCOMPLETE"))
            val saved = stage.getJSONObject("pages")
            if (stage.has("obsolete") || saved.length() != manifest.pages.length() || pages.size != manifest.pages.length()) SyncWire.fail("SYNC_STAGE_INCOMPLETE")
            pages.forEachIndexed { index, page ->
                val frame = saved.optJSONObject(index.toString()) ?: SyncWire.fail("SYNC_STAGE_INCOMPLETE")
                if (page.index != index || frame.opt("digest") != page.digest || frame.opt("referenceEnvelope") != page.envelope || !SyncWire.same(frame.opt("refs"), JSONArray().also { a -> page.refs.forEach { a.put(it.raw) } })) SyncWire.fail("SYNC_STAGE_INCOMPLETE")
            }
            retain(identity); messages.clearCloud(identity.namespace); records.clear(identity.namespace, "object")
        }
        val globals = mutableSetOf<String>(); val fresh = mutableListOf<MessageRow>(); val ram = mutableListOf<MessageRow>()
        for (value in values) {
            val key = "${identity.namespace}|sync-ref|${value.ref.key}"
            val prior = records.one(key)?.let { JSONObject(it.payload) }
            val global = value.ref.kind == "message" && records.one("${scope(identity)}|global|${value.ref.id}") != null
            if ((prior?.opt("state") == "global" || global) && value.state != "global") {
                if (manifest != null) SyncWire.fail("SYNC_GLOBAL_REGRESSION")
                continue
            }
            if (prior != null && prior.has("currentRevision") && value.currentRevision != null && BigInteger(prior.getString("currentRevision")) > BigInteger(value.currentRevision)) {
                if (manifest != null) SyncWire.fail("SYNC_REVISION_REGRESSION") else continue
            }
            records.put(SyncRecord(key, identity.namespace, "ref", value.metadata().toString()))
            if (value.ref.kind == "message") {
                val previous = messages.one(identity.namespace, value.ref.id) ?: messages.one(retained(identity.namespace), value.ref.id)
                val row = projected(identity, value, previous) ?: continue
                SelfStateStore(database).invalidateCountInTransaction(identity, row.conversationId)
                if (value.state == "global") {
                    globals.add(row.id)
                    records.put(SyncRecord("${scope(identity)}|global|${row.id}", scope(identity), "global", JSONObject().put("id", row.id).toString()))
                    records.put(SyncRecord("${scope(identity)}|cleanup|${row.id}", scope(identity), "cleanup", JSONObject().put("id", row.id).toString()))
                    messages.across(holderPrefix(identity), row.id).filter { it.state !in setOf("pending", "frozen") }.forEach { messages.put(erased(it)) }
                }
                val newlyUnread = previous == null && row.senderId != identity.user && !row.retracted && !row.contentUnavailable && manifest == null
                if (newlyUnread) fresh.add(row)
                ram.add(row)
                messages.put(hidden(row).copy(forwardAdmission = ""))
                localUnread(identity, row, newlyUnread, if (control.optString("schemaVersion", "1") == "2") null else reading)
            } else {
                val body = value.body?.let { JSONObject(it.toString()) }
                if (value.ref.kind in SelfStateWire.kinds) {
                    if (control.optString("schemaVersion", "1") != "2") SyncWire.fail("SYNC_SCHEMA_CHANGED")
                    if (body != null) {
                        if (value.ref.raw.opt("subjectUserId") != identity.user || (body.opt("removed") == true && value.ref.state != "removed")) SyncWire.fail("SELF_STATE_CONTEXT_CHANGED")
                        SelfStateWire.validate(value.ref.kind, body, identity, value.ref.raw.optString("conversationId"), value.ref.id)
                        SelfStateStore(database).acceptInTransaction(identity, value.ref.kind, body)
                    }
                }
                if (value.ref.kind == "announcement") body?.remove("message")
                body?.optJSONObject("message")?.takeIf { it.optBoolean("forwarded") }?.let { message ->
                    message.put("text", "").put("forwardUnavailable", true).remove("attachmentURL")
                    message.remove("attachmentId"); message.remove("forwardAdmission")
                }
                records.put(record(identity.namespace, "object", value.ref.key, value.metadata().put("value", body)))
            }
        }
        if (globals.isNotEmpty()) {
            // Stored forward projections do not expose their source ID. Retire every controlled body.
            messages.forwardHolders(holderPrefix(identity)).forEach { messages.put(if (it.retracted) erased(it) else Forwarding.hidden(it)) }
            eraseUnread(identity, globals)
        }
        for (proof in proofs) {
            val candidates = messages.outbox(identity.namespace).filter { it.clientMsgId == proof.clientId && it.conversationId == proof.conversationId && it.senderId == identity.user }
            candidates.forEach { row -> if (SyncWire.same(originalMessageBody(row), JSONObject(proof.originalBody))) messages.remove(identity.namespace, row.id) }
        }
        val allowed = records.rows(identity.namespace, "object").map { JSONObject(it.payload) }
            .filter { it.optJSONObject("ref")?.opt("kind") == "conversation" && it.opt("state") == "available" }
            .map { it.getJSONObject("ref").getString("id") }.toSet()
        messages.outbox(identity.namespace).filter { it.conversationId !in allowed && it.state == "pending" }.forEach { messages.freeze(identity.namespace, it.id) }
        messages.states(identity.namespace).filter { it.conversationId !in allowed }.forEach { messages.markRead(identity.namespace, it.conversationId) }
        for (index in 0 until delta.events.length()) {
            val event = delta.events.getJSONObject(index)
            val id = event.optString("originEventId").ifBlank { "range-" + event.optString("from") + "-" + event.optString("to") }
            val key = "${identity.namespace}|sync-event|$id"; val old = records.one(key)
            if (old != null && !SyncWire.same(JSONObject(old.payload), event)) SyncWire.fail("SYNC_ORIGIN_CONFLICT")
            records.put(SyncRecord(key, identity.namespace, "event", event.toString()))
        }
        control.put("cursor", delta.cursor).put("position", delta.position).put("permissionGeneration", manifest?.permission ?: expected.getString("permissionGeneration"))
            .put("classificationWatermark", classification).put("guardGeneration", UUID.randomUUID().toString())
        control.remove("restriction"); control.remove("snapshotRequest"); control.remove("manifestId")
        save(identity, control)
        if (!current()) SyncWire.fail("SYNC_CONTEXT_CHANGED")
        val cleanup = records.rows(scope(identity), "cleanup").map { SyncWire.id(JSONObject(it.payload).opt("id")) }.toSet()
        SyncInstall(ram, records.rows(identity.namespace, "object").map { JSONObject(it.payload) }, globals + cleanup, delta.position, fresh.filter { it.conversationId in allowed && it.id !in globals })
    }
}
