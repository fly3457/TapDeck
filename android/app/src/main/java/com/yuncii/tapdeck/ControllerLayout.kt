package com.yuncii.tapdeck

import kotlin.math.min
import kotlin.math.roundToInt

/** All sizes are based on the measured viewport, after system-bar/cutout insets. */
data class ControllerLayout(
    val width: Int,
    val height: Int,
    val scale: Float,
    val header: Int,
    val panelContent: Int,
    val panelPadding: Int,
) {
    val panel: Int get() = panelContent + panelPadding * 2
    val touchpad: Int get() = (height - header - panel).coerceAtLeast(0)

    companion object {
        fun measure(width: Int, height: Int): ControllerLayout {
            val w = width.coerceAtLeast(0)
            val h = height.coerceAtLeast(0)
            val scale = if (w == 0) 0f else min(1f, h / (0.82f * w))
            val unit = w * scale
            return ControllerLayout(w, h, scale, (unit * 0.10f).roundToInt(),
                (unit * 0.60f).roundToInt(), (unit * 0.01f).roundToInt())
        }
    }
}

/** Shared by native Canvas controls and Compose key surfaces. ARGB colors. */
object ControllerStyle {
    val PANEL = 0xFFDCDDDF.toInt()
    val NORMAL = 0xFFFFFFFF.toInt()
    val SPECIAL = 0xFFB6B9C2.toInt()
    val PRESSED = 0xFF175CD3.toInt()
    val BORDER = 0xFFD0D2D6.toInt()
    val SPECIAL_BORDER = 0xFFAEB1BA.toInt()
    val LABEL = 0xFF111216.toInt()
    val SECONDARY = 0xFF929796.toInt()
    val DISCONNECTED = 0xFFF59E0B.toInt()
    val CONNECTED = 0xFF22C55E.toInt()
    const val CORNER = 0.0125f
    const val SIDE = 0.0075f
    const val GAP = 0.015f
}

/** Leaves room for the mode row, footer and recording ring even in short windows. */
data class VoiceGeometry(val width: Float, val height: Float, val unit: Float) {
    val modeHeight = unit * 0.07f
    val footerHeight = unit * 0.045f
    val footerTop = (height - footerHeight).coerceAtLeast(modeHeight)
    val controlTop = modeHeight + unit * 0.005f
    val controlBottom = (footerTop - unit * 0.005f).coerceAtLeast(controlTop)
    val halo = unit * 0.012f
    val radius = min(unit * 0.06f, ((controlBottom - controlTop) / 2 - halo).coerceAtLeast(0f))
    val left = radius + halo + width * ControllerStyle.SIDE
    val right = (width - left).coerceAtLeast(left)
    val top = controlTop + radius + halo
    val bottom = (controlBottom - radius - halo).coerceAtLeast(top)

    fun center(nx: Float, ny: Float): Pair<Float, Float> =
        (left + (right - left) * nx.coerceIn(0f, 1f)) to (top + (bottom - top) * ny.coerceIn(0f, 1f))
}
