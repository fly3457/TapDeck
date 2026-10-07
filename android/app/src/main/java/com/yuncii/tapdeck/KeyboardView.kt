package com.yuncii.tapdeck

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** 键的行为类型。 */
private enum class Kind {
    /** 字母 / 数字 / 符号 / 空格：短按主键位，长按右上角副键位。 */
    Dual,

    /** Shift：短按单次大写，长按锁定大写。 */
    Shift,

    /** Alt / 退格 / 回车 / Shift+Enter：短按单次触发，长按真按住、连续触发。 */
    Hold,

    /** 「.」键：短按句点，长按等于长按语音输入键。 */
    VoiceDual,
}

/** 键盘上的一个键：中间主键位、右上角副键位（可为空）。 */
private data class Key(
    val primary: String,
    val secondary: String = "",
    val mainLabel: String = "",
    val altLabel: String = "",
    val kind: Kind = Kind.Dual,
    /** 键宽，单位是可用宽度的百分比；常规字母键 8.5%。 */
    val widthPercent: Float = 8.5f,
)

/** 键之间的间隙与左右留白，都是可用宽度的百分比。 */
private const val KEY_GAP_PERCENT = 1.5f
private const val SIDE_PADDING_PERCENT = 0.75f

/**
 * 全键盘。激活后取代下方的快捷键区与语音区，占据屏幕下半部分。
 *
 * 布局按需求逐格排：
 * 1 行 副键 `1234567890` / 主键 `qwertyuiop`
 * 2 行 副键 `-/:;()~'"` / 主键 `asdfghjkl`
 * 3 行 副键 `[Shift]@-#&?!…[Backspace]` / 主键 `[Shift]zxcvbnm[Backspace]`
 * 4 行 副键 `[alt][语音输入][Shift+Enter][Enter]` / 主键 `[alt].[空格][Shift+Enter][Enter]`
 *
 * 每格承载两个键位：中间是**主键位**（短按），右上角是**副键位**（长按）。
 *
 * 宽度按屏幕宽度的百分比固定：常规字母键与「.」8.5%、间隙 1.5%、左右各留 0.75%。
 * 因此 1 / 3 / 4 行的总宽 = 8.5×n + 1.5×(n-1) + 0.75×2 = 100%（3 行的 `[Shift]`、
 * `[Backspace]` 各 13.5%，4 行的 `[alt]`、`[Enter]` 各 18.5%、`[Shift+Enter]` 13.5%、
 * 空格 33.5%）；2 行只有 9 键，占 88.5%，左边对齐、不铺满。
 */
@Composable
fun KeyboardView(
    connected: Boolean,
    voiceActive: Boolean,
    hold: KeyHold,
    beginVoice: (String) -> Boolean,
    stopVoice: () -> Unit,
) {
    val rows: List<List<Key>> = listOf(
        // 1 行：副键位数字 + 主键位字母
        listOf("1|Q", "2|W", "3|E", "4|R", "5|T", "6|Y", "7|U", "8|I", "9|O", "0|P").map { dual(it) },
        // 2 行：副键位符号 + 主键位字母（a - s / d : f ; g ( h ) j ~ k ' l "）
        listOf(
            "Minus|A", "Slash|S", "LeftShift+Semicolon|D", "Semicolon|F",
            "LeftShift+9|G", "LeftShift+0|H", "LeftShift+Backquote|J", "Quote|K", "LeftShift+Quote|L",
        ).map { dual(it) },
        // 3 行：Shift + 副键位符号 + 主键位字母 + 退格
        listOf(
            Key(KeyHold.SHIFT, mainLabel = "⇧", kind = Kind.Shift, widthPercent = 13.5f),
            dual("LeftShift+2|Z"),
            dual("Minus|X"),
            dual("LeftShift+3|C"),
            dual("LeftShift+7|V"),
            dual("LeftShift+Slash|B"),
            dual("LeftShift+1|N"),
            dual("Ellipsis|M"),
            Key("Backspace", mainLabel = "⌫", kind = Kind.Hold, widthPercent = 13.5f),
        ),
        // 4 行：Alt + 「.（长按语音输入）」+ 空格（长按 Shift+Enter）+ Shift+Enter + 回车
        listOf(
            Key("LeftAlt", mainLabel = "Alt", kind = Kind.Hold, widthPercent = 18.5f),
            Key("Period", mainLabel = ".", altLabel = "🎤", kind = Kind.VoiceDual),
            dual("LeftShift+Enter|Space").copy(mainLabel = "空格", altLabel = "⇧⏎", widthPercent = 33.5f),
            Key("LeftShift+Enter", "Enter", mainLabel = "⇧⏎", altLabel = "⏎", kind = Kind.Dual, widthPercent = 13.5f),
            Key("Enter", mainLabel = "⏎", kind = Kind.Hold, widthPercent = 18.5f),
        ),
    )

    Surface(modifier = Modifier.fillMaxSize(), color = Color(0xFFF1F3F6)) {
        BoxWithConstraints(Modifier.fillMaxSize()) {
            val available = maxWidth
            val gap = available * (KEY_GAP_PERCENT / 100f)
            val side = available * (SIDE_PADDING_PERCENT / 100f)
            Column(
                Modifier.fillMaxSize().padding(horizontal = side, vertical = side),
                verticalArrangement = Arrangement.spacedBy(gap),
            ) {
                rows.forEach { row ->
                    Row(
                        Modifier.weight(1f).fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(gap),
                    ) {
                        row.forEach { item ->
                            KeyboardKeyCell(
                                item = item,
                                width = available * (item.widthPercent / 100f),
                                connected = connected,
                                voiceActive = voiceActive,
                                hold = hold,
                                beginVoice = beginVoice,
                                stopVoice = stopVoice,
                            )
                        }
                    }
                }
            }
        }
    }
}

/** 解析 "副键位|主键位"，生成带键面标签的按键。 */
private fun dual(spec: String): Key {
    val parts = spec.split('|', limit = 2)
    val secondary = if (parts.size == 2) parts[0] else ""
    val primary = if (parts.size == 2) parts[1] else parts[0]
    return Key(primary = primary, secondary = secondary, mainLabel = label(primary), altLabel = label(secondary))
}

/** 键名 -> 键面显示。 */
private fun label(name: String): String = when (name) {
    "" -> ""
    "Space" -> "空格"
    "Minus" -> "-"
    "Slash" -> "/"
    "Semicolon" -> ";"
    "Quote" -> "'"
    "Comma" -> ","
    "Period" -> "."
    "Enter" -> "⏎"
    "Backspace" -> "⌫"
    "LeftAlt" -> "Alt"
    "LeftShift+1" -> "!"
    "LeftShift+2" -> "@"
    "LeftShift+3" -> "#"
    "LeftShift+7" -> "&"
    "LeftShift+9" -> "("
    "LeftShift+0" -> ")"
    "LeftShift+Semicolon" -> ":"
    "LeftShift+Quote" -> "\""
    "LeftShift+Backquote" -> "~"
    "LeftShift+Slash" -> "?"
    "LeftShift+Enter" -> "⇧⏎"
    "Ellipsis" -> "…"
    else -> name
}

@Composable
private fun RowScope.KeyboardKeyCell(
    item: Key,
    width: Dp,
    connected: Boolean,
    voiceActive: Boolean,
    hold: KeyHold,
    beginVoice: (String) -> Boolean,
    stopVoice: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    val active = when (item.kind) {
        Kind.Shift -> hold.shiftOn
        Kind.VoiceDual -> voiceActive
        Kind.Hold -> hold.isHeld(item.primary)
        Kind.Dual -> hold.isPressed(KeyHold.dualId(item.primary, item.secondary)) || hold.isPressed(item.primary)
    }
    val description = when (item.kind) {
        Kind.Shift -> "Shift：短按单次大写，长按锁定大写"
        Kind.VoiceDual -> "句点：短按输入句点，长按等于长按语音输入"
        Kind.Hold -> when (item.primary) {
            "Backspace" -> "退格：短按一次，长按连续退格"
            "LeftAlt" -> "Alt：短按一次，长按连续按住"
            else -> "回车：短按一次，长按连续回车"
        }
        Kind.Dual -> if (item.altLabel.isEmpty()) "${item.mainLabel} 键"
        else "${item.mainLabel} 键，长按输入 ${item.altLabel}"
    }
    Surface(
        modifier = Modifier.width(width).fillMaxHeight()
            .semantics { contentDescription = description }
            .keyGesture(item, connected, hold, scope, beginVoice, stopVoice),
        shape = MaterialTheme.shapes.small,
        color = if (active) Color(0xFF175CD3) else Color.White,
        contentColor = if (active) Color.White else Color(0xFF1F2A37),
        border = BorderStroke(1.dp, if (active) Color(0xFF175CD3) else Color(0xFFCBD5E1)),
    ) {
        Box(Modifier.fillMaxSize().padding(horizontal = 1.dp)) {
            if (item.altLabel.isNotEmpty()) {
                Text(
                    text = item.altLabel,
                    modifier = Modifier.align(Alignment.TopEnd).padding(top = 1.dp, end = 2.dp),
                    fontSize = 10.sp, lineHeight = 11.sp,
                    color = if (active) Color.White.copy(alpha = 0.85f) else Color(0xFF8A94A6),
                    maxLines = 1, overflow = TextOverflow.Clip,
                )
            }
            if (item.mainLabel.isNotEmpty()) {
                Text(
                    text = item.mainLabel,
                    modifier = Modifier.align(Alignment.Center),
                    fontSize = if (item.mainLabel.length <= 2) 16.sp else 12.sp,
                    lineHeight = 17.sp, textAlign = TextAlign.Center,
                    maxLines = 1, overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

/** 一个键的按下 / 抬起手势。 */
private fun Modifier.keyGesture(
    item: Key,
    connected: Boolean,
    hold: KeyHold,
    scope: CoroutineScope,
    beginVoice: (String) -> Boolean,
    stopVoice: () -> Unit,
): Modifier = pointerInput(connected, item.primary, item.secondary, item.kind) {
    if (!connected) return@pointerInput
    awaitEachGesture {
        // 父布局会先消费按下事件，所以这里不要求未消费。
        awaitFirstDown(requireUnconsumed = false)
        when (item.kind) {
            // 双键位：按住期间由 KeyHold 计时切到副键位，这里只负责按下与抬起。
            Kind.Dual -> {
                if (hold.shiftOn && isLetter(item.primary)) {
                    hold.pressWithShift(item.primary)
                    waitForUpOrCancellation()
                    hold.releaseWithShift(item.primary)
                } else {
                    hold.pressDual(item.primary, item.secondary)
                    waitForUpOrCancellation()
                    hold.releaseDual(item.primary, item.secondary)
                }
            }
            // 短按单次触发，长按真按住（退格 / Alt / 回车）。
            Kind.Hold -> {
                var long = false
                val timer = scope.launch {
                    delay(KeyHold.LONG_PRESS_MS)
                    long = true
                    hold.holdDown(item.primary)
                }
                val up = waitForUpOrCancellation()
                timer.cancel()
                if (long) hold.holdUp(item.primary) else if (up != null) hold.tap(item.primary)
            }
            // Shift：短按单次大写，长按锁定大写。
            Kind.Shift -> {
                var locked = false
                val timer = scope.launch {
                    delay(KeyHold.LONG_PRESS_MS)
                    locked = true
                    hold.lockShift()
                }
                val up = waitForUpOrCancellation()
                timer.cancel()
                if (!locked && up != null) hold.tapShift()
            }
            // 「.」：短按句点，长按等于长按语音输入（按住说话，松手结束）。
            Kind.VoiceDual -> {
                var talking = false
                val timer = scope.launch {
                    delay(KeyHold.LONG_PRESS_MS)
                    talking = beginVoice(MicBallView.MODE_HOLD)
                }
                val up = waitForUpOrCancellation()
                timer.cancel()
                if (talking) stopVoice() else if (up != null) hold.tap(item.primary)
            }
        }
    }
}

private fun isLetter(name: String) = name.length == 1 && name[0] in 'A'..'Z'
