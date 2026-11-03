package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class VideoKeyboardSizeTest {
    @Test fun portraitMatchesNormalLayout() {
        val full = videoKeyboardSize(1216, 2688, VideoKeyboard.Full)!!
        assertEquals(1216, full.width); assertEquals(1f, full.scale, 0f)
        assertEquals((1216 * 0.60f).toInt() + full.padding * 2, full.height)
        val shortcuts = videoKeyboardSize(1216, 2688, VideoKeyboard.Shortcuts)!!
        assertEquals(1216, shortcuts.width)
        assertTrue(shortcuts.height < full.height)
        assertNull(videoKeyboardSize(1216, 2688, VideoKeyboard.None))
    }

    @Test fun landscapeKeepsPortraitWidthAndStaysShort() {
        val full = videoKeyboardSize(2688, 1216, VideoKeyboard.Full)!!
        assertEquals(1216, full.width)
        assertTrue("全键盘不超过屏高约 42%", full.height <= 1216 * 0.42f)
        assertTrue(full.scale < 1f)
        val shortcuts = videoKeyboardSize(2688, 1216, VideoKeyboard.Shortcuts)!!
        assertEquals(1216, shortcuts.width)
        assertTrue("快捷键不超过屏高约 24%", shortcuts.height <= 1216 * 0.24f)
    }

    @Test fun nearlySquareWindowNeverExceedsView() {
        val s = videoKeyboardSize(1000, 900, VideoKeyboard.Full)!!
        assertTrue(s.width <= 1000 && s.height <= 900)
    }
}
