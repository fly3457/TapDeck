package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class KeyFeedbackPolicyTest {
    private class Backend : FeedbackBackend {
        override var available = true
        var accepted = true
        var fail = false
        val calls = mutableListOf<KeyFeedback>()
        override fun play(kind: KeyFeedback): Boolean {
            if (fail) throw SecurityException("denied")
            calls += kind
            return accepted
        }
    }

    @Test fun appSwitchIsReadOnEveryPressAndDoesNotChangeSystemSettings() {
        val backend = Backend(); var enabled = true
        val controller = KeyFeedbackController({ enabled }, backend)
        assertEquals(FeedbackResult.Requested, controller.perform(KeyFeedback.Press))
        enabled = false
        assertEquals(FeedbackResult.AppDisabled, controller.perform(KeyFeedback.Press))
        enabled = true
        assertEquals(FeedbackResult.Requested, controller.perform(KeyFeedback.LongPress))
        assertEquals(listOf(KeyFeedback.Press, KeyFeedback.LongPress), backend.calls)
    }

    @Test fun missingMotorNeverSendsRequests() {
        val backend = Backend(); val controller = KeyFeedbackController({ true }, backend)
        backend.available = false
        assertEquals(FeedbackResult.NoVibrator, controller.availability())
        assertEquals(FeedbackResult.NoVibrator, controller.perform(KeyFeedback.LongPress))
        assertTrue(backend.calls.isEmpty())
    }

    @Test fun directEffectsAreRequestedOnceWithoutAnExtraViewPulse() {
        val backend = Backend(); val controller = KeyFeedbackController({ true }, backend)
        for (kind in KeyFeedback.entries) {
            assertEquals(FeedbackResult.Requested, controller.perform(kind))
        }
        assertEquals(listOf(KeyFeedback.Press, KeyFeedback.LongPress), backend.calls)
    }

    @Test fun rejectedRequestIsPropagatedWithoutRetrying() {
        val backend = Backend().apply { accepted = false }
        val controller = KeyFeedbackController({ true }, backend)
        assertEquals(FeedbackResult.Failed, controller.perform(KeyFeedback.Press))
        backend.accepted = true
        assertEquals(FeedbackResult.Requested, controller.perform(KeyFeedback.LongPress))
        assertEquals(listOf(KeyFeedback.Press, KeyFeedback.LongPress), backend.calls)
    }

    @Test fun failedBackendDoesNotCrashInput() {
        val backend = Backend().apply { fail = true }
        val controller = KeyFeedbackController({ true }, backend)
        assertEquals(FeedbackResult.Failed, controller.perform(KeyFeedback.Press))
        assertTrue(backend.calls.isEmpty())
        val absentService = object : FeedbackBackend {
            override val available: Boolean get() = throw IllegalStateException("unavailable")
            override fun play(kind: KeyFeedback) = error("must not play")
        }
        assertEquals(FeedbackResult.Failed, KeyFeedbackController({ true }, absentService).availability())
    }
}
