package com.jobs.im

import android.system.Os
import android.system.OsConstants
import java.io.File
import java.io.FileOutputStream
import java.io.RandomAccessFile
import java.net.URLDecoder
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.nio.file.Files
import java.nio.file.StandardCopyOption
import java.security.MessageDigest
import java.util.UUID
import javax.net.ssl.HttpsURLConnection
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlin.coroutines.resumeWithException

data class PersonalExportFile(val file: File, val fileName: String, val size: Long, val sha256: String) {
    fun dispose() {
        runCatching { if (file.exists() && file.delete()) PersonalExportFiles.syncDirectory(file.parentFile!!) }
        PersonalExportFiles.release(file)
    }
}

object PersonalExportFiles {
    const val limit = 1024L * 1024 * 1024
    const val directoryLimit = 32L * 1024 * 1024
    private val active = java.util.concurrent.ConcurrentHashMap.newKeySet<String>()

    internal fun retain(file: File) { active.add(file.absolutePath) }
    internal fun release(file: File) { active.remove(file.absolutePath) }

    fun clearOwnedOrphans(root: File) {
        val base = root.canonicalFile
        if (!base.exists()) return
        check(base.isDirectory) { "EXPORT_STORAGE_UNAVAILABLE" }
        base.listFiles()?.filter { it.isDirectory && it.name.matches(Regex("[a-f0-9]{64}")) && it.canonicalFile == it.absoluteFile }
            ?.forEach(::clearOrphans)
    }

    fun clearOrphans(directory: File) {
        directory.listFiles()?.filter { it.isFile && it.canonicalFile == it.absoluteFile &&
            it.name.matches(Regex("[a-f0-9-]{36}\\.(?:part|zip)")) && it.absolutePath !in active }?.forEach {
            check(it.delete()) { "EXPORT_STORAGE_UNAVAILABLE" }
        }
        syncDirectory(directory)
    }

    fun filename(header: String?): String {
        if (header == null || !Regex("^attachment(?:\\s*;|$)", RegexOption.IGNORE_CASE).containsMatchIn(header)) PersonalExportWire.fail("EXPORT_FILE_INVALID")
        val names = mutableMapOf<String, String>()
        val pattern = Regex(""";\s*(filename\*|filename)\s*=\s*("(?:[^"\\]|\\.)*"|[^;]*)""", RegexOption.IGNORE_CASE)
        for (match in pattern.findAll(header)) {
            val key = match.groupValues[1].lowercase()
            if (key in names) PersonalExportWire.fail("EXPORT_FILE_INVALID")
            var value = match.groupValues[2].trim()
            if (value.startsWith('"')) {
                if (value.length < 2 || !value.endsWith('"')) PersonalExportWire.fail("EXPORT_FILE_INVALID")
                value = value.drop(1).dropLast(1).replace(Regex("""\\(.)"""), "$1")
            }
            names[key] = value
        }
        var name = names["filename*"] ?: names["filename"] ?: PersonalExportWire.fail("EXPORT_FILE_INVALID")
        if ("filename*" in names) {
            if (!name.startsWith("UTF-8''", ignoreCase = true)) PersonalExportWire.fail("EXPORT_FILE_INVALID")
            name = try { URLDecoder.decode(name.drop(7).replace("+", "%2B"), "UTF-8") }
                catch (_: Exception) { PersonalExportWire.fail("EXPORT_FILE_INVALID") }
        }
        if (name.isBlank() || name.length > 180 || name != name.trim() || Regex("""[\p{Cc}\p{Cf}/\\<>:"|?*]""").containsMatchIn(name) || !name.endsWith(".zip", ignoreCase = true)) PersonalExportWire.fail("EXPORT_FILE_INVALID")
        return name
    }

    fun syncDirectory(directory: File) {
        check(directory.isDirectory) { "EXPORT_STORAGE_UNAVAILABLE" }
        val descriptor = Os.open(directory.canonicalPath, OsConstants.O_RDONLY, 0)
        try { Os.fsync(descriptor) } finally { Os.close(descriptor) }
    }

    fun directory(root: File, namespace: String): File {
        val base = root.canonicalFile
        if (!base.exists()) { check(base.mkdir()) { "EXPORT_STORAGE_UNAVAILABLE" }; syncDirectory(base.parentFile!!) }
        check(base.isDirectory) { "EXPORT_STORAGE_UNAVAILABLE" }
        val directory = File(base, Isolation.digest(namespace))
        if (!directory.exists()) { check(directory.mkdir()) { "EXPORT_STORAGE_UNAVAILABLE" }; syncDirectory(base) }
        check(directory.isDirectory && directory.canonicalFile == directory.absoluteFile) { "EXPORT_STORAGE_UNAVAILABLE" }
        return directory
    }

    fun sha256(file: File): String {
        val digest = MessageDigest.getInstance("SHA-256")
        file.inputStream().use { input ->
            val buffer = ByteArray(32768)
            while (true) { val count = input.read(buffer); if (count < 0) break; digest.update(buffer, 0, count) }
        }
        return digest.digest().joinToString("") { "%02x".format(it) }
    }

    // HTTP200 can end mid-stream after cancellation. Check the complete directory without
    // loading a potentially large package into RAM or extracting any ZIP paths.
    fun validateZIP(file: File) = RandomAccessFile(file, "r").use { input ->
        val length = input.length()
        if (length > limit) PersonalExportWire.fail("EXPORT_FILE_LIMIT")
        if (length < 22) PersonalExportWire.fail("EXPORT_FILE_INVALID")
        val start = maxOf(0, length - 65557)
        val bytes = ByteArray((length - start).toInt()); input.seek(start); input.readFully(bytes)
        val tail = ByteBuffer.wrap(bytes).order(ByteOrder.LITTLE_ENDIAN)
        fun short(at: Int) = tail.getShort(at).toInt() and 0xffff
        fun uint(at: Int) = tail.getInt(at).toLong() and 0xffffffffL
        val end = (bytes.size - 22 downTo 0).firstOrNull { tail.getInt(it) == 0x06054b50 && it + 22 + short(it + 20) == bytes.size }
            ?: PersonalExportWire.fail("EXPORT_FILE_INVALID")
        if (short(end + 4) != 0 || short(end + 6) != 0 || short(end + 8) != short(end + 10)) PersonalExportWire.fail("EXPORT_FILE_INVALID")
        var count = short(end + 10).toLong(); var size = uint(end + 12); var offset = uint(end + 16); var boundary = start + end
        if (count == 65535L || size == 0xffffffffL || offset == 0xffffffffL) {
            if (end < 20 || tail.getInt(end - 20) != 0x07064b50 || tail.getInt(end - 16) != 0 || tail.getInt(end - 4) != 1) PersonalExportWire.fail("EXPORT_FILE_INVALID")
            val recordAt = tail.getLong(end - 12)
            if (recordAt < 0 || recordAt > length - 56) PersonalExportWire.fail("EXPORT_FILE_INVALID")
            val record = ByteArray(56); input.seek(recordAt); input.readFully(record)
            val zip64 = ByteBuffer.wrap(record).order(ByteOrder.LITTLE_ENDIAN)
            val recordSize = zip64.getLong(4)
            if (zip64.getInt(0) != 0x06064b50 || zip64.getInt(16) != 0 || zip64.getInt(20) != 0 || recordSize < 44 || recordSize > length || recordAt + 12 + recordSize != start + end - 20) PersonalExportWire.fail("EXPORT_FILE_INVALID")
            count = zip64.getLong(32); size = zip64.getLong(40); offset = zip64.getLong(48); boundary = recordAt
            if (zip64.getLong(24) != count) PersonalExportWire.fail("EXPORT_FILE_INVALID")
        }
        if (size > directoryLimit || count > directoryLimit / 46) PersonalExportWire.fail("EXPORT_FILE_LIMIT")
        if (count < 1 || size < count * 46 || offset < 0 || size < 0 || offset > length || offset + size > boundary) PersonalExportWire.fail("EXPORT_FILE_INVALID")
        var position = offset
        repeat(count.toInt()) {
            if (position + 46 > offset + size) PersonalExportWire.fail("EXPORT_FILE_INVALID")
            val row = ByteArray(46); input.seek(position); input.readFully(row)
            val entry = ByteBuffer.wrap(row).order(ByteOrder.LITTLE_ENDIAN)
            val compressed = entry.getInt(20).toLong() and 0xffffffffL
            val local = entry.getInt(42).toLong() and 0xffffffffL
            val name = entry.getShort(28).toInt() and 0xffff
            val extra = entry.getShort(30).toInt() and 0xffff
            val comment = entry.getShort(32).toInt() and 0xffff
            if (entry.getInt(0) != 0x02014b50 || name == 0 || entry.getShort(34).toInt() != 0 || local + compressed + 30 > offset) PersonalExportWire.fail("EXPORT_FILE_INVALID")
            position += 46 + name + extra + comment
        }
        if (position != offset + size) PersonalExportWire.fail("EXPORT_FILE_INVALID")
    }
}

internal suspend fun streamPersonalExport(
    connection: HttpsURLConnection,
    directory: File,
    current: () -> Boolean,
    progress: (Long, Long?) -> Unit,
): PersonalExportFile = withContext(Dispatchers.IO) {
    suspendCancellableCoroutine { continuation ->
        val part = File(directory, UUID.randomUUID().toString() + ".part")
        val target = File(directory, UUID.randomUUID().toString() + ".zip")
        PersonalExportFiles.retain(part); PersonalExportFiles.retain(target)
        continuation.invokeOnCancellation { connection.disconnect() }
        try {
            if (!current() || !continuation.isActive) throw kotlinx.coroutines.CancellationException("EXPORT_CONTEXT_CHANGED")
            if (connection.responseCode != 200) {
                val raw = connection.errorStream?.use { readLimited(it, 65536) }?.toString(Charsets.UTF_8).orEmpty()
                throw ApiFailure(connection.responseCode, runCatching { org.json.JSONObject(raw).getJSONObject("error").getString("code") }.getOrDefault("EXPORT_DOWNLOAD_DENIED"))
            }
            val name = PersonalExportFiles.filename(connection.getHeaderField("Content-Disposition"))
            if (connection.contentType?.substringBefore(';')?.trim()?.lowercase() != "application/zip") PersonalExportWire.fail("EXPORT_FILE_INVALID")
            val declared = connection.getHeaderField("Content-Length")?.let {
                if (!it.matches(Regex("0|[1-9][0-9]*"))) PersonalExportWire.fail("EXPORT_FILE_INVALID")
                it.toLongOrNull() ?: PersonalExportWire.fail("EXPORT_FILE_LIMIT")
            }
            if (declared != null && declared > PersonalExportFiles.limit) PersonalExportWire.fail("EXPORT_FILE_LIMIT")
            val digest = MessageDigest.getInstance("SHA-256"); var size = 0L; var lastProgress = 0L
            FileOutputStream(part).use { output ->
                connection.inputStream.use { input ->
                    val buffer = ByteArray(32768)
                    while (true) {
                        if (!current() || !continuation.isActive) throw kotlinx.coroutines.CancellationException("EXPORT_CONTEXT_CHANGED")
                        val count = input.read(buffer); if (count < 0) break
                        size += count
                        if (size > PersonalExportFiles.limit) PersonalExportWire.fail("EXPORT_FILE_LIMIT")
                        output.write(buffer, 0, count); digest.update(buffer, 0, count)
                        if (System.currentTimeMillis() - lastProgress > 200) { progress(size, declared); lastProgress = System.currentTimeMillis() }
                    }
                }
                output.flush(); output.fd.sync()
            }
            if (declared != null && declared != size) PersonalExportWire.fail("EXPORT_FILE_INVALID")
            PersonalExportFiles.validateZIP(part)
            if (!current() || !continuation.isActive) throw kotlinx.coroutines.CancellationException("EXPORT_CONTEXT_CHANGED")
            Files.move(part.toPath(), target.toPath(), StandardCopyOption.ATOMIC_MOVE)
            PersonalExportFiles.syncDirectory(directory)
            val result = PersonalExportFile(target, name, size, digest.digest().joinToString("") { "%02x".format(it) })
            continuation.resume(result, onCancellation = { _, file, _ -> file.dispose() })
        } catch (error: Exception) {
            part.delete(); target.delete(); PersonalExportFiles.release(target)
            runCatching { PersonalExportFiles.syncDirectory(directory) }
            if (continuation.isActive) continuation.resumeWithException(error)
        } finally { PersonalExportFiles.release(part); connection.disconnect() }
    }
}
