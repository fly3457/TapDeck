package com.yuncii.tapdeck

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.layout.*
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
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
    /** 字母 / 数字 / 符号 / `.`：短按主键位一次，长按副键位一次。 */
    Dual,

    /** Shift：短按单次大写，长按锁定大写。 */
    Shift,

    /** Alt / 退格 / 回车 / Shift+Enter：短按一次，长按真按住、连续触发。 */
    Hold,

    /** 空格：短按一次空格；长按＝长按语音输入（开始传音并保持 PC 长按热键，松手结束）。 */
    VoiceDual,
}

/** 键盘上的一个键：主键位（短按）与副键位（长按）。 */
private data class Key(
    val primary: String,
    val secondary: String = "",
    val mainLabel: String = "",
    val altLabel: String = "",
    val kind: Kind = Kind.Dual,
    /** 特殊键（Shift / 退格 / Alt / Shift+Enter / Enter）用灰底，常规键白底。 */
    val special: Boolean = false,
    /** 键宽，单位是可用宽度的百分比；常规字母键 8.5%。 */
    val widthPercent: Float = 8.5f,
)

/** 键之间的间隙与左右留白，都是可用宽度的百分比。 */
private const val KEY_GAP_PERCENT = 1.5f
private const val SIDE_PADDING_PERCENT = 0.75f

/** 常规键（字母 / `.` / 空格）白底，其余特殊键用灰色底。 */
private val NORMAL_COLOR = Color.White
private val SPECIAL_COLOR = Color(0xFFCCCCCC)
private val PRESSED_COLOR = Color(0xFF175CD3)
private val NORMAL_BORDER = Color(0xFF8B95A5)
private val SPECIAL_BORDER = Color(0xFF6E6E6E)
private val LABEL_COLOR = Color(0xFF1F2A37)
private val ALT_LABEL_COLOR = Color(0xFF4B5563)

/**
 * 全键盘。激活后取代下方的快捷键区与语音区，占据屏幕下半部分。
 *
 * 布局按需求逐格排：
 * 1 行 副键 `1234567890` / 主键 `qwertyuiop`
 * 2 行 副键 `-/:;()~'"` / 主键 `asdfghjkl`
 * 3 行 副键 `[Shift]@-#&?!…[Backspace]` / 主键 `[Shift]zxcvbnm[Backspace]`
 * 4 行 `[alt]` · `.`（长按 `,`）· `空格` · `[Shift+Enter]` · `[Enter]`
 *
 * 触发方式：
 * - 双键位键（字母 / 数字 / 符号 / `.`）：短按只发主键位一次，长按只发副键位一次。
 * - Shift：短按单次大写，长按锁定。
 * - 退格 / Alt / 回车 / Shift+Enter：短按一次，长按真按住、由 Windows 连续触发。
 * - 空格：短按一次空格，长按＝长按语音输入（同时开始传音并保持 PC 长按热键，松手结束）。
 *
 * 宽度按屏幕宽度的百分比固定：常规键与 `.` 8.5%、间隙 1.5%、左右各留 0.75%。
 * 1 / 3 / 4 行的总宽正好铺满（3 行的 `[Shift]`、`[Backspace]` 各 13.5%，4 行的 `[alt]`、
 * `[Enter]` 各 18.5%、`[Shift+Enter]` 13.5%、空格 33.5%）；2 行只有 9 键，整行居中。
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
            Key(KeyHold.SHIFT, mainLabel = "⇧", kind = Kind.Shift, special = true, widthPercent = 13.5f),
            dual("LeftShift+2|Z"),
            dual("Minus|X"),
            dual("LeftShift+3|C"),
            dual("LeftShift+7|V"),
            dual("LeftShift+Slash|B"),
            dual("LeftShift+1|N"),
            dual("Ellipsis|M"),
            Key("Backspace", mainLabel = "⌫", kind = Kind.Hold, special = true, widthPercent = 13.5f),
        ),
        // 4 行：Alt + 「.（长按 ,）」+ 空格（长按语音输入）+ Shift+Enter + 回车
        // 空格 / 退格 / 回车 / Shift+Enter 长按都是真按住，由 Windows 连续触发；
        // 空格的长按改为长按语音输入。
        listOf(
            Key("LeftAlt", mainLabel = "Alt", kind = Kind.Hold, special = true, widthPercent = 18.5f),
            dual("Comma|Period"),
            Key("Space", mainLabel = "空格", altLabel = "🎤", kind = Kind.VoiceDual, widthPercent = 33.5f),
            Key("LeftShift+Enter", mainLabel = "⇧⏎", kind = Kind.Hold, special = true, widthPercent = 13.5f),
            Key("Enter", mainLabel = "⏎", kind = Kind.Hold, special = true, widthPercent = 18.5f),
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
                        // 铺满的行居中不受影响；只有 9 键的 2 行会因此整体居中。
                        horizontalArrangement = Arrangement.spacedBy(gap, Alignment.CenterHorizontally),
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
    // 手指按住时立刻点亮（短按的键值在抬手或长按判定后才发出）。
    var pressed by remember { mutableStateOf(false) }
    val active = pressed || when (item.kind) {
        Kind.Shift -> hold.shiftOn
        Kind.Hold -> hold.isHeld(item.primary)
        Kind.VoiceDual -> voiceActive
        Kind.Dual -> false
    }
    val description = when (item.kind) {
        Kind.Shift -> "Shift：短按单次大写，长按锁定大写"
        Kind.VoiceDual -> "空格：短按一次空格，长按等于长按语音输入（按住说话，松手结束）"
        Kind.Hold -> when (item.primary) {
            "Backspace" -> "退格：短按一次，长按连续退格"
            "LeftAlt" -> "Alt：短按一次，长按连续按住"
            "LeftShift+Enter" -> "Shift+Enter：短按一次，长按连续换行"
            else -> "回车：短按一次，长按连续回车"
        }
        Kind.Dual -> if (item.altLabel.isEmpty()) "${item.mainLabel} 键"
        else "${item.mainLabel} 键，长按输入 ${item.altLabel}"
    }
    Surface(
        modifier = Modifier.width(width).fillMaxHeight()
            .semantics { contentDescription = description }
            .keyGesture(item, connected, hold, scope, beginVoice, stopVoice) { pressed = it },
        shape = MaterialTheme.shapes.small,
        color = when {
            active -> PRESSED_COLOR
            item.special -> SPECIAL_COLOR
            else -> NORMAL_COLOR
        },
        contentColor = if (active) Color.White else LABEL_COLOR,
        border = BorderStroke(
            1.dp,
            when {
                active -> PRESSED_COLOR
                item.special -> SPECIAL_BORDER
                else -> NORMAL_BORDER
            },
        ),
    ) {
        Column(
            modifier = Modifier.fillMaxSize().padding(horizontal = 2.dp, vertical = 2.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            if (item.altLabel.isNotEmpty()) {
                Text(
                    text = item.altLabel,
                    fontSize = if (item.altLabel.length == 1) 14.sp else 12.sp,
                    lineHeight = 16.sp,
                    textAlign = TextAlign.Center,
                    color = if (active) Color.White.copy(alpha = 0.9f) else ALT_LABEL_COLOR,
                    maxLines = 1,
                    overflow = TextOverflow.Clip,
                )
            }
            if (item.mainLabel.isNotEmpty()) {
                Text(
                    text = item.mainLabel,
                    fontSize = when {
                        item.mainLabel.length == 1 -> 20.sp
                        item.mainLabel.length == 2 -> 17.sp
                        else -> 13.sp
                    },
                    lineHeight = 22.sp,
                    textAlign = TextAlign.Center,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

/** 一个键的按下 / 抬起手势；[onPressed] 用于立刻点亮键面。 */
private fun Modifier.keyGesture(
    item: Key,
    connected: Boolean,
    hold: KeyHold,
    scope: CoroutineScope,
    beginVoice: (String) -> Boolean,
    stopVoice: () -> Unit,
    onPressed: (Boolean) -> Unit,
): Modifier = pointerInput(connected, item.primary, item.secondary, item.kind) {
    if (!connected) return@pointerInput
    awaitEachGesture {
        // 父布局会先消费按下事件，所以这里不要求未消费。
        awaitFirstDown(requireUnconsumed = false)
        onPressed(true)
        when (item.kind) {
            // 双键位：判定长按后才发副键位，因此长按不会先冒出一个主键位；
            // 短按在抬手时发主键位一次，副键位也只发一次、不会连续触发。
            Kind.Dual -> {
                var long = false
                val timer = scope.launch {
                    delay(KeyHold.LONG_PRESS_MS)
                    long = true
                    hold.dualLong(item.secondary)
                }
                val up = waitForUpOrCancellation()
                timer.cancel()
                if (!long && up != null) hold.dualShort(item.primary)
            }
            // 空格：短按一次空格；长按开始传音并保持 PC 长按热键，松手结束。
            Kind.VoiceDual -> {
                var talking = false
                val timer = scope.launch {
                    delay(KeyHold.LONG_PRESS_MS)
                    talking = beginVoice(MicBallView.MODE_HOLD)
                }
                val up = waitForUpOrCancellation()
                timer.cancel()
                if (talking) stopVoice() else if (up != null) hold.dualShort(item.primary)
            }
            // 长按键：短按一次，长按真按住（由 Windows 连续触发）。
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
        }
        onPressed(false)
    }
}
