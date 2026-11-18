package com.jobs.im

import java.time.Instant
import org.json.JSONObject

class PersonalExportFailure(val code: String) : Exception(code)

data class PersonalExportView(
    val id: String,
    val artifactId: String,
    val epoch: String,
    val generation: String,
    val state: String,
    val expiresAt: Instant,
    val cleanup: String,
    val failureCode: String,
) {
    val terminal get() = state in setOf("cancelled", "expired", "failed")
    val ready get() = state == "ready" && cleanup == "published" && expiresAt.isAfter(Instant.now())
}

data class PersonalExportIntent(
    val requestId: String,
    val originalBody: String,
    val bodySHA256: String,
    val createdAt: String,
    val storeGeneration: String,
    val familyId: String,
    val deviceId: String,
    val jobId: String = "",
    val cancelBody: String = "",
    val cancelSHA256: String = "",
) {
    fun json() = JSONObject().put("requestId", requestId).put("originalBody", originalBody)
        .put("bodySHA256", bodySHA256).put("createdAt", createdAt).put("storeGeneration", storeGeneration)
        .put("familyId", familyId).put("deviceId", deviceId).put("jobId", jobId)
        .put("cancelBody", cancelBody).put("cancelSHA256", cancelSHA256)
}

object PersonalExportWire {
    fun fail(code: String = "EXPORT_INVALID_RESPONSE"): Nothing = throw PersonalExportFailure(code)

    fun view(value: JSONObject, identity: SyncIdentity, job: String = ""): PersonalExportView {
        if (value.opt("format") != "zip" || value.opt("scope") != "personal_current") fail()
        val source = value.optJSONObject("job") ?: fail()
        val id = SyncWire.id(source.opt("id"))
        if (job.isNotEmpty() && job != id) fail()
        val epoch = SyncWire.counter(source.opt("restoreEpoch"))
        val generation = SyncWire.counter(source.opt("buildGeneration"))
        if (epoch != identity.epoch || epoch == "0" || generation == "0") fail("RESTORE_EPOCH_CHANGED")
        val state = source.optString("state")
        if (state !in setOf("requested", "building", "ready", "cancelled", "expired", "failed")) fail()
        val cleanup = source.optString("cleanupState")
        if (!cleanup.matches(Regex("[a-z_]{1,48}"))) fail()
        for (key in listOf("coverage", "exclusions")) {
            val list = value.optJSONArray(key) ?: fail()
            if (list.length() > 64 || (0 until list.length()).any { list.opt(it) !is String || !list.getString(it).matches(Regex("[a-z_]{1,100}")) }) fail()
        }
        if (value.has("capturedAt")) SelfStateWire.utc(value.opt("capturedAt"))
        val failure = source.optString("failureCode")
        if (failure.isNotEmpty() && !failure.matches(Regex("[A-Z0-9_]{1,64}"))) fail()
        return PersonalExportView(id, SyncWire.id(source.opt("artifactId")), epoch, generation, state,
            SelfStateWire.utc(source.opt("expiresAt")), cleanup, failure)
    }

    fun intent(value: JSONObject, identity: SyncIdentity, store: String): PersonalExportIntent {
        val keys = setOf("requestId", "originalBody", "bodySHA256", "createdAt", "storeGeneration", "familyId", "deviceId", "jobId", "cancelBody", "cancelSHA256")
        if (value.keys().asSequence().any { it !in keys }) fail("EXPORT_ORIGINAL_CHANGED")
        val id = SyncWire.id(value.opt("requestId"))
        val raw = SyncWire.text(value.opt("originalBody"), 1024)
        val digest = SyncWire.digest(value.opt("bodySHA256"))
        val body = JSONObject(raw)
        if (body.keys().asSequence().toSet() != setOf("requestId", "restoreEpoch") ||
            body.opt("requestId") != id || body.opt("restoreEpoch") != identity.epoch ||
            SyncWire.hash(raw.toByteArray(Charsets.UTF_8)) != digest ||
            value.opt("storeGeneration") != store || value.opt("familyId") != identity.family || value.opt("deviceId") != identity.device) fail("EXPORT_ORIGINAL_CHANGED")
        val createdAt = SyncWire.text(value.opt("createdAt"), 64)
        SelfStateWire.utc(createdAt)
        val job = value.optString("jobId")
        if (job.isNotEmpty()) SyncWire.id(job)
        val cancel = value.optString("cancelBody")
        val cancelSHA = value.optString("cancelSHA256")
        if (cancel.isEmpty() != cancelSHA.isEmpty()) fail("EXPORT_ORIGINAL_CHANGED")
        if (cancel.isNotEmpty()) {
            if (cancel.length > 512 || SyncWire.hash(cancel.toByteArray(Charsets.UTF_8)) != SyncWire.digest(cancelSHA)) fail("EXPORT_ORIGINAL_CHANGED")
            val parsed = JSONObject(cancel)
            if (parsed.keys().asSequence().toSet() != setOf("requestId") || SyncWire.id(parsed.opt("requestId")) == id) fail("EXPORT_ORIGINAL_CHANGED")
        }
        return PersonalExportIntent(id, raw, digest, createdAt, store, identity.family, identity.device, job, cancel, cancelSHA)
    }
}
