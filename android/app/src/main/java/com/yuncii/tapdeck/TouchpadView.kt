package com.yuncii.tapdeck

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import android.view.Gravity
import android.widget.FrameLayout
import android.widget.ImageButton
import android.widget.ImageView
import kotlin.math.roundToInt
import kotlin.math.min

private fun Paint.fitLabel(text: String, size: Float, available: Float) {
    textAlign = Paint.Align.CENTER
    textSize = size
    val measured = measureText(text)
    if (measured > available) textSize *= available / measured
}

class TouchpadView(context: Context, private val client: TouchSink) : FrameLayout(context) {
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val density = resources.displayMetrics.density
    private val configuration = ViewConfiguration.get(context)
    private val gestures = TouchpadGestures(client, configuration.scaledTouchSlop / density.toDouble(), configuration.scaledDoubleTapSlop / density.toDouble())
    private val handler = Handler(Looper.getMainLooper())
    private val deadline = Runnable { if (connected) gestures.advance(SystemClock.uptimeMillis()); scheduleDeadline() }
    var onSensitivitySettings: () -> Unit = {}
    private val settingsButton = ImageButton(context).apply {
        id = R.id.touchpad_sensitivity_button
        contentDescription = "触控板灵敏度设置"
        minimumWidth = 0; minimumHeight = 0
        scaleType = ImageView.ScaleType.FIT_CENTER
        setImageResource(R.drawable.ic_lucide_sliders_horizontal)
        imageTintList = android.content.res.ColorStateList.valueOf(Color.rgb(160, 160, 160))
        val background = android.util.TypedValue()
        context.theme.resolveAttribute(android.R.attr.selectableItemBackgroundBorderless, background, true)
        setBackgroundResource(background.resourceId)
        setOnTouchListener { _, event -> if (event.actionMasked == MotionEvent.ACTION_DOWN) cancel(); false }
        setOnClickListener { cancel(); onSensitivitySettings() }
    }
    var verticalScale = 1f
        set(value) { if (field != value) { field = value; requestLayout(); invalidate() } }
    var connected = false
        set(value) { if (field != value) { if (!value) cancel(); field = value; invalidate() } }
    init {
        setWillNotDraw(false)
        isMotionEventSplittingEnabled = true
        importantForAccessibility = IMPORTANT_FOR_ACCESSIBILITY_NO
        // Separate targets let the icon and touchpad retain their own pointer IDs.
        addView(object : View(context) {
            init { isClickable = true; contentDescription = "触控板" }
            override fun onTouchEvent(event: MotionEvent) = this@TouchpadView.onTouchEvent(event)
        }, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT))
        addView(settingsButton, LayoutParams(0, 0, Gravity.TOP or Gravity.START))
    }
    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        val availableWidth = MeasureSpec.getSize(widthMeasureSpec)
        val availableHeight = MeasureSpec.getSize(heightMeasureSpec)
        val top = (availableWidth * 0.01f * verticalScale).roundToInt().coerceAtMost((availableHeight - 1).coerceAtLeast(0))
        val size = (availableWidth * 0.10f * verticalScale).roundToInt().coerceAtLeast(1).coerceAtMost((availableHeight - top).coerceAtLeast(1))
        val pad = (size * 0.225f).roundToInt()
        settingsButton.setPadding(pad, pad, pad, pad)
        (settingsButton.layoutParams as LayoutParams).apply {
            this.width = size; this.height = size
            marginStart = (availableWidth * 0.01f).roundToInt()
            topMargin = top
        }
        super.onMeasure(widthMeasureSpec, heightMeasureSpec)
    }
    override fun onDraw(c: Canvas) {
        paint.color = Color.BLACK; c.drawRect(0f, 0f, width.toFloat(), height.toFloat(), paint)
        val unit = width * verticalScale
        val availableWidth = (width * 0.96f).coerceAtLeast(1f)
        val title = if (connected) "触控板" else "连接电脑后使用触控板"
        val hints = listOf("轻点左击 · 双击按住拖拽 · 双指右击", "双指滚动 / 缩放 · 三指上下滑切换窗口")
        paint.fitLabel(title, unit * 0.05f, availableWidth)
        val titleSize = paint.textSize
        val titleMetrics = paint.fontMetrics
        paint.fitLabel(hints.maxBy { paint.measureText(it) }, unit * 0.029f, availableWidth)
        val hintSize = paint.textSize
        val hintMetrics = paint.fontMetrics
        val titleHeight = titleMetrics.descent - titleMetrics.ascent
        val hintHeight = hintMetrics.descent - hintMetrics.ascent
        val gap = unit * 0.014f
        val groupHeight = titleHeight + gap + hintHeight * hints.size
        val availableHeight = (height - unit * 0.024f).coerceAtLeast(0f)
        val fit = if (groupHeight > 0f) min(1f, availableHeight / groupHeight) else 0f
        val top = (height - groupHeight * fit) / 2f
        paint.color = Color.WHITE; paint.textSize = titleSize * fit
        c.drawText(title, width / 2f, top - titleMetrics.ascent * fit, paint)
        paint.color = Color.rgb(203, 213, 225)
        paint.textSize = hintSize * fit
        hints.forEachIndexed { index, hint -> c.drawText(hint, width / 2f, top + (titleHeight + gap + index * hintHeight - hintMetrics.ascent) * fit, paint) }
        paint.color = Color.rgb(60, 66, 74); paint.strokeWidth = density; c.drawLine(0f, height - density, width.toFloat(), height - density, paint)
    }
    override fun onTouchEvent(e: MotionEvent): Boolean {
        if (!connected) return true
        parent?.requestDisallowInterceptTouchEvent(true)
        val phase = when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> TouchPhase.Down
            MotionEvent.ACTION_POINTER_DOWN -> TouchPhase.Add
            MotionEvent.ACTION_MOVE -> TouchPhase.Move
            MotionEvent.ACTION_POINTER_UP -> TouchPhase.Remove
            MotionEvent.ACTION_UP -> TouchPhase.Up
            MotionEvent.ACTION_CANCEL -> TouchPhase.Cancel
            else -> return true
        }
        val points = (0 until e.pointerCount).map { TouchPoint(e.getPointerId(it), e.getX(it) / density.toDouble(), e.getY(it) / density.toDouble()) }
        gestures.accept(TouchFrame(e.eventTime, phase, points, e.getPointerId(e.actionIndex)))
        scheduleDeadline()
        if (phase == TouchPhase.Up) performClick()
        return true
    }
    private fun scheduleDeadline() {
        handler.removeCallbacks(deadline)
        if (connected) gestures.nextDeadline?.let { handler.postDelayed(deadline, (it - SystemClock.uptimeMillis()).coerceAtLeast(0)) }
    }
    fun cancel() { handler.removeCallbacks(deadline); gestures.cancel() }
    override fun onDetachedFromWindow() { cancel(); super.onDetachedFromWindow() }
    override fun performClick(): Boolean { super.performClick(); return true }
}
