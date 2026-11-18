package com.jobs.im

import android.app.Application
import android.content.Intent
import android.graphics.Bitmap
import android.util.Base64
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.io.IOException
import java.util.UUID
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NativeSelfStateHttpsTest {
    private val instrumentation get() = InstrumentationRegistry.getInstrumentation()
    private val context get() = instrumentation.targetContext
    private fun uuid() = UUID.randomUUID().toString()

    private fun accountFact(username: String, requestId: String = "", delivery: JSONObject? = null, closed: Boolean = false) {
        val args = InstrumentationRegistry.getArguments()
        val run = SyncWire.id(args.getString("nativeSelfRun"))
        val directory = File(context.filesDir, "NativeSelfAccounts").apply { mkdirs() }
        val file = android.util.AtomicFile(File(directory, "$run.json"))
        val all = if (file.baseFile.isFile) JSONArray(file.openRead().use { it.readBytes().toString(Charsets.UTF_8) }) else JSONArray()
        var index = (0 until all.length()).firstOrNull { all.getJSONObject(it).opt("username") == username } ?: all.length()
        val fact = all.optJSONObject(index) ?: JSONObject().put("runId", run).put("username", username)
            .put("deploymentId", args.getString("deploymentId")).put("restoreEpoch", args.getString("restoreEpoch"))
        if (requestId.isNotBlank()) fact.put("requestId", requestId)
        delivery?.let {
            fact.put("userId", it.getJSONObject("user").getString("id")).put("familyId", it.getString("familyId"))
                .put("deviceId", it.getString("deviceId"))
        }
        fact.put("closed", closed)
        all.put(index, fact)
        val stream = file.startWrite()
        try { stream.write(all.toString().toByteArray()); stream.fd.sync(); file.finishWrite(stream) }
        catch (error: Exception) { file.failWrite(stream); throw error }
    }

    private class Live(val base: String, val ca: ByteArray, val bootstrap: JSONObject, val alice: JSONObject, val bob: JSONObject, val registrationId: String) {
        val a get() = Api(base, alice.getString("token"), ca)
        val b get() = Api(base, bob.getString("token"), ca)
        val epoch get() = bootstrap.getString("restoreEpoch")
        val deployment get() = bootstrap.getString("deploymentId")
        suspend fun group(title: String): JSONObject = a.request("/conversations", "POST", JSONObject().put("kind", "group")
            .put("title", title).put("memberIds", JSONArray(listOf(bob.getJSONObject("user").getString("id")))))
        suspend fun peerMessage(conversation: String, text: String): JSONObject {
            val destination = b.request("/conversations/$conversation")
            return b.request("/conversations/$conversation/messages", "POST", JSONObject().put("type", "text").put("text", text)
                .put("clientMsgId", UUID.randomUUID().toString()).put("restoreEpoch", epoch).put("sendAdmission", destination.getString("sendAdmission")))
        }
    }

    private suspend fun fixture(): Live {
        val args = InstrumentationRegistry.getArguments()
        assertEquals("An explicit new native self-state scope is required", "true", args.getString("nativeSelf"))
        val base = Isolation.endpoint(args.getString("liveUrl") ?: error("New URL required"))
        val ca = Base64.decode(args.getString("caBase64") ?: error("New public CA required"), Base64.DEFAULT)
        val api = Api(base, ca = ca)
        val boot = api.request("/bootstrap")
        assertEquals(args.getString("deploymentId"), boot.getString("deploymentId"))
        assertEquals(args.getString("restoreEpoch"), boot.getString("restoreEpoch"))
        for (flag in listOf("authoritative_sync", "sync_jetstream", "sync_self_state", "cloud_drafts", "auth_refresh"))
            assertTrue("Required native capability absent: $flag", boot.getJSONObject("features").getBoolean(flag))
        assertTrue("CA trust must remain instance-specific", runCatching { Api(base).request("/bootstrap") }.isFailure)
        val suffix = uuid().replace("-", "").take(12)
        val password = "Native S2 fixture " + uuid()
        val requestId = uuid()
        suspend fun register(name: String, nonce: String): JSONObject {
            accountFact(name, nonce)
            val result = api.request("/auth/register", "POST", JSONObject().put("username", name).put("password", password)
                .put("requestId", nonce).put("deviceName", "Android S2 verification"))
            accountFact(name, nonce, result)
            return result
        }
        val alice = register("s2_android_a_$suffix", requestId)
        val bob = try { register("s2_android_b_$suffix", uuid()) } catch (error: Exception) {
            runCatching { Api(base, alice.getString("token"), ca).request("/auth/logout", "POST", JSONObject()) }
            throw error
        }
        val f = Live(base, ca, boot, alice, bob, requestId)
        try {
            val request = f.a.request("/contacts/requests", "POST", JSONObject().put("userId", bob.getJSONObject("user").getString("id")).put("requestId", uuid()))
            f.b.request("/contacts/requests/${request.getString("id")}/accept", "POST", JSONObject())
        } catch (error: Exception) { close(f); throw error }
        return f
    }

    private suspend fun close(f: Live) {
        for ((api, delivery) in listOf(f.a to f.alice, f.b to f.bob)) {
            api.request("/auth/logout", "POST", JSONObject())
            try { api.request("/me"); fail("Owned session must be closed") }
            catch (expected: ApiFailure) { assertEquals(401, expected.status) }
            accountFact(delivery.getJSONObject("user").getString("username"), delivery = delivery, closed = true)
        }
    }

    private fun identity(f: Live, profile: String): SyncIdentity {
        val pair = NativeSession.delivery(f.alice, SessionBinding(profile, f.base, f.deployment, f.epoch))
        return SyncIdentity(Isolation.namespace(profile, f.deployment, pair.userId, false, f.epoch, "personal", pair.deviceId), profile,
            f.deployment, pair.userId, f.epoch, pair.deviceId, pair.familyId)
    }

    private suspend fun start(database: ChatDatabase, identity: SyncIdentity) {
        database.messages().putNativeScope(NativeScope(uuid(), "live|${identity.profile}|${identity.deployment}|", identity.namespace, identity.device, "owned HTTPS fixture"))
    }

    @Test fun realSchemaTwoReadViewPreferencesAndAtomicOriginalQueue(): Unit = runBlocking {
        val f = fixture()
        val name = "JobsIM-test-self-https-${uuid()}.db"
        val database = ChatDatabase.open(context, false, name)
        try {
            val group = f.group("Android S2 atomic fixture")
            val conversation = group.getString("id")
            val first = f.peerMessage(conversation, "Actually observed message")
            f.peerMessage(conversation, "Unobserved eligible hole")
            val identity = identity(f, uuid())
            start(database, identity)
            val destination = f.a.request("/conversations/$conversation")
            val original = MessageRow("${identity.namespace}|local-original", identity.namespace, conversation, "local-original",
                first.getString("clientMsgId"), identity.user, text = "Distinct unsent original body", state = "pending",
                restoreEpoch = f.epoch, membershipGeneration = destination.getString("membershipGeneration"), sendAdmission = destination.getString("sendAdmission"))
            database.messages().put(original)
            val engine = NativeSyncEngine(database)
            val installed = engine.run(identity, f.a, null, { true })
            val control = SyncStore(database).ensure(identity) { true }
            assertEquals("2", control.getString("schemaVersion"))
            assertEquals(installed.position, control.getString("position"))
            assertTrue(control.has("cursor")); assertFalse(control.has("restriction"))
            assertTrue(database.messages().list(identity.namespace, conversation).any { it.id == first.getString("id") })
            assertTrue("Original queue must remain byte-equivalent", original == database.messages().outbox(identity.namespace).single())
            val self = SelfStateModel(database, { identity }, { f.a }) { }
            self.activate(true, true); self.refresh(conversation)
            assertEquals("2", self.reads.getValue(conversation).getString("unreadCount"))
            self.changeView(conversation, JSONObject().put("manualUnread", true).put("pinnedRank", 0).put("archived", true))
            self.observe(com.jobs.im.conversation(destination, "fixture"), listOf(first.getString("id")))
            assertEquals("1", self.reads.getValue(conversation).getString("unreadCount"))
            assertTrue(self.views.getValue(conversation).getBoolean("manualUnread"))
            assertTrue(self.views.getValue(conversation).getBoolean("archived"))
            self.setFont("large")
            assertEquals("large", f.a.request("/me/preferences").getString("fontPreset"))
            engine.run(identity, f.a, null, { true })
            assertTrue("Original queue must remain byte-equivalent", original == database.messages().outbox(identity.namespace).single())
            val read = f.a.request("/conversations/$conversation/read")
            assertEquals("exact_current_source", read.getString("countStatus"))
            SyncWire.counter(read.opt("countSourceWatermark"))
        } finally { database.close(); context.deleteDatabase(name); close(f) }
    }

    @Test fun realDraftLossUsesReadonlyOriginalBodyAndRetiredSourceKeepsIndependentText(): Unit = runBlocking {
        val f = fixture()
        val name = "JobsIM-test-draft-https-${uuid()}.db"
        var database = ChatDatabase.open(context, false, name)
        try {
            val group = f.group("Android cloud draft loss fixture").getString("id")
            val source = f.peerMessage(group, "Referenced source without copied quote")
            val identity = identity(f, uuid()); start(database, identity)
            NativeSyncEngine(database).run(identity, f.a, null, { true })
            var puts = 0
            val results = mutableListOf<JSONObject>()
            val dropping = object : Api(f.base, f.alice.getString("token"), f.ca) {
                override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                    val response = super.request(path, method, data)
                    if (path == "/conversations/$group/draft" && method == "PUT") { puts++; throw IOException("Controlled loss after real server response") }
                    if (path == "/conversations/$group/draft-results") results.add(JSONObject(data!!.toString()))
                    return response
                }
            }
            val model = SelfStateModel(database, { identity }, { dropping }) { }
            model.activate(true, true)
            try { model.saveDraft(group, "Original independently owned text", message(source, identity.namespace, f.epoch)); fail("Controlled response loss required") }
            catch (_: IOException) { }
            val original = SelfStateStore(database).pending(identity).single()
            val originalBody = original.body.toString()
            database.close(); database = ChatDatabase.open(context, false, name)
            val reopened = SelfStateModel(database, { identity }, { dropping }) { }
            reopened.activate(true, true)
            assertTrue("Cold recovery must retain the exact original body", originalBody == reopened.pending.single().body.toString())
            reopened.recover(reopened.pending.single())
            assertEquals(1, puts)
            assertEquals("PUT", results.single().getString("method"))
            assertTrue(SyncWire.same(JSONObject(originalBody), results.single().getJSONObject("body")))
            assertEquals("Original independently owned text", f.a.request("/conversations/$group/draft").getString("ownedText"))
            val retiredGroup = f.group("Android draft retired fixture").getString("id")
            val retiringSource = f.peerMessage(retiredGroup, "Source before deliberate real edit")
            val editing = object : Api(f.base, f.alice.getString("token"), f.ca) {
                override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
                    if (path == "/conversations/$retiredGroup/draft" && method == "PUT") {
                        val fresh = f.b.request("/messages/${retiringSource.getString("id")}")
                        f.b.request("/messages/${retiringSource.getString("id")}", "PATCH", JSONObject().put("text", "Source deliberately edited")
                            .put("expectedVersion", fresh.getString("version")))
                    }
                    return super.request(path, method, data)
                }
            }
            val retiring = SelfStateModel(database, { identity }, { editing }) { }
            retiring.activate(true, true)
            retiring.saveDraft(retiredGroup, "Retired but independently owned text", message(retiringSource, identity.namespace, f.epoch))
            assertEquals("DRAFT_RETIRED", retiring.problem)
            assertEquals("retired", JSONObject(database.syncRecords().rows(identity.namespace, "self-history").last { JSONObject(it.payload).opt("status") == "retired" }.payload).getString("status"))
            assertEquals("Retired but independently owned text", retiring.independentDraftText(retiredGroup))
            assertTrue(retiring.pending.isEmpty())
            assertTrue(database.messages().outbox(identity.namespace).isEmpty())
        } finally { database.close(); context.deleteDatabase(name); close(f) }
    }

    @Test fun nativeSelfStateAndCloudDraftControlsRenderAndOperateThroughHttps(): Unit = runBlocking {
        val f = fixture()
        val app = context.applicationContext as Application
        val preferences = app.getSharedPreferences("JobsIM.settings", 0)
        val keys = listOf("profile", "url", "deployment", "restoreEpoch", "ca", "language", "theme", "font", "alerts")
        val previous = keys.associateWith { preferences.all[it] }
        val profile = uuid(); val binding = SessionBinding(profile, f.base, f.deployment, f.epoch)
        val pair = NativeSession.delivery(f.alice, binding)
        val vault = TokenVault(app); val markers = NonSecretSessionState(app)
        val database = ChatDatabase.open(app, false)
        var activity: MainActivity? = null
        lateinit var model: ChatModel
        fun step(name: String) {
            val run = SyncWire.id(InstrumentationRegistry.getArguments().getString("nativeSelfRun"))
            val folder = File(context.filesDir, "NativeSelfSteps").apply { mkdirs() }
            val file = File(folder, "$run.jsonl")
            val code = model.errorCode.takeIf { it.matches(Regex("[A-Z0-9_]{1,96}")) }.orEmpty()
            file.appendText(JSONObject().put("stage", name).put("busy", model.busy).put("selected", model.selected?.id)
                .put("selfEnabled", model.selfState.enabled).put("errorCode", code).toString() + "\n")
        }
        fun screenshot(name: String) {
            instrumentation.waitForIdleSync()
            val bitmap = instrumentation.uiAutomation.takeScreenshot()
            val run = SyncWire.id(InstrumentationRegistry.getArguments().getString("nativeSelfRun"))
            val dir = File(app.getExternalFilesDir(null), "SelfStateUiEvidence/$run").apply { mkdirs() }
            File(dir, name).outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
            bitmap.recycle()
        }
        suspend fun waitFor(stage: String, predicate: () -> Boolean) {
            try { withTimeout(60000) { while (!predicate()) { delay(100) } } }
            catch (_: TimeoutCancellationException) {
                runCatching { screenshot("99-Failed-" + stage.replace(' ', '-') + ".png") }
                val code = model.errorCode.takeIf { it.matches(Regex("[A-Z0-9_]{1,96}")) } ?: "NO_PROTOCOL_ERROR"
                throw AssertionError("Native S2 UI $stage timed out: $code")
            }
        }
        val ui = NativeForwardActions()
        suspend fun press(label: String) {
            waitFor("button idle") { !model.busy }
            repeat(100) { attempt ->
                for (candidate in ui.nodes().filter { it.isVisibleToUser && (it.text?.toString() == label || it.contentDescription?.toString() == label) }) {
                    var button: android.view.accessibility.AccessibilityNodeInfo? = candidate
                    while (button != null && !button.isClickable) button = button.parent
                    // A disabled control is not permission to click its scrim or navigation ancestor.
                    if (button == null || !button.isEnabled || model.busy) continue
                    if (button.performAction(android.view.accessibility.AccessibilityNodeInfo.ACTION_CLICK) || ui.tap(button)) {
                        instrumentation.waitForIdleSync()
                        delay(250)
                        step("pressed:" + label)
                        return
                    }
                }
                if (attempt % 6 == 5) ui.scroll(true)
                delay(100)
            }
            screenshot("99-Button-Unavailable.png")
            throw AssertionError("Enabled native control unavailable: $label")
        }
        try {
            val group = f.group("Native S2 UI conversation").getString("id")
            f.peerMessage(group, "Visible native S2 message")
            preferences.edit().putString("profile", profile).putString("url", f.base).putString("deployment", f.deployment)
                .putString("restoreEpoch", f.epoch).putString("ca", Base64.encodeToString(f.ca, Base64.NO_WRAP))
                .putString("language", "en").putString("theme", "light").putFloat("font", 1f).putBoolean("alerts", false).commit()
            val coordinator = SessionCoordinator(vault, markers, { database.messages() }, { b, token -> Api(b.origin, token, f.ca) }, { "owned-native-s2-ui" })
            coordinator.prepareInitial(binding, InitialAuthIntent(f.registrationId, "register", pair.username), "owned-native-s2-ui")
            coordinator.commitInitial(pair, f.registrationId, "owned-native-s2-ui")
            activity = instrumentation.startActivitySync(Intent(app, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)) as MainActivity
            withContext(Dispatchers.Main) {
                // Use the production Activity owner; disposing a second model would retire its shared session.
                model = androidx.lifecycle.ViewModelProvider(activity!!)[ChatModel::class.java]
            }
            waitFor("ready") { model.selfState.enabled && model.conversations.any { it.id == group } && !model.busy }
            press("Native S2 UI conversation")
            ui.waitText("Visible native S2 message")
            waitFor("selected chat") { model.selected?.id == group && !model.busy }
            press("My synced state")
            waitFor("state reload") { !model.busy && model.selfState.views[group] != null }
            press("Mark unread")
            withTimeout(15000) {
                while (!f.a.request("/conversations/$group/view").getBoolean("manualUnread")) delay(200)
            }
            step("server-manual-unread-confirmed")
            waitFor("manual unread") { model.selfState.views[group]?.opt("manualUnread") == true && !model.busy }
            screenshot("01-S2-State-Light.png")
            press("Close")
            ui.edit("Write a message", "Independent native cloud draft")
            press("My synced state")
            waitFor("draft reload") { !model.busy && model.selfState.drafts[group] != null }
            press("Save cloud draft")
            waitFor("cloud save") { model.selfState.drafts[group]?.opt("ownedText") == "Independent native cloud draft" && !model.busy }
            screenshot("02-S2-Cloud-Draft.png")
            press("Close")
            ui.edit("Write a message", "New local input retained until explicit load")
            press("My synced state")
            press("Load cloud draft into editor")
            waitFor("explicit load") { model.draft == "Independent native cloud draft" && !model.busy }
            press("Close")
            screenshot("03-S2-Loaded-Draft.png")
            press("Back"); press("Settings"); press("Large")
            waitFor("font preference") { model.font == 1.2f && !model.busy }
            assertEquals("large", f.a.request("/me/preferences").getString("fontPreset"))
            screenshot("04-S2-Large-Settings.png")
            press("Dark"); delay(500)
            screenshot("05-S2-Dark-Settings.png")
            assertTrue(f.a.request("/conversations/$group/view").getBoolean("manualUnread"))
        } catch (error: Throwable) {
            runCatching { screenshot("99-Failed-UI.png"); step("failed") }
            throw error
        } finally {
            withContext(Dispatchers.Main) { activity?.finish() }
            delay(300)
            vault.clear(profile); markers.clearAfterSecureCommit(profile)
            val edit = preferences.edit()
            for ((key, value) in previous) when (value) {
                null -> edit.remove(key)
                is String -> edit.putString(key, value)
                is Float -> edit.putFloat(key, value)
                is Int -> edit.putInt(key, value)
                is Boolean -> edit.putBoolean(key, value)
                is Long -> edit.putLong(key, value)
            }
            check(edit.commit())
            database.close()
            close(f)
        }
    }
}
