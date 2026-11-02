package com.yuncii.tapdeck

import android.content.Context
import android.graphics.Color
import android.graphics.Rect
import android.graphics.drawable.GradientDrawable
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.util.TypedValue
import android.view.Choreographer
import android.view.Gravity
import android.view.MotionEvent
import android.view.SurfaceHolder
import android.view.SurfaceView
import android.view.View
import android.view.ViewConfiguration
import android.widget.FrameLayout
import android.widget.TextView
import kotlin.math.roundToInt

/**
 * 视频模式：整屏显示 PC 画面，画面上叠加触控板手势与语音悬浮球。
 *
 * 画面由 [VideoDecoder] 直接解码到 SurfaceView（硬件合成，延迟最低）；缩放与平移
 * 只改变 SurfaceView 的布局位置，由 [VideoViewport] 计算。手势与普通触控板完全相同，
 * 唯一区别是双指捏合在本地缩放画面，不再向 PC 发送 Ctrl+滚轮。
 */
class VideoModeView(
    context: Context,
    private val client: TouchSink,
    val decoder: VideoDecoder,
    beginMic: (String) -> Boolean,
    endMic: (Boolean) -> Unit,
    saveMicPosition: (Float, Float) -> Unit,
) : FrameLayout(context) {
    private val density = resources.displayMetrics.density
    val viewport = VideoViewport()
    var onExit: () -> Unit = {}
    var onRetry: () -> Unit = {}
    var connected = false
        set(v) { if (field != v) { if (!v) cancel(); field = v } }

    private val surface = SurfaceView(context).apply {
        holder.addCallback(object : SurfaceHolder.Callback {
            override fun surfaceCreated(h: SurfaceHolder) { decoder.setSurface(h.surface) }
            override fun surfaceChanged(h: SurfaceHolder, f: Int, w: Int, height: Int) { decoder.setSurface(h.surface) }
            override fun surfaceDestroyed(h: SurfaceHolder) { decoder.detachSurfaceBlocking() }
        })
    }

    /** 捏合交给视口；其余手势原样发往 PC。 */
    private val sink = object : TouchSink by client {
        override fun pinch(scale: Double, focusX: Double, focusY: Double, panX: Double, panY: Double): Boolean {
            viewport.pinch(scale, focusX * density, focusY * density, panX * density, panY * density)
            requestLayout()
            return true
        }
    }
    private val configuration = ViewConfiguration.get(context)
    private val gestures = TouchpadGestures(sink, configuration.scaledTouchSlop / density.toDouble(), configuration.scaledDoubleTapSlop / density.toDouble())
    private val handler = Handler(Looper.getMainLooper())
    private val deadline = Runnable { if (connected) gestures.advance(SystemClock.uptimeMillis()); scheduleDeadline() }

    private val touchLayer = object : View(context) {
        init { id = R.id.video_touch_layer; isClickable = true; contentDescription = "电脑画面：单指移动鼠标，轻点左击，双指滚动，捏合缩放画面" }
        override fun onTouchEvent(e: MotionEvent): Boolean = this@VideoModeView.touch(e)
        override fun performClick(): Boolean { super.performClick(); return true }
    }

    private fun pill(color: Int) = GradientDrawable().apply { cornerRadius = 18 * density; setColor(color) }
    private val statusButton = TextView(context).apply {
        id = R.id.video_exit_button
        setTextColor(Color.WHITE)
        setTextSize(TypedValue.COMPLEX_UNIT_SP, 13f)
        val pad = (10 * density).roundToInt()
        setPadding(pad, (6 * density).roundToInt(), pad, (6 * density).roundToInt())
        background = pill(0x99000000.toInt())
        maxLines = 1
        contentDescription = "退出视频模式"
        setOnClickListener { cancel(); onExit() }
    }
    private val message = TextView(context).apply {
        setTextColor(Color.WHITE)
        setTextSize(TypedValue.COMPLEX_UNIT_SP, 16f)
        gravity = Gravity.CENTER
        val pad = (16 * density).roundToInt()
        setPadding(pad, pad, pad, pad)
        background = pill(0xB3000000.toInt())
        setOnClickListener { if (state.phase == VideoPhase.Error) onRetry() }
    }
    val mic = FloatingMicView(context, beginMic, endMic, saveMicPosition).apply { id = R.id.video_mic }

    private var state = VideoState()
    private var rttMs = 0L
    private var lastStatus = 0L

    init {
        id = R.id.video_mode
        setBackgroundColor(Color.BLACK)
        isMotionEventSplittingEnabled = true
        addView(surface, LayoutParams(0, 0))
        addView(touchLayer, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT))
        addView(message, LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT, Gravity.CENTER))
        addView(statusButton, LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT, Gravity.TOP or Gravity.START).apply {
            leftMargin = (12 * density).roundToInt(); topMargin = (12 * density).roundToInt()
        })
        addView(mic, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT))
        render()
    }

    /** 控制端状态（主线程）：视频阶段、画面尺寸、RTT。 */
    fun update(video: VideoState, rtt: Long) {
        rttMs = rtt
        val format = video.format
        if (format != state.format && format != null) {
            decoder.setFormat(format.width, format.height)
            surface.holder.setFixedSize(format.width, format.height)
            requestLayout()
        }
        val changed = video.phase != state.phase || video.error != state.error || video.cursorVisible != state.cursorVisible
        state = video
        if (changed) render()
    }

    /** PC 光标位置（PC 屏幕像素，主线程）：画面平滑跟随，保证光标可见。 */
    fun cursor(c: VideoCursor) {
        if (c.visible) viewport.cursor(c.x.toDouble(), c.y.toDouble())
        frames.start()
    }

    private fun render() {
        val fps = decoder.fps
        statusButton.text = buildString {
            append("‹ 退出")
            if (state.phase == VideoPhase.Streaming) {
                append(" · ").append(fps).append(" fps · ").append(rttMs).append(" ms")
                if (!state.cursorVisible) append(" · 光标在其他显示器")
            }
        }
        val text = when (state.phase) {
            VideoPhase.Idle, VideoPhase.Starting -> if (connected) "正在打开电脑画面…" else "正在连接电脑…"
            VideoPhase.Error -> state.error + "\n点击重试"
            VideoPhase.Streaming -> ""
        }
        message.text = text
        message.visibility = if (text.isEmpty()) GONE else VISIBLE
    }

    private val frames = object : Choreographer.FrameCallback {
        private var running = false
        fun start() { if (!running && isAttachedToWindow) { running = true; Choreographer.getInstance().postFrameCallback(this) } }
        override fun doFrame(frameTimeNanos: Long) {
            running = false
            if (viewport.step()) { placeSurface(); start() }
            val now = SystemClock.uptimeMillis()
            if (now - lastStatus >= 500) { lastStatus = now; render() }
        }
    }
    private val statusTicker = object : Runnable {
        override fun run() { render(); if (isAttachedToWindow) handler.postDelayed(this, 1000) }
    }

    override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
        val w = MeasureSpec.getSize(widthMeasureSpec); val h = MeasureSpec.getSize(heightMeasureSpec)
        state.format?.let { viewport.resize(w.toDouble(), h.toDouble(), it.screenWidth.toDouble(), it.screenHeight.toDouble()) }
        surface.layoutParams.width = viewport.displayWidth.roundToInt().coerceAtLeast(1)
        surface.layoutParams.height = viewport.displayHeight.roundToInt().coerceAtLeast(1)
        super.onMeasure(widthMeasureSpec, heightMeasureSpec)
    }

    override fun onLayout(changed: Boolean, left: Int, top: Int, right: Int, bottom: Int) {
        super.onLayout(changed, left, top, right, bottom)
        placeSurface()
        if (Build.VERSION.SDK_INT >= 29) {
            // 尽量减少单指从屏幕边缘滑动时误触系统返回手势（系统限制每侧最多 200dp）。
            val h = bottom - top; val w = right - left
            val band = (200 * density).roundToInt().coerceAtMost(h); val edge = (32 * density).roundToInt()
            val y = (h - band) / 2
            touchLayer.systemGestureExclusionRects = listOf(Rect(0, y, edge, y + band), Rect(w - edge, y, w, y + band))
        }
    }

    private fun placeSurface() {
        if (!viewport.ready) { surface.layout(0, 0, 0, 0); return }
        val l = viewport.offsetX.roundToInt(); val t = viewport.offsetY.roundToInt()
        val w = viewport.displayWidth.roundToInt(); val h = viewport.displayHeight.roundToInt()
        if (surface.width != w || surface.height != h) { requestLayout(); return }
        surface.layout(l, t, l + w, t + h)
    }

    private fun touch(e: MotionEvent): Boolean {
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
        if (phase == TouchPhase.Up) touchLayer.performClick()
        return true
    }

    private fun scheduleDeadline() {
        handler.removeCallbacks(deadline)
        if (connected) gestures.nextDeadline?.let { handler.postDelayed(deadline, (it - SystemClock.uptimeMillis()).coerceAtLeast(0)) }
    }

    fun cancel() { handler.removeCallbacks(deadline); gestures.cancel(); mic.cancel() }

    override fun onAttachedToWindow() { super.onAttachedToWindow(); handler.post(statusTicker) }
    override fun onDetachedFromWindow() { cancel(); handler.removeCallbacks(statusTicker); super.onDetachedFromWindow() }
}
