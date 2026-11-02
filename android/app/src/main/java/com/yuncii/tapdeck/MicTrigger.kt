package com.yuncii.tapdeck

import kotlin.math.hypot

/**
 * 语音控件的手势判定，下半区语音区与视频模式悬浮球共用：
 * - 长按语音输入：按住 [HOLD_MS] 后开始，松手结束；
 * - 单击语音输入：轻点开始，录音中再轻点结束；
 * - 两种模式下移动超过 slop 即为拖动控件，不触发语音。
 */
class MicTrigger(private val slop: Float, private val host: Host) {
    interface Host {
        val gestureMode: String
        val available: Boolean
        val status: String
        fun begin(mode: String): Boolean
        fun end(abort: Boolean)
        /** 延迟 delayMs 后调用 [holdElapsed]；[unschedule] 取消。 */
        fun schedule(delayMs: Long)
        fun unschedule()
        /** 拖动中：相对按下点的位移。 */
        fun drag(dx: Float, dy: Float)
        fun dragEnd()
        fun feedback(kind: KeyFeedback)
        /** 语音已开始，用于立即显示“准备”状态。 */
        fun started(mode: String)
    }
    companion object { const val HOLD_MS = 300L }

    var tracking = false; private set
    private var downX = 0f; private var downY = 0f; private var downTime = 0L
    private var dragging = false
    private var holdStarted = false

    fun down(x: Float, y: Float, time: Long) {
        if (host.available) host.feedback(KeyFeedback.Press)
        tracking = true; downX = x; downY = y; downTime = time
        dragging = false; holdStarted = false
        if (host.gestureMode == MicBallView.MODE_HOLD && host.available && host.status == "idle") host.schedule(HOLD_MS)
    }

    fun move(x: Float, y: Float) {
        if (!tracking) return
        val dx = x - downX; val dy = y - downY
        if (!dragging && hypot(dx, dy) > slop) { dragging = true; host.unschedule() }
        if (dragging) host.drag(dx, dy)
    }

    fun holdElapsed() {
        if (!tracking || dragging || !host.available || host.gestureMode != MicBallView.MODE_HOLD || host.status != "idle") return
        holdStarted = host.begin(MicBallView.MODE_HOLD)
        if (holdStarted) { host.feedback(KeyFeedback.LongPress); host.started(MicBallView.MODE_HOLD) }
    }

    fun up(x: Float, y: Float, time: Long) {
        if (!tracking) return
        host.unschedule()
        val tapped = !dragging && hypot(x - downX, y - downY) <= slop && time - downTime < HOLD_MS
        when {
            holdStarted -> host.end(false)
            dragging || !host.available -> Unit
            host.gestureMode == MicBallView.MODE_TOGGLE && tapped ->
                if (host.status == "idle") { if (host.begin(MicBallView.MODE_TOGGLE)) host.started(MicBallView.MODE_TOGGLE) } else host.end(false)
        }
        if (dragging) host.dragEnd()
        reset()
    }

    fun cancel() {
        host.unschedule()
        if (holdStarted) host.end(true)
        if (dragging) host.dragEnd()
        reset()
    }

    private fun reset() { tracking = false; dragging = false; holdStarted = false }
}
