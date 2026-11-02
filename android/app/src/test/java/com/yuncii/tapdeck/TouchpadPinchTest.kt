package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

/** 视频模式：捏合在本地缩放画面，双指拖动与轻点仍发送滚轮与右键。 */
class TouchpadPinchTest {
    private class LocalPinchSink : TouchSink {
        val events = mutableListOf<String>()
        var factor = 1.0
        override fun move(dx: Double, dy: Double) { events.add("move") }
        override fun scroll(dx: Double, dy: Double) { events.add("scroll") }
        override fun button(name: String, down: Boolean) { events.add("$name:$down") }
        override fun click(name: String) { events.add("click:$name") }
        override fun zoom(steps: Int) { events.add("zoom") }
        override fun pinch(scale: Double, focusX: Double, focusY: Double, panX: Double, panY: Double): Boolean {
            factor *= scale; events.add("pinch"); return true
        }
    }
    private fun p(id: Int, x: Double, y: Double = 100.0) = TouchPoint(id, x, y)
    private fun TouchpadGestures.event(time: Long, phase: TouchPhase, vararg points: TouchPoint, changed: Int = points.first().id) = accept(TouchFrame(time, phase, points.toList(), changed))

    @Test fun pinchIsHandledLocallyInsteadOfPcZoom() {
        val sink = LocalPinchSink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.event(100, TouchPhase.Down, p(1, 100.0)); g.event(110, TouchPhase.Add, p(1, 100.0), p(2, 200.0), changed = 2)
        g.event(130, TouchPhase.Move, p(1, 70.0), p(2, 230.0))
        g.event(140, TouchPhase.Move, p(1, 50.0), p(2, 250.0))
        assertTrue(sink.events.isNotEmpty() && sink.events.all { it == "pinch" })
        assertEquals(2.0, sink.factor, 1e-9)
    }

    @Test fun twoFingerDragStillScrollsAndTapRightClicks() {
        val sink = LocalPinchSink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.event(100, TouchPhase.Down, p(1, 100.0)); g.event(110, TouchPhase.Add, p(1, 100.0), p(2, 200.0), changed = 2)
        g.event(130, TouchPhase.Move, p(1, 100.0, 140.0), p(2, 200.0, 140.0))
        assertEquals(listOf("scroll"), sink.events)
        g.cancel(); sink.events.clear()
        g.event(200, TouchPhase.Down, p(1, 100.0)); g.event(210, TouchPhase.Add, p(1, 100.0), p(2, 200.0), changed = 2)
        g.event(230, TouchPhase.Remove, p(1, 100.0), p(2, 200.0), changed = 2); g.event(240, TouchPhase.Up, p(1, 100.0))
        assertEquals(listOf("click:right"), sink.events)
    }
}
