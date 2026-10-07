package com.yuncii.tapdeck

import android.content.Context
import android.media.AudioAttributes
import android.os.Build
import android.os.VibrationAttributes
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
import android.provider.Settings
import android.view.HapticFeedbackConstants
import android.view.View
import androidx.compose.runtime.staticCompositionLocalOf

internal val LocalKeyFeedback = staticCompositionLocalOf<(KeyFeedback) -> Unit> { {} }

/** Uses device-tuned effects directly; it does not depend on the Compose host View. */
internal class AndroidFeedbackBackend(private val context: Context) : FeedbackBackend {
    @Suppress("DEPRECATION")
    private val vibrator: Vibrator? = if (Build.VERSION.SDK_INT >= 31)
        context.getSystemService(VibratorManager::class.java)?.defaultVibrator
    else context.getSystemService(Vibrator::class.java)
    override val available: Boolean get() = vibrator?.hasVibrator() == true
    override val systemEnabled: Boolean get() =
        Settings.System.getInt(context.contentResolver, Settings.System.HAPTIC_FEEDBACK_ENABLED, 1) != 0 &&
            Settings.System.getInt(context.contentResolver, "haptic_feedback_intensity", -1) != 0

    override fun play(kind: KeyFeedback, legacyFeedback: () -> Boolean): Boolean {
        if (Build.VERSION.SDK_INT >= 29) {
            val effect = VibrationEffect.createPredefined(if (kind == KeyFeedback.Press)
                VibrationEffect.EFFECT_CLICK else VibrationEffect.EFFECT_HEAVY_CLICK)
            if (Build.VERSION.SDK_INT >= 33) vibrator!!.vibrate(effect,
                VibrationAttributes.createForUsage(VibrationAttributes.USAGE_TOUCH))
            else vibrator!!.vibrate(effect, AudioAttributes.Builder()
                .setUsage(AudioAttributes.USAGE_ASSISTANCE_SONIFICATION)
                .setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION).build())
            return true // The system accepted a request; this is not proof of a physical pulse.
        }
        return legacyFeedback()
    }
}

internal fun View.keyFeedback(kind: KeyFeedback, controller: KeyFeedbackController =
    KeyFeedbackController({ true }, AndroidFeedbackBackend(context))): FeedbackResult = controller.perform(kind) {
    isHapticFeedbackEnabled = true
    performHapticFeedback(if (kind == KeyFeedback.Press) HapticFeedbackConstants.VIRTUAL_KEY else HapticFeedbackConstants.LONG_PRESS)
}
