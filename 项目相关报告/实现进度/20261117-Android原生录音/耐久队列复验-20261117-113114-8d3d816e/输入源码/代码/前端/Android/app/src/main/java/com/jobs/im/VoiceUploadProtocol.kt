// Created by Jobs on 2026年11月17日，星期二.
package com.jobs.im

import java.net.URI
import java.security.MessageDigest
import java.util.UUID
import org.json.JSONObject

data class VoiceUploadOwner(
    val namespace: String,
    val profile: String,
    val origin: String,
    val deployment: String,
    val user: String,
    val device: String,
    val family: String,
    val epoch: String,
)

data class VoiceUploadSource(
    val owner: VoiceUploadOwner,
    val conversation: String,
    val admission: String,
    val membership: String,
    val replyTo: String = "",
    val conversationTitle: String = "",
)

enum class VoiceUploadPhase { PREPARED, UPLOAD_UNKNOWN, OBJECT_CONFIRMED, ENQUEUE_UNKNOWN, QUEUED, COMPLETED, CANCELLED }

data class VoiceUploadIntent(
    val requestId: String,
    val clientMsgId: String,
    val source: VoiceUploadSource,
    val size: Long,
    val sha256: String,
    val durationMillis: Long,
    val phase: VoiceUploadPhase = VoiceUploadPhase.PREPARED,
    val objectId: String = "",
    val frozen: Boolean = false,
    val freezeReason: String = "",
) {
    val name get() = "voice-" + requestId + ".m4a"
    val route get() = "/conversations/" + source.conversation + "/messages"
    fun row(): MessageRow {
        check(objectId.isNotEmpty() && phase in setOf(VoiceUploadPhase.OBJECT_CONFIRMED,
            VoiceUploadPhase.ENQUEUE_UNKNOWN, VoiceUploadPhase.QUEUED, VoiceUploadPhase.COMPLETED))
        return MessageRow(
            key = source.owner.namespace + "|" + clientMsgId, namespace = source.owner.namespace,
            conversationId = source.conversation, id = clientMsgId, clientMsgId = clientMsgId,
            senderId = source.owner.user, type = "audio", text = name, attachmentId = objectId,
            replyToId = source.replyTo, state = "pending", restoreEpoch = source.owner.epoch,
            membershipGeneration = source.membership, sendAdmission = source.admission,
        )
    }
}

data class VoiceUploadEntry(val requestId: String, val intent: VoiceUploadIntent?, val problem: String = "")
data class VoiceUploadListing(val entries: List<VoiceUploadEntry>, val unrecognized: Int)

object VoiceUploadRules {
    const val MAX_TASKS = 32
    const val MAX_RECORDS = 128
    const val MAX_TOTAL_BYTES = 64L * 1024 * 1024
    const val SPACE_RESERVE = 16L * 1024 * 1024
    const val MAX_MANIFEST_BYTES = 16 * 1024
    val terminal = setOf(VoiceUploadPhase.COMPLETED, VoiceUploadPhase.CANCELLED)

    fun id(value: String): String {
        require(UUID.fromString(value).toString() == value) { "VOICE_QUEUE_INVALID_ID" }
        return value
    }
    fun digest(bytes: ByteArray): String = MessageDigest.getInstance("SHA-256").digest(bytes)
        .joinToString("") { "%02x".format(it.toInt() and 255) }

    fun validate(source: VoiceUploadSource) {
        val owner = source.owner
        for (value in listOf(owner.namespace, owner.profile, source.admission))
            require(value.isNotBlank() && value.length <= 4096 && value.none(Char::isISOControl))
        require(listOf(owner.profile, owner.deployment, owner.user, owner.device, owner.epoch).none { '|' in it })
        require(owner.namespace == Isolation.namespace(owner.profile, owner.deployment, owner.user, false,
            owner.epoch, "personal", owner.device)) { "VOICE_QUEUE_NAMESPACE_MISMATCH" }
        val uri = URI(owner.origin)
        require(uri.scheme == "https" && !uri.host.isNullOrBlank() && uri.userInfo == null &&
            uri.rawQuery == null && uri.rawFragment == null && Isolation.endpoint(owner.origin) == owner.origin)
        listOf(owner.deployment, owner.user, owner.device, owner.family, source.conversation).forEach(::id)
        require(owner.epoch.matches(Regex("[1-9][0-9]{0,18}")) && owner.epoch.toLongOrNull() != null)
        require(source.membership.isEmpty() || (source.membership.matches(Regex("0|[1-9][0-9]{0,18}")) &&
            source.membership.toLongOrNull() != null))
        if (source.replyTo.isNotEmpty()) id(source.replyTo)
        require(source.conversationTitle.length <= 512 && source.conversationTitle.none(Char::isISOControl))
    }

    fun validate(intent: VoiceUploadIntent) {
        id(intent.requestId); id(intent.clientMsgId); validate(intent.source)
        require(VoiceRecordingRules.validClip(intent.durationMillis, intent.size))
        require(intent.sha256.matches(Regex("[0-9a-f]{64}")))
        if (intent.objectId.isNotEmpty()) id(intent.objectId)
        if (intent.phase in setOf(VoiceUploadPhase.OBJECT_CONFIRMED, VoiceUploadPhase.ENQUEUE_UNKNOWN,
                VoiceUploadPhase.QUEUED, VoiceUploadPhase.COMPLETED)) require(intent.objectId.isNotEmpty())
        require(intent.freezeReason in setOf("", "OWNER_CHANGED", "ACCOUNT_ERASED", "SOURCE_INVALID"))
    }

    fun mayWrite(intent: VoiceUploadIntent, current: VoiceUploadOwner?): Boolean =
        !intent.frozen && intent.phase !in terminal && current == intent.source.owner

    fun capacity(active: Int, records: Int, retainedBytes: Long, incoming: Long, available: Long): Boolean =
        active < MAX_TASKS && records < MAX_RECORDS && incoming in 1..VoiceRecordingRules.MAX_BYTES.toLong() &&
            retainedBytes >= 0 && retainedBytes <= MAX_TOTAL_BYTES - incoming &&
            available >= SPACE_RESERVE + incoming * 2

    fun sameMessage(intent: VoiceUploadIntent, row: MessageRow): Boolean =
        sameAcceptedContent(intent, row) && row.sendAdmission == intent.source.admission &&
            row.membershipGeneration == intent.source.membership && row.id == intent.clientMsgId &&
            row.state in setOf("pending", "frozen")

    fun sameAcceptedContent(intent: VoiceUploadIntent, row: MessageRow): Boolean =
        row.namespace == intent.source.owner.namespace && row.clientMsgId == intent.clientMsgId &&
            row.senderId == intent.source.owner.user && row.conversationId == intent.source.conversation &&
            row.type == "audio" && row.text == intent.name && row.attachmentId == intent.objectId &&
            row.replyToId == intent.source.replyTo && row.restoreEpoch == intent.source.owner.epoch &&
            row.mentionedUserIds == "[]" && !row.mentionAll && !row.forwarded
}

internal object VoiceUploadWire {
    fun encode(value: VoiceUploadIntent): ByteArray {
        VoiceUploadRules.validate(value)
        val owner = value.source.owner
        return JSONObject().put("schema", "1").put("requestId", value.requestId).put("clientMsgId", value.clientMsgId)
            .put("namespace", owner.namespace).put("profile", owner.profile).put("origin", owner.origin)
            .put("deployment", owner.deployment).put("user", owner.user).put("device", owner.device)
            .put("family", owner.family).put("epoch", owner.epoch).put("conversation", value.source.conversation)
            .put("admission", value.source.admission).put("membership", value.source.membership)
            .put("replyTo", value.source.replyTo).put("size", value.size.toString()).put("sha256", value.sha256)
            .put("conversationTitle", value.source.conversationTitle)
            .put("durationMillis", value.durationMillis.toString()).put("phase", value.phase.name)
            .put("objectId", value.objectId).put("frozen", value.frozen).put("freezeReason", value.freezeReason)
            .toString().toByteArray(Charsets.UTF_8).also { require(it.size <= VoiceUploadRules.MAX_MANIFEST_BYTES) }
    }

    fun decode(bytes: ByteArray): VoiceUploadIntent {
        require(bytes.size <= VoiceUploadRules.MAX_MANIFEST_BYTES)
        val json = JSONObject(Charsets.UTF_8.newDecoder().decode(java.nio.ByteBuffer.wrap(bytes)).toString())
        val keys = setOf("schema", "requestId", "clientMsgId", "namespace", "profile", "origin", "deployment", "user",
            "device", "family", "epoch", "conversation", "admission", "membership", "replyTo", "size", "sha256",
            "durationMillis", "phase", "objectId", "frozen", "freezeReason", "conversationTitle")
        require(json.keys().asSequence().toSet() == keys && json.getString("schema") == "1")
        fun text(key: String): String { require(json.get(key) is String); return json.getString(key) }
        val owner = VoiceUploadOwner(text("namespace"), text("profile"), text("origin"), text("deployment"),
            text("user"), text("device"), text("family"), text("epoch"))
        require(json.get("frozen") is Boolean)
        return VoiceUploadIntent(text("requestId"), text("clientMsgId"),
            VoiceUploadSource(owner, text("conversation"), text("admission"), text("membership"), text("replyTo"), text("conversationTitle")),
            text("size").toLong(), text("sha256"), text("durationMillis").toLong(), VoiceUploadPhase.valueOf(text("phase")),
            text("objectId"), json.getBoolean("frozen"), text("freezeReason")).also(VoiceUploadRules::validate)
    }
}
