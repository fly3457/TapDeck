package com.yuncii.tapdeck

import kotlin.math.PI
import kotlin.math.abs
import kotlin.math.sign

/**
 * 视频模式体感控制：把陀螺仪角速度换算成画面平移，效果等同单指拖动画面。
 *
 * 手机像一扇看向电脑画面的窗：向右转看到右边（画面向左移），把手机顶部朝自己倾斜（背面朝下）看到下面（画面向上移）。
 * 绕屏幕法线的转动（在桌面上旋转手机）不参与。
 *
 * 防抖分两步：先用 One Euro 自适应低通滤波压掉手抖（慢时截止频率低、更平稳，快转时截止频率升高、不拖沓），
 * 再减去死区，静止或零漂时完全不动。每次开始监听传感器时调用 [reset]。
 */
internal class MotionPan {
    companion object {
        /** 滤波后角速度低于该值（rad/s）视为静止，不平移；超过部分才计入，起步不突跳。 */
        const val DEAD_ZONE = 0.05
        /** 每弧度平移的距离为视图长边的倍数：转动约 30° 平移约 0.6 个长边。 */
        const val PAN_PER_RAD = 1.2
        /** 两次采样间隔上限（秒）：暂停后恢复时不按很长的间隔一次性跳动。 */
        const val MAX_DT = 0.1
        /** One Euro 参数：静止截止频率（Hz）、随角加速度提高截止频率的系数、角加速度估计的截止频率（Hz）。 */
        const val MIN_CUTOFF = 1.2
        const val BETA = 0.2
        const val D_CUTOFF = 1.0

        /** 设备坐标系角速度 (wx, wy) 按屏幕方向换算为（绕屏幕横轴的俯仰, 绕屏幕竖轴的偏航）。ROTATION_90 表示手机逆时针转了 90°。 */
        fun screenRates(wx: Double, wy: Double, rotation: Int): Pair<Double, Double> {
            val (sx, sy) = when (rotation and 3) {
                1 -> (0.0 to -1.0) to (1.0 to 0.0)
                2 -> (-1.0 to 0.0) to (0.0 to -1.0)
                3 -> (0.0 to 1.0) to (-1.0 to 0.0)
                else -> (1.0 to 0.0) to (0.0 to 1.0)
            }
            // 俯仰正值为顶部朝自己倾斜，偏航正值为向左转。
            return wx * sx.first + wy * sx.second to wx * sy.first + wy * sy.second
        }

        private fun deadZone(v: Double) = if (abs(v) <= DEAD_ZONE) 0.0 else v - sign(v) * DEAD_ZONE
    }

    private val pitch = OneEuro(MIN_CUTOFF, BETA, D_CUTOFF)
    private val yaw = OneEuro(MIN_CUTOFF, BETA, D_CUTOFF)

    fun reset() { pitch.reset(); yaw.reset() }

    /**
     * 一次陀螺仪采样（rad/s，SensorEvent.values[0..1]）在 dt 秒内对应的画面平移（像素）。
     * rotation 为 Surface.ROTATION_0..270（0..3）。
     */
    fun step(wx: Double, wy: Double, rotation: Int, dt: Double, pxPerRad: Double): Pair<Double, Double> {
        val t = dt.coerceIn(0.0, MAX_DT)
        if (t <= 0.0) return 0.0 to 0.0
        val (p, y) = screenRates(wx, wy, rotation)
        val fp = pitch.filter(p, t); val fy = yaw.filter(y, t)
        return deadZone(fy) * t * pxPerRad to -deadZone(fp) * t * pxPerRad
    }
}

/** One Euro 滤波（Casiez 等，CHI 2012）：截止频率随信号变化速度自适应，兼顾去抖与低延迟。 */
internal class OneEuro(private val minCutoff: Double, private val beta: Double, private val dCutoff: Double) {
    private var x = Double.NaN
    private var dx = 0.0

    fun reset() { x = Double.NaN; dx = 0.0 }

    fun filter(value: Double, dt: Double): Double {
        if (x.isNaN()) { x = value; dx = 0.0; return value }
        dx += alpha(dCutoff, dt) * ((value - x) / dt - dx)
        x += alpha(minCutoff + beta * abs(dx), dt) * (value - x)
        return x
    }

    private fun alpha(cutoff: Double, dt: Double): Double { val tau = 1.0 / (2 * PI * cutoff); return dt / (dt + tau) }
}
