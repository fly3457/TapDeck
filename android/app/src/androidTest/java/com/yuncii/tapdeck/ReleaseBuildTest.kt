package com.yuncii.tapdeck

import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.os.Build
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.security.MessageDigest
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.*
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Runs against the delivered APK, including across a debug -> release update. */
@RunWith(AndroidJUnit4::class)
class ReleaseBuildTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val context = instrumentation.targetContext
    private val store = PairStore(context)
    private val snapshot = File(context.filesDir, "release-upgrade-check.json")
    private val first = Peer("127.0.0.1", 42443, 42080, "ab".repeat(32), "升级电脑 A", "upgrade-a")
    private val second = first.copy(pin = "cd".repeat(32), httpPort = 43080, name = "升级电脑 B", token = "upgrade-b")
    private val expectedCatalog = PeerCatalog().upsert(first).upsert(second)
        .rename(second.id, "升级备注").voice(first.id, "voice-1").voice(second.id, "voice-2").select(second.id)

    @Suppress("DEPRECATION")
    private fun certificate(): String {
        val signatures = if (Build.VERSION.SDK_INT >= 28) context.packageManager
            .getPackageInfo(context.packageName, PackageManager.GET_SIGNING_CERTIFICATES).signingInfo!!.apkContentsSigners
        else context.packageManager.getPackageInfo(context.packageName, PackageManager.GET_SIGNATURES).signatures!!
        assertEquals(1, signatures.size)
        return MessageDigest.getInstance("SHA-256").digest(signatures.single().toByteArray())
            .joinToString("") { "%02x".format(it.toInt() and 255) }
    }

    private fun requireEmulator() {
        val emulator = instrumentation.uiAutomation.executeShellCommand("getprop ro.kernel.qemu").use {
            android.os.ParcelFileDescriptor.AutoCloseInputStream(it).readBytes().toString(Charsets.UTF_8).trim()
        }
        assumeTrue("isolated emulator required", emulator == "1")
    }

    @Test fun releaseBuildDisablesDebugging() {
        assertEquals("com.yuncii.tapdeck", context.packageName)
        assertEquals("release", BuildConfig.BUILD_TYPE)
        assertFalse(BuildConfig.DEBUG)
        assertEquals(0, context.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE)
        assertEquals(0, context.applicationInfo.flags and ApplicationInfo.FLAG_TEST_ONLY)
        assertEquals(0, context.applicationInfo.flags and ApplicationInfo.FLAG_ALLOW_BACKUP)
    }

    /** Invoke only before the upgrade, on the previously delivered debug APK. */
    @Test fun captureDebugUpgradeState() {
        requireEmulator()
        assertNotEquals(0, context.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE)
        assertFalse("unfinished upgrade snapshot exists", snapshot.exists())
        runBlocking {
            val originalCatalog = store.loadCatalog()
            val originalInput = store.loadInputSettings()
            val originalKeyboard = store.loadUiMode().second
            val originalBall = store.loadBallPosition()
            snapshot.writeText(buildJsonObject {
                put("catalog", wireJson.encodeToJsonElement(originalCatalog))
                put("sensitivity", originalInput.sensitivity)
                put("haptics", originalInput.haptics)
                put("keyboard", originalKeyboard)
                put("ballX", originalBall.first)
                put("ballY", originalBall.second)
                put("deviceId", store.deviceId())
                put("certificate", certificate())
            }.toString())
            store.updateCatalog { expectedCatalog }
            store.saveInputSettings(DeviceInputSettings(1.7, false))
            store.saveKeyboardMode(true)
            store.saveBallPosition(0.25f, 0.75f)
        }
    }

    /** A fresh process must decrypt the original Keystore-protected credentials. */
    @Test fun verifyReleaseUpgradeState() {
        requireEmulator()
        releaseBuildDisablesDebugging()
        assertTrue("capture the previous APK's state first", snapshot.isFile)
        val original = wireJson.parseToJsonElement(snapshot.readText()).jsonObject
        runBlocking {
            try {
                assertEquals(expectedCatalog, store.loadCatalog())
                assertEquals(original["deviceId"]!!.jsonPrimitive.content, store.deviceId())
                assertEquals(original["certificate"]!!.jsonPrimitive.content, certificate())
                assertEquals(DeviceInputSettings(1.7, false), store.loadInputSettings())
                assertTrue(store.loadUiMode().second)
                assertEquals(0.25f to 0.75f, store.loadBallPosition())
            } finally {
                store.updateCatalog { wireJson.decodeFromJsonElement<PeerCatalog>(original["catalog"]!!) }
                store.saveInputSettings(DeviceInputSettings(original["sensitivity"]!!.jsonPrimitive.double, original["haptics"]!!.jsonPrimitive.boolean))
                store.saveKeyboardMode(original["keyboard"]!!.jsonPrimitive.boolean)
                store.saveBallPosition(original["ballX"]!!.jsonPrimitive.float, original["ballY"]!!.jsonPrimitive.float)
                assertTrue("cannot remove completed upgrade snapshot", snapshot.delete())
            }
        }
    }
}
