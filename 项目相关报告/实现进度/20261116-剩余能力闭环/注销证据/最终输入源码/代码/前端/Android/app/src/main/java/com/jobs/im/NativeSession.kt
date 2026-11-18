package com.jobs.im

import java.time.Instant
import org.json.JSONObject

data class SessionBinding(
    val profile: String,
    val origin: String,
    val deployment: String,
    val epoch: String,
)

// Credentials deliberately have no data-class toString: diagnostic output must not print secrets.
class NativeSession
private constructor(
    val binding: SessionBinding,
    val userId: String,
    val username: String,
    val deviceId: String,
    val token: String,
    val refreshToken: String,
    val familyId: String,
    val refreshGeneration: String,
    val accessExpiresAt: String,
    val idleExpiresAt: String,
    val absoluteExpiresAt: String,
    val sourceContextId: String,
) {
    override fun toString() = "NativeSession(redacted)"

    fun stored(): JSONObject =
        JSONObject()
            .put("version", 2)
            .put("profile", binding.profile)
            .put("origin", binding.origin)
            .put("deploymentId", binding.deployment)
            .put("restoreEpoch", binding.epoch)
            .put("userId", userId)
            .put("username", username)
            .put("deviceId", deviceId)
            .put("token", token)
            .put("refreshToken", refreshToken)
            .put("familyId", familyId)
            .put("refreshGeneration", refreshGeneration)
            .put("accessExpiresAt", accessExpiresAt)
            .put("idleExpiresAt", idleExpiresAt)
            .put("absoluteExpiresAt", absoluteExpiresAt)
            .put("sourceContextId", sourceContextId)

    companion object {
        fun delivery(result: JSONObject, binding: SessionBinding): NativeSession {
            require(result.getString("restoreEpoch") == binding.epoch) { "RESTORE_EPOCH_CHANGED" }
            val user = result.getJSONObject("user")
            return parse(result, binding, user.getString("id"), user.getString("username"))
        }

        fun restore(raw: JSONObject, expected: SessionBinding): NativeSession {
            require(
                raw.getInt("version") == 2 &&
                    raw.getString("profile") == expected.profile &&
                    raw.getString("origin") == expected.origin &&
                    raw.getString("deploymentId") == expected.deployment &&
                    raw.getString("restoreEpoch") == expected.epoch
            ) {
                "SESSION_BINDING_CHANGED"
            }
            return parse(raw, expected, raw.getString("userId"), raw.getString("username"))
        }

        private fun parse(
            value: JSONObject,
            binding: SessionBinding,
            userId: String,
            username: String,
        ): NativeSession {
            fun opaque(key: String): String =
                value.getString(key).also {
                    require(
                        it.isNotBlank() &&
                            it.length <= 16384 &&
                            !it.contains('\r') &&
                            !it.contains('\n')
                    ) {
                        "SESSION_INVALID"
                    }
                }
            require(value.get("refreshGeneration") is String) { "SESSION_INVALID" }
            val generation = value.getString("refreshGeneration")
            require(generation.matches(Regex("0|[1-9][0-9]*"))) { "SESSION_INVALID" }
            val access = value.getString("accessExpiresAt")
            val idle = value.getString("idleExpiresAt")
            val absolute = value.getString("absoluteExpiresAt")
            listOf(access, idle, absolute).forEach { Instant.parse(it) }
            require(
                value.getString("sourceContextId") == "personal" &&
                    userId.isNotBlank() &&
                    username.isNotBlank()
            ) {
                "SESSION_CONTEXT_INVALID"
            }
            return NativeSession(
                binding,
                userId,
                username,
                opaque("deviceId"),
                opaque("token"),
                opaque("refreshToken"),
                opaque("familyId"),
                generation,
                access,
                idle,
                absolute,
                "personal",
            )
        }
    }
}

data class InitialAuthIntent(
    val requestId: String,
    val purpose: String,
    val username: String,
    val previousFamilyId: String = "",
    val previousRequestId: String = "",
    val createdAt: Long = System.currentTimeMillis(),
    val deliveryExpired: Boolean = false,
    val previousRequestIds: List<String> = emptyList(),
) {
    fun json(): JSONObject =
        JSONObject()
            .put("requestId", requestId)
            .put("purpose", purpose)
            .put("username", username)
            .put("previousFamilyId", previousFamilyId)
            .put("previousRequestId", previousRequestId)
            .put("createdAt", createdAt)
            .put("deliveryExpired", deliveryExpired)
            .also { if (previousRequestIds.isNotEmpty()) it.put("previousRequestIds", org.json.JSONArray(previousRequestIds)) }

    companion object {
        fun parse(value: JSONObject) =
            InitialAuthIntent(
                    value.getString("requestId"),
                    value.getString("purpose"),
                    value.getString("username"),
                    value.optString("previousFamilyId"),
                    value.optString("previousRequestId"),
                    value.getLong("createdAt"),
                    value.optBoolean("deliveryExpired"),
                    value.optJSONArray("previousRequestIds")?.let { items ->
                        (0 until items.length()).map { items.getString(it) }
                    } ?: emptyList(),
                )
                .also {
                    require(
                        it.requestId.isNotBlank() &&
                            it.purpose in listOf("login", "register", "reauthenticate") &&
                            it.username.isNotBlank() && it.previousRequestIds.size <= 64 &&
                            it.previousRequestIds.distinct().size == it.previousRequestIds.size &&
                            it.previousRequestIds.all { id -> id.toByteArray(Charsets.UTF_8).size in 8..128 }
                    )
                }
    }
}

private fun JSONObject.removeCommit(): JSONObject {
    remove("commitId")
    return this
}

class NonSecretSessionState(private val context: android.content.Context) {
    private val preferences = context.getSharedPreferences("JobsIM.session-intents", 0)

    private fun key(profile: String) = Isolation.digest(profile)

    private fun read(profile: String) =
        preferences.getString(key(profile), null)?.let { JSONObject(it) } ?: JSONObject()

    private fun write(profile: String, value: JSONObject) {
        check(preferences.edit().putString(key(profile), value.toString()).commit()) {
            "SESSION_MARKER_SAVE_FAILED"
        }
    }

    fun unknownFamily(profile: String): String = read(profile).optString("unknownFamilyId")

    fun phase(profile: String): String = read(profile).optString("phase")

    fun blockedReason(profile: String): String =
        read(profile).optString("blockedReason", "REFRESH_REAUTH_REQUIRED")

    fun commitId(profile: String): String = read(profile).optString("commitId")

    fun stageSecureWrite(profile: String, familyId: String, commitId: String) {
        write(
            profile,
            read(profile)
                .put("unknownFamilyId", familyId)
                .put("unknown", true)
                .put("phase", "saving")
                .put("commitId", commitId),
        )
    }

    fun secureWriteFinished(profile: String, familyId: String, commitId: String) {
        val state = read(profile)
        require(
            state.optString("commitId") == commitId &&
                state.optString("unknownFamilyId") == familyId
        )
        write(profile, state.put("phase", "stored"))
    }

    fun markUnknown(profile: String, familyId: String, reason: String = "REFRESH_OUTCOME_UNKNOWN") {
        require(familyId.isNotBlank())
        write(
            profile,
            read(profile)
                .put("unknownFamilyId", familyId)
                .put("unknown", true)
                .put("phase", "unknown")
                .put("blockedReason", reason)
                .removeCommit(),
        )
    }

    fun intent(profile: String): InitialAuthIntent? =
        read(profile).optJSONObject("initialIntent")?.let { InitialAuthIntent.parse(it) }

    fun prepare(profile: String, intent: InitialAuthIntent) {
        val state = read(profile)
        val old = state.optJSONObject("initialIntent")
        if (old != null && old.optString("requestId") == intent.requestId) {
            val original = InitialAuthIntent.parse(old)
            require(
                original.purpose == intent.purpose &&
                    original.username == intent.username &&
                    original.previousFamilyId == intent.previousFamilyId &&
                    original.previousRequestId == intent.previousRequestId &&
                    original.previousRequestIds == intent.previousRequestIds
            ) {
                "AUTH_INTENT_CONFLICT"
            }
        }
        if (old != null && old.optString("requestId") != intent.requestId) {
            require(intent.previousRequestId == old.getString("requestId")) {
                "AUTH_INTENT_PENDING"
            }
            val history = state.optJSONArray("priorUnknownIntents") ?: org.json.JSONArray()
            history.put(old)
            state.put("priorUnknownIntents", history)
        }
        write(profile, state.put("initialIntent", intent.json()))
    }

    fun unresolvedRequestIds(profile: String): List<String> {
        val state = read(profile)
        val history = state.optJSONArray("priorUnknownIntents") ?: org.json.JSONArray()
        val records = (0 until history.length()).map { history.getJSONObject(it) }.toMutableList()
        state.optJSONObject("initialIntent")?.let { records.add(it) }
        val chain = linkedSetOf<String>()
        records.map { InitialAuthIntent.parse(it) }.forEach { intent ->
            chain.addAll(intent.previousRequestIds)
            if (intent.previousRequestId.isNotBlank()) chain.add(intent.previousRequestId)
            chain.add(intent.requestId)
        }
        require(chain.size <= 64) { "AUTHENTICATION_CHAIN_LIMIT" }
        return chain.toList()
    }

    fun expired(profile: String) {
        intent(profile)?.let { prepare(profile, it.copy(deliveryExpired = true)) }
    }

    fun clearKnownFamily(profile: String, familyId: String) {
        val state = read(profile)
        if (state.optString("unknownFamilyId") == familyId) {
            listOf("unknownFamilyId", "unknown", "phase", "commitId", "blockedReason").forEach {
                state.remove(it)
            }
        }
        if (
            state.optJSONObject("initialIntent") == null &&
                state.optString("unknownFamilyId").isEmpty()
        )
            clearAfterSecureCommit(profile)
        else write(profile, state)
    }

    fun clearAfterSecureCommit(profile: String) {
        check(preferences.edit().remove(key(profile)).commit()) { "SESSION_MARKER_SAVE_FAILED" }
    }
}
