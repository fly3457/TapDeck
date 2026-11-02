package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class MicTriggerTest {
    private class Host(override var gestureMode: String, override var available: Boolean = true) : MicTrigger.Host {
        override var status = "idle"
        val events = mutableListOf<String>()
        var scheduled = false
        var beginResult = true
        override fun begin(mode: String): Boolean { events.add("begin:$mode"); return beginResult }
        override fun end(abort: Boolean) { events.add("end:$abort"); status = "idle" }
        override fun schedule(delayMs: Long) { scheduled = true }
        override fun unschedule() { scheduled = false }
        override fun drag(dx: Float, dy: Float) { if (events.lastOrNull() != "drag") events.add("drag") }
        override fun dragEnd() { events.add("dragEnd") }
        override fun feedback(kind: KeyFeedback) {}
        override fun started(mode: String) { status = "preparing"; events.add("started") }
    }

    @Test fun holdStartsAfterDelayAndEndsOnRelease() {
        val h = Host(MicBallView.MODE_HOLD); val t = MicTrigger(10f, h)
        t.down(0f, 0f, 0); assertTrue(h.scheduled)
        t.holdElapsed(); assertEquals(listOf("begin:hold", "started"), h.events)
        t.up(0f, 0f, 1000); assertEquals(listOf("begin:hold", "started", "end:false"), h.events)
        assertFalse(t.tracking)
    }

    @Test fun shortTapInHoldModeDoesNothing() {
        val h = Host(MicBallView.MODE_HOLD); val t = MicTrigger(10f, h)
        t.down(0f, 0f, 0); t.up(0f, 0f, 100)
        assertTrue(h.events.isEmpty()); assertFalse(h.scheduled)
    }

    @Test fun toggleTapStartsThenStops() {
        val h = Host(MicBallView.MODE_TOGGLE); val t = MicTrigger(10f, h)
        t.down(0f, 0f, 0); assertFalse(h.scheduled); t.up(2f, 2f, 100)
        assertEquals(listOf("begin:toggle", "started"), h.events)
        h.status = "transmitting"
        t.down(0f, 0f, 500); t.up(0f, 0f, 600)
        assertEquals(listOf("begin:toggle", "started", "end:false"), h.events)
    }

    @Test fun dragMovesWithoutStartingVoice() {
        val h = Host(MicBallView.MODE_HOLD); val t = MicTrigger(10f, h)
        t.down(0f, 0f, 0); t.move(30f, 0f); assertFalse(h.scheduled)
        t.holdElapsed(); t.up(30f, 0f, 1000)
        assertEquals(listOf("drag", "dragEnd"), h.events)
    }

    @Test fun cancelAbortsActiveHold() {
        val h = Host(MicBallView.MODE_HOLD); val t = MicTrigger(10f, h)
        t.down(0f, 0f, 0); t.holdElapsed(); t.cancel()
        assertEquals(listOf("begin:hold", "started", "end:true"), h.events)
    }

    @Test fun unavailableIgnoresTaps() {
        val h = Host(MicBallView.MODE_TOGGLE, available = false); val t = MicTrigger(10f, h)
        t.down(0f, 0f, 0); t.up(0f, 0f, 50)
        assertTrue(h.events.isEmpty())
    }
}
