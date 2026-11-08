package com.yuncii.tapdeck

import kotlin.math.abs
import kotlin.math.hypot
import kotlin.math.ln
import kotlin.math.max
import kotlin.math.min

interface TouchSink {
    val doubleClickMs: Int get() = 500
    /** PC 设置的“自然滚动”；为真时手指上移内容随之上移（向下滚动）。 */
    val naturalScroll: Boolean get() = true
    fun move(dx: Double, dy: Double)
    fun scroll(dx: Double, dy: Double)
    fun button(name: String, down: Boolean)
    fun click(name: String = "left")
    fun zoom(steps: Int) {}
    /**
     * 连续捏合：scale 为相对上一帧的缩放比，focus 为双指中心，pan 为中心位移（dp）。
     * 返回 true 表示已在本地处理（视频模式缩放画面），不再发送 PC 缩放。
     */
    fun pinch(scale: Double, focusX: Double, focusY: Double, panX: Double, panY: Double): Boolean = false
    fun gesture(direction: String) {}
    /** 视频模式直接操作：单指拖动平移手机上的画面（dp），不移动鼠标。 */
    fun pan(dx: Double, dy: Double) {}
    /** 视频模式直接操作：单指拖动后抬手，vx／vy 为抬手瞬间的速度（dp/s），用于惯性滑动。 */
    fun fling(vx: Double, vy: Double) {}
    /** 视频模式直接操作：在触点位置单击，轻点为 left、长按为 right（dp）。 */
    fun clickAt(x: Double, y: Double, button: String) {}
    /** 视频模式直接操作：双指滚动开始前把鼠标移到双指中心（dp），滚轮才作用于手指下方的区域。 */
    fun hoverAt(x: Double, y: Double) {}
    /** 视频模式直接操作：双击后按住左键 down、拖动 move、松开 up（dp）。 */
    fun pointerAt(action: String, x: Double, y: Double) {}
}

internal data class TouchPoint(val id: Int, val x: Double, val y: Double)
internal enum class TouchPhase { Down, Add, Move, Remove, Up, Cancel }
internal data class TouchFrame(val time: Long, val phase: TouchPhase, val points: List<TouchPoint>, val changed: Int = points.firstOrNull()?.id ?: -1)

/**
 * Coordinates are dp; time is monotonic. Android only supplies events and the deadline timer.
 *
 * direct 为真时是视频模式的直接操作：单指拖动平移画面、轻点在该位置左击、长按在该位置右击，
 * 不再有相对移动与双击拖拽；双指与三指手势不变。
 */
internal class TouchpadGestures(
    private val sink: TouchSink,
    private val slop: Double,
    private val doubleTapSlop: Double,
    private val direct: Boolean = false,
) {
    private enum class Mode { Idle, Single, Second, Drag, Pan, Two, TwoEnding, Scroll, Pinch, Three, Consumed }
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
    // 直接操作：与触控板一致，首次轻点延迟 [TAP_WINDOW] 确认（pendingTap），期间再次按下即取消该次点击、
    // 在原位置按住左键——快速抬起补发一次点击构成双击，按住移动即拖拽（拖动窗口不会被双击最大化）。
    // directTap 为刚完成的双击，紧接着的轻点立即在同一位置按下构成三击。anchor 为按下位置。
    private var directTap: Tap? = null
    private var completeDouble = false
    private var anchor = TouchPoint(-1, 0.0, 0.0)
    private val origins = mutableMapOf<Int, TouchPoint>()
    private var ids = emptyList<Int>()
    // 拖动画面时最近的触点轨迹（时间、坐标），抬手时估算惯性速度。
    private val panTrail = ArrayDeque<Pair<Long, TouchPoint>>()
    // 本次按下只用于停住惯性滑动：不触发轻点左击和长按右击，拖动仍正常平移。
    private var stopOnly = false
    private val longPressDeadline: Long? get() = if (direct && mode == Mode.Single && !moved && !stopOnly) started + LONG_PRESS else null

    /** 视频模式：刚按下的这一指用于停住惯性，不当作点击或长按。须在 Down 帧之后调用。 */
    fun suppressTap() { if (mode == Mode.Single) stopOnly = true }
    val nextDeadline: Long? get() = listOfNotNull(longPressDeadline, pendingTap?.let { it.time + TAP_WINDOW }).minOrNull()

    fun advance(now: Long) {
        pendingTap?.let { if (now >= it.time + TAP_WINDOW) flushTap() }
        longPressDeadline?.let { if (now >= it) { sink.clickAt(origin.x, origin.y, "right"); mode = Mode.Consumed } }
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
        val near = { t: Tap? -> t != null && frame.time - t.time in 0 until TAP_WINDOW && distance(point, t.point) <= doubleTapSlop }
        val tap = if (direct && !near(pendingTap) && near(directTap)) directTap else pendingTap
        val second = near(tap)
        directTap = null
        if (second && direct) {
            // 在上一次轻点的位置按下：落点相同，Windows 才会判定为双击／三击。待确认的首次点击被取消，
            // 由本次按下充当，快速抬起时再补一次点击。
            completeDouble = tap === pendingTap
            pendingTap = null
            anchor = tap!!.point
            leftHeld = true
            sink.pointerAt("down", anchor.x, anchor.y)
            mode = Mode.Second
        } else if (second) {
            pendingTap = null
            leftHeld = true
            sink.button("left", true)
            mode = Mode.Second
        } else {
            flushTap()
            mode = Mode.Single
        }
        started = frame.time
        stopOnly = false; panTrail.clear()
        origin = point; last = if (direct && mode == Mode.Second) anchor else point; moved = false
        origins.clear(); origins[point.id] = point
        ids = listOf(point.id)
    }

    private fun add(frame: TouchFrame) {
        pendingTap = null
        directTap = null
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
            Mode.Single, Mode.Pan -> if (direct) {
                val point = points.singleOrNull() ?: return
                // 超过触摸阈值即为拖动画面；从按下点开始累计，阈值内的位移不会丢失。
                if (moved) {
                    if (mode != Mode.Pan) { panTrail.clear(); panTrail.addLast(started to last) }
                    mode = Mode.Pan
                    sink.pan(point.x - last.x, point.y - last.y)
                    last = point
                    recordPan(frame.time, point)
                }
            } else {
                val point = points.singleOrNull() ?: return
                if (moved) {
                    sink.move(point.x - last.x, point.y - last.y)
                    last = point
                }
            }
            Mode.Second, Mode.Drag -> if (direct) {
                val point = points.singleOrNull() ?: return
                // 双击后按住并移动：左键保持按下，跟随手指的绝对位置拖动（例如拖动窗口）。
                if (mode == Mode.Second && moved) mode = Mode.Drag
                if (mode == Mode.Drag) { sink.pointerAt("move", point.x, point.y); last = point }
            } else {
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
                    else if (pan >= slop) {
                        mode = Mode.Scroll
                        if (direct) sink.hoverAt(pairCenter.x, pairCenter.y)
                    }
                }
                when (mode) {
                    Mode.Scroll -> sink.scroll(nowCenter.x - lastCenter.x, nowCenter.y - lastCenter.y)
                    Mode.Pinch -> if (!sink.pinch(span / lastSpan, nowCenter.x, nowCenter.y, nowCenter.x - lastCenter.x, nowCenter.y - lastCenter.y)) {
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
            Mode.Pan -> if (direct) {
                // 抬手点通常与最后一次移动同位置；仍计入轨迹，停顿后再抬手时速度为 0。
                frame.points.singleOrNull()?.let { point ->
                    if (point.x != last.x || point.y != last.y) { sink.pan(point.x - last.x, point.y - last.y); last = point }
                    recordPan(frame.time, point)
                }
                val (vx, vy) = panVelocity(frame.time)
                sink.fling(vx, vy)
            }
            Mode.Single -> if (direct) {
                // 快速轻点等待可能的第二次按下；稍慢的轻点（未到长按）立即单击。
                if (stopOnly) Unit
                else if (!moved && elapsed in 0 until TAP_WINDOW) pendingTap = Tap(frame.time, origin)
                else if (!moved && elapsed in 0 until LONG_PRESS) sink.clickAt(origin.x, origin.y, "left")
            } else if (!moved && elapsed in 0 until TAP_WINDOW) pendingTap = Tap(frame.time, origin)
            Mode.Second, Mode.Drag -> if (direct) {
                val held = mode == Mode.Second
                releaseLeft()
                if (held && !moved) {
                    // 与触控板相同：第二次快速抬起才补发点击构成双击；按住不动后松开不会打开文件。
                    val shortLimit = min(TAP_WINDOW, (sink.doubleClickMs - 50).coerceAtLeast(1).toLong())
                    if (completeDouble && elapsed in 0 until shortLimit) sink.clickAt(anchor.x, anchor.y, "left")
                    // 继续轻点构成三击。
                    directTap = Tap(frame.time, anchor)
                }
            } else if (mode == Mode.Second) {
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

    private fun recordPan(time: Long, point: TouchPoint) {
        panTrail.addLast(time to point)
        while (panTrail.size > 2 && time - panTrail.first().first > VELOCITY_WINDOW) panTrail.removeFirst()
    }

    /**
     * 抬手速度（dp/s）：取抬手前 [VELOCITY_WINDOW] 内最早的轨迹点到抬手点的平均速度。
     * 手指停住超过该窗口后再抬起（窗口内没有更早的点）时速度为 0，不会滑动。
     */
    private fun panVelocity(now: Long): Pair<Double, Double> {
        val end = panTrail.lastOrNull() ?: return 0.0 to 0.0
        val start = panTrail.firstOrNull { now - it.first <= VELOCITY_WINDOW } ?: return 0.0 to 0.0
        val dt = (end.first - start.first) / 1000.0
        // 窗口内的轨迹末端到抬手之间停住过：按停顿时长摊薄速度。
        val span = (now - start.first) / 1000.0
        if (dt < MIN_VELOCITY_SPAN / 1000.0 || span <= 0) return 0.0 to 0.0
        return (end.second.x - start.second.x) / span to (end.second.y - start.second.y) / span
    }

    private fun flushTap() {
        val tap = pendingTap ?: return
        pendingTap = null
        if (direct) sink.clickAt(tap.point.x, tap.point.y, "left") else sink.click("left")
    }
    private fun releaseLeft() {
        if (!leftHeld) return
        leftHeld = false
        if (direct) sink.pointerAt("up", last.x, last.y) else sink.button("left", false)
    }
    fun cancel() {
        pendingTap = null; directTap = null; releaseLeft()
        mode = Mode.Idle; origins.clear(); ids = emptyList(); zoomRemainder = 0.0
    }
    private fun distance(a: TouchPoint, b: TouchPoint) = hypot(a.x - b.x, a.y - b.y)
    private fun center(points: List<TouchPoint>) = TouchPoint(-1, points.sumOf { it.x } / points.size, points.sumOf { it.y } / points.size)
    companion object {
        const val TAP_WINDOW = 300L
        /** 视频模式长按右击的判定时间。 */
        const val LONG_PRESS = 500L
        /** 估算抬手速度所用的轨迹时长（ms）。 */
        const val VELOCITY_WINDOW = 100L
        /** 轨迹短于该时长（ms）时不估算速度，避免单个抖动样本产生巨大速度。 */
        const val MIN_VELOCITY_SPAN = 10L
    }
}

/** Windows vertical and horizontal wheels have different positive directions. */
internal fun scrollUnits(dx: Double, dy: Double, natural: Boolean): Pair<Long, Long> {
    val direction = if (natural) 1 else -1
    return (-dx * 5 * direction * 1024).toLong() to (dy * 5 * direction * 1024).toLong()
}
