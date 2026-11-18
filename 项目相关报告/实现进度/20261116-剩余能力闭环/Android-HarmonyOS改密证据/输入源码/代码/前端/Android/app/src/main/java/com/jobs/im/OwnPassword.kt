package com.jobs.im

import androidx.compose.runtime.*
import java.text.Normalizer
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.*
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONObject

data class PasswordEnvironment(val profile: String, val origin: String, val deployment: String, val epoch: String, val stamp: String)
data class PasswordOwner(val user: String, val username: String, val device: String, val family: String, val namespace: String, val token: String) {
    override fun toString() = "PasswordOwner(redacted)"
}
data class OwnPasswordIntent(val id: String, val profile: String, val origin: String, val deployment: String, val epoch: String,
    val user: String, val username: String, val device: String, val family: String, val namespace: String, val created: String,
    val attempts: Int = 0, val state: String = "unsent", val result: String = "") {
    fun json() = JSONObject().put("id", id).put("profile", profile).put("origin", origin).put("deployment", deployment).put("epoch", epoch)
        .put("user", user).put("username", username).put("device", device).put("family", family).put("namespace", namespace)
        .put("created", created).put("attempts", attempts).put("state", state).put("result", result)
}

object OwnPasswordRules {
    fun normalized(current: String, replacement: String, confirmation: String): Pair<String, String> {
        val old = Normalizer.normalize(current, Normalizer.Form.NFC)
        val next = Normalizer.normalize(replacement, Normalizer.Form.NFC)
        require(Normalizer.normalize(confirmation, Normalizer.Form.NFC) == next && old != next &&
            old.isNotEmpty() && old.toByteArray(Charsets.UTF_8).size <= 512 && next.toByteArray(Charsets.UTF_8).size <= 512 &&
            next.codePointCount(0, next.length) in 15..128 &&
            listOf(old, next).none { text -> text.indices.any { i -> Character.isSurrogate(text[i]) &&
                !(Character.isHighSurrogate(text[i]) && i + 1 < text.length && Character.isLowSurrogate(text[i + 1]) ||
                    Character.isLowSurrogate(text[i]) && i > 0 && Character.isHighSurrogate(text[i - 1])) } }) { "PASSWORD_CHANGE_INPUT_INVALID" }
        return old to next
    }
    fun blocked(row: OwnPasswordIntent) = row.state == "changed" || row.attempts > 0 && row.state in setOf("unknown", "pending")
    fun parse(value: JSONObject): OwnPasswordIntent {
        require(value.keys().asSequence().toSet() == setOf("id", "profile", "origin", "deployment", "epoch", "user", "username", "device", "family", "namespace", "created", "attempts", "state", "result"))
        val row = OwnPasswordIntent(value.getString("id"), value.getString("profile"), value.getString("origin"), value.getString("deployment"), value.getString("epoch"),
            value.getString("user"), value.getString("username"), value.getString("device"), value.getString("family"), value.getString("namespace"), value.getString("created"), value.getInt("attempts"), value.getString("state"), value.getString("result"))
        listOf(row.id, row.deployment, row.user, row.device, row.family).forEach { SyncWire.id(it) }
        require(row.profile.isNotEmpty() && row.namespace.isNotEmpty() && row.username.matches(Regex("[a-z0-9_]{3,32}")) &&
            row.epoch.matches(Regex("[1-9][0-9]*")) && row.attempts >= 0 && row.state in setOf("unsent", "unknown", "pending", "changed", "retired", "rejected"))
        Instant.parse(row.created)
        if (row.result.isNotEmpty()) result(JSONObject(row.result), row)
        return row
    }
    fun result(value: JSONObject, original: OwnPasswordIntent): String {
        require(value.opt("requestId") == original.id && value.opt("restoreEpoch") == original.epoch &&
            value.opt("status") in setOf("pending", "changed", "retired") && value.opt("current") is Boolean && value.opt("requiresNewLogin") is Boolean)
        for (key in listOf("credentialGeneration", "securityVersion")) require((value.opt(key) as? String)?.matches(Regex("[1-9][0-9]*")) == true)
        val state = value.getString("status")
        require(!value.getBoolean("current") || state == "changed")
        require(state != "changed" || value.getBoolean("requiresNewLogin"))
        // Only the public result projection can reach ordinary persistent storage.
        return JSONObject().put("requestId", original.id).put("restoreEpoch", original.epoch).put("status", state)
            .put("credentialGeneration", value.getString("credentialGeneration")).put("securityVersion", value.getString("securityVersion"))
            .put("current", value.getBoolean("current")).put("requiresNewLogin", value.getBoolean("requiresNewLogin")).toString()
    }
}

class OwnPasswordModel(private val database: ChatDatabase, private val scope: CoroutineScope,
    private val environment: () -> PasswordEnvironment, private val owner: () -> PasswordOwner?,
    private val raw: (String) -> Api, private val stopOriginal: suspend (OwnPasswordIntent) -> Unit,
    private val prepareOwner: suspend () -> Unit = {}) {
    val rows = mutableStateListOf<OwnPasswordIntent>()
    var busy by mutableStateOf(false); private set
    var notice by mutableStateOf(""); private set
    var failed by mutableStateOf(false); private set
    private val lock = Mutex()
    private val records get() = database.syncRecords()
    private fun namespace(profile: String) = "$profile|password-changes"
    private fun key(row: OwnPasswordIntent) = "${namespace(row.profile)}|${row.id}"
    private fun check(env: PasswordEnvironment) { if (environment() != env) throw CancellationException("PASSWORD_CHANGE_CONTEXT_CHANGED") }
    fun blocked(family: String) = family.isNotEmpty() && rows.any { it.profile == environment().profile && it.family == family && OwnPasswordRules.blocked(it) }
    suspend fun load() {
        val env = environment()
        val values = records.rows(namespace(env.profile), "password-change").map {
            val row = OwnPasswordRules.parse(JSONObject(it.payload)); require(row.profile == env.profile && row.origin == env.origin && row.deployment == env.deployment && key(row) == it.key); row
        }.sortedByDescending { it.created }
        check(env); rows.clear(); rows.addAll(values)
    }
    fun refresh() = scope.launch { try { load(); failed = false } catch (_: Exception) { failed = true } }
    private suspend fun insert(row: OwnPasswordIntent, env: PasswordEnvironment) = database.withTransaction {
        check(env)
        require(records.rows(namespace(row.profile), "password-change").none { OwnPasswordRules.parse(JSONObject(it.payload)).let { old -> old.family == row.family && old.state in setOf("unsent", "unknown", "pending") } }) { "PASSWORD_CHANGE_ORIGINAL_PENDING" }
        records.put(SyncRecord(key(row), namespace(row.profile), "password-change", row.json().toString())); check(env)
    }
    private suspend fun replace(before: OwnPasswordIntent, after: OwnPasswordIntent, env: PasswordEnvironment) = database.withTransaction {
        check(env)
        val stored = records.one(key(before)) ?: error("PASSWORD_CHANGE_ORIGINAL_CHANGED")
        require(stored.payload == before.json().toString()) { "PASSWORD_CHANGE_ORIGINAL_CHANGED" }
        OwnPasswordRules.parse(after.json()); records.put(stored.copy(payload = after.json().toString())); check(env)
    }
    private suspend fun capability(row: OwnPasswordIntent, env: PasswordEnvironment, required: Boolean): Boolean {
        check(env); val boot = raw("").request("/bootstrap"); check(env)
        require(boot.opt("deploymentId") == row.deployment)
        val available = boot.opt("restoreEpoch") == row.epoch && boot.optJSONObject("features")?.opt("own_password_change") == true
        require(!required || available) { "PASSWORD_CHANGE_CAPABILITY_REQUIRED" }; return available
    }
    fun perform(id: String?, oldInput: String, newInput: String, confirmation: String, otp: String, retry: Boolean) {
        if (busy) return
        val pair = try { OwnPasswordRules.normalized(oldInput, newInput, confirmation) } catch (_: Exception) { failed = true; notice = "invalid"; return }
        if (otp.isNotEmpty() && !otp.matches(Regex("[0-9]{6}"))) { failed = true; notice = "invalid"; return }
        val old = pair.first.toCharArray(); val next = pair.second.toCharArray(); val code = otp.toCharArray()
        val env = environment(); busy = true; failed = false; notice = ""
        scope.launch {
            try { lock.withLock {
                check(env); load()
                if (id == null) { prepareOwner(); check(env) }
                var row = if (id == null) {
                    val current = owner() ?: error("PASSWORD_CHANGE_ORIGINAL_FAMILY_REQUIRED")
                    OwnPasswordIntent(UUID.randomUUID().toString(), env.profile, env.origin, env.deployment, env.epoch, current.user,
                        current.username, current.device, current.family, current.namespace, Instant.now().toString()).also {
                        capability(it, env, true); check(env); require(owner() == current); insert(it, env)
                    }
                } else rows.single { it.id == id }
                val available = capability(row, env, id == null)
                fun proof(includeOTP: Boolean) = JSONObject().put("requestId", row.id).put("username", row.username).put("deviceId", row.device)
                    .put("restoreEpoch", row.epoch).put("currentPassword", String(old)).put("newPassword", String(next)).also { if (includeOTP && code.isNotEmpty()) it.put("otp", String(code)) }
                var unseen = false
                val lookup = try { raw("").request("/auth/password-change-results", "POST", proof(false)) }
                    catch (error: ApiFailure) { check(env); if (error.status == 404 && error.code == "PASSWORD_CHANGE_OPERATION_NOT_FOUND") { unseen = true; null } else throw error }
                check(env)
                var result = lookup?.let { JSONObject(OwnPasswordRules.result(it, row)) }
                if (result == null && id != null && !retry) { notice = "unknown"; load(); return@withLock }
                if (result == null || result.optString("status") == "pending" && retry) {
                    val candidate = owner()
                    val credential = if (available && unseen && candidate != null && candidate.family == row.family && candidate.device == row.device && candidate.user == row.user && candidate.namespace == row.namespace) candidate.token else ""
                    require(id != null || credential.isNotEmpty()) { "PASSWORD_CHANGE_ORIGINAL_FAMILY_REQUIRED" }
                    val prior = row
                    row = row.copy(attempts = row.attempts + 1, state = "unknown")
                    replace(prior, row, env); load() // Freeze ordinary business before the first creating request.
                    try { result = JSONObject(OwnPasswordRules.result(raw(credential).request("/auth/password-change", "POST", proof(credential.isNotEmpty())), row)); check(env) }
                    catch (error: ApiFailure) {
                        check(env)
                        if (id == null && prior.attempts == 0 && error.status in setOf(400, 401, 403, 429)) replace(row, row.copy(state = "rejected"), env)
                        throw error
                    }
                }
                val value = result ?: error("PASSWORD_CHANGE_RESULT_UNKNOWN")
                val nextRow = row.copy(state = value.getString("status"), result = value.toString())
                replace(row, nextRow, env); load(); notice = nextRow.state
                if (value.getBoolean("requiresNewLogin")) stopOriginal(nextRow)
            } } catch (_: CancellationException) { /* A late operation keeps its old public responsibility. */ }
            catch (error: Exception) { if (environment() == env) { failed = true; notice = when (error.message) { "PASSWORD_CHANGE_CAPABILITY_REQUIRED" -> "unavailable"; "PASSWORD_CHANGE_ORIGINAL_PENDING" -> "blocked"; else -> "error" }; runCatching { load() } } }
            finally { old.fill('\u0000'); next.fill('\u0000'); code.fill('\u0000'); busy = false }
        }
    }
}
