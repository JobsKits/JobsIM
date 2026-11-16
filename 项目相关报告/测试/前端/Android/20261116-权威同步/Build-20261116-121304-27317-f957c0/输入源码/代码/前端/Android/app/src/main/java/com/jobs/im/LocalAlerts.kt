package com.jobs.im

import android.app.*
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build

class LocalAlerts(private val context: Context) {
    private val manager = context.getSystemService(NotificationManager::class.java)

    fun deliver(source: String, current: String, row: MessageRow, title: String, language: String) {
        if (
            source != current ||
                !context.getSharedPreferences("JobsIM.settings", 0).getBoolean("alerts", false)
        )
            return
        if (
            Build.VERSION.SDK_INT >= 33 &&
                context.checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) !=
                    PackageManager.PERMISSION_GRANTED
        )
            return
        manager.createNotificationChannel(
            NotificationChannel("messages", "JobsIM", NotificationManager.IMPORTANCE_DEFAULT)
        )
        val intent =
            PendingIntent.getActivity(
                context,
                0,
                Intent(context, MainActivity::class.java),
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
        val notification =
            Notification.Builder(context, "messages")
                .setSmallIcon(android.R.drawable.ic_dialog_email)
                .setContentTitle("JobsIM")
                .setContentText(Strings.text(language, "newLocalMessage"))
                .setVisibility(Notification.VISIBILITY_PRIVATE)
                .setContentIntent(intent)
                .setAutoCancel(true)
                .build()
        manager.notify(Isolation.digest(source + row.id).hashCode(), notification)
    }

    fun clear() {
        manager.cancelAll()
    }
}
