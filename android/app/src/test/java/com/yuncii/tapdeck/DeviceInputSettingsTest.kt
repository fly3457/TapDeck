package com.yuncii.tapdeck

import java.io.IOException
import kotlinx.coroutines.*
import org.junit.Assert.*
import org.junit.Test

class DeviceInputSettingsTest {
    @Test fun baselineAndLimits() {
        val defaults = DeviceInputSettings()
        assertEquals(1.0, defaults.sensitivity, 0.0)
        assertEquals(2.0, defaults.pointerGain, 0.0)
        assertTrue(defaults.haptics)
        assertEquals(1.0, DeviceInputSettings(0.5).pointerGain, 0.0)
        assertEquals(6.0, DeviceInputSettings(3.0).pointerGain, 0.0)
        assertEquals(0.5, DeviceInputSettings.normalize(-100.0), 0.0)
        assertEquals(3.0, DeviceInputSettings.normalize(100.0), 0.0)
        for (invalid in listOf(Double.NaN, Double.NEGATIVE_INFINITY, Double.POSITIVE_INFINITY))
            assertEquals(1.0, DeviceInputSettings.normalize(invalid), 0.0)
        assertEquals(1.3, DeviceInputSettings.normalize(1.26), 0.0)
        assertEquals("1.0×", defaults.sensitivityLabel)
    }

    @Test fun editBeforeLoadKeepsOtherSavedPreferencesAndLastValue() = runBlocking {
        withTimeout(3000) {
            val loaded = CompletableDeferred<Unit>()
            val saved = mutableListOf<DeviceInputSettings>()
            val prefs = DeviceInputPreferences({ loaded.await(); DeviceInputSettings(2.2, false) }, { saved.add(it) })
            try {
                prefs.update { it.copy(sensitivity = 0.5) }
                prefs.update { it.copy(sensitivity = 3.0) }
                assertEquals(6.0, prefs.state.value.pointerGain, 0.0)
                loaded.complete(Unit)
                // Close flushes the final edit even before loading has finished.
                prefs.close(); prefs.awaitClosed()
                assertEquals(DeviceInputSettings(3.0, false), prefs.state.value)
                assertEquals(DeviceInputSettings(3.0, false), saved.last())
            } finally { loaded.complete(Unit); prefs.close() }
        }
    }

    @Test fun slowSaveNeverOverwritesNewerEditAndDevicesAreIndependent() = runBlocking {
        withTimeout(3000) {
            val started = CompletableDeferred<Unit>()
            val finish = CompletableDeferred<Unit>()
            val saved = mutableListOf<DeviceInputSettings>()
            val first = DeviceInputPreferences({ DeviceInputSettings() }, {
                started.complete(Unit); finish.await(); saved.add(it)
            })
            val other = DeviceInputPreferences({ DeviceInputSettings(0.7, false) }, {})
            try {
                first.update { it.copy(sensitivity = 0.5) }
                started.await()
                first.update { it.copy(sensitivity = 3.0) }
                first.update { it.copy(haptics = false) }
                assertEquals(DeviceInputSettings(3.0, false), first.state.value)
                finish.complete(Unit)
                first.close(); other.close(); first.awaitClosed(); other.awaitClosed()
                assertEquals(DeviceInputSettings(3.0, false), saved.last())
                assertEquals(DeviceInputSettings(0.7, false), other.state.value)
            } finally { finish.complete(Unit); first.close(); other.close() }
        }
    }

    @Test fun saveFailureDoesNotBreakFutureUpdates() = runBlocking {
        withTimeout(3000) {
            val failed = CompletableDeferred<Unit>()
            val saved = mutableListOf<DeviceInputSettings>()
            var fail = true
            val prefs = DeviceInputPreferences({ throw IOException("load") }, {
                if (fail) { fail = false; throw IOException("save") }; saved.add(it)
            }, { if (it.message == "save") failed.complete(Unit) })
            try {
                prefs.update { it.copy(sensitivity = 0.5) }; failed.await()
                prefs.update { it.copy(sensitivity = 2.0) }
                prefs.close(); prefs.awaitClosed()
                assertEquals(2.0, saved.last().sensitivity, 0.0)
            } finally { prefs.close() }
        }
    }
}
