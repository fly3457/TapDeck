package com.yuncii.tapdeck

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.*
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class ConnectionReminderTest {
    @Test fun startsEveryThreeSecondsAndReturnsToBaseline() = runTest {
        val events = mutableListOf<Pair<Long, Float>>()
        val job = launch {
            ConnectionReminder.run { target, duration ->
                events += testScheduler.currentTime to target
                delay(duration.toLong())
            }
        }
        advanceTimeBy(2999)
        assertTrue(events.isEmpty())
        advanceTimeBy(3721)
        runCurrent()
        assertEquals(listOf(3000L to -1f, 3180L to 0f, 6000L to -1f, 6180L to 0f), events)
        job.cancelAndJoin()
    }

    @Test fun cancellationStopsWaitingOrAnActiveJumpAndResumeWaitsForNewPeriod() = runTest {
        val events = mutableListOf<Long>()
        suspend fun animate(target: Float, duration: Int) {
            events += testScheduler.currentTime
            delay(duration.toLong())
        }
        val waiting = launch { ConnectionReminder.run(::animate) }
        advanceTimeBy(1000)
        waiting.cancelAndJoin()
        advanceTimeBy(5000)
        assertTrue(events.isEmpty())
        val jumping = launch { ConnectionReminder.run(::animate) }
        advanceTimeBy(3050)
        jumping.cancelAndJoin()
        assertEquals(listOf(9000L), events)
        advanceTimeBy(5000)
        assertEquals(1, events.size)
    }
}
