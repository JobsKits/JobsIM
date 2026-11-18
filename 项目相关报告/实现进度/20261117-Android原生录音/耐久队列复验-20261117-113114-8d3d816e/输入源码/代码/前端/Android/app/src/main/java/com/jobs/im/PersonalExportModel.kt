package com.jobs.im

import android.app.Application
import android.net.Uri
import android.os.ParcelFileDescriptor
import android.provider.DocumentsContract
import androidx.compose.runtime.*
import androidx.compose.runtime.snapshots.Snapshot
import java.io.File
import java.time.Instant
import java.util.UUID
import java.util.concurrent.ConcurrentHashMap
import kotlinx.coroutines.*
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import org.json.JSONObject

private object PersonalExportLocks {
    private val locks = ConcurrentHashMap<String, Mutex>()
    fun get(namespace: String) = locks.getOrPut(namespace) { Mutex() }
}

class PersonalExportModel(
    private val app: Application,
    private val scope: CoroutineScope,
    database: ChatDatabase,
    private val identity: () -> SyncIdentity?,
    private val transport: () -> Api,
) {
    private val store = PersonalExportStore(database)
    val rows = mutableStateListOf<PersonalExportIntent>()
    val views = mutableStateMapOf<String, PersonalExportView>()
    var enabled by mutableStateOf(false)
        private set
    var busy by mutableStateOf(false)
        private set
    var problem by mutableStateOf("")
        private set
    var notice by mutableStateOf("")
        private set
    var downloading by mutableStateOf(false)
        private set
    var downloadedBytes by mutableLongStateOf(0)
        private set
    var shown by mutableIntStateOf(20)
        private set
    val connected get() = identity() != null
    private var serial = 0
    private var sequence = 0
    private var task: Job? = null
    private data class Bound(val identity: SyncIdentity, val serial: Int)
    private class Prepared(val id: String, val original: Bound, val requestId: String, val file: PersonalExportFile) {
        var claimed = false
    }
    private var prepared by mutableStateOf<Prepared?>(null)
    val preparedId get() = prepared?.id.orEmpty()
    private fun capture() = identity()?.let { Bound(it, serial) } ?: PersonalExportWire.fail("EXPORT_CONNECT_REQUIRED")
    private fun current(original: Bound) = original.serial == serial && identity() == original.identity
    private fun guard(original: Bound) { if (!current(original)) throw CancellationException("EXPORT_CONTEXT_CHANGED") }

    init {
        scope.launch(Dispatchers.IO) { runCatching { PersonalExportFiles.clearOwnedOrphans(File(app.noBackupFilesDir, "personal-exports")) } }
    }

    fun reset() {
        serial++; sequence++; task?.cancel(); task = null
        prepared?.file?.dispose(); prepared = null
        rows.clear(); views.clear(); enabled = false; busy = false; downloading = false
        downloadedBytes = 0; problem = ""; notice = ""; shown = 20
    }

    private suspend fun capability(original: Bound, required: Boolean): Api {
        guard(original)
        val api = transport()
        val boot = api.request("/bootstrap")
        guard(original)
        if (boot.opt("deploymentId") != original.identity.deployment || boot.opt("restoreEpoch") != original.identity.epoch) PersonalExportWire.fail("EXPORT_CONTEXT_CHANGED")
        enabled = boot.optJSONObject("features")?.opt("chat_export") == true
        if (required && !enabled) PersonalExportWire.fail("EXPORT_UNAVAILABLE")
        return api
    }

    private fun work(action: suspend (Bound) -> Unit): Job {
        if (busy || prepared != null) return scope.launch {}
        val original = try { capture() } catch (_: Exception) { problem = "EXPORT_CONNECT_REQUIRED"; return scope.launch {} }
        busy = true; problem = ""; notice = ""
        return scope.launch {
            try { PersonalExportLocks.get(original.identity.namespace).withLock { guard(original); action(original) } }
            catch (error: CancellationException) { throw error }
            catch (error: Exception) {
                if (current(original)) {
                    problem = when (error) { is ApiFailure -> error.code; is PersonalExportFailure -> error.code; else -> "EXPORT_UNKNOWN" }
                    runCatching { refreshCurrent(original) }
                }
            } finally { if (current(original)) { busy = false; downloading = false } }
        }.also { task = it }
    }

    private suspend fun loadOriginal(original: Bound, requestId: String) = store.original(original.identity, requestId) { current(original) }
    private suspend fun lookup(bound: Bound, row: PersonalExportIntent, api: Api): PersonalExportView? {
        val raw = try { api.request("/me/data-export-results", "POST", JSONObject(row.originalBody)) }
            catch (error: ApiFailure) { guard(bound); if (error.status == 404 && error.code == "EXPORT_OPERATION_NOT_FOUND") return null else throw error }
        guard(bound)
        val view = PersonalExportWire.view(raw, bound.identity, row.jobId)
        store.bind(bound.identity, row, view.id) { current(bound) }
        guard(bound)
        return view
    }

    private suspend fun status(bound: Bound, row: PersonalExportIntent, api: Api): PersonalExportView? {
        if (row.jobId.isEmpty()) return lookup(bound, row, api)
        val raw = api.request("/me/data-exports/${row.jobId}")
        guard(bound)
        return PersonalExportWire.view(raw, bound.identity, row.jobId)
    }

    private suspend fun refreshCurrent(original: Bound, supplied: Api? = null) {
        val api = supplied ?: capability(original, false)
        val reading = ++sequence
        guard(original); views.clear()
        val known = store.list(original.identity) { current(original) }
        val next = mutableMapOf<String, PersonalExportView>()
        var failed = false
        for (row in known.take(shown)) {
            try { status(original, row, api)?.let { next[row.requestId] = it } }
            catch (error: CancellationException) { throw error }
            catch (_: Exception) { guard(original); failed = true }
        }
        val refreshed = store.list(original.identity) { current(original) }
        guard(original)
        if (reading != sequence) return
        rows.clear(); rows.addAll(refreshed); views.clear(); views.putAll(next)
        if (failed && problem.isEmpty()) problem = "EXPORT_UNKNOWN"
    }

    fun refresh() = work { original ->
        val directory = withContext(Dispatchers.IO) { PersonalExportFiles.directory(File(app.noBackupFilesDir, "personal-exports"), original.identity.namespace) }
        withContext(Dispatchers.IO) { PersonalExportFiles.clearOrphans(directory) }
        guard(original); refreshCurrent(original)
    }

    fun more() { shown += 20; refresh() }

    fun create() = work { original ->
        val api = capability(original, true)
        val row = store.prepare(original.identity) { current(original) }
        if (lookup(original, row, api) == null) {
            val value = PersonalExportWire.view(api.request("/me/data-exports", "POST", JSONObject(row.originalBody)), original.identity)
            guard(original); store.bind(original.identity, row, value.id) { current(original) }
        }
        refreshCurrent(original, api)
    }

    fun check(requestId: String, retry: Boolean = false) = work { original ->
        val api = capability(original, retry)
        val row = loadOriginal(original, requestId)
        val found = lookup(original, row, api)
        if (found == null && retry && row.jobId.isEmpty()) {
            val value = PersonalExportWire.view(api.request("/me/data-exports", "POST", JSONObject(row.originalBody)), original.identity)
            guard(original); store.bind(original.identity, row, value.id) { current(original) }
        } else if (found == null) notice = "exportPendingHint"
        refreshCurrent(original, api)
    }

    fun cancel(requestId: String) = work { original ->
        val api = capability(original, false)
        val row = store.cancellation(original.identity, loadOriginal(original, requestId)) { current(original) }
        val view = PersonalExportWire.view(api.request("/me/data-exports/${row.jobId}/cancel", "POST", JSONObject(row.cancelBody)), original.identity, row.jobId)
        guard(original); views[requestId] = view
        refreshCurrent(original, api)
    }

    private suspend fun ticket(original: Bound, row: PersonalExportIntent, api: Api): String {
        val live = status(original, row, api) ?: PersonalExportWire.fail("EXPORT_UNAVAILABLE")
        if (!live.ready || row.cancelBody.isNotEmpty()) PersonalExportWire.fail("EXPORT_UNAVAILABLE")
        val result = api.request("/me/data-exports/${row.jobId}/download-ticket", "POST", JSONObject())
        guard(original)
        val token = result.opt("downloadToken") as? String ?: PersonalExportWire.fail()
        val expires = SelfStateWire.utc(result.opt("expiresAt"))
        if (result.opt("format") != "zip" || !token.matches(Regex("[A-Za-z0-9_-]{16,256}")) ||
            !expires.isAfter(Instant.now()) || expires.isAfter(Instant.now().plusSeconds(65)) || expires.isAfter(live.expiresAt)) PersonalExportWire.fail()
        return token
    }

    fun download(requestId: String) = work { original ->
        val api = capability(original, true)
        val row = loadOriginal(original, requestId)
        if (row.jobId.isEmpty() || row.cancelBody.isNotEmpty()) PersonalExportWire.fail("EXPORT_UNAVAILABLE")
        val token = ticket(original, row, api)
        val directory = withContext(Dispatchers.IO) { PersonalExportFiles.directory(File(app.noBackupFilesDir, "personal-exports"), original.identity.namespace) }
        guard(original); downloading = true; downloadedBytes = 0
        val file = api.downloadExport("/me/data-exports/${row.jobId}/download", token, directory, { current(original) }) { bytes, _ ->
            if (current(original)) Snapshot.withMutableSnapshot { downloadedBytes = bytes }
        }
        if (!current(original)) { file.dispose(); guard(original) }
        prepared = Prepared(UUID.randomUUID().toString(), original, requestId, file)
    }

    fun stopDownload() { if (downloading) { notice = "exportDownloadStopped"; task?.cancel() } }

    fun claimSaveName(id: String): String? {
        val value = prepared ?: return null
        if (id != value.id || value.claimed || !current(value.original)) return null
        value.claimed = true
        return value.file.fileName
    }

    fun save(id: String, uri: Uri?) {
        val selected = prepared ?: return
        if (selected.id != id || !selected.claimed) return
        if (uri == null) { selected.file.dispose(); prepared = null; if (current(selected.original)) notice = "exportSaveCancelled"; return }
        task = scope.launch {
            PersonalExportLocks.get(selected.original.identity.namespace).withLock {
                busy = true; problem = ""; notice = ""
                try {
                    guard(selected.original)
                    if (prepared !== selected) throw CancellationException("EXPORT_CONTEXT_CHANGED")
                    val api = capability(selected.original, true)
                    val row = loadOriginal(selected.original, selected.requestId)
                    ticket(selected.original, row, api)
                    withContext(Dispatchers.IO) {
                        guard(selected.original)
                        if (selected.file.file.length() != selected.file.size || PersonalExportFiles.sha256(selected.file.file) != selected.file.sha256) PersonalExportWire.fail("EXPORT_FILE_INVALID")
                        PersonalExportFiles.validateZIP(selected.file.file)
                        val descriptor = app.contentResolver.openFileDescriptor(uri, "rwt") ?: PersonalExportWire.fail("EXPORT_SAVE_FAILED")
                        ParcelFileDescriptor.AutoCloseOutputStream(descriptor).use { output ->
                            selected.file.file.inputStream().use { input ->
                                val buffer = ByteArray(32768)
                                while (true) {
                                    ensureActive(); guard(selected.original)
                                    val count = input.read(buffer); if (count < 0) break
                                    output.write(buffer, 0, count)
                                }
                            }
                            output.flush(); output.fd.sync(); guard(selected.original)
                        }
                    }
                    guard(selected.original); notice = "exportSaved"
                } catch (error: Exception) {
                    withContext(NonCancellable + Dispatchers.IO) { runCatching { DocumentsContract.deleteDocument(app.contentResolver, uri) } }
                    if (current(selected.original)) problem = when (error) { is PersonalExportFailure -> error.code; is ApiFailure -> error.code; else -> "EXPORT_SAVE_FAILED" }
                } finally {
                    selected.file.dispose()
                    if (prepared === selected) prepared = null
                    if (current(selected.original)) busy = false
                }
            }
        }
    }
}
