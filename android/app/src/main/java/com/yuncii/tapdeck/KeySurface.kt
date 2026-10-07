package com.yuncii.tapdeck

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Surface
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalViewConfiguration
import androidx.compose.ui.platform.ViewConfiguration
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.DpSize
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp

/** Dense keyboard cells use their actual bounds; dialogs retain normal touch targets. */
@Composable
fun CompactControls(content: @Composable () -> Unit) {
    val original = LocalViewConfiguration.current
    val compact = remember(original) {
        object : ViewConfiguration by original {
            override val minimumTouchTargetSize = DpSize.Zero
        }
    }
    CompositionLocalProvider(LocalViewConfiguration provides compact, content = content)
}

/** Convert a width-derived visual size to sp without magnifying it twice. */
@Composable
fun widthFont(width: Dp, fraction: Float, scale: Float = 1f): TextUnit =
    with(LocalDensity.current) { (width * fraction * scale).toSp() }

@Composable
fun KeySurface(
    modifier: Modifier,
    viewportWidth: Dp,
    active: Boolean = false,
    special: Boolean = false,
    content: @Composable () -> Unit,
) {
    Surface(
        modifier = modifier,
        shape = RoundedCornerShape(viewportWidth * ControllerStyle.CORNER),
        color = Color(when {
            active -> ControllerStyle.PRESSED
            special -> ControllerStyle.SPECIAL
            else -> ControllerStyle.NORMAL
        }),
        contentColor = Color(if (active) ControllerStyle.NORMAL else ControllerStyle.LABEL),
        border = BorderStroke(0.5.dp, Color(when {
            active -> ControllerStyle.PRESSED
            special -> ControllerStyle.SPECIAL_BORDER
            else -> ControllerStyle.BORDER
        })),
        shadowElevation = 1.dp,
        content = content,
    )
}
