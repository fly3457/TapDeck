package com.yuncii.tapdeck

import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf

/**
 * 全键盘的按键状态管理。
 *
 * - 双键位键（字母 / 数字 / 符号 / `.` / 空格）：**短按只发主键位一次、长按只发副键位一次**。
 *   判定由手势层计时完成（见 `KeyboardView.keyGesture`），因此长按时不会先冒出一个主键位，
 *   副键位也不会因为按住而在 PC 上连续触发。
 * - 长按键（Ctrl / 退格 / 回车 / Shift+Enter）：短按一次，长按真按住，由 Windows 连续触发。
 * - Shift：短按单次大写（用一次就复位），长按锁定大写，再次短按解除锁定。
 *
 * 状态存在 Compose 快照状态里，键面直接读它点亮。
 */
class KeyHold(
    private val down: (String) -> Unit,
    private val up: (String) -> Unit,
) {
    companion object {
        /** 长按判定：超过这个时间算长按。 */
        const val LONG_PRESS_MS = 400L
        /** Shift 键值。 */
        const val SHIFT = "LeftShift"
    }

    /** 真实长按中的键值（长按键）。 */
    private val held = mutableStateMapOf<String, Boolean>()

    private val shiftOnState = mutableStateOf(false)
    private val shiftLockedState = mutableStateOf(false)

    /** Shift 是否生效（单次或锁定）。 */
    val shiftOn: Boolean get() = shiftOnState.value

    /** Shift 是否处于锁定状态。 */
    val shiftLocked: Boolean get() = shiftLockedState.value

    /** 某个长按键当前是否被真实按住。 */
    fun isHeld(text: String): Boolean = held.containsKey(text)

    /** 组合键文本：附加修饰键 + 目标键。 */
    private fun chord(key: String, extra: String): String = when {
        key.isEmpty() -> extra
        extra.isEmpty() -> key
        else -> "$extra+$key"
    }

    // ---- Shift ----

    /** Shift 短按：点亮单次大写；已在生效（单次或锁定）时改为解除。 */
    fun tapShift() {
        if (shiftOnState.value) {
            clearShift()
        } else {
            shiftOnState.value = true
            down(SHIFT)
        }
    }

    /** Shift 长按：锁定大写，直到再次短按解除。 */
    fun lockShift() {
        if (!shiftOnState.value) down(SHIFT)
        shiftOnState.value = true
        shiftLockedState.value = true
    }

    private fun clearShift() {
        if (shiftOnState.value) up(SHIFT)
        shiftOnState.value = false
        shiftLockedState.value = false
    }

    /** 用掉一次单次 Shift：锁定状态保持不变。 */
    private fun useShift() {
        if (shiftOnState.value && !shiftLockedState.value) clearShift()
    }

    // ---- 双键位键 ----

    /** 双键位键短按：主键位一次；Shift 生效时带上 LeftShift。 */
    fun dualShort(primary: String) {
        if (primary.isEmpty()) return
        val text = if (shiftOnState.value) chord(primary, SHIFT) else primary
        down(text)
        up(text)
        useShift()
    }

    /** 双键位键长按：副键位一次，不重复、不附带额外的 Shift（副键位自带 Shift 组合）。 */
    fun dualLong(secondary: String) {
        if (secondary.isEmpty()) return
        down(secondary)
        up(secondary)
        useShift()
    }

    // ---- 长按键（Ctrl / 退格 / 回车 / Shift+Enter）----

    /** 短按：一次完整按键。 */
    fun tap(text: String) {
        if (text.isEmpty()) return
        down(text)
        up(text)
    }

    /** 长按：真的按住，Windows 会连续触发。 */
    fun holdDown(text: String) {
        if (text.isEmpty() || held.containsKey(text)) return
        held[text] = true
        down(text)
    }

    /** 长按结束。 */
    fun holdUp(text: String) {
        if (held.remove(text) != null) up(text)
    }

    /** 切换界面、退出全键盘或断开连接时释放所有按键与修饰键。 */
    fun releaseAll() {
        held.keys.toList().asReversed().forEach { up(it) }
        held.clear()
        if (shiftOnState.value) up(SHIFT)
        shiftOnState.value = false
        shiftLockedState.value = false
    }
}
