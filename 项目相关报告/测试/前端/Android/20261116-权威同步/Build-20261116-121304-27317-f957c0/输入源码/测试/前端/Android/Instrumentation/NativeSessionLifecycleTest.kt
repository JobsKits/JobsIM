package com.jobs.im

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.*
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NativeSessionLifecycleTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext

    private fun delivered(binding: SessionBinding, generation: String = "0", device: String = "device-one") =
        NativeSession.delivery(JSONObject()
            .put("restoreEpoch", binding.epoch)
            .put("user", JSONObject().put("id", "owner").put("username", "owner"))
            .put("deviceId", device).put("token", "test-access-$generation-$device")
            .put("refreshToken", "test-refresh-$generation-$device").put("familyId", "family-$device")
            .put("refreshGeneration", generation).put("sourceContextId", "personal")
            .put("accessExpiresAt", "2026-11-15T12:15:00Z")
            .put("idleExpiresAt", "2026-12-15T12:00:00Z")
            .put("absoluteExpiresAt", "2027-02-13T12:00:00Z"), binding)

    private fun binding() = SessionBinding(UUID.randomUUID().toString(), "https://localhost:36443", "isolated", "1")

    @Test
    fun retainedApiObjectCannotAdoptANewFamilyWhenInvokedAfterSwitch() = runBlocking {
        val binding = binding()
        val name = "JobsIM-test-native-session-${UUID.randomUUID()}.db"
        val database = ChatDatabase.open(context, false, name)
        val markers = NonSecretSessionState(context)
        val vault = TokenVault(context)
        val calls = AtomicInteger()
        fun raw() = object : Api(binding.origin) {
            override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                calls.incrementAndGet()
                return JSONObject().put("ok", true)
            }
        }
        val coordinator = SessionCoordinator(vault, markers, { database.messages() }, { _, _ -> raw() }, { "owner" })
        try {
            val first = InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner")
            coordinator.prepareInitial(binding, first, "owner")
            coordinator.commitInitial(delivered(binding), first.requestId, "owner")
            val oldApi = SessionApi(binding, coordinator, { raw() }, { error("unexpected blocked session") }, null)
            val second = InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner")
            coordinator.prepareInitial(binding, second, "owner")
            coordinator.commitInitial(delivered(binding, device = "new-device"), second.requestId, "owner")
            assertTrue(runCatching { oldApi.request("/me") }.exceptionOrNull() is CancellationException)
            assertEquals(0, calls.get())
            SessionApi(binding, coordinator, { raw() }, { error("unexpected blocked session") }, null).request("/me")
            assertEquals(1, calls.get())
        } finally {
            coordinator.forgetMemory(); vault.clear(binding.profile); markers.clearAfterSecureCommit(binding.profile)
            database.close(); context.deleteDatabase(name)
        }
    }

    @Test
    fun concurrentExpiredRequestsShareOneRotationAndPublishOnlyDurablePair() = runBlocking {
        val binding = binding()
        val name = "JobsIM-test-native-session-${UUID.randomUUID()}.db"
        val database = ChatDatabase.open(context, false, name)
        val markers = NonSecretSessionState(context)
        val vault = TokenVault(context)
        val refreshCalls = AtomicInteger()
        val businessCalls = AtomicInteger()
        var owner = "one"
        lateinit var coordinator: SessionCoordinator
        fun raw(credential: String) = object : Api(binding.origin, credential) {
            override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                return when (path) {
                    "/bootstrap" -> JSONObject().put("deploymentId", binding.deployment)
                        .put("restoreEpoch", "1").put("features", JSONObject().put("auth_refresh", true))
                    "/auth/refresh" -> {
                        assertEquals("", credential)
                        assertEquals("0", data!!.getString("refreshGeneration"))
                        assertEquals("test-refresh-0-device-one", data.getString("refreshToken"))
                        assertTrue(data.getString("requestId").isNotEmpty())
                        refreshCalls.incrementAndGet()
                        delay(100)
                        delivered(binding, "1").stored().put("user", JSONObject().put("id", "owner").put("username", "owner"))
                    }
                    else -> {
                        assertEquals("original-id", data!!.getString("clientMsgId"))
                        assertEquals("original-admission", data.getString("sendAdmission"))
                        businessCalls.incrementAndGet()
                        if (credential == "test-access-0-device-one") throw ApiFailure(401, "ACCESS_EXPIRED")
                        assertEquals("1", vault.loadSession(binding)!!.refreshGeneration)
                        assertEquals("", markers.unknownFamily(binding.profile))
                        JSONObject().put("ok", true)
                    }
                }
            }
        }
        coordinator = SessionCoordinator(vault, markers, { database.messages() }, { _, token -> raw(token) }, { owner })
        try {
            val initial = InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner")
            coordinator.prepareInitial(binding, initial, owner)
            coordinator.commitInitial(delivered(binding), initial.requestId, owner)
            val ns = database.messages().nativeScope("live|${binding.profile}|isolated|owner|personal|device-one|1")!!.namespace
            val api = SessionApi(binding, coordinator, { raw(it) }, { error("unexpected blocked session") }, null)
            coroutineScope {
                (1..12).map { async(Dispatchers.Default) {
                    api.request("/messages", "POST", JSONObject().put("clientMsgId", "original-id").put("sendAdmission", "original-admission"))
                } }.awaitAll()
            }
            assertEquals(1, refreshCalls.get())
            assertTrue(businessCalls.get() >= 12)
            assertEquals("1", coordinator.capture()!!.session.refreshGeneration)
            assertEquals(ns, database.messages().nativeScope(ns)!!.namespace)
            assertEquals("device-one", vault.loadSession(binding)!!.deviceId)
            owner = "other-account"
            assertTrue(runCatching { coordinator.requireCurrent(SessionCapture(delivered(binding), "one")) }.exceptionOrNull() is CancellationException)
        } finally {
            coordinator.forgetMemory(); vault.clear(binding.profile); markers.clearAfterSecureCommit(binding.profile)
            database.close(); context.deleteDatabase(name)
        }
    }

    @Test
    fun secureWriteFailureAndUnknownRefreshStayBlockedAcrossRestart() = runBlocking {
        val binding = binding()
        val name = "JobsIM-test-native-session-${UUID.randomUUID()}.db"
        val database = ChatDatabase.open(context, false, name)
        val markers = NonSecretSessionState(context)
        val vault = TokenVault(context)
        val refreshCalls = AtomicInteger()
        fun raw() = object : Api(binding.origin) {
            override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                if (path == "/bootstrap") return JSONObject().put("deploymentId", binding.deployment)
                    .put("restoreEpoch", "1").put("features", JSONObject().put("auth_refresh", true))
                refreshCalls.incrementAndGet()
                throw java.io.IOException("controlled response loss")
            }
        }
        val stable = SessionCoordinator(vault, markers, { database.messages() }, { _, _ -> raw() }, { "owner" })
        try {
            val initial = InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner")
            stable.prepareInitial(binding, initial, "owner")
            stable.commitInitial(delivered(binding), initial.requestId, "owner")
            assertTrue(runCatching { stable.refreshed(stable.capture()!!) }.exceptionOrNull() is SessionVerificationRequired)
            repeat(3) { assertTrue(runCatching { stable.refreshed(stable.capture()!!) }.exceptionOrNull() is SessionVerificationRequired) }
            assertEquals(1, refreshCalls.get())
            stable.forgetMemory()
            val restarted = SessionCoordinator(vault, markers, { database.messages() }, { _, _ -> raw() }, { "owner" })
            assertTrue(runCatching { restarted.restore(binding, "owner") }.exceptionOrNull() is SessionVerificationRequired)
            val replacement = InitialAuthIntent(UUID.randomUUID().toString(), "reauthenticate", "owner", "family-device-one")
            restarted.prepareInitial(binding, replacement, "owner")
            val failing = SessionCoordinator(vault, markers, { database.messages() }, { _, _ -> raw() }, { "owner" },
                { pair, commit -> vault.saveSession(pair, commit); throw java.io.IOException("controlled fsync failure") })
            val failure = runCatching { failing.commitInitial(delivered(binding, device = "device-two"), replacement.requestId, "owner") }.exceptionOrNull()
            assertTrue(failure is SessionVerificationRequired)
            assertNull(failing.session)
            assertEquals("SESSION_STORAGE_FAILED", markers.blockedReason(binding.profile))
            assertTrue(runCatching { restarted.restore(binding, "owner") }.exceptionOrNull() is SessionVerificationRequired)
            val stored = context.getSharedPreferences("JobsIM.session-intents", 0).getString(Isolation.digest(binding.profile), "")!!
            assertFalse(stored.contains("test-access")); assertFalse(stored.contains("test-refresh"))
        } finally {
            stable.forgetMemory(); vault.clear(binding.profile); markers.clearAfterSecureCommit(binding.profile)
            database.close(); context.deleteDatabase(name)
        }
    }

    @Test
    fun committedPairHandoffRecoversAndRetiresOriginalOutboxWithoutChangingPayload() = runBlocking {
        val binding = binding()
        val name = "JobsIM-test-native-session-${UUID.randomUUID()}.db"
        var database = ChatDatabase.open(context, false, name)
        val markers = NonSecretSessionState(context)
        val vault = TokenVault(context)
        val pair = delivered(binding, device = "new-device")
        val oldNs = Isolation.namespace(binding.profile, binding.deployment, "owner", false, "1")
        val original = MessageRow("$oldNs|original", oldNs, "room", "original", clientMsgId = "original",
            senderId = "owner", text = "retained", state = "pending", sendAdmission = "old-proof")
        val coordinator = SessionCoordinator(vault, markers, { database.messages() }, { _, _ -> error("no HTTP permitted") }, { "owner" })
        try {
            database.messages().enqueue(original)
            database.messages().saveDraft(oldNs, "room", "1", "retained draft", 4)
            val intent = InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner")
            markers.prepare(binding.profile, intent)
            val commit = UUID.randomUUID().toString()
            markers.stageSecureWrite(binding.profile, pair.familyId, commit)
            vault.saveSession(pair, commit)
            markers.secureWriteFinished(binding.profile, pair.familyId, commit)
            // A crash after the secure rename resumes only the database handoff, never refreshes.
            database.close(); database = ChatDatabase.open(context, false, name)
            assertEquals(pair.deviceId, coordinator.restore(binding, "owner")!!.deviceId)
            assertEquals(original.copy(state = "frozen"), database.messages().one(oldNs, "original"))
            database.messages().enqueue(original.copy(key = "$oldNs|late", id = "late", clientMsgId = "late"), 4)
            assertEquals("frozen", database.messages().one(oldNs, "late")!!.state)
            assertEquals("retained draft", database.messages().state(oldNs, "room")!!.draft)
            val newNs = Isolation.namespace(binding.profile, binding.deployment, "owner", false, "1", "personal", pair.deviceId)
            assertNotEquals(oldNs, newNs)
            database.messages().enqueue(original.copy(key = "$newNs|fresh", namespace = newNs, id = "fresh", clientMsgId = "fresh"))
            assertEquals("pending", database.messages().one(newNs, "fresh")!!.state)
            assertNull(markers.intent(binding.profile))
            assertTrue(Strings.complete())
        } finally {
            coordinator.forgetMemory(); vault.clear(binding.profile); markers.clearAfterSecureCommit(binding.profile)
            database.close(); context.deleteDatabase(name)
        }
    }

    @Test
    fun initialUnknownChainKeepsOriginalIdsAndRejectsLateDelivery() = runBlocking {
        val binding = binding()
        val name = "JobsIM-test-native-session-${UUID.randomUUID()}.db"
        val database = ChatDatabase.open(context, false, name)
        val markers = NonSecretSessionState(context)
        val vault = TokenVault(context)
        val coordinator = SessionCoordinator(vault, markers, { database.messages() }, { _, _ -> error("no HTTP permitted") }, { "owner" })
        try {
            val first = InitialAuthIntent(UUID.randomUUID().toString(), "register", "owner", createdAt = 1)
            coordinator.prepareInitial(binding, first, "owner")
            // Local elapsed time alone cannot retire an unbuilt registration.
            assertFalse(markers.intent(binding.profile)!!.deliveryExpired)
            val successor = InitialAuthIntent(UUID.randomUUID().toString(), "reauthenticate", "owner", previousRequestId = first.requestId,
                previousRequestIds = markers.unresolvedRequestIds(binding.profile))
            coordinator.prepareInitial(binding, successor, "owner")
            assertTrue(runCatching { coordinator.commitInitial(delivered(binding), first.requestId, "owner") }.exceptionOrNull() is CancellationException)
            assertNull(vault.loadSession(binding))
            val recovered = NonSecretSessionState(context).intent(binding.profile)!!
            assertEquals(successor, recovered)
            assertEquals(listOf(first.requestId, successor.requestId), markers.unresolvedRequestIds(binding.profile))
            val completion = InitialAuthIntent(UUID.randomUUID().toString(), "reauthenticate", "owner",
                previousRequestId = successor.requestId, previousRequestIds = markers.unresolvedRequestIds(binding.profile))
            coordinator.prepareInitial(binding, completion, "owner")
            assertEquals(listOf(first.requestId, successor.requestId), markers.intent(binding.profile)!!.previousRequestIds)
            assertTrue(runCatching { coordinator.prepareInitial(binding, InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner"), "owner") }.isFailure)
            markers.expired(binding.profile)
            assertTrue(markers.intent(binding.profile)!!.deliveryExpired)
            assertEquals(completion.requestId, markers.intent(binding.profile)!!.requestId)
        } finally {
            coordinator.forgetMemory(); vault.clear(binding.profile); markers.clearAfterSecureCommit(binding.profile)
            database.close(); context.deleteDatabase(name)
        }
    }

    @Test
    fun lateExpiredBusinessCannotBorrowReplacementFamilyAndOther401NeverRefreshes() = runBlocking {
        val binding = binding()
        val name = "JobsIM-test-native-session-${UUID.randomUUID()}.db"
        val database = ChatDatabase.open(context, false, name)
        val markers = NonSecretSessionState(context)
        val vault = TokenVault(context)
        var owner = "original-owner"
        val entered = CompletableDeferred<Unit>()
        val release = CompletableDeferred<Unit>()
        val businessCalls = AtomicInteger()
        val refreshCalls = AtomicInteger()
        val coordinator = SessionCoordinator(vault, markers, { database.messages() }, { _, _ ->
            refreshCalls.incrementAndGet(); error("late request must not refresh")
        }, { owner })
        try {
            val first = InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner")
            coordinator.prepareInitial(binding, first, owner)
            coordinator.commitInitial(delivered(binding), first.requestId, owner)
            val api = SessionApi(binding, coordinator, { _ -> object : Api(binding.origin) {
                override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                    businessCalls.incrementAndGet(); entered.complete(Unit); release.await()
                    throw ApiFailure(401, "ACCESS_EXPIRED")
                }
            } }, { error("old callback must not reach the replacement screen") }, null)
            val old = async { runCatching { api.request("/messages", "POST", JSONObject().put("clientMsgId", "old-intent")) } }
            entered.await()
            owner = "replacement-owner"
            val next = InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner")
            coordinator.prepareInitial(binding, next, owner)
            coordinator.commitInitial(delivered(binding, device = "replacement"), next.requestId, owner)
            release.complete(Unit)
            assertTrue(old.await().exceptionOrNull() is CancellationException)
            assertEquals(1, businessCalls.get())
            assertEquals(0, refreshCalls.get())
            assertEquals("replacement", vault.loadSession(binding)!!.deviceId)
            val invalid = SessionApi(binding, coordinator, { _ -> object : Api(binding.origin) {
                override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject =
                    throw ApiFailure(401, "ACCESS_INVALID")
            } }, { }, null)
            assertTrue(runCatching { invalid.request("/me") }.exceptionOrNull() is SessionVerificationRequired)
            assertEquals(0, refreshCalls.get())
            assertEquals("ACCESS_INVALID", markers.blockedReason(binding.profile))
        } finally {
            release.complete(Unit)
            coordinator.forgetMemory(); vault.clear(binding.profile); markers.clearAfterSecureCommit(binding.profile)
            database.close(); context.deleteDatabase(name)
        }
    }

    @Test
    fun lateEpochInvalidationAndLegacyCommitCannotEraseRecoveredPair() = runBlocking {
        val oldBinding = binding()
        val newBinding = oldBinding.copy(epoch = "2")
        val name = "JobsIM-test-native-session-${UUID.randomUUID()}.db"
        val db = ChatDatabase.open(context, false, name)
        val vault = TokenVault(context)
        val markers = NonSecretSessionState(context)
        val old = SessionCoordinator(vault, markers, { db.messages() }, { _, _ -> error("no HTTP") }, { "old-model" })
        val current = SessionCoordinator(vault, markers, { db.messages() }, { _, _ -> error("no HTTP") }, { "new-model" })
        try {
            val first = InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner")
            old.prepareInitial(oldBinding, first, "old-model")
            old.commitInitial(delivered(oldBinding), first.requestId, "old-model")
            val next = InitialAuthIntent(UUID.randomUUID().toString(), "login", "owner")
            current.prepareInitial(newBinding, next, "new-model")
            val recovered = current.commitInitial(delivered(newBinding, device = "recovered-device"), next.requestId, "new-model")
            val ns = Isolation.namespace(newBinding.profile, newBinding.deployment, "owner", false, "2", "personal", recovered.deviceId)
            val row = MessageRow("$ns|new", ns, "room", "new", clientMsgId = "new", senderId = "owner", text = "new epoch", state = "pending", restoreEpoch = "2")
            db.messages().enqueue(row)
            old.invalidateEpoch(oldBinding, "old-legacy-access", "old-model")
            assertTrue(runCatching { old.commitLegacy(oldBinding, "late-legacy-access", "old-model") }.exceptionOrNull() is CancellationException)
            old.clearLegacy(oldBinding, "old-legacy-access")
            old.forgetMemory()
            assertEquals("2", vault.loadSession(newBinding)!!.binding.epoch)
            assertEquals("recovered-device", vault.loadSession(newBinding)!!.deviceId)
            assertEquals("pending", db.messages().one(ns, "new")!!.state)
            assertEquals(ns, db.messages().nativeScope(ns)!!.namespace)
            assertEquals("recovered-device", current.requireCurrent(current.capture()!!).deviceId)
        } finally {
            old.forgetMemory(); current.forgetMemory(); vault.clear(oldBinding.profile); markers.clearAfterSecureCommit(oldBinding.profile)
            db.close(); context.deleteDatabase(name)
        }
    }
}
