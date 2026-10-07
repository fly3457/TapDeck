package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class KeyFeedbackPolicyTest {
    private class Backend : FeedbackBackend {
        override var available = true
        override var systemEnabled = true
        var useLegacy = false
        var fail = false
        val calls = mutableListOf<KeyFeedback>()
        override fun play(kind: KeyFeedback, legacyFeedback: () -> Boolean): Boolean {
            if (fail) throw SecurityException("denied")
            calls += kind
            return if (useLegacy) legacyFeedback() else true
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
        assertTrue(backend.systemEnabled)
    }

    @Test fun systemDisabledAndMissingMotorNeverSendRequests() {
        val backend = Backend(); val controller = KeyFeedbackController({ true }, backend)
        backend.systemEnabled = false
        assertEquals(FeedbackResult.SystemDisabled, controller.availability())
        assertEquals(FeedbackResult.SystemDisabled, controller.perform(KeyFeedback.Press))
        backend.available = false
        assertEquals(FeedbackResult.NoVibrator, controller.availability())
        assertEquals(FeedbackResult.NoVibrator, controller.perform(KeyFeedback.LongPress))
        assertTrue(backend.calls.isEmpty())
    }

    @Test fun directEffectsAreRequestedOnceWithoutAnExtraViewPulse() {
        val backend = Backend(); val controller = KeyFeedbackController({ true }, backend)
        var viewCalls = 0
        for (kind in KeyFeedback.entries) {
            assertEquals(FeedbackResult.Requested, controller.perform(kind) { viewCalls++; true })
        }
        assertEquals(listOf(KeyFeedback.Press, KeyFeedback.LongPress), backend.calls)
        assertEquals(0, viewCalls)
    }

    @Test fun legacyResultIsPropagatedAndDoesNotRetryOrDuplicate() {
        val backend = Backend().apply { useLegacy = true }
        val controller = KeyFeedbackController({ true }, backend); var viewCalls = 0
        assertEquals(FeedbackResult.Failed, controller.perform(KeyFeedback.Press) { viewCalls++; false })
        assertEquals(FeedbackResult.Requested, controller.perform(KeyFeedback.LongPress) { viewCalls++; true })
        assertEquals(2, viewCalls)
        assertEquals(listOf(KeyFeedback.Press, KeyFeedback.LongPress), backend.calls)
    }

    @Test fun failedBackendDoesNotCrashInput() {
        val backend = Backend().apply { fail = true }
        val controller = KeyFeedbackController({ true }, backend)
        assertEquals(FeedbackResult.Failed, controller.perform(KeyFeedback.Press))
        assertTrue(backend.calls.isEmpty())
        val absentService = object : FeedbackBackend {
            override val available: Boolean get() = throw IllegalStateException("unavailable")
            override val systemEnabled = true
            override fun play(kind: KeyFeedback, legacyFeedback: () -> Boolean) = error("must not play")
        }
        assertEquals(FeedbackResult.Failed, KeyFeedbackController({ true }, absentService).availability())
    }
}
