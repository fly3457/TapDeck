package com.yuncii.tapdeck

import kotlin.math.abs
import kotlin.math.sign

/**
 * 视频模式体感控制：把陀螺仪角速度换算成画面平移，效果等同单指拖动画面。
 *
 * 手机像一扇看向电脑画面的窗：向右转看到右边（画面向左移），把手机顶部朝自己倾斜（背面朝下）看到下面（画面向上移）。
 * 绕屏幕法线的转动（在桌面上旋转手机）不参与。
 */
internal object MotionPan {
    /** 角速度低于该值（rad/s）视为手抖或零漂，不平移；超过部分才计入，起步不突跳。 */
    const val DEAD_ZONE = 0.03
    /** 每弧度平移的距离为视图长边的倍数：转动约 30° 平移约 0.6 个长边。 */
    const val PAN_PER_RAD = 1.2
    /** 两次采样间隔上限（秒）：暂停后恢复时不按很长的间隔一次性跳动。 */
    const val MAX_DT = 0.1

    /**
     * 设备坐标系角速度 (wx, wy)（rad/s，SensorEvent.values[0..1]）在 dt 秒内对应的画面平移（像素）。
     * rotation 为 Surface.ROTATION_0..270（0..3），横屏时按屏幕方向换算坐标轴。
     */
    fun delta(wx: Double, wy: Double, rotation: Int, dt: Double, pxPerRad: Double): Pair<Double, Double> {
        // 屏幕横轴、竖轴在设备坐标系中的方向（ROTATION_90 表示手机逆时针转了 90°）。
        val (sx, sy) = when (rotation and 3) {
            1 -> (0.0 to -1.0) to (1.0 to 0.0)
            2 -> (-1.0 to 0.0) to (0.0 to -1.0)
            3 -> (0.0 to 1.0) to (-1.0 to 0.0)
            else -> (1.0 to 0.0) to (0.0 to 1.0)
        }
        val pitch = wx * sx.first + wy * sx.second // 绕屏幕横轴：正值为顶部朝自己倾斜
        val yaw = wx * sy.first + wy * sy.second // 绕屏幕竖轴：正值为向左转
        val t = dt.coerceIn(0.0, MAX_DT) * pxPerRad
        return deadZone(yaw) * t to -deadZone(pitch) * t
    }

    private fun deadZone(v: Double) = if (abs(v) <= DEAD_ZONE) 0.0 else v - sign(v) * DEAD_ZONE
}
