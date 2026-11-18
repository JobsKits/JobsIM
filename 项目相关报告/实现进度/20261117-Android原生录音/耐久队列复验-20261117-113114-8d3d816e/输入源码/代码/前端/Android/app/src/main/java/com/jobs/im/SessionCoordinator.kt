package com.jobs.im

import java.math.BigInteger
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import org.json.JSONObject

class SessionVerificationRequired(val familyId: String, val reason: String) : Exception(reason)

private object SessionRegistry {
    private val locks = ConcurrentHashMap<String, Mutex>()
    private val sessions = ConcurrentHashMap<String, NativeSession>()

    fun key(binding: SessionBinding) =
        binding.profile + "|" + binding.origin + "|" + binding.deployment

    fun lock(binding: SessionBinding) = locks.getOrPut(key(binding)) { Mutex() }

    fun current(binding: SessionBinding) = sessions[key(binding)]

    fun publish(session: NativeSession) {
        sessions[key(session.binding)] = session
    }

    fun remove(session: NativeSession) {
        sessions.remove(key(session.binding), session)
    }
}

class SessionCapture internal constructor(val session: NativeSession, internal val owner: String) {
    override fun toString() = "SessionCapture(redacted)"
}

class SessionCoordinator(
    private val vault: TokenVault,
    val markers: NonSecretSessionState,
    private val database: () -> Messages,
    private val transport: (SessionBinding, String) -> Api,
    private val owner: () -> String,
    private val persist: (NativeSession, String) -> Unit = vault::saveSession,
) {
    var session: NativeSession? = null
        private set

    private fun sameIdentity(a: NativeSession, b: NativeSession) =
        a.binding == b.binding &&
            a.userId == b.userId &&
            a.familyId == b.familyId &&
            a.deviceId == b.deviceId &&
            a.sourceContextId == b.sourceContextId

    private fun requireOwner(tag: String) {
        if (owner() != tag) throw CancellationException("SESSION_OWNER_CHANGED")
    }

    fun capture(): SessionCapture? = session?.let { SessionCapture(it, owner()) }

    fun requireCurrent(capture: SessionCapture): NativeSession {
        requireOwner(capture.owner)
        val current = SessionRegistry.current(capture.session.binding)
        if (current == null || !sameIdentity(current, capture.session))
            throw CancellationException("SESSION_FAMILY_CHANGED")
        return current
    }

    fun forgetMemory() {
        session?.let { SessionRegistry.remove(it) }
        session = null
    }

    suspend fun restore(binding: SessionBinding, tag: String): NativeSession? =
        SessionRegistry.lock(binding).withLock {
            requireOwner(tag)
            val unknown = markers.unknownFamily(binding.profile)
            val restored = withContext(Dispatchers.IO) { vault.loadSession(binding) }
            if (restored == null) {
                if (unknown.isNotEmpty())
                    throw SessionVerificationRequired(
                        unknown,
                        markers.blockedReason(binding.profile),
                    )
                return@withLock null
            }
            requireOwner(tag)
            if (unknown.isNotEmpty()) {
                val receipt = withContext(Dispatchers.IO) { vault.savedCommit(binding) }
                if (
                    markers.phase(binding.profile) != "stored" ||
                        receipt.isBlank() ||
                        markers.commitId(binding.profile) != receipt ||
                        unknown != restored.familyId
                )
                    throw SessionVerificationRequired(
                        unknown,
                        markers.blockedReason(binding.profile),
                    )
            } else if (markers.intent(binding.profile) != null) {
                throw SessionVerificationRequired(
                    restored.familyId,
                    "AUTHENTICATION_OUTCOME_UNKNOWN",
                )
            }
            database().adoptNative(restored)
            requireOwner(tag)
            if (unknown.isNotEmpty()) markers.clearAfterSecureCommit(binding.profile)
            SessionRegistry.publish(restored)
            session = restored
            restored
        }

    suspend fun prepareInitial(binding: SessionBinding, intent: InitialAuthIntent, tag: String) {
        SessionRegistry.lock(binding).withLock {
            requireOwner(tag)
            markers.prepare(binding.profile, intent)
        }
    }

    suspend fun commitInitial(
        delivery: NativeSession,
        initialRequestId: String,
        tag: String,
    ): NativeSession =
        SessionRegistry.lock(delivery.binding).withLock {
            requireOwner(tag)
            if (markers.intent(delivery.binding.profile)?.requestId != initialRequestId)
                throw CancellationException("AUTH_INTENT_SUPERSEDED")
            commitLocked(delivery, tag)
        }

    private suspend fun commitLocked(delivery: NativeSession, tag: String): NativeSession {
        requireOwner(tag)
        val b = delivery.binding
        val commitId = UUID.randomUUID().toString()
        markers.stageSecureWrite(b.profile, delivery.familyId, commitId)
        try {
            withContext(Dispatchers.IO) { persist(delivery, commitId) }
            requireOwner(tag)
            val receipt = withContext(Dispatchers.IO) { vault.savedCommit(b) }
            requireOwner(tag)
            check(receipt == commitId) { "SESSION_STORAGE_FAILED" }
            markers.secureWriteFinished(b.profile, delivery.familyId, commitId)
            database().adoptNative(delivery)
            requireOwner(tag)
            markers.clearAfterSecureCommit(b.profile)
        } catch (error: Exception) {
            if (error !is CancellationException)
                runCatching {
                    markers.markUnknown(b.profile, delivery.familyId, "SESSION_STORAGE_FAILED")
                }
            throw if (error is CancellationException) error
            else SessionVerificationRequired(delivery.familyId, "SESSION_STORAGE_FAILED")
        }
        SessionRegistry.publish(delivery)
        session = delivery
        return delivery
    }

    suspend fun refreshed(capture: SessionCapture): NativeSession =
        SessionRegistry.lock(capture.session.binding).withLock {
            val active = requireCurrent(capture)
            if (active.token != capture.session.token) {
                session = active
                return@withLock active
            }
            val b = active.binding
            if (markers.unknownFamily(b.profile).isNotEmpty() || markers.intent(b.profile) != null)
                throw SessionVerificationRequired(
                    markers.unknownFamily(b.profile).ifBlank { active.familyId },
                    markers.blockedReason(b.profile),
                )
            val bootstrap = transport(b, "").request("/bootstrap")
            requireCurrent(capture)
            require(
                bootstrap.getString("deploymentId") == b.deployment &&
                    bootstrap.getString("restoreEpoch") == b.epoch
            ) {
                "SESSION_BINDING_CHANGED"
            }
            require(bootstrap.optJSONObject("features")?.optBoolean("auth_refresh") == true) {
                "AUTH_REFRESH_UNSUPPORTED"
            }
            markers.markUnknown(b.profile, active.familyId)
            val result =
                try {
                    transport(b, "")
                        .request(
                            "/auth/refresh",
                            "POST",
                            JSONObject()
                                .put("refreshToken", active.refreshToken)
                                .put("familyId", active.familyId)
                                .put("refreshGeneration", active.refreshGeneration)
                                .put("requestId", UUID.randomUUID().toString()),
                        )
                } catch (error: Exception) {
                    if (error is CancellationException) throw error
                    val reason =
                        (error as? ApiFailure)?.code?.takeIf {
                            it in
                                setOf(
                                    "REFRESH_REUSE_DETECTED",
                                    "REFRESH_INVALID",
                                    "REFRESH_EXPIRED",
                                    "REFRESH_REAUTH_REQUIRED",
                                )
                        } ?: "REFRESH_OUTCOME_UNKNOWN"
                    markers.markUnknown(b.profile, active.familyId, reason)
                    throw SessionVerificationRequired(active.familyId, reason)
                }
            requireCurrent(capture)
            val next =
                try {
                    NativeSession.delivery(result, b).also {
                        require(
                            sameIdentity(active, it) &&
                                BigInteger(it.refreshGeneration) ==
                                    BigInteger(active.refreshGeneration) + BigInteger.ONE &&
                                java.time.Instant.parse(it.absoluteExpiresAt) ==
                                    java.time.Instant.parse(active.absoluteExpiresAt)
                        ) {
                            "SESSION_ROTATION_INVALID"
                        }
                    }
                } catch (_: Exception) {
                    throw SessionVerificationRequired(active.familyId, "REFRESH_OUTCOME_UNKNOWN")
                }
            commitLocked(next, capture.owner)
        }

    suspend fun reject(capture: SessionCapture, reason: String): Nothing {
        SessionRegistry.lock(capture.session.binding).withLock {
            requireCurrent(capture)
            markers.markUnknown(capture.session.binding.profile, capture.session.familyId, reason)
        }
        throw SessionVerificationRequired(capture.session.familyId, reason)
    }

    suspend fun clearKnown(session: NativeSession) {
        SessionRegistry.lock(session.binding).withLock {
            val current = SessionRegistry.current(session.binding)
            if (current != null && sameIdentity(current, session)) {
                val saved = withContext(Dispatchers.IO) { vault.loadSession(session.binding) }
                if (saved != null && sameIdentity(saved, session))
                    withContext(Dispatchers.IO) { vault.clear(session.binding.profile) }
                markers.clearKnownFamily(session.binding.profile, session.familyId)
                database().retireNative(session.binding)
                SessionRegistry.remove(current)
            }
            this.session = null
        }
    }

    suspend fun clearLegacy(binding: SessionBinding, expected: String) {
        SessionRegistry.lock(binding).withLock {
            if (SessionRegistry.current(binding) == null && vault.loadLegacy(binding.profile) == expected)
                withContext(Dispatchers.IO) { vault.clear(binding.profile) }
        }
    }

    suspend fun commitLegacy(binding: SessionBinding, credential: String, tag: String) {
        SessionRegistry.lock(binding).withLock {
            requireOwner(tag)
            if (SessionRegistry.current(binding) != null || markers.intent(binding.profile) != null ||
                markers.unknownFamily(binding.profile).isNotBlank())
                throw CancellationException("SESSION_FAMILY_CHANGED")
            val paired = withContext(Dispatchers.IO) { vault.loadSession(binding) }
            requireOwner(tag)
            if (paired != null) throw CancellationException("SESSION_FAMILY_CHANGED")
            withContext(Dispatchers.IO) { vault.save(binding.profile, credential) }
            requireOwner(tag)
        }
    }

    suspend fun invalidateEpoch(binding: SessionBinding, originalLegacy: String, tag: String) {
        SessionRegistry.lock(binding).withLock {
            requireOwner(tag)
            database().freezeScope("live|${binding.profile}|${binding.deployment}|", "|${binding.epoch}")
            requireOwner(tag)
            val current = SessionRegistry.current(binding)
            // An older model may observe the epoch change after another model completed recovery.
            if (current != null && current.binding != binding) return@withLock
            val removable = withContext(Dispatchers.IO) {
                runCatching { vault.loadSession(binding) }.getOrNull() != null ||
                    originalLegacy.isNotEmpty() && runCatching { vault.loadLegacy(binding.profile) }.getOrNull() == originalLegacy
            }
            requireOwner(tag)
            if (removable) withContext(Dispatchers.IO) { vault.clear(binding.profile) }
            requireOwner(tag)
            current?.let { SessionRegistry.remove(it) }
            if (session?.binding == binding) session = null
        }
    }
}

class SessionApi(
    private val binding: SessionBinding,
    private val coordinator: SessionCoordinator,
    private val raw: (String) -> Api,
    private val blocked: (SessionVerificationRequired) -> Unit,
    ca: ByteArray?,
) : Api(binding.origin, ca = ca) {
    private val original = coordinator.capture()

    private suspend fun <T> authenticated(action: suspend (Api) -> T): T {
        val capture = original ?: throw CancellationException("SESSION_NOT_ACTIVE")
        val active = coordinator.requireCurrent(capture)
        if (active.binding != binding) throw CancellationException("SESSION_BINDING_CHANGED")
        try {
            val result = action(raw(active.token))
            coordinator.requireCurrent(capture)
            return result
        } catch (failure: ApiFailure) {
            if (failure.status != 401) throw failure
            try {
                val now = coordinator.requireCurrent(capture)
                val renewed =
                    when {
                        now.token != active.token -> now
                        failure.code == "ACCESS_EXPIRED" -> coordinator.refreshed(capture)
                        else -> coordinator.reject(capture, failure.code)
                    }
                coordinator.requireCurrent(capture)
                val result =
                    try {
                        action(raw(renewed.token))
                    } catch (second: ApiFailure) {
                        if (second.status == 401) coordinator.reject(capture, second.code)
                        else throw second
                    }
                coordinator.requireCurrent(capture)
                return result
            } catch (error: SessionVerificationRequired) {
                coordinator.requireCurrent(capture)
                blocked(error)
                throw error
            }
        }
    }

    override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
        val original = data?.toString()
        return authenticated {
            it.request(path, method, original?.let { body -> JSONObject(body) })
        }
    }

    override suspend fun migration(path:String,method:String,data:JSONObject?,store:String,current:()->Boolean):JSONObject {
        val body=data?.toString();return authenticated{it.migration(path,method,body?.let{raw->JSONObject(raw)},store,current)}
    }
    override suspend fun migrationBytes(path:String,store:String,limit:Int,current:()->Boolean):ByteArray {
        var received:ByteArray?=null;return try{authenticated{it.migrationBytes(path,store,limit,current).also{value->received=value}}}catch(error:Exception){received?.fill(0);throw error}
    }

    override suspend fun upload(
        name: String,
        mime: String,
        bytes: ByteArray,
        path: String,
        fields: Map<String, String>,
    ): JSONObject = authenticated { it.upload(name, mime, bytes, path, fields) }

    override suspend fun download(path: String, limit: Int): Pair<ByteArray, String> =
        authenticated {
            it.download(path, limit)
        }

    override suspend fun downloadExport(path: String, downloadToken: String, directory: java.io.File,
        current: () -> Boolean, progress: (Long, Long?) -> Unit): PersonalExportFile {
        var received: PersonalExportFile? = null
        return try {
            authenticated { it.downloadExport(path, downloadToken, directory, current, progress).also { file -> received = file } }
        } catch (error: Exception) { received?.dispose(); throw error }
    }
}

// A pre-refresh singleton credential remains useful for reading its old instance only.
class LegacyReadOnlyApi(base: String, private val raw: Api, ca: ByteArray?) : Api(base, ca = ca) {
    override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
        require(method == "GET" || method == "HEAD" || path == "/auth/logout" && method == "POST") {
            "AUTH_REFRESH_UNSUPPORTED"
        }
        return raw.request(path, method, data)
    }

    override suspend fun migration(path:String,method:String,data:JSONObject?,store:String,current:()->Boolean):JSONObject = error("AUTH_REFRESH_UNSUPPORTED")
    override suspend fun migrationBytes(path:String,store:String,limit:Int,current:()->Boolean):ByteArray = error("AUTH_REFRESH_UNSUPPORTED")

    override suspend fun upload(name: String, mime: String, bytes: ByteArray, path: String,
        fields: Map<String, String>): JSONObject = error("AUTH_REFRESH_UNSUPPORTED")

    override suspend fun download(path: String, limit: Int) = raw.download(path, limit)

    override suspend fun downloadExport(path: String, downloadToken: String, directory: java.io.File,
        current: () -> Boolean, progress: (Long, Long?) -> Unit): PersonalExportFile = error("AUTH_REFRESH_UNSUPPORTED")
}
