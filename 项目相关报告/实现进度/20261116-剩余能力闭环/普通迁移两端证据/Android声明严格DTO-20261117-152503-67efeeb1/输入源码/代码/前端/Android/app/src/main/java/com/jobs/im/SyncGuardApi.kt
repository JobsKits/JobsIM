package com.jobs.im

import kotlinx.coroutines.CancellationException
import org.json.JSONObject

data class LocalProjectionGuard(val value: String, val ready: Boolean)

class SyncGuardApi(
    base: String,
    private val delegate: Api,
    private val current: () -> Boolean,
    private val guard: suspend () -> LocalProjectionGuard?,
) : Api(base) {
    private fun lifecycle() {
        if (!current()) throw CancellationException("CONTEXT_CHANGED")
    }

    private fun protectsProjection(path: String): Boolean {
        val normalized = path.removePrefix("/api")
        return normalized.startsWith("/conversations") || normalized.startsWith("/messages") ||
            normalized.startsWith("/search") || normalized.startsWith("/favorites") || normalized.startsWith("/media")
    }

    private suspend fun before(path: String, readOnlyResult: Boolean = false): LocalProjectionGuard? {
        lifecycle()
        val value = guard()
        lifecycle()
        if (protectsProjection(path) && !readOnlyResult && value != null && !value.ready) throw SyncFailure("SYNC_NOT_READY")
        return value
    }

    private suspend fun after(original: LocalProjectionGuard?) {
        lifecycle()
        if (guard() != original) throw CancellationException("SYNC_LOCAL_RESTRICTION_CHANGED")
        lifecycle()
    }

    override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
        lifecycle()
        if (path.startsWith("/sync/") || path.startsWith("/auth/") || path == "/bootstrap") {
            val result = delegate.request(path, method, data)
            lifecycle()
            return result
        }
        val stamp = before(path, method == "POST" && path.endsWith("/message-results"))
        val result = try { delegate.request(path, method, data) } catch (error: Exception) { after(stamp); throw error }
        after(stamp)
        fun tag(value: JSONObject) {
            if (value.has("id") && value.has("conversationId")) value.put("_jobsProjectionGuard", stamp?.value.orEmpty())
        }
        tag(result)
        result.optJSONObject("message")?.let(::tag)
        result.optJSONArray("items")?.let { items -> for (index in 0 until items.length()) items.optJSONObject(index)?.let(::tag) }
        return result
    }

    override suspend fun migration(path:String,method:String,data:JSONObject?,store:String,current:()->Boolean):JSONObject {
        val stamp=before(path);val result=try{delegate.migration(path,method,data,store,current)}catch(error:Exception){after(stamp);throw error};after(stamp);return result
    }
    override suspend fun receipt(path:String,method:String,data:JSONObject?,store:String,current:()->Boolean):JSONObject {
        val stamp=before(path);val result=try{delegate.receipt(path,method,data,store,current)}catch(error:Exception){after(stamp);throw error};after(stamp);return result
    }
    override suspend fun migrationBytes(path:String,store:String,limit:Int,current:()->Boolean):ByteArray {
        val stamp=before(path);val result=try{delegate.migrationBytes(path,store,limit,current)}catch(error:Exception){after(stamp);throw error};try{after(stamp);return result}catch(error:Exception){result.fill(0);throw error}
    }

    override suspend fun upload(name: String, mime: String, bytes: ByteArray, path: String, fields: Map<String, String>): JSONObject {
        val stamp = before(path)
        val result = try { delegate.upload(name, mime, bytes, path, fields) } catch (error: Exception) { after(stamp); throw error }
        after(stamp)
        return result
    }

    override suspend fun download(path: String, limit: Int): Pair<ByteArray, String> {
        val stamp = before(path)
        val result = try { delegate.download(path, limit) } catch (error: Exception) { after(stamp); throw error }
        after(stamp)
        return result
    }

    override suspend fun downloadExport(path: String, downloadToken: String, directory: java.io.File,
        current: () -> Boolean, progress: (Long, Long?) -> Unit): PersonalExportFile {
        val stamp = before(path)
        var file: PersonalExportFile? = null
        try {
            file = delegate.downloadExport(path, downloadToken, directory, { current() && this.current() }, progress)
            after(stamp)
            return file
        } catch (error: Exception) { file?.dispose(); throw error }
    }
}
