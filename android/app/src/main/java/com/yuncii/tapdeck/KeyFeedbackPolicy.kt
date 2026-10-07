package com.yuncii.tapdeck

enum class KeyFeedback { Press, LongPress }

internal enum class FeedbackResult { Requested, AppDisabled, SystemDisabled, NoVibrator, Failed }

internal interface FeedbackBackend {
    val available: Boolean
    val systemEnabled: Boolean
    fun play(kind: KeyFeedback, legacyFeedback: () -> Boolean): Boolean
}

/** All input regions and the test button use the same gate and backend. */
internal class KeyFeedbackController(private val enabled: () -> Boolean, private val backend: FeedbackBackend) {
    fun perform(kind: KeyFeedback, legacyFeedback: () -> Boolean = { false }): FeedbackResult {
        if (!enabled()) return FeedbackResult.AppDisabled
        return try {
            when {
                !backend.available -> FeedbackResult.NoVibrator
                !backend.systemEnabled -> FeedbackResult.SystemDisabled
                backend.play(kind, legacyFeedback) -> FeedbackResult.Requested
                else -> FeedbackResult.Failed
            }
        } catch (_: Exception) { FeedbackResult.Failed }
    }

    fun availability(): FeedbackResult = try {
        when {
            !backend.available -> FeedbackResult.NoVibrator
            !backend.systemEnabled -> FeedbackResult.SystemDisabled
            else -> FeedbackResult.Requested
        }
    } catch (_: Exception) { FeedbackResult.Failed }
}
