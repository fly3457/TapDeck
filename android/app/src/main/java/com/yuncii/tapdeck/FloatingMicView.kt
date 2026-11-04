package com.yuncii.tapdeck

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.RectF
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import kotlin.math.hypot

/**
 * 视频模式的语音悬浮球：铺满父视图的透明层，只有命中球体的触摸才消费，
 * 其余触摸穿透给下面的触控手势层。手势与下半区语音区一致（[MicTrigger]），
 * 固定使用第一个已启用的语音组，可拖动到屏幕任意位置。
 */
class FloatingMicView(
    context: Context,
    private val begin: (String) -> Boolean,
    private val end: (Boolean) -> Unit,
    private val savePosition: (Float, Float) -> Unit,
) : View(context) {
    private val density = resources.displayMetrics.density
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val ring = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE; strokeCap = Paint.Cap.ROUND }
    private val box = RectF()
    // 视频模式画面优先：比最初的 32dp 缩小 20%。
    private val radius = 32 * 0.8f * density
    /** 球的位置变化（拖动、恢复位置、尺寸变化）时回调，跟随的快捷键面板据此重新定位。 */
    var onMoved: () -> Unit = {}
    val ballRadius: Float get() = radius
    fun ballCenter(): Pair<Float, Float> = center()
    private var nx = 0.86f; private var ny = 0.78f
    private var restored = false; private var moved = false
    private var pointer = -1
    private var originX = 0f; private var originY = 0f
    var onFeedback: (KeyFeedback) -> Unit = {}

    var gestureMode = MicBallView.MODE_TOGGLE
        set(v) { if (field != v) { field = v; invalidate() } }
    var status = "idle"
        set(v) { if (field != v) { field = v; invalidate() } }
    var mode = ""
    var available = false
        set(v) { if (field != v) { if (!v) cancel(); field = v; invalidate() } }
    var level = 0f
        set(v) { field = v; if (status != "idle") invalidate() }
    var profileName = ""
        set(v) { if (field != v) { field = v; contentDescription = "语音悬浮球：$v，可拖动" } }

    private val trigger: MicTrigger = MicTrigger(ViewConfiguration.get(context).scaledTouchSlop.toFloat(), object : MicTrigger.Host {
        override val gestureMode get() = this@FloatingMicView.gestureMode
        override val available get() = this@FloatingMicView.available
        override val status get() = this@FloatingMicView.status
        override fun begin(mode: String) = this@FloatingMicView.begin(mode)
        override fun end(abort: Boolean) = this@FloatingMicView.end(abort)
        override fun schedule(delayMs: Long) { postDelayed(hold, delayMs) }
        override fun unschedule() { removeCallbacks(hold) }
        override fun drag(dx: Float, dy: Float) = moveTo(originX + dx, originY + dy)
        override fun dragEnd() = savePosition(nx, ny)
        override fun feedback(kind: KeyFeedback) = onFeedback(kind)
        override fun started(mode: String) { this@FloatingMicView.mode = mode; if (this@FloatingMicView.status == "idle") this@FloatingMicView.status = "preparing" }
    })
    private val hold: Runnable = Runnable { trigger.holdElapsed(); invalidate() }

    init { isClickable = true; contentDescription = "语音悬浮球，可拖动" }

    private fun center(): Pair<Float, Float> {
        val margin = radius + 8 * density
        val w = (width - margin * 2).coerceAtLeast(0f); val h = (height - margin * 2).coerceAtLeast(0f)
        return (margin + w * nx) to (margin + h * ny)
    }
    private fun moveTo(x: Float, y: Float) {
        val margin = radius + 8 * density
        val w = width - margin * 2; val h = height - margin * 2
        nx = if (w > 0) ((x - margin) / w).coerceIn(0f, 1f) else 0.5f
        ny = if (h > 0) ((y - margin) / h).coerceIn(0f, 1f) else 0.5f
        moved = true; invalidate(); onMoved()
    }
    fun restorePosition(x: Float, y: Float) {
        if (!restored && !moved && x.isFinite() && y.isFinite()) { nx = x.coerceIn(0f, 1f); ny = y.coerceIn(0f, 1f); restored = true; invalidate(); onMoved() }
    }
    override fun onSizeChanged(w: Int, h: Int, oldw: Int, oldh: Int) { super.onSizeChanged(w, h, oldw, oldh); onMoved() }

    private fun hit(x: Float, y: Float): Boolean {
        val (cx, cy) = center()
        // 触摸目标比可见球体略大，便于在画面上快速按到。
        return hypot(x - cx, y - cy) <= radius * 1.25f
    }

    override fun onDraw(c: Canvas) {
        val (cx, cy) = center()
        val recording = status in listOf("preparing", "transmitting", "stopping")
        paint.style = Paint.Style.FILL
        paint.color = when {
            status == "preparing" -> Color.rgb(186, 116, 11)
            status == "stopping" -> Color.rgb(112, 121, 135)
            status == "transmitting" && mode == MicBallView.MODE_TOGGLE -> Color.rgb(23, 128, 97)
            status == "transmitting" -> Color.rgb(206, 53, 64)
            available -> Color.rgb(23, 92, 211)
            else -> Color.rgb(133, 146, 162)
        }
        paint.alpha = if (recording) 240 else 200
        if (gestureMode == MicBallView.MODE_TOGGLE) {
            box.set(cx - radius, cy - radius, cx + radius, cy + radius)
            c.drawRoundRect(box, radius * 0.3f, radius * 0.3f, paint)
        } else c.drawCircle(cx, cy, radius, paint)
        if (status == "transmitting" && level > 0f) {
            ring.color = Color.rgb(70, 159, 220); ring.strokeWidth = 4 * density
            val r = radius + 5 * density
            box.set(cx - r, cy - r, cx + r, cy + r)
            c.drawArc(box, -90f, 360f * level.coerceIn(0f, 1f), false, ring)
        }
        paint.alpha = 255
        paint.color = Color.WHITE
        paint.textAlign = Paint.Align.CENTER
        paint.textSize = radius * 0.5f
        val caption = when {
            !available && status == "idle" -> "禁用"
            gestureMode == MicBallView.MODE_TOGGLE -> if (status == "idle") "说话" else "结束"
            status == "preparing" -> "准备"
            else -> "按住"
        }
        c.drawText(caption, cx, cy - (paint.ascent() + paint.descent()) / 2, paint)
    }

    override fun onTouchEvent(e: MotionEvent): Boolean {
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                if (!hit(e.x, e.y)) return false
                parent?.requestDisallowInterceptTouchEvent(true)
                pointer = e.getPointerId(0)
                center().let { originX = it.first; originY = it.second }
                trigger.down(e.x, e.y, e.eventTime)
            }
            MotionEvent.ACTION_MOVE -> {
                val i = e.findPointerIndex(pointer); if (i < 0) return pointer >= 0
                trigger.move(e.getX(i), e.getY(i))
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_POINTER_UP -> {
                if (pointer < 0 || e.getPointerId(e.actionIndex) != pointer) return pointer >= 0
                trigger.up(e.getX(e.actionIndex), e.getY(e.actionIndex), e.eventTime)
                pointer = -1; performClick(); invalidate()
            }
            MotionEvent.ACTION_CANCEL -> cancel()
        }
        return true
    }

    fun cancel() { trigger.cancel(); pointer = -1; invalidate() }
    override fun onDetachedFromWindow() { cancel(); super.onDetachedFromWindow() }
    override fun performClick(): Boolean { super.performClick(); return true }
}
