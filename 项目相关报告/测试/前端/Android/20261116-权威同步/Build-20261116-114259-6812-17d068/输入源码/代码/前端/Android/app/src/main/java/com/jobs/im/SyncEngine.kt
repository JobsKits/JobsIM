package com.jobs.im

import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONArray
import org.json.JSONObject

class NativeSyncEngine(private val database: ChatDatabase) {
    companion object {
        private val locks = ConcurrentHashMap<String, Mutex>()
        private val obsolete = setOf("SNAPSHOT_EXPIRED", "SNAPSHOT_INVALIDATED", "SYNC_SNAPSHOT_EXPIRED", "SYNC_SNAPSHOT_INVALIDATED", "SYNC_PAGE_NOT_INSTALLABLE", "SYNC_PERMISSION_CHANGED")
    }

    private val store = SyncStore(database)
    suspend fun responsibility(identity: SyncIdentity) = store.responsibility(identity)

    private fun active(current: () -> Boolean) {
        if (!current()) throw CancellationException("SYNC_CONTEXT_CHANGED")
    }

    private suspend fun reconcile(identity: SyncIdentity, api: Api, current: () -> Boolean): List<SyncOutboxProof> {
        val proofs = mutableListOf<SyncOutboxProof>()
        val candidates = database.messages().pending(identity.namespace).take(32)
        for (row in candidates) {
            active(current)
            if (row.senderId != identity.user || row.sendAdmission.isBlank()) continue
            val body = runCatching { originalMessageBody(row) }.getOrNull() ?: continue
            try {
                val result = api.request("/conversations/${row.conversationId}/message-results", "POST", body)
                active(current)
                val message = result.optJSONObject("message") ?: result
                SyncProtocol.requireCommittedMessage(message, row.clientMsgId, row.conversationId, identity.user)
                proofs.add(SyncOutboxProof(row.clientMsgId, row.conversationId, body.toString()))
            } catch (error: Exception) {
                active(current)
                if (error is CancellationException || error is SessionVerificationRequired) throw error
                if (error is ApiFailure && error.code in setOf("NOT_A_MEMBER", "HISTORY_NOT_ALLOWED")) {
                    database.messages().freeze(identity.namespace, row.id)
                    throw SyncFailure("SYNC_PERMISSION_CHANGED")
                }
                if (error is ApiFailure && (error.status == 401 || error.code in setOf("RESTORE_EPOCH_CHANGED", "STORE_GENERATION_CHANGED", "DEPLOYMENT_CHANGED", "CONTEXT_CHANGED"))) throw error
                if (error is SyncFailure && error.reason in setOf("SYNC_CONTEXT_CHANGED", "SYNC_PERMISSION_CHANGED", "SYNC_LOCAL_RESTRICTION_CHANGED")) throw error
                // An auxiliary unknown result leaves the original queue intact, but cannot block a verified global deletion.
            }
        }
        return proofs
    }

    suspend fun run(identity: SyncIdentity, api: Api, reading: String?, current: () -> Boolean, restricted: (String) -> Unit = {}): SyncInstall =
        locks.getOrPut(identity.namespace) { Mutex() }.withLock {
            active(current)
            try {
                val bootstrap = api.request("/bootstrap")
                active(current)
                if (bootstrap.opt("deploymentId") != identity.deployment || bootstrap.opt("restoreEpoch") != identity.epoch) SyncWire.fail("SYNC_CONTEXT_CHANGED")
                if (bootstrap.optJSONObject("features")?.opt("authoritative_sync") != true) SyncWire.fail("SYNC_CAPABILITY_REQUIRED")
                var control = store.ensure(identity, current)
                val generation = SyncWire.id(control.opt("storeGeneration"))
                val registered = api.request("/sync/device", "PUT", JSONObject()
                    .put("requestId", control.getString("registrationRequestId")).put("storeGeneration", generation))
                active(current)
                val stream = SyncWire.id(registered.opt("streamId"))
                identity.checkBinding(registered, generation, stream)
                if (registered.opt("qualification") != "transport_only" || registered.opt("qualifiedAck") != false) SyncWire.fail()
                control = store.bind(identity, control, stream, current)
                val context = api.request("/sync/context")
                active(current)
                store.assertGuard(identity, control, current)
                identity.checkBinding(context, generation, stream)
                if (context.opt("qualification") != "transport_only" || context.opt("qualifiedAck") != false || context.opt("afterReceiptEnabled") != false) SyncWire.fail()
                SyncWire.counter(context.opt("head"))
                val permission = SyncWire.digest(context.opt("permissionGeneration"))
                if (control.optString("permissionGeneration").let { it.isNotEmpty() && it != permission }) {
                    store.restrict(identity, "SYNC_PERMISSION_CHANGED", true, current)
                    restricted("SYNC_PERMISSION_CHANGED")
                    control = store.ensure(identity, current)
                }
                if (control.has("cursor") && !control.has("restriction") && control.opt("permissionGeneration") == permission) {
                    val wire = api.request("/sync/changes?cursor=" + java.net.URLEncoder.encode(control.getString("cursor"), "UTF-8") + "&limit=500")
                    active(current)
                    val delta = SyncProtocol.changes(wire, control.getString("position"), identity.epoch, permission)
                    val proofs = reconcile(identity, api, current)
                    active(current)
                    return@withLock store.commit(identity, control, null, emptyList(), delta, delta.items, control.getString("classificationWatermark"), proofs, reading, current)
                }
                control = store.begin(identity, control, current)
                val request = control.getJSONObject("snapshotRequest")
                val raw = api.request("/sync/snapshots", "POST", JSONObject(request.toString()))
                active(current)
                val manifest = SyncManifest(raw).also { it.check(identity, generation, stream) }
                control = store.manifest(identity, manifest, request.getString("requestId"), control, current)
                val pages = mutableListOf<SyncPage>(); val values = mutableListOf<SyncProjection>(); val seen = mutableSetOf<String>()
                for (index in 0 until manifest.pages.length()) {
                    val page = api.request("/sync/snapshots/${manifest.id}/pages/$index")
                    active(current)
                    val parsed = SyncProtocol.page(page, manifest, index)
                    if (parsed.refs.any { !seen.add(it.key) }) SyncWire.fail("SYNC_DUPLICATE_REFERENCE")
                    store.page(identity, manifest, parsed, control, current)
                    pages.add(parsed); values.addAll(parsed.items)
                }
                val validationResult = api.request("/sync/snapshots/${manifest.id}/validate", "POST", JSONObject().put("pageDigests", JSONArray().also { a -> pages.forEach { a.put(it.digest) } }))
                active(current)
                val classification = SyncProtocol.validation(validationResult, manifest)
                // A validated cut is not shown until a current-permission delta confirms it.
                val changes = api.request("/sync/changes?cursor=" + java.net.URLEncoder.encode(manifest.token, "UTF-8") + "&limit=500")
                active(current)
                val delta = SyncProtocol.changes(changes, manifest.boundary, identity.epoch, manifest.permission)
                values.addAll(delta.items)
                val proofs = reconcile(identity, api, current)
                active(current)
                store.commit(identity, control, manifest, pages, delta, values, classification, proofs, reading, current)
            } catch (error: Exception) {
                if (error is CancellationException || !current()) throw error
                val reason = when (error) { is SyncFailure -> error.reason; is ApiFailure -> error.code; else -> "SYNC_UNAVAILABLE" }
                runCatching { store.restrict(identity, reason, reason in obsolete, current) }
                restricted(reason)
                throw error
            }
        }
}
