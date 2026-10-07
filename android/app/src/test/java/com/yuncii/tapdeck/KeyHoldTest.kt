package com.yuncii.tapdeck

import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * 全键盘按键状态机的单元测试：断言发到 PC 的组合键文本序列。
 *
 * 双键位键在这里只覆盖「发什么」：短按主键位一次、长按副键位一次；
 * 「什么时候发」（长按判定）由 `KeyboardView.keyGesture` 的计时器决定。
 */
class KeyHoldTest {
    private val events = mutableListOf<String>()
    private val hold = KeyHold({ events += "down $it" }, { events += "up $it" })

    @Test
    fun dualShortSendsPrimaryOnce() {
        hold.dualShort("A")
        assertEquals(listOf("down A", "up A"), events)
    }

    @Test
    fun dualLongSendsSecondaryOnce() {
        hold.dualLong("Minus")
        assertEquals(listOf("down Minus", "up Minus"), events)
    }

    @Test
    fun dualLongKeepsItsShiftChord() {
        hold.dualLong("LeftShift+2")
        assertEquals(listOf("down LeftShift+2", "up LeftShift+2"), events)
    }

    @Test
    fun shiftTapIsOneShot() {
        hold.tapShift()
        assertEquals(true, hold.shiftOn)
        hold.dualShort("A")
        assertEquals(listOf("down LeftShift", "down LeftShift+A", "up LeftShift+A", "up LeftShift"), events)
        assertEquals(false, hold.shiftOn)
    }

    @Test
    fun shiftLongPressLocksUntilTappedAgain() {
        hold.lockShift()
        hold.dualShort("A")
        hold.dualShort("B")
        assertEquals(true, hold.shiftLocked)
        // 锁定后连续两个字母都带 Shift，且 Shift 不会被抬起。
        assertEquals(
            listOf("down LeftShift", "down LeftShift+A", "up LeftShift+A", "down LeftShift+B", "up LeftShift+B"),
            events,
        )
        hold.tapShift()
        assertEquals("up LeftShift", events.last())
        assertEquals(false, hold.shiftOn)
    }

    @Test
    fun holdKeyTapsOnce() {
        hold.tap("Backspace")
        assertEquals(listOf("down Backspace", "up Backspace"), events)
    }

    @Test
    fun holdKeyStaysDownWhilePressed() {
        hold.holdDown("LeftAlt")
        assertEquals(listOf("down LeftAlt"), events)
        hold.holdUp("LeftAlt")
        assertEquals(listOf("down LeftAlt", "up LeftAlt"), events)
    }

    @Test
    fun releaseAllReleasesEverything() {
        hold.tapShift()
        hold.holdDown("Enter")
        hold.releaseAll()
        assertEquals(listOf("down LeftShift", "down Enter", "up Enter", "up LeftShift"), events)
    }
}
