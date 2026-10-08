package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class VoiceProfilesTest {
    @Test fun defaultsAndUnicodeNameLimits() {
        val p = defaultVoiceProfiles()
        assertEquals(listOf("voice-1", "voice-2", "voice-3"), p.map { it.id })
        assertEquals(listOf(true, true, false), p.map { it.enabled })
        assertEquals("RightCtrl+L", p[0].toggle_start_key)
        assertEquals("RightCtrl+L", p[0].toggle_stop_key)
        assertEquals("RightAlt", p[1].hold_key)
        assertEquals("Ctrl+Shift+M", p[2].hold_key)
        for (name in listOf("中文English12345", "中".repeat(8), "a".repeat(16), "😀".repeat(8))) {
            assertEquals(16, voiceNameLength(" $name "))
            assertTrue(p[0].copy(name = name).valid())
            assertFalse(p[0].copy(name = name + "a").valid())
        }
        assertFalse(p[0].copy(name = "  ").valid())
    }

    @Test fun selectionRemembersIdAcrossRenameTypeChangeAndDisabledGroups() {
        val all = defaultVoiceProfiles().map { it.copy(enabled = true) }
        val selection = VoiceSelection()
        assertEquals("voice-1", selection.reconcile(all)?.id)
        assertEquals("voice-2", selection.next(all)?.id)
        assertEquals("voice-3", selection.next(all)?.id)
        assertEquals("voice-1", selection.next(all)?.id)
        val restored = VoiceSelection("voice-3")
        val renamed = all.map { it.copy(name = "新名字", mode = "toggle") }
        assertEquals("voice-3", restored.reconcile(renamed)?.id)
        assertEquals("新名字", restored.reconcile(renamed)?.name)
        assertEquals("voice-1", restored.reconcile(all.map { it.copy(enabled = it.id != "voice-3") })?.id)
        val single = all.map { it.copy(enabled = it.id == "voice-2") }
        assertEquals("voice-2", restored.reconcile(single)?.id)
        assertEquals("voice-2", restored.next(single)?.id)
        assertNull(restored.reconcile(all.map { it.copy(enabled = false) }))
        assertEquals("voice-2", restored.reconcile(all)?.id)
    }

    @Test fun oldPreferenceAndOldPcCompatibility() {
        val config = wireJson.decodeFromString<PcConfig>("""{"voice":{"hold_key":"F8","toggle_start_key":"F9","toggle_stop_key":"F10"}}""").validate()
        val legacy = config.voiceProfiles(false)
        assertEquals(2, legacy.size)
        assertEquals("F8", legacy[1].hold_key)
        assertEquals("F10", legacy[0].toggle_stop_key)
        assertEquals("voice-2", VoiceSelection(legacyMode = "hold").reconcile(legacy)?.id)
        assertEquals("voice-1", VoiceSelection(legacyMode = "toggle").reconcile(legacy)?.id)
        assertEquals("voice-1", VoiceSelection().reconcile(legacy)?.id)
        assertThrows(IllegalArgumentException::class.java) { config.voiceProfiles(true) }
        assertThrows(IllegalArgumentException::class.java) { config.copy(voice = config.voice.copy(profiles = emptyList())).validate() }
    }
}
