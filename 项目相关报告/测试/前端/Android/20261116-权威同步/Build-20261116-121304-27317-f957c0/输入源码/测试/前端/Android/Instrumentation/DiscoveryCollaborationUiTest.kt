package com.jobs.im

import android.app.Application
import android.content.Intent
import android.graphics.Bitmap
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.util.Base64
import android.view.MotionEvent
import android.view.accessibility.AccessibilityNodeInfo
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.util.UUID
import kotlinx.coroutines.*
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DiscoveryCollaborationUiTest {
    @Test
    fun nativeFavoritesExactSourceMentionsAndAnnouncementControlsReachRealBackend() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val args = InstrumentationRegistry.getArguments()
        val base = args.getString("liveUrl")
        val encoded = args.getString("caBase64")
        assumeTrue(!base.isNullOrBlank() && !encoded.isNullOrBlank())
        val ca = Base64.decode(encoded, Base64.DEFAULT)
        val anonymous = Api(base!!, ca = ca)
        val boot = anonymous.request("/bootstrap")
        assertEquals(args.getString("deploymentId"), boot.getString("deploymentId"))
        assertTrue(boot.getJSONObject("features").getBoolean("role_admission"))
        val suffix = UUID.randomUUID().toString().replace("-", "").take(10)
        val password = "Native UI feature fixture " + UUID.randomUUID()
        suspend fun account(name: String) =
            anonymous.request(
                "/auth/register",
                "POST",
                JSONObject()
                    .put("username", name)
                    .put("password", password)
                    .put("requestId", UUID.randomUUID())
                    .put("deviceName", "Native feature UI"),
            )
        val a = account("android_native_a_" + suffix)
        val b = account("android_native_b_" + suffix)
        val alice = Api(base, a.getString("token"), ca)
        val bob = Api(base, b.getString("token"), ca)
        val me = a.getJSONObject("user").getString("id")
        val peer = b.getJSONObject("user").getString("id")
        val app = instrumentation.targetContext.applicationContext as Application
        fun nodes(): List<AccessibilityNodeInfo> {
            if (android.os.Build.VERSION.SDK_INT >= 33) instrumentation.uiAutomation.clearCache()
            val result = mutableListOf<AccessibilityNodeInfo>()
            fun walk(node: AccessibilityNodeInfo?) {
                if (node == null) return
                result.add(node)
                for (i in 0 until node.childCount) walk(node.getChild(i))
            }
            walk(instrumentation.uiAutomation.rootInActiveWindow)
            return result
        }
        fun matches(node: AccessibilityNodeInfo, text: String) =
            node.isVisibleToUser &&
                (node.text?.toString().orEmpty() +
                        node.contentDescription?.toString().orEmpty() +
                        node.hintText?.toString().orEmpty())
                    .contains(text)
        fun scroll(forward: Boolean): Boolean {
            val action =
                if (forward) AccessibilityNodeInfo.ACTION_SCROLL_FORWARD
                else AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD
            val moved =
                nodes().firstOrNull { it.isVisibleToUser && it.isScrollable }?.performAction(action)
                    ?: false
            if (moved) Thread.sleep(120)
            return moved
        }
        fun top() {
            repeat(10) { if (!scroll(false)) return }
        }
        fun waitText(text: String) {
            repeat(100) {
                if (nodes().any { matches(it, text) }) return
                Thread.sleep(100)
            }
            error("Native feature text missing: $text")
        }
        fun tap(node: AccessibilityNodeInfo): Boolean {
            val rect = Rect()
            node.getBoundsInScreen(rect)
            val metrics = app.resources.displayMetrics
            if (!rect.intersect(0, 0, metrics.widthPixels, metrics.heightPixels)) return false
            var parent = node.parent
            while (parent != null) {
                if (parent.isScrollable) {
                    val viewport = Rect()
                    parent.getBoundsInScreen(viewport)
                    if (!rect.intersect(viewport)) return false
                }
                parent = parent.parent
            }
            if (rect.width() <= 0 || rect.height() <= 0 || rect.height() > 900) return false
            val start = SystemClock.uptimeMillis()
            val down =
                MotionEvent.obtain(
                    start,
                    start,
                    MotionEvent.ACTION_DOWN,
                    rect.exactCenterX(),
                    rect.exactCenterY(),
                    0,
                )
            val up =
                MotionEvent.obtain(
                    start,
                    SystemClock.uptimeMillis(),
                    MotionEvent.ACTION_UP,
                    rect.exactCenterX(),
                    rect.exactCenterY(),
                    0,
                )
            try {
                instrumentation.sendPointerSync(down)
                instrumentation.sendPointerSync(up)
            } finally {
                down.recycle()
                up.recycle()
            }
            return true
        }
        fun click(text: String) {
            repeat(60) {
                val candidates =
                    nodes()
                        .filter { matches(it, text) }
                        .sortedBy {
                            if (
                                it.text?.toString() == text ||
                                    it.contentDescription?.toString() == text
                            )
                                0
                            else 1
                        }
                for (candidate in candidates) {
                    var node: AccessibilityNodeInfo? = candidate
                    while (node != null) {
                        if (node.className?.toString()?.contains("EditText") == true) break
                        if (
                            node.isClickable &&
                                node.isEnabled &&
                                (node.performAction(AccessibilityNodeInfo.ACTION_CLICK) ||
                                    tap(candidate))
                        ) {
                            instrumentation.waitForIdleSync()
                            Thread.sleep(500)
                            return
                        }
                        node = node.parent
                    }
                }
                if (it % 12 == 11) top() else scroll(true)
                Thread.sleep(100)
            }
            error("Native feature action missing: $text")
        }
        fun edit(label: String, value: String) {
            repeat(50) {
                for (candidate in nodes().filter { matches(it, label) }) {
                    var node: AccessibilityNodeInfo? = candidate
                    while (node != null) {
                        if (node.isEditable) {
                            val data = Bundle()
                            data.putCharSequence(
                                AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE,
                                value,
                            )
                            if (node.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, data)) {
                                Thread.sleep(150)
                                return
                            }
                        }
                        node = node.parent
                    }
                }
                Thread.sleep(100)
            }
            error("Native editable field missing: $label")
        }
        fun screenshot(name: String) {
            instrumentation.waitForIdleSync()
            Thread.sleep(300)
            val dir = File(app.getExternalFilesDir(null), "DiscoveryUiEvidence").apply { mkdirs() }
            File(dir, name).outputStream().use {
                instrumentation.uiAutomation
                    .takeScreenshot()
                    .compress(Bitmap.CompressFormat.PNG, 100, it)
            }
        }
        try {
            alice.request("/me", "PATCH", JSONObject().put("displayName", "Native UI Alice"))
            bob.request("/me", "PATCH", JSONObject().put("displayName", "Native UI Bob"))
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
            val created =
                alice.request(
                    "/conversations",
                    "POST",
                    JSONObject()
                        .put("kind", "group")
                        .put("title", "Native feature group")
                        .put("memberIds", JSONArray(listOf(peer))),
                )
            val group = conversationSnapshot(alice, created.getString("id"))
            val peerGroup = conversationSnapshot(bob, created.getString("id"))
            val mention =
                bob.request(
                    "/conversations/${created.getString("id")}/messages",
                    "POST",
                    JSONObject()
                        .put("clientMsgId", UUID.randomUUID())
                        .put("restoreEpoch", boot.getString("restoreEpoch"))
                        .put("sendAdmission", peerGroup.getString("sendAdmission"))
                        .put("type", "text")
                        .put("text", "Native mention highlights")
                        .put("mentionedUserIds", JSONArray(listOf(me))),
                )
            alice.request("/messages/${mention.getString("id")}/favorite", "PUT")
            val source =
                alice.request(
                    "/conversations/${created.getString("id")}/messages",
                    "POST",
                    JSONObject()
                        .put("clientMsgId", UUID.randomUUID())
                        .put("restoreEpoch", boot.getString("restoreEpoch"))
                        .put("sendAdmission", group.getString("sendAdmission"))
                        .put("type", "text")
                        .put("text", "Announcement UI original"),
                )
            alice.request(
                "/conversations/${created.getString("id")}/announcement",
                "PUT",
                JSONObject()
                    .put("messageId", source.getString("id"))
                    .put("sourceVersion", source.getString("contentVersion"))
                    .put("expectedVersion", "0")
                    .put("membershipGeneration", group.getString("membershipGeneration"))
                    .put("roleGeneration", group.getString("roleGeneration"))
                    .put("requestId", UUID.randomUUID()),
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
            val activity =
                instrumentation.startActivitySync(
                    Intent(app, MainActivity::class.java)
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TASK)
                )
            waitText("Native feature group")
            click("My favorites")
            waitText("Native mention highlights")
            screenshot("01-Native-Favorites.png")
            click("Native mention highlights")
            waitText("Original message")
            waitText("Mentioned you")
            screenshot("02-Exact-Source-Mention.png")
            click("Mention members")
            waitText("Native UI Bob")
            click("Native UI Bob")
            click("Done")
            waitText("Selected mentions")
            edit("Write a message", "Native UI directed mention")
            instrumentation.runOnMainSync {
                WindowInsetsControllerCompat(activity.window, activity.window.decorView)
                    .hide(WindowInsetsCompat.Type.ime())
            }
            instrumentation.waitForIdleSync()
            click("Send")
            instrumentation.runOnMainSync {
                WindowInsetsControllerCompat(activity.window, activity.window.decorView)
                    .hide(WindowInsetsCompat.Type.ime())
            }
            instrumentation.waitForIdleSync()
            withTimeout(10000) {
                while (true) {
                    val messages =
                        bob.request(
                                "/conversations/${created.getString("id")}/messages?after=0&limit=200"
                            )
                            .getJSONArray("items")
                    val sent =
                        (0 until messages.length())
                            .map { messages.getJSONObject(it) }
                            .firstOrNull { it.optString("text") == "Native UI directed mention" }
                    if (sent != null) {
                        assertTrue(sent.getBoolean("mentionedMe"))
                        break
                    }
                    delay(100)
                }
            }
            click("Members")
            waitText("Close")
            top()
            screenshot("03a-Group-Before-Announcement.png")
            click("Group announcement")
            screenshot("03b-After-Announcement-Click.png")
            waitText("Announcement UI original")
            screenshot("03-Native-Announcement.png")
            click("Announcement UI original")
            click("No expiry")
            click("After one day")
            click("Publish announcement")
            withTimeout(10000) {
                while (
                    alice
                        .request("/conversations/${created.getString("id")}/announcement")
                        .getString("version") != "2"
                ) delay(100)
            }
            assertTrue(
                alice
                    .request("/conversations/${created.getString("id")}/announcement")
                    .getString("expiresAt")
                    .isNotBlank()
            )
            click("Clear announcement")
            withTimeout(10000) {
                while (
                    alice
                        .request("/conversations/${created.getString("id")}/announcement")
                        .getString("state") != "empty"
                ) delay(100)
            }
            top()
            waitText("No group announcement")
            screenshot("04-Native-Cleared-Announcement.png")
            click("Close")
            click("Search")
            edit("Search", "Announcement UI original")
            click("Search")
            waitText("Announcement UI original")
            screenshot("05-Native-Search.png")
            click("Announcement UI original")
            waitText("Original message")
            click("Back")
            click("Settings")
            click("Dark")
            withTimeout(5000) {
                while (
                    app.getSharedPreferences("JobsIM.settings", 0).getString("theme", "") != "dark"
                ) delay(100)
            }
            Thread.sleep(1000)
            click("My favorites")
            waitText("Native mention highlights")
            screenshot("06-Native-Dark-Favorites.png")
            click("Close")
        } finally {
            screenshot("99-Before-Cleanup.png")
            alice.request("/auth/logout", "POST", JSONObject())
            bob.request("/auth/logout", "POST", JSONObject())
        }
    }
}
