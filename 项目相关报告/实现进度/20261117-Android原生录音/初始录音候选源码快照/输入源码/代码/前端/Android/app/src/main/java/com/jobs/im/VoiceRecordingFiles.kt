// Created by Jobs on 2026年11月17日，星期二.
package com.jobs.im

import android.content.Context
import java.io.File
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

data class VoiceRecordingUpload(val lease: VoiceRecordingLease, internal val file: File)

internal object VoiceRecordingFiles {
    private val live = ConcurrentHashMap.newKeySet<String>()
    private val name = Regex("JobsIM-voice-[0-9a-f-]{36}\\.m4a")

    private fun root(context: Context): File {
        val folder = File(context.cacheDir, "VoiceRecordings")
        check((folder.isDirectory || folder.mkdirs()) && !java.nio.file.Files.isSymbolicLink(folder.toPath())) {
            "VOICE_CACHE_UNAVAILABLE"
        }
        return folder.canonicalFile
    }

    private fun owned(context: Context, file: File): Boolean =
        name.matches(file.name) && file.isFile && !java.nio.file.Files.isSymbolicLink(file.toPath()) &&
            file.canonicalFile == file.absoluteFile && file.parentFile?.canonicalFile == root(context)

    fun prune(context: Context) {
        val directory = root(context)
        directory.listFiles()?.forEach { file ->
            if (owned(context, file) && !live.contains(file.absolutePath)) file.delete()
        }
    }

    fun create(context: Context): File {
        prune(context)
        val directory = root(context)
        val file = File(directory, "JobsIM-voice-" + UUID.randomUUID() + ".m4a")
        check(file.createNewFile())
        live.add(file.absolutePath)
        return file
    }

    fun remove(context: Context, file: File?) {
        if (file == null) return
        runCatching { if (owned(context, file)) file.delete() }
        live.remove(file.absolutePath)
    }

    suspend fun read(context: Context, upload: VoiceRecordingUpload): Triple<String, String, ByteArray> =
        withContext(Dispatchers.IO) {
            check(upload.lease.permits(upload.lease.owner) && owned(context, upload.file))
            val bytes = upload.file.inputStream().use { readLimited(it, VoiceRecordingRules.MAX_BYTES) }
            check(bytes.isNotEmpty() && upload.lease.permits(upload.lease.owner) && owned(context, upload.file))
            // Only this module's AAC/MPEG-4 output receives the explicit MIME; picked files keep readAsset's rules.
            Triple(upload.file.name, "audio/mp4", bytes)
        }
}
