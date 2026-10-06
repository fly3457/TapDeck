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

/**
 * 语音触发控件。两种模式互斥，由顶部的模式开关选择：
 * - [MODE_HOLD] 长按语音输入：控件是圆形，按住触发录音，松手结束。
 * - [MODE_TOGGLE] 单击语音输入：控件是方形，轻点开始，再轻点结束。
 * 两种模式下都可以拖动控件调整位置。
 */
class MicBallView(
    context: Context,
    private val begin: (String) -> Boolean,
    private val end: (Boolean) -> Unit,
    private val savePosition: (Float, Float) -> Unit = { _, _ -> },
) : View(context) {
    companion object {
        const val HOLD_MS = 300L
        const val MODE_HOLD = "hold"
        const val MODE_TOGGLE = "toggle"
        /** 语音输入区背景：偏深的米黄。 */
        val BACKGROUND = Color.rgb(253, 230, 175)
    }
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val ring = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE }
    private val hintPaint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val box = RectF()
    private val density = resources.displayMetrics.density
    private val slop = ViewConfiguration.get(context).scaledTouchSlop.toFloat()
    private var nx = 0.5f; private var ny = 0.42f
    private var restored = false; private var movedPosition = false
    private var pointer = -1
    private var downX = 0f; private var downY = 0f
    private var originX = 0f; private var originY = 0f
    private var downTime = 0L; private var lastTap = 0L
    private var tapX = 0f; private var tapY = 0f
    private var dragging = false
    private var holdStarted = false
    var status = "idle"
        set(v) { if (field != v) { field = v; if (v == "idle") holdStarted = false; invalidate() } }
    /** 服务端确认的录音模式，仅用于文案；形状由 [gestureMode] 决定。 */
    var mode = ""
        set(v) { if (field != v) { field = v; invalidate() } }
    var available = false
        set(v) { if (field != v) { if (!v) cancel(); field = v; invalidate() } }
    var level = 0f
        set(v) { field = v; if (status != "idle") invalidate() }

    /** 外部模式开关当前选中的模式；形状、手势与文案都由它决定。 */
    var gestureMode = MODE_HOLD
        set(v) { if (field != v) { field = v; invalidate() } }

    private val hold = Runnable {
        if (pointer >= 0 && !dragging && available && gestureMode == MODE_HOLD && status == "idle") {
            lastTap = 0
            holdStarted = begin(MODE_HOLD)
            if (holdStarted) {
                mode = MODE_HOLD
                if (status == "idle") status = "preparing"
            }
            invalidate()
        }
    }
    init {
        isClickable = true
        contentDescription = "语音输入区：按开关选择长按或单击，可拖动调整位置"
    }

    private fun radius(): Float = (height * 0.20f).coerceIn(24 * density, 38 * density)
        .coerceAtMost(min(width / 2f - 8 * density, controlHeight() / 2).coerceAtLeast(1f))
    /** 标题基线与录音控件可拖动范围的起点：顶部留出标题所需空间。 */
    private fun titleBaseline(): Float = topInset() + 18 * density
    private fun controlTop(): Float = titleBaseline() + 12 * density
    /** 底部为电平条 / 拖动提示留出空间，避免控件压住提示文字。 */
    private fun controlBottom(): Float = (height - bottomInset()).coerceAtLeast(controlTop())
    private fun controlHeight(): Float = (controlBottom() - controlTop()).coerceAtLeast(1f)
    // 顶部标题、底部提示行与电平条所占的高度，按区域高度取比例并设下限；
    // 区域本身只占屏幕 25%，因此这里保持紧凑，把空间留给可拖动的录音控件。
    private fun topInset(): Float = maxOf(height * 0.13f, 22 * density)
    private fun bottomInset(): Float = maxOf(height * 0.14f, 26 * density)
    private fun bounds(): RectF {
        val r = radius()
        val top = controlTop() + r
        val bottom = (controlBottom() - r).coerceAtLeast(top)
        return RectF(r + 8 * density, top, (width - r - 8 * density).coerceAtLeast(r + 8 * density), bottom)
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

    /** 形状只由当前模式决定：单击语音输入＝方形，长按语音输入＝圆形（空闲与录音中都是）。 */
    private fun square(): Boolean = gestureMode == MODE_TOGGLE
    private fun halfSize(): Float = radius()
    private fun shapeBounds(cx: Float, cy: Float): RectF {
        val h = halfSize()
        box.set(cx - h, cy - h, cx + h, cy + h)
        return box
    }
    private fun drawShape(c: Canvas, cx: Float, cy: Float, p: Paint) {
        if (square()) c.drawRoundRect(shapeBounds(cx, cy), halfSize() * 0.24f, halfSize() * 0.24f, p) else c.drawCircle(cx, cy, radius(), p)
    }
    private fun hit(x: Float, y: Float, cx: Float, cy: Float): Boolean =
        if (square()) x >= cx - halfSize() && x <= cx + halfSize() && y >= cy - halfSize() && y <= cy + halfSize()
        else hypot(x - cx, y - cy) <= radius()

    private fun label(c: Canvas, text: String, x: Float, y: Float, size: Float, maxWidth: Float, bold: Boolean = false) {
        paint.style = Paint.Style.FILL; paint.textAlign = Paint.Align.CENTER; paint.textSize = size
        paint.typeface = if (bold) android.graphics.Typeface.DEFAULT_BOLD else android.graphics.Typeface.DEFAULT
        val measured = paint.measureText(text)
        if (measured > maxWidth) paint.textSize *= maxWidth.coerceAtLeast(1f) / measured
        c.drawText(text, x, y, paint)
    }
    override fun onDraw(c: Canvas) {
        c.drawColor(BACKGROUND)
        val (cx, cy) = ballCenter()
        val title = when (status) {
            "preparing" -> if (gestureMode == MODE_TOGGLE) "单击语音输入 · 准备中…" else "长按语音输入 · 准备中…"
            "stopping" -> "正在结束…"
            "transmitting" -> if (mode == MODE_TOGGLE) "单击录音中 · 轻点停止" else "长按录音中 · 松手停止"
            else -> if (available) {
                if (gestureMode == MODE_TOGGLE) "单击语音输入 · 轻点开始 / 再轻点结束" else "长按语音输入 · 按住说话，松手结束"
            } else "连接电脑后使用语音输入"
        }
        paint.color = Color.rgb(63, 79, 96)
        label(c, title, width / 2f, titleBaseline(), min(13 * density, 22 * density), width - 24 * density, bold = true)
        paint.style = Paint.Style.FILL
        paint.color = when {
            status == "preparing" -> Color.rgb(186, 116, 11)
            status == "stopping" -> Color.rgb(112, 121, 135)
            status == "transmitting" && mode == MODE_TOGGLE -> Color.rgb(23, 128, 97)
            status == "transmitting" -> Color.rgb(206, 53, 64)
            available -> Color.rgb(23, 92, 211)
            else -> Color.rgb(133, 146, 162)
        }
        drawShape(c, cx, cy, paint)
        if (status == "transmitting") {
            val progress = level.coerceIn(0f, 1f)
            if (progress > 0f) {
                ring.color = Color.rgb(70, 159, 220)
                ring.strokeWidth = 5 * density
                ring.strokeCap = Paint.Cap.ROUND
                val r = radius() + 7 * density
                if (square()) {
                    val inset = radius() - r
                    box.set(cx + inset, cy + inset, cx - inset, cy - inset)
                    c.drawArc(box, -90f, 360f * progress, false, ring)
                } else {
                    box.set(cx - r, cy - r, cx + r, cy + r)
                    c.drawArc(box, -90f, 360f * progress, false, ring)
                }
            }
        }
        paint.color = Color.WHITE
        val caption = when (status) {
            "preparing" -> "准备"
            "stopping" -> "结束"
            "transmitting" -> if (mode == MODE_TOGGLE) "单击" else "长按"
            else -> if (gestureMode == MODE_TOGGLE) "轻点" else "长按"
        }
        label(c, caption, cx, cy + radius() * 0.17f, captionSize(caption), radius() * 1.7f)
        // 底部三行：电平条在最上，其下是「麦克风电平」文字，提示文字固定在最下面。
        val hintBaseline = height - 8 * density
        val barTop = height - 22 * density
        hintPaint.color = Color.rgb(138, 120, 88)
        hintPaint.textAlign = Paint.Align.CENTER
        hintPaint.textSize = min(11 * density, 18 * density)
        c.drawText(if (available) "可拖动到区域任意位置" else "连接电脑后使用语音输入", width / 2f, hintBaseline, hintPaint)
        if (status == "transmitting") {
            paint.color = Color.rgb(120, 96, 60)
            label(c, "麦克风电平 ${(level * 100).toInt()}%", width / 2f, barTop + 14 * density, min(12 * density, height * 0.07f), width - 24 * density)
            paint.color = Color.rgb(70, 159, 220)
            c.drawRect(0f, barTop, width * level.coerceIn(0f, 1f), barTop + 5 * density, paint)
        }
    }

    /** 两字说明用大字号，避免在方形控件里显得空。 */
    private fun captionSize(text: String): Float = if (text.length <= 2) radius() * 0.55f else radius() * 0.45f
    override fun onTouchEvent(e: MotionEvent): Boolean {
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                val (cx, cy) = ballCenter()
                if (!hit(e.x, e.y, cx, cy)) return false
                parent?.requestDisallowInterceptTouchEvent(true)
                pointer = e.getPointerId(0); downX = e.x; downY = e.y; originX = cx; originY = cy; downTime = e.eventTime
                dragging = false; holdStarted = false
                if (gestureMode == MODE_HOLD && available && status == "idle") postDelayed(hold, HOLD_MS)
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
                val tapped = !dragging && hypot(e.x - downX, e.y - downY) <= slop && e.eventTime - downTime < HOLD_MS
                if (holdStarted) { end(false); lastTap = 0 }
                else if (dragging || !available) lastTap = 0
                else if (gestureMode == MODE_TOGGLE && tapped) {
                    // 免按模式：轻点开始，录音中再轻点结束。
                    if (status == "idle") { if (begin(MODE_TOGGLE)) { mode = MODE_TOGGLE; status = "preparing" } }
                    else end(false)
                    lastTap = 0
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
