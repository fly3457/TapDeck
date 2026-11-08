package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test
import kotlin.math.hypot

/** 视频模式单指拖动的惯性：抬手速度估算、滑行距离与 2 倍上限、按下即停不误点。 */
class PanInertiaTest {
    private class Sink : TouchSink {
        val events = mutableListOf<String>()
        var fling: Pair<Double, Double>? = null
        override fun move(dx: Double, dy: Double) {}
        override fun scroll(dx: Double, dy: Double) {}
        override fun button(name: String, down: Boolean) {}
        override fun click(name: String) {}
        override fun fling(vx: Double, vy: Double) { fling = vx to vy }
        override fun clickAt(x: Double, y: Double, button: String) { events.add("at:$button") }
    }
    private fun p(x: Double, y: Double = 100.0) = TouchPoint(1, x, y)
    private fun TouchpadGestures.event(time: Long, phase: TouchPhase, point: TouchPoint) = accept(TouchFrame(time, phase, listOf(point), point.id))
    private fun gestures(sink: Sink) = TouchpadGestures(sink, 8.0, 40.0, direct = true)

    /** 按 10ms 一帧、每帧 step dp 向右拖动 frames 帧。 */
    private fun TouchpadGestures.drag(start: Long, frames: Int, step: Double): Long {
        event(start, TouchPhase.Down, p(100.0))
        var t = start; var x = 100.0
        repeat(frames) { t += 10; x += step; event(t, TouchPhase.Move, p(x)) }
        return t
    }

    /** 不碰边时一直推进到停止，返回总滑行距离。 */
    private fun glide(vx: Double, vy: Double = 0.0): Double {
        val inertia = PanInertia()
        if (!inertia.start(vx, vy)) return 0.0
        var sx = 0.0; var sy = 0.0; var frames = 0
        while (inertia.active && frames < 10_000) { val (dx, dy) = inertia.step(1 / 60.0); sx += dx; sy += dy; frames++ }
        return hypot(sx, sy)
    }

    @Test fun flingVelocityComesFromLastMovement() {
        val s = Sink(); val g = gestures(s)
        // 每 10ms 移动 15dp，即 1500dp/s。
        val t = g.drag(0, 10, 15.0)
        g.event(t + 5, TouchPhase.Up, p(250.0))
        val (vx, vy) = s.fling!!
        assertEquals(1500.0, vx, 120.0)
        assertEquals(0.0, vy, 1e-9)
        assertTrue("拖动不点击", s.events.isEmpty())
    }

    @Test fun pausingBeforeLiftGivesNoFling() {
        val s = Sink(); val g = gestures(s)
        val t = g.drag(0, 10, 15.0)
        // 手指停住 200ms 后再抬起：速度为 0，不滑动。
        g.event(t + 200, TouchPhase.Up, p(250.0))
        assertEquals(0.0, s.fling!!.first, 1e-9)
        assertFalse(PanInertia().start(s.fling!!.first, s.fling!!.second))
    }

    @Test fun fasterFlingGlidesFartherUpToTwiceNormal() {
        val normal = glide(PanInertia.NORMAL_SPEED)
        assertEquals(PanInertia.NORMAL_SPEED * PanInertia.TAU, normal, 15.0)
        val slow = glide(800.0)
        val fast = glide(2500.0)
        assertTrue(slow < normal && normal < fast)
        // 再快也只滑到普通甩动的 2 倍。
        val capped = glide(9000.0)
        assertEquals(normal * 2, capped, 30.0)
        assertEquals(capped, glide(20000.0), 1e-6)
        // 斜向甩动按合速度截断，方向不变。
        val inertia = PanInertia(); inertia.start(6000.0, 8000.0)
        assertEquals(PanInertia.MAX_SPEED, hypot(inertia.vx, inertia.vy), 1e-6)
        assertEquals(0.75, inertia.vx / inertia.vy, 1e-9)
    }

    @Test fun slowReleaseDoesNotGlideAndHaltStopsAxis() {
        assertEquals(0.0, glide(100.0), 0.0)
        val inertia = PanInertia(); inertia.start(1000.0, 1000.0)
        inertia.halt(x = true, y = false)
        assertEquals(0.0, inertia.vx, 0.0)
        assertTrue(inertia.active)
        inertia.halt(x = false, y = true)
        assertFalse(inertia.active)
    }

    @Test fun glideIsFrameRateIndependent() {
        fun run(dt: Double): Double {
            val i = PanInertia(); i.start(2000.0, 0.0); var s = 0.0
            repeat((1.0 / dt).toInt()) { s += i.step(dt).first }
            return s
        }
        assertEquals(run(1 / 60.0), run(1 / 120.0), 1.0)
    }

    @Test fun touchThatStopsGlideNeitherClicksNorLongPresses() {
        val s = Sink(); val g = gestures(s)
        g.event(1000, TouchPhase.Down, p(200.0))
        g.suppressTap()
        assertNull("停住惯性的这一下不触发长按右击", g.nextDeadline)
        g.advance(2000)
        g.event(2100, TouchPhase.Up, p(200.0))
        g.advance(5000)
        assertTrue(s.events.isEmpty())
        // 下一次普通轻点照常左击。
        g.event(6000, TouchPhase.Down, p(200.0)); g.event(6050, TouchPhase.Up, p(200.0)); g.advance(7000)
        assertEquals(listOf("at:left"), s.events)
    }
}
