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
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

/** 键盘上的一个键：显示标签 + 发送给 PC 的键名；[extra] 是随键一起按下的修饰键。 */
private class Key(
    val label: String,
    val name: String = "",
    val extra: String = "",
    val modifier: Boolean = false,
    val symbolsToggle: Boolean = false,
    val wide: Boolean = false,
)

/** 符号页每个键的上档输出（键名 -> 键名, 显示字符）。 */
private fun shifted(name: String): Pair<String, String>? = when (name) {
    "1" -> "1" to "!"
    "2" -> "2" to "@"
    "3" -> "3" to "#"
    "4" -> "4" to "$"
    "5" -> "5" to "%"
    "6" -> "6" to "^"
    "7" -> "7" to "&"
    "8" -> "8" to "*"
    "9" -> "9" to "("
    "0" -> "0" to ")"
    "Minus" -> "Minus" to "_"
    "Plus" -> "Plus" to "+"
    "LeftBracket" -> "LeftBracket" to "{"
    "RightBracket" -> "RightBracket" to "}"
    "Semicolon" -> "Semicolon" to ":"
    "Quote" -> "Quote" to "\""
    "Comma" -> "Comma" to "<"
    "Period" -> "Period" to ">"
    "Slash" -> "Slash" to "?"
    "Backslash" -> "Backslash" to "|"
    "Backquote" -> "Backquote" to "~"
    else -> null
}

private val SYMBOL_ROW_1 = listOf("1", "2", "3", "4", "5", "6", "7", "8", "9", "0")
private val SYMBOL_ROW_2 = listOf("Minus", "Plus", "LeftBracket", "RightBracket", "Semicolon", "Quote", "Comma", "Period", "Slash", "Backslash")
private val SYMBOL_ROW_3 = listOf("Backquote", "Home", "End", "PageUp", "PageDown", "Delete")
private val NAV_KEYS = listOf(Key("Esc", "Esc"), Key("Tab", "Tab"), Key("←", "Left"), Key("↑", "Up"), Key("↓", "Down"), Key("→", "Right"), Key("退格", "Backspace", wide = true))
private val MODIFIER_KEYS = listOf(Key("Ctrl", "LeftCtrl", modifier = true), Key("Alt", "LeftAlt", modifier = true), Key("Win", "LeftWin", modifier = true))
private val LETTER_ROWS = listOf(
    listOf("Q", "W", "E", "R", "T", "Y", "U", "I", "O", "P"),
    listOf("A", "S", "D", "F", "G", "H", "J", "K", "L"),
    listOf("Z", "X", "C", "V", "B", "N", "M"),
)

/**
 * 全键盘。激活后取代下方的快捷键区与语音区，占据屏幕下半部分：
 * - 修饰键（Ctrl / Alt / Win）是锁定式的：点一下按下、再点一下抬起。
 * - Shift 与「符号」同样是点击切换；Shift 生效时字母发送大写、符号发送上档字符。
 * - 普通键按下即按下、松手即抬起，按住会按系统节奏自动重复。
 */
@Composable
fun KeyboardView(connected: Boolean, hold: KeyHold) {
    var symbols by remember { mutableStateOf(false) }
    // Shift 是一次性修饰键，状态保存在 KeyHold 里（随下一个键抬起后自动复位）。
    val shift = hold.isLatched(KeyHold.SHIFT)

    fun display(name: String): String = when (name) {
        "Minus" -> "-"
        "Plus" -> "="
        "LeftBracket" -> "["
        "RightBracket" -> "]"
        "Semicolon" -> ";"
        "Quote" -> "'"
        "Comma" -> ","
        "Period" -> "."
        "Slash" -> "/"
        "Backslash" -> "\\"
        "Backquote" -> "`"
        "PageUp" -> "PgUp"
        "PageDown" -> "PgDn"
        "Delete" -> "Del"
        else -> name
    }

    /** 把按键名变成按键描述：Shift 生效时通过附加 LeftShift 输出大写 / 上档字符。 */
    fun key(name: String): Key {
        if (symbols) {
            val pair = shifted(name)
            if (pair != null && shift) return Key(pair.second, pair.first)
            return Key(display(name), name)
        }
        if (name.length == 1 && name[0] in 'A'..'Z') return Key(name, name)
        return Key(display(name), name)
    }

    val rows: List<List<Key>> = if (symbols) {
        listOf(
            MODIFIER_KEYS + NAV_KEYS,
            SYMBOL_ROW_1.map { key(it) },
            SYMBOL_ROW_2.map { key(it) },
            listOf(Key("Shift", KeyHold.SHIFT, modifier = true)) + SYMBOL_ROW_3.map { key(it) } +
                listOf(Key("符号", symbolsToggle = true), Key("回车", "Enter", wide = true), Key("Space", "Space", wide = true)),
        )
    } else {
        listOf(
            MODIFIER_KEYS + NAV_KEYS,
            LETTER_ROWS[0].map { key(it) },
            listOf(Key("Shift", KeyHold.SHIFT, modifier = true)) + LETTER_ROWS[1].map { key(it) },
            listOf(Key("符号", symbolsToggle = true)) + LETTER_ROWS[2].map { key(it) } +
                listOf(Key("PgUp", "PageUp"), Key("PgDn", "PageDown"), Key("回车", "Enter", wide = true), Key("Space", "Space", wide = true)),
        )
    }

    Surface(modifier = Modifier.fillMaxSize(), color = Color(0xFFF1F3F6)) {
        Column(Modifier.fillMaxSize().padding(4.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            rows.forEach { row ->
                Row(Modifier.weight(1f).fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(3.dp)) {
                    row.forEach { item ->
                        KeyboardKeyCell(
                            item = item,
                            connected = connected,
                            hold = hold,
                            onSymbols = { symbols = !symbols; hold.releaseAll() },
                        )
                    }
                    repeat((10 - row.size).coerceAtLeast(0)) { Spacer(Modifier.weight(1f)) }
                }
            }
        }
    }
}

@Composable
private fun RowScope.KeyboardKeyCell(
    item: Key,
    connected: Boolean,
    hold: KeyHold,
    onSymbols: () -> Unit,
) {
    val on = when {
        item.modifier -> hold.isLatched(item.name)
        else -> false
    }
    val description = when {
        item.name == KeyHold.SHIFT -> "Shift 键"
        item.symbolsToggle -> "切换字母 / 符号页"
        item.modifier -> "${item.label} 修饰键"
        else -> "${item.label} 键"
    }
    Surface(
        modifier = Modifier.weight(if (item.wide) 2f else 1f).fillMaxHeight()
            .semantics { contentDescription = description }
            .keyGesture(connected, item, hold, onSymbols),
        shape = MaterialTheme.shapes.small,
        color = if (on) Color(0xFF175CD3) else Color.White,
        contentColor = if (on) Color.White else Color(0xFF1F2A37),
        border = BorderStroke(1.dp, if (on) Color(0xFF175CD3) else Color(0xFFCBD5E1)),
    ) {
        Box(Modifier.fillMaxSize().padding(horizontal = 1.dp), contentAlignment = Alignment.Center) {
            Text(
                text = item.label,
                fontSize = if (item.label.length <= 1) 17.sp else 13.sp,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

/** 一个键的按下 / 抬起手势；「符号」与修饰键按点击切换处理。 */
private fun Modifier.keyGesture(
    connected: Boolean,
    key: Key,
    hold: KeyHold,
    onSymbols: () -> Unit,
): Modifier = pointerInput(connected, key.name, key.label, key.extra) {
    awaitEachGesture {
        awaitFirstDown(requireUnconsumed = false)
        if (!connected) return@awaitEachGesture
        when {
            key.symbolsToggle -> {
                onSymbols()
                waitForUpOrCancellation()
            }
            key.modifier -> {
                hold.toggleModifier(key.name)
                waitForUpOrCancellation()
            }
            hold.isLatched(KeyHold.SHIFT) -> {
                hold.pressShift(key.name)
                waitForUpOrCancellation()
                hold.release(key.name)
            }
            else -> {
                hold.press(key.name, key.extra)
                waitForUpOrCancellation()
                hold.release(key.name)
            }
        }
    }
}
