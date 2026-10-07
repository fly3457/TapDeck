package com.yuncii.tapdeck

import kotlinx.coroutines.currentCoroutineContext
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive

internal object ConnectionReminder {
    const val PERIOD_MS = 3000L
    const val LEG_MS = 180
    const val AMPLITUDE = 0.008f

    /** The caller's lifecycle-owned coroutine cancels both the wait and an active jump. */
    suspend fun run(animate: suspend (Float, Int) -> Unit) {
        delay(PERIOD_MS)
        while (currentCoroutineContext().isActive) {
            animate(-1f, LEG_MS)
            animate(0f, LEG_MS)
            delay(PERIOD_MS - LEG_MS * 2)
        }
    }
}
