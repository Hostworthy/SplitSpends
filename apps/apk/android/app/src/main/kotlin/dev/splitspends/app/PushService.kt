package dev.splitspends.app

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import org.json.JSONObject
import org.unifiedpush.android.connector.data.PushMessage
import org.unifiedpush.flutter.connector.UnifiedPushService

/**
 * Receives UnifiedPush events. Messages are Web Push payloads from the
 * SplitSpends server ({ title, body, url }), already decrypted by the
 * connector library; they are shown here, natively, so a notification
 * appears even when the Flutter UI is not running. Endpoint changes are
 * forwarded to Flutter by the parent class, which re-registers the phone
 * with the server.
 */
class PushService : UnifiedPushService() {
    override fun onMessage(message: PushMessage, instance: String) {
        if (!message.decrypted) return
        val json = try {
            JSONObject(String(message.content, Charsets.UTF_8))
        } catch (_: Exception) {
            return
        }
        show(this, json.optString("title", "SplitSpends"), json.optString("body", ""))
    }

    companion object {
        private const val CHANNEL = "activity"

        fun show(context: Context, title: String, body: String) {
            if (Build.VERSION.SDK_INT >= 33 &&
                context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
            ) {
                return
            }
            val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            if (Build.VERSION.SDK_INT >= 26 && manager.getNotificationChannel(CHANNEL) == null) {
                manager.createNotificationChannel(
                    NotificationChannel(CHANNEL, "Expenses and payments", NotificationManager.IMPORTANCE_DEFAULT),
                )
            }
            val open = PendingIntent.getActivity(
                context,
                0,
                Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP),
                PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
            )
            val builder = if (Build.VERSION.SDK_INT >= 26) {
                android.app.Notification.Builder(context, CHANNEL)
            } else {
                @Suppress("DEPRECATION")
                android.app.Notification.Builder(context)
            }
            val notification = builder
                .setSmallIcon(R.drawable.ic_notification)
                .setContentTitle(title)
                .setContentText(body)
                .setStyle(android.app.Notification.BigTextStyle().bigText(body))
                .setContentIntent(open)
                .setAutoCancel(true)
                .build()
            manager.notify((System.currentTimeMillis() % Int.MAX_VALUE).toInt(), notification)
        }
    }
}
