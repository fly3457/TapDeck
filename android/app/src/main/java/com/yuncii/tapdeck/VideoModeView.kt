package com.yuncii.tapdeck

import android.content.Context
import android.graphics.Color
import android.graphics.Rect
import android.graphics.drawable.GradientDrawable
import android.hardware.Sensor
import android.hardware.SensorEvent
import android.hardware.SensorEventListener
import android.hardware.SensorManager
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
    // 改为直接点击后不再自动跟随光标平移，以免点击边缘时画面跳动；用户单指拖动画面。
    val viewport = VideoViewport().apply { followCursor = false }
    var onExit: () -> Unit = {}
    var onRetry: () -> Unit = {}
    var connected = false
        set(v) { if (field != v) { if (!v) cancel(); field = v } }

    private val surface = SurfaceView(context).apply {
        holder.addCallback(object : SurfaceHolder.Callback {
            override fun surfaceCreated(h: SurfaceHolder) { decoder.setSurface(h.surface) }
            // 旋转时系统可能在同一个 Surface 对象下换了新的缓冲队列：必须重新指定解码输出，否则画面停在最后一帧。
            override fun surfaceChanged(h: SurfaceHolder, f: Int, w: Int, height: Int) { decoder.surfaceChanged(h.surface) }
            override fun surfaceDestroyed(h: SurfaceHolder) { decoder.detachSurfaceBlocking() }
        })
    }

    /** 捏合交给视口；其余手势原样发往 PC。 */
    /** 在 PC 画面的 (x, y) 处单击：action 为 click 或 right_click，坐标为 PC 屏幕像素。 */
    var onPointer: (String, Int, Int) -> Unit = { _, _, _ -> }
    var onFeedback: (KeyFeedback) -> Unit = {}
    private val sink = object : TouchSink by client {
        override fun pinch(scale: Double, focusX: Double, focusY: Double, panX: Double, panY: Double): Boolean {
            viewport.pinch(scale, focusX * density, focusY * density, panX * density, panY * density)
            requestLayout()
            return true
        }
        override fun pan(dx: Double, dy: Double) {
            viewport.pan(dx * density, dy * density)
            placeSurface()
        }
        override fun pointerAt(action: String, x: Double, y: Double) {
            val (cx, cy) = viewport.toContentClamped(x * density, y * density) ?: return
            onPointer(action, cx.toInt(), cy.toInt())
        }
        override fun hoverAt(x: Double, y: Double) {
            val (cx, cy) = viewport.toContent(x * density, y * density) ?: return
            onPointer("move", cx.toInt(), cy.toInt())
        }
        override fun clickAt(x: Double, y: Double, button: String) {
            // 点在两侧黑边等画面之外时不操作电脑。
            val (cx, cy) = viewport.toContent(x * density, y * density) ?: return
            if (button == "right") onFeedback(KeyFeedback.LongPress)
            onPointer(if (button == "right") "right_click" else "click", cx.toInt(), cy.toInt())
        }
    }
    private val configuration = ViewConfiguration.get(context)
    // 直接操作：单指拖动平移画面、轻点左击、长按右击；双指滚动／缩放画面、双指轻点右击与三指手势不变。
    private val gestures = TouchpadGestures(sink, configuration.scaledTouchSlop / density.toDouble(), configuration.scaledDoubleTapSlop / density.toDouble(), direct = true)
    private val handler = Handler(Looper.getMainLooper())
    private val deadline = Runnable { if (connected) gestures.advance(SystemClock.uptimeMillis()); scheduleDeadline() }

    private val touchLayer = object : View(context) {
        init { id = R.id.video_touch_layer; isClickable = true; contentDescription = "电脑画面：单指拖动画面，轻点该位置左击，长按该位置右击，双指滚动，捏合缩放画面" }
        override fun onTouchEvent(e: MotionEvent): Boolean = this@VideoModeView.touch(e)
        override fun performClick(): Boolean { super.performClick(); return true }
    }

    private fun pill(color: Int) = GradientDrawable().apply { cornerRadius = 18 * density; setColor(color) }
    // 左上角连接信息：固定宽度，超出省略，图标不会因文字长短跳动；“光标在其他显示器”单独一行。
    private val statusLine = TextView(context).apply {
        setTextColor(Color.WHITE)
        setTextSize(TypedValue.COMPLEX_UNIT_SP, 13f)
        isSingleLine = true
        ellipsize = android.text.TextUtils.TruncateAt.END
    }
    private val cursorLine = TextView(context).apply {
        setTextColor(0xFFFFD27A.toInt())
        setTextSize(TypedValue.COMPLEX_UNIT_SP, 12f)
        isSingleLine = true
        ellipsize = android.text.TextUtils.TruncateAt.END
        text = "光标在其他显示器"
        visibility = GONE
    }
    private val statusButton = android.widget.LinearLayout(context).apply {
        id = R.id.video_exit_button
        orientation = android.widget.LinearLayout.VERTICAL
        gravity = Gravity.CENTER_VERTICAL
        minimumHeight = (36 * density).roundToInt()
        val pad = (10 * density).roundToInt()
        setPadding(pad, (6 * density).roundToInt(), pad, (6 * density).roundToInt())
        background = pill(0x99000000.toInt())
        isClickable = true
        contentDescription = "退出视频模式"
        addView(statusLine, android.widget.LinearLayout.LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT))
        addView(cursorLine, android.widget.LinearLayout.LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.WRAP_CONTENT))
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

    /** 底部叠加键盘的容器；MainActivity 放入 Compose 键盘，尺寸由 [videoKeyboardSize] 决定。 */
    val keyboardHost = FrameLayout(context).apply { id = R.id.video_keyboard_host; visibility = GONE; isMotionEventSplittingEnabled = true }
    /** 键盘切换时回调（用于释放仍按住的键）；scale 变化时回调给键盘纵向缩放。 */
    var onKeyboardChange: (VideoKeyboard) -> Unit = {}
    var onKeyboardSize: (VideoKeyboardSize?) -> Unit = {}
    var keyboard = VideoKeyboard.None
        private set
    private var keyboardSize: VideoKeyboardSize? = null

    private fun iconButton(viewId: Int, icon: Int, description: String, click: () -> Unit) = android.widget.ImageButton(context).apply {
        id = viewId
        setImageResource(icon)
        imageTintList = android.content.res.ColorStateList.valueOf(Color.WHITE)
        scaleType = android.widget.ImageView.ScaleType.FIT_CENTER
        val pad = (8 * density).roundToInt()
        setPadding(pad, pad, pad, pad)
        background = pill(0x99000000.toInt())
        contentDescription = description
        setOnClickListener { click() }
    }
    /** 点击铃铛：MainActivity 打开电脑通知列表。 */
    var onNotifications: () -> Unit = {}
    private val notificationBadge = TextView(context).apply {
        setTextColor(Color.WHITE)
        setTextSize(TypedValue.COMPLEX_UNIT_SP, 10f)
        gravity = Gravity.CENTER
        includeFontPadding = false
        minWidth = (16 * density).roundToInt()
        val pad = (3 * density).roundToInt()
        setPadding(pad, 0, pad, 0)
        background = GradientDrawable().apply { cornerRadius = 8 * density; setColor(0xFFE5484D.toInt()) }
        visibility = GONE
    }
    // 铃铛与普通模式顶栏一致：红色角标显示未读数，角标不拦截点击。
    private val notificationsButton = FrameLayout(context).apply {
        addView(iconButton(R.id.video_notifications_button, R.drawable.ic_lucide_bell, "电脑通知") { cancel(); onNotifications() },
            LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT))
        addView(notificationBadge, LayoutParams(LayoutParams.WRAP_CONTENT, (16 * density).roundToInt(), Gravity.TOP or Gravity.END))
    }
    private var unread = -1
    /** 未读通知数；为 0 时隐藏角标。 */
    fun setUnread(count: Int) {
        if (count == unread) return
        unread = count
        notificationBadge.text = if (count > 99) "99+" else count.toString()
        notificationBadge.visibility = if (count > 0) VISIBLE else GONE
        findViewById<View>(R.id.video_notifications_button).contentDescription = if (count > 0) "电脑通知，$count 条未读" else "电脑通知"
    }
    // “更多”是两段式开关：点一下在下方展开恢复默认比例与体感控制，再点一下收起；点菜单项或画面都不会收起。
    private var menuOpen = false
    private val moreButton = iconButton(R.id.video_more_button, R.drawable.ic_lucide_ellipsis, "展开更多画面操作") { showMenu(!menuOpen) }
    private val resetButton = iconButton(R.id.video_reset_button, R.drawable.ic_ratio_1_1, "恢复默认画面比例") {
        viewport.reset(aroundCursor = false); requestLayout()
    }
    private val motionButton = iconButton(R.id.video_motion_button, R.drawable.ic_lucide_rotate_3d, "开启体感控制") {
        if (gyroscope == null) android.widget.Toast.makeText(context, "这台手机没有陀螺仪，无法使用体感控制", android.widget.Toast.LENGTH_SHORT).show()
        else setMotion(!motion)
    }
    private val menu = android.widget.LinearLayout(context).apply {
        orientation = android.widget.LinearLayout.VERTICAL
        visibility = GONE
        val icon = (TOOLBAR_ICON_DP * density).roundToInt(); val gap = (TOOLBAR_GAP_DP * density).roundToInt()
        for ((i, b) in listOf(resetButton, motionButton).withIndex())
            addView(b, android.widget.LinearLayout.LayoutParams(icon, icon).apply { if (i > 0) topMargin = gap })
    }

    // 体感控制：转动手机由陀螺仪平移画面（见 MotionPan）；只在视频模式可见且开关打开时监听传感器。
    private val sensors = context.getSystemService(SensorManager::class.java)
    private val gyroscope: Sensor? = sensors?.getDefaultSensor(Sensor.TYPE_GYROSCOPE)
    var motion = false
        private set
    private var visibleNow = false
    private var gyroListening = false
    private var lastGyro = 0L
    private val motionPan = MotionPan()
    private val gyroListener = object : SensorEventListener {
        override fun onSensorChanged(e: SensorEvent) {
            val prev = lastGyro; lastGyro = e.timestamp
            if (prev == 0L || !connected || !viewport.ready) return
            val (dx, dy) = motionPan.step(e.values[0].toDouble(), e.values[1].toDouble(), display?.rotation ?: 0,
                (e.timestamp - prev) / 1e9, MotionPan.PAN_PER_RAD * maxOf(width, height))
            if (dx != 0.0 || dy != 0.0) { viewport.pan(dx, dy); placeSurface() }
        }
        override fun onAccuracyChanged(sensor: Sensor?, accuracy: Int) {}
    }

    fun setMotion(on: Boolean) {
        motion = on && gyroscope != null
        motionButton.background = pill(if (motion) ACTIVE_PILL else IDLE_PILL)
        motionButton.contentDescription = if (motion) "关闭体感控制" else "开启体感控制"
        updateMoreButton()
        updateGyro()
    }

    private fun showMenu(open: Boolean) {
        if (menuOpen == open) return
        menuOpen = open
        menu.visibility = if (open) VISIBLE else GONE
        moreButton.contentDescription = if (open) "收起更多画面操作" else "展开更多画面操作"
        updateMoreButton()
        requestLayout()
    }

    /** 菜单展开或体感控制开启时“更多”显示为蓝色。 */
    private fun updateMoreButton() { moreButton.background = pill(if (menuOpen || motion) ACTIVE_PILL else IDLE_PILL) }

    private fun updateGyro() {
        val want = motion && visibleNow && isAttachedToWindow
        if (want == gyroListening) return
        gyroListening = want
        if (want) { lastGyro = 0L; motionPan.reset(); sensors?.registerListener(gyroListener, gyroscope, SensorManager.SENSOR_DELAY_GAME) }
        else sensors?.unregisterListener(gyroListener)
    }

    override fun onVisibilityAggregated(isVisible: Boolean) {
        super.onVisibilityAggregated(isVisible)
        visibleNow = isVisible
        updateGyro()
    }

    private val shortcutsButton = iconButton(R.id.video_shortcuts_button, R.drawable.ic_lucide_layout_grid, "显示快捷键盘") {
        showKeyboard(if (keyboard == VideoKeyboard.Shortcuts) VideoKeyboard.None else VideoKeyboard.Shortcuts)
    }
    private val keyboardButton = iconButton(R.id.video_keyboard_button, R.drawable.ic_lucide_keyboard, "显示全键盘") {
        showKeyboard(if (keyboard == VideoKeyboard.Full) VideoKeyboard.None else VideoKeyboard.Full)
    }
    // 图标固定在屏幕右上角，右对齐：铃铛、快捷键盘、全键盘、更多（最右）。
    private val toolbar = android.widget.LinearLayout(context).apply {
        orientation = android.widget.LinearLayout.HORIZONTAL
        gravity = Gravity.CENTER_VERTICAL
        val icon = (TOOLBAR_ICON_DP * density).roundToInt(); val gap = (TOOLBAR_GAP_DP * density).roundToInt()
        for ((i, b) in listOf(notificationsButton, shortcutsButton, keyboardButton, moreButton).withIndex())
            addView(b, android.widget.LinearLayout.LayoutParams(icon, icon).apply { if (i > 0) leftMargin = gap })
    }

    /** 跟随悬浮球的快捷键面板容器；网格由屏幕方向决定（竖屏 2 列、横屏 2 行）。 */
    val shortcutHost = FrameLayout(context).apply { id = R.id.video_shortcut_host; visibility = GONE; isMotionEventSplittingEnabled = true }
    var onShortcutGrid: (ShortcutPanelGrid) -> Unit = {}
    /** PC 当前启用的快捷键数量，决定面板行列。 */
    var shortcutCount = 8
        set(v) { if (field != v) { field = v; requestLayout() } }
    private var shortcutGrid: ShortcutPanelGrid? = null

    private var state = VideoState()
    private var rttMs = 0L
    private var lastStatus = 0L

    init {
        id = R.id.video_mode
        setBackgroundColor(Color.BLACK)
        isMotionEventSplittingEnabled = true
        addView(surface, LayoutParams(0, 0))
        addView(touchLayer, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT))
        // 键盘只占自身区域，区域外的触摸仍落到下面的手势层。
        addView(keyboardHost, LayoutParams(0, 0, Gravity.BOTTOM or Gravity.CENTER_HORIZONTAL))
        addView(shortcutHost, LayoutParams(0, 0))
        addView(message, LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT, Gravity.CENTER))
        val margin = (12 * density).roundToInt()
        addView(statusButton, LayoutParams((STATUS_WIDTH_DP * density).roundToInt(), LayoutParams.WRAP_CONTENT, Gravity.TOP or Gravity.START).apply { leftMargin = margin; topMargin = margin })
        addView(toolbar, LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT, Gravity.TOP or Gravity.END).apply { rightMargin = margin; topMargin = margin })
        addView(menu, LayoutParams(LayoutParams.WRAP_CONTENT, LayoutParams.WRAP_CONTENT))
        if (gyroscope == null) motionButton.alpha = 0.4f
        addView(mic, LayoutParams(LayoutParams.MATCH_PARENT, LayoutParams.MATCH_PARENT))
        mic.onMoved = { placeShortcuts() }
        render()
    }

    /** 快捷键盘与全键盘互斥；再次点击当前键盘的图标即收起。 */
    fun showKeyboard(mode: VideoKeyboard) {
        if (keyboard == mode) return
        keyboard = mode
        onKeyboardChange(mode)
        keyboardHost.visibility = if (mode == VideoKeyboard.Full) VISIBLE else GONE
        shortcutHost.visibility = if (mode == VideoKeyboard.Shortcuts) VISIBLE else GONE
        shortcutsButton.background = pill(if (mode == VideoKeyboard.Shortcuts) ACTIVE_PILL else IDLE_PILL)
        keyboardButton.background = pill(if (mode == VideoKeyboard.Full) ACTIVE_PILL else IDLE_PILL)
        shortcutsButton.contentDescription = if (mode == VideoKeyboard.Shortcuts) "收起快捷键盘" else "显示快捷键盘"
        keyboardButton.contentDescription = if (mode == VideoKeyboard.Full) "收起全键盘" else "显示全键盘"
        requestLayout()
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
        statusLine.text = buildString {
            append("‹ 退出")
            if (state.phase == VideoPhase.Streaming) append(" · ").append(fps).append(" fps · ").append(rttMs).append(" ms")
        }
        cursorLine.visibility = if (state.phase == VideoPhase.Streaming && !state.cursorVisible) VISIBLE else GONE
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
        // 底部叠加只用于全键盘；快捷键改为跟随悬浮球的小面板。
        val size = videoKeyboardSize(w, h, if (keyboard == VideoKeyboard.Full) VideoKeyboard.Full else VideoKeyboard.None)
        val grid = shortcutPanelGrid(shortcutCount, landscape = w > h)
        val cellW = SHORTCUT_CELL_W_DP * density; val cellH = SHORTCUT_CELL_H_DP * density; val gap = SHORTCUT_GAP_DP * density
        shortcutHost.layoutParams.width = (grid.columns * cellW + (grid.columns + 1) * gap).roundToInt()
        shortcutHost.layoutParams.height = (grid.rows * cellH + (grid.rows + 1) * gap).roundToInt()
        if (grid != shortcutGrid) { shortcutGrid = grid; post { onShortcutGrid(grid) } }
        // 窄屏竖屏时连接信息让出宽度，不与右侧四个图标重叠。
        val margin = 12 * density
        val tools = (TOOLBAR_ICONS * TOOLBAR_ICON_DP + (TOOLBAR_ICONS - 1) * TOOLBAR_GAP_DP) * density
        statusButton.layoutParams.width = minOf(STATUS_WIDTH_DP * density, w - tools - 3 * margin).roundToInt().coerceAtLeast((96 * density).roundToInt())
        keyboardHost.layoutParams.width = size?.width ?: 0
        keyboardHost.layoutParams.height = size?.height ?: 0
        if (size != keyboardSize) { keyboardSize = size; post { onKeyboardSize(keyboardSize) } }
        super.onMeasure(widthMeasureSpec, heightMeasureSpec)
    }

    override fun onLayout(changed: Boolean, left: Int, top: Int, right: Int, bottom: Int) {
        super.onLayout(changed, left, top, right, bottom)
        placeSurface()
        placeShortcuts()
        // 菜单在“更多”按钮正下方，与其左对齐。
        if (menu.visibility == VISIBLE) {
            val x = toolbar.left + moreButton.left; val y = toolbar.bottom + (TOOLBAR_GAP_DP * density).roundToInt()
            menu.layout(x, y, x + menu.measuredWidth, y + menu.measuredHeight)
        }
        if (Build.VERSION.SDK_INT >= 29) {
            // 尽量减少单指从屏幕边缘滑动时误触系统返回手势（系统限制每侧最多 200dp）。
            val h = bottom - top; val w = right - left
            val band = (200 * density).roundToInt().coerceAtMost(h); val edge = (32 * density).roundToInt()
            val y = (h - band) / 2
            touchLayer.systemGestureExclusionRects = listOf(Rect(0, y, edge, y + band), Rect(w - edge, y, w, y + band))
        }
    }

    /** 快捷键面板跟随悬浮球：在球下方居中，下方放不下时改到上方。 */
    private fun placeShortcuts() {
        val w = shortcutHost.measuredWidth; val h = shortcutHost.measuredHeight
        if (w <= 0 || h <= 0 || mic.width <= 0) return
        val (bx, by) = mic.ballCenter()
        val (x, y) = shortcutPanelPosition(bx, by, mic.ballRadius, w.toFloat(), h.toFloat(), width.toFloat(), height.toFloat(), 8 * density)
        shortcutHost.layout(x.roundToInt(), y.roundToInt(), x.roundToInt() + w, y.roundToInt() + h)
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

    override fun onAttachedToWindow() { super.onAttachedToWindow(); handler.post(statusTicker); updateGyro() }
    override fun onDetachedFromWindow() { cancel(); handler.removeCallbacks(statusTicker); super.onDetachedFromWindow(); updateGyro() }
}

private const val STATUS_WIDTH_DP = 176
private const val TOOLBAR_ICONS = 4
private const val TOOLBAR_ICON_DP = 36
private const val TOOLBAR_GAP_DP = 8
private const val ACTIVE_PILL = 0xCC175CD3.toInt()
private const val IDLE_PILL = 0x99000000.toInt()
private const val SHORTCUT_CELL_W_DP = 62
private const val SHORTCUT_CELL_H_DP = 40
private const val SHORTCUT_GAP_DP = 4
