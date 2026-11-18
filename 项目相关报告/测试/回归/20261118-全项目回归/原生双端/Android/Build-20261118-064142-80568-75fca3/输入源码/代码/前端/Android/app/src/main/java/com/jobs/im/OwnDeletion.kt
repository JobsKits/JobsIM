package com.jobs.im

import androidx.compose.runtime.*
import java.text.Normalizer
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.*
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONObject

data class OwnDeletionIntent(val meta: OwnPasswordIntent, val attempts: Int = 0, val state: String = "unsent",
    val result: String = "", val cancelId: String = "", val cancelState: String = "", val cancelVersion: String = "") {
    val id get() = meta.id
    fun json() = JSONObject().put("meta", meta.json()).put("attempts", attempts).put("state", state)
        .put("result", result).put("cancelId", cancelId).put("cancelState", cancelState).also { if(cancelVersion.isNotEmpty()) it.put("cancelVersion",cancelVersion) }
}
object OwnDeletionRules {
    val states = setOf("unsent", "unknown", "pending", "requested", "cancel_pending", "processing", "completed", "cancelled", "retired", "rejected")
    fun password(value: String): String = Normalizer.normalize(value, Normalizer.Form.NFC).also {
        require(it.isNotEmpty() && it.toByteArray(Charsets.UTF_8).size <= 512 && it.indices.none { i -> Character.isSurrogate(it[i]) && !(Character.isHighSurrogate(it[i]) && i + 1 < it.length && Character.isLowSurrogate(it[i + 1]) || Character.isLowSurrogate(it[i]) && i > 0 && Character.isHighSurrogate(it[i - 1])) })
    }
    fun definiteRejection(status: Int, code: String) = status == 400 && code == "INVALID_ACCOUNT_DELETION" || status == 403 && code == "DATA_RIGHTS_REJECTED" || status == 429 && code == "RATE_LIMITED"
    fun blocked(row: OwnDeletionIntent) = row.attempts > 0 && row.state !in setOf("unsent", "rejected")
    fun parse(value: JSONObject): OwnDeletionIntent {
        val fields=setOf("meta", "attempts", "state", "result", "cancelId", "cancelState")
        require(value.keys().asSequence().toSet() in setOf(fields,fields+"cancelVersion"))
        val meta = OwnPasswordRules.parse(value.getJSONObject("meta"))
        require(meta.attempts == 0 && meta.state == "unsent" && meta.result.isEmpty())
        val row = OwnDeletionIntent(meta, value.getInt("attempts"), value.getString("state"), value.getString("result"), value.getString("cancelId"), value.getString("cancelState"), if(value.has("cancelVersion")) SyncWire.counter(value.opt("cancelVersion")).also { require(it!="0") } else "")
        require(row.attempts >= 0 && row.state in states && row.cancelState in setOf("", "prepared", "unknown", "cancel_pending", "cancelled"))
        if (row.cancelId.isNotEmpty()) SyncWire.id(row.cancelId) else require(row.cancelState.isEmpty() && row.cancelVersion.isEmpty())
        if (row.result.isNotEmpty()) {
            val stored=JSONObject(row.result)
            require(stored.keys().asSequence().toSet()==setOf("deletionId","requestId","status","requestVersion","restoreEpoch","cancelUntil","cancelRequestId","requiresNewLogin") && (stored.opt("status")==row.state || row.state=="unknown"))
            result(stored,row)
        }
        return row
    }
    fun cancelReady(row: OwnDeletionIntent): Boolean = row.state=="requested" && row.result.isNotEmpty() && (row.cancelId.isEmpty() || row.cancelState=="prepared" && row.cancelVersion.isNotEmpty() && row.cancelVersion==JSONObject(row.result).getString("requestVersion"))
    fun cancelFrozen(row: OwnDeletionIntent): Boolean = row.cancelId.isNotEmpty() && (row.cancelVersion.isEmpty() || row.result.isNotEmpty() && row.cancelVersion!=JSONObject(row.result).getString("requestVersion"))
    fun result(value: JSONObject, row: OwnDeletionIntent): String {
        require(value.opt("requestId") == row.id && value.opt("restoreEpoch") == row.meta.epoch &&
            value.opt("status") in states - setOf("unsent", "unknown", "rejected") && value.opt("requiresNewLogin") == true)
        SyncWire.id(value.getString("deletionId"))
        if (row.result.isNotEmpty()) {
            val previous=JSONObject(row.result)
            require(previous.getString("deletionId")==value.getString("deletionId") && java.math.BigInteger(value.getString("requestVersion"))>=java.math.BigInteger(previous.getString("requestVersion")))
        }
        require(SyncWire.counter(value.opt("requestVersion")) != "0")
        val until = value.opt("cancelUntil")
        require(value.has("cancelUntil") && (until == JSONObject.NULL || until is String))
        require(until is String || value.getString("status") in setOf("pending", "retired"))
        if (until is String) Instant.parse(until)
        val cancellation = value.optString("cancelRequestId")
        require(cancellation.isEmpty() || cancellation.length in 8..128)
        require(cancellation.isEmpty() || row.cancelId.isEmpty() || cancellation==row.cancelId)
        return JSONObject().put("deletionId", value.getString("deletionId")).put("requestId", row.id)
            .put("restoreEpoch", row.meta.epoch).put("status", value.getString("status")).put("requestVersion", value.getString("requestVersion"))
            .put("cancelUntil", until).put("cancelRequestId", cancellation).put("requiresNewLogin", true).toString()
    }
}

class OwnDeletionModel(private val database: ChatDatabase, private val scope: CoroutineScope,
    private val environment: () -> PasswordEnvironment, private val owner: () -> PasswordOwner?,
    private val raw: (String) -> Api, private val stopOriginal: suspend (OwnDeletionIntent) -> Unit,
    private val prepareOwner: suspend () -> Unit = {}) {
    val rows = mutableStateListOf<OwnDeletionIntent>()
    var busy by mutableStateOf(false); private set
    var notice by mutableStateOf(""); private set
    var failed by mutableStateOf(false); private set
    private val lock = Mutex()
    private var active: Job? = null
    private fun namespace(profile: String) = "$profile|account-deletions"
    private fun key(row: OwnDeletionIntent) = "${namespace(row.meta.profile)}|${row.id}"
    private fun check(env: PasswordEnvironment) { if (environment() != env) throw CancellationException("DELETION_CONTEXT_CHANGED") }
    fun cancelSensitive() { active?.cancel() }
    fun blocked(family: String) = family.isNotEmpty() && rows.any { it.meta.profile == environment().profile && it.meta.family == family && OwnDeletionRules.blocked(it) }
    suspend fun load() {
        val env = environment()
        val found = database.syncRecords().rows(namespace(env.profile), "account-deletion").map {
            val row = OwnDeletionRules.parse(JSONObject(it.payload))
            require(row.meta.profile == env.profile && row.meta.origin == env.origin && row.meta.deployment == env.deployment && key(row) == it.key); row
        }.sortedByDescending { it.meta.created }
        check(env); rows.clear(); rows.addAll(found)
    }
    fun refresh() = scope.launch { try { load(); failed = false } catch (_: Exception) { failed = true } }
    private suspend fun save(before: OwnDeletionIntent?, after: OwnDeletionIntent, env: PasswordEnvironment) = database.withTransaction {
        check(env); OwnDeletionRules.parse(after.json())
        if (before == null) require(database.syncRecords().rows(namespace(env.profile), "account-deletion").none {
            val old = OwnDeletionRules.parse(JSONObject(it.payload)); old.meta.family == after.meta.family && old.state !in setOf("rejected", "retired", "cancelled", "completed")
        }) { "DELETION_ORIGINAL_PENDING" }
        else {
            require(before.meta==after.meta && after.attempts>=before.attempts && (before.cancelId.isEmpty() || before.cancelId==after.cancelId && before.cancelVersion==after.cancelVersion)) { "DELETION_ORIGINAL_CHANGED" }
            require(database.syncRecords().one(key(before))?.payload == before.json().toString()) { "DELETION_ORIGINAL_CHANGED" }
        }
        database.syncRecords().put(SyncRecord(key(after), namespace(env.profile), "account-deletion", after.json().toString())); check(env)
    }
    private suspend fun capability(row: OwnDeletionIntent, env: PasswordEnvironment): Boolean {
        val value = raw("").request("/bootstrap"); check(env)
        require(value.opt("deploymentId") == row.meta.deployment)
        return value.opt("restoreEpoch") == row.meta.epoch && value.optJSONObject("features")?.opt("own_account_deletion") == true
    }
    private fun launch(password: String, recovery: String, otp: String, action: suspend (PasswordEnvironment, CharArray, CharArray, CharArray) -> Unit) {
        if (busy) return
        val old = password.toCharArray(); val code = recovery.toCharArray(); val factor = otp.toCharArray(); val env = environment()
        busy = true; failed = false; notice = ""
        active = scope.launch {
            try { lock.withLock { check(env); load(); action(env, old, code, factor) } }
            catch (_: CancellationException) { /* Persisted unknown responsibility stays with its original owner. */ }
            catch (error: Exception) { if (environment() == env) { failed = true; notice = when (error.message) { "DELETION_CAPABILITY_REQUIRED" -> "unavailable"; "DELETION_ORIGINAL_PENDING" -> "blocked"; else -> "error" }; runCatching { load() } } }
            finally { old.fill('\u0000'); code.fill('\u0000'); factor.fill('\u0000'); busy = false; active = null }
        }
    }
    fun request(id: String?, password: String, otp: String, retry: Boolean) = launch(password, "", otp) { env, secret, _, factor ->
        val normalized = OwnDeletionRules.password(String(secret))
        require(factor.isEmpty() || String(factor).matches(Regex("[0-9]{6}")))
        if (id == null) { prepareOwner(); check(env) }
        var row = if (id == null) {
            val current = owner() ?: error("DELETION_ORIGINAL_FAMILY_REQUIRED")
            OwnDeletionIntent(OwnPasswordIntent(UUID.randomUUID().toString(), env.profile, env.origin, env.deployment, env.epoch,
                current.user, current.username, current.device, current.family, current.namespace, Instant.now().toString())).also {
                require(capability(it, env)) { "DELETION_CAPABILITY_REQUIRED" }; check(env); require(owner() == current); save(null, it, env)
            }
        } else rows.single { it.id == id }
        val available = capability(row, env)
        fun body(includeOTP: Boolean) = JSONObject().put("requestId", row.id).put("username", row.meta.username).put("deviceId", row.meta.device)
            .put("restoreEpoch", row.meta.epoch).put("password", normalized).also { if (includeOTP && factor.isNotEmpty()) it.put("otp", String(factor)) }
        var unseen = false
        var result = try { raw("").request("/auth/deletion-results", "POST", body(false)).let { JSONObject(OwnDeletionRules.result(it, row)) } }
            catch (error: ApiFailure) { check(env); if (error.status == 404 && error.code == "ACCOUNT_DELETION_NOT_FOUND") { unseen = true; null } else throw error }
        check(env)
        if (result == null && id != null && !retry) { notice = "unknown"; return@launch }
        if (result == null || result.optString("status") == "pending" && retry) {
            require(available) { "DELETION_CAPABILITY_REQUIRED" }
            val current = owner()
            val token = if (unseen && current?.family == row.meta.family && current.device == row.meta.device && current.user == row.meta.user && current.namespace == row.meta.namespace) current.token else ""
            require(!unseen || token.isNotEmpty()) { "DELETION_ORIGINAL_FAMILY_REQUIRED" }
            val before = row; row = row.copy(attempts = row.attempts + 1, state = "unknown"); save(before, row, env); load()
            try { result = JSONObject(OwnDeletionRules.result(raw(token).request("/auth/deletion-requests", "POST", body(token.isNotEmpty())), row)); check(env) }
            catch (error: ApiFailure) { check(env); if (id == null && before.attempts == 0 && OwnDeletionRules.definiteRejection(error.status, error.code)) save(row, row.copy(state = "rejected"), env); throw error }
        }
        publish(row, result ?: error("DELETION_RESULT_UNKNOWN"), env)
    }
    fun rights(id: String, password: String, recovery: String, otp: String, cancel: Boolean, expectedVersion: String = "") = launch(password, recovery, otp) { env, secret, code, factor ->
        var row = rows.single { it.id == id }; val original = JSONObject(row.result)
        require((secret.isNotEmpty()) != code.isNotEmpty() && code.size <= 128)
        require(!cancel || original.getString("requestVersion") == expectedVersion && OwnDeletionRules.cancelReady(row)) { "DELETION_CANCEL_QUERY_FIRST" }
        val purpose = if (cancel) "cancel" else "query"
        if (cancel && row.cancelId.isEmpty()) {
            val before = row; row = row.copy(cancelId = UUID.randomUUID().toString(), cancelState = "prepared", cancelVersion = expectedVersion); save(before, row, env); load()
        }
        val input = JSONObject().put("username", row.meta.username).put("deletionId", original.getString("deletionId"))
            .put("requestVersion", original.getString("requestVersion")).put("purpose", purpose)
        if (secret.isNotEmpty()) input.put("password", OwnDeletionRules.password(String(secret))) else input.put("recoveryCode", String(code))
        if (factor.isNotEmpty()) input.put("otp", String(factor))
        val proof = raw("").request("/auth/data-rights/proofs", "POST", input); check(env)
        require(proof.opt("purpose") == purpose && proof.opt("deletionId") == original.getString("deletionId") &&
            SyncWire.counter(proof.opt("requestVersion")) != "0" && proof.opt("restoreEpoch") == env.epoch)
        require(Instant.parse(proof.getString("expiresAt")).isAfter(Instant.now()) && proof.getString("proof").length in 32..128)
        require(java.math.BigInteger(proof.getString("requestVersion"))>=java.math.BigInteger(original.getString("requestVersion")))
        if (cancel) require(proof.opt("requestVersion") == row.cancelVersion)
        val body = JSONObject().put("proof", proof.getString("proof"))
        if (cancel) {
            body.put("requestId", row.cancelId)
            val before = row; row = row.copy(cancelState = "unknown"); save(before, row, env); load()
        }
        val value = raw("").request("/auth/data-rights/" + if (cancel) "cancel" else "query", "POST", body); check(env)
        val result=JSONObject(OwnDeletionRules.result(value,row))
        if(!cancel) require(result.getString("requestVersion")==proof.getString("requestVersion") && result.getString("deletionId")==proof.getString("deletionId"))
        publish(row, result, env, !cancel)
    }
    private suspend fun publish(before: OwnDeletionIntent, result: JSONObject, env: PasswordEnvironment, queried: Boolean = true) {
        check(env)
        val reportedCancel = result.optString("cancelRequestId")
        val row = before.copy(state = result.getString("status"), result = result.toString(), cancelState = when {
            before.cancelId.isNotEmpty() && reportedCancel == before.cancelId && result.getString("status") in setOf("cancel_pending", "cancelled") -> result.getString("status")
            queried && before.cancelState=="unknown" && before.cancelVersion.isNotEmpty() && before.cancelVersion==result.getString("requestVersion") && result.getString("status")=="requested" && reportedCancel.isEmpty() -> "prepared"
            else -> before.cancelState
        })
        save(before, row, env); load(); notice = row.state
        if (OwnDeletionRules.blocked(row)) stopOriginal(row)
    }
}
