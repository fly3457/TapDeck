package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

/** 视频模式单指拖到上下边缘后继续拖：溢出转滚轮的防抖与方向。 */
class EdgeScrollTest {
    @Test fun overflowScrollsOnlyAfterSlop() {
        val e = EdgeScroll()
        // 累计 10dp 未到 16dp 阈值，不滚动。
        assertEquals(0.0, e.overflow(-6.0), 0.0)
        assertEquals(0.0, e.overflow(-4.0), 0.0)
        assertFalse(e.scrolling)
        // 累计到 -20dp：超出阈值的 -4dp 立即滚动，方向与手指相同。
        assertEquals(-4.0, e.overflow(-10.0), 1e-9)
        assertTrue(e.scrolling)
        // 之后的溢出全部转成滚动，包括反方向。
        assertEquals(-3.0, e.overflow(-3.0), 0.0)
        assertEquals(5.0, e.overflow(5.0), 0.0)
    }

    @Test fun jitterBackAndForthNeverScrolls() {
        val e = EdgeScroll()
        repeat(50) { assertEquals(0.0, e.overflow(if (it % 2 == 0) 10.0 else -10.0), 0.0) }
        assertFalse(e.scrolling)
    }

    @Test fun noOverflowWhileViewportStillPans() {
        val e = EdgeScroll()
        // 视口吃掉全部位移时溢出只有浮点误差，不计入。
        repeat(100) { assertEquals(0.0, e.overflow(1e-9), 0.0) }
        assertEquals(0.0, e.overflow(Double.NaN), 0.0)
        assertFalse(e.scrolling)
    }

    @Test fun resetStartsNewDrag() {
        val e = EdgeScroll()
        e.overflow(30.0); assertTrue(e.scrolling)
        e.reset()
        assertFalse(e.scrolling)
        assertEquals(0.0, e.overflow(8.0), 0.0)
    }

    @Test fun viewportReportsVerticalOverflowAtEdge() {
        // 竖屏：画面按高度填满，纵向不能平移，纵向拖动全部溢出；横向可平移，不溢出。
        val v = VideoViewport()
        v.resize(1096.0, 2560.0, 3840.0, 2160.0)
        val y0 = v.offsetY; val x0 = v.offsetX
        v.pan(-100.0, -100.0)
        assertEquals(0.0, v.offsetY - y0, 1e-9)
        assertEquals(-100.0, v.offsetX - x0, 1e-9)
    }
}
