package com.yuncii.tapdeck

import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive

internal object ConnectionReminder {
    const val PERIOD_MS = 3000L
    const val AMPLITUDE = 0.012f
    private val bounces = listOf(1f to 220, 0.5f to 155, 0.25f to 110)
    private val animationDuration = bounces.sumOf { it.second * 2 }

    /** The caller's lifecycle-owned coroutine cancels both the wait and an active jump. */
    suspend fun run(animate: suspend (Float, Int) -> Unit) {
        delay(PERIOD_MS)
        while (currentCoroutineContext().isActive) {
            // Each rebound loses height; flight time shrinks with its square root.
            for ((height, duration) in bounces) {
                animate(-height, duration)
                animate(0f, duration)
            }
            delay(PERIOD_MS - animationDuration)
        }
    }
}
