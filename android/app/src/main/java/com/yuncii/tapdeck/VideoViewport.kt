package com.yuncii.tapdeck

import kotlin.math.abs
import kotlin.math.max
import kotlin.math.min

/**
 * 视频模式的视口：PC 画面（content，单位为 PC 屏幕像素）在手机视图（view，像素）中的
 * 缩放与位置。横竖屏共用同一条规则——按高度填满、水平居中：竖屏裁掉左右只显示中间，
 * 20:9 横屏完整显示并在两侧留窄黑边。画面本身从不旋转。
 *
 * 用户捏合后保持用户的缩放与位置；PC 光标移动到视图边缘的跟随区时平滑平移，保证光标可见。
 */
class VideoViewport(private val followMargin: Double = 0.12, private val smoothing: Double = 0.35) {
    var viewWidth = 0.0; private set
    var viewHeight = 0.0; private set
    var contentWidth = 0.0; private set
    var contentHeight = 0.0; private set
    var scale = 1.0; private set
    var offsetX = 0.0; private set
    var offsetY = 0.0; private set
    private var targetX = 0.0
    private var targetY = 0.0
    private var cursorX = Double.NaN
    private var cursorY = Double.NaN

    val ready get() = viewWidth > 0 && viewHeight > 0 && contentWidth > 0 && contentHeight > 0
    val defaultScale get() = if (contentHeight > 0) viewHeight / contentHeight else 1.0
    /** 最小为完整显示整个 PC 画面，最大为 PC 像素的 3 倍（且至少默认缩放的 4 倍）。 */
    val minScale get() = min(defaultScale, if (contentWidth > 0) viewWidth / contentWidth else 1.0)
    val maxScale get() = max(3.0, defaultScale * 4)
    val displayWidth get() = contentWidth * scale
    val displayHeight get() = contentHeight * scale

    /** 视图尺寸（含横竖屏切换）或 PC 画面尺寸变化：重新套用默认规则，并尽量让光标留在画面内。 */
    fun resize(viewWidth: Double, viewHeight: Double, contentWidth: Double, contentHeight: Double) {
        if (viewWidth == this.viewWidth && viewHeight == this.viewHeight && contentWidth == this.contentWidth && contentHeight == this.contentHeight) return
        this.viewWidth = viewWidth; this.viewHeight = viewHeight
        this.contentWidth = contentWidth; this.contentHeight = contentHeight
        reset()
    }

    fun reset() {
        if (!ready) return
        scale = defaultScale
        offsetX = (viewWidth - displayWidth) / 2
        offsetY = (viewHeight - displayHeight) / 2
        if (!cursorX.isNaN() && displayWidth > viewWidth) offsetX = viewWidth / 2 - cursorX * scale
        clamp()
        targetX = offsetX; targetY = offsetY
    }

    /** 以双指中心为焦点缩放，同时随双指中心平移。 */
    fun pinch(factor: Double, focusX: Double, focusY: Double, panX: Double, panY: Double) {
        if (!ready || !factor.isFinite() || factor <= 0) return
        val next = (scale * factor).coerceIn(minScale, maxScale)
        val ratio = next / scale
        offsetX = focusX - (focusX - offsetX) * ratio + panX
        offsetY = focusY - (focusY - offsetY) * ratio + panY
        scale = next
        clamp()
        targetX = offsetX; targetY = offsetY
    }

    /** PC 光标位置（PC 屏幕像素）。只在光标移动时更新跟随目标，不会拉回用户刚平移的视野。 */
    fun cursor(x: Double, y: Double) {
        val moved = cursorX.isNaN() || abs(x - cursorX) > 0.5 || abs(y - cursorY) > 0.5
        cursorX = x; cursorY = y
        if (!ready || !moved) return
        targetX = follow(targetX, x * scale, viewWidth, displayWidth)
        targetY = follow(targetY, y * scale, viewHeight, displayHeight)
    }

    private fun follow(offset: Double, position: Double, view: Double, display: Double): Double {
        if (display <= view) return (view - display) / 2
        val margin = view * followMargin
        val onScreen = offset + position
        val next = when {
            onScreen < margin -> margin - position
            onScreen > view - margin -> view - margin - position
            else -> offset
        }
        return next.coerceIn(view - display, 0.0)
    }

    /** 每帧调用：向跟随目标平滑移动。返回位置是否变化。 */
    fun step(): Boolean {
        val dx = targetX - offsetX
        val dy = targetY - offsetY
        if (abs(dx) < 0.5 && abs(dy) < 0.5) {
            if (dx == 0.0 && dy == 0.0) return false
            offsetX = targetX; offsetY = targetY
            return true
        }
        offsetX += dx * smoothing
        offsetY += dy * smoothing
        return true
    }

    /** 画面小于视图时居中；大于视图时不露出画面外的黑边。 */
    private fun clamp() {
        offsetX = if (displayWidth <= viewWidth) (viewWidth - displayWidth) / 2 else offsetX.coerceIn(viewWidth - displayWidth, 0.0)
        offsetY = if (displayHeight <= viewHeight) (viewHeight - displayHeight) / 2 else offsetY.coerceIn(viewHeight - displayHeight, 0.0)
    }
}
