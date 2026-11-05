package com.yuncii.tapdeck

import kotlin.math.PI
import kotlin.math.abs

/**
 * 视频模式体感控制：把陀螺仪角速度换算成画面平移，效果等同单指拖动画面。
 *
 * 向右转看到右边（画面向左移）；手机顶部朝自己倾斜，画面向下移、看到上面（与“窗口”直觉相反，按用户实测要求反转）。
 * 平移只与转过的角度成正比：转得快慢只影响画面移动得快慢，同样的转角始终移动同样的距离（0.5.6–0.5.9 曾有加速度曲线，0.5.10 按用户要求去掉）。
 * 绕屏幕法线的转动（在桌面上旋转手机）不参与。
 *
 * 防抖分两步：先用 One Euro 自适应低通滤波压掉手抖（慢时截止频率低、更平稳，快转时截止频率升高、不拖沓），
 * 再经平滑门限：低于 [DEAD_ZONE] 完全不动，[DEAD_ZONE]..[FULL_RATE] 之间逐渐放开，超过后原样计入、不扣减距离。
 * 每次开始监听传感器时调用 [reset]。
 */
internal class MotionPan {
    companion object {
        /** 滤波后角速度低于该值（rad/s）视为静止或零漂，不平移。 */
        const val DEAD_ZONE = 0.05
        /** 角速度达到该值（rad/s，约 6°/s）起原样计入；与 [DEAD_ZONE] 之间平滑过渡，起步不突跳。 */
        const val FULL_RATE = 0.1
        /** 每弧度平移的距离为视图长边的倍数（0.5.8 曾降为 0.96，0.5.9 按实测恢复）。 */
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

        /** 平滑门限：死区内为 0，到 [FULL_RATE] 时为原值，中间按 smoothstep 过渡。 */
        fun gate(v: Double): Double {
            val t = ((abs(v) - DEAD_ZONE) / (FULL_RATE - DEAD_ZONE)).coerceIn(0.0, 1.0)
            return v * t * t * (3 - 2 * t)
        }
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
        val k = t * pxPerRad
        return gate(yaw.filter(y, t)) * k to gate(pitch.filter(p, t)) * k
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
