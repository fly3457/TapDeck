package com.yuncii.tapdeck

import kotlin.math.abs
import kotlin.math.sign

/**
 * 视频模式单指拖动的“平移画面／滚动页面”判定。
 *
 * - 平常单指拖动平移画面；上下方向被画面边缘挡住而没平移出去的部分（溢出，dp）累计超过 [SLOP] 后，
 *   本次拖动进入滚动模式：超出部分和之后的纵向位移都转成 PC 滚轮，且不再平移画面（两种移动不叠加）。
 * - 滚动模式下抬手后 [STICKY_MS] 内再次按下，不论方向都直接是滚动模式；之后恢复为平移画面。
 * - 滚动惯性未停时按下也直接是滚动模式（由调用方用 force 传入）。
 */
class EdgeScroll(private val slop: Double = SLOP, private val stickyMs: Long = STICKY_MS) {
    /** 本次拖动是否为滚动模式。 */
    var scrolling = false; private set
    private var pending = 0.0
    private var stickyUntil = Long.MIN_VALUE

    /** 手指按下（时间为 ms，单调递增）。force 为真时直接进入滚动模式（例如滚动惯性尚未停止）。 */
    fun down(now: Long, force: Boolean = false) {
        pending = 0.0
        scrolling = force || now < stickyUntil
    }

    /** 手指抬起：刚才是滚动模式时，[STICKY_MS] 内的下一次拖动仍是滚动。 */
    fun up(now: Long) {
        if (scrolling) stickyUntil = now + stickyMs
    }

    /**
     * 平移模式下本帧的纵向溢出（手指位移减去视口实际平移，dp）。返回本帧应滚动的距离（dp，与手指方向相同），
     * 未达到阈值时返回 0；达到阈值即切换为滚动模式。
     */
    fun overflow(dy: Double): Double {
        if (!dy.isFinite() || abs(dy) < EPSILON) return 0.0
        if (scrolling) return dy
        // 方向反转时重新累计：来回抖动不会凑满阈值。
        if (pending != 0.0 && sign(pending) != sign(dy)) pending = 0.0
        pending += dy
        if (abs(pending) < slop) return 0.0
        scrolling = true
        val out = pending - sign(pending) * slop
        pending = 0.0
        return out
    }

    companion object {
        /** 开始滚动前需要累计的溢出（dp）。 */
        const val SLOP = 16.0
        /** 滚动后多久内的下一次拖动仍是滚动（ms）。 */
        const val STICKY_MS = 1000L
        /** 浮点误差以下的溢出视为没有。 */
        const val EPSILON = 1e-6
        /** 双指滚动时每 dp 发送的滚轮单位（见 [scrollUnits]：dp × 5 × 1024 / 1024）。 */
        const val WHEEL_UNITS_PER_DP = 5.0
        /**
         * 每 PC 像素对应的滚轮单位（120 为一格）。按努比亚 NX741J + PC-77（4K）上的 Chrome／Claude 实测标定：
         * 原来每 dp 发 5 单位时，页面在手机上移动约为手指的 2.26 倍，折合约 1.18 PC 像素／单位。
         * 不同软件、不同显示缩放比例的滚动步长不同，只能做到近似跟手。
         */
        const val WHEEL_UNITS_PER_PC_PX = 0.85

        /**
         * 手指移动 1dp 时传给 [TouchSink.scroll] 的距离（dp），使页面在手机上大致 1:1 跟手。
         * density 为手机 dp→px，viewScale 为画面缩放（手机像素／PC 像素）。
         */
        fun followGain(density: Double, viewScale: Double): Double {
            if (density <= 0 || viewScale <= 0 || !density.isFinite() || !viewScale.isFinite()) return 1.0
            return density / viewScale * WHEEL_UNITS_PER_PC_PX / WHEEL_UNITS_PER_DP
        }
    }
}
