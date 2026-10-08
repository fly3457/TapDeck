package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test
import kotlin.math.abs

class KeyboardGeometryTest {
    @Test fun rowsUseTheRequestedWidthsAndMarginsWithoutAccumulatedRounding() {
        for (width in listOf(320, 411, 640, 720, 1080, 1234, 1404, 2048)) {
            val height = (width * 0.60).toInt()
            val rows = KeyboardGeometry.measure(width, height)
            assertEquals(listOf(10, 9, 9, 5), rows.map { it.size })
            val expected = listOf(
                List(10) { 0.089 }, List(9) { 0.089 },
                listOf(0.1385) + List(7) { 0.089 } + 0.1385,
                listOf(0.188, 0.089, 0.3365, 0.1385, 0.188),
            )
            rows.forEachIndexed { rowIndex, row ->
                row.forEachIndexed { index, cell ->
                    assertEquals("width $width row $rowIndex key $index", width * expected[rowIndex][index], cell.width.toDouble(), 1.0)
                    assertTrue(cell.left >= 0 && cell.right <= width)
                    if (index > 0) assertEquals(width * 0.01, (cell.left - row[index - 1].right).toDouble(), 1.0)
                }
                assertEquals("row must be centered", row.first().left.toDouble(), (width - row.last().right).toDouble(), 1.0)
                if (rowIndex != 1) assertEquals(width * 0.01, row.first().left.toDouble(), 0.6)
                else assertEquals(width * 0.0595, row.first().left.toDouble(), 0.6)
            }
            assertEquals(0, rows.first().first().top)
            assertEquals(height, rows.last().first().bottom)
            assertTrue(rows.map { it.first().height }.let { it.max() - it.min() } <= 1)
        }
    }

    @Test fun shortWindowsOnlyScaleVerticalDimensionsAndReuseNativeMargins() {
        for ((width, height) in listOf(1080 to 2200, 640 to 860, 1404 to 1782, 1080 to 600)) {
            val layout = ControllerLayout.measure(width, height)
            val rows = KeyboardGeometry.measure(width, layout.panelContent, layout.scale)
            for (index in 1..3) {
                val actual = rows[index].first().top - rows[index - 1].first().bottom
                assertEquals(width * 0.01 * layout.scale, actual.toDouble(), 1.0)
            }
            assertEquals(width * 0.089, rows.first().first().width.toDouble(), 1.0)
            assertEquals(layout.panelPadding, layout.panelPadding + rows.first().first().top)
            assertEquals(layout.panelContent, rows.last().last().bottom)
            assertEquals(height, layout.header + layout.touchpad + layout.panel)
        }
    }

    @Test fun touchRegionsSplitInternalGapsWithoutChangingFacesOrOuterMargins() {
        for (width in listOf(320, 411, 640, 720, 1080, 1234, 1404, 2048)) {
            for ((height, scale) in listOf((width * 0.60).toInt() to 1f, 130 to 0.4f)) {
                val faces = KeyboardGeometry.measure(width, height, scale)
                val targets = KeyboardGeometry.touchBounds(faces)
                targets.forEachIndexed { rowIndex, row ->
                    assertEquals(faces[rowIndex].first().left, row.first().left)
                    assertEquals(faces[rowIndex].last().right, row.last().right)
                    row.forEachIndexed { column, target ->
                        val face = faces[rowIndex][column]
                        assertTrue(target.left <= face.left && target.right >= face.right)
                        assertTrue(target.top <= face.top && target.bottom >= face.bottom)
                        if (column > 0) {
                            val previousFace = faces[rowIndex][column - 1]
                            val midpoint = previousFace.right + (face.left - previousFace.right) / 2
                            assertEquals(midpoint, row[column - 1].right)
                            assertEquals(midpoint, target.left)
                        }
                        if (rowIndex > 0) {
                            val previousBottom = faces[rowIndex - 1].first().bottom
                            val midpoint = previousBottom + (face.top - previousBottom) / 2
                            assertEquals(midpoint, targets[rowIndex - 1].first().bottom)
                            assertEquals(midpoint, target.top)
                        }
                    }
                }
                assertEquals(0, targets.first().first().top)
                assertEquals(height, targets.last().first().bottom)
            }
        }
    }

    @Test fun tinyAndAwkwardPixelSizesNeverProduceOverlapOrOutOfBoundsCells() {
        for (width in 0..1024) for (height in listOf(0, 1, 2, 3, 12, 39, 600)) {
            val layout = ControllerLayout.measure(width, height)
            val rows = KeyboardGeometry.measure(width, layout.panelContent, layout.scale)
            for (regions in listOf(rows, KeyboardGeometry.touchBounds(rows))) regions.forEachIndexed { index, row ->
                row.forEachIndexed { column, cell ->
                    assertTrue("$width/$height: $cell", cell.width >= 0 && cell.height >= 0 && cell.left >= 0 && cell.right <= width && cell.top >= 0 && cell.bottom <= layout.panelContent)
                    if (column > 0) assertTrue(row[column - 1].right <= cell.left)
                    if (index > 0) assertTrue(regions[index - 1].first().bottom <= cell.top)
                }
            }
            if (width > 0) assertTrue(abs(rows[0].last().right + rows[0].first().left - width) <= 1)
        }
    }
}
