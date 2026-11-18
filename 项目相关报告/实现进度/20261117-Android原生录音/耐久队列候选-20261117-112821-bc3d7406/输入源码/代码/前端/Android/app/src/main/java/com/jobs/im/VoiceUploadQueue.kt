// Created by Jobs on 2026年11月17日，星期二.
package com.jobs.im

import android.content.Context
import android.os.StatFs
import android.system.Os
import android.system.OsConstants
import android.util.AtomicFile
import java.io.File
import java.io.FileOutputStream
import java.io.RandomAccessFile
import java.nio.file.Files
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/** Disk-only responsibility. Construction and enumeration never perform HTTP or replay a send. */
internal enum class VoiceUploadCheckpoint { CLIP_SYNCED, MANIFEST_SYNCED, PUBLISHED }

class VoiceUploadQueue private constructor(context: Context, name: String, private val checkpoint: (VoiceUploadCheckpoint) -> Unit) {
    constructor(context: Context) : this(context, "VoiceUploads", {})
    internal constructor(context: Context, testingId: UUID, checkpoint: (VoiceUploadCheckpoint) -> Unit = {}) :
        this(context, "JobsIM-test-VoiceUploads-" + testingId, checkpoint)
    private val parent = context.applicationContext.noBackupFilesDir.canonicalFile
    private val root = File(parent, name)
    private val gate = gates.getOrPut(root.absolutePath) { Mutex() }
    private val files = setOf("clip.m4a", "intent.json", "intent.json.bak", "intent.json.new",
        "owner.json", "owner.json.bak", "owner.json.new")

    companion object { private val gates = ConcurrentHashMap<String, Mutex>() }

    private fun directory(file: File): Boolean = file.isDirectory && !Files.isSymbolicLink(file.toPath()) &&
        file.canonicalFile == file.absoluteFile
    private fun regular(file: File): Boolean = file.isFile && !Files.isSymbolicLink(file.toPath()) &&
        file.canonicalFile == file.absoluteFile

    private fun prepareRoot() {
        val existed = root.exists()
        check((root.isDirectory || root.mkdir()) && directory(root) && root.parentFile == parent) { "VOICE_QUEUE_DIRECTORY_INVALID" }
        if (!existed) { Os.chmod(root.path, 448); sync(parent) }
    }

    private fun sync(file: File) {
        check(directory(file)) { "VOICE_QUEUE_DIRECTORY_INVALID" }
        val descriptor = Os.open(file.absolutePath, OsConstants.O_RDONLY or OsConstants.O_DIRECTORY, 0)
        try { Os.fsync(descriptor) } finally { Os.close(descriptor) }
    }

    private suspend fun <T> locked(block: () -> T): T = withContext(Dispatchers.IO) {
        gate.withLock {
            prepareRoot()
            val lock = File(root, ".queue.lock")
            check(!lock.exists() || regular(lock)) { "VOICE_QUEUE_LOCK_INVALID" }
            RandomAccessFile(lock, "rw").channel.use { channel -> channel.lock().use { block() } }
        }
    }

    private fun location(id: String): File = File(root, VoiceUploadRules.id(id)).also {
        check(it.parentFile == root && (!it.exists() || directory(it))) { "VOICE_QUEUE_DIRECTORY_INVALID" }
    }

    private fun safeFiles(place: File) {
        check(directory(place)) { "VOICE_QUEUE_DIRECTORY_INVALID" }
        check(place.listFiles()?.all { it.name in files && regular(it) } == true) { "VOICE_QUEUE_UNRECOGNIZED_FILES" }
    }

    private fun read(directory: File): VoiceUploadIntent {
        safeFiles(directory)
        val value = readMetadata(directory, "intent.json")
        check(directory.name == value.requestId || directory.name == ".staging-" + value.requestId)
        if (value.phase !in VoiceUploadRules.terminal) {
            val clip = File(directory, "clip.m4a")
            check(regular(clip) && clip.length() == value.size) { "VOICE_QUEUE_CLIP_MISSING" }
        }
        return value
    }

    private fun readMetadata(place: File, name: String): VoiceUploadIntent {
        safeFiles(place)
        val bytes = AtomicFile(File(place, name)).openRead().use { readLimited(it, VoiceUploadRules.MAX_MANIFEST_BYTES) }
        return VoiceUploadWire.decode(bytes).also {
            check(place.name == it.requestId || place.name == ".staging-" + it.requestId)
        }
    }

    private fun write(directory: File, value: VoiceUploadIntent) = writeMetadata(directory, "intent.json", value)

    private fun writeMetadata(directory: File, name: String, value: VoiceUploadIntent) {
        safeFiles(directory)
        val atomic = AtomicFile(File(directory, name))
        val bytes = VoiceUploadWire.encode(value)
        val output = atomic.startWrite()
        try {
            output.write(bytes)
            output.fd.sync()
            atomic.finishWrite(output)
            sync(directory)
            check(AtomicFile(File(directory, name)).openRead().use {
                readLimited(it, VoiceUploadRules.MAX_MANIFEST_BYTES)
            }.contentEquals(bytes)) { "VOICE_QUEUE_MANIFEST_NOT_COMMITTED" }
        } catch (failure: Exception) {
            atomic.failWrite(output)
            throw failure
        }
    }

    private fun partialOwner(place: File): VoiceUploadIntent = readMetadata(place, "owner.json").also {
        check(it.phase == VoiceUploadPhase.PREPARED && it.objectId.isEmpty())
    }

    private fun placeOf(id: String): File {
        val final = location(id)
        if (final.exists()) return final
        return File(root, ".staging-" + VoiceUploadRules.id(id)).also { check(directory(it)) }
    }

    private fun verifyClip(directory: File, value: VoiceUploadIntent): ByteArray {
        val clip = File(directory, "clip.m4a")
        check(regular(clip) && clip.length() == value.size) { "VOICE_QUEUE_CLIP_MISSING" }
        val bytes = clip.inputStream().use { readLimited(it, VoiceRecordingRules.MAX_BYTES) }
        check(bytes.size.toLong() == value.size && VoiceUploadRules.digest(bytes) == value.sha256) { "VOICE_QUEUE_CLIP_CHANGED" }
        return bytes
    }

    private fun recoverStages() {
        for (staged in root.listFiles().orEmpty().filter { it.name.startsWith(".staging-") }) {
            // An incomplete stage may still hold the only clip. Keep it visible for repair; never silently delete it.
            val recovered = runCatching {
                val value = read(staged)
                verifyClip(staged, value)
                value
            }.getOrNull() ?: continue
            val destination = location(recovered.requestId)
            if (!destination.exists()) {
                Os.rename(staged.absolutePath, destination.absolutePath)
                sync(root)
            }
        }
    }

    private fun eraseDirectory(place: File) {
        safeFiles(place)
        for (file in place.listFiles().orEmpty().sortedBy { if (it.name == "intent.json") 1 else 0 })
            check(file.delete()) { "VOICE_QUEUE_CLEANUP_PENDING" }
        check(place.delete()) { "VOICE_QUEUE_CLEANUP_PENDING" }
        sync(root)
    }

    private fun listing(): VoiceUploadListing {
        recoverStages()
        var unknown = 0
        val entries = mutableListOf<VoiceUploadEntry>()
        for (file in root.listFiles().orEmpty().sortedBy { it.name }) {
            if (file.name == ".queue.lock") continue
            val id = file.name.removePrefix(".staging-")
            if (runCatching { VoiceUploadRules.id(id) }.isFailure) { unknown++; continue }
            if (directory(file) && file.listFiles()?.isEmpty() == true) {
                check(file.delete()); sync(root); continue
            }
            val loaded = runCatching { read(file) }
            val attributed = loaded.getOrNull() ?: runCatching { partialOwner(file) }.getOrNull()
            entries.add(VoiceUploadEntry(id, attributed,
                if (loaded.isFailure || file.name.startsWith(".staging-")) "VOICE_QUEUE_RECOVERY_REQUIRED" else ""))
        }
        return VoiceUploadListing(entries, unknown)
    }

    suspend fun list(): VoiceUploadListing = locked { listing() }
    suspend fun get(id: String): VoiceUploadIntent = locked { read(location(id)) }

    /** Called only after explicit Send. The returned task is durable; it is not a sent or accepted message. */
    suspend fun prepare(source: VoiceUploadSource, bytes: ByteArray, durationMillis: Long,
        requestId: String = UUID.randomUUID().toString(), clientMsgId: String = UUID.randomUUID().toString()): VoiceUploadIntent = locked {
        VoiceUploadRules.validate(source)
        require(VoiceRecordingRules.validClip(durationMillis, bytes.size.toLong()))
        val value = VoiceUploadIntent(VoiceUploadRules.id(requestId), VoiceUploadRules.id(clientMsgId), source,
            bytes.size.toLong(), VoiceUploadRules.digest(bytes), durationMillis)
        var all = listing()
        val existing = location(requestId)
        if (existing.exists()) {
            val stored = read(existing)
            check(stored.requestId == value.requestId && stored.clientMsgId == value.clientMsgId && stored.source == value.source &&
                stored.size == value.size && stored.sha256 == value.sha256 && stored.durationMillis == value.durationMillis) {
                "VOICE_QUEUE_ORIGINAL_BODY_CHANGED"
            }
            if (stored.phase !in VoiceUploadRules.terminal) verifyClip(existing, stored)
            sync(existing); sync(root)
            return@locked stored
        }
        check(all.unrecognized == 0 && all.entries.none { it.problem.isNotEmpty() && it.requestId != requestId }) {
            "VOICE_QUEUE_RECOVERY_REQUIRED"
        }
        all = all.copy(entries = all.entries.filter { it.requestId != requestId })
        check(all.entries.none { entry -> entry.intent?.let {
            it.clientMsgId == clientMsgId && it.source.owner.origin == source.owner.origin &&
                it.source.owner.deployment == source.owner.deployment && it.source.owner.user == source.owner.user
        } == true }) { "VOICE_QUEUE_CLIENT_ID_ALREADY_OWNED" }
        if (all.entries.size >= VoiceUploadRules.MAX_RECORDS) {
            val completed = all.entries.filter { entry -> entry.intent!!.phase in VoiceUploadRules.terminal &&
                !File(location(entry.requestId), "clip.m4a").exists() }
                .sortedBy { location(it.requestId).lastModified() }
            for (entry in completed.take(all.entries.size - VoiceUploadRules.MAX_RECORDS + 1)) eraseDirectory(location(entry.requestId))
            all = listing()
        }
        val active = all.entries.count { it.intent!!.phase !in VoiceUploadRules.terminal }
        val retained = all.entries.sumOf { entry ->
            File(location(entry.requestId), "clip.m4a").takeIf(::regular)?.length() ?: 0
        }
        check(VoiceUploadRules.capacity(active, all.entries.size, retained, bytes.size.toLong(), StatFs(root.path).availableBytes)) {
            "VOICE_QUEUE_FULL_OR_LOW_SPACE"
        }
        val id = requestId
        val staged = File(root, ".staging-" + id)
        check((staged.isDirectory || staged.mkdir()) && directory(staged))
        Os.chmod(staged.path, 448)
        sync(root)
        safeFiles(staged)
        val ownerHeader = File(staged, "owner.json")
        if (ownerHeader.exists() || File(staged, "owner.json.bak").exists())
            check(partialOwner(staged) == value) { "VOICE_QUEUE_ORIGINAL_BODY_CHANGED" }
        // Attribution precedes unique bytes so account erasure can safely finish an interrupted copy.
        writeMetadata(staged, "owner.json", value)
        val previousManifest = File(staged, "intent.json")
        if (previousManifest.exists() || File(staged, "intent.json.bak").exists())
            check(read(staged) == value) { "VOICE_QUEUE_ORIGINAL_BODY_CHANGED" }
        val previousClip = File(staged, "clip.m4a")
        if (previousClip.exists()) {
            val previous = previousClip.inputStream().use { readLimited(it, VoiceRecordingRules.MAX_BYTES) }
            check(previous.size <= bytes.size && bytes.copyOfRange(0, previous.size).contentEquals(previous)) {
                "VOICE_QUEUE_ORIGINAL_BODY_CHANGED"
            }
        }
        // On failure retain the stage and the caller's cache source. No network permission is returned.
        FileOutputStream(File(staged, "clip.m4a")).use { output -> output.write(bytes); output.fd.sync() }
        sync(staged)
        checkpoint(VoiceUploadCheckpoint.CLIP_SYNCED)
        write(staged, value)
        checkpoint(VoiceUploadCheckpoint.MANIFEST_SYNCED)
        Os.rename(staged.absolutePath, location(id).absolutePath)
        sync(root)
        checkpoint(VoiceUploadCheckpoint.PUBLISHED)
        check(read(location(id)) == value)
        value
    }

    suspend fun bytesForUpload(id: String, current: VoiceUploadOwner): ByteArray = locked {
        val value = read(location(id))
        check(VoiceUploadRules.mayWrite(value, current) && value.phase == VoiceUploadPhase.UPLOAD_UNKNOWN) { "VOICE_QUEUE_OWNER_CHANGED" }
        verifyClip(location(id), value)
    }

    private suspend fun update(id: String, change: (VoiceUploadIntent) -> VoiceUploadIntent): VoiceUploadIntent = locked {
        val place = location(id)
        val value = change(read(place))
        write(place, value)
        value
    }

    suspend fun uploadStarted(id: String, current: VoiceUploadOwner, explicitRetry: Boolean): VoiceUploadIntent = update(id) {
        check(VoiceUploadRules.mayWrite(it, current) && it.phase in setOf(VoiceUploadPhase.PREPARED, VoiceUploadPhase.UPLOAD_UNKNOWN))
        check(explicitRetry) { "VOICE_QUEUE_EXPLICIT_ACTION_REQUIRED" }
        it.copy(phase = VoiceUploadPhase.UPLOAD_UNKNOWN)
    }

    suspend fun objectPrepared(id: String, objectId: String): VoiceUploadIntent = update(id) {
        VoiceUploadRules.id(objectId)
        check(it.phase in setOf(VoiceUploadPhase.PREPARED, VoiceUploadPhase.UPLOAD_UNKNOWN) &&
            (it.objectId.isEmpty() || it.objectId == objectId))
        it.copy(phase = VoiceUploadPhase.UPLOAD_UNKNOWN, objectId = objectId)
    }

    suspend fun objectConfirmed(id: String, objectId: String, size: Long, sha256: String): VoiceUploadIntent = update(id) {
        VoiceUploadRules.id(objectId)
        check(it.phase in setOf(VoiceUploadPhase.PREPARED, VoiceUploadPhase.UPLOAD_UNKNOWN, VoiceUploadPhase.OBJECT_CONFIRMED) &&
            it.size == size && it.sha256 == sha256 && (it.objectId.isEmpty() || it.objectId == objectId))
        it.copy(phase = VoiceUploadPhase.OBJECT_CONFIRMED, objectId = objectId)
    }

    suspend fun enqueueStarted(id: String, current: VoiceUploadOwner): VoiceUploadIntent = update(id) {
        check(VoiceUploadRules.mayWrite(it, current) && it.phase in setOf(VoiceUploadPhase.OBJECT_CONFIRMED,
            VoiceUploadPhase.ENQUEUE_UNKNOWN, VoiceUploadPhase.QUEUED))
        it.copy(phase = VoiceUploadPhase.ENQUEUE_UNKNOWN)
    }

    suspend fun queued(id: String, row: MessageRow): VoiceUploadIntent = update(id) {
        check(it.phase in setOf(VoiceUploadPhase.ENQUEUE_UNKNOWN, VoiceUploadPhase.QUEUED) && VoiceUploadRules.sameMessage(it, row))
        it.copy(phase = VoiceUploadPhase.QUEUED)
    }

    suspend fun freeze(id: String, reason: String = "OWNER_CHANGED"): VoiceUploadIntent = update(id) {
        it.copy(frozen = true, freezeReason = reason)
    }

    suspend fun freezeAccount(profile: String, deployment: String, user: String) = locked {
        val all = listing()
        check(all.unrecognized == 0 && all.entries.all { it.intent != null }) { "VOICE_QUEUE_RECOVERY_REQUIRED" }
        for (entry in all.entries) {
            val value = entry.intent!!
            val owner = value.source.owner
            if (owner.profile == profile && owner.deployment == deployment && owner.user == user &&
                value.phase !in VoiceUploadRules.terminal && !value.frozen)
                if (entry.problem.isEmpty()) write(location(value.requestId), value.copy(frozen = true, freezeReason = "OWNER_CHANGED"))
                else writeMetadata(placeOf(value.requestId), "owner.json", value.copy(frozen = true, freezeReason = "OWNER_CHANGED"))
        }
    }

    suspend fun cancelUnsubmitted(id: String, current: VoiceUploadOwner): VoiceUploadIntent = locked {
        val place = placeOf(id)
        val value = runCatching { read(place) }.getOrElse { partialOwner(place) }
        check(!value.frozen && value.source.owner == current &&
            value.phase in setOf(VoiceUploadPhase.PREPARED, VoiceUploadPhase.CANCELLED))
        val cancelled = value.copy(phase = VoiceUploadPhase.CANCELLED)
        write(place, cancelled)
        removeClip(place)
        if (place.name.startsWith(".staging-")) { Os.rename(place.path, location(id).path); sync(root) }
        cancelled
    }

    private fun removeClip(place: File) {
        val clip = File(place, "clip.m4a")
        check(!clip.exists() || regular(clip))
        check(!clip.exists() || clip.delete()) { "VOICE_QUEUE_CLEANUP_PENDING" }
        sync(place)
    }

    /** Receipt must be established by original-message reconciliation outside this disk queue. */
    suspend fun complete(id: String, accepted: MessageRow): VoiceUploadIntent = locked {
        val place = location(id)
        val value = read(place)
        check(value.phase in setOf(VoiceUploadPhase.OBJECT_CONFIRMED, VoiceUploadPhase.ENQUEUE_UNKNOWN,
            VoiceUploadPhase.QUEUED, VoiceUploadPhase.COMPLETED) &&
            VoiceUploadRules.sameAcceptedContent(value, accepted) && accepted.state == "received")
        val completed = value.copy(phase = VoiceUploadPhase.COMPLETED)
        write(place, completed)
        removeClip(place)
        completed
    }

    /** Hook for authoritative account erasure; a corrupt/unattributable record keeps cleanup pending. */
    suspend fun eraseAccount(profile: String, deployment: String, user: String) = locked {
        val all = listing()
        check(all.unrecognized == 0 && all.entries.all { it.intent != null }) { "VOICE_QUEUE_CLEANUP_PENDING" }
        for (entry in all.entries) {
            val value = entry.intent!!
            val owner = value.source.owner
            if (owner.profile == profile && owner.deployment == deployment && owner.user == user) {
                val place = placeOf(entry.requestId)
                safeFiles(place)
                write(place, value.copy(phase = VoiceUploadPhase.CANCELLED, frozen = true, freezeReason = "ACCOUNT_ERASED"))
                // Keep the attribution manifest until all other files are gone; an empty task directory is safely recoverable.
                eraseDirectory(place)
            }
        }
    }

    /** The caller supplies verified global cleanup facts and their exact original client-message identities. */
    suspend fun eraseMessages(scope: String, serverId: String, originals: List<MessageRow>) = locked {
        check(originals.all { it.id == serverId })
        val all = listing()
        check(all.unrecognized == 0 && all.entries.none { it.problem.isNotEmpty() }) { "VOICE_QUEUE_CLEANUP_PENDING" }
        for (entry in all.entries) {
            val value = entry.intent!!
            val owner = value.source.owner
            val ownScope = "live|" + owner.profile + "|" + owner.deployment + "|" + owner.user + "|personal"
            val original = originals.any { it.senderId == owner.user && it.clientMsgId == value.clientMsgId &&
                it.namespace.startsWith("live|" + owner.profile + "|" + owner.deployment + "|" + owner.user + "|") }
            if (ownScope == scope && original) {
                val place = location(entry.requestId)
                write(place, value.copy(phase = VoiceUploadPhase.CANCELLED, frozen = true, freezeReason = "SOURCE_INVALID"))
                removeClip(place)
            }
        }
    }
}
