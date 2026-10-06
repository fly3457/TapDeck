package com.yuncii.tapdeck

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.view.MotionEvent
import android.view.View
import android.view.ViewConfiguration
import kotlin.math.abs

private fun Canvas.centeredLabel(text: String, paint: Paint, size: Float, width: Int, y: Float, density: Float) {
    paint.textAlign = Paint.Align.CENTER
    paint.textSize = size * density
    val available = (width - 24 * density).coerceAtLeast(1f)
    val measured = paint.measureText(text)
    if (measured > available) paint.textSize *= available / measured
    drawText(text, width / 2f, y, paint)
}

interface TouchSink {
    fun move(dx: Double, dy: Double)
    fun scroll(dx: Double, dy: Double)
    fun button(name: String, down: Boolean)
    fun click(name: String = "left")
}
class TouchpadView(context: Context, private val client: TouchSink) : View(context) {
    private val paint = Paint(Paint.ANTI_ALIAS_FLAG)
    private val density = resources.displayMetrics.density
    private val slop = ViewConfiguration.get(context).scaledTouchSlop
    private var lastX = 0f; private var lastY = 0f; private var downX = 0f; private var downY = 0f
    private var lastTapX = 0f; private var lastTapY = 0f; private var lastTap = 0L
    private var started = 0L; private var travel = 0f; private var multi = false; private var dragging = false
    var connected = false
        set(value) { if (field != value) { if (!value) cancel(); field = value; invalidate() } }
    init { isClickable = true; contentDescription = "触控板" }
    override fun onDraw(c: Canvas) {
        paint.color = Color.BLACK; c.drawRect(0f, 0f, width.toFloat(), height.toFloat(), paint)
        paint.color = Color.WHITE; paint.textAlign = Paint.Align.CENTER
        c.centeredLabel(if (connected) "触控板" else "连接电脑后使用触控板", paint, 21f, width, height / 2f, density)
        paint.color = Color.rgb(203, 213, 225)
        c.centeredLabel("轻点左击 · 双指右击 / 滚动 · 双击按住拖拽", paint, 13f, width, height / 2f + 32 * density, density)
        paint.color = Color.rgb(60, 66, 74); paint.strokeWidth = density; c.drawLine(0f, height - density, width.toFloat(), height - density, paint)
    }
    override fun onTouchEvent(e: MotionEvent): Boolean {
        if (!connected) return true
        parent.requestDisallowInterceptTouchEvent(true)
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> {
                started = e.eventTime; downX = e.x; downY = e.y; lastX = e.x; lastY = e.y; travel = 0f; multi = false
                dragging = e.eventTime - lastTap < ViewConfiguration.getDoubleTapTimeout() && abs(e.x - lastTapX) < slop * 4 && abs(e.y - lastTapY) < slop * 4
                if (dragging) client.button("left", true)
            }
            MotionEvent.ACTION_POINTER_DOWN -> {
                if (dragging) { client.button("left", false); dragging = false }
                multi = true; lastTap = 0; lastX = (e.getX(0) + e.getX(1)) / 2; lastY = (e.getY(0) + e.getY(1)) / 2
            }
            MotionEvent.ACTION_MOVE -> {
                if (e.pointerCount >= 2) {
                    val nx = (e.getX(0) + e.getX(1)) / 2; val ny = (e.getY(0) + e.getY(1)) / 2
                    val dx = nx - lastX; val dy = ny - lastY; travel += abs(dx) + abs(dy)
                    client.scroll((dx / density).toDouble(), (dy / density).toDouble()); lastX = nx; lastY = ny
                } else if (!multi) {
                    val dx = e.x - lastX; val dy = e.y - lastY; travel += abs(dx) + abs(dy)
                    client.move((dx / density).toDouble(), (dy / density).toDouble()); lastX = e.x; lastY = e.y
                }
            }
            MotionEvent.ACTION_UP -> {
                if (dragging) { client.button("left", false); dragging = false; lastTap = 0 }
                else if (travel < slop * 2 && e.eventTime - started < 350) {
                    client.click(if (multi) "right" else "left")
                    if (!multi) { lastTap = e.eventTime; lastTapX = downX; lastTapY = downY }
                } else lastTap = 0
                performClick()
            }
            MotionEvent.ACTION_CANCEL -> cancel()
        }
        return true
    }
    fun cancel() { if (dragging) client.button("left", false); dragging = false; lastTap = 0 }
    override fun onDetachedFromWindow() { cancel(); super.onDetachedFromWindow() }
    override fun performClick(): Boolean { super.performClick(); return true }
}
