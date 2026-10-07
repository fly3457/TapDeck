package com.yuncii.tapdeck

import kotlin.math.abs
import kotlin.math.hypot
import kotlin.math.ln
import kotlin.math.max
import kotlin.math.min

interface TouchSink {
    val doubleClickMs: Int get() = 500
    fun move(dx: Double, dy: Double)
    fun scroll(dx: Double, dy: Double)
    fun button(name: String, down: Boolean)
    fun click(name: String = "left")
    fun zoom(steps: Int) {}
    fun gesture(direction: String) {}
}

internal data class TouchPoint(val id: Int, val x: Double, val y: Double)
internal enum class TouchPhase { Down, Add, Move, Remove, Up, Cancel }
internal data class TouchFrame(val time: Long, val phase: TouchPhase, val points: List<TouchPoint>, val changed: Int = points.firstOrNull()?.id ?: -1)

/** Coordinates are dp; time is monotonic. Android only supplies events and the deadline timer. */
internal class TouchpadGestures(
    private val sink: TouchSink,
    private val slop: Double,
    private val doubleTapSlop: Double,
) {
    private enum class Mode { Idle, Single, Second, Drag, Two, TwoEnding, Scroll, Pinch, Three, Consumed }
    private data class Tap(val time: Long, val point: TouchPoint)
    private var mode = Mode.Idle
    private var started = 0L
    private var origin = TouchPoint(-1, 0.0, 0.0)
    private var last = origin
    private var pairCenter = origin
    private var lastCenter = origin
    private var initialSpan = 1.0
    private var lastSpan = 1.0
    private var zoomRemainder = 0.0
    private var moved = false
    private var leftHeld = false
    private var pendingTap: Tap? = null
    private val origins = mutableMapOf<Int, TouchPoint>()
    private var ids = emptyList<Int>()
    val nextDeadline: Long? get() = pendingTap?.let { it.time + TAP_WINDOW }

    fun advance(now: Long) {
        if (nextDeadline?.let { now >= it } == true) flushTap()
    }

    fun accept(frame: TouchFrame) {
        if (frame.phase == TouchPhase.Cancel) { cancel(); return }
        advance(frame.time)
        when (frame.phase) {
            TouchPhase.Down -> down(frame)
            TouchPhase.Add -> add(frame)
            TouchPhase.Move -> move(frame)
            TouchPhase.Remove -> remove(frame)
            TouchPhase.Up -> up(frame)
            TouchPhase.Cancel -> Unit
        }
    }

    private fun down(frame: TouchFrame) {
        releaseLeft()
        val point = frame.points.singleOrNull() ?: run { cancel(); return }
        val tap = pendingTap
        val second = tap != null && frame.time - tap.time in 0 until TAP_WINDOW && distance(point, tap.point) <= doubleTapSlop
        if (second) {
            pendingTap = null
            leftHeld = true
            sink.button("left", true)
            mode = Mode.Second
        } else {
            flushTap()
            mode = Mode.Single
        }
        started = frame.time
        origin = point; last = point; moved = false
        origins.clear(); origins[point.id] = point
        ids = listOf(point.id)
    }

    private fun add(frame: TouchFrame) {
        pendingTap = null
        releaseLeft()
        if (mode == Mode.Idle || mode == Mode.Consumed || mode == Mode.TwoEnding) { mode = Mode.Consumed; return }
        track(frame.points)
        ids = frame.points.map { it.id }.sorted()
        when (ids.size) {
            2 -> {
                val points = ordered(frame.points) ?: return
                pairCenter = center(points); lastCenter = pairCenter
                initialSpan = distance(points[0], points[1]).coerceAtLeast(1.0)
                lastSpan = initialSpan; zoomRemainder = 0.0
                mode = Mode.Two
            }
            3 -> { pairCenter = center(frame.points); mode = Mode.Three }
            else -> mode = Mode.Consumed
        }
    }

    private fun track(points: List<TouchPoint>) {
        points.forEach { point ->
            val first = origins.getOrPut(point.id) { point }
            if (distance(point, first) >= slop) moved = true
        }
    }

    private fun ordered(points: List<TouchPoint>): List<TouchPoint>? {
        if (points.size != ids.size) { mode = Mode.Consumed; return null }
        val byId = points.associateBy { it.id }
        return ids.map { byId[it] ?: run { mode = Mode.Consumed; return null } }
    }

    private fun move(frame: TouchFrame) {
        if (mode == Mode.Idle || mode == Mode.Consumed || mode == Mode.TwoEnding) return
        track(frame.points)
        val points = ordered(frame.points) ?: return
        when (mode) {
            Mode.Single, Mode.Second, Mode.Drag -> {
                val point = points.singleOrNull() ?: return
                if (mode == Mode.Second && moved) mode = Mode.Drag
                // Buffer tap jitter: the double-click's mouse position stays stable.
                if (moved || mode == Mode.Drag) {
                    sink.move(point.x - last.x, point.y - last.y)
                    last = point
                }
            }
            Mode.Two, Mode.Scroll, Mode.Pinch -> {
                val nowCenter = center(points)
                val span = distance(points[0], points[1]).coerceAtLeast(1.0)
                if (mode == Mode.Two) {
                    val pan = distance(nowCenter, pairCenter)
                    val spread = abs(span - initialSpan)
                    if (spread >= max(8.0, initialSpan * 0.04) && spread > pan * 1.2) mode = Mode.Pinch
                    else if (pan >= slop) mode = Mode.Scroll
                }
                when (mode) {
                    Mode.Scroll -> sink.scroll(nowCenter.x - lastCenter.x, nowCenter.y - lastCenter.y)
                    Mode.Pinch -> {
                        zoomRemainder += ln(span / lastSpan) / ln(1.12)
                        val steps = zoomRemainder.toInt()
                        if (steps != 0) { sink.zoom(steps); zoomRemainder -= steps }
                    }
                    else -> Unit
                }
                // Keep the original baseline until a gesture wins the dead zone.
                if (mode != Mode.Two) { lastCenter = nowCenter; lastSpan = span }
            }
            Mode.Three -> {
                val now = center(points)
                val dy = now.y - pairCenter.y
                if (abs(dy) >= 32.0 && abs(dy) >= abs(now.x - pairCenter.x) * 1.5) {
                    sink.gesture(if (dy < 0) "up" else "down")
                    mode = Mode.Consumed
                }
            }
            else -> Unit
        }
    }

    private fun remove(frame: TouchFrame) {
        track(frame.points)
        mode = if (mode == Mode.Two || mode == Mode.TwoEnding) Mode.TwoEnding else Mode.Consumed
        releaseLeft()
        // Never reinterpret the surviving fingers as another gesture.
    }

    private fun up(frame: TouchFrame) {
        track(frame.points)
        val elapsed = frame.time - started
        when (mode) {
            Mode.Single -> if (!moved && elapsed in 0 until TAP_WINDOW) pendingTap = Tap(frame.time, origin)
            Mode.Second -> {
                releaseLeft()
                // The first PC click starts on the SECOND finger-down. Complete the
                // second PC click only now; a held second tap can never open a file.
                val shortLimit = min(TAP_WINDOW, (sink.doubleClickMs - 50).coerceAtLeast(1).toLong())
                if (!moved && elapsed in 0 until shortLimit) sink.click("left")
            }
            Mode.Two, Mode.TwoEnding -> if (!moved && elapsed in 0 until TAP_WINDOW) sink.click("right")
            else -> Unit
        }
        releaseLeft()
        mode = Mode.Idle; origins.clear(); ids = emptyList()
    }

    private fun flushTap() {
        if (pendingTap != null) { pendingTap = null; sink.click("left") }
    }
    private fun releaseLeft() { if (leftHeld) { leftHeld = false; sink.button("left", false) } }
    fun cancel() {
        pendingTap = null; releaseLeft()
        mode = Mode.Idle; origins.clear(); ids = emptyList(); zoomRemainder = 0.0
    }
    private fun distance(a: TouchPoint, b: TouchPoint) = hypot(a.x - b.x, a.y - b.y)
    private fun center(points: List<TouchPoint>) = TouchPoint(-1, points.sumOf { it.x } / points.size, points.sumOf { it.y } / points.size)
    companion object { const val TAP_WINDOW = 300L }
}

/** Windows vertical and horizontal wheels have different positive directions. */
internal fun scrollUnits(dx: Double, dy: Double, natural: Boolean): Pair<Long, Long> {
    val direction = if (natural) 1 else -1
    return (-dx * 5 * direction * 1024).toLong() to (dy * 5 * direction * 1024).toLong()
}
