package com.yuncii.tapdeck

import android.app.Application
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.launch

/**
 * 进程级的连接与电脑通知。连接不再随 Activity / ViewModel 销毁，前台服务
 * [LinkService] 在后台保持进程，电脑转发的通知可以继续到达。
 * 惰性创建：只运行仪器测试、不打开界面时不会自动连接电脑。
 */
class TapDeckApp : Application() {
    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    val notifications by lazy { PcNotificationCenter(this, appScope) }
    val client: TapClient by lazy {
        TapClient(this, appScope, notifications).also { client ->
            appScope.launch { client.restore() }
            // 开关打开且选定了电脑时保持前台服务；关闭开关或忘记电脑时停止。
            appScope.launch {
                combine(notifications.backgroundEnabled, client.state) { enabled, state -> enabled && state.selectedPeerId != null }
                    .distinctUntilChanged().collect { run -> LinkService.sync(this@TapDeckApp, run) }
            }
            appScope.launch { notifications.backgroundEnabled.collect { client.refreshLinkMode() } }
        }
    }

    /** 回到前台时补启动被系统拒绝的服务。 */
    fun ensureLinkService() {
        val state = client.state.value
        LinkService.sync(this, notifications.backgroundEnabled.value && state.selectedPeerId != null)
    }
}
