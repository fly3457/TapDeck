package com.yuncii.tapdeck

import kotlinx.serialization.Serializable

@Serializable data class VoiceProfile(
    val id: String,
    val name: String,
    val enabled: Boolean,
    val mode: String,
    val hold_key: String = "",
    val toggle_start_key: String = "",
    val toggle_stop_key: String = "",
) {
    fun valid() = name.trim().isNotEmpty() && voiceNameLength(name) <= 16 && mode in listOf("hold", "toggle")
}

fun voiceNameLength(name: String): Int = name.trim().codePoints().toArray().sumOf { if (it <= 127) 1 else 2 }

fun defaultVoiceProfiles() = listOf(
    VoiceProfile("voice-1", "单击语音输入", true, "toggle", toggle_start_key = "RightCtrl+L", toggle_stop_key = "RightCtrl+L"),
    VoiceProfile("voice-2", "长按语音输入", true, "hold", hold_key = "RightAlt"),
    VoiceProfile("voice-3", "GPT听写", false, "hold", hold_key = "Ctrl+Shift+M"),
)

fun PcConfig.voiceProfiles(supported: Boolean): List<VoiceProfile> = if (supported) {
    requireNotNull(voice.profiles) { "电脑缺少三组语音配置" }
} else listOf(
    VoiceProfile("voice-1", "单击语音输入", true, "toggle", toggle_start_key = voice.toggle_start_key, toggle_stop_key = voice.toggle_stop_key),
    VoiceProfile("voice-2", "长按语音输入", true, "hold", hold_key = voice.hold_key),
)

/** Stable IDs survive renames/type changes. Legacy mode is only used before the first ID selection. */
class VoiceSelection(var id: String? = null, private var legacyMode: String? = null) {
    fun reconcile(profiles: List<VoiceProfile>): VoiceProfile? {
        val enabled = profiles.filter { it.enabled }
        val selected = enabled.firstOrNull { it.id == id }
            ?: if (id == null) enabled.firstOrNull { it.mode == legacyMode } ?: enabled.firstOrNull() else enabled.firstOrNull()
        if (selected != null) { id = selected.id; legacyMode = null }
        return selected
    }
    fun next(profiles: List<VoiceProfile>): VoiceProfile? {
        val current = reconcile(profiles)
        val enabled = profiles.filter { it.enabled }
        if (enabled.size < 2) return current
        return enabled[(enabled.indexOfFirst { it.id == current?.id } + 1) % enabled.size].also { id = it.id }
    }
}
