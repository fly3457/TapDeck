package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

/** 视频模式单指拖动：边缘溢出转滚轮的防抖、滚动与平移互斥、1 秒粘滞窗口、1:1 跟手换算。 */
class EdgeScrollTest {
    @Test fun overflowScrollsOnlyAfterSlop() {
        val e = EdgeScroll(); e.down(0)
        // 累计 10dp 未到 16dp 阈值，不滚动。
        assertEquals(0.0, e.overflow(-6.0), 0.0)
        assertEquals(0.0, e.overflow(-4.0), 0.0)
        assertFalse(e.scrolling)
        // 累计到 -20dp：超出阈值的 -4dp 立即滚动，方向与手指相同；之后本次拖动都是滚动模式。
        assertEquals(-4.0, e.overflow(-10.0), 1e-9)
        assertTrue(e.scrolling)
    }

    @Test fun jitterBackAndForthNeverScrolls() {
        val e = EdgeScroll(); e.down(0)
        repeat(50) { assertEquals(0.0, e.overflow(if (it % 2 == 0) 10.0 else -10.0), 0.0) }
        assertFalse(e.scrolling)
    }

    @Test fun noOverflowWhileViewportStillPans() {
        val e = EdgeScroll(); e.down(0)
        // 视口吃掉全部位移时溢出只有浮点误差，不计入。
        repeat(100) { assertEquals(0.0, e.overflow(1e-9), 0.0) }
        assertEquals(0.0, e.overflow(Double.NaN), 0.0)
        assertFalse(e.scrolling)
    }

    @Test fun scrollStaysForOneSecondAfterLift() {
        val e = EdgeScroll()
        e.down(1000); e.overflow(-30.0); assertTrue(e.scrolling)
        e.up(1500)
        // 抬手后 1 秒内再次按下，不论方向直接是滚动模式。
        e.down(2400); assertTrue(e.scrolling)
        e.up(2600)
        // 每次滚动抬手都重新计时：从 2600 起 1 秒内仍是滚动。
        e.down(3500); assertTrue(e.scrolling)
        e.up(3550)
        // 超过 1 秒恢复为平移画面。
        e.down(4600); assertFalse(e.scrolling)
        e.up(4700)
        // 平移模式的抬手不开启粘滞窗口。
        e.down(4800); assertFalse(e.scrolling)
    }

    @Test fun forceScrollWhenScrollInertiaRunning() {
        val e = EdgeScroll()
        e.down(10_000, force = true)
        assertTrue(e.scrolling)
    }

    @Test fun followGainMatchesCalibration() {
        // 努比亚：density 3.25，竖屏画面缩放 2688/2160；原来每 dp 发 5 单位时页面移动为手指的约 2.26 倍。
        val gain = EdgeScroll.followGain(3.25, 2688.0 / 2160.0)
        assertEquals(1 / 2.26, gain, 0.02)
        // 画面放大一倍，同样的手指位移对应的 PC 像素减半，滚动量也减半。
        assertEquals(gain / 2, EdgeScroll.followGain(3.25, 2 * 2688.0 / 2160.0), 1e-9)
        assertEquals(1.0, EdgeScroll.followGain(0.0, 1.0), 0.0)
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
