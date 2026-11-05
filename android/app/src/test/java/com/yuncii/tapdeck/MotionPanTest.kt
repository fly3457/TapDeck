package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test
import kotlin.math.PI
import kotlin.math.abs
import kotlin.math.sin

class MotionPanTest {
    private val px = 1000.0
    private val dt = 0.02 // 陀螺仪 SENSOR_DELAY_GAME 约 50 Hz

    /** 连续输入 n 个相同采样，返回累计平移与最后一次平移。 */
    private fun steady(wx: Double, wy: Double, rotation: Int = 0, n: Int = 100): Triple<Double, Double, Pair<Double, Double>> {
        val m = MotionPan(); var sx = 0.0; var sy = 0.0; var last = 0.0 to 0.0
        repeat(n) { last = m.step(wx, wy, rotation, dt, px); sx += last.first; sy += last.second }
        return Triple(sx, sy, last)
    }

    @Test fun turningRightMovesContentLeft() {
        // 竖屏向右转：绕设备 y 轴顺时针（俯视），角速度为负。
        val (sx, sy, last) = steady(0.0, -1.0)
        assertTrue("画面应向左移", sx < 0)
        assertEquals(0.0, sy, 1e-9)
        // 持续转动时滤波收敛，每步平移等于（角速度 − 死区）× 加速度倍率 × 时间 × 系数。
        val rate = 1.0 - MotionPan.DEAD_ZONE
        assertEquals(-rate * MotionPan.gain(rate) * dt * px, last.first, 0.5)
    }

    @Test fun tiltingTopTowardUserMovesContentDown() {
        // 按实测要求上下反转：顶部朝自己倾斜（绕设备 x 轴正向）时画面向下移。
        val (sx, sy, _) = steady(1.0, 0.0)
        assertEquals(0.0, sx, 1e-9)
        assertTrue("画面应向下移", sy > 0)
    }

    @Test fun landscapeUsesScreenAxes() {
        // ROTATION_90（手机逆时针横放）：屏幕竖轴是设备 x 轴，向右转为绕设备 x 轴负向转动。
        fun check(wx: Double, wy: Double, rotation: Int, pitch: Double, yaw: Double) {
            val (p, y) = MotionPan.screenRates(wx, wy, rotation)
            assertEquals(pitch, p, 1e-12); assertEquals(yaw, y, 1e-12)
        }
        check(-1.0, 0.0, 1, 0.0, -1.0)
        check(1.0, 0.0, 3, 0.0, -1.0)
        check(0.0, 1.0, 2, 0.0, -1.0) // ROTATION_180：上下左右都翻转
        check(0.0, -1.0, 1, 1.0, 0.0)
        assertTrue(steady(-1.0, 0.0, rotation = 1).first < 0)
    }

    @Test fun fasterTurnMovesFartherForSameAngle() {
        // 同样向右转 0.5 rad：2 rad/s 用 0.25 秒，0.25 rad/s 用 2 秒。
        fun distance(rate: Double): Double {
            val m = MotionPan(); var sum = 0.0
            repeat((0.5 / rate / dt).toInt()) { sum += m.step(0.0, -rate, 0, dt, px).first }
            return abs(sum)
        }
        val fast = distance(2.0); val slow = distance(0.25)
        assertTrue("快转 $fast px 应远大于慢转 $slow px", fast > slow * 2)
    }

    @Test fun gainIsSmoothAndBounded() {
        assertEquals(MotionPan.SLOW_GAIN, MotionPan.gain(0.0), 1e-12)
        assertEquals(MotionPan.FAST_GAIN, MotionPan.gain(10.0), 1e-12)
        var prev = 0.0
        for (i in 0..100) { val g = MotionPan.gain(i * 0.02); assertTrue(g >= prev); prev = g }
    }

    @Test fun handTremorIsSuppressed() {
        // 8 Hz、幅度 0.15 rad/s 的手抖（高于死区）：未滤波时会来回抖动，滤波后几乎不动。
        val m = MotionPan(); var total = 0.0; var raw = 0.0
        repeat(250) { i ->
            val w = 0.15 * sin(2 * PI * 8 * i * dt)
            total += abs(m.step(w, w, 0, dt, px).first)
            raw += abs((abs(w) - MotionPan.DEAD_ZONE).coerceAtLeast(0.0)) * dt * px
        }
        assertTrue("滤波后抖动累计 $total px 应远小于未滤波的 $raw px", total < raw * 0.1)
        assertTrue("抖动累计应小于 2 px，实际 $total", total < 2.0)
    }

    @Test fun noiseAtRestDoesNotDrift() {
        val (sx, sy, _) = steady(0.03, -0.04, n = 500)
        assertEquals(0.0, sx, 1e-9); assertEquals(0.0, sy, 1e-9)
    }

    @Test fun longGapsAreCappedAndZeroGapIsIgnored() {
        val m = MotionPan()
        m.step(0.0, -1.0, 0, dt, px)
        val capped = m.step(0.0, -1.0, 0, 5.0, px).first
        assertTrue("间隔应按 ${MotionPan.MAX_DT}s 封顶", abs(capped) <= MotionPan.MAX_DT * MotionPan.FAST_GAIN * px)
        assertEquals(0.0 to 0.0, m.step(0.0, -1.0, 0, 0.0, px))
    }

    @Test fun resetForgetsPreviousMotion() {
        val m = MotionPan()
        repeat(50) { m.step(0.0, -2.0, 0, dt, px) }
        m.reset()
        // 重置后第一次采样直接取当前值：静止时不会带着之前的转动继续平移。
        assertEquals(0.0, m.step(0.0, 0.0, 0, dt, px).first, 1e-9)
    }
}
