package com.yuncii.tapdeck

import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Test

class TouchpadGesturesTest {
    private class Sink(override val doubleClickMs: Int = 500) : TouchSink {
        val events = mutableListOf<String>()
        val zooms = mutableListOf<Int>()
        override fun move(dx: Double, dy: Double) { events.add("move") }
        override fun scroll(dx: Double, dy: Double) { events.add("scroll:$dx:$dy") }
        override fun button(name: String, down: Boolean) { events.add("$name:$down") }
        override fun click(name: String) { events.add("click:$name") }
        override fun zoom(steps: Int) { zooms.add(steps); events.add("zoom") }
        override fun gesture(direction: String) { events.add("gesture:$direction") }
    }
    private fun point(id: Int = 9, x: Double = 100.0, y: Double = 100.0) = TouchPoint(id, x, y)
    private fun TouchpadGestures.event(time: Long, phase: TouchPhase, vararg points: TouchPoint, changed: Int = points.firstOrNull()?.id ?: -1) = accept(TouchFrame(time, phase, points.toList(), changed))
    private fun TouchpadGestures.tap(time: Long) { event(time, TouchPhase.Down, point()); event(time + 20, TouchPhase.Up, point()) }

    @Test fun singleClickWaitsUntilDeadlineAndRunsOnce() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.tap(100); assertTrue(sink.events.isEmpty()); assertEquals(420L, g.nextDeadline)
        g.advance(419); assertTrue(sink.events.isEmpty())
        g.advance(420); g.advance(1000)
        assertEquals(listOf("click:left"), sink.events); assertNull(g.nextDeadline)
    }
    @Test fun secondDownHoldsOneButtonAndDoubleClickOnlyCompletesOnUp() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.tap(100); g.event(200, TouchPhase.Down, point())
        assertEquals(listOf("left:true"), sink.events); assertNull(g.nextDeadline)
        g.event(210, TouchPhase.Move, point(x = 102.0))
        assertEquals(listOf("left:true"), sink.events)
        g.event(230, TouchPhase.Up, point(x = 102.0)); g.advance(1000)
        assertEquals(listOf("left:true", "left:false", "click:left"), sink.events)
    }
    @Test fun stationarySecondHoldReleasesWithoutDoubleClick() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.tap(100); g.event(200, TouchPhase.Down, point()); g.advance(2000)
        assertEquals(listOf("left:true"), sink.events)
        g.event(2000, TouchPhase.Up, point())
        assertEquals(listOf("left:true", "left:false"), sink.events)
    }
    @Test fun movingSecondTapDragsAndNeverCompletesDoubleClick() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.tap(100); g.event(200, TouchPhase.Down, point())
        g.event(230, TouchPhase.Move, point(x = 130.0)); g.event(240, TouchPhase.Up, point(x = 130.0))
        assertEquals(listOf("left:true", "move", "left:false"), sink.events)
    }
    @Test fun computerDoubleClickTimeLimitsSecondTap() {
        val sink = Sink(150); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.tap(100); g.event(200, TouchPhase.Down, point()); g.event(310, TouchPhase.Up, point())
        assertEquals(listOf("left:true", "left:false"), sink.events)
    }
    @Test fun doubleTapDeadlineAndPositionBoundaries() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.tap(100); g.event(420, TouchPhase.Down, point())
        assertEquals(listOf("click:left"), sink.events)
        g.cancel(); sink.events.clear(); g.tap(1000)
        g.event(1100, TouchPhase.Down, point(x = 150.0))
        assertEquals(listOf("click:left"), sink.events)
    }
    @Test fun cancelDropsPendingClickAndReleasesOnlyOnce() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.tap(100); g.cancel(); g.advance(1000); assertTrue(sink.events.isEmpty())
        g.tap(1200); g.event(1300, TouchPhase.Down, point()); g.cancel(); g.cancel()
        assertEquals(listOf("left:true", "left:false"), sink.events)
    }
    @Test fun addedFingerCancelsSecondTapHold() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.tap(100); g.event(200, TouchPhase.Down, point())
        g.event(210, TouchPhase.Add, point(), point(42, 200.0))
        assertEquals(listOf("left:true", "left:false"), sink.events)
        g.cancel(); g.advance(1000)
        assertEquals(listOf("left:true", "left:false"), sink.events)
    }
    @Test fun twoFingerTapIsRightClickAndLiftCannotMoveCursor() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.event(100, TouchPhase.Down, point()); g.event(110, TouchPhase.Add, point(), point(42, 200.0))
        g.event(130, TouchPhase.Remove, point(42, 200.0), point(), changed = 42)
        g.event(140, TouchPhase.Up, point()); g.advance(1000)
        assertEquals(listOf("click:right"), sink.events)
    }
    @Test fun scrollLocksAndUsesPointerIdsInsteadOfArrayOrder() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.event(100, TouchPhase.Down, point()); g.event(110, TouchPhase.Add, point(), point(42, 200.0))
        g.event(130, TouchPhase.Move, point(42, 200.0, 70.0), point(y = 70.0))
        g.event(140, TouchPhase.Move, point(x = 80.0, y = 60.0), point(42, 220.0, 60.0))
        g.event(150, TouchPhase.Remove, point(42, 220.0, 60.0), point(x = 80.0, y = 60.0), changed = 42)
        g.event(160, TouchPhase.Move, point(x = 90.0, y = 80.0)); g.event(170, TouchPhase.Up, point(x = 90.0, y = 80.0))
        assertEquals(listOf("scroll:0.0:-30.0", "scroll:0.0:-10.0"), sink.events)
    }
    @Test fun spreadAndPinchZoomWithoutScrollOrClick() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.event(100, TouchPhase.Down, point()); g.event(110, TouchPhase.Add, point(), point(42, 200.0))
        g.event(130, TouchPhase.Move, point(x = 70.0), point(42, 230.0))
        assertTrue(sink.zooms.sum() > 0)
        val before = sink.zooms.sum()
        g.event(140, TouchPhase.Move, point(42, 180.0), point(x = 120.0))
        assertTrue(sink.zooms.sum() < before)
        g.event(150, TouchPhase.Remove, point(x = 120.0), point(42, 180.0), changed = 42); g.event(160, TouchPhase.Up, point(x = 120.0))
        assertTrue(sink.events.all { it == "zoom" })
    }
    @Test fun pinchJitterStaysBelowThresholdAndZoomStepsAccumulate() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.event(100, TouchPhase.Down, point()); g.event(110, TouchPhase.Add, point(), point(42, 200.0))
        g.event(120, TouchPhase.Move, point(x = 98.0), point(42, 202.0)); assertTrue(sink.events.isEmpty())
        g.event(130, TouchPhase.Move, point(x = 95.0), point(42, 205.0)); assertTrue(sink.events.isEmpty())
        g.event(140, TouchPhase.Move, point(x = 92.0), point(42, 208.0))
        assertEquals(1, sink.zooms.sum())
        g.event(150, TouchPhase.Move, point(x = 92.0, y = 150.0), point(42, 208.0, 150.0))
        assertEquals(listOf("zoom"), sink.events)
    }
    @Test fun threeFingerSwipeTriggersOnceAndNeverFallsBackToScrolling() {
        for (direction in listOf(-1, 1)) {
            val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
            g.event(100, TouchPhase.Down, point()); g.event(110, TouchPhase.Add, point(), point(42, 200.0))
            g.event(120, TouchPhase.Add, point(77, 300.0), point(), point(42, 200.0))
            fun points(delta: Double) = arrayOf(point(42, 200.0, 100.0 + delta), point(77, 300.0, 100.0 + delta), point(y = 100.0 + delta))
            g.event(130, TouchPhase.Move, *points(20.0 * direction)); assertTrue(sink.events.isEmpty())
            g.event(140, TouchPhase.Move, *points(40.0 * direction)); g.event(150, TouchPhase.Move, *points(80.0 * direction))
            g.event(160, TouchPhase.Remove, *points(80.0 * direction), changed = 77)
            g.event(170, TouchPhase.Move, point(42, 200.0, 120.0), point(y = 120.0))
            g.event(180, TouchPhase.Remove, point(), point(42, 200.0), changed = 42); g.event(190, TouchPhase.Up, point())
            assertEquals(listOf("gesture:${if (direction < 0) "up" else "down"}"), sink.events)
        }
    }
    @Test fun sidewaysThreeFingerAndFourFingerGesturesAreIgnored() {
        val sink = Sink(); val g = TouchpadGestures(sink, 8.0, 40.0)
        g.event(100, TouchPhase.Down, point()); g.event(110, TouchPhase.Add, point(), point(42, 200.0))
        g.event(120, TouchPhase.Add, point(), point(42, 200.0), point(77, 300.0))
        g.event(130, TouchPhase.Move, point(x = 200.0, y = 140.0), point(42, 300.0, 140.0), point(77, 400.0, 140.0))
        assertTrue(sink.events.isEmpty())
        g.event(140, TouchPhase.Add, point(), point(42, 200.0), point(77, 300.0), point(81, 400.0))
        g.event(150, TouchPhase.Remove, point(), point(42, 200.0), point(77, 300.0), point(81, 400.0), changed = 81)
        g.event(160, TouchPhase.Move, point(y = 180.0), point(42, 200.0, 180.0), point(77, 300.0, 180.0))
        g.cancel(); assertTrue(sink.events.isEmpty())
    }
    @Test fun scrollDirectionUsesDifferentHorizontalAndVerticalSigns() {
        assertEquals(-5 * 1024L to 5 * 1024L, scrollUnits(1.0, 1.0, true))
        assertEquals(5 * 1024L to -5 * 1024L, scrollUnits(1.0, 1.0, false))
        assertTrue(scrollUnits(0.0, -10.0, true).second < 0)
        assertTrue(scrollUnits(0.0, 10.0, true).second > 0)
    }
    @Test fun readyCapabilitiesAreOptionalAndBounded() {
        assertEquals(TouchpadCapabilities(), wireJson.parseToJsonElement("{}").jsonObject.touchpadCapabilities())
        val caps = wireJson.parseToJsonElement("{\"features\":[\"touchpad_zoom\",\"three_finger\"],\"double_click_ms\":250}").jsonObject.touchpadCapabilities()
        assertEquals(setOf(ZOOM_FEATURE, GESTURE_FEATURE), caps.features); assertEquals(250, caps.doubleClickMs)
        assertEquals(500, wireJson.parseToJsonElement("{\"double_click_ms\":9999}").jsonObject.touchpadCapabilities().doubleClickMs)
    }
}
