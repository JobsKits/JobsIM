package com.jobs.im

import android.app.Application
import android.content.Intent
import android.graphics.Bitmap
import android.util.Base64
import android.view.accessibility.AccessibilityNodeInfo
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NativeUiTest {
    @Test
    fun realConversationGroupAndSettingsRenderInNativeActivity() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val args = InstrumentationRegistry.getArguments()
        val base = args.getString("liveUrl")
        val encoded = args.getString("caBase64")
        assumeTrue(!base.isNullOrBlank() && !encoded.isNullOrBlank())
        val ca = Base64.decode(encoded, Base64.DEFAULT)
        val anonymous = Api(base!!, ca = ca)
        val boot = anonymous.request("/bootstrap")
        val suffix = UUID.randomUUID().toString().replace("-", "").take(10)
        val password = "UI native " + UUID.randomUUID()
        suspend fun account(name: String) =
            anonymous.request(
                "/auth/register",
                "POST",
                JSONObject()
                    .put("username", name)
                    .put("password", password)
                    .put("requestId", UUID.randomUUID().toString())
                    .put("deviceName", "Native UI fixture"),
            )
        val a = account("android_ui_a_" + suffix)
        val b = account("android_ui_b_" + suffix)
        val alice = Api(base, a.getString("token"), ca)
        val bob = Api(base, b.getString("token"), ca)
        val me = a.getJSONObject("user").getString("id")
        val peer = b.getJSONObject("user").getString("id")
        val app = instrumentation.targetContext.applicationContext as Application
        fun find(text: String): AccessibilityNodeInfo? {
            fun walk(node: AccessibilityNodeInfo?): AccessibilityNodeInfo? {
                if (node == null) return null
                if (
                    node.isVisibleToUser &&
                        (node.text?.toString().orEmpty() +
                                node.contentDescription?.toString().orEmpty() +
                                node.hintText?.toString().orEmpty())
                            .contains(text)
                )
                    return node
                for (i in 0 until node.childCount) walk(node.getChild(i))?.let {
                    return it
                }
                return null
            }
            return walk(instrumentation.uiAutomation.rootInActiveWindow)
        }
        fun waitText(text: String): AccessibilityNodeInfo {
            repeat(80) {
                find(text)?.let {
                    return it
                }
                Thread.sleep(100)
            }
            error("Native UI text missing: $text")
        }
        fun click(text: String) {
            var node: AccessibilityNodeInfo? = waitText(text)
            while (node != null) {
                if (node.isClickable && node.performAction(AccessibilityNodeInfo.ACTION_CLICK))
                    return
                node = node.parent
            }
            error("Native UI action unavailable: $text")
        }
        fun screenshot(name: String) {
            val dir = File(app.getExternalFilesDir(null), "UiEvidence").apply { mkdirs() }
            val bitmap = instrumentation.uiAutomation.takeScreenshot()
            File(dir, name).outputStream().use {
                bitmap.compress(Bitmap.CompressFormat.PNG, 100, it)
            }
        }
        try {
            alice.request("/me", "PATCH", JSONObject().put("displayName", "Native UI Alice"))
            bob.request("/me", "PATCH", JSONObject().put("displayName", "Native UI Bob"))
            val request =
                alice.request(
                    "/contacts/requests",
                    "POST",
                    JSONObject().put("userId", peer).put("requestId", UUID.randomUUID().toString()),
                )
            bob.request(
                "/contacts/requests/${request.getString("id")}/accept",
                "POST",
                JSONObject(),
            )
            val group =
                alice.request(
                    "/conversations",
                    "POST",
                    JSONObject()
                        .put("kind", "group")
                        .put("title", "UI fixture group")
                        .put("memberIds", JSONArray(listOf(peer))),
                )
            val snapshot = conversationSnapshot(alice, group.getString("id"))
            alice.request(
                "/conversations/${group.getString("id")}/messages",
                "POST",
                JSONObject()
                    .put("sendAdmission", snapshot.getString("sendAdmission"))
                    .put("clientMsgId", UUID.randomUUID().toString())
                    .put("type", "text")
                    .put("text", "Native UI message")
                    .put("restoreEpoch", boot.getString("restoreEpoch")),
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
                .putString("theme", "light")
                .commit()
            TokenVault(app).save(profile, a.getString("token"))
            app.startActivity(
                Intent(app, MainActivity::class.java)
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
            )
            click("UI fixture group")
            waitText("Native UI message")
            screenshot("01-Native-Chat.png")
            click("Members")
            waitText("Native UI Bob")
            waitText("Transfer ownership")
            screenshot("02-Native-Group.png")
            click("Close")
            click("Back")
            click("Settings")
            waitText("Display name")
            waitText("Profile photo")
            screenshot("03-Native-Settings.png")
            click("Dark")
            Thread.sleep(1000)
            instrumentation.waitForIdleSync()
            screenshot("04-Native-Dark-Settings.png")
            click("System")
            Thread.sleep(1000)
            instrumentation.waitForIdleSync()
            screenshot("05-Native-System-Settings.png")
        } finally {
            screenshot("99-Before-Cleanup.png")
            alice.request("/auth/logout", "POST", JSONObject())
            bob.request("/auth/logout", "POST", JSONObject())
        }
    }
}
