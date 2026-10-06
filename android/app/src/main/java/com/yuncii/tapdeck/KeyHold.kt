package com.yuncii.tapdeck

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/**
 * 全键盘的按键状态管理：
 * - 按下即发送 key_down、抬起发送 key_up，因此按住可以保持按下（Windows 会重复）。
 * - 修饰键（Shift / Ctrl / Alt / Win）是锁定式的：点一下按下，再点一下抬起。
 * - 其它键按住不放时按系统节奏自动重复，抬起时用**按下时用过的同一段文本**释放，
 *   避免中途切换修饰键导致按键残留。
 */
class KeyHold(
    private val down: (String) -> Unit,
    private val up: (String) -> Unit,
) {
    companion object {
        /** 按住多久开始自动重复，以及重复间隔。 */
        const val REPEAT_DELAY_MS = 450L
        const val REPEAT_INTERVAL_MS = 60L
        /** 一次性修饰键：Shift。 */
        const val SHIFT = "LeftShift"
    }
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    /** 已锁定的修饰键，按按下顺序保留，发送时拼在普通键之前。 */
    private val latched = LinkedHashSet<String>()
    /** 当前被手指按住的普通键 -> 按下时实际发送的组合键文本与重复任务。 */
    private val pressing = mutableMapOf<String, Pressed>()

    private class Pressed(val text: String, val job: Job, val extra: String = "")

    fun isLatched(modifier: String): Boolean = latched.contains(modifier)

    /** 组合键文本：已锁定修饰键 + 本次附加修饰键 + 目标键。 */
    private fun chord(key: String, extra: String): String {
        val parts = ArrayList<String>(latched.size + 2)
        parts.addAll(latched)
        if (extra.isNotEmpty() && !latched.contains(extra)) parts.add(extra)
        if (key.isNotEmpty() && !parts.contains(key)) parts.add(key)
        return parts.joinToString("+")
    }

    /** 修饰键：切换锁定状态，锁定时立刻按下，解锁时抬起。 */
    fun toggleModifier(modifier: String): Boolean {
        if (!latched.add(modifier)) {
            latched.remove(modifier)
            up(modifier)
            return false
        }
        down(modifier)
        return true
    }

    /** 普通键按下（可带附加修饰键），并在按住后自动重复。 */
    fun press(key: String, extra: String = "") {
        if (pressing.containsKey(key)) return
        val text = chord(key, extra)
        if (text.isEmpty()) return
        down(text)
        val job = scope.launch {
            delay(REPEAT_DELAY_MS)
            while (isActive) {
                // 重复只发送按下；释放统一走 release，避免残留按下状态。
                val repeat = chord(key, extra)
                if (repeat.isEmpty()) return@launch
                down(repeat)
                delay(REPEAT_INTERVAL_MS)
            }
        }
        pressing[key] = Pressed(text, job, extra)
    }

    /** 普通键抬起。 */
    fun release(key: String) {
        val pressed = pressing.remove(key) ?: return
        pressed.job.cancel()
        up(pressed.text)
        // Shift 是「一次性」修饰键：随下一个键抬起后自动复位，与实体键盘一致。
        if (pressed.extra == SHIFT) {
            latched.remove(SHIFT)
            up(SHIFT)
        }
    }

    /** Shift 生效时按下普通键：临时附加 LeftShift。 */
    fun pressShift(key: String) {
        if (pressing.containsKey(key)) return
        val text = chord(key, SHIFT)
        if (text.isEmpty()) return
        down(text)
        val job = scope.launch {
            delay(REPEAT_DELAY_MS)
            while (isActive) {
                // Shift 在首个键抬起后已复位，后续重复按普通键发送。
                val repeat = chord(key, if (latched.contains(SHIFT)) SHIFT else "")
                if (repeat.isEmpty()) return@launch
                down(repeat)
                delay(REPEAT_INTERVAL_MS)
            }
        }
        pressing[key] = Pressed(text, job, SHIFT)
    }

    /** 断开连接、切换界面或退出全键盘时释放所有按键状态。 */
    fun releaseAll() {
        pressing.values.forEach { it.job.cancel() }
        pressing.values.toList().asReversed().forEach { up(it.text) }
        pressing.clear()
        latched.toList().asReversed().forEach { up(it) }
        latched.clear()
    }
}
