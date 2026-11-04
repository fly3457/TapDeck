package com.yuncii.tapdeck

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.util.Log
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/**
 * 电脑通知的收件箱、系统通知与前台横幅。进程级单例，由 [TapDeckApp] 持有；
 * App 在前台时只显示界面内横幅，在后台时弹出 Android 系统通知。
 */
class PcNotificationCenter(private val app: Context, private val scope: CoroutineScope) {
    companion object {
        const val CHANNEL_PC = "pc_notifications"
        const val CHANNEL_LINK = "background_link"
        /** 点击系统通知打开 App 时附带，MainActivity 据此打开通知列表。 */
        const val EXTRA_OPEN_NOTIFICATIONS = "com.yuncii.tapdeck.OPEN_NOTIFICATIONS"
    }

    private val store = PairStore(app)
    private val saving = Mutex()
    private val loaded = CompletableDeferred<Unit>()
    private val mutable = MutableStateFlow(NotificationInbox())
    val inbox = mutable.asStateFlow()
    private val backgroundState = MutableStateFlow(true)
    /** “后台接收电脑通知”开关：开启时 App 进入后台仍由前台服务保持连接。 */
    val backgroundEnabled = backgroundState.asStateFlow()
    private var permissionAsked = false
    private val bannerFlow = MutableSharedFlow<PcNotification>(extraBufferCapacity = 8)
    /** 前台收到的新通知，界面顶部短暂显示。 */
    val banner = bannerFlow.asSharedFlow()
    /** MainActivity 可见时为真。 */
    @Volatile var foreground = false
    private val manager = app.getSystemService(NotificationManager::class.java)

    init {
        createChannels()
        scope.launch {
            try {
                val (inbox, prefs) = withContext(Dispatchers.IO) { store.loadInbox() to store.loadNotifyPreferences() }
                mutable.value = inbox
                backgroundState.value = prefs.first
                permissionAsked = prefs.second
            } catch (e: Exception) {
                Log.w("TapDeck", "电脑通知读取失败", e)
            } finally { loaded.complete(Unit) }
        }
    }

    private fun createChannels() {
        manager?.createNotificationChannel(NotificationChannel(CHANNEL_PC, "电脑通知", NotificationManager.IMPORTANCE_HIGH).apply {
            description = "转发自已连接电脑的 Windows 系统通知"
        })
        manager?.createNotificationChannel(NotificationChannel(CHANNEL_LINK, "后台连接", NotificationManager.IMPORTANCE_MIN).apply {
            description = "后台保持与电脑的连接以接收通知"
            setShowBadge(false)
        })
    }

    suspend fun cursor(peerId: String): NotifyCursor? { loaded.await(); return mutable.value.cursor(peerId) }

    /** 合并一批 PC 通知；在主线程调用。 */
    fun receive(peerId: String, peerName: String, boot: String, items: List<WireNotification>) {
        scope.launch(Dispatchers.Main.immediate) {
            loaded.await()
            val (next, fresh) = mutable.value.accept(peerId, peerName, boot, items)
            if (next == mutable.value) return@launch
            mutable.value = next
            persist()
            if (fresh.isEmpty()) return@launch
            if (foreground) bannerFlow.tryEmit(fresh.last())
            else post(NotificationInbox.alertable(fresh, System.currentTimeMillis()))
        }
    }

    fun markRead() {
        if (mutable.value.unread == 0) return
        mutable.value = mutable.value.markRead()
        persist()
        cancelPosted()
    }

    fun clear() {
        mutable.value = mutable.value.clear()
        persist()
        cancelPosted()
    }

    fun forget(peerId: String) {
        scope.launch(Dispatchers.Main.immediate) {
            loaded.await()
            mutable.value = mutable.value.forget(peerId)
            persist()
        }
    }

    fun setBackgroundEnabled(on: Boolean) {
        if (backgroundState.value == on) return
        backgroundState.value = on
        scope.launch { runCatching { withContext(Dispatchers.IO) { store.saveNotifyBackground(on) } } }
    }

    fun permissionGranted(): Boolean = Build.VERSION.SDK_INT < 33 ||
        app.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

    /** 是否应自动请求一次通知权限（只问一次，之后由设置页按钮引导）。 */
    suspend fun shouldAskPermission(): Boolean {
        loaded.await()
        if (permissionGranted() || permissionAsked || !backgroundState.value) return false
        permissionAsked = true
        runCatching { withContext(Dispatchers.IO) { store.markNotifyPermissionAsked() } }
        return true
    }

    /** 应用是否已豁免电池优化：努比亚等机型不豁免时，即使有前台服务也会在锁屏后冻结进程。 */
    fun batteryExempt(): Boolean =
        app.getSystemService(android.os.PowerManager::class.java)?.isIgnoringBatteryOptimizations(app.packageName) ?: true

    /** 是否应主动提示一次关闭电池优化（开启后台接收且尚未豁免时）。 */
    suspend fun shouldAskBattery(): Boolean {
        loaded.await()
        if (!backgroundState.value || batteryExempt()) return false
        val asked = runCatching { withContext(Dispatchers.IO) { store.loadBatteryAsked() } }.getOrDefault(true)
        if (asked) return false
        runCatching { withContext(Dispatchers.IO) { store.markBatteryAsked() } }
        return true
    }

    private fun persist() {
        val snapshot = mutable.value
        scope.launch {
            saving.withLock {
                // 保存最新状态即可：排队中的旧快照被跳过。
                if (snapshot !== mutable.value) return@withLock
                runCatching { withContext(Dispatchers.IO) { store.saveInbox(snapshot) } }
                    .onFailure { Log.w("TapDeck", "电脑通知保存失败", it) }
            }
        }
    }

    private fun openIntent(requestCode: Int): PendingIntent = PendingIntent.getActivity(app, requestCode,
        Intent(app, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
            .putExtra(EXTRA_OPEN_NOTIFICATIONS, true),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)

    private fun post(items: List<PcNotification>) {
        if (items.isEmpty() || !permissionGranted()) return
        val manager = manager ?: return
        for (n in items) {
            val group = "pc:" + n.peerId
            val notification = Notification.Builder(app, CHANNEL_PC)
                .setSmallIcon(R.drawable.ic_lucide_bell)
                .setContentTitle(n.heading)
                .setContentText(n.body)
                .setStyle(Notification.BigTextStyle().bigText(n.body))
                .setSubText(n.peerName.ifBlank { null })
                .setWhen(if (n.time > 0) n.time else System.currentTimeMillis())
                .setShowWhen(true)
                .setAutoCancel(true)
                .setGroup(group)
                .setCategory(Notification.CATEGORY_MESSAGE)
                .setContentIntent(openIntent(0))
                .build()
            runCatching { manager.notify(n.key, 0, notification) }.onFailure { Log.w("TapDeck", "系统通知发送失败", it) }
        }
        // 同一台 PC 多条通知折叠为一组。
        for ((peerId, list) in items.groupBy { it.peerId }) {
            val summary = Notification.Builder(app, CHANNEL_PC)
                .setSmallIcon(R.drawable.ic_lucide_bell)
                .setContentTitle(list.last().peerName.ifBlank { "电脑通知" })
                .setGroup("pc:$peerId")
                .setGroupSummary(true)
                .setAutoCancel(true)
                .setContentIntent(openIntent(0))
                .build()
            runCatching { manager.notify("summary:$peerId", 0, summary) }
        }
    }

    /** 打开 App 内列表后撤下已弹出的电脑通知（保留前台服务通知）。 */
    private fun cancelPosted() {
        val manager = manager ?: return
        runCatching {
            manager.activeNotifications.filter { it.notification.channelId == CHANNEL_PC }.forEach { manager.cancel(it.tag, it.id) }
        }
    }
}
