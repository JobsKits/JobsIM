package com.jobs.im

import android.app.Application
import android.content.Intent
import android.util.Base64
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.UUID
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class ForwardUiTest {
    @Test
    fun nativeSourceSelectionAndExplicitOriginalRetryCreateRealReferences() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val args = InstrumentationRegistry.getArguments()
        val base = args.getString("liveUrl")
        val encoded = args.getString("caBase64")
        assumeTrue(!base.isNullOrBlank() && !encoded.isNullOrBlank())
        val ca = Base64.decode(encoded, Base64.DEFAULT)
        val anonymous = Api(base!!, ca = ca)
        val boot = anonymous.request("/bootstrap")
        assertEquals(args.getString("deploymentId"), boot.getString("deploymentId"))
        assertTrue(boot.getJSONObject("features").getBoolean("forwarding"))
        val suffix = UUID.randomUUID().toString().replace("-", "").take(10)
        val password = "Forward native UI fixture " + UUID.randomUUID()
        suspend fun account(name: String) =
            anonymous.request(
                "/auth/register",
                "POST",
                JSONObject()
                    .put("username", name)
                    .put("password", password)
                    .put("requestId", UUID.randomUUID())
                    .put("deviceName", "Forward native UI"),
            )
        val a = account("android_forward_ui_a_" + suffix)
        val b = account("android_forward_ui_b_" + suffix)
        val alice = Api(base, a.getString("token"), ca)
        val bob = Api(base, b.getString("token"), ca)
        val me = a.getJSONObject("user").getString("id")
        val peer = b.getJSONObject("user").getString("id")
        val app = instrumentation.targetContext.applicationContext as Application
        val ui = NativeForwardActions()
        try {
            val invitation =
                alice.request(
                    "/contacts/requests",
                    "POST",
                    JSONObject().put("userId", peer).put("requestId", UUID.randomUUID()),
                )
            bob.request(
                "/contacts/requests/${invitation.getString("id")}/accept",
                "POST",
                JSONObject(),
            )
            suspend fun group(title: String) =
                alice.request(
                    "/conversations",
                    "POST",
                    JSONObject()
                        .put("kind", "group")
                        .put("title", title)
                        .put("memberIds", JSONArray(listOf(peer))),
                )
            val sourceGroup = group("Native forward source")
            val target = group("Native forward target")
            val sourceConversation = conversationSnapshot(alice, sourceGroup.getString("id"))
            val source =
                alice.request(
                    "/conversations/${sourceGroup.getString("id")}/messages",
                    "POST",
                    JSONObject()
                        .put("clientMsgId", UUID.randomUUID())
                        .put("type", "text")
                        .put("text", "Original for native forwarding")
                        .put("restoreEpoch", boot.getString("restoreEpoch"))
                        .put("sendAdmission", sourceConversation.getString("sendAdmission")),
                )
            val profile = UUID.randomUUID().toString()
            app.getSharedPreferences("JobsIM.settings", 0)
                .edit()
                .putString("profile", profile)
                .putString("url", base)
                .putString("deployment", boot.getString("deploymentId"))
                .putString("restoreEpoch", boot.getString("restoreEpoch"))
                .putString("ca", encoded)
                .putString("language", "en")
                .putString("theme", "dark")
                .commit()
            TokenVault(app).save(profile, a.getString("token"))
            val ns =
                Isolation.namespace(
                    profile,
                    boot.getString("deploymentId"),
                    me,
                    false,
                    boot.getString("restoreEpoch"),
                )
            val sourceWithProof = alice.request("/messages/${source.getString("id")}")
            val targetSnapshot = conversationSnapshot(alice, target.getString("id"))
            val originalId = UUID.randomUUID().toString()
            val originalBody =
                JSONObject()
                    .put("clientMsgId", originalId)
                    .put("type", "forward")
                    .put("forwardOfId", source.getString("id"))
                    .put("forwardSourceVersion", source.getString("contentVersion"))
                    .put("forwardAdmission", sourceWithProof.getString("forwardAdmission"))
                    .put("restoreEpoch", boot.getString("restoreEpoch"))
                    .put("sendAdmission", targetSnapshot.getString("sendAdmission"))
            val database = ChatDatabase.open(app, false)
            try {
                database
                    .messages()
                    .enqueue(
                        MessageRow(
                            "$ns|$originalId",
                            ns,
                            target.getString("id"),
                            originalId,
                            originalId,
                            me,
                            type = "forward",
                            text = "",
                            state = "pending",
                            restoreEpoch = boot.getString("restoreEpoch"),
                            membershipGeneration = targetSnapshot.getString("membershipGeneration"),
                            sendAdmission = targetSnapshot.getString("sendAdmission"),
                            forwardPayload = originalBody.toString(),
                        )
                    )
            } finally {
                database.close()
            }
            val activity =
                instrumentation.startActivitySync(
                    Intent(app, MainActivity::class.java)
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
                )
            ui.click("Native forward source")
            ui.waitText("Original for native forwarding")
            ui.click("Original for native forwarding")
            ui.click("Forward")
            ui.waitText("Choose destination")
            ui.screenshot("01-Native-Forward-Selection-Dark.png")
            ui.click("Native forward target")
            ui.click("Forward")
            withTimeout(10000) {
                while (
                    alice
                        .request(
                            "/conversations/${target.getString("id")}/messages?after=0&limit=200"
                        )
                        .getJSONArray("items")
                        .length() != 1
                ) delay(100)
            }
            ui.click("Back")
            ui.click("Native forward target")
            ui.waitText("Forwarded message")
            ui.waitText("Check original result")
            ui.screenshot("02-Native-Forward-Unknown.png")
            ui.click("Check original result")
            ui.waitText("Retry original send")
            ui.screenshot("03-Native-Forward-Explicit-Retry.png")
            ui.click("Retry original send")
            withTimeout(10000) {
                while (
                    alice
                        .request(
                            "/conversations/${target.getString("id")}/messages?after=0&limit=200"
                        )
                        .getJSONArray("items")
                        .length() != 2
                ) delay(100)
            }
            val model =
                withContext(Dispatchers.Main) {
                    androidx.lifecycle
                        .ViewModelProvider(activity as MainActivity)[ChatModel::class.java]
                }
            withTimeout(10000) {
                while (
                    model.busy ||
                        model.messages.count {
                            it.forwarded && it.text == "Original for native forwarding"
                        } != 2
                ) delay(50)
            }
            ui.waitText("Original for native forwarding")
            ui.screenshot("04-Native-Forward-Delivered.png")
            val delivered =
                alice.request(
                    "/conversations/${target.getString("id")}/message-results",
                    "POST",
                    originalBody,
                )
            val result = delivered.optJSONObject("message") ?: delivered
            assertEquals(originalId, result.getString("clientMsgId"))
            assertTrue(result.getBoolean("forwarded"))
            assertEquals("Original for native forwarding", result.getString("text"))
            assertFalse(result.has("attachmentId"))
            assertTrue(
                alice
                    .request("/conversations/${target.getString("id")}/messages?after=0&limit=200")
                    .getJSONArray("items")
                    .let { items ->
                        (0 until items.length()).all {
                            items.getJSONObject(it).getBoolean("forwarded")
                        }
                    }
            )
        } finally {
            ui.screenshot("99-Forward-Before-Cleanup.png")
            alice.request("/auth/logout", "POST", JSONObject())
            bob.request("/auth/logout", "POST", JSONObject())
        }
    }
}
