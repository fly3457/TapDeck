package com.yuncii.tapdeck

import android.content.Context
import android.os.Build
import android.os.VibrationAttributes
import android.os.VibrationEffect
import android.os.Vibrator
import android.os.VibratorManager
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
    override fun play(kind: KeyFeedback): Boolean {
        val effect = if (Build.VERSION.SDK_INT >= 29) {
            VibrationEffect.createPredefined(if (kind == KeyFeedback.Press)
                VibrationEffect.EFFECT_CLICK else VibrationEffect.EFFECT_HEAVY_CLICK)
        } else VibrationEffect.createOneShot(if (kind == KeyFeedback.Press) 15L else 30L, VibrationEffect.DEFAULT_AMPLITUDE)
        // 模拟实体遥控器按键，由 App 开关决定是否请求；不读取或修改系统触感开关。
        // 系统总震动开关、硬件反馈强度及厂商策略仍可能限制实际马达输出。
        if (Build.VERSION.SDK_INT >= 33) vibrator!!.vibrate(effect,
            VibrationAttributes.createForUsage(VibrationAttributes.USAGE_PHYSICAL_EMULATION))
        else vibrator!!.vibrate(effect)
        return true // A request is not proof of a physical pulse.
    }
}

internal fun View.keyFeedback(kind: KeyFeedback, controller: KeyFeedbackController =
    KeyFeedbackController({ true }, AndroidFeedbackBackend(context))): FeedbackResult = controller.perform(kind)
