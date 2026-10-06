package com.yuncii.tapdeck

import android.os.SystemClock
import android.view.MotionEvent
import android.view.View
import android.view.ViewGroup
import androidx.lifecycle.ViewModelProvider
import android.content.Intent
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith
import java.util.concurrent.atomic.AtomicInteger
import java.io.File
import kotlinx.serialization.json.*

@RunWith(AndroidJUnit4::class)
class DeviceTest {
    private fun motion(down: Long, action: Int, points: List<Pair<Float, Float>>): MotionEvent {
        val properties = points.indices.map { MotionEvent.PointerProperties().apply { id = it; toolType = MotionEvent.TOOL_TYPE_FINGER } }.toTypedArray()
        val coordinates = points.map { MotionEvent.PointerCoords().apply { x = it.first; y = it.second; pressure = 1f; size = 1f } }.toTypedArray()
        return MotionEvent.obtain(down, SystemClock.uptimeMillis(), action, points.size, properties, coordinates, 0, 0, 1f, 1f, 0, 0, android.view.InputDevice.SOURCE_TOUCHSCREEN, 0)
    }
    @Test fun gesturesMoveClickScrollDragAndCancel() {
        DeviceActivity().use { scenario -> scenario.onActivity { activity ->
            val events = mutableListOf<String>()
            val sink = object : TouchSink {
                override fun move(dx: Double, dy: Double) { events.add("move") }
                override fun scroll(dx: Double, dy: Double) { events.add("scroll") }
                override fun button(name: String, down: Boolean) { events.add("$name:$down") }
                override fun click(name: String) { events.add("click:$name") }
            }
            val pad = TouchpadView(activity, sink).apply { connected = true }
            android.widget.FrameLayout(activity).addView(pad)
            pad.layout(0, 0, 600, 600)
            fun send(action: Int, points: List<Pair<Float,Float>> = listOf(200f to 200f)) { val event = motion(SystemClock.uptimeMillis(), action, points); pad.onTouchEvent(event); event.recycle() }
            send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_UP)
            assertEquals(listOf("click:left"), events)
            events.clear(); send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_MOVE, listOf(300f to 220f)); send(MotionEvent.ACTION_CANCEL)
            assertEquals(listOf("left:true", "move", "left:false"), events)
            events.clear(); send(MotionEvent.ACTION_DOWN)
            send(MotionEvent.ACTION_POINTER_DOWN or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(200f to 200f, 300f to 200f))
            send(MotionEvent.ACTION_POINTER_UP or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(200f to 200f, 300f to 200f)); send(MotionEvent.ACTION_UP)
            assertEquals(listOf("click:right"), events)
            events.clear(); send(MotionEvent.ACTION_DOWN)
            send(MotionEvent.ACTION_POINTER_DOWN or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(200f to 200f, 300f to 200f))
            send(MotionEvent.ACTION_MOVE, listOf(200f to 300f, 300f to 300f)); send(MotionEvent.ACTION_CANCEL)
            assertEquals(listOf("scroll"), events)
        } }
    }
    @Test fun touchpadAndMicrophoneSplitPointers() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.uiAutomation.grantRuntimePermission("com.yuncii.tapdeck", "android.permission.RECORD_AUDIO")
        DeviceActivity().use { scenario ->
            lateinit var client: TapClient
            var micPoint = 0f to 0f; var padPoint = 0f to 0f
            scenario.onActivity { activity ->
                client = ViewModelProvider(activity)[TapViewModel::class.java].client
                val children = views(activity.window.decorView)
                val mic = children.filterIsInstance<MicBallView>().single(); val pad = children.filterIsInstance<TouchpadView>().single()
                val a = IntArray(2); mic.getLocationOnScreen(a); val center = mic.ballCenter(); micPoint = (a[0] + center.first) to (a[1] + center.second)
                pad.getLocationOnScreen(a); padPoint = (a[0] + pad.width / 3f) to (a[1] + pad.height / 3f)
            }
            await { client.state.value.connected }
            val x = client.javaClass.getDeclaredField("x").apply { isAccessible = true }
            val scroll = client.javaClass.getDeclaredField("sy").apply { isAccessible = true }
            val beforeX = x.getLong(client); val beforeScroll = scroll.getLong(client)
            val down = SystemClock.uptimeMillis()
            fun inject(action: Int, points: List<Pair<Float,Float>>) { val event = motion(down, action, points); assertTrue(instrumentation.uiAutomation.injectInputEvent(event, true)); event.recycle() }
            try {
                inject(MotionEvent.ACTION_DOWN, listOf(micPoint)); await { client.state.value.mic == "transmitting" }
                inject(MotionEvent.ACTION_POINTER_DOWN or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(micPoint, padPoint))
                repeat(5) { n -> inject(MotionEvent.ACTION_MOVE, listOf(micPoint, (padPoint.first + (n + 1) * 30) to padPoint.second)); SystemClock.sleep(20) }
                assertTrue("touchpad did not move while microphone was held", x.getLong(client) > beforeX)
                assertEquals("microphone pointer was treated as a second touchpad finger", beforeScroll, scroll.getLong(client))
                assertEquals("transmitting", client.state.value.mic)
                inject(MotionEvent.ACTION_POINTER_UP or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(micPoint, (padPoint.first + 150) to padPoint.second))
                inject(MotionEvent.ACTION_UP, listOf(micPoint)); await(3000) { client.state.value.mic == "idle" }
            } finally { client.stopMic(true) }
        }
    }
    // Launch only the target app. BOOX freezes the separate test APK, so the
    // ActivityScenario helper activities in that package cannot be launched.
    private class DeviceActivity : java.io.Closeable {
        private val instrumentation = InstrumentationRegistry.getInstrumentation()
        private val intent = Intent(instrumentation.targetContext, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_SINGLE_TOP)
        private var activity = instrumentation.startActivitySync(intent) as MainActivity
        fun onActivity(block: (MainActivity) -> Unit) { instrumentation.runOnMainSync { block(activity) }; instrumentation.waitForIdleSync() }
        fun moveToState(state: androidx.lifecycle.Lifecycle.State) {
            if (state == androidx.lifecycle.Lifecycle.State.CREATED) {
                instrumentation.uiAutomation.executeShellCommand("input keyevent KEYCODE_HOME").use { android.os.ParcelFileDescriptor.AutoCloseInputStream(it).readBytes() }
            } else instrumentation.runOnMainSync { instrumentation.targetContext.startActivity(intent) }
            instrumentation.waitForIdleSync()
            val expected = if (state == androidx.lifecycle.Lifecycle.State.CREATED) androidx.lifecycle.Lifecycle.State.CREATED else androidx.lifecycle.Lifecycle.State.RESUMED
            val deadline = SystemClock.elapsedRealtime() + 3000
            while (activity.lifecycle.currentState != expected && SystemClock.elapsedRealtime() < deadline) SystemClock.sleep(20)
            assertEquals(expected, activity.lifecycle.currentState)
        }
        override fun close() { instrumentation.runOnMainSync { activity.finish() }; instrumentation.waitForIdleSync() }
    }
    private fun await(timeoutMs: Long = 10000, condition: () -> Boolean) {
        val deadline = SystemClock.elapsedRealtime() + timeoutMs
        while (!condition() && SystemClock.elapsedRealtime() < deadline) SystemClock.sleep(20)
        assertTrue("condition did not become true within $timeoutMs ms", condition())
    }
    private fun views(root: View): List<View> = listOf(root) + if (root is ViewGroup) (0 until root.childCount).flatMap { views(root.getChildAt(it)) } else emptyList()
    @Test fun percentageLayoutAndCameraOptional() {
        DeviceActivity().use { scenario ->
            SystemClock.sleep(1500)
            scenario.onActivity { activity ->
                val root = activity.window.decorView
                val children = views(root)
                val touch = children.filterIsInstance<TouchpadView>().single()
                val mic = children.filterIsInstance<MicBallView>().single()
                assertEquals(root.width, touch.width)
                assertEquals(root.height * Regions.TOUCHPAD, touch.height.toFloat(), 2f)
                assertEquals(root.height * Regions.MICROPHONE, mic.height.toFloat(), 2f)
                assertFalse(children.any { it is android.widget.ScrollView })
                assertEquals(1f, Regions.CONNECTION + Regions.TOUCHPAD + Regions.SHORTCUTS + Regions.MICROPHONE, 0.0001f)
            }
            val instrumentation = InstrumentationRegistry.getInstrumentation()
            fun nodes(node: android.view.accessibility.AccessibilityNodeInfo): List<android.view.accessibility.AccessibilityNodeInfo> = listOf(node) + (0 until node.childCount).flatMap { index -> node.getChild(index)?.let { nodes(it) } ?: emptyList() }
            val root = instrumentation.uiAutomation.rootInActiveWindow
            assertNotNull("missing accessibility tree", root)
            val buttons = nodes(root).filter { it.contentDescription?.startsWith("快捷键 ") == true }
            var expected = emptyList<Int>(); var top = 0; var bottom = 0; var viewportWidth = 0
            scenario.onActivity { activity ->
                val client = ViewModelProvider(activity)[TapViewModel::class.java].client
                expected = client.state.value.config.visibleShortcuts().map { it.index + 1 }
                val children = views(activity.window.decorView); val touch = children.filterIsInstance<TouchpadView>().single(); val mic = children.filterIsInstance<MicBallView>().single()
                val p = IntArray(2); touch.getLocationOnScreen(p); top = p[1] + touch.height; mic.getLocationOnScreen(p); bottom = p[1]; viewportWidth = touch.width
            }
            InstrumentationRegistry.getArguments().getString("shortcutCount")?.toIntOrNull()?.let { assertEquals(it, expected.size) }
            assertEquals("wrong dynamic button count", expected.size, buttons.size)
            val rects = buttons.map { node -> android.graphics.Rect().also { node.getBoundsInScreen(it) } }
            buttons.forEachIndexed { index, node ->
                assertTrue(node.contentDescription.toString().startsWith("快捷键 ${expected[index]}："))
                assertTrue("button not touchable", node.isClickable || node.parent?.isClickable == true)
                val r = rects[index]
                assertTrue("button outside shortcut region: $r", r.top >= top && r.bottom <= bottom && r.left >= 0 && r.right <= viewportWidth)
                assertTrue("empty button", r.width() > 0 && r.height() > 0)
                for (j in 0 until index) assertFalse("buttons overlap", android.graphics.Rect.intersects(r, rects[j]))
            }
            assertEquals(if (expected.size > 4) 2 else 1, rects.map { it.top }.distinct().size)
        }
    }
    @Test fun mixedVoiceGestures100Times() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.uiAutomation.grantRuntimePermission("com.yuncii.tapdeck", "android.permission.RECORD_AUDIO")
        DeviceActivity().use { scenario ->
            lateinit var client: TapClient
            lateinit var mic: MicBallView
            scenario.onActivity { activity ->
                client = ViewModelProvider(activity)[TapViewModel::class.java].client
                mic = views(activity.window.decorView).filterIsInstance<MicBallView>().single()
            }
            await { client.state.value.connected }
            val original = mic.normalizedPosition()
            var down = 0L
            fun event(action: Int, dx: Float = 0f, dy: Float = 0f) = scenario.onActivity {
                val p = mic.ballCenter()
                if (action == MotionEvent.ACTION_DOWN) down = SystemClock.uptimeMillis()
                val e = MotionEvent.obtain(down, SystemClock.uptimeMillis(), action, p.first + dx, p.second + dy, 0)
                mic.dispatchTouchEvent(e); e.recycle()
            }
            fun tap() { event(MotionEvent.ACTION_DOWN); event(MotionEvent.ACTION_UP) }
            try {
                for (i in 0 until 100) {
                    scenario.onActivity { mic.cancel() }
                    val toggle = i % 2 == 1
                    if (toggle) { tap(); SystemClock.sleep(40); tap() } else event(MotionEvent.ACTION_DOWN)
                    await(3000) { client.state.value.mic == "transmitting" || client.state.value.error.isNotEmpty() }
                    assertEquals("round $i: ${client.state.value.error}", "transmitting", client.state.value.mic)
                    assertEquals(if (toggle) "toggle" else "hold", client.state.value.micMode)
                    SystemClock.sleep(50)
                    when (i % 10) {
                        2 -> { event(MotionEvent.ACTION_MOVE, 80f); assertEquals("transmitting", client.state.value.mic); event(MotionEvent.ACTION_UP) }
                        3 -> { event(MotionEvent.ACTION_DOWN); event(MotionEvent.ACTION_MOVE, -80f); event(MotionEvent.ACTION_CANCEL); assertEquals("transmitting", client.state.value.mic); tap() }
                        4 -> event(MotionEvent.ACTION_CANCEL)
                        5 -> { event(MotionEvent.ACTION_DOWN); event(MotionEvent.ACTION_CANCEL); assertEquals("transmitting", client.state.value.mic); tap() }
                        6, 7 -> {
                            scenario.moveToState(androidx.lifecycle.Lifecycle.State.CREATED)
                            await(3000) { client.state.value.mic == "idle" }
                            scenario.moveToState(androidx.lifecycle.Lifecycle.State.RESUMED)
                        }
                        8, 9 -> {
                            scenario.onActivity { client.disconnect("test disconnect") }
                            assertEquals("idle", client.state.value.mic)
                            kotlinx.coroutines.runBlocking { client.restore() }
                            await { client.state.value.connected }
                            assertEquals("idle", client.state.value.mic)
                        }
                        else -> if (toggle) tap() else event(MotionEvent.ACTION_UP)
                    }
                    await(3000) { client.state.value.mic == "idle" }
                    val recorder = client.javaClass.getDeclaredField("recorder").apply { isAccessible = true }.get(client)
                    val running = recorder.javaClass.getDeclaredField("running").apply { isAccessible = true }.get(recorder) as java.util.concurrent.atomic.AtomicBoolean
                    assertFalse("capture continued on round $i", running.get())
                }
            } finally {
                scenario.onActivity { client.stopMic(true); ViewModelProvider(it)[TapViewModel::class.java].saveBallPosition(original.first, original.second) }
            }
        }
    }
    @Test fun stopWhilePreparingDoesNotStartLateCapture() {
        InstrumentationRegistry.getInstrumentation().uiAutomation.grantRuntimePermission("com.yuncii.tapdeck", "android.permission.RECORD_AUDIO")
        DeviceActivity().use { scenario ->
            lateinit var client: TapClient
            scenario.onActivity { client = ViewModelProvider(it)[TapViewModel::class.java].client }
            await { client.state.value.connected }
            repeat(10) {
                scenario.onActivity { assertTrue(client.startMic("toggle")); client.stopMic() }
                await(3000) { client.state.value.mic == "idle" }
                SystemClock.sleep(80)
                val requested = client.javaClass.getDeclaredField("recordingRequested").apply { isAccessible = true }.getBoolean(client)
                assertFalse("late ready revived recording", requested)
            }
        }
    }
    @Test fun physicalAudioRecordStopsCleanly() {
        InstrumentationRegistry.getInstrumentation().uiAutomation.grantRuntimePermission("com.yuncii.tapdeck", "android.permission.RECORD_AUDIO")
        DeviceActivity().use {
            val frames = AtomicInteger()
            val failures = mutableListOf<String>()
            val recorder = MicCapture({ pcm, position, _ -> assertEquals(960, pcm.size); assertEquals(0, position % 480); frames.incrementAndGet() }, { synchronized(failures) { failures.add(it) } })
            repeat(10) {
                val before = frames.get()
                recorder.start(); await(2000) { frames.get() > before }
                recorder.stop(); val stopped = frames.get(); SystemClock.sleep(80)
                assertEquals("audio continued after stop", stopped, frames.get())
            }
            assertTrue(failures.toString(), failures.isEmpty())
        }
    }
    @Test fun wifiAudioAndMouseSoak() {
        val args = InstrumentationRegistry.getArguments()
        val seconds = args.getString("soakSeconds")?.toLongOrNull() ?: 0
        assumeTrue("pass -e soakSeconds 1800 to run the 30-minute test", seconds > 0)
        InstrumentationRegistry.getInstrumentation().uiAutomation.grantRuntimePermission("com.yuncii.tapdeck", "android.permission.RECORD_AUDIO")
        DeviceActivity().use { scenario ->
            lateinit var client: TapClient
            scenario.onActivity { activity -> client = ViewModelProvider(activity)[TapViewModel::class.java].client }
            await { client.state.value.connected }
            scenario.onActivity { client.startMic() }
            await { client.state.value.mic == "transmitting" }
            val started = SystemClock.elapsedRealtime()
            val rtts = mutableListOf<Long>()
            var nextSample = started
            var direction = 1.0
            try {
                while (SystemClock.elapsedRealtime() - started < seconds * 1000) {
                    assertTrue("connection lost: ${client.state.value.error}", client.state.value.connected)
                    assertEquals("audio stopped: ${client.state.value.error}", "transmitting", client.state.value.mic)
                    client.move(direction, 0.0); direction = -direction
                    val now = SystemClock.elapsedRealtime()
                    if (now >= nextSample) { rtts.add(client.state.value.rttMs); nextSample = now + 250 }
                    SystemClock.sleep(16)
                }
            } finally { scenario.onActivity { client.stopMic() } }
            await(3000) { client.state.value.mic == "idle" }
            rtts.sort()
            val result = buildJsonObject {
                put("duration_seconds", seconds); put("samples", rtts.size)
                put("rtt_median_ms", rtts[rtts.size / 2]); put("rtt_p95_ms", rtts[((rtts.size - 1) * 0.95).toInt()]); put("rtt_max_ms", rtts.last())
                put("connection_survived", true); put("microphone_stopped", true)
            }
            val context = InstrumentationRegistry.getInstrumentation().targetContext
            File(context.filesDir, "soak-result.json").writeText(result.toString())
            android.util.Log.i("TapDeckTest", result.toString())
        }
    }
    @Test fun holdForExternalKeyProbe() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        assumeTrue("coordinated Windows key-state probe only", InstrumentationRegistry.getArguments().getString("externalKeyProbe") == "true")
        instrumentation.uiAutomation.grantRuntimePermission("com.yuncii.tapdeck", "android.permission.RECORD_AUDIO")
        DeviceActivity().use { scenario ->
            lateinit var client: TapClient
            scenario.onActivity { client = ViewModelProvider(it)[TapViewModel::class.java].client }
            await { client.state.value.connected }
            scenario.onActivity { assertTrue(client.startMic("hold")) }
            try { await { client.state.value.mic == "transmitting" }; SystemClock.sleep(4000) }
            finally { scenario.onActivity { client.stopMic() } }
            await { client.state.value.mic == "idle" }
        }
    }
    @Test fun receiverInterruptionStopsMicAndReconnects() {
        assumeTrue("receiver restart is coordinated by the PC harness", InstrumentationRegistry.getArguments().getString("restartReceiver") == "true")
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.uiAutomation.grantRuntimePermission("com.yuncii.tapdeck", "android.permission.RECORD_AUDIO")
        DeviceActivity().use { scenario ->
            lateinit var client: TapClient
            scenario.onActivity { client = ViewModelProvider(it)[TapViewModel::class.java].client }
            await { client.state.value.connected }
            scenario.onActivity { client.startMic() }
            await { client.state.value.mic == "transmitting" }
            File(instrumentation.targetContext.filesDir, "restart-ready").writeText("ready")
            await(15000) { !client.state.value.connected && client.state.value.mic == "idle" }
            await(15000) { client.state.value.connected }
            scenario.onActivity { client.startMic() }
            await { client.state.value.mic == "transmitting" }
            SystemClock.sleep(100)
            scenario.onActivity { client.stopMic() }
            await { client.state.value.mic == "idle" }
        }
    }
    @Test fun screenSleepStopsMicrophone() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val keyguard = instrumentation.targetContext.getSystemService(android.app.KeyguardManager::class.java)
        assumeTrue("physical screen test requires a device without a secure lock", !keyguard.isDeviceSecure)
        instrumentation.uiAutomation.grantRuntimePermission("com.yuncii.tapdeck", "android.permission.RECORD_AUDIO")
        fun shell(command: String) { instrumentation.uiAutomation.executeShellCommand(command).use { android.os.ParcelFileDescriptor.AutoCloseInputStream(it).readBytes() } }
        DeviceActivity().use { scenario ->
            lateinit var client: TapClient
            scenario.onActivity { client = ViewModelProvider(it)[TapViewModel::class.java].client }
            await { client.state.value.connected }
            scenario.onActivity { client.startMic() }
            await { client.state.value.mic == "transmitting" }
            try {
                shell("input keyevent KEYCODE_SLEEP")
                await(3000) { client.state.value.mic == "idle" }
                assertFalse(instrumentation.targetContext.getSystemService(android.os.PowerManager::class.java).isInteractive)
            } finally { shell("input keyevent KEYCODE_WAKEUP") }
            scenario.moveToState(androidx.lifecycle.Lifecycle.State.RESUMED)
            assertEquals("idle", client.state.value.mic)
        }
    }
    @Test fun disconnectWhileRecordingThenRestoreStartsFreshMic() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.uiAutomation.grantRuntimePermission("com.yuncii.tapdeck", "android.permission.RECORD_AUDIO")
        DeviceActivity().use { scenario ->
            lateinit var client: TapClient
            scenario.onActivity { client = ViewModelProvider(it)[TapViewModel::class.java].client }
            await { client.state.value.connected }
            scenario.onActivity { client.startMic() }
            await { client.state.value.mic == "transmitting" }
            scenario.onActivity { client.disconnect() }
            assertEquals("idle", client.state.value.mic)
            kotlinx.coroutines.runBlocking { client.restore() }
            await { client.state.value.connected }
            scenario.onActivity { client.startMic() }
            await { client.state.value.mic == "transmitting" }
            scenario.onActivity { client.stopMic() }
            await { client.state.value.mic == "idle" }
        }
    }
    @Test fun ballGestureHitTestDragAndPointerOwnership() {
        DeviceActivity().use { scenario ->
            lateinit var ball: MicBallView
            val events = mutableListOf<String>()
            scenario.onActivity { activity ->
                ball = MicBallView(activity, { mode ->
                    events.add("start:$mode"); ball.mode = mode; ball.status = "transmitting"; true
                }, { abort ->
                    events.add("stop:$abort"); ball.status = "idle"; ball.mode = ""
                }).apply { available = true }
                activity.addContentView(ball, android.view.ViewGroup.LayoutParams(android.view.ViewGroup.LayoutParams.MATCH_PARENT, 320))
            }
            var down = 0L
            fun send(action: Int, dx: Float = 0f, dy: Float = 0f) = scenario.onActivity {
                val (x, y) = ball.ballCenter()
                if (action == MotionEvent.ACTION_DOWN) down = SystemClock.uptimeMillis()
                val e = MotionEvent.obtain(down, SystemClock.uptimeMillis(), action, x + dx, y + dy, 0)
                ball.dispatchTouchEvent(e); e.recycle()
            }
            fun tap() { send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_UP) }
            scenario.onActivity {
                val now = SystemClock.uptimeMillis()
                val e = MotionEvent.obtain(now, now, MotionEvent.ACTION_DOWN, 0f, 0f, 0)
                assertFalse("blank voice region consumed a press", ball.dispatchTouchEvent(e)); e.recycle()
            }
            tap(); SystemClock.sleep(350)
            scenario.onActivity { assertTrue(events.isEmpty()); ball.cancel() }
            send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_MOVE, 80f)
            SystemClock.sleep(350); send(MotionEvent.ACTION_UP)
            scenario.onActivity { assertTrue("drag started audio", events.isEmpty()); ball.cancel() }
            tap(); SystemClock.sleep(40); tap()
            scenario.onActivity { assertEquals(listOf("start:toggle"), events) }
            send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_MOVE, -80f); send(MotionEvent.ACTION_CANCEL)
            scenario.onActivity { assertEquals("transmitting", ball.status); assertEquals(1, events.size) }
            tap()
            scenario.onActivity { assertEquals(listOf("start:toggle", "stop:false"), events); events.clear(); ball.cancel() }
            send(MotionEvent.ACTION_DOWN); SystemClock.sleep(350)
            scenario.onActivity { assertEquals(listOf("start:hold"), events) }
            scenario.onActivity {
                val p = ball.ballCenter()
                val e = motion(down, MotionEvent.ACTION_POINTER_UP or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(p, 0f to 0f))
                ball.onTouchEvent(e); e.recycle()
                assertEquals("another finger stopped hold", 1, events.size)
            }
            send(MotionEvent.ACTION_MOVE, 20000f, -20000f)
            scenario.onActivity { val p = ball.ballCenter(); assertTrue(p.first < ball.width); assertTrue(p.second > 0); assertEquals("transmitting", ball.status) }
            send(MotionEvent.ACTION_CANCEL)
            scenario.onActivity { assertEquals(listOf("start:hold", "stop:true"), events) }
        }
    }
    @Test fun ballPositionSurvivesActivityRestart() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val store = PairStore(instrumentation.targetContext)
        val original = kotlinx.coroutines.runBlocking { store.loadBallPosition() }
        var expected = 0f to 0f
        try {
            DeviceActivity().use { scenario ->
                scenario.onActivity { activity ->
                    val mic = views(activity.window.decorView).filterIsInstance<MicBallView>().single()
                    val p = mic.ballCenter(); val now = SystemClock.uptimeMillis()
                    for ((action, dx, dy) in listOf(Triple(MotionEvent.ACTION_DOWN, 0f, 0f), Triple(MotionEvent.ACTION_MOVE, 20000f, -20000f), Triple(MotionEvent.ACTION_UP, 20000f, -20000f))) {
                        val e = MotionEvent.obtain(now, SystemClock.uptimeMillis(), action, p.first + dx, p.second + dy, 0); mic.dispatchTouchEvent(e); e.recycle()
                    }
                    expected = mic.normalizedPosition()
                    assertEquals("idle", ViewModelProvider(activity)[TapViewModel::class.java].client.state.value.mic)
                }
                await { kotlinx.coroutines.runBlocking { store.loadBallPosition() } == expected }
            }
            DeviceActivity().use { scenario ->
                SystemClock.sleep(300)
                scenario.onActivity { activity ->
                    val mic = views(activity.window.decorView).filterIsInstance<MicBallView>().single()
                    assertEquals(expected, mic.normalizedPosition())
                }
            }
        } finally { kotlinx.coroutines.runBlocking { store.saveBallPosition(original.first, original.second) } }
    }

}
