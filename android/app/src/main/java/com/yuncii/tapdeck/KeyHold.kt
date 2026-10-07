package com.yuncii.tapdeck

import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * 全键盘的按键状态管理。
 *
 * - 按下发 `key_down`、抬起发 `key_up`，所以「按住」等于 PC 上真的按住，连续触发交给 Windows。
 * - 双键位键（字母 / 数字 / 符号）：短按主键位，按住 [LONG_PRESS_MS] 后改按右上角副键位。
 * - 长按键（Alt / 退格 / 回车 / Shift+Enter）：短按单次触发，长按真按住、连续触发。
 * - Shift：短按单次大写（用一次就复位），长按锁定大写，再次短按解除锁定。
 *
 * 状态存在 Compose 快照状态里，键面直接读它点亮，不需要额外通知。
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

        /** 双键位键在状态表里的 id。 */
        fun dualId(primary: String, secondary: String) = "$primary|$secondary"
    }

    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    private class Pressed(val text: String, val secondary: String, val job: Job)

    /** 已按下的键 id -> 按下时实际发送的组合键文本。 */
    private val pressing = mutableStateMapOf<String, Pressed>()

    /** 长按已经切到副键位的双键位键 id。 */
    private val swapped = mutableStateMapOf<String, Boolean>()

    /** 真实长按中的键值（长按键）。 */
    private val held = mutableStateMapOf<String, Boolean>()

    private val shiftOnState = mutableStateOf(false)
    private val shiftLockedState = mutableStateOf(false)

    /** Shift 是否生效（单次或锁定）。 */
    val shiftOn: Boolean get() = shiftOnState.value

    /** Shift 是否处于锁定状态。 */
    val shiftLocked: Boolean get() = shiftLockedState.value

    /** 某个键当前是否按下。 */
    fun isPressed(id: String): Boolean = pressing.containsKey(id)

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

    // ---- 单次 / 真实长按 ----

    /** 单次按键：按下即抬起（长按键的短按）。 */
    fun tap(text: String) {
        if (text.isEmpty()) return
        down(text)
        up(text)
    }

    /** 长按键按下：真的按住，Windows 会连续触发。 */
    fun holdDown(text: String) {
        if (text.isEmpty() || held.containsKey(text)) return
        held[text] = true
        down(text)
    }

    /** 长按键抬起。 */
    fun holdUp(text: String) {
        if (held.remove(text) != null) up(text)
    }

    // ---- 双键位键 ----

    /** 双键位键按下：先按主键位，长按后换成副键位。 */
    fun pressDual(primary: String, secondary: String) {
        val id = dualId(primary, secondary)
        if (pressing.containsKey(id)) return
        val text = chord(primary, "")
        if (text.isEmpty()) return
        val alt = chord(secondary, "")
        down(text)
        val job = scope.launch {
            if (alt.isEmpty()) return@launch
            delay(LONG_PRESS_MS)
            // 期间已经抬手（或被取消）就不再切换。
            if (pressing[id]?.text != text) return@launch
            swapped[id] = true
            up(text)
            down(alt)
        }
        pressing[id] = Pressed(text, alt, job)
    }

    /** 双键位键抬起：按当前生效的键位释放一次。 */
    fun releaseDual(primary: String, secondary: String) {
        val id = dualId(primary, secondary)
        val pressed = pressing.remove(id) ?: return
        pressed.job.cancel()
        if (swapped.remove(id) == true && pressed.secondary.isNotEmpty()) {
            up(pressed.secondary)
        } else {
            up(pressed.text)
        }
    }

    // ---- Shift + 普通键 ----

    /** Shift 生效时按下字母 / 符号键：临时附带 LeftShift。 */
    fun pressWithShift(key: String) {
        if (key.isEmpty() || pressing.containsKey(key)) return
        val text = chord(key, SHIFT)
        down(text)
        pressing[key] = Pressed(text, "", Job())
    }

    /** 带 Shift 的键抬起：单次 Shift 用完即复位，锁定状态保持不变。 */
    fun releaseWithShift(key: String) {
        val pressed = pressing.remove(key) ?: return
        up(pressed.text)
        if (!shiftLockedState.value) clearShift()
    }

    /** 切换界面、退出全键盘或断开连接时释放所有按键与修饰键。 */
    fun releaseAll() {
        val releases = ArrayList<String>(pressing.size)
        for ((id, pressed) in pressing) {
            pressed.job.cancel()
            releases.add(if (swapped[id] == true && pressed.secondary.isNotEmpty()) pressed.secondary else pressed.text)
        }
        // 后按下的先抬起，避免修饰键残留。
        releases.asReversed().forEach { up(it) }
        pressing.clear()
        swapped.clear()
        held.keys.toList().asReversed().forEach { up(it) }
        held.clear()
        if (shiftOnState.value) up(SHIFT)
        shiftOnState.value = false
        shiftLockedState.value = false
    }
}
