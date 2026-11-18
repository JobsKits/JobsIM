package com.jobs.im

import android.app.Application
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.io.IOException
import java.time.Instant
import java.util.UUID
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class PersonalExportLifecycleTest {
    private fun uuid() = UUID.randomUUID().toString()
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext

    private inner class Fixture(val database: ChatDatabase) {
        val profile = uuid(); val deployment = uuid(); val user = uuid(); val device = uuid(); val family = uuid()
        val ns = Isolation.namespace(profile, deployment, user, false, "1", "personal", device)
        val identity = SyncIdentity(ns, profile, deployment, user, "1", device, family)
        var live: SyncIdentity? = identity
        var enabled = true
        var mode = "ok"
        var cancelUnknown = false
        var afterDownload: (() -> Unit)? = null
        val writes = mutableListOf<Pair<String, String>>()
        val facts = mutableMapOf<String, JSONObject>()
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
        val store = PersonalExportStore(database)
        val api = object : Api("https://personal-export.invalid") {
            override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                if (path == "/bootstrap") return JSONObject().put("deploymentId", deployment).put("restoreEpoch", "1").put("features", JSONObject().put("chat_export", enabled))
                if (path == "/me/data-export-results") return facts[data!!.getString("requestId")]?.let { JSONObject(it.toString()) } ?: throw ApiFailure(404, "EXPORT_OPERATION_NOT_FOUND")
                if (path == "/me/data-exports") {
                    val raw = data!!.toString()
                    assertTrue(store.list(identity) { live == identity }.any { it.originalBody == raw })
                    writes.add(path to raw)
                    if (mode == "unreached") throw IOException("Injected before receipt")
                    val id = uuid()
                    val value = view(id)
                    facts[data.getString("requestId")] = value
                    if (mode == "lost") throw IOException("Injected after commit")
                    return JSONObject(value.toString())
                }
                val row = facts.values.firstOrNull { path.startsWith("/me/data-exports/" + it.getJSONObject("job").getString("id")) } ?: throw ApiFailure(404, "EXPORT_NOT_FOUND")
                if (path.endsWith("/cancel")) {
                    assertTrue(store.list(identity) { live == identity }.any { it.cancelBody == data!!.toString() })
                    writes.add(path to data!!.toString())
                    if (cancelUnknown) throw IOException("Injected cancellation response unknown")
                    row.getJSONObject("job").put("state", "cancelled").put("cleanupState", "cleared")
                }
                if (path.endsWith("/download-ticket")) return JSONObject().put("format", "zip").put("downloadToken", "RAM_ONLY_TEST_TICKET_123456789").put("expiresAt", Instant.now().plusSeconds(50).toString())
                return JSONObject(row.toString())
            }

            override suspend fun downloadExport(path: String, downloadToken: String, directory: File, current: () -> Boolean, progress: (Long, Long?) -> Unit): PersonalExportFile {
                assertFalse(path.contains('?')); assertEquals("RAM_ONLY_TEST_TICKET_123456789", downloadToken)
                val file = File(directory, uuid() + ".zip")
                PersonalExportFiles.retain(file)
                ZipOutputStream(file.outputStream()).use { zip -> zip.putNextEntry(ZipEntry("account-export.json")); zip.write("{}".toByteArray()); zip.closeEntry() }
                afterDownload?.invoke()
                return PersonalExportFile(file, "jobsim-personal-data.zip", file.length(), PersonalExportFiles.sha256(file))
            }
        }
        val model = PersonalExportModel(context.applicationContext as Application, scope, database, { live }, { api })
        fun view(id: String) = JSONObject().put("job", JSONObject().put("id", id).put("artifactId", uuid()).put("restoreEpoch", "1")
            .put("buildGeneration", "9007199254740993").put("state", "ready").put("expiresAt", Instant.now().plusSeconds(3600).toString()).put("cleanupState", "published"))
            .put("format", "zip").put("scope", "personal_current").put("coverage", JSONArray(listOf("account_public_fields"))).put("exclusions", JSONArray(listOf("access_and_refresh_tokens")))
        suspend fun start() {
            database.messages().putNativeScope(NativeScope(uuid(), "live|$profile|$deployment|", ns, device, "owned fixture"))
            SyncStore(database).ensure(identity) { true }
        }
    }

    private fun fixture(block: suspend (Fixture) -> Unit) = runBlocking {
        val name = "JobsIM-test-export-${uuid()}.db"
        val db = ChatDatabase.open(context, false, name)
        val f = Fixture(db)
        try { f.start(); block(f) }
        finally { withContext(Dispatchers.Main) { f.model.reset(); f.scope.cancel() }; db.close(); context.deleteDatabase(name) }
    }

    @Test fun lostCreateReconcilesOriginalWithoutSecondCreatingPost() = fixture { f ->
        f.mode = "lost"
        withContext(Dispatchers.Main) { f.model.create().join() }
        val row = f.store.list(f.identity) { true }.single()
        assertTrue(row.jobId.isNotEmpty())
        withContext(Dispatchers.Main) { f.model.check(row.requestId).join() }
        assertEquals(1, f.writes.count { it.first == "/me/data-exports" })
        assertEquals(row.originalBody, f.store.list(f.identity) { true }.single().originalBody)
    }

    @Test fun unreachedOriginalNeedsExplicitSameBodyRetry() = fixture { f ->
        f.mode = "unreached"
        withContext(Dispatchers.Main) { f.model.create().join() }
        val row = f.store.list(f.identity) { true }.single()
        withContext(Dispatchers.Main) { f.model.check(row.requestId).join() }
        assertEquals(1, f.writes.size)
        f.mode = "ok"
        withContext(Dispatchers.Main) { f.model.check(row.requestId, true).join() }
        assertEquals(2, f.writes.size)
        assertTrue(f.writes.all { it.second == row.originalBody })
        assertEquals(row.requestId, f.store.list(f.identity) { true }.single().requestId)
    }

    @Test fun capabilityWithdrawalKeepsOriginalCancellationButNoNewCreateOrDownload() = fixture { f ->
        withContext(Dispatchers.Main) { f.model.create().join() }
        val row = f.store.list(f.identity) { true }.single()
        f.enabled = false
        withContext(Dispatchers.Main) { f.model.create().join(); f.model.download(row.requestId).join(); f.model.cancel(row.requestId).join() }
        assertEquals(1, f.writes.count { it.first == "/me/data-exports" })
        assertEquals("cancelled", f.model.views[row.requestId]?.state)
        val cancelled = f.store.list(f.identity) { true }.single()
        assertNotEquals(row.requestId, JSONObject(cancelled.cancelBody).getString("requestId"))
        assertFalse(cancelled.json().toString().contains("RAM_ONLY"))
    }

    @Test fun cancellationUnknownRetainsNonceAndReadOnlyCheckNeverResendsIt() = fixture { f ->
        withContext(Dispatchers.Main) { f.model.create().join() }
        val row = f.store.list(f.identity) { true }.single()
        f.cancelUnknown = true
        withContext(Dispatchers.Main) { f.model.cancel(row.requestId).join(); f.model.check(row.requestId).join() }
        val cancelBody = f.store.list(f.identity) { true }.single().cancelBody
        assertEquals(1, f.writes.count { it.first.endsWith("/cancel") })
        f.cancelUnknown = false
        withContext(Dispatchers.Main) { f.model.cancel(row.requestId).join() }
        assertTrue(f.writes.filter { it.first.endsWith("/cancel") }.all { it.second == cancelBody })
    }

    @Test fun cancelledSystemSelectionIsNeverSavedAndOldFamilyCannotPublishPreparedFile() = fixture { f ->
        withContext(Dispatchers.Main) { f.model.create().join() }
        val row = f.store.list(f.identity) { true }.single()
        withContext(Dispatchers.Main) { f.model.download(row.requestId).join() }
        withContext(Dispatchers.Main) { val id = f.model.preparedId; assertNotNull(f.model.claimSaveName(id)); f.model.save(id, null) }
        assertEquals("exportSaveCancelled", f.model.notice)
        assertEquals("", f.model.preparedId)
        f.afterDownload = { f.live = f.identity.copy(family = uuid()) }
        withContext(Dispatchers.Main) { f.model.download(row.requestId).join() }
        assertEquals("", f.model.preparedId)
        val directory = File(File(context.noBackupFilesDir, "personal-exports"), Isolation.digest(f.ns))
        assertTrue(directory.listFiles().orEmpty().none { it.extension == "zip" || it.extension == "part" })
    }
}
