package com.jobs.im

import android.app.NotificationManager
import android.os.Build
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class NotificationPrivacyTest {
    @Test
    fun localAlertHidesBodyAndRejectsAnotherNamespace() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val app = instrumentation.targetContext
        if (Build.VERSION.SDK_INT >= 33)
            instrumentation.uiAutomation
                .executeShellCommand(
                    "pm grant ${app.packageName} android.permission.POST_NOTIFICATIONS"
                )
                .use { android.os.ParcelFileDescriptor.AutoCloseInputStream(it).readBytes() }
        val prefs = app.getSharedPreferences("JobsIM.settings", 0)
        val before = prefs.getBoolean("alerts", false)
        val manager = app.getSystemService(NotificationManager::class.java)
        val alerts = LocalAlerts(app)
        try {
            alerts.clear()
            prefs.edit().putBoolean("alerts", true).commit()
            val row =
                MessageRow(
                    "fixture",
                    "own",
                    "room",
                    "id",
                    senderId = "peer",
                    text = "Private body must never appear",
                )
            alerts.deliver("foreign", "own", row, "Private conversation title", "en")
            Thread.sleep(100)
            assertTrue(manager.activeNotifications.isEmpty())
            alerts.deliver("own", "own", row, "Private conversation title", "en")
            Thread.sleep(250)
            val notice = manager.activeNotifications.single().notification
            assertEquals(
                "You have a new message",
                notice.extras.getCharSequence("android.text").toString(),
            )
            assertFalse(notice.extras.toString().contains(row.text))
            assertFalse(notice.extras.toString().contains("Private conversation title"))
        } finally {
            alerts.clear()
            prefs.edit().putBoolean("alerts", before).commit()
        }
    }
}
