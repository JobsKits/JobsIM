package com.jobs.im

import androidx.compose.runtime.*
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.*
import org.json.JSONObject

data class GroupAnnouncement(
    val conversationId: String,
    val version: String,
    val state: String,
    val canManage: Boolean,
    val source: MessageRow?,
    val expiresAt: String = "",
)

class AnnouncementModel(
    private val scope: CoroutineScope,
    private val context: () -> String,
    private val transport: () -> Api,
    private val database: () -> Messages,
    private val namespace: () -> String,
    private val epoch: () -> String,
    private val user: () -> String,
    private val verifyRole: suspend () -> Boolean,
) {
    var group by mutableStateOf<Conversation?>(null)
        private set

    var current by mutableStateOf<GroupAnnouncement?>(null)
        private set

    var source by mutableStateOf<MessageRow?>(null)
        private set

    var pending by mutableStateOf<LocalOperation?>(null)
        private set

    var loading by mutableStateOf(false)
        private set

    var problem by mutableStateOf("")
        private set

    var roleProtected by mutableStateOf(false)
        private set

    private var serial = 0
    private var task: Job? = null
    val terminalPending: Boolean
        get() =
            pending?.let {
                runCatching { JSONObject(it.payload).optString("terminalError").isNotEmpty() }
                    .getOrDefault(false)
            } ?: false

    fun reset() {
        task?.cancel()
        serial++
        group = null
        current = null
        source = null
        pending = null
        loading = false
        problem = ""
        roleProtected = false
    }

    private fun work(action: suspend (String, Int) -> Unit): Job {
        if (loading) return scope.launch {}
        val stamp = context()
        val serial = this.serial
        loading = true
        problem = ""
        return scope
            .launch {
                try {
                    action(stamp, serial)
                } catch (e: Exception) {
                    if (
                        e !is CancellationException &&
                            context() == stamp &&
                            this@AnnouncementModel.serial == serial
                    ) {
                        if (e is ApiFailure && e.status in listOf(401, 403, 404)) {
                            current = null
                            source = null
                        }
                        problem =
                            if (e is ApiFailure && e.code.contains("SOURCE"))
                                "announcementUnavailable"
                            else "failure"
                    }
                } finally {
                    if (context() == stamp && this@AnnouncementModel.serial == serial)
                        loading = false
                }
            }
            .also { task = it }
    }

    private fun guard(stamp: String, serial: Int) {
        if (context() != stamp || this.serial != serial)
            throw CancellationException("CONTEXT_CHANGED")
    }

    private fun parse(value: JSONObject, ns: String, restoreEpoch: String): GroupAnnouncement {
        val state = value.getString("state")
        require(state in listOf("empty", "active", "unavailable", "expired"))
        val source =
            if (state == "active") message(value.getJSONObject("message"), ns, restoreEpoch)
            else null
        require(
            source == null ||
                (!source.retracted &&
                    !source.contentUnavailable &&
                    !source.forwardUnavailable &&
                    source.conversationId == value.getString("conversationId"))
        )
        return GroupAnnouncement(
            value.getString("conversationId"),
            value.getString("version"),
            state,
            value.getBoolean("canManage"),
            source,
            value.optString("expiresAt").takeUnless { it == "null" }.orEmpty(),
        )
    }

    fun load(snapshot: Conversation): Job {
        reset()
        group = snapshot
        return work { stamp, serial ->
            roleProtected = verifyRole()
            guard(stamp, serial)
            val api = transport()
            val ns = namespace()
            val restoreEpoch = epoch()
            val latest = conversation(api.request("/conversations/${snapshot.id}"), snapshot.title)
            val value = api.request("/conversations/${snapshot.id}/announcement")
            val original = database().operation(ns, "announcement-${snapshot.id}")
            val protected = verifyRole()
            guard(stamp, serial)
            require(latest.id == snapshot.id && latest.kind == "group")
            group = latest
            current = parse(value, ns, restoreEpoch)
            pending = original
            roleProtected = protected
        }
    }

    fun reload(): Job = group?.let { load(it) } ?: scope.launch {}

    fun choose(row: MessageRow): Job = work { stamp, serial ->
        val group = group ?: return@work
        require(row.namespace == namespace() && row.conversationId == group.id)
        val latest = message(transport().request("/messages/${row.id}"), namespace(), epoch())
        val protected = verifyRole()
        guard(stamp, serial)
        require(
            latest.id == row.id &&
                latest.conversationId == group.id &&
                !latest.retracted &&
                !latest.contentUnavailable &&
                !latest.forwardUnavailable
        )
        source = latest
        roleProtected = protected
    }

    fun publish(expiryDays: Int = 0): Job = write("PUT", expiryDays)

    fun clear(): Job = write("DELETE", 0)

    private fun write(method: String, expiryDays: Int): Job = work { stamp, serial ->
        val group = group ?: return@work
        val current = current ?: return@work
        roleProtected = verifyRole()
        guard(stamp, serial)
        require(roleProtected && group.roleGeneration.isNotBlank()) { "QUALIFICATION_CHANGED" }
        require(current.canManage && group.ownerId == user() && !group.readOnly)
        require(group.membershipGeneration.isNotBlank() && expiryDays in listOf(0, 1, 7))
        val ns = namespace()
        val purpose = "announcement-${group.id}"
        val dao = database()
        var original = dao.operation(ns, purpose)
        guard(stamp, serial)
        if (
            original != null && JSONObject(original.payload).optString("terminalError").isNotEmpty()
        ) {
            dao.archiveOperation(original)
            guard(stamp, serial)
            original = null
        }
        if (original == null) {
            val requestId = UUID.randomUUID().toString()
            val body =
                JSONObject()
                    .put("requestId", requestId)
                    .put("expectedVersion", current.version)
                    .put("membershipGeneration", group.membershipGeneration)
                    .put("roleGeneration", group.roleGeneration)
            if (method == "PUT") {
                val selected = source ?: error("SOURCE_REQUIRED")
                require(
                    selected.namespace == ns &&
                        selected.conversationId == group.id &&
                        !selected.retracted &&
                        !selected.contentUnavailable &&
                        !selected.forwardUnavailable
                )
                body
                    .put("messageId", selected.id)
                    .put("sourceVersion", selected.contentVersion.ifBlank { selected.version })
                if (expiryDays > 0)
                    body.put("expiresAt", Instant.now().plusSeconds(expiryDays * 86400L).toString())
            }
            val envelope =
                JSONObject().put("conversationId", group.id).put("method", method).put("body", body)
            original =
                dao.prepareOperation(
                    LocalOperation(
                        ns + "|" + purpose,
                        ns,
                        purpose,
                        requestId,
                        envelope.toString(),
                        restoreEpoch = epoch(),
                    )
                )
            guard(stamp, serial)
        }
        pending = original
        submit(original, stamp, serial)
    }

    fun retry(): Job = work { stamp, serial ->
        val group = group ?: return@work
        val original = database().operation(namespace(), "announcement-${group.id}") ?: return@work
        guard(stamp, serial)
        require(JSONObject(original.payload).optString("terminalError").isEmpty())
        pending = original
        submit(original, stamp, serial)
    }

    private suspend fun submit(original: LocalOperation, stamp: String, serial: Int) {
        require(original.namespace == namespace() && original.restoreEpoch == epoch())
        val envelope = JSONObject(original.payload)
        val id = envelope.getString("conversationId")
        val method = envelope.getString("method")
        require(method == "PUT" || method == "DELETE")
        val body = envelope.getJSONObject("body")
        require(body.getString("requestId") == original.requestId)
        val api = transport()
        roleProtected = verifyRole()
        guard(stamp, serial)
        if (!roleProtected || body.optString("roleGeneration").isBlank()) {
            val result =
                try {
                    api.request("/conversations/$id/announcement/operations/${original.requestId}")
                } catch (e: ApiFailure) {
                    guard(stamp, serial)
                    if (
                        roleProtected &&
                            body.optString("roleGeneration").isBlank() &&
                            e.status == 404
                    ) {
                        val terminal =
                            original.copy(
                                payload =
                                    JSONObject(original.payload)
                                        .put("terminalError", "QUALIFICATION_CHANGED")
                                        .toString()
                            )
                        database().putOperation(terminal)
                        guard(stamp, serial)
                        pending = terminal
                        current = null
                        source = null
                    }
                    throw e
                }
            guard(stamp, serial)
            finish(original, result, api, stamp, serial)
            return
        }
        val result =
            try {
                api.request("/conversations/$id/announcement", method, body)
            } catch (e: ApiFailure) {
                guard(stamp, serial)
                if (
                    e.status == 409 &&
                        e.code in
                            listOf(
                                "MEMBERSHIP_GENERATION_CHANGED",
                                "ANNOUNCEMENT_VERSION_CONFLICT",
                                "ANNOUNCEMENT_SOURCE_CHANGED",
                                "SOURCE_VERSION_CONFLICT",
                                "QUALIFICATION_CHANGED",
                            )
                ) {
                    val terminal =
                        original.copy(
                            payload =
                                JSONObject(original.payload).put("terminalError", e.code).toString()
                        )
                    database().putOperation(terminal)
                    guard(stamp, serial)
                    pending = terminal
                    source = null
                    current = null
                }
                throw e
            }
        guard(stamp, serial)
        finish(original, result, api, stamp, serial)
    }

    fun inspect(): Job = work { stamp, serial ->
        val group = group ?: return@work
        val original = database().operation(namespace(), "announcement-${group.id}") ?: return@work
        guard(stamp, serial)
        val api = transport()
        val result =
            api.request("/conversations/${group.id}/announcement/operations/${original.requestId}")
        guard(stamp, serial)
        finish(original, result, api, stamp, serial)
    }

    private suspend fun finish(
        original: LocalOperation,
        result: JSONObject,
        api: Api,
        stamp: String,
        serial: Int,
    ) {
        require(
            result.getString("operationId") == original.requestId &&
                result.getString("status") == "applied"
        )
        val id = JSONObject(original.payload).getString("conversationId")
        // The original result can be historical. Only the fresh GET may supply current body.
        val latest = parse(api.request("/conversations/$id/announcement"), namespace(), epoch())
        val protected = verifyRole()
        guard(stamp, serial)
        database().clearOperation(original.namespace, original.purpose, original.requestId)
        guard(stamp, serial)
        current = latest
        pending = null
        source = null
        roleProtected = protected
    }
}
