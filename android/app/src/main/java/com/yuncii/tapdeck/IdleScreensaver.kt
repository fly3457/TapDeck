package com.yuncii.tapdeck

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Typeface
import android.os.SystemClock
import android.view.MotionEvent
import android.view.View
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlin.random.Random

/** 空闲黑屏屏保的本机偏好：默认关闭，空闲 5 分钟后黑屏。 */
data class IdleScreensaverSettings(val enabled: Boolean = false, val minutes: Int = DEFAULT_MINUTES) {
    companion object {
        val OPTIONS = listOf(1, 3, 5, 10, 30)
        const val DEFAULT_MINUTES = 5
        fun normalizeMinutes(value: Int?): Int = value?.takeIf { it in OPTIONS } ?: DEFAULT_MINUTES
    }
    val timeoutMs: Long? get() = if (enabled) normalizeMinutes(minutes) * 60_000L else null
}

/**
 * 空闲计时与唤醒：不依赖 Android 视图，便于单元测试。
 * - 手指按着、暂停（视频模式、语音输入、弹窗、离开前台）或未开启时不计时；
 * - 屏保显示时，第一下触碰只用来唤醒，整个手势（直到抬起）都不传给下面的控件。
 */
internal class IdleScreensaverController(
    private val schedule: (Long) -> Unit,
    private val cancel: () -> Unit,
    private val show: () -> Unit,
    private val hide: () -> Unit,
) {
    var showing = false
        private set
    private var timeoutMs: Long? = null
    private var paused = true
    private var touching = false
    private var swallowing = false

    fun configure(timeoutMs: Long?) {
        this.timeoutMs = timeoutMs
        if (timeoutMs == null) dismiss() else rearm()
    }

    fun setPaused(paused: Boolean) {
        if (this.paused == paused) return
        this.paused = paused
        if (paused) dismiss() else rearm()
    }

    /** 返回 true 表示这次触摸被屏保吃掉。 */
    fun onTouch(action: Int): Boolean {
        val end = action == MotionEvent.ACTION_UP || action == MotionEvent.ACTION_CANCEL
        touching = !end
        if (showing && action == MotionEvent.ACTION_DOWN) {
            swallowing = true
            dismiss()
        }
        if (swallowing) {
            if (end) { swallowing = false; rearm() }
            return true
        }
        if (touching) cancel() else rearm()
        return false
    }

    /** 实体按键：屏保显示时只唤醒；否则重新计时。 */
    fun onKey(): Boolean {
        if (showing) { dismiss(); return true }
        rearm()
        return false
    }

    fun onTimeout() {
        if (showing || touching || paused || timeoutMs == null) return
        showing = true
        show()
    }

    /** 设置页“预览屏保”：不受开关与时长影响，碰一下同样解除。 */
    fun preview() {
        if (showing) return
        cancel()
        showing = true
        show()
    }

    private fun dismiss() {
        cancel()
        if (showing) { showing = false; hide() }
        rearm()
    }

    private fun rearm() {
        cancel()
        val timeout = timeoutMs ?: return
        if (!showing && !touching && !paused) schedule(timeout)
    }
}

/** 在 [0, range] 之间往返的三角波：一直向前走的距离折算成碰边回弹后的位置。 */
internal fun bounce(distance: Float, range: Float): Float {
    if (range <= 0f || !distance.isFinite()) return 0f
    val period = range * 2f
    val p = ((distance % period) + period) % period
    return if (p <= range) p else period - p
}

/** 黑底上缓慢移动、碰到屏幕边缘回弹的时钟。OLED 黑色像素不发光，移动的文字也不会在固定位置烧屏。 */
internal class ScreensaverView(context: Context) : View(context) {
    private val density = resources.displayMetrics.density
    private val timePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        color = Color.rgb(140, 140, 140)
        textSize = 64f * density
        typeface = Typeface.create("sans-serif-light", Typeface.NORMAL)
        textAlign = Paint.Align.CENTER
    }
    private val datePaint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        color = Color.rgb(96, 96, 96)
        textSize = 16f * density
        textAlign = Paint.Align.CENTER
    }
    private val timeFormat = SimpleDateFormat("HH:mm", Locale.getDefault())
    private val dateFormat = SimpleDateFormat("M月d日 EEEE", Locale.CHINA)
    /** 每秒移动的距离（dp）：横竖速度不同，轨迹才能慢慢扫过整个屏幕。 */
    private val speedX = 22f * density
    private val speedY = 15f * density
    private var startedAt = 0L
    private var offsetX = 0f
    private var offsetY = 0f
    private val tick = object : Runnable {
        override fun run() { invalidate(); postOnAnimationDelayed(this, FRAME_MS) }
    }

    init {
        setBackgroundColor(Color.BLACK)
        visibility = GONE
        isClickable = true
        contentDescription = "空闲黑屏屏保，触碰屏幕恢复"
    }

    fun start() {
        startedAt = SystemClock.uptimeMillis()
        // 每次从随机位置出发，避免时钟总在同一处起步。
        offsetX = Random.nextFloat() * 4000f * density
        offsetY = Random.nextFloat() * 4000f * density
        visibility = VISIBLE
        removeCallbacks(tick)
        postOnAnimation(tick)
    }

    fun stop() {
        removeCallbacks(tick)
        visibility = GONE
    }

    override fun onDetachedFromWindow() { removeCallbacks(tick); super.onDetachedFromWindow() }

    override fun onDraw(canvas: Canvas) {
        super.onDraw(canvas)
        val now = Date()
        val time = timeFormat.format(now)
        val date = dateFormat.format(now)
        val gap = 6f * density
        val blockW = maxOf(timePaint.measureText(time), datePaint.measureText(date))
        val timeH = timePaint.fontMetrics.let { it.descent - it.ascent }
        val dateH = datePaint.fontMetrics.let { it.descent - it.ascent }
        val blockH = timeH + gap + dateH
        val seconds = (SystemClock.uptimeMillis() - startedAt) / 1000f
        val left = bounce(offsetX + speedX * seconds, width - blockW)
        val top = bounce(offsetY + speedY * seconds, height - blockH)
        val cx = left + blockW / 2f
        canvas.drawText(time, cx, top - timePaint.fontMetrics.ascent, timePaint)
        canvas.drawText(date, cx, top + timeH + gap - datePaint.fontMetrics.ascent, datePaint)
    }

    private companion object { const val FRAME_MS = 33L }
}
