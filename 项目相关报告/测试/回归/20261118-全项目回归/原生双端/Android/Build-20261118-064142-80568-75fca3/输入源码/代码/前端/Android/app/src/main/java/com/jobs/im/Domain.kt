package com.jobs.im

import java.time.Instant
import org.json.JSONObject

data class Conversation(
    val id: String,
    val title: String,
    val kind: String = "direct",
    val ownerId: String = "",
    val memberIds: List<String> = emptyList(),
    val readOnly: Boolean = false,
    val membershipGeneration: String = "",
    val sendAdmission: String = "",
    val roleGeneration: String = "",
)

data class Person(
    val id: String,
    val name: String,
    val username: String,
    val avatar: String = "",
    val membershipGeneration: String = "",
    val role: String = "",
    val profileVersion: String = "1",
    val state: String = "active",
    val roleGeneration: String = "",
)

data class ContactRequest(
    val id: String,
    val requesterId: String,
    val name: String,
    val username: String,
)

data class DeviceSession(val id: String, val name: String)

data class Relationship(val blockedByMe: Boolean, val friend: Boolean, val inboundGate: String)

data class ReportRecord(val id: String, val targetId: String, val state: String, val reason: String)

fun globalMessagePlaceholder(row: MessageRow) = row.copy(text = "", attachmentId = "", attachmentURL = "",
    forwardAdmission = "", forwardPayload = "", replyToId = "", reaction = "", pinned = false, favorite = false,
    mentionedUserIds = "[]", mentionAll = false, mentionedMe = false, contentUnavailable = false, retracted = true)

fun conversation(value: JSONObject, fallback: String) =
    Conversation(
        value.getString("id"),
        value.optString("title").ifBlank { fallback },
        value.getString("kind"),
        value.optString("ownerId"),
        value
            .optJSONArray("memberIds")
            ?.let { ids -> (0 until ids.length()).map { ids.getString(it) } }
            .orEmpty(),
        value.optBoolean("readOnly"),
        value.optString("membershipGeneration"),
        value.optString("sendAdmission"),
        value.optString("roleGeneration"),
    )

fun person(value: JSONObject) =
    Person(
        value.getString("id"),
        value.optString("displayName").ifBlank { value.optString("username") },
        value.optString("username"),
        value.optString("avatarThumbnailURL").ifBlank { value.optString("avatarURL") },
        value.optString("membershipGeneration"),
        value.optString("role"),
        value.optString("profileVersion", "1"),
        value.optString("state", "active"),
        value.optString("roleGeneration"),
    )

fun message(value: JSONObject, ns: String, epoch: String): MessageRow {
    val id = value.getString("id")
    val unavailable = value.optBoolean("forwardUnavailable")
    return MessageRow(
        key = "$ns|$id",
        namespace = ns,
        conversationId = value.getString("conversationId"),
        id = id,
        clientMsgId = value.optString("clientMsgId"),
        senderId = value.getString("senderId"),
        type = value.optString("type", "text"),
        text = if (unavailable) "" else value.optString("text"),
        attachmentId =
            if (unavailable) ""
            else value.optString("attachmentId").takeUnless { it == "null" }.orEmpty(),
        replyToId = value.optString("replyToId").takeUnless { it == "null" }.orEmpty(),
        createdAt =
            runCatching { Instant.parse(value.getString("createdAt")).toEpochMilli() }
                .getOrDefault(0),
        pinned = value.optBoolean("pinned"),
        favorite = value.optBoolean("favorite"),
        version = value.optString("version", "1"),
        edited = value.optString("version", "1") != "1",
        retracted = value.optBoolean("deleted"),
        restoreEpoch = epoch,
        sequence = value.optString("sequence", "0"),
        contentUnavailable = value.optBoolean("contentUnavailable"),
        mentionedUserIds = value.optJSONArray("mentionedUserIds")?.toString() ?: "[]",
        mentionAll = value.optBoolean("mentionAll"),
        mentionedMe = value.optBoolean("mentionedMe"),
        contentVersion =
            value.optString("contentVersion").ifBlank { value.optString("version", "1") },
        forwarded = value.optBoolean("forwarded"),
        forwardUnavailable = unavailable,
        forwardAdmission = if (unavailable) "" else value.optString("forwardAdmission"),
        attachmentURL = if (unavailable) "" else value.optString("attachmentURL"),
        reaction =
            value.optJSONArray("reactions")?.takeIf { it.length() > 0 }?.let { "👍" }.orEmpty(),
        projectionGuard = value.optString("_jobsProjectionGuard"),
    )
}
