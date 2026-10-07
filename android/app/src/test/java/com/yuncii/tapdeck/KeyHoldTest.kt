package com.yuncii.tapdeck

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Test

/**
 * 全键盘按键状态机的单元测试：断言发到 PC 的组合键文本序列。
 *
 * 覆盖双键位键的短按 / 长按、Shift 单次与锁定、长按键的单次与真实按住。
 */
@OptIn(ExperimentalCoroutinesApi::class)
class KeyHoldTest {
    private val events = mutableListOf<String>()
    private lateinit var hold: KeyHold
    private val scheduler = StandardTestDispatcher()

    @Before
    fun setUp() {
        Dispatchers.setMain(scheduler)
        hold = KeyHold({ events += "down $it" }, { events += "up $it" })
    }

    @After
    fun tearDown() {
        Dispatchers.resetMain()
    }

    @Test
    fun dualTapSendsPrimary() = runTest(scheduler) {
        hold.pressDual("A", "Minus")
        hold.releaseDual("A", "Minus")
        assertEquals(listOf("down A", "up A"), events)
    }

    @Test
    fun dualLongPressSwitchesToSecondary() = runTest(scheduler) {
        hold.pressDual("A", "Minus")
        advanceTimeBy(KeyHold.LONG_PRESS_MS + 1)
        hold.releaseDual("A", "Minus")
        assertEquals(listOf("down A", "up A", "down Minus", "up Minus"), events)
    }

    @Test
    fun symbolSecondaryKeepsItsShiftChord() = runTest(scheduler) {
        hold.pressDual("D", "LeftShift+Semicolon")
        advanceTimeBy(KeyHold.LONG_PRESS_MS + 1)
        hold.releaseDual("D", "LeftShift+Semicolon")
        assertEquals(listOf("down D", "up D", "down LeftShift+Semicolon", "up LeftShift+Semicolon"), events)
    }

    @Test
    fun holdKeyTapsOnce() = runTest(scheduler) {
        hold.tap("Backspace")
        assertEquals(listOf("down Backspace", "up Backspace"), events)
    }

    @Test
    fun holdKeyStaysDownWhilePressed() = runTest(scheduler) {
        hold.holdDown("LeftAlt")
        advanceTimeBy(5_000)
        assertEquals(listOf("down LeftAlt"), events)
        hold.holdUp("LeftAlt")
        assertEquals(listOf("down LeftAlt", "up LeftAlt"), events)
    }

    @Test
    fun shiftTapIsOneShot() = runTest(scheduler) {
        hold.tapShift()
        assertEquals(true, hold.shiftOn)
        hold.pressWithShift("A")
        hold.releaseWithShift("A")
        assertEquals(listOf("down LeftShift", "down LeftShift+A", "up LeftShift+A", "up LeftShift"), events)
        assertEquals(false, hold.shiftOn)
    }

    @Test
    fun shiftLongPressLocksUntilTappedAgain() = runTest(scheduler) {
        hold.lockShift()
        hold.pressWithShift("A")
        hold.releaseWithShift("A")
        assertEquals(true, hold.shiftLocked)
        // 锁定后连续两个字母都带 Shift，且 Shift 不会被抬起。
        hold.pressWithShift("B")
        hold.releaseWithShift("B")
        assertEquals(
            listOf("down LeftShift", "down LeftShift+A", "up LeftShift+A", "down LeftShift+B", "up LeftShift+B"),
            events,
        )
        hold.tapShift()
        assertEquals(listOf("up LeftShift"), events.takeLast(1))
        assertEquals(false, hold.shiftOn)
    }

    @Test
    fun releaseAllReleasesEverything() = runTest(scheduler) {
        hold.tapShift()
        hold.holdDown("Enter")
        hold.pressDual("K", "Quote")
        hold.releaseAll()
        assertEquals(listOf("down LeftShift", "down Enter", "down K", "up K", "up Enter", "up LeftShift"), events)
    }
}
