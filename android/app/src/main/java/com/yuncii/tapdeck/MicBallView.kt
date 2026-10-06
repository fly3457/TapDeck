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
import kotlin.math.min

class MicBallView(
    context: Context,
    private val begin: (String) -> Boolean,
    private val end: (Boolean) -> Unit,
    private val savePosition: (Float, Float) -> Unit = { _, _ -> },
) : View(context) {
    companion object { const val HOLD_MS = 300L }
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val density = resources.displayMetrics.density
    private val slop = ViewConfiguration.get(context).scaledTouchSlop.toFloat()
    private val doubleSlop = ViewConfiguration.get(context).scaledDoubleTapSlop.toFloat()
    private var nx = 0.5f; private var ny = 0.5f
    private var restored = false; private var movedPosition = false
    private var pointer = -1
    private var downX = 0f; private var downY = 0f
    private var originX = 0f; private var originY = 0f
    private var downTime = 0L; private var lastTap = 0L
    private var tapX = 0f; private var tapY = 0f
    private var dragging = false; private var holdStarted = false; private var toggleAtDown = false
    var status = "idle"
        set(v) { if (field != v) { field = v; if (v == "idle") holdStarted = false; invalidate() } }
    var mode = ""
        set(v) { if (field != v) { field = v; invalidate() } }
    var available = false
        set(v) { if (field != v) { if (!v) cancel(); field = v; invalidate() } }
    var level = 0f
        set(v) { field = v; if (status != "idle") invalidate() }

    private val hold = Runnable {
        if (pointer >= 0 && !dragging && available && status == "idle") {
            lastTap = 0
            holdStarted = begin("hold")
            if (holdStarted) { mode = "hold"; if (status == "idle") status = "preparing" }
            invalidate()
        }
    }
    init { isClickable = true; contentDescription = "语音圆球：长按说话，双击免按，拖动调整位置" }

    private fun radius(): Float = (height * 0.22f).coerceIn(24 * density, 44 * density)
        .coerceAtMost(min(width / 2f - 8 * density, height * 0.56f / 2).coerceAtLeast(1f))
    private fun bounds(): RectF {
        val r = radius()
        return RectF(r + 8 * density, height * 0.20f + r, (width - r - 8 * density).coerceAtLeast(r + 8 * density), (height * 0.76f - r).coerceAtLeast(height * 0.20f + r))
    }
    fun ballCenter(): Pair<Float, Float> {
        val b = bounds()
        return (b.left + b.width() * nx) to (b.top + b.height() * ny)
    }
    fun normalizedPosition(): Pair<Float, Float> = nx to ny
    fun restorePosition(x: Float, y: Float) {
        if (!restored && !movedPosition && x.isFinite() && y.isFinite()) { nx = x.coerceIn(0f, 1f); ny = y.coerceIn(0f, 1f); restored = true; invalidate() }
    }
    private fun reposition(x: Float, y: Float) {
        val b = bounds()
        nx = if (b.width() > 0) ((x - b.left) / b.width()).coerceIn(0f, 1f) else 0.5f
        ny = if (b.height() > 0) ((y - b.top) / b.height()).coerceIn(0f, 1f) else 0.5f
        movedPosition = true; invalidate()
    }
    private fun label(c: Canvas, text: String, x: Float, y: Float, size: Float, maxWidth: Float) {
        paint.style = Paint.Style.FILL; paint.textAlign = Paint.Align.CENTER; paint.textSize = size
        val measured = paint.measureText(text)
        if (measured > maxWidth) paint.textSize *= maxWidth.coerceAtLeast(1f) / measured
        c.drawText(text, x, y, paint)
    }
    override fun onDraw(c: Canvas) {
        c.drawColor(Color.WHITE)
        val (cx, cy) = ballCenter(); val r = radius()
        paint.color = Color.rgb(63, 79, 96)
        val title = when (status) { "preparing" -> "准备中…"; "stopping" -> "正在结束…"; "transmitting" -> if (mode == "toggle") "免按录音 · 单击停止" else "长按录音 · 松手停止"; else -> if (available) "长按说话 · 双击免按" else "连接电脑后使用语音圆球" }
        label(c, title, width / 2f, height * 0.14f, min(14 * density, height * 0.09f), width - 24 * density)
        paint.color = when { status == "preparing" -> Color.rgb(186, 116, 11); status == "stopping" -> Color.rgb(112, 121, 135); status == "transmitting" && mode == "toggle" -> Color.rgb(23, 128, 97); status == "transmitting" -> Color.rgb(206, 53, 64); available -> Color.rgb(23, 92, 211); else -> Color.rgb(133, 146, 162) }
        paint.style = Paint.Style.FILL; c.drawCircle(cx, cy, r, paint)
        paint.color = Color.WHITE
        label(c, when (status) { "preparing" -> "准备"; "stopping" -> "结束"; "transmitting" -> if (mode == "toggle") "免按" else "按住"; else -> "说话" }, cx, cy + r * 0.17f, r * 0.45f, r * 1.65f)
        paint.color = Color.rgb(95, 111, 128)
        val foot = if (status == "transmitting") "麦克风电平 ${(level * 100).toInt()}% · 可拖动圆球" else "拖动圆球调整位置 · 仅圆球响应录音"
        label(c, foot, width / 2f, height * 0.90f, min(12 * density, height * 0.075f), width - 24 * density)
        if (status == "transmitting") { paint.color = Color.rgb(70, 159, 220); c.drawRect(0f, height - 4 * density, width * level.coerceIn(0f, 1f), height.toFloat(), paint) }
    }
    override fun onTouchEvent(e: MotionEvent): Boolean {
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                val (cx, cy) = ballCenter()
                if (hypot(e.x - cx, e.y - cy) > radius()) return false
                parent?.requestDisallowInterceptTouchEvent(true)
                pointer = e.getPointerId(0); downX = e.x; downY = e.y; originX = cx; originY = cy; downTime = e.eventTime
                dragging = false; holdStarted = false
                toggleAtDown = mode == "toggle" && status in listOf("preparing", "transmitting")
                if (available && status == "idle") postDelayed(hold, HOLD_MS)
                invalidate()
            }
            MotionEvent.ACTION_MOVE -> {
                val i = e.findPointerIndex(pointer); if (i < 0) return pointer >= 0
                val dx = e.getX(i) - downX; val dy = e.getY(i) - downY
                if (!dragging && hypot(dx, dy) > slop) { dragging = true; lastTap = 0; removeCallbacks(hold) }
                if (dragging) reposition(originX + dx, originY + dy)
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_POINTER_UP -> {
                if (pointer < 0 || e.getPointerId(e.actionIndex) != pointer) return pointer >= 0
                removeCallbacks(hold)
                if (holdStarted) { end(false); lastTap = 0 }
                else if (!dragging && toggleAtDown) { end(false); lastTap = 0 }
                else if (!dragging && available && status == "idle" && e.eventTime - downTime < HOLD_MS) {
                    if (lastTap > 0 && e.eventTime - lastTap <= ViewConfiguration.getDoubleTapTimeout() && hypot(downX - tapX, downY - tapY) <= doubleSlop) {
                        lastTap = 0
                        if (begin("toggle")) { mode = "toggle"; if (status == "idle") status = "preparing" }
                    }
                    else { lastTap = e.eventTime; tapX = downX; tapY = downY }
                } else lastTap = 0
                if (dragging) savePosition(nx, ny)
                pointer = -1; holdStarted = false; dragging = false; performClick(); invalidate()
            }
            MotionEvent.ACTION_CANCEL -> cancel()
        }
        return pointer >= 0 || e.actionMasked == MotionEvent.ACTION_UP
    }
    fun cancel() {
        removeCallbacks(hold)
        if (holdStarted) end(true)
        if (dragging) savePosition(nx, ny)
        pointer = -1; holdStarted = false; dragging = false; lastTap = 0; invalidate()
    }
    override fun onDetachedFromWindow() { cancel(); super.onDetachedFromWindow() }
    override fun performClick(): Boolean { super.performClick(); return true }
}
