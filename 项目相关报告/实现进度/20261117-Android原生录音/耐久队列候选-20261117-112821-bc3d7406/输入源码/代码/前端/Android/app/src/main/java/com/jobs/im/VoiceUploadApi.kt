// Created by Jobs on 2026年11月17日，星期二.
package com.jobs.im

import org.json.JSONObject

sealed interface VoiceUploadResult {
    data object Missing : VoiceUploadResult
    data object Retired : VoiceUploadResult
    data class Prepared(val objectId: String) : VoiceUploadResult
    data class Committed(val objectId: String, val size: Long, val sha256: String) : VoiceUploadResult
}

/** Uses a captured original-family transport. The current owner is checked at every network boundary. */
class VoiceUploadApi(private val transport: Api, private val current: () -> VoiceUploadOwner?,
    private val maySubmit: (VoiceUploadIntent) -> Boolean) {
    private fun guard(intent: VoiceUploadIntent) {
        check(current() == intent.source.owner) { "VOICE_QUEUE_OWNER_CHANGED" }
    }

    suspend fun requireCapability(intent: VoiceUploadIntent) {
        guard(intent)
        val result = transport.request("/media/capabilities")
        guard(intent)
        check(result.optString("protocol") == "stable-upload-v1" && result.opt("supported") == true &&
            result.optString("resultPath") == "/api/media/upload-results" &&
            result.opt("maxBytes") is String &&
            (result.getString("maxBytes").toLongOrNull() ?: 0) >= intent.size) { "VOICE_STABLE_UPLOAD_UNAVAILABLE" }
    }

    private fun fields(intent: VoiceUploadIntent): Map<String, String> = linkedMapOf(
        "requestId" to intent.requestId, "userId" to intent.source.owner.user,
        "deviceId" to intent.source.owner.device, "familyId" to intent.source.owner.family,
        "restoreEpoch" to intent.source.owner.epoch, "name" to intent.name,
        "size" to intent.size.toString(), "sha256" to intent.sha256,
    )

    suspend fun lookup(intent: VoiceUploadIntent): VoiceUploadResult {
        guard(intent)
        val result = try {
            transport.request("/media/upload-results", "POST", JSONObject(fields(intent)))
        } catch (failure: ApiFailure) {
            guard(intent)
            if (failure.status == 404 && failure.code == "UPLOAD_NOT_FOUND") return VoiceUploadResult.Missing
            if (failure.status == 410 && failure.code == "UPLOAD_RETIRED") return VoiceUploadResult.Retired
            throw failure
        }
        guard(intent)
        return parse(intent, result)
    }

    suspend fun uploadOriginal(intent: VoiceUploadIntent, bytes: ByteArray): VoiceUploadResult {
        guard(intent)
        check(intent.phase == VoiceUploadPhase.UPLOAD_UNKNOWN && !intent.frozen &&
            maySubmit(intent) && bytes.size.toLong() == intent.size && VoiceUploadRules.digest(bytes) == intent.sha256)
        val result = transport.upload(intent.name, "audio/mp4", bytes, fields = fields(intent))
        guard(intent)
        return parse(intent, result)
    }

    private fun parse(intent: VoiceUploadIntent, result: JSONObject): VoiceUploadResult {
        check(result.optString("protocol") == "stable-upload-v1" && result.optString("requestId") == intent.requestId)
        val objectId = VoiceUploadRules.id(result.getString("id"))
        check(intent.objectId.isEmpty() || intent.objectId == objectId) { "VOICE_UPLOAD_RESULT_CHANGED" }
        return when (result.getString("uploadState")) {
            "prepared" -> {
                check(!result.has("url")) { "VOICE_UPLOAD_RESULT_INVALID" }
                VoiceUploadResult.Prepared(objectId)
            }
            "committed" -> {
                val rawSize = result.get("size")
                check(rawSize is Int || rawSize is Long)
                val size = result.getLong("size")
                check(result.getString("name") == intent.name && result.getString("contentType") == "audio/mp4" &&
                    size == intent.size && result.getString("sha256") == intent.sha256 &&
                    result.getString("url") == "/api/media/" + objectId) { "VOICE_UPLOAD_RESULT_INVALID" }
                VoiceUploadResult.Committed(objectId, size, intent.sha256)
            }
            else -> error("VOICE_UPLOAD_RESULT_INVALID")
        }
    }
}
