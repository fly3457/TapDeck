package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class MotionPanTest {
    private val px = 1000.0

    @Test fun turningRightMovesContentLeft() {
        // 竖屏向右转：绕设备 y 轴顺时针（俯视），角速度为负。
        val (dx, dy) = MotionPan.delta(0.0, -1.0, 0, 0.1, px)
        assertTrue("画面应向左移", dx < 0)
        assertEquals(0.0, dy, 1e-9)
        assertEquals(-(1.0 - MotionPan.DEAD_ZONE) * 0.1 * px, dx, 1e-9)
    }

    @Test fun tiltingTopTowardUserMovesContentUp() {
        // 顶部朝自己倾斜：绕设备 x 轴正向转动，看到画面下方，画面向上移。
        val (dx, dy) = MotionPan.delta(1.0, 0.0, 0, 0.1, px)
        assertEquals(0.0, dx, 1e-9)
        assertTrue("画面应向上移", dy < 0)
    }

    @Test fun landscapeUsesScreenAxes() {
        // ROTATION_90（手机逆时针横放）：屏幕竖轴是设备 x 轴，向右转为绕设备 x 轴负向转动。
        val (dx, dy) = MotionPan.delta(-1.0, 0.0, 1, 0.1, px)
        assertTrue(dx < 0); assertEquals(0.0, dy, 1e-9)
        // ROTATION_270 方向相反。
        val (dx2, _) = MotionPan.delta(1.0, 0.0, 3, 0.1, px)
        assertTrue(dx2 < 0)
        // ROTATION_180：上下左右都翻转。
        val (dx3, dy3) = MotionPan.delta(0.0, 1.0, 2, 0.1, px)
        assertTrue(dx3 < 0); assertEquals(0.0, dy3, 1e-9)
    }

    @Test fun smallJitterIsIgnoredAndLongGapsAreCapped() {
        val (jx, jy) = MotionPan.delta(0.02, -0.02, 0, 0.1, px)
        assertEquals(0.0, jx, 1e-12); assertEquals(0.0, jy, 1e-12)
        val (capped, _) = MotionPan.delta(0.0, -1.0, 0, 5.0, px)
        val (normal, _) = MotionPan.delta(0.0, -1.0, 0, MotionPan.MAX_DT, px)
        assertEquals(normal, capped, 1e-9)
    }
}
