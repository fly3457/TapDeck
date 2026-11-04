package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

/** 视频模式直接操作：单指拖动平移画面、轻点左击、长按右击，双指与三指不变。 */
class VideoDirectGesturesTest {
    private class Sink : TouchSink {
        val events = mutableListOf<String>()
        var panX = 0.0; var panY = 0.0
        override fun move(dx: Double, dy: Double) { events.add("move") }
        override fun scroll(dx: Double, dy: Double) { events.add("scroll") }
        override fun button(name: String, down: Boolean) { events.add("$name:$down") }
        override fun click(name: String) { events.add("click:$name") }
        override fun pan(dx: Double, dy: Double) { panX += dx; panY += dy; if (events.lastOrNull() != "pan") events.add("pan") }
        override fun clickAt(x: Double, y: Double, button: String) { events.add("at:$button:${x.toInt()},${y.toInt()}") }
        override fun hoverAt(x: Double, y: Double) { events.add("hover:${x.toInt()},${y.toInt()}") }
        override fun pointerAt(action: String, x: Double, y: Double) { if (action != "move" || events.lastOrNull()?.startsWith("ptr:move") != true) events.add("ptr:$action:${x.toInt()},${y.toInt()}") }
        override fun pinch(scale: Double, focusX: Double, focusY: Double, panX: Double, panY: Double): Boolean { events.add("pinch"); return true }
    }
    private fun p(id: Int = 1, x: Double = 100.0, y: Double = 100.0) = TouchPoint(id, x, y)
    private fun TouchpadGestures.event(time: Long, phase: TouchPhase, vararg points: TouchPoint, changed: Int = points.first().id) = accept(TouchFrame(time, phase, points.toList(), changed))
    private fun gestures(sink: Sink) = TouchpadGestures(sink, 8.0, 40.0, direct = true)

    @Test fun tapClicksAtTouchedPositionImmediately() {
        val s = Sink(); val g = gestures(s)
        g.event(100, TouchPhase.Down, p(x = 120.0, y = 340.0)); g.event(180, TouchPhase.Up, p(x = 122.0, y = 341.0))
        assertEquals(listOf("at:left:120,340"), s.events)
        assertNull(g.nextDeadline)
        // 相隔较久的第二次轻点是新的单击。
        g.event(900, TouchPhase.Down, p(x = 120.0, y = 340.0)); g.event(950, TouchPhase.Up, p(x = 120.0, y = 340.0))
        assertEquals(listOf("at:left:120,340", "at:left:120,340"), s.events)
    }

    @Test fun doubleAndTripleTapPressAtFirstTapPosition() {
        val s = Sink(); val g = gestures(s)
        g.event(100, TouchPhase.Down, p(x = 120.0, y = 340.0)); g.event(150, TouchPhase.Up, p(x = 120.0, y = 340.0))
        // 第二次落点略有偏差，仍按第一次的位置按下，Windows 才判定为双击。
        g.event(250, TouchPhase.Down, p(x = 124.0, y = 343.0)); g.event(300, TouchPhase.Up, p(x = 124.0, y = 343.0))
        g.event(400, TouchPhase.Down, p(x = 121.0, y = 339.0)); g.event(450, TouchPhase.Up, p(x = 121.0, y = 339.0))
        assertEquals(listOf("at:left:120,340", "ptr:down:120,340", "ptr:up:120,340", "ptr:down:120,340", "ptr:up:120,340"), s.events)
        assertNull("双击按住期间不触发长按右击", g.nextDeadline)
    }

    @Test fun doubleTapHoldAndMoveDragsWithLeftButton() {
        val s = Sink(); val g = gestures(s)
        g.event(100, TouchPhase.Down, p(x = 100.0, y = 100.0)); g.event(150, TouchPhase.Up, p(x = 100.0, y = 100.0))
        g.event(250, TouchPhase.Down, p(x = 100.0, y = 100.0))
        g.advance(2000)
        g.event(2100, TouchPhase.Move, p(x = 140.0, y = 160.0))
        g.event(2200, TouchPhase.Move, p(x = 180.0, y = 200.0))
        g.event(2300, TouchPhase.Up, p(x = 180.0, y = 200.0))
        assertEquals(listOf("at:left:100,100", "ptr:down:100,100", "ptr:move:140,160", "ptr:up:180,200"), s.events)
        assertTrue("拖拽没有平移画面", s.panX == 0.0)
        // 拖拽结束后的轻点是新的单击，不会接成三击。
        g.event(2400, TouchPhase.Down, p(x = 180.0, y = 200.0)); g.event(2450, TouchPhase.Up, p(x = 180.0, y = 200.0))
        assertEquals("at:left:180,200", s.events.last())
    }

    @Test fun cancelDuringDragReleasesButton() {
        val s = Sink(); val g = gestures(s)
        g.event(100, TouchPhase.Down, p()); g.event(150, TouchPhase.Up, p())
        g.event(250, TouchPhase.Down, p()); g.event(260, TouchPhase.Move, p(x = 150.0))
        g.accept(TouchFrame(270, TouchPhase.Cancel, emptyList()))
        assertEquals("ptr:up:150,100", s.events.last())
    }

    @Test fun longPressRightClicksOnceAtPosition() {
        val s = Sink(); val g = gestures(s)
        g.event(100, TouchPhase.Down, p(x = 50.0, y = 60.0))
        assertEquals(100L + TouchpadGestures.LONG_PRESS, g.nextDeadline)
        g.advance(599); assertTrue(s.events.isEmpty())
        g.advance(600); g.advance(900)
        g.event(1000, TouchPhase.Up, p(x = 50.0, y = 60.0))
        assertEquals(listOf("at:right:50,60"), s.events)
    }

    @Test fun dragPansWithoutMovingMouseOrClicking() {
        val s = Sink(); val g = gestures(s)
        g.event(100, TouchPhase.Down, p())
        g.event(120, TouchPhase.Move, p(x = 104.0))
        assertTrue(s.events.isEmpty())
        g.event(140, TouchPhase.Move, p(x = 130.0, y = 90.0))
        assertNull("拖动后不再长按右击", g.nextDeadline)
        g.event(700, TouchPhase.Move, p(x = 150.0, y = 80.0)); g.advance(2000)
        g.event(800, TouchPhase.Up, p(x = 150.0, y = 80.0))
        assertEquals(listOf("pan"), s.events)
        assertEquals(50.0, s.panX, 1e-9); assertEquals(-20.0, s.panY, 1e-9)
    }

    @Test fun twoFingerGesturesAreUnchanged() {
        val s = Sink(); val g = gestures(s)
        g.event(100, TouchPhase.Down, p(1, 100.0)); g.event(110, TouchPhase.Add, p(1, 100.0), p(2, 200.0), changed = 2)
        g.event(130, TouchPhase.Move, p(1, 100.0, 140.0), p(2, 200.0, 140.0))
        // 滚动开始前先把鼠标移到双指起始中心，滚轮才作用于手指下方的区域。
        assertEquals(listOf("hover:150,100", "scroll"), s.events)
        g.event(140, TouchPhase.Move, p(1, 100.0, 160.0), p(2, 200.0, 160.0))
        assertEquals("只在滚动开始时移动一次", listOf("hover:150,100", "scroll", "scroll"), s.events)
        g.cancel(); s.events.clear()
        g.event(200, TouchPhase.Down, p(1, 100.0)); g.event(210, TouchPhase.Add, p(1, 100.0), p(2, 200.0), changed = 2)
        g.event(230, TouchPhase.Remove, p(1, 100.0), p(2, 200.0), changed = 2); g.event(240, TouchPhase.Up, p(1, 100.0))
        assertEquals(listOf("click:right"), s.events)
        g.cancel(); s.events.clear()
        g.event(300, TouchPhase.Down, p(1, 100.0)); g.event(310, TouchPhase.Add, p(1, 100.0), p(2, 200.0), changed = 2)
        g.event(330, TouchPhase.Move, p(1, 70.0), p(2, 230.0))
        assertEquals(listOf("pinch"), s.events)
        // 第二指加入后不会再触发单指长按。
        g.advance(5000); assertEquals(listOf("pinch"), s.events)
    }

    @Test fun normalTouchpadKeepsRelativeMovement() {
        val s = Sink(); val g = TouchpadGestures(s, 8.0, 40.0)
        g.event(100, TouchPhase.Down, p()); g.event(120, TouchPhase.Move, p(x = 130.0)); g.event(140, TouchPhase.Up, p(x = 130.0))
        assertEquals(listOf("move"), s.events)
    }
}
