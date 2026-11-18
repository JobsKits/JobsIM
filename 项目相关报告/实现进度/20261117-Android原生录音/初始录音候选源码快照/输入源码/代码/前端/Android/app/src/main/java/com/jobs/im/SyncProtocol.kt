package com.jobs.im

import java.math.BigDecimal
import java.math.BigInteger
import java.nio.ByteBuffer
import java.nio.charset.CodingErrorAction
import java.security.MessageDigest
import java.time.Instant
import java.util.Base64
import org.json.JSONArray
import org.json.JSONObject
import org.json.JSONTokener

class SyncFailure(val reason: String) : Exception(reason)

object SyncWire {
    fun fail(code: String = "SYNC_INVALID_RESPONSE"): Nothing = throw SyncFailure(code)

    fun counter(value: Any?): String {
        val text = value as? String ?: fail()
        if (!text.matches(Regex("0|[1-9][0-9]{0,18}")) || BigInteger(text) > BigInteger.valueOf(Long.MAX_VALUE)) fail()
        return text
    }

    fun id(value: Any?): String {
        val text = value as? String ?: fail()
        if (!text.matches(Regex("[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}"))) fail()
        return text
    }

    fun clientMessageId(value: Any?): String {
        val text = value as? String ?: fail("SYNC_INVALID_MESSAGE")
        if (text.toByteArray(Charsets.UTF_8).size !in 8..128) fail("SYNC_INVALID_MESSAGE")
        return text
    }

    fun text(value: Any?, limit: Int = 4096): String =
        (value as? String)?.takeIf { it.isNotEmpty() && it.length <= limit } ?: fail()

    fun flag(value: Any?): Boolean = value as? Boolean ?: fail()

    fun hash(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }

    fun digest(value: Any?): String = text(value, 64).also {
        if (!it.matches(Regex("[0-9a-f]{64}"))) fail()
    }

    fun originalBytes(encoded: Any?, expected: Any?): String {
        val input = text(encoded, 16 * 1024 * 1024)
        if (!input.matches(Regex("[A-Za-z0-9_-]+")) || input.length % 4 == 1) fail()
        val bytes = try { Base64.getUrlDecoder().decode(input) } catch (_: IllegalArgumentException) { fail() }
        if (Base64.getUrlEncoder().withoutPadding().encodeToString(bytes) != input) fail()
        if (hash(bytes) != digest(expected)) fail("SYNC_DIGEST_MISMATCH")
        return try {
            Charsets.UTF_8.newDecoder().onMalformedInput(CodingErrorAction.REPORT)
                .onUnmappableCharacter(CodingErrorAction.REPORT).decode(ByteBuffer.wrap(bytes)).toString()
        } catch (_: Exception) { fail() }
    }

    fun parseOriginal(encoded: Any?, expected: Any?): Any {
        val tokener = JSONTokener(originalBytes(encoded, expected))
        val result = try { tokener.nextValue() } catch (_: Exception) { fail() }
        if (tokener.nextClean() != '\u0000') fail()
        return result
    }

    fun same(a: Any?, b: Any?): Boolean = when {
        a === JSONObject.NULL || a == null -> b === JSONObject.NULL || b == null
        a is JSONObject && b is JSONObject -> {
            val keys = a.keys().asSequence().toSet()
            keys == b.keys().asSequence().toSet() && keys.all { same(a.opt(it), b.opt(it)) }
        }
        a is JSONArray && b is JSONArray -> a.length() == b.length() && (0 until a.length()).all { same(a.opt(it), b.opt(it)) }
        a is Number && b is Number -> runCatching { BigDecimal(a.toString()).compareTo(BigDecimal(b.toString())) == 0 }.getOrDefault(false)
        else -> a?.javaClass == b?.javaClass && a == b
    }
}

data class SyncIdentity(
    val namespace: String,
    val profile: String,
    val deployment: String,
    val user: String,
    val epoch: String,
    val device: String,
    val family: String,
) {
    fun check() {
        listOf(deployment, user, device, family).forEach { SyncWire.id(it) }
        if (SyncWire.counter(epoch) == "0" || namespace != Isolation.namespace(profile, deployment, user, false, epoch, "personal", device)) SyncWire.fail("SYNC_CONTEXT_CHANGED")
    }

    fun binding(store: String, stream: String) = JSONObject()
        .put("scopeId", deployment).put("principalId", user).put("sourceContextId", "personal")
        .put("deviceId", device).put("restoreEpoch", epoch).put("storeGeneration", store).put("streamId", stream)

    fun checkBinding(value: JSONObject, store: String, stream: String) {
        val expected = binding(store, stream)
        if (expected.keys().asSequence().any { value.opt(it) != expected.opt(it) }) SyncWire.fail("SYNC_CONTEXT_CHANGED")
    }
}

data class SyncRef(val raw: JSONObject) {
    val owner = SyncWire.text(raw.opt("owner"), 32)
    val kind = SyncWire.text(raw.opt("kind"), 32)
    val id = SyncWire.id(raw.opt("id"))
    val revision = SyncWire.counter(raw.opt("revision"))
    val epoch = SyncWire.counter(raw.opt("revisionEpoch"))
    val state = SyncWire.text(raw.opt("state"), 16)
    val key get() = "$owner|$kind|$id"

    init {
        val kinds = mapOf("auth" to setOf("account", "devices"), "directory" to setOf("profile", "contacts", "relationship", "contact_request", "preferences"), "conversation" to setOf("conversation", "announcement"), "message" to setOf("message"), "sync" to setOf("conversation_view", "read_state", "read_observation", "draft"))
        val allowed = setOf("owner", "kind", "id", "revision", "revisionEpoch", "state", "subjectUserId", "conversationId", "targetUserId")
        if (kind !in kinds[owner].orEmpty() || state !in setOf("active", "removed") || epoch == "0" || raw.keys().asSequence().any { it !in allowed }) SyncWire.fail()
        listOf("subjectUserId", "conversationId", "targetUserId").filter { raw.has(it) }.forEach { SyncWire.id(raw.opt(it)) }
    }
}

data class SyncProjection(
    val ref: SyncRef,
    val state: String,
    val currentRevision: String?,
    val currentEpoch: String?,
    val body: JSONObject?,
    val bodyDigest: String,
) {
    fun metadata() = JSONObject().put("ref", ref.raw).put("state", state)
        .put("currentRevision", currentRevision).put("currentRevisionEpoch", currentEpoch).put("bodyDigest", bodyDigest)
}

data class SyncManifest(val raw: JSONObject) {
    val schema = raw.optString("schemaVersion")
    val id = SyncWire.id(raw.opt("snapshotId"))
    val epoch = SyncWire.counter(raw.opt("restoreEpoch"))
    val boundary = SyncWire.counter(raw.opt("boundary"))
    val token = SyncWire.text(raw.opt("boundaryToken"))
    val permission = SyncWire.digest(raw.opt("permissionGeneration"))
    val classification = SyncWire.counter(raw.opt("classificationWatermark"))
    val pages = raw.optJSONArray("pageManifest") ?: SyncWire.fail()
    val expires = runCatching { Instant.parse(SyncWire.text(raw.opt("expiresAt"), 64)) }.getOrElse { SyncWire.fail() }

    fun check(identity: SyncIdentity, store: String, stream: String, expectedSchema: String = "1") {
        identity.checkBinding(raw, store, stream)
        if (schema != expectedSchema || schema !in setOf("1", "2")) SyncWire.fail("SYNC_SCHEMA_CHANGED")
        if (pages.length() !in 1..200000 || !expires.isAfter(Instant.now())) SyncWire.fail("SYNC_SNAPSHOT_EXPIRED")
        var count = 0
        for (index in 0 until pages.length()) {
            val page = pages.getJSONObject(index)
            if (page.opt("page") != index || page.opt("count") !is Int || page.getInt("count") !in 0..100) SyncWire.fail()
            SyncWire.digest(page.opt("digest")); count += page.getInt("count")
        }
        if (count > 200000 || raw.optJSONObject("coverage") == null) SyncWire.fail()
        val owners = raw.optJSONObject("ownerWatermarks") ?: SyncWire.fail()
        listOf("auth", "directory", "conversation", "message").forEach { SyncWire.counter(owners.opt(it)) }
        if (schema == "2") { SyncWire.counter(owners.opt("sync")); SelfStateWire.coverage(raw.getJSONObject("coverage")) }
    }
}

data class SyncPage(val index: Int, val digest: String, val envelope: String, val refs: List<SyncRef>, val items: List<SyncProjection>)
data class SyncDelta(val cursor: String, val position: String, val head: String, val more: Boolean, val items: List<SyncProjection>, val events: JSONArray)

object SyncProtocol {
    fun requireCommittedMessage(value: JSONObject, client: String, conversation: String, user: String) {
        SyncWire.id(value.opt("id"))
        if (value.opt("senderId") != user || value.opt("clientMsgId") != client || value.opt("conversationId") != conversation)
            SyncWire.fail("SYNC_OUTBOX_RESULT_INVALID")
        SyncWire.clientMessageId(value.opt("clientMsgId"))
        if (SyncWire.counter(value.opt("version")) == "0") SyncWire.fail("SYNC_OUTBOX_RESULT_INVALID")
        SyncWire.counter(value.opt("sequence"))
        if (value.has("contentVersion")) SyncWire.counter(value.opt("contentVersion"))
        val unavailableForward = value.opt("type") == "forward" && value.opt("forwarded") == true &&
            value.opt("forwardUnavailable") == true && value.opt("text") == ""
        if ((value.opt("type") !in setOf("text", "image", "audio", "video", "file") && !unavailableForward) || value.opt("text") !is String)
            SyncWire.fail("SYNC_OUTBOX_RESULT_INVALID")
        runCatching { Instant.parse(SyncWire.text(value.opt("createdAt"), 64)) }.getOrElse { SyncWire.fail("SYNC_OUTBOX_RESULT_INVALID") }
    }

    private fun projection(row: JSONObject, expected: SyncRef, epoch: String): SyncProjection {
        val ref = SyncRef(row.getJSONObject("ref"))
        if (!SyncWire.same(ref.raw, expected.raw) || ref.epoch != epoch) SyncWire.fail("SYNC_CONTEXT_CHANGED")
        val state = SyncWire.text(row.opt("state"), 16)
        if (state !in setOf("available", "global", "unavailable", "unknown", "redacted")) SyncWire.fail()
        val revision = if (row.has("currentRevision")) SyncWire.counter(row.opt("currentRevision")) else null
        val currentEpoch = if (row.has("currentRevisionEpoch")) SyncWire.counter(row.opt("currentRevisionEpoch")) else null
        if (currentEpoch != null && currentEpoch != epoch) SyncWire.fail("RESTORE_EPOCH_CHANGED")
        val match = SyncWire.flag(row.opt("revisionMatch"))
        if (match != (revision != null && currentEpoch != null && revision == ref.revision && currentEpoch == ref.epoch)) SyncWire.fail("SYNC_REVISION_MISMATCH")
        if (state in setOf("available", "global", "unavailable") && (revision == null || currentEpoch == null)) SyncWire.fail()
        val body = if (row.has("object")) {
            if (state !in setOf("available", "global", "unavailable")) SyncWire.fail()
            val value = SyncWire.parseOriginal(row.opt("bodyEnvelope"), row.opt("bodyDigest")) as? JSONObject ?: SyncWire.fail()
            if (!SyncWire.same(value, row.opt("object"))) SyncWire.fail("SYNC_DIGEST_MISMATCH")
            value
        } else null
        if (state == "available" && body == null) SyncWire.fail()
        return SyncProjection(ref, state, revision, currentEpoch, body, if (body == null) "" else SyncWire.digest(row.opt("bodyDigest")))
    }

    fun page(row: JSONObject, manifest: SyncManifest, index: Int): SyncPage {
        val expected = manifest.pages.optJSONObject(index) ?: SyncWire.fail()
        if (row.opt("snapshotId") != manifest.id || row.opt("page") != index || row.opt("restoreEpoch") != manifest.epoch || row.opt("permissionGeneration") != manifest.permission) SyncWire.fail("SYNC_CONTEXT_CHANGED")
        if (row.opt("installable") != true || row.opt("requiresFinalValidation") != true || row.opt("requiresAtomicInstall") != true || row.opt("qualifiedAck") != false || row.opt("digestScope") != "immutable_references_only") SyncWire.fail("SYNC_PAGE_NOT_INSTALLABLE")
        val digest = SyncWire.digest(expected.opt("digest"))
        if (row.opt("pageDigest") != digest) SyncWire.fail("SYNC_DIGEST_MISMATCH")
        val refs = SyncWire.parseOriginal(row.opt("referenceEnvelope"), digest) as? JSONArray ?: SyncWire.fail()
        val items = row.optJSONArray("items") ?: SyncWire.fail()
        if (!SyncWire.same(refs, row.opt("refs")) || refs.length() != expected.getInt("count") || items.length() != refs.length()) SyncWire.fail()
        val parsed = (0 until refs.length()).map { SyncRef(refs.getJSONObject(it)) }
        if (parsed.map { it.key }.toSet().size != parsed.size) SyncWire.fail("SYNC_DUPLICATE_REFERENCE")
        val projections = parsed.mapIndexed { i, ref ->
            projection(items.getJSONObject(i), ref, manifest.epoch).also {
                if (it.state in setOf("unknown", "redacted") || it.currentRevision != ref.revision || it.currentEpoch != ref.epoch) SyncWire.fail("SYNC_PAGE_NOT_INSTALLABLE")
            }
        }
        return SyncPage(index, digest, SyncWire.text(row.opt("referenceEnvelope"), 16 * 1024 * 1024), parsed, projections)
    }

    fun validation(row: JSONObject, manifest: SyncManifest): String {
        if (row.opt("canInstall") != true || row.opt("boundaryToken") != manifest.token || row.opt("restoreEpoch") != manifest.epoch || row.opt("permissionGeneration") != manifest.permission || row.opt("requiresLocalAtomicInstall") != true || row.opt("qualifiedAck") != false) SyncWire.fail("SYNC_SNAPSHOT_INVALIDATED")
        val watermark = SyncWire.counter(row.opt("classificationWatermark"))
        if (BigInteger(watermark) < BigInteger(manifest.classification) || !manifest.expires.isAfter(Instant.now())) SyncWire.fail("SYNC_SNAPSHOT_INVALIDATED")
        return watermark
    }

    fun changes(row: JSONObject, previous: String, epoch: String, permission: String): SyncDelta {
        if (row.opt("restoreEpoch") != epoch) SyncWire.fail("RESTORE_EPOCH_CHANGED")
        if (row.opt("permissionGeneration") != permission || SyncWire.flag(row.opt("snapshotRequired"))) SyncWire.fail("SYNC_PERMISSION_CHANGED")
        if (row.opt("qualifiedAck") != false) SyncWire.fail()
        val items = row.optJSONArray("items") ?: SyncWire.fail()
        if (items.length() > 500) SyncWire.fail()
        val projections = mutableListOf<SyncProjection>(); val events = JSONArray(); val origins = mutableSetOf<String>()
        var next = BigInteger(SyncWire.counter(previous)) + BigInteger.ONE
        for (index in 0 until items.length()) {
            val event = items.getJSONObject(index)
            when (event.opt("kind")) {
                "redacted" -> {
                    val from = SyncWire.counter(event.opt("from")); val to = SyncWire.counter(event.opt("to"))
                    if (BigInteger(from) != next || BigInteger(to) < next) SyncWire.fail("SYNC_CONTINUITY_GAP")
                    SyncWire.digest(event.opt("rangeDigest")); events.put(event); next = BigInteger(to) + BigInteger.ONE
                }
                "reference" -> {
                    val position = SyncWire.counter(event.opt("position")); val origin = SyncWire.id(event.opt("originEventId")); val ref = SyncRef(event.getJSONObject("ref"))
                    if (BigInteger(position) != next || !origins.add(origin)) SyncWire.fail("SYNC_CONTINUITY_GAP")
                    val plain = SyncWire.parseOriginal(event.opt("eventEnvelope"), event.opt("eventDigest"))
                    if (!SyncWire.same(plain, JSONObject().put("position", position).put("originEventId", origin).put("ref", ref.raw))) SyncWire.fail("SYNC_DIGEST_MISMATCH")
                    val projected = projection(event.getJSONObject("projection"), ref, epoch)
                    if (projected.state in setOf("unknown", "redacted")) SyncWire.fail("SYNC_PROJECTION_UNKNOWN")
                    projections.add(projected)
                    events.put(JSONObject().put("position", position).put("originEventId", origin).put("eventDigest", SyncWire.digest(event.opt("eventDigest"))).put("eventEnvelope", SyncWire.text(event.opt("eventEnvelope"), 16 * 1024 * 1024)))
                    next += BigInteger.ONE
                }
                else -> SyncWire.fail()
            }
        }
        val position = SyncWire.counter(row.opt("position")); val head = SyncWire.counter(row.opt("head")); val more = SyncWire.flag(row.opt("hasMore"))
        if (BigInteger(position) != next - BigInteger.ONE || BigInteger(head) < BigInteger(position) || more != (BigInteger(position) < BigInteger(head))) SyncWire.fail("SYNC_CONTINUITY_GAP")
        return SyncDelta(SyncWire.text(row.opt("cursor")), position, head, more, projections, events)
    }
}
