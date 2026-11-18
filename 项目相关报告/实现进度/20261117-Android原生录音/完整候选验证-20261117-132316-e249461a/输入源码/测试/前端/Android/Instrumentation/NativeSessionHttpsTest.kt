package com.jobs.im

import android.app.Application
import android.content.Intent
import android.util.Base64
import androidx.activity.compose.setContent
import androidx.lifecycle.ViewModelStore
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.util.UUID
import java.util.concurrent.atomic.AtomicInteger
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NativeSessionHttpsTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private class Fixture(val base: String, val ca: ByteArray, val boot: JSONObject) {
        val anonymous get() = Api(base, ca = ca)
    }
    private suspend fun fixture(): Fixture {
        val args = InstrumentationRegistry.getArguments()
        assumeTrue("Explicit native refresh scope is required", args.getString("nativeRefresh") == "true")
        val base = args.getString("liveUrl")
        val ca = args.getString("caBase64")
        assumeTrue("Fresh HTTPS instance and CA are required", !base.isNullOrBlank() && !ca.isNullOrBlank())
        val bytes = Base64.decode(ca, Base64.DEFAULT)
        val boot = Api(base!!, ca = bytes).request("/bootstrap")
        assertEquals(args.getString("deploymentId"), boot.getString("deploymentId"))
        assertTrue(boot.getJSONObject("features").getBoolean("auth_refresh"))
        assertTrue(boot.getJSONObject("features").getBoolean("auth_retirement_chain"))
        return Fixture(base, bytes, boot)
    }
    private suspend fun register(f: Fixture, username: String, password: String) =
        f.anonymous.request("/auth/register", "POST", JSONObject().put("username", username)
            .put("password", password).put("requestId", UUID.randomUUID().toString())
            .put("deviceName", "Android refresh verification"))

    private suspend fun expireOnlyThisAccess(f: Fixture, session: NativeSession) {
        // The host harness changes this owned test family's access deadline only.
        val id = UUID.randomUUID().toString()
        val request = File(context.filesDir, "native-auth-clock.json")
        val receipt = File(context.filesDir, "native-auth-clock-$id.done")
        android.util.AtomicFile(request).let { atomic ->
            val stream = atomic.startWrite()
            try {
                stream.write(JSONObject().put("runId", InstrumentationRegistry.getArguments().getString("nativeRefreshRun"))
                    .put("requestId", id).put("familyId", session.familyId)
                    .put("userId", session.userId).put("deploymentId", f.boot.getString("deploymentId"))
                    .put("restoreEpoch", session.binding.epoch).put("refreshGeneration", session.refreshGeneration).toString().toByteArray())
                stream.fd.sync(); atomic.finishWrite(stream)
            } catch (error: Exception) { atomic.failWrite(stream); throw error }
        }
        try {
            withTimeout(120000) { while (!receipt.exists() || receipt.readText().trim() != id) delay(100) }
            assertEquals(id, receipt.readText().trim())
        } finally { request.delete(); receipt.delete() }
    }

    @Test
    fun realExpiredAccessSingleFlightPreservesOriginalSendAndSecurePair(): Unit = runBlocking {
        val f = fixture()
        val suffix = UUID.randomUUID().toString().replace("-", "").take(12)
        val password = "Android access fixture " + UUID.randomUUID()
        val initial = register(f, "rt_android_$suffix", password)
        val peer = register(f, "rt_peer_$suffix", password)
        val binding = SessionBinding(UUID.randomUUID().toString(), f.base,
            f.boot.getString("deploymentId"), f.boot.getString("restoreEpoch"))
        val pair = NativeSession.delivery(initial, binding)
        val vault = TokenVault(context)
        val markers = NonSecretSessionState(context)
        val name = "JobsIM-test-native-https-${UUID.randomUUID()}.db"
        val db = ChatDatabase.open(context, false, name)
        val rotations = AtomicInteger()
        val expired = AtomicInteger()
        val coordinator = SessionCoordinator(vault, markers, { db.messages() }, { _, token ->
            object : Api(f.base, token, f.ca) {
                override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                    if (path == "/auth/refresh") { assertTrue(token.isEmpty()); rotations.incrementAndGet() }
                    return super.request(path, method, data)
                }
            }
        }, { "same-native-device" })
        val peerApi = Api(f.base, peer.getString("token"), f.ca)
        try {
            val intent = InitialAuthIntent(UUID.randomUUID().toString(), "login", pair.username)
            coordinator.prepareInitial(binding, intent, "same-native-device")
            coordinator.commitInitial(pair, intent.requestId, "same-native-device")
            val api = SessionApi(binding, coordinator, { token -> object : Api(f.base, token, f.ca) {
                override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                    try { return super.request(path, method, data) }
                    catch (failure: ApiFailure) { if (failure.code == "ACCESS_EXPIRED") expired.incrementAndGet(); throw failure }
                }
            } }, { error("unexpected session verification") }, f.ca)
            val peerId = peer.getJSONObject("user").getString("id")
            val request = api.request("/contacts/requests", "POST", JSONObject().put("userId", peerId).put("requestId", UUID.randomUUID()))
            peerApi.request("/contacts/requests/${request.getString("id")}/accept", "POST", JSONObject())
            val conversation = api.request("/conversations", "POST", JSONObject().put("kind", "group")
                .put("title", "Native refresh outbox").put("memberIds", JSONArray(listOf(peerId))))
            val conversationId = conversation.getString("id")
            val source = api.request("/conversations/$conversationId")
            val ns = Isolation.namespace(binding.profile, binding.deployment, pair.userId, false, binding.epoch, "personal", pair.deviceId)
            val clientId = UUID.randomUUID().toString()
            val payload = JSONObject().put("clientMsgId", clientId).put("type", "text").put("text", "Original native refresh intent")
                .put("restoreEpoch", binding.epoch).put("sendAdmission", source.getString("sendAdmission"))
            val row = MessageRow("$ns|$clientId", ns, conversationId, clientId, clientId, pair.userId,
                text = payload.getString("text"), state = "pending", restoreEpoch = binding.epoch,
                membershipGeneration = source.getString("membershipGeneration"), sendAdmission = source.getString("sendAdmission"))
            db.messages().enqueue(row)
            expireOnlyThisAccess(f, pair)
            val sent = coroutineScope {
                val write = async { api.request("/conversations/$conversationId/messages", "POST", payload) }
                val readers = (1..5).map { async { api.request("/me") } }
                readers.awaitAll(); write.await()
            }
            assertTrue(expired.get() > 0)
            assertEquals(1, rotations.get())
            val committed = vault.loadSession(binding)!!
            assertEquals(pair.familyId, committed.familyId)
            assertEquals(pair.deviceId, committed.deviceId)
            val expectedGeneration = (java.math.BigInteger(pair.refreshGeneration) + java.math.BigInteger.ONE).toString()
            assertEquals(expectedGeneration, committed.refreshGeneration)
            assertEquals(pair.absoluteExpiresAt, committed.absoluteExpiresAt)
            assertTrue(row == db.messages().one(ns, clientId))
            assertEquals(ns, db.messages().nativeScope(ns)!!.namespace)
            assertEquals(clientId, sent.getString("clientMsgId"))
            assertEquals(pair.userId, api.request("/auth/context").getJSONObject("user").getString("id"))
            coordinator.forgetMemory()
            assertEquals(expectedGeneration, coordinator.restore(binding, "same-native-device")!!.refreshGeneration)
            api.request("/auth/logout", "POST", JSONObject())
        } finally {
            coordinator.forgetMemory(); vault.clear(binding.profile); markers.clearAfterSecureCommit(binding.profile)
            runCatching { peerApi.request("/auth/logout", "POST", JSONObject()) }
            db.close(); context.deleteDatabase(name)
        }
    }

    @Test
    fun realLostRefreshAndThreeIntentClosureRetireUnknownAncestors(): Unit = runBlocking {
        val f = fixture()
        val suffix = UUID.randomUUID().toString().replace("-", "").take(12)
        val username = "rt_chain_$suffix"
        val password = "Android closure fixture " + UUID.randomUUID()
        val initial = register(f, username, password)
        val binding = SessionBinding(UUID.randomUUID().toString(), f.base, f.boot.getString("deploymentId"), f.boot.getString("restoreEpoch"))
        val pair = NativeSession.delivery(initial, binding)
        val vault = TokenVault(context)
        val markers = NonSecretSessionState(context)
        val name = "JobsIM-test-native-https-${UUID.randomUUID()}.db"
        val db = ChatDatabase.open(context, false, name)
        val rotations = AtomicInteger()
        val coordinator = SessionCoordinator(vault, markers, { db.messages() }, { _, token ->
            object : Api(f.base, token, f.ca) {
                override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                    val response = super.request(path, method, data)
                    if (path == "/auth/refresh") { rotations.incrementAndGet(); throw java.net.SocketTimeoutException("controlled response loss") }
                    return response
                }
            }
        }, { "owner" })
        try {
            val first = InitialAuthIntent(UUID.randomUUID().toString(), "login", username)
            coordinator.prepareInitial(binding, first, "owner")
            coordinator.commitInitial(pair, first.requestId, "owner")
            expireOnlyThisAccess(f, pair)
            val api = SessionApi(binding, coordinator, { Api(f.base, it, f.ca) }, { }, f.ca)
            assertTrue(runCatching { api.request("/me") }.exceptionOrNull() is SessionVerificationRequired)
            repeat(3) { assertTrue(runCatching { coordinator.refreshed(coordinator.capture()!!) }.exceptionOrNull() is SessionVerificationRequired) }
            assertEquals(1, rotations.get())
            coordinator.forgetMemory()
            assertTrue(runCatching { coordinator.restore(binding, "owner") }.exceptionOrNull() is SessionVerificationRequired)
            val recovery = InitialAuthIntent(UUID.randomUUID().toString(), "reauthenticate", username, pair.familyId)
            coordinator.prepareInitial(binding, recovery, "owner")
            val recovered = f.anonymous.request("/auth/reauthenticate", "POST", JSONObject().put("username", username).put("password", password)
                .put("previousFamilyId", pair.familyId).put("requestId", recovery.requestId).put("deviceName", "Android verified recovery"))
            coordinator.commitInitial(NativeSession.delivery(recovered, binding), recovery.requestId, "owner")
            Api(f.base, recovered.getString("token"), f.ca).request("/auth/logout", "POST", JSONObject())
            coordinator.forgetMemory()

            fun body(intent: InitialAuthIntent) = JSONObject().put("username", username).put("password", password)
                .put("requestId", intent.requestId).put("deviceName", "Android original chain")
                .also { if (intent.previousRequestId.isNotBlank()) it.put("previousRequestId", intent.previousRequestId)
                    if (intent.previousRequestIds.isNotEmpty()) it.put("previousRequestIds", JSONArray(intent.previousRequestIds)) }
            val root = InitialAuthIntent(UUID.randomUUID().toString(), "login", username)
            coordinator.prepareInitial(binding, root, "owner")
            val rootResult = f.anonymous.request("/auth/login", "POST", body(root))
            // R0 really committed but its delivery is treated as lost. R1 is saved, never sent.
            val r1 = InitialAuthIntent(UUID.randomUUID().toString(), "reauthenticate", username,
                previousRequestId = root.requestId, previousRequestIds = markers.unresolvedRequestIds(binding.profile))
            coordinator.prepareInitial(binding, r1, "owner")
            val originalR1 = body(r1).toString()
            val r2 = InitialAuthIntent(UUID.randomUUID().toString(), "reauthenticate", username,
                previousRequestId = r1.requestId, previousRequestIds = markers.unresolvedRequestIds(binding.profile))
            coordinator.prepareInitial(binding, r2, "owner")
            assertEquals(listOf(root.requestId, r1.requestId), r2.previousRequestIds)
            val result = f.anonymous.request("/auth/reauthenticate", "POST", body(r2))
            val current = coordinator.commitInitial(NativeSession.delivery(result, binding), r2.requestId, "owner")
            val old = runCatching { Api(f.base, rootResult.getString("token"), f.ca).request("/me") }.exceptionOrNull()
            assertTrue(old is ApiFailure && old.status == 401)
            val late = runCatching { f.anonymous.request("/auth/reauthenticate", "POST", JSONObject(originalR1)) }.exceptionOrNull()
            assertTrue(late is ApiFailure && late.status == 401 && late.code == "AUTHENTICATION_RETIRED")
            assertEquals(current.deviceId, Api(f.base, current.token, f.ca).request("/auth/context").getString("deviceId"))
            Api(f.base, current.token, f.ca).request("/auth/logout", "POST", JSONObject())
        } finally {
            coordinator.forgetMemory(); vault.clear(binding.profile); markers.clearAfterSecureCommit(binding.profile)
            db.close(); context.deleteDatabase(name)
        }
    }

    @Test
    fun nativeUnknownSignInCanVerifyAgainAndColdRestoreWithoutMovingDeviceScope(): Unit = runBlocking {
        val f = fixture()
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val app = context.applicationContext as Application
        val username = "rt_ui_" + UUID.randomUUID().toString().replace("-", "").take(12)
        val password = "Android UI reauthentication " + UUID.randomUUID()
        val registered = register(f, username, password)
        val binding = SessionBinding(UUID.randomUUID().toString(), f.base, f.boot.getString("deploymentId"), f.boot.getString("restoreEpoch"))
        app.getSharedPreferences("JobsIM.settings", 0).edit().putString("profile", binding.profile)
            .putString("url", f.base).putString("deployment", binding.deployment).putString("restoreEpoch", binding.epoch)
            .putString("ca", Base64.encodeToString(f.ca, Base64.NO_WRAP)).putString("language", "en").putString("theme", "light").commit()
        val vault = TokenVault(app)
        val markers = NonSecretSessionState(app)
        val firstHolder = ViewModelStore()
        val secondHolder = ViewModelStore()
        val thirdHolder = ViewModelStore()
        lateinit var model: ChatModel
        var lostToken = ""
        var activity: MainActivity? = null
        suspend fun awaitTheme(dark: Boolean) {
            withTimeout(20000) {
                while (true) {
                    val bitmap = instrumentation.uiAutomation.takeScreenshot()
                    val pixel = bitmap.getPixel(8, bitmap.height / 2)
                    val brightness = (android.graphics.Color.red(pixel) + android.graphics.Color.green(pixel) + android.graphics.Color.blue(pixel)) / 3
                    bitmap.recycle()
                    if (if (dark) brightness < 80 else brightness > 180) break
                    delay(100)
                }
            }
        }
        suspend fun awaitReady(stage: String, ready: () -> Boolean) {
            try {
                withTimeout(60000) {
                    while (!ready()) {
                        if (!model.busy && model.error.isNotBlank()) {
                            val code = model.errorCode.takeIf { it.matches(Regex("[A-Z0-9_]+")) } ?: "NON_PROTOCOL_ERROR"
                            throw AssertionError("Native auth $stage failed: $code")
                        }
                        delay(100)
                    }
                }
            } catch (_: TimeoutCancellationException) {
                val code = model.errorCode.takeIf { it.matches(Regex("[A-Z0-9_]+")) } ?: "NO_PROTOCOL_ERROR"
                throw AssertionError("Native auth $stage timed out; busy=${model.busy}, identityReady=${model.userId.isNotBlank()}, code=$code")
            }
        }
        try {
            withContext(Dispatchers.Main) {
                model = ChatModel(app) { base, credential, ca -> object : Api(base, credential, ca) {
                    override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                        val result = super.request(path, method, data)
                        if (path == "/auth/login") {
                            lostToken = result.getString("token")
                            throw java.net.SocketTimeoutException("controlled initial delivery loss")
                        }
                        return result
                    }
                } }
                firstHolder.put("auth", model)
            }
            withContext(Dispatchers.Main) { model.authenticate("login", username, password) }.join()
            assertTrue(model.userId.isEmpty())
            assertTrue(lostToken.isNotBlank())
            val originalId = markers.intent(binding.profile)!!.requestId
            withContext(Dispatchers.Main) {
                firstHolder.clear()
                model = ChatModel(app)
                secondHolder.put("auth", model)
            }
            withTimeout(10000) { while (model.pendingAuthentication == null) delay(100) }
            assertEquals(originalId, model.pendingAuthentication!!.requestId)
            activity = instrumentation.startActivitySync(Intent(app, MainActivity::class.java)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)) as MainActivity
            withContext(Dispatchers.Main) { activity!!.setContent { JobsIM(model) } }
            val ui = NativeForwardActions()
            ui.waitText("Retry original authentication")
            awaitTheme(false)
            ui.screenshot("Auth-01-Unknown-Light.png")
            withContext(Dispatchers.Main) { model.setting("theme", "dark") }
            awaitTheme(true)
            ui.screenshot("Auth-02-Unknown-Dark.png")
            ui.edit("Password", password)
            repeat(2) { ui.scroll(true) }
            ui.waitText("Verify again and end previous sign-in")
            ui.screenshot("Auth-02b-Unknown-Actions-Dark.png")
            ui.click("Verify again and end previous sign-in")
            awaitReady("reauthentication") { model.userId.isNotBlank() && !model.busy }
            assertEquals(registered.getJSONObject("user").getString("id"), model.userId)
            assertEquals("", model.error)
            assertNull(markers.intent(binding.profile))
            val pair = vault.loadSession(binding)!!
            val namespace = model.storageNamespace
            assertTrue(namespace.contains("|personal|${pair.deviceId}|"))
            val old = runCatching { Api(f.base, lostToken, f.ca).request("/me") }.exceptionOrNull()
            assertTrue(old is ApiFailure && old.status == 401)
            ui.waitText("Nothing here yet")
            ui.screenshot("Auth-03-Verified-Dark.png")
            withContext(Dispatchers.Main) {
                model.setting("theme", "light")
                secondHolder.clear()
                model = ChatModel(app)
                thirdHolder.put("auth", model)
                activity!!.setContent { JobsIM(model) }
            }
            awaitReady("cold-restore") { model.userId.isNotBlank() }
            assertEquals(namespace, model.storageNamespace)
            assertEquals(pair.deviceId, vault.loadSession(binding)!!.deviceId)
            ui.waitText("Nothing here yet")
            awaitTheme(false)
            ui.screenshot("Auth-04-Restored-Light.png")
            withContext(Dispatchers.Main) { model.logout() }.join()
            assertTrue(model.token.isBlank())
        } finally {
            runCatching { Api(f.base, registered.getString("token"), f.ca).request("/auth/logout", "POST", JSONObject()) }
            runCatching { vault.loadSession(binding)?.let { Api(f.base, it.token, f.ca).request("/auth/logout", "POST", JSONObject()) } }
            withContext(Dispatchers.Main) { activity?.finish(); firstHolder.clear(); secondHolder.clear(); thirdHolder.clear() }
            vault.clear(binding.profile); markers.clearAfterSecureCommit(binding.profile)
        }
    }
}
