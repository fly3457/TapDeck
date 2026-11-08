package com.yuncii.tapdeck

import kotlin.math.abs
import kotlin.math.hypot
import kotlin.math.sign

/**
 * 视频模式单指拖动的“平移画面／滚动页面”判定。
 *
 * - 开始拖动时，手指位移累计到 [DECIDE] 就按方向判定一次，整次拖动不再改：
 *   横向为主（|dx| > |dy|）只平移画面，这次拖动永远不会滚页面；竖向为主才允许转成滚动。
 * - 竖向拖动时，上下方向被画面边缘挡住而没平移出去的部分（溢出，dp）累计超过 [SLOP] 后进入滚动模式：
 *   超出部分和之后的纵向位移都转成 PC 滚轮，且不再平移画面（两种移动不叠加）。
 * - 滚动抬手后 [STICKY_MS] 内（或滚动惯性未停时，由调用方用 force 传入）再次按下：先暂存位移、判定方向，
 *   竖向直接滚动（不用再攒 [SLOP]），横向照常平移画面并结束这个窗口。
 */
class EdgeScroll(
    private val slop: Double = SLOP,
    private val stickyMs: Long = STICKY_MS,
    private val decide: Double = DECIDE,
) {
    enum class Axis { Unknown, Horizontal, Vertical }

    /** 本次拖动是否为滚动模式。 */
    var scrolling = false; private set
    /** 本次拖动判定的方向。 */
    var axis = Axis.Unknown; private set
    private var pending = 0.0
    private var stickyUntil = Long.MIN_VALUE
    private var sticky = false
    private var totalX = 0.0
    private var totalY = 0.0

    /** 手指按下（时间为 ms，单调递增）。force 为真表示滚动惯性尚未停止，与 1 秒窗口同样处理。 */
    fun down(now: Long, force: Boolean = false) {
        pending = 0.0
        scrolling = false
        axis = Axis.Unknown
        totalX = 0.0
        totalY = 0.0
        sticky = force || now < stickyUntil
    }

    /** 手指抬起：刚才是滚动模式时，[STICKY_MS] 内的下一次竖向拖动仍是滚动。 */
    fun up(now: Long) {
        if (scrolling) stickyUntil = now + stickyMs
    }

    /**
     * 单指拖动的一帧位移（dp）。返回本帧要交给画面平移或页面滚动的位移；处于滚动窗口且方向未定时暂存并返回 null，
     * 判定后一次性返回暂存的位移。调用后看 [scrolling] 决定是滚动还是平移。
     */
    fun move(dx: Double, dy: Double): Pair<Double, Double>? {
        if (axis != Axis.Unknown) return dx to dy
        if (dx.isFinite()) totalX += dx
        if (dy.isFinite()) totalY += dy
        if (hypot(totalX, totalY) < decide) return if (sticky) null else dx to dy
        axis = if (abs(totalX) > abs(totalY)) Axis.Horizontal else Axis.Vertical
        if (!sticky) return dx to dy
        if (axis == Axis.Vertical) scrolling = true else stickyUntil = Long.MIN_VALUE
        return totalX to totalY
    }

    /**
     * 平移模式下本帧的纵向溢出（手指位移减去视口实际平移，dp）。返回本帧应滚动的距离（dp，与手指方向相同），
     * 横向拖动或未达到阈值时返回 0；达到阈值即切换为滚动模式。
     */
    fun overflow(dy: Double): Double {
        if (!dy.isFinite() || abs(dy) < EPSILON) return 0.0
        if (scrolling) return dy
        if (axis != Axis.Vertical) return 0.0
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
        /** 判定拖动方向所需的手指位移（dp）。 */
        const val DECIDE = 8.0
        /** 开始滚动前需要累计的溢出（dp）。 */
        const val SLOP = 16.0
        /** 滚动后多久内的下一次竖向拖动仍是滚动（ms）。 */
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
