package com.yuncii.tapdeck

import kotlin.math.abs
import kotlin.math.sign

/**
 * 视频模式单指拖动到上下边缘后继续拖：把视口没吃掉的纵向位移（溢出，dp）转成 PC 滚轮。
 *
 * 每次拖动的溢出累计超过 [SLOP] 才开始滚动，避免左右平移时手指的上下抖动带出滚动；
 * 开始后超出阈值的部分和之后的全部溢出都返回给调用方去滚动。只处理纵向。
 */
class EdgeScroll(private val slop: Double = SLOP) {
    /** 本次拖动是否已开始滚动。 */
    var scrolling = false; private set
    private var pending = 0.0

    /** 新的一次拖动开始（手指按下）。 */
    fun reset() { scrolling = false; pending = 0.0 }

    /**
     * 本帧纵向溢出（手指位移减去视口实际平移，dp）。返回本帧应滚动的距离（dp，与手指方向相同），
     * 未达到阈值时返回 0。
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
        /** 浮点误差以下的溢出视为没有。 */
        const val EPSILON = 1e-6
    }
}
