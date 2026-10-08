package com.yuncii.tapdeck

enum class KeyFeedback { Press, LongPress }

internal enum class FeedbackResult { Requested, AppDisabled, NoVibrator, Failed }

internal interface FeedbackBackend {
    val available: Boolean
    fun play(kind: KeyFeedback): Boolean
}

/** All input regions and the test button use the same gate and backend. */
internal class KeyFeedbackController(private val enabled: () -> Boolean, private val backend: FeedbackBackend) {
    fun perform(kind: KeyFeedback): FeedbackResult {
        if (!enabled()) return FeedbackResult.AppDisabled
        return try {
            when {
                !backend.available -> FeedbackResult.NoVibrator
                backend.play(kind) -> FeedbackResult.Requested
                else -> FeedbackResult.Failed
            }
        } catch (_: Exception) { FeedbackResult.Failed }
    }

    fun availability(): FeedbackResult = try {
        when {
            !backend.available -> FeedbackResult.NoVibrator
            else -> FeedbackResult.Requested
        }
    } catch (_: Exception) { FeedbackResult.Failed }
}
