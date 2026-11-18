package com.jobs.im

import android.app.Application
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.time.Instant
import java.util.UUID
import kotlinx.coroutines.*
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class OwnDeletionLifecycleTest {
    private fun uuid() = UUID.randomUUID().toString()
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private inner class Fixture(val db: ChatDatabase) {
        val profile = uuid(); val deployment = uuid(); val family = uuid(); val user = uuid(); val device = uuid(); val deletionId = uuid()
        val owner = PasswordOwner(user, "deletion_fixture", device, family, Isolation.namespace(profile, deployment, user, false, "1", "personal", device), "synthetic-family-token")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
        val calls = mutableListOf<Pair<String, JSONObject>>()
        var mode = "lost"; var committed = false; var available = true; var cancelled = false; var cancellation = ""
        fun result(id: String) = JSONObject().put("deletionId", deletionId).put("requestId", id).put("status", if (cancelled) "cancelled" else "requested")
            .put("requestVersion", if (cancelled) "2" else "1").put("restoreEpoch", "1").put("cancelUntil", Instant.now().plusSeconds(604800).toString())
            .put("cancelRequestId", cancellation).put("requiresNewLogin", true)
        fun model() = OwnDeletionModel(db, scope, { PasswordEnvironment(profile, "https://deletion.invalid", deployment, "1", "owner") }, { owner }, { token ->
            object : Api("https://deletion.invalid", token) {
                override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                    if (path == "/bootstrap") return JSONObject().put("deploymentId", deployment).put("restoreEpoch", "1").put("features", JSONObject().put("own_account_deletion", available))
                    val body = data ?: JSONObject(); calls.add(path to body)
                    if (path.endsWith("deletion-results") && !committed) throw ApiFailure(404, "ACCOUNT_DELETION_NOT_FOUND")
                    val stored = db.syncRecords().rows("$profile|account-deletions", "account-deletion").single()
                    val row = OwnDeletionRules.parse(JSONObject(stored.payload))
                    if (path.endsWith("deletion-requests")) {
                        assertEquals("unknown", row.state); committed = true
                        if (mode == "lost") throw ApiFailure(0, "TRANSPORT_UNKNOWN")
                        if (mode == "prepared403") throw ApiFailure(403, "DEPENDENCY_FORBIDDEN")
                    }
                    if (path.endsWith("/proofs")) return JSONObject().put("proof", "synthetic-purpose-proof-" + "x".repeat(32)).put("deletionId", JSONObject(row.result).getString("deletionId"))
                        .put("purpose", body.getString("purpose")).put("requestVersion", if (cancelled) "2" else "1").put("restoreEpoch", "1").put("expiresAt", Instant.now().plusSeconds(300).toString())
                    if (path.endsWith("/cancel")) { cancellation = body.getString("requestId"); cancelled = true; if (mode == "cancel-lost") throw ApiFailure(0, "TRANSPORT_UNKNOWN") }
                    return result(row.id)
                }
            }
        }, {})
        suspend fun request(model: OwnDeletionModel, id: String? = null, retry: Boolean = false) {
            withContext(Dispatchers.Main) { model.request(id, "synthetic-original-password", "123456", retry) }; await(model)
        }
        suspend fun rights(model: OwnDeletionModel, cancel: Boolean) {
            val row = model.rows.single(); val version = JSONObject(row.result).getString("requestVersion")
            withContext(Dispatchers.Main) { model.rights(row.id, "synthetic-original-password", "", "", cancel, version) }; await(model)
        }
        suspend fun await(model: OwnDeletionModel) { withTimeout(5000) { while (model.busy) delay(10) } }
    }
    private fun fixture(action: suspend (Fixture) -> Unit) = runBlocking {
        val name = "JobsIM-test-deletion-${uuid()}.db"; val db = ChatDatabase.open(context, false, name); val fixture = Fixture(db)
        try { action(fixture) } finally { fixture.scope.cancel(); db.close(); context.deleteDatabase(name) }
    }
    @Test fun unknownSurvivesReopenWithoutPasswordOtpOrFamilyToken() = fixture { f ->
        val model = f.model(); f.request(model); val stored = f.db.syncRecords().rows("${f.profile}|account-deletions", "account-deletion").single().payload
        for (secret in listOf("synthetic-original-password", "123456", "synthetic-family-token")) assertFalse(stored.contains(secret))
        val restored = f.model(); restored.load(); assertTrue(restored.blocked(f.family)); f.mode = "ok"; f.request(restored, restored.rows.single().id)
        assertEquals(1, f.calls.count { it.first.endsWith("deletion-requests") }); assertEquals("requested", restored.rows.single().state)
    }
    @Test fun preparedDependency403CannotReleaseTheOriginalGate() = fixture { f ->
        f.mode = "prepared403"; val model = f.model(); f.request(model); assertTrue(model.blocked(f.family)); assertEquals("unknown", model.rows.single().state)
    }
    @Test fun withdrawnCapabilityKeepsUnknownLookupReadonly() = fixture { f ->
        val model = f.model(); f.request(model); f.available = false; f.committed = false; f.mode = "ok"; f.request(model, model.rows.single().id, true)
        assertEquals(1, f.calls.count { it.first.endsWith("deletion-requests") }); assertTrue(model.blocked(f.family))
    }
    @Test fun unknownCancellationRetainsOneNonceAndQueriesItsOriginalResult() = fixture { f ->
        f.mode = "ok"; val model = f.model(); f.request(model); f.mode = "cancel-lost"; f.rights(model, true)
        val original = model.rows.single(); assertEquals("unknown", original.cancelState); assertTrue(original.cancelId.isNotEmpty())
        f.rights(model, false); assertEquals(original.cancelId, model.rows.single().cancelId); assertEquals("cancelled", model.rows.single().state)
        val stored = f.db.syncRecords().rows("${f.profile}|account-deletions", "account-deletion").single().payload
        assertFalse(stored.contains("synthetic-purpose-proof")); assertEquals(1, f.calls.count { it.first.endsWith("/cancel") })
    }
    @Test fun finalLocalErasureKeepsOtherAccountsAndThePublicResponsibility() = fixture { f ->
        f.mode = "ok"; val model = f.model(); f.request(model); val before = model.rows.single(); val row = before.copy(state = "completed", result = JSONObject(before.result).put("status","completed").toString())
        val own = row.meta.namespace; val other = Isolation.namespace(f.profile, f.deployment, uuid(), false, "1", "personal", uuid())
        f.db.syncRecords().put(SyncRecord("$own|private", own, "fixture", "private own body")); f.db.syncRecords().put(SyncRecord("$other|private", other, "fixture", "other account body"))
        val app = context.applicationContext as Application; val directory = File(app.cacheDir, "Attachments/${Isolation.digest(own)}"); directory.mkdirs(); val file = File(directory, "fixture.bin"); file.writeText("owned fixture bytes")
        val avatarDirectory=File(app.noBackupFilesDir,"AvatarSources/${Isolation.digest(own)}");avatarDirectory.mkdirs();val avatar=File(avatarDirectory,uuid());avatar.writeText("owned original avatar")
        OwnDeletionCleanup.erase(app, f.db, row)
        assertFalse(file.exists()); assertFalse(avatar.exists()); assertNull(f.db.syncRecords().one("$own|private")); assertNotNull(f.db.syncRecords().one("$other|private"))
        assertEquals(1, f.db.syncRecords().rows("${f.profile}|account-deletions", "account-deletion").size)
    }
}
