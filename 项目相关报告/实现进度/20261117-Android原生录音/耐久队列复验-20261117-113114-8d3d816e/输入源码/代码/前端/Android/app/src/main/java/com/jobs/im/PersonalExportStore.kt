package com.jobs.im

import java.time.Instant
import java.util.UUID
import org.json.JSONObject

class PersonalExportStore(private val database: ChatDatabase) {
    private val records get() = database.syncRecords()
    private fun key(identity: SyncIdentity, id: String) = "${identity.namespace}|data-export|$id"

    private suspend fun physical(identity: SyncIdentity, current: () -> Boolean): String {
        identity.check()
        if (!current() || database.messages().nativeScope(identity.namespace)?.namespace != identity.namespace)
            PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        // Reuse the actual Room store identity; an export must not invent a sync registration.
        val raw = records.one("${identity.namespace}|sync-control")?.payload ?: PersonalExportWire.fail("EXPORT_STORE_NOT_READY")
        val control = JSONObject(raw)
        val generation = SyncWire.id(control.opt("storeGeneration"))
        identity.checkBinding(control, generation, control.optString("streamId"))
        if (control.opt("familyId") != identity.family || control.opt("profile") != identity.profile) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        return generation
    }

    private suspend fun rows(identity: SyncIdentity, store: String): List<PersonalExportIntent> =
        records.rows(identity.namespace, "data-export").also { if (it.size > 10000) PersonalExportWire.fail("EXPORT_STORE_LIMIT") }
            .map { row ->
                val value = PersonalExportWire.intent(JSONObject(row.payload), identity, store)
                if (row.key != key(identity, value.requestId)) PersonalExportWire.fail("EXPORT_ORIGINAL_CHANGED")
                value
            }.sortedByDescending { it.createdAt }

    suspend fun list(identity: SyncIdentity, current: () -> Boolean): List<PersonalExportIntent> = database.withTransaction {
        val result = rows(identity, physical(identity, current))
        if (!current()) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        result
    }

    suspend fun original(identity: SyncIdentity, id: String, current: () -> Boolean) = database.withTransaction {
        val store = physical(identity, current)
        val row = records.one(key(identity, id)) ?: PersonalExportWire.fail("EXPORT_ORIGINAL_CHANGED")
        val intent = PersonalExportWire.intent(JSONObject(row.payload), identity, store)
        if (!current()) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        intent
    }

    suspend fun prepare(identity: SyncIdentity, current: () -> Boolean): PersonalExportIntent = database.withTransaction {
        val store = physical(identity, current)
        if (rows(identity, store).any { it.jobId.isEmpty() }) PersonalExportWire.fail("EXPORT_ORIGINAL_PENDING")
        val id = UUID.randomUUID().toString()
        val body = JSONObject().put("requestId", id).put("restoreEpoch", identity.epoch).toString()
        val intent = PersonalExportIntent(id, body, SyncWire.hash(body.toByteArray(Charsets.UTF_8)), Instant.now().toString(), store, identity.family, identity.device)
        if (!current()) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        records.put(SyncRecord(key(identity, id), identity.namespace, "data-export", intent.json().toString()))
        if (!current()) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        intent
    }

    suspend fun bind(identity: SyncIdentity, original: PersonalExportIntent, job: String, current: () -> Boolean) = database.withTransaction {
        val store = physical(identity, current)
        val row = records.one(key(identity, original.requestId)) ?: PersonalExportWire.fail("EXPORT_ORIGINAL_CHANGED")
        val saved = PersonalExportWire.intent(JSONObject(row.payload), identity, store)
        if (saved.originalBody != original.originalBody || saved.bodySHA256 != original.bodySHA256 || saved.storeGeneration != original.storeGeneration ||
            saved.jobId.isNotEmpty() && saved.jobId != job) PersonalExportWire.fail("EXPORT_ORIGINAL_CHANGED")
        SyncWire.id(job)
        if (!current()) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        records.put(row.copy(payload = saved.copy(jobId = job).json().toString()))
        if (!current()) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
    }

    suspend fun cancellation(identity: SyncIdentity, original: PersonalExportIntent, current: () -> Boolean): PersonalExportIntent = database.withTransaction {
        val store = physical(identity, current)
        val row = records.one(key(identity, original.requestId)) ?: PersonalExportWire.fail("EXPORT_ORIGINAL_CHANGED")
        val saved = PersonalExportWire.intent(JSONObject(row.payload), identity, store)
        if (saved.originalBody != original.originalBody || saved.bodySHA256 != original.bodySHA256 || saved.jobId != original.jobId || saved.jobId.isEmpty()) PersonalExportWire.fail("EXPORT_ORIGINAL_CHANGED")
        if (saved.cancelBody.isNotEmpty()) return@withTransaction saved
        val body = JSONObject().put("requestId", UUID.randomUUID().toString()).toString()
        val next = saved.copy(cancelBody = body, cancelSHA256 = SyncWire.hash(body.toByteArray(Charsets.UTF_8)))
        if (!current()) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        records.put(row.copy(payload = next.json().toString()))
        if (!current()) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        next
    }
}
