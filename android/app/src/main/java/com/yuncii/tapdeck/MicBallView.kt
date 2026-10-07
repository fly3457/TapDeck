package com.yuncii.tapdeck

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.RectF
import android.text.TextPaint
import android.text.TextUtils
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import kotlin.math.hypot

/**
 * 语音触发控件。两种模式互斥，入口是本区顶部的模式按钮（Lucide 图标 + 文字）：
 * - [MODE_HOLD] 长按语音输入：图标 mic-audio-lines，圆形控件，按住说话、松手结束。
 * - [MODE_TOGGLE] 单击语音输入：图标 mic-signal，方形控件，轻点开始、再点结束。
 * 两种模式下都可以拖动控件调整位置。
 */
class MicBallView(
    context: Context,
    private val begin: (String) -> Boolean,
    private val end: (Boolean) -> Unit,
    private val savePosition: (Float, Float) -> Unit = { _, _ -> },
    private val saveMode: (String) -> Unit = {},
) : View(context) {
    companion object {
        const val HOLD_MS = 300L
        const val MODE_HOLD = "hold"
        const val MODE_TOGGLE = "toggle"
        val BACKGROUND = ControllerStyle.PANEL
    }
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val ring = Paint(Paint.ANTI_ALIAS_FLAG).apply { style = Paint.Style.STROKE }
    private val hintPaint = TextPaint(Paint.ANTI_ALIAS_FLAG)
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
    /** 顶部模式按钮的命中区域（每帧按实际排版更新）。 */
    private val modeButton = RectF()
    private var modePressed = false
    private val iconLines by lazy { context.getDrawable(R.drawable.ic_lucide_mic_audio_lines) }
    private val iconSignal by lazy { context.getDrawable(R.drawable.ic_lucide_mic_signal) }
    private var dragging = false
    private var holdStarted = false
    var onFeedback: (KeyFeedback) -> Unit = { keyFeedback(it) }
    var verticalScale = 1f
        set(v) { if (field != v) { field = v; invalidate() } }
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
                onFeedback(KeyFeedback.LongPress)
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

    private fun geometry() = VoiceGeometry(width.toFloat(), height.toFloat(), width * verticalScale)
    private fun radius(): Float = geometry().radius
    private fun bounds(): RectF {
        val g = geometry()
        return RectF(g.left, g.top, g.right, g.bottom)
    }
    fun ballCenter(): Pair<Float, Float> {
        return geometry().center(nx, ny)
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
        val g = geometry()
        val (cx, cy) = ballCenter()
        drawModeButton(c)
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
                ring.strokeWidth = g.unit * 0.01f
                ring.strokeCap = Paint.Cap.ROUND
                val r = radius() + g.unit * 0.007f
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
        // One caption and a separate level bar fit inside the 0.045W footer.
        hintPaint.color = ControllerStyle.SECONDARY
        hintPaint.textAlign = Paint.Align.CENTER
        hintPaint.typeface = android.graphics.Typeface.DEFAULT
        hintPaint.textSize = g.unit * 0.025f
        val hint = when {
            status == "transmitting" -> "麦克风电平 ${(level * 100).toInt()}% · 可拖动调整位置"
            available -> "可拖动到区域任意位置"
            else -> "连接电脑后使用语音输入"
        }
        val footerCaption = TextUtils.ellipsize(hint, hintPaint, (width - g.unit * 0.03f).coerceAtLeast(0f), TextUtils.TruncateAt.END).toString()
        val baseline = g.footerTop + (g.footerHeight - g.unit * 0.008f) / 2f - (hintPaint.ascent() + hintPaint.descent()) / 2f
        c.drawText(footerCaption, width / 2f, baseline, hintPaint)
        if (status == "transmitting") {
            paint.color = ControllerStyle.PRESSED
            c.drawRect(0f, height - g.unit * 0.004f, width * level.coerceIn(0f, 1f), height.toFloat(), paint)
        }
    }

    /** 两字说明用大字号，避免在方形控件里显得空。 */
    private fun captionSize(text: String): Float = if (text.length <= 2) radius() * 0.55f else radius() * 0.45f

    /**
     * 顶部一行：模式按钮（Lucide 图标 + 模式名）+ 当前状态的提示文字。
     * 按钮是唯一的模式切换入口：按住说话 / 轻点开始。
     */
    private fun drawModeButton(c: Canvas) {
        val toggle = gestureMode == MODE_TOGGLE
        val text = if (toggle) "单击语音输入" else "长按语音输入"
        val hint = when (status) {
            "preparing" -> "准备中…"
            "stopping" -> "正在结束…"
            "transmitting" -> if (mode == MODE_TOGGLE) "录音中，再点结束" else "录音中，松手结束"
            else -> when {
                !available -> "连接电脑后使用语音输入"
                toggle -> "轻点开始，再点结束"
                else -> "按住说话，松手结束"
            }
        }
        val g = geometry()
        val textSize = g.unit * 0.03f
        val iconSize = g.unit * 0.04f
        val padH = g.unit * 0.0125f
        val gap = g.unit * 0.01f
        hintPaint.textSize = textSize
        hintPaint.textAlign = Paint.Align.LEFT
        hintPaint.typeface = android.graphics.Typeface.DEFAULT_BOLD
        val labelWidth = hintPaint.measureText(text)
        hintPaint.typeface = android.graphics.Typeface.DEFAULT
        hintPaint.textSize = g.unit * 0.025f
        val hintWidth = hintPaint.measureText(hint)
        val buttonWidth = padH * 2 + iconSize + gap + labelWidth
        val total = buttonWidth + gap * 2 + hintWidth
        val left = ((width - total) / 2f).coerceAtLeast(width * ControllerStyle.SIDE)
        val pillHeight = g.unit * 0.058f
        val top = (g.modeHeight - pillHeight) / 2f
        val corner = width * ControllerStyle.CORNER

        modeButton.set(left, top, left + buttonWidth, top + pillHeight)
        paint.style = Paint.Style.FILL
        paint.color = Color.rgb(165, 168, 174)
        c.save()
        c.translate(0f, density)
        c.drawRoundRect(modeButton, corner, corner, paint)
        c.restore()
        paint.color = if (modePressed) ControllerStyle.PRESSED else ControllerStyle.NORMAL
        c.drawRoundRect(modeButton, corner, corner, paint)
        paint.style = Paint.Style.STROKE
        paint.strokeWidth = 0.5f * density
        paint.color = if (modePressed) ControllerStyle.PRESSED else ControllerStyle.BORDER
        c.drawRoundRect(modeButton, corner, corner, paint)
        paint.style = Paint.Style.FILL

        val icon = if (toggle) iconSignal else iconLines
        val iconLeft = (modeButton.left + padH).toInt()
        val iconTop = (modeButton.centerY() - iconSize / 2f).toInt()
        icon?.setBounds(iconLeft, iconTop, iconLeft + iconSize.toInt(), iconTop + iconSize.toInt())
        icon?.setTint(if (modePressed) Color.WHITE else ControllerStyle.LABEL)
        icon?.draw(c)

        hintPaint.textSize = textSize
        hintPaint.color = if (modePressed) Color.WHITE else ControllerStyle.LABEL
        hintPaint.textAlign = Paint.Align.LEFT
        hintPaint.typeface = android.graphics.Typeface.DEFAULT_BOLD
        c.drawText(text, modeButton.left + padH + iconSize + gap, modeButton.centerY() - (hintPaint.descent() + hintPaint.ascent()) / 2f, hintPaint)
        hintPaint.typeface = android.graphics.Typeface.DEFAULT
        hintPaint.textSize = g.unit * 0.025f
        hintPaint.color = ControllerStyle.SECONDARY
        val hintLeft = modeButton.right + gap * 2
        val fittedHint = TextUtils.ellipsize(hint, hintPaint, (width - width * ControllerStyle.SIDE - hintLeft).coerceAtLeast(0f), TextUtils.TruncateAt.END).toString()
        c.drawText(fittedHint, hintLeft, modeButton.centerY() - (hintPaint.descent() + hintPaint.ascent()) / 2f, hintPaint)
    }

    /** 点击模式按钮：在长按 / 单击之间切换，并通知外部记录到本地。 */
    private fun toggleMode() {
        gestureMode = if (gestureMode == MODE_TOGGLE) MODE_HOLD else MODE_TOGGLE
        saveMode(gestureMode)
        invalidate()
    }
    override fun onTouchEvent(e: MotionEvent): Boolean {
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                // 顶部模式按钮优先：命中就只切换模式，不进入录音 / 拖动手势。
                if (modeButton.contains(e.x, e.y)) {
                    onFeedback(KeyFeedback.Press)
                    pointer = -1; dragging = false; holdStarted = false
                    modePressed = true; invalidate()
                    return true
                }
                val (cx, cy) = ballCenter()
                if (!hit(e.x, e.y, cx, cy)) return false
                if (available) onFeedback(KeyFeedback.Press)
                parent?.requestDisallowInterceptTouchEvent(true)
                pointer = e.getPointerId(0); downX = e.x; downY = e.y; originX = cx; originY = cy; downTime = e.eventTime
                dragging = false; holdStarted = false
                if (gestureMode == MODE_HOLD && available && status == "idle") postDelayed(hold, HOLD_MS)
                invalidate()
            }
            MotionEvent.ACTION_MOVE -> {
                if (modePressed) return true
                val i = e.findPointerIndex(pointer); if (i < 0) return pointer >= 0
                val dx = e.getX(i) - downX; val dy = e.getY(i) - downY
                if (!dragging && hypot(dx, dy) > slop) { dragging = true; lastTap = 0; removeCallbacks(hold) }
                if (dragging) reposition(originX + dx, originY + dy)
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_POINTER_UP -> {
                if (modePressed) {
                    modePressed = false
                    if (modeButton.contains(e.x, e.y)) toggleMode() else invalidate()
                    performClick()
                    return true
                }
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
        pointer = -1; holdStarted = false; dragging = false; lastTap = 0; modePressed = false; invalidate()
    }
    override fun onDetachedFromWindow() { cancel(); super.onDetachedFromWindow() }
    override fun performClick(): Boolean { super.performClick(); return true }
}
