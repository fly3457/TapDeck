package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class ShortcutPanelTest {
    @Test fun portraitStacksTwoColumnsLandscapeTwoRows() {
        assertEquals(ShortcutPanelGrid(columns = 2, rows = 4), shortcutPanelGrid(8, landscape = false))
        assertEquals(ShortcutPanelGrid(columns = 4, rows = 2), shortcutPanelGrid(8, landscape = true))
        assertEquals(ShortcutPanelGrid(columns = 2, rows = 3), shortcutPanelGrid(5, landscape = false))
        assertEquals(ShortcutPanelGrid(columns = 3, rows = 2), shortcutPanelGrid(5, landscape = true))
        assertEquals(ShortcutPanelGrid(columns = 1, rows = 1), shortcutPanelGrid(1, landscape = true))
        assertEquals(ShortcutPanelGrid(columns = 1, rows = 1), shortcutPanelGrid(0, landscape = false))
    }

    @Test fun panelSitsBelowBallAndFlipsAboveNearBottom() {
        // 球在中间：正下方、水平居中。
        assertEquals(450f to 1240f, shortcutPanelPosition(540f, 1200f, 30f, 180f, 300f, 1080f, 2400f, 10f))
        // 球靠近底部：改到上方。
        val (_, y) = shortcutPanelPosition(540f, 2300f, 30f, 180f, 300f, 1080f, 2400f, 10f)
        assertEquals(2300f - 30f - 10f - 300f, y)
        // 靠边时不超出屏幕。
        assertEquals(0f, shortcutPanelPosition(20f, 500f, 30f, 180f, 300f, 1080f, 2400f, 10f).first)
        assertEquals(900f, shortcutPanelPosition(1070f, 500f, 30f, 180f, 300f, 1080f, 2400f, 10f).first)
        // 上下都放不下（横屏矮视图）：留在下方并钳制在视图内。
        val (_, tight) = shortcutPanelPosition(540f, 100f, 30f, 180f, 300f, 1080f, 350f, 10f)
        assertEquals(50f, tight)
    }
}
