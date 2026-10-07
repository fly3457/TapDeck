package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class ControllerLayoutTest {
    @Test fun portraitPhoneAndTabletLeaveAllRemainingHeightToTouchpad() {
        for ((width, height) in listOf(1080 to 2200, 720 to 1180, 640 to 860, 1404 to 1782)) {
            val m = ControllerLayout.measure(width, height)
            assertEquals(1f, m.scale, 0f)
            assertEquals(width * 0.10f, m.header.toFloat(), 1f)
            assertEquals(width * 0.60f, m.panelContent.toFloat(), 1f)
            assertEquals(width * 0.01f, m.panelPadding.toFloat(), 1f)
            assertEquals(height, m.header + m.panel + m.touchpad)
            assertEquals(height - width * 0.72f, m.touchpad.toFloat(), 2f)
        }
    }

    @Test fun smallViewportScalesDownAndLeavesRoomForTouchpad() {
        val m = ControllerLayout.measure(1080, 600)
        assertEquals(600f / (1080 * 0.82f), m.scale, 0.00001f)
        assertEquals(600, m.header + m.panel + m.touchpad)
        assertTrue(m.touchpad >= 1080 * m.scale * 0.10f - 2)
        assertTrue(m.panel > 0)
    }

    @Test fun zeroAndTinyMeasurementsNeverOverflow() {
        for (width in listOf(0, 1, 320, 1080)) for (height in 0..12) {
            val m = ControllerLayout.measure(width, height)
            assertTrue(m.scale.isFinite())
            assertTrue(m.touchpad >= 0)
            assertTrue("$m", m.header + m.panel <= height)
        }
    }

    @Test fun voiceControlAndRingStayBetweenModeRowAndFooterAtEverySavedPosition() {
        for ((width, height) in listOf(1080 to 2200, 640 to 860, 1404 to 1782, 1080 to 600)) {
            val m = ControllerLayout.measure(width, height)
            val g = VoiceGeometry(width.toFloat(), (m.panelContent / 2).toFloat(), width * m.scale)
            assertEquals(width * m.scale * 0.06f, g.radius, 0.01f)
            assertTrue(g.right > g.left)
            assertTrue(g.bottom > g.top)
            for (position in listOf(0f, 0.42f, 0.5f, 1f)) {
                val (x, y) = g.center(position, position)
                assertTrue(x - g.radius - g.halo >= 0)
                assertTrue(x + g.radius + g.halo <= width)
                assertTrue(y - g.radius - g.halo >= g.modeHeight)
                assertTrue(y + g.radius + g.halo <= g.footerTop)
            }
        }
    }
}
