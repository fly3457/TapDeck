package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

/** 视频模式单指拖动：开始时按方向判定、边缘溢出转滚轮的防抖、滚动与平移互斥、1 秒粘滞窗口、1:1 跟手换算。 */
class EdgeScrollTest {
    /** 按下后先竖向拖动一帧（10dp），让本次拖动判定为竖向。 */
    private fun verticalDrag(now: Long = 0): EdgeScroll {
        val e = EdgeScroll(); e.down(now)
        assertEquals(0.0 to -10.0, e.move(0.0, -10.0))
        assertEquals(EdgeScroll.Axis.Vertical, e.axis)
        return e
    }

    @Test fun overflowScrollsOnlyAfterSlop() {
        val e = verticalDrag()
        // 累计 10dp 未到 16dp 阈值，不滚动。
        assertEquals(0.0, e.overflow(-6.0), 0.0)
        assertEquals(0.0, e.overflow(-4.0), 0.0)
        assertFalse(e.scrolling)
        // 累计到 -20dp：超出阈值的 -4dp 立即滚动，方向与手指相同；之后本次拖动都是滚动模式。
        assertEquals(-4.0, e.overflow(-10.0), 1e-9)
        assertTrue(e.scrolling)
    }

    @Test fun horizontalDragNeverScrolls() {
        // 画面上下已放得下：横拖时手指往下偏 3°，纵向漂移全部溢出，也不能转成滚动、锁住左右平移。
        val e = EdgeScroll(); e.down(0)
        assertEquals(10.0 to 0.5, e.move(10.0, 0.5))
        assertEquals(EdgeScroll.Axis.Horizontal, e.axis)
        repeat(60) {
            assertEquals(5.0 to 0.26, e.move(5.0, 0.26))
            assertEquals(0.0, e.overflow(0.26), 0.0)
        }
        assertFalse(e.scrolling)
    }

    @Test fun directionDecidedOnceAfterDecideDistance() {
        val e = EdgeScroll(); e.down(0)
        // 未到 8dp 时照常平移、方向未定，溢出也不计。
        assertEquals(3.0 to 2.0, e.move(3.0, 2.0))
        assertEquals(EdgeScroll.Axis.Unknown, e.axis)
        assertEquals(0.0, e.overflow(-30.0), 0.0)
        // 累计 (6, 9)：竖向为主。之后改成横拖也不改判定。
        e.move(3.0, 7.0)
        assertEquals(EdgeScroll.Axis.Vertical, e.axis)
        e.move(50.0, 0.0)
        assertEquals(EdgeScroll.Axis.Vertical, e.axis)
    }

    @Test fun jitterBackAndForthNeverScrolls() {
        val e = verticalDrag()
        repeat(50) { assertEquals(0.0, e.overflow(if (it % 2 == 0) 10.0 else -10.0), 0.0) }
        assertFalse(e.scrolling)
    }

    @Test fun noOverflowWhileViewportStillPans() {
        val e = verticalDrag()
        // 视口吃掉全部位移时溢出只有浮点误差，不计入。
        repeat(100) { assertEquals(0.0, e.overflow(1e-9), 0.0) }
        assertEquals(0.0, e.overflow(Double.NaN), 0.0)
        assertFalse(e.scrolling)
    }

    @Test fun verticalDragScrollsWithinOneSecondAfterLift() {
        val e = verticalDrag(1000); e.overflow(-30.0); assertTrue(e.scrolling)
        e.up(1500)
        // 抬手后 1 秒内再次按下：方向未定时暂存位移，竖拖判定后一次性滚动暂存的位移，不用再攒阈值。
        e.down(2400)
        assertFalse(e.scrolling)
        assertNull(e.move(0.0, 5.0))
        assertEquals(1.0 to 9.0, e.move(1.0, 4.0))
        assertTrue(e.scrolling)
        e.up(2600)
        // 每次滚动抬手都重新计时：从 2600 起 1 秒内仍是滚动。
        e.down(3500); e.move(0.0, -10.0); assertTrue(e.scrolling)
        e.up(3550)
        // 超过 1 秒恢复为平移画面。
        e.down(4600); e.move(0.0, -10.0); assertFalse(e.scrolling)
        e.up(4700)
        // 平移模式的抬手不开启粘滞窗口。
        e.down(4800); e.move(0.0, -10.0); assertFalse(e.scrolling)
    }

    @Test fun horizontalDragPansWithinOneSecondAfterLift() {
        val e = verticalDrag(0); e.overflow(-30.0); e.up(100)
        // 1 秒内横拖：照常平移画面（交还暂存的位移），并结束滚动窗口。
        e.down(300)
        assertNull(e.move(4.0, 1.0))
        assertEquals(10.0 to 2.0, e.move(6.0, 1.0))
        assertFalse(e.scrolling)
        assertEquals(0.0, e.overflow(-30.0), 0.0)
        e.up(500)
        e.down(600); e.move(0.0, -10.0); assertFalse(e.scrolling)
    }

    @Test fun forceScrollWhenScrollInertiaRunning() {
        val e = EdgeScroll()
        e.down(10_000, force = true)
        e.move(0.0, 10.0)
        assertTrue(e.scrolling)
        // 惯性中按下后横拖：停住惯性并平移画面。
        e.down(20_000, force = true)
        e.move(10.0, 0.0)
        assertFalse(e.scrolling)
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
