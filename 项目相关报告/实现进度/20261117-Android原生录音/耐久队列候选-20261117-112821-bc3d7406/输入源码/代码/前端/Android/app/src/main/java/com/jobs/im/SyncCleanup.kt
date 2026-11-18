package com.jobs.im

import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

class SyncMediaCleanup(private val database: ChatDatabase, private val cache: File,
    private val voiceUploads: VoiceUploadQueue? = null) {
    companion object {
        fun sidecar(cache: File, namespace: String, id: String) = File(cache,
            "Attachments/" + Isolation.digest(namespace.removeSuffix("|sync-retained")) + "/" + Isolation.digest(id) + ".bin")

        fun remove(cache: File, namespace: String, id: String): Boolean {
            val file = sidecar(cache, namespace, id)
            return !file.exists() || file.delete()
        }
    }

    suspend fun drain(plans: List<SyncCleanupPlan>) {
        val store = SyncStore(database)
        for ((scope, group) in plans.groupBy { it.scope }) {
            val prefix = scope.removeSuffix("personal")
            val forwards = database.messages().forwardHolders(prefix)
            val forwardClean = withContext(Dispatchers.IO) {
                forwards.map { remove(cache, it.namespace, it.id) }.all { it }
            }
            for (plan in group) {
                val holders = database.messages().across(prefix, plan.id)
                val removed = withContext(Dispatchers.IO) {
                    holders.map { remove(cache, it.namespace, it.id) }.all { it }
                }
                voiceUploads?.eraseMessages(plan.scope, plan.id, holders)
                if (removed && forwardClean) store.completeCleanup(plan)
            }
        }
    }
}
