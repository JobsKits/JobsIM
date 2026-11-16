package com.jobs.im

import java.net.URI
import org.json.JSONObject

object Forwarding {
    val permanentErrors =
        setOf(
            "NOT_A_MEMBER",
            "HISTORY_NOT_ALLOWED",
            "FORWARD_ADMISSION_CHANGED",
            "FORWARD_SOURCE_CHANGED",
            "FORWARD_SOURCE_UNAVAILABLE",
            "INVALID_FORWARD",
            "FORWARD_PROTOCOL_REQUIRED",
            "FORWARD_DEPTH_LIMIT",
            "SEND_ADMISSION_CHANGED",
            "QUALIFICATION_CHANGED",
            "FEATURE_DISABLED",
        )

    fun payload(row: MessageRow): JSONObject {
        require(row.type == "forward" && row.forwardPayload.isNotBlank()) {
            "FORWARD_PROTOCOL_REQUIRED"
        }
        val body = JSONObject(row.forwardPayload)
        require(
            body.keys().asSequence().toSet() ==
                setOf(
                    "clientMsgId",
                    "type",
                    "forwardOfId",
                    "forwardSourceVersion",
                    "forwardAdmission",
                    "restoreEpoch",
                    "sendAdmission",
                )
        ) {
            "INVALID_FORWARD"
        }
        require(
            body.getString("type") == "forward" &&
                body.getString("clientMsgId") == row.clientMsgId &&
                body.getString("restoreEpoch") == row.restoreEpoch &&
                body.getString("sendAdmission") == row.sendAdmission &&
                body.getString("forwardAdmission").isNotBlank() &&
                body.getString("forwardOfId").isNotBlank() &&
                body.getString("forwardSourceVersion").isNotBlank()
        ) {
            "INVALID_FORWARD"
        }
        return body
    }

    fun hidden(row: MessageRow) =
        row.copy(
            text = "",
            attachmentId = "",
            attachmentURL = "",
            forwardAdmission = "",
            forwardUnavailable = true,
            mentionedUserIds = "[]",
            mentionAll = false,
            mentionedMe = false,
            replyToId = "",
        )

    fun attachmentPath(base: String, id: String, value: String): String {
        require(id.matches(Regex("[A-Za-z0-9-]+")))
        val expected = "/api/messages/$id/attachment"
        val uri = URI(value)
        require(uri.rawQuery == null && uri.rawFragment == null && uri.rawUserInfo == null)
        if (uri.isAbsolute) {
            val origin = URI(base)
            require(uri.scheme == "https" && uri.host == origin.host && uri.port == origin.port)
        } else require(uri.rawAuthority == null && value.startsWith('/'))
        require(uri.rawPath == expected) { "FORWARD_ATTACHMENT_INVALID" }
        return expected
    }
}
