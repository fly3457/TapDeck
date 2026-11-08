package com.yuncii.tapdeck

import android.view.MotionEvent
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class IdleScreensaverTest {
    private class Harness {
        var armed: Long? = null
        var shown = 0
        var hidden = 0
        val controller = IdleScreensaverController(
            schedule = { armed = it },
            cancel = { armed = null },
            show = { shown++ },
            hide = { hidden++ },
        )
        /** 模拟定时器到点。 */
        fun fire() { armed = null; controller.onTimeout() }
    }

    @Test fun settingsDefaultOffAndNormalizeMinutes() {
        val default = IdleScreensaverSettings()
        assertFalse(default.enabled)
        assertNull(default.timeoutMs)
        assertEquals(5 * 60_000L, default.copy(enabled = true).timeoutMs)
        assertEquals(5, IdleScreensaverSettings.normalizeMinutes(null))
        assertEquals(5, IdleScreensaverSettings.normalizeMinutes(7))
        assertEquals(30, IdleScreensaverSettings.normalizeMinutes(30))
    }

    @Test fun disabledOrPausedNeverArms() {
        val h = Harness()
        h.controller.configure(null)
        h.controller.setPaused(false)
        assertNull(h.armed)
        h.controller.configure(60_000L)
        assertEquals(60_000L, h.armed)
        h.controller.setPaused(true)
        assertNull(h.armed)
    }

    @Test fun showsAfterTimeoutAndFirstTouchOnlyWakes() {
        val h = Harness()
        h.controller.configure(60_000L); h.controller.setPaused(false)
        h.fire()
        assertTrue(h.controller.showing); assertEquals(1, h.shown)
        // 唤醒的这一下（按下、移动、抬起）全部被吃掉，不会传给按键或触控板。
        assertTrue(h.controller.onTouch(MotionEvent.ACTION_DOWN))
        assertFalse(h.controller.showing); assertEquals(1, h.hidden)
        assertNull("手指按着时不计时", h.armed)
        assertTrue(h.controller.onTouch(MotionEvent.ACTION_MOVE))
        assertTrue(h.controller.onTouch(MotionEvent.ACTION_UP))
        assertEquals(60_000L, h.armed)
        // 之后的触摸正常传递。
        assertFalse(h.controller.onTouch(MotionEvent.ACTION_DOWN))
        assertFalse(h.controller.onTouch(MotionEvent.ACTION_UP))
    }

    @Test fun holdingFingerBlocksTimeout() {
        val h = Harness()
        h.controller.configure(60_000L); h.controller.setPaused(false)
        assertFalse(h.controller.onTouch(MotionEvent.ACTION_DOWN))
        assertNull(h.armed)
        h.controller.onTimeout()
        assertFalse(h.controller.showing)
        h.controller.onTouch(MotionEvent.ACTION_CANCEL)
        assertEquals(60_000L, h.armed)
    }

    @Test fun pausingDismissesAndKeyWakes() {
        val h = Harness()
        h.controller.configure(60_000L); h.controller.setPaused(false)
        h.fire()
        h.controller.setPaused(true)
        assertFalse(h.controller.showing); assertNull(h.armed)
        h.controller.setPaused(false)
        h.fire()
        assertTrue(h.controller.onKey())
        assertFalse(h.controller.showing)
        assertFalse(h.controller.onKey())
    }

    @Test fun previewIgnoresSwitchAndTouchStillWakes() {
        val h = Harness()
        h.controller.preview()
        assertTrue(h.controller.showing)
        assertTrue(h.controller.onTouch(MotionEvent.ACTION_DOWN))
        assertTrue(h.controller.onTouch(MotionEvent.ACTION_UP))
        assertFalse(h.controller.showing)
        assertNull("开关关闭时预览结束后不再计时", h.armed)
    }

    @Test fun bounceReflectsAtEdges() {
        assertEquals(0f, bounce(0f, 100f), 0f)
        assertEquals(40f, bounce(40f, 100f), 0f)
        assertEquals(100f, bounce(100f, 100f), 0f)
        assertEquals(70f, bounce(130f, 100f), 0f)
        assertEquals(0f, bounce(200f, 100f), 0f)
        assertEquals(30f, bounce(230f, 100f), 0f)
        assertEquals(30f, bounce(-30f, 100f), 0f)
        assertEquals("区域比时钟还小时贴边", 0f, bounce(50f, -10f), 0f)
        for (d in 0..5000 step 7) {
            val p = bounce(d.toFloat(), 333f)
            assertTrue(p in 0f..333f)
        }
    }
}
