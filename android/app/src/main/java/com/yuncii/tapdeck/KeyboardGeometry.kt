package com.yuncii.tapdeck

import kotlin.math.min
import kotlin.math.roundToInt

internal data class KeyboardCellBounds(val left: Int, val top: Int, val right: Int, val bottom: Int) {
    val width: Int get() = right - left
    val height: Int get() = bottom - top
}

/** Keyboard-only dimensions; the ordinary shortcut and voice panel use their own spacing. */
internal object KeyboardGeometry {
    const val SIDE = 0.01
    const val GAP = 0.01
    const val ORDINARY = (1.0 - 2 * SIDE - 9 * GAP) / 10
    private const val UNIT = ORDINARY + GAP
    const val WIDE = 1.5 * UNIT - GAP
    const val DOUBLE = 2 * UNIT - GAP
    const val SPACE = 3.5 * UNIT - GAP

    val rowWidths: List<List<Double>> = listOf(
        List(10) { ORDINARY },
        List(9) { ORDINARY },
        listOf(WIDE) + List(7) { ORDINARY } + WIDE,
        listOf(DOUBLE, ORDINARY, SPACE, WIDE, DOUBLE),
    )

    /** Height excludes the native panel's existing top/bottom margins. Round edges, not widths. */
    fun measure(width: Int, height: Int, scale: Float = 1f): List<List<KeyboardCellBounds>> {
        val w = width.coerceAtLeast(0)
        val h = height.coerceAtLeast(0)
        val verticalScale = if (scale.isFinite()) scale.coerceIn(0f, 1f).toDouble() else 0.0
        val rowGap = min(w * GAP * verticalScale, h / 3.0)
        val rowHeight = (h - rowGap * 3) / 4
        return rowWidths.mapIndexed { rowIndex, widths ->
            val top = (rowIndex * (rowHeight + rowGap)).roundToInt().coerceIn(0, h)
            val bottom = (rowIndex * (rowHeight + rowGap) + rowHeight).roundToInt().coerceIn(top, h)
            val total = widths.sum() + (widths.size - 1) * GAP
            var x = w * (1.0 - total) / 2
            widths.map { fraction ->
                val left = x.roundToInt().coerceIn(0, w)
                val right = (x + w * fraction).roundToInt().coerceIn(left, w)
                x += w * (fraction + GAP)
                KeyboardCellBounds(left, top, right, bottom)
            }
        }
    }

    /** Internal gaps are visual only: neighbours share them at a single pixel boundary. */
    fun touchBounds(rows: List<List<KeyboardCellBounds>>): List<List<KeyboardCellBounds>> =
        rows.mapIndexed { rowIndex, row ->
            row.mapIndexed { column, cell ->
                val top = if (rowIndex == 0) cell.top else {
                    val previous = rows[rowIndex - 1].first().bottom
                    previous + (cell.top - previous) / 2
                }
                val bottom = if (rowIndex == rows.lastIndex) cell.bottom else
                    cell.bottom + (rows[rowIndex + 1].first().top - cell.bottom) / 2
                val left = if (column == 0) cell.left else {
                    val previous = row[column - 1].right
                    previous + (cell.left - previous) / 2
                }
                val right = if (column == row.lastIndex) cell.right else
                    cell.right + (row[column + 1].left - cell.right) / 2
                KeyboardCellBounds(left, top, right, bottom)
            }
        }
}
