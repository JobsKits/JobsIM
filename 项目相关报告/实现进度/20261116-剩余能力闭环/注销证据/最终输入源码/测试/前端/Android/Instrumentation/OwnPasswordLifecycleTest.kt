package com.jobs.im

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.*
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith
import java.time.Instant
import java.util.UUID

@RunWith(AndroidJUnit4::class)
class OwnPasswordLifecycleTest {
    private fun uuid() = UUID.randomUUID().toString()
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private inner class Fixture(val db: ChatDatabase) {
        val profile = uuid(); val deployment = uuid(); val family = uuid(); var stamp = "first"
        var owner = PasswordOwner(uuid(), "fixture_user", uuid(), family, uuid(), "synthetic-original-token")
        val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
        val calls = mutableListOf<Triple<String, String, String>>()
        var mode = "lost"; var committed = false; var stops = 0; var enabled = true; var serverEpoch = "1"; var gate: CompletableDeferred<Unit>? = null
        fun model() = OwnPasswordModel(db, scope,
            { PasswordEnvironment(profile, "https://password.invalid", deployment, "1", stamp) }, { owner },
            { token -> object : Api("https://password.invalid", token) {
                override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                    if (path == "/bootstrap") return JSONObject().put("deploymentId", deployment).put("restoreEpoch", serverEpoch).put("features", JSONObject().put("own_password_change", enabled))
                    calls.add(Triple(path, data!!.toString(), token))
                    if (path.endsWith("-results") && !committed) throw ApiFailure(404, "PASSWORD_CHANGE_OPERATION_NOT_FOUND")
                    if (path.endsWith("-results") && mode == "wrong-proof") throw ApiFailure(403, "PASSWORD_CHANGE_REJECTED")
                    if (path == "/auth/password-change") {
                        val persisted = db.syncRecords().rows("$profile|password-changes", "password-change").single()
                        assertEquals("unknown", JSONObject(persisted.payload).getString("state"))
                        committed = true; gate?.await()
                        if (mode == "lost") throw ApiFailure(0, "TRANSPORT_UNKNOWN")
                        if (mode == "prepared403") throw ApiFailure(403, "DEPENDENCY_FORBIDDEN")
                    }
                    return JSONObject().put("requestId", data.getString("requestId")).put("status", if(mode=="pending") "pending" else "changed").put("restoreEpoch", "1")
                        .put("credentialGeneration", "9007199254740993").put("securityVersion", "3").put("current", mode!="pending").put("requiresNewLogin", mode!="pending")
                }
            } }, { original -> if (owner.family == original.family) stops++ })
        suspend fun run(model: OwnPasswordModel, id: String? = null, retry: Boolean = false) {
            withContext(Dispatchers.Main) { model.perform(id, "fixture-original-password", "fixture-replacement-password", "fixture-replacement-password", "123456", retry) }
            withTimeout(5000) { while (model.busy) delay(10) }
        }
    }
    private fun fixture(action: suspend (Fixture) -> Unit) = runBlocking {
        val name = "JobsIM-test-password-${uuid()}.db"; val db = ChatDatabase.open(context, false, name); val f = Fixture(db)
        try { action(f) } finally { f.scope.cancel(); db.close(); context.deleteDatabase(name) }
    }
    @Test fun unknownSurvivesReconstructionAndContainsNoPasswordsOrOtp() = fixture { f ->
        val model = f.model(); f.run(model)
        assertTrue(model.blocked(f.family))
        val stored = f.db.syncRecords().rows("${f.profile}|password-changes", "password-change").single().payload
        for (value in listOf("fixture-original-password", "fixture-replacement-password", "123456", "synthetic-original-token")) assertFalse(stored.contains(value))
        val restored = f.model(); withContext(Dispatchers.Main) { restored.load() }; assertTrue(restored.blocked(f.family))
        f.mode = "ok"; f.run(restored, restored.rows.single().id)
        assertEquals(1, f.calls.count { it.first == "/auth/password-change" }); assertEquals(1, f.stops)
        assertTrue(f.calls.filter { it.first.endsWith("-results") }.all { it.third.isEmpty() && !JSONObject(it.second).has("otp") })
    }
    @Test fun wrongProofCannotUnpauseUnknownOrCreateAnotherOperation() = fixture { f ->
        val model = f.model(); f.run(model); f.mode = "wrong-proof"; f.run(model, model.rows.single().id)
        assertTrue(model.blocked(f.family)); assertEquals(1, f.calls.count { it.first == "/auth/password-change" }); assertEquals(0, f.stops)
    }
    @Test fun explicitRetryNeverBorrowsSuccessorFamilyToken() = fixture { f ->
        val model = f.model(); f.run(model); val id = model.rows.single().id
        f.committed = false; f.mode = "ok"; f.owner = f.owner.copy(family = uuid(), token = "synthetic-successor-token")
        f.run(model, id, true)
        assertEquals(1, f.calls.count { it.first == "/auth/password-change" }); assertTrue(model.failed); assertEquals(0, f.stops)
    }
    @Test fun lateReplyCannotClearAChangedIdentity() = fixture { f ->
        val model = f.model(); f.mode = "ok"; val gate = CompletableDeferred<Unit>(); f.gate = gate
        withContext(Dispatchers.Main) { model.perform(null, "fixture-original-password", "fixture-replacement-password", "fixture-replacement-password", "", false) }
        withTimeout(5000) { while (!f.committed) delay(10) }
        withContext(Dispatchers.Main) { f.stamp = "successor"; f.owner = f.owner.copy(family = uuid()); gate.complete(Unit) }
        withTimeout(5000) { while (model.busy) delay(10) }
        assertEquals(0, f.stops)
        assertEquals("unknown", JSONObject(f.db.syncRecords().rows("${f.profile}|password-changes", "password-change").single().payload).getString("state"))
    }
    @Test fun withdrawnCapabilityAndChangedEpochAllowLookupButNoPendingOrUnseenMutation() = fixture { f ->
        val model=f.model(); f.run(model); val id=model.rows.single().id; f.mode="pending"
        for (restored in listOf(false,true)) for (unseen in listOf(false,true)) {
            f.enabled=restored; f.serverEpoch=if(restored) "2" else "1"; f.committed=!unseen
            val count=f.calls.count { it.first=="/auth/password-change" }
            f.run(model,id,false); f.run(model,id,true)
            assertTrue(model.failed); assertEquals(count,f.calls.count { it.first=="/auth/password-change" })
            assertTrue(f.calls.takeLast(2).all { it.first.endsWith("-results")&&it.third.isEmpty() })
        }
    }
    @Test fun preparedDependency403KeepsOriginalPaused() = fixture { f ->
        val model=f.model();f.mode="prepared403";f.run(model)
        assertTrue(model.blocked(f.family));assertEquals("unknown",model.rows.single().state)
    }

}
