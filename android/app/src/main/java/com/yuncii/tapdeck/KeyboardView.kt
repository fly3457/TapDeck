package com.yuncii.tapdeck

import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.layout.*
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.Layout
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.disabled
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.Constraints
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** 键的行为类型。 */
private enum class Kind {
    /** 字母 / 数字 / 符号 / `.`：短按主键位一次，长按副键位一次。 */
    Dual,

    /** Shift：短按单次大写，长按锁定大写。 */
    Shift,

    /** Ctrl / 退格 / 回车 / Shift+Enter：短按一次，长按真按住、连续触发。 */
    Hold,

    /** 空格：短按一次空格；长按＝长按语音输入（开始传音并保持 PC 长按热键，松手结束）。 */
    VoiceDual,
}

private enum class KeyIcon { Shift, Backspace, Enter, ShiftEnter }

/** 键盘上的一个键：主键位（短按）与副键位（长按）。 */
private data class Key(
    val primary: String,
    val secondary: String = "",
    val mainLabel: String = "",
    val altLabel: String = "",
    val icon: KeyIcon? = null,
    val kind: Kind = Kind.Dual,
    /** 特殊键（Shift / 退格 / Ctrl / Shift+Enter / Enter）用灰底，常规键白底。 */
    val special: Boolean = false,
)

/**
 * 全键盘。激活后取代下方的快捷键区与语音区，占据屏幕下半部分。
 *
 * 布局按需求逐格排：
 * 1 行 副键 `1234567890` / 主键 `qwertyuiop`
 * 2 行 副键 `-/:;()~'"` / 主键 `asdfghjkl`
 * 3 行 副键 `[Shift]@-#&?!…[Backspace]` / 主键 `[Shift]zxcvbnm[Backspace]`
 * 4 行 `[Ctrl]` · `.`（长按 `,`）· `空格` · `[Shift+Enter]` · `[Enter]`
 *
 * 触发方式：
 * - 双键位键（字母 / 数字 / 符号 / `.`）：短按只发主键位一次，长按只发副键位一次。
 * - Shift：短按单次大写，长按锁定。
 * - 退格 / Ctrl / 回车 / Shift+Enter：短按一次，长按真按住、由 Windows 连续触发。
 * - 空格：短按一次空格，长按＝长按语音输入（同时开始传音并保持 PC 长按热键，松手结束）。
 *
 * KeyboardGeometry 统一计算键面边界：常规键 8.9%、间隙与左右留白 1%。
 * 特殊键保留原网格跨度；第 2 行居中。上下留白复用原生父容器，不重复添加。
 */
@Composable
fun KeyboardView(
    connected: Boolean,
    voiceActive: Boolean,
    scale: Float = 1f,
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
            Key(KeyHold.SHIFT, mainLabel = "Shift", icon = KeyIcon.Shift, kind = Kind.Shift, special = true),
            dual("LeftShift+2|Z"),
            dual("Minus|X"),
            dual("LeftShift+3|C"),
            dual("LeftShift+7|V"),
            dual("LeftShift+Slash|B"),
            dual("LeftShift+1|N"),
            dual("Ellipsis|M"),
            Key("Backspace", mainLabel = "Backspace", icon = KeyIcon.Backspace, kind = Kind.Hold, special = true),
        ),
        // 4 行：Ctrl + 「.（长按 ,）」+ 空格（长按语音输入）+ Shift+Enter + 回车
        // 空格 / 退格 / 回车 / Shift+Enter 长按都是真按住，由 Windows 连续触发；
        // 空格的长按改为长按语音输入。
        listOf(
            Key("LeftCtrl", mainLabel = "Ctrl", kind = Kind.Hold, special = true),
            dual("Comma|Period"),
            Key("Space", mainLabel = "空格", kind = Kind.VoiceDual),
            Key("LeftShift+Enter", mainLabel = "Shift+Enter", icon = KeyIcon.ShiftEnter, kind = Kind.Hold, special = true),
            Key("Enter", mainLabel = "Enter", icon = KeyIcon.Enter, kind = Kind.Hold, special = true),
        ),
    )

    Surface(modifier = Modifier.fillMaxSize(), color = Color(ControllerStyle.PANEL)) {
        BoxWithConstraints(Modifier.fillMaxSize()) {
            val available = maxWidth
            Layout(modifier = Modifier.fillMaxSize(), content = {
                rows.flatten().forEach { item ->
                    KeyboardKeyCell(
                        item = item, viewportWidth = available, scale = scale,
                        connected = connected, voiceActive = voiceActive, hold = hold,
                        beginVoice = beginVoice, stopVoice = stopVoice,
                    )
                }
            }) { measurables, constraints ->
                val cells = KeyboardGeometry.measure(constraints.maxWidth, constraints.maxHeight, scale).flatten()
                check(measurables.size == cells.size) { "Keyboard keys and geometry must match" }
                val placeables = measurables.mapIndexed { index, measurable ->
                    val cell = cells[index]
                    measurable.measure(Constraints.fixed(cell.width, cell.height))
                }
                layout(constraints.maxWidth, constraints.maxHeight) {
                    placeables.forEachIndexed { index, placeable ->
                        placeable.placeRelative(cells[index].left, cells[index].top)
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
    "Enter" -> "Enter"
    "Backspace" -> "Backspace"
    "LeftCtrl" -> "Ctrl"
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
    "LeftShift+Enter" -> "Shift+Enter"
    "Ellipsis" -> "…"
    else -> name
}

@Composable
private fun KeyboardKeyCell(
    item: Key,
    viewportWidth: Dp,
    scale: Float,
    connected: Boolean,
    voiceActive: Boolean,
    hold: KeyHold,
    beginVoice: (String) -> Boolean,
    stopVoice: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    val feedback by rememberUpdatedState(LocalKeyFeedback.current)
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
            "LeftCtrl" -> "Ctrl：短按一次，长按连续按住，可配合其他键"
            "LeftShift+Enter" -> "Shift+Enter：短按一次，长按连续换行"
            else -> "回车：短按一次，长按连续回车"
        }
        Kind.Dual -> if (item.altLabel.isEmpty()) "${item.mainLabel} 键"
        else "${item.mainLabel} 键，长按输入 ${item.altLabel}"
    }
    KeySurface(
        modifier = Modifier.fillMaxSize()
            .semantics(mergeDescendants = true) {
                contentDescription = description
                role = Role.Button
                if (!connected) disabled()
                onClick {
                    if (!connected) false else {
                        feedback(KeyFeedback.Press)
                        when (item.kind) {
                            Kind.Shift -> hold.tapShift()
                            Kind.Hold -> hold.tap(item.primary)
                            Kind.Dual, Kind.VoiceDual -> hold.dualShort(item.primary)
                        }
                        true
                    }
                }
            }
            .keyGesture(item, connected, hold, scope, beginVoice, stopVoice, { feedback(it) }) { pressed = it },
        viewportWidth = viewportWidth,
        active = active,
        special = item.special,
    ) {
        val unit = viewportWidth * scale
        val secondaryColor = if (active) Color.White.copy(alpha = 0.9f) else Color(ControllerStyle.SECONDARY)
        val mainSize = widthFont(viewportWidth, if (item.mainLabel.length == 1) 0.05f else 0.04f, scale)
        val altSize = widthFont(viewportWidth, 0.028f, scale)
        // Center the two hints as a group, with a small explicit gap. This keeps
        // secondary labels off the top edge and close to their primary labels.
        Column(
            Modifier.fillMaxSize().padding(horizontal = unit * 0.004f),
            verticalArrangement = Arrangement.spacedBy(unit * 0.002f, Alignment.CenterVertically),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            if (item.kind == Kind.VoiceDual) {
                Icon(
                    painter = painterResource(R.drawable.ic_lucide_mic_audio_lines),
                    contentDescription = null,
                    tint = secondaryColor,
                    modifier = Modifier.size(unit * 0.035f),
                )
            } else if (item.altLabel.isNotEmpty()) {
                Text(
                    text = item.altLabel,
                    fontSize = altSize,
                    lineHeight = altSize * 1.1f,
                    textAlign = TextAlign.Center,
                    color = secondaryColor,
                    maxLines = 1,
                    overflow = TextOverflow.Clip,
                )
            }
            if (item.icon != null) {
                val icons = when (item.icon) {
                    KeyIcon.Shift -> listOf(R.drawable.ic_lucide_arrow_big_up)
                    KeyIcon.Backspace -> listOf(R.drawable.ic_lucide_delete)
                    KeyIcon.Enter -> listOf(R.drawable.ic_lucide_corner_down_left)
                    KeyIcon.ShiftEnter -> listOf(R.drawable.ic_lucide_arrow_big_up, R.drawable.ic_lucide_corner_down_left)
                }
                Row(horizontalArrangement = Arrangement.spacedBy(unit * 0.0025f)) {
                    icons.forEach { resource ->
                        Icon(painterResource(resource), contentDescription = null,
                            tint = LocalContentColor.current, modifier = Modifier.size(unit * 0.05f))
                    }
                }
            } else if (item.mainLabel.isNotEmpty()) {
                Text(
                    text = item.mainLabel,
                    fontSize = mainSize,
                    lineHeight = mainSize * 1.1f,
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
    feedback: (KeyFeedback) -> Unit,
    onPressed: (Boolean) -> Unit,
): Modifier = pointerInput(connected, item.primary, item.secondary, item.kind) {
    if (!connected) return@pointerInput
    awaitEachGesture {
        // 父布局会先消费按下事件，所以这里不要求未消费。
        awaitFirstDown(requireUnconsumed = false)
        feedback(KeyFeedback.Press)
        onPressed(true)
        var timer: Job? = null
        var held = false
        var talking = false
        try {
            when (item.kind) {
                // 双键位：判定长按后才发副键位，因此长按不会先冒出一个主键位；
                // 短按在抬手时发主键位一次，副键位也只发一次、不会连续触发。
                Kind.Dual -> {
                    var long = false
                    timer = scope.launch {
                        delay(KeyHold.LONG_PRESS_MS)
                        long = true
                        hold.dualLong(item.secondary)
                        feedback(KeyFeedback.LongPress)
                    }
                    val up = waitForUpOrCancellation()
                    timer?.cancel()
                    if (!long && up != null) hold.dualShort(item.primary)
                }
                // 空格：短按一次空格；长按开始传音并保持 PC 长按热键，松手结束。
                Kind.VoiceDual -> {
                    timer = scope.launch {
                        delay(KeyHold.LONG_PRESS_MS)
                        talking = beginVoice(MicBallView.MODE_HOLD)
                        if (talking) feedback(KeyFeedback.LongPress)
                    }
                    val up = waitForUpOrCancellation()
                    timer?.cancel()
                    if (talking) { stopVoice(); talking = false } else if (up != null) hold.dualShort(item.primary)
                }
                // 长按键：短按一次，长按真按住（由 Windows 连续触发）。
                Kind.Hold -> {
                    var long = false
                    timer = scope.launch {
                        delay(KeyHold.LONG_PRESS_MS)
                        long = true
                        held = true
                        hold.holdDown(item.primary)
                        feedback(KeyFeedback.LongPress)
                    }
                    val up = waitForUpOrCancellation()
                    timer?.cancel()
                    if (long) { hold.holdUp(item.primary); held = false } else if (up != null) hold.tap(item.primary)
                }
                // Shift：短按单次大写，长按锁定大写。
                Kind.Shift -> {
                    var locked = false
                    timer = scope.launch {
                        delay(KeyHold.LONG_PRESS_MS)
                        locked = true
                        hold.lockShift()
                        feedback(KeyFeedback.LongPress)
                    }
                    val up = waitForUpOrCancellation()
                    timer?.cancel()
                    if (!locked && up != null) hold.tapShift()
                }
            }
        } finally {
            timer?.cancel()
            if (held) hold.holdUp(item.primary)
            if (talking) stopVoice()
            onPressed(false)
        }
    }
}
