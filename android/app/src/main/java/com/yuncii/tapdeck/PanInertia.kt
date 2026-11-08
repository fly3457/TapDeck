package com.yuncii.tapdeck

import kotlin.math.exp
import kotlin.math.hypot

/**
 * 视频模式单指拖动画面后的惯性滑动（单位 dp、秒），与帧率无关。
 *
 * 速度按指数衰减：v(t) = v0·e^(−t/τ)，总滑行距离 = v0·τ，所以甩得越快滑得越远。
 * 抬手速度低于 [MIN_SPEED] 视为放下，不滑动；高于 [MAX_SPEED]（普通甩动速度 [NORMAL_SPEED] 的 2 倍）
 * 时按方向不变截断，最远滑行距离即为普通甩动的 2 倍。
 */
class PanInertia(private val tau: Double = TAU) {
    var vx = 0.0; private set
    var vy = 0.0; private set
    val active get() = vx != 0.0 || vy != 0.0

    /** 抬手时开始滑动；速度太慢时不滑动。返回是否开始滑动。 */
    fun start(vx: Double, vy: Double): Boolean {
        stop()
        val speed = hypot(vx, vy)
        if (!speed.isFinite() || speed < MIN_SPEED) return false
        val k = if (speed > MAX_SPEED) MAX_SPEED / speed else 1.0
        this.vx = vx * k; this.vy = vy * k
        return true
    }

    /** 推进 dt 秒，返回这段时间的位移（dp）；速度衰减到 [STOP_SPEED] 以下即停止。 */
    fun step(dt: Double): Pair<Double, Double> {
        if (!active || dt <= 0) return 0.0 to 0.0
        val decay = exp(-dt / tau)
        val travel = tau * (1 - decay)
        val dx = vx * travel; val dy = vy * travel
        vx *= decay; vy *= decay
        if (hypot(vx, vy) < STOP_SPEED) stop()
        return dx to dy
    }

    /** 画面在某个方向已到边缘：该方向不再滑动。 */
    fun halt(x: Boolean, y: Boolean) {
        if (x) vx = 0.0
        if (y) vy = 0.0
    }

    fun stop() { vx = 0.0; vy = 0.0 }

    companion object {
        /** 衰减时间常数（秒）：总滑行距离 = 抬手速度 × τ。 */
        const val TAU = 0.325
        /** 普通甩动速度（dp/s），约滑行 490dp。 */
        const val NORMAL_SPEED = 1500.0
        /** 速度上限为普通甩动的 2 倍，最远滑行约 975dp。 */
        const val MAX_SPEED = NORMAL_SPEED * 2
        /** 低于该速度抬手视为放下，不滑动（dp/s）。 */
        const val MIN_SPEED = 150.0
        /** 衰减到该速度以下停止（dp/s）。 */
        const val STOP_SPEED = 20.0

        /** 以 v0 抬手、不碰边时的总滑行距离（dp）。 */
        fun glideDistance(speed: Double): Double =
            if (speed < MIN_SPEED) 0.0 else speed.coerceAtMost(MAX_SPEED) * TAU
    }
}
