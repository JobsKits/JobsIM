// Created by Jobs on 2026年11月17日，星期二.
package com.jobs.im

/** Only an explicit UI action calls act; loading the queue does not perform network requests. */
class VoiceUploadCoordinator(
    private val queue: VoiceUploadQueue,
    private val current: () -> VoiceUploadOwner?,
    private val maySubmit: (VoiceUploadIntent) -> Boolean,
    private val makeApi: () -> Api,
    private val local: suspend (VoiceUploadIntent) -> MessageRow?,
    private val enqueue: suspend (VoiceUploadIntent) -> MessageRow,
    private val accept: suspend (VoiceUploadIntent, MessageRow) -> Unit,
    private val submit: suspend (MessageRow) -> Unit,
) {
    private fun guard(intent: VoiceUploadIntent) {
        check(intent.source.owner == current() && !intent.frozen) { "VOICE_QUEUE_OWNER_CHANGED" }
    }

    suspend fun act(id: String, retryOriginal: Boolean): VoiceUploadIntent {
        var intent = queue.get(id)
        guard(intent)
        if (intent.phase in VoiceUploadRules.terminal) return intent
        val api = makeApi()
        val media = VoiceUploadApi(api, current, maySubmit) { queue.assertNetworkAllowed(it.requestId, it.source.owner) }
        if (intent.phase in setOf(VoiceUploadPhase.PREPARED, VoiceUploadPhase.UPLOAD_UNKNOWN)) {
            val result = media.lookup(intent)
            guard(intent)
            intent = observe(intent, result)
            if (result is VoiceUploadResult.Retired || intent.frozen) return intent
            if (result is VoiceUploadResult.Missing || result is VoiceUploadResult.Prepared) {
                if (!retryOriginal) return intent
                check(maySubmit(intent)) { "VOICE_QUEUE_ORIGINAL_CHAT_REQUIRED" }
                media.requireCapability(intent)
                guard(intent)
                intent = queue.uploadStarted(id, intent.source.owner, true)
                val bytes = queue.bytesForUpload(id, intent.source.owner)
                guard(intent)
                intent = observe(intent, media.uploadOriginal(intent, bytes))
                if (intent.phase != VoiceUploadPhase.OBJECT_CONFIRMED) return intent
            }
        }
        if (intent.phase in setOf(VoiceUploadPhase.OBJECT_CONFIRMED, VoiceUploadPhase.ENQUEUE_UNKNOWN, VoiceUploadPhase.QUEUED)) {
            if (reconcileMessage(intent, api)) return queue.get(id)
            if (!retryOriginal) return intent
            guard(intent)
            check(maySubmit(intent)) { "VOICE_QUEUE_ORIGINAL_CHAT_REQUIRED" }
            intent = queue.enqueueStarted(id, intent.source.owner)
            val existing = local(intent)
            guard(intent)
            val row = existing ?: enqueue(intent)
            guard(intent)
            check(VoiceUploadRules.sameMessage(intent, row)) { "VOICE_QUEUE_OUTBOX_CHANGED" }
            queue.queued(id, row)
            check(maySubmit(intent)) { "VOICE_QUEUE_ORIGINAL_CHAT_REQUIRED" }
            queue.assertNetworkAllowed(id, intent.source.owner)
            guard(intent)
            submit(row)
            // An unknown response leaves the original queue/Outbox intact. Never mint another clientMsgId.
            reconcileMessage(intent, api)
        }
        return queue.get(id)
    }

    private suspend fun observe(intent: VoiceUploadIntent, result: VoiceUploadResult): VoiceUploadIntent = when (result) {
        VoiceUploadResult.Missing -> intent
        VoiceUploadResult.Retired -> queue.freeze(intent.requestId, "SOURCE_INVALID")
        is VoiceUploadResult.Prepared -> queue.objectPrepared(intent.requestId, result.objectId)
        is VoiceUploadResult.Committed -> queue.objectConfirmed(intent.requestId, result.objectId, result.size, result.sha256)
    }

    private suspend fun reconcileMessage(intent: VoiceUploadIntent, api: Api): Boolean {
        guard(intent)
        val original = intent.row()
        queue.assertNetworkAllowed(intent.requestId, intent.source.owner)
        guard(intent)
        val response = try {
            api.request("/conversations/" + intent.source.conversation + "/message-results", "POST", originalMessageBody(original))
        } catch (failure: ApiFailure) {
            guard(intent)
            if (failure.status == 404 && failure.code == "MESSAGE_RESULT_NOT_FOUND") return false
            throw failure
        }
        guard(intent)
        val committed = response.optJSONObject("message") ?: response
        SyncProtocol.requireCommittedMessage(committed, intent.clientMsgId, intent.source.conversation, intent.source.owner.user)
        val accepted = message(committed, intent.source.owner.namespace, intent.source.owner.epoch)
        check(VoiceUploadRules.sameAcceptedContent(intent, accepted)) { "VOICE_QUEUE_MESSAGE_RESULT_CHANGED" }
        queue.assertNetworkAllowed(intent.requestId, intent.source.owner)
        guard(intent)
        accept(intent, accepted)
        guard(intent)
        queue.complete(intent.requestId, accepted)
        return true
    }
}
