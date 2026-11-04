package com.yuncii.tapdeck

import android.app.Notification
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch

/**
 * 后台保持与电脑连接的前台服务（类型 connectedDevice）：App 不在前台时进程不被回收，
 * 电脑转发的通知可以继续到达。只显示一条低优先级常驻通知，本身不做网络工作。
 */
class LinkService : Service() {
    companion object {
        private const val NOTIFICATION_ID = 41
        @Volatile var running = false
            private set

        /** 按开关与配对状态启动或停止服务。Android 12+ 只能在前台时启动，失败时等待下次回到前台。 */
        fun sync(context: Context, run: Boolean) {
            val intent = Intent(context, LinkService::class.java)
            if (!run) { if (running) context.stopService(intent); return }
            if (running) return
            runCatching { context.startForegroundService(intent) }.onFailure { Log.w("TapDeck", "后台连接服务未能启动", it) }
        }
    }

    private var job: Job? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val app = application as TapDeckApp
        try {
            val notification = build(linkText(app.client.state.value))
            if (Build.VERSION.SDK_INT >= 29) startForeground(NOTIFICATION_ID, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_CONNECTED_DEVICE)
            else startForeground(NOTIFICATION_ID, notification)
        } catch (e: Exception) {
            // 系统重启服务时若不允许后台启动前台服务（未豁免电池优化），等用户再次打开 App。
            Log.w("TapDeck", "后台连接服务启动被拒绝", e)
            stopSelf()
            return START_NOT_STICKY
        }
        running = true
        if (job == null) job = app.appScope.launch {
            app.client.state.map(::linkText).distinctUntilChanged().collect { text ->
                runCatching { getSystemService(NotificationManager::class.java)?.notify(NOTIFICATION_ID, build(text)) }
            }
        }
        return START_STICKY
    }

    override fun onDestroy() {
        running = false
        job?.cancel(); job = null
        super.onDestroy()
    }

    private fun linkText(state: ClientState): String = when {
        state.connected && NOTIFY_FEATURE !in state.touchpad.features -> "已连接 ${state.peerName} · 电脑端版本过旧，无法转发通知"
        state.connected && !(application as TapDeckApp).notifications.batteryExempt() -> "已连接 ${state.peerName} · 锁屏后可能被系统冻结，请在 TapDeck 设置中允许后台运行"
        state.connected -> "已连接 ${state.peerName} · 正在接收电脑通知"
        state.selectedPeerId == null -> "未选择电脑"
        else -> "${state.peerName} · ${state.status}（自动重连）"
    }

    private fun build(text: String): Notification = Notification.Builder(this, PcNotificationCenter.CHANNEL_LINK)
        .setSmallIcon(R.drawable.ic_lucide_plug)
        .setContentTitle("TapDeck 后台连接")
        .setContentText(text)
        .setOngoing(true)
        .setShowWhen(false)
        .setCategory(Notification.CATEGORY_SERVICE)
        .setContentIntent(PendingIntent.getActivity(this, 1,
            Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE))
        .build()
}
