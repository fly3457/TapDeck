package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class VideoViewportTest {
    private fun settle(v: VideoViewport) { repeat(100) { v.step() } }

    @Test fun portraitFillsHeightAndCropsBothSides() {
        val v = VideoViewport(); v.resize(1080.0, 2400.0, 1920.0, 1080.0)
        assertEquals(2400.0 / 1080.0, v.scale, 1e-9)
        assertEquals(2400.0, v.displayHeight, 1e-6)
        assertEquals(0.0, v.offsetY, 1e-9)
        // 水平居中：左右裁掉的宽度相等。
        assertEquals(-(v.displayWidth - 1080.0) / 2, v.offsetX, 1e-6)
    }

    @Test fun landscapeShowsWholeScreenWithSideBars() {
        val v = VideoViewport(); v.resize(2400.0, 1080.0, 1920.0, 1080.0)
        assertEquals(1.0, v.scale, 1e-9)
        assertEquals(240.0, v.offsetX, 1e-9)
        assertEquals(0.0, v.offsetY, 1e-9)
    }

    @Test fun pinchKeepsFocusAndRespectsLimits() {
        val v = VideoViewport(); v.resize(1080.0, 2400.0, 1920.0, 1080.0)
        val before = v.scale
        // 焦点下的 PC 像素在缩放前后保持不动。
        val focusX = 540.0; val focusY = 1200.0
        val contentX = (focusX - v.offsetX) / v.scale; val contentY = (focusY - v.offsetY) / v.scale
        v.pinch(1.5, focusX, focusY, 0.0, 0.0)
        assertEquals(before * 1.5, v.scale, 1e-9)
        assertEquals(focusX, v.offsetX + contentX * v.scale, 1e-6)
        assertEquals(focusY, v.offsetY + contentY * v.scale, 1e-6)
        v.pinch(100.0, focusX, focusY, 0.0, 0.0); assertEquals(v.maxScale, v.scale, 1e-9)
        // 缩到最小：完整显示整个 PC 画面并居中。
        v.pinch(0.0001, focusX, focusY, 0.0, 0.0)
        assertEquals(1080.0 / 1920.0, v.scale, 1e-9)
        assertEquals(0.0, v.offsetX, 1e-9)
        assertEquals((2400.0 - v.displayHeight) / 2, v.offsetY, 1e-6)
        v.pinch(Double.NaN, 0.0, 0.0, 0.0, 0.0); assertEquals(1080.0 / 1920.0, v.scale, 1e-9)
    }

    @Test fun pinchPanNeverExposesOutsideContent() {
        val v = VideoViewport(); v.resize(1080.0, 2400.0, 1920.0, 1080.0)
        v.pinch(1.0, 540.0, 1200.0, 100000.0, -100000.0)
        assertEquals(0.0, v.offsetX, 1e-9)
        assertEquals(0.0, v.offsetY, 1e-9)
    }

    @Test fun cursorNearEdgeIsFollowedSmoothly() {
        val v = VideoViewport(); v.resize(1080.0, 2400.0, 1920.0, 1080.0)
        val start = v.offsetX
        v.cursor(960.0, 540.0); settle(v); assertEquals(start, v.offsetX, 1e-9) // 中间：不动
        v.cursor(1900.0, 540.0)
        assertTrue(v.step()); assertTrue(v.offsetX < start)
        settle(v)
        val onScreen = v.offsetX + 1900.0 * v.scale
        assertTrue(onScreen <= 1080.0 * 0.88 + 1e-6 || v.offsetX == 1080.0 - v.displayWidth)
        assertFalse(v.step())
        v.cursor(0.0, 540.0); settle(v); assertEquals(0.0, v.offsetX, 1e-6)
    }

    @Test fun userPanIsKeptUntilCursorMoves() {
        val v = VideoViewport(); v.resize(1080.0, 2400.0, 1920.0, 1080.0)
        v.cursor(960.0, 540.0); settle(v)
        v.pinch(1.0, 540.0, 1200.0, 800.0, 0.0)
        val panned = v.offsetX
        v.cursor(960.0, 540.0); settle(v)
        assertEquals(panned, v.offsetX, 1e-9)
    }

    @Test fun rotationReappliesRuleAroundCursor() {
        val v = VideoViewport(); v.resize(2400.0, 1080.0, 1920.0, 1080.0)
        v.cursor(1700.0, 500.0); settle(v)
        v.resize(1080.0, 2400.0, 1920.0, 1080.0)
        assertEquals(2400.0 / 1080.0, v.scale, 1e-9)
        val onScreen = v.offsetX + 1700.0 * v.scale
        assertTrue(onScreen in 0.0..1080.0)
    }
}
