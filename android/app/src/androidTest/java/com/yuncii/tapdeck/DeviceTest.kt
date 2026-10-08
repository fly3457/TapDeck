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
import kotlinx.coroutines.flow.MutableStateFlow
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Rect
import android.view.accessibility.AccessibilityNodeInfo
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.ComposeView

@RunWith(AndroidJUnit4::class)
class DeviceTest {
    private fun motion(down: Long, action: Int, points: List<Pair<Float, Float>>, ids: List<Int> = points.indices.toList()): MotionEvent {
        val properties = ids.map { MotionEvent.PointerProperties().apply { id = it; toolType = MotionEvent.TOOL_TYPE_FINGER } }.toTypedArray()
        val coordinates = points.map { MotionEvent.PointerCoords().apply { x = it.first; y = it.second; pressure = 1f; size = 1f } }.toTypedArray()
        return MotionEvent.obtain(down, SystemClock.uptimeMillis(), action, points.size, properties, coordinates, 0, 0, 1f, 1f, 0, 0, android.view.InputDevice.SOURCE_TOUCHSCREEN, 0)
    }
    @Test fun gesturesMoveClickScrollDragAndCancel() {
        DeviceActivity().use { scenario ->
            val events = java.util.concurrent.CopyOnWriteArrayList<String>()
            lateinit var pad: TouchpadView
            scenario.onActivity { activity ->
            val sink = object : TouchSink {
                override fun move(dx: Double, dy: Double) { events.add("move") }
                override fun scroll(dx: Double, dy: Double) { events.add("scroll") }
                override fun button(name: String, down: Boolean) { events.add("$name:$down") }
                override fun click(name: String) { events.add("click:$name") }
            }
            pad = TouchpadView(activity, sink).apply { connected = true }
            android.widget.FrameLayout(activity).addView(pad)
            pad.layout(0, 0, 600, 600)
            }
            fun send(action: Int, points: List<Pair<Float,Float>> = listOf(200f to 200f)) { scenario.onActivity { val event = motion(SystemClock.uptimeMillis(), action, points); pad.onTouchEvent(event); event.recycle() } }
            send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_UP)
            assertTrue("single click must wait for the second tap", events.isEmpty())
            await(1000) { events.size == 1 }
            assertEquals(listOf("click:left"), events.toList())
            events.clear()
            send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_UP); send(MotionEvent.ACTION_DOWN)
            assertEquals("second down must only hold the first PC click", listOf("left:true"), events.toList())
            send(MotionEvent.ACTION_UP)
            assertEquals(listOf("left:true", "left:false", "click:left"), events.toList())
            events.clear()
            send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_UP); send(MotionEvent.ACTION_DOWN)
            send(MotionEvent.ACTION_MOVE, listOf(202f to 202f))
            assertEquals("tap jitter must not move the cursor", listOf("left:true"), events.toList())
            send(MotionEvent.ACTION_MOVE, listOf(300f to 220f)); send(MotionEvent.ACTION_UP)
            assertEquals(listOf("left:true", "move", "left:false"), events.toList())
            events.clear()
            send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_UP); send(MotionEvent.ACTION_DOWN)
            SystemClock.sleep(330); send(MotionEvent.ACTION_UP)
            assertEquals("stationary hold must not double click", listOf("left:true", "left:false"), events.toList())
            events.clear()
            send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_UP); send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_CANCEL)
            assertEquals(listOf("left:true", "left:false"), events.toList())
            events.clear(); send(MotionEvent.ACTION_DOWN); send(MotionEvent.ACTION_UP)
            scenario.onActivity { pad.connected = false }
            SystemClock.sleep(330)
            assertTrue("disconnect must cancel the delayed click", events.isEmpty())
            scenario.onActivity { pad.connected = true }
            events.clear(); send(MotionEvent.ACTION_DOWN)
            send(MotionEvent.ACTION_POINTER_DOWN or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(200f to 200f, 300f to 200f))
            send(MotionEvent.ACTION_POINTER_UP or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(200f to 200f, 300f to 200f)); send(MotionEvent.ACTION_UP)
            assertEquals(listOf("click:right"), events.toList())
            events.clear(); send(MotionEvent.ACTION_DOWN)
            send(MotionEvent.ACTION_POINTER_DOWN or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(200f to 200f, 300f to 200f))
            send(MotionEvent.ACTION_MOVE, listOf(200f to 300f, 300f to 300f)); send(MotionEvent.ACTION_CANCEL)
            assertEquals(listOf("scroll"), events.toList())
            scenario.onActivity { pad.cancel() }
        }
    }
    @Test fun multiFingerZoomSwipeAndPointerIds() {
        DeviceActivity().use { scenario -> scenario.onActivity { activity ->
            val events = mutableListOf<String>()
            val sink = object : TouchSink {
                override fun move(dx: Double, dy: Double) { events.add("move") }
                override fun scroll(dx: Double, dy: Double) { events.add("scroll:${if (dy < 0) "up" else "down"}") }
                override fun button(name: String, down: Boolean) { events.add("$name:$down") }
                override fun click(name: String) { events.add("click:$name") }
                override fun zoom(steps: Int) { events.add("zoom:$steps") }
                override fun gesture(direction: String) { events.add("gesture:$direction") }
            }
            val pad = TouchpadView(activity, sink).apply { connected = true }
            val density = activity.resources.displayMetrics.density
            fun send(action: Int, points: List<Pair<Float,Float>>, ids: List<Int>) {
                val event = motion(SystemClock.uptimeMillis(), action, points.map { it.first * density to it.second * density }, ids)
                pad.onTouchEvent(event); event.recycle()
            }
            val add = MotionEvent.ACTION_POINTER_DOWN or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT)
            val remove = MotionEvent.ACTION_POINTER_UP or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT)
            fun two() { send(MotionEvent.ACTION_DOWN, listOf(100f to 150f), listOf(7)); send(add, listOf(100f to 150f, 200f to 150f), listOf(7, 2)) }
            two()
            send(MotionEvent.ACTION_MOVE, listOf(220f to 150f, 80f to 150f), listOf(2, 7))
            send(MotionEvent.ACTION_MOVE, listOf(200f to 150f, 100f to 150f), listOf(2, 7))
            assertTrue(events.any { it.startsWith("zoom:") && it.substringAfter(':').toInt() > 0 })
            assertTrue(events.any { it.startsWith("zoom:") && it.substringAfter(':').toInt() < 0 })
            assertFalse(events.any { it.startsWith("scroll") })
            send(remove, listOf(200f to 150f, 100f to 150f), listOf(2, 7))
            send(MotionEvent.ACTION_MOVE, listOf(200f to 250f), listOf(2)); send(MotionEvent.ACTION_UP, listOf(200f to 250f), listOf(2))
            assertFalse("lifting one finger must not click or move", events.any { it.startsWith("click") || it == "move" })
            events.clear(); two()
            send(MotionEvent.ACTION_MOVE, listOf(100f to 110f, 200f to 110f), listOf(7, 2))
            send(MotionEvent.ACTION_MOVE, listOf(200f to 170f, 100f to 170f), listOf(2, 7))
            assertEquals(listOf("scroll:up", "scroll:down"), events)
            send(MotionEvent.ACTION_POINTER_DOWN or (2 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(200f to 170f, 100f to 170f, 150f to 170f), listOf(2, 7, 12))
            send(MotionEvent.ACTION_MOVE, listOf(150f to 125f, 200f to 125f, 100f to 125f), listOf(12, 2, 7))
            send(MotionEvent.ACTION_MOVE, listOf(150f to 60f, 200f to 60f, 100f to 60f), listOf(12, 2, 7))
            send(remove, listOf(150f to 60f, 200f to 60f, 100f to 60f), listOf(12, 2, 7))
            send(MotionEvent.ACTION_MOVE, listOf(150f to 250f, 100f to 250f), listOf(12, 7)); send(remove, listOf(150f to 250f, 100f to 250f), listOf(12, 7))
            send(MotionEvent.ACTION_UP, listOf(150f to 250f), listOf(12))
            assertEquals(listOf("scroll:up", "scroll:down", "gesture:up"), events)
            events.clear(); two()
            send(MotionEvent.ACTION_POINTER_DOWN or (2 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(100f to 150f, 200f to 150f, 150f to 150f), listOf(7, 2, 12))
            send(MotionEvent.ACTION_MOVE, listOf(100f to 195f, 200f to 195f, 150f to 195f), listOf(7, 2, 12))
            assertEquals(listOf("gesture:down"), events)
            pad.cancel(); events.clear(); two()
            send(MotionEvent.ACTION_POINTER_DOWN or (2 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(100f to 150f, 200f to 150f, 150f to 150f), listOf(7, 2, 12))
            send(MotionEvent.ACTION_POINTER_DOWN or (3 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(100f to 150f, 200f to 150f, 150f to 150f, 180f to 150f), listOf(7, 2, 12, 9))
            send(MotionEvent.ACTION_MOVE, listOf(100f to 220f, 200f to 220f, 150f to 220f, 180f to 220f), listOf(7, 2, 12, 9))
            assertTrue("four fingers must be ignored", events.isEmpty())
            pad.cancel()
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
        init {
            InstrumentationRegistry.getArguments().getString("uiHeight")?.toIntOrNull()?.takeIf { it > 0 }?.let { height ->
                onActivity { a -> a.findViewById<View>(R.id.controller_regions).apply { layoutParams.height = height; requestLayout() } }
            }
        }
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
                val viewport = activity.findViewById<View>(R.id.controller_regions)
                val expected = ControllerLayout.measure(viewport.width, viewport.height)
                val panel = activity.findViewById<ViewGroup>(R.id.control_panel)
                assertEquals(viewport.width, touch.width)
                assertEquals(expected.header, activity.findViewById<View>(R.id.connection_header).height)
                assertEquals(expected.touchpad, touch.height)
                assertEquals(expected.panelContent / 2f, mic.height.toFloat(), 1f)
                assertEquals(expected.panel, panel.height)
                assertEquals(expected.panelPadding, panel.paddingTop)
                assertEquals(expected.panelPadding, panel.paddingBottom)
                assertFalse(children.any { it is android.widget.ScrollView })
                assertEquals(viewport.height, expected.header + expected.touchpad + panel.height)
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
    @Suppress("UNCHECKED_CAST")
    private fun uiState(activity: MainActivity): MutableStateFlow<ClientState> {
        val client = ViewModelProvider(activity)[TapViewModel::class.java].client
        return client.javaClass.getDeclaredField("mutable").apply { isAccessible = true }.get(client) as MutableStateFlow<ClientState>
    }
    private fun accessibilityNodes(node: AccessibilityNodeInfo): List<AccessibilityNodeInfo> =
        listOf(node) + (0 until node.childCount).flatMap { index -> node.getChild(index)?.let { accessibilityNodes(it) } ?: emptyList() }

    private fun saveUiScreenshot(name: String) {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val bitmap = instrumentation.uiAutomation.takeScreenshot() ?: error("Screenshot unavailable")
        val dir = File(instrumentation.targetContext.getExternalFilesDir(null), "ui-validation").apply { mkdirs() }
        val label = InstrumentationRegistry.getArguments().getString("uiLabel") ?: "default"
        File(dir, "$label-$name.png").outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
        bitmap.recycle()
    }

    @Test fun deviceSensitivitySliderPersistsAndKeepsPcConfigIndependent() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val automation = instrumentation.uiAutomation
        val store = PairStore(instrumentation.targetContext)
        val original = kotlinx.coroutines.runBlocking { store.loadInputSettings() }
        val originalCatalog = kotlinx.coroutines.runBlocking { store.loadCatalog() }
        fun tapNode(node: AccessibilityNodeInfo) {
            val bounds = Rect().also(node::getBoundsInScreen)
            val down = SystemClock.uptimeMillis()
            for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                val event = motion(down, action, listOf(bounds.exactCenterX() to bounds.exactCenterY()))
                assertTrue(automation.injectInputEvent(event, true)); event.recycle()
            }
            instrumentation.waitForIdleSync()
        }
        try {
            DeviceActivity().use { scenario ->
                lateinit var client: TapClient
                scenario.onActivity { activity ->
                    client = ViewModelProvider(activity)[TapViewModel::class.java].client
                    client.setSensitivity(1.0); client.setHaptics(true)
                    uiState(activity).value = ClientState()
                }
                await { kotlinx.coroutines.runBlocking { store.loadInputSettings() } == DeviceInputSettings() }
                var iconBounds = Rect(); var touchBounds = Rect()
                scenario.onActivity { activity ->
                    val button = activity.findViewById<View>(R.id.touchpad_sensitivity_button)
                    val touch = views(activity.window.decorView).filterIsInstance<TouchpadView>().single()
                    button.getGlobalVisibleRect(iconBounds); touch.getGlobalVisibleRect(touchBounds)
                    assertTrue("icon bounds $iconBounds outside touchpad $touchBounds", touchBounds.contains(iconBounds))
                    assertTrue(iconBounds.width() > 0 && iconBounds.height() > 0)
                    assertTrue(iconBounds.left < touchBounds.left + touch.width * 0.12)
                }
                fun nodes() = automation.rootInActiveWindow?.let(::accessibilityNodes).orEmpty()
                fun text(value: String) = nodes().firstOrNull { it.text?.toString() == value }
                val down = SystemClock.uptimeMillis()
                for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                    val event = motion(down, action, listOf(iconBounds.exactCenterX() to iconBounds.exactCenterY()))
                    assertTrue(automation.injectInputEvent(event, true)); event.recycle()
                }
                await { text("触控板灵敏度") != null }
                assertNotNull(text("1.0×"))
                assertTrue(nodes().none { node -> node.text?.toString()?.let {
                    it.contains("慢速") || it.contains("快速") || it.contains("立即生效") || it.contains("保存在这台")
                } == true })
                fun setProgress(value: Float) {
                    val slider = nodes().single { it.contentDescription?.toString() == "触控板灵敏度滑块" }
                    assertTrue(slider.performAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_SET_PROGRESS.id,
                        android.os.Bundle().apply { putFloat(AccessibilityNodeInfo.ACTION_ARGUMENT_PROGRESS_VALUE, value) }))
                    await { client.inputSettings.value.sensitivity == DeviceInputSettings.normalize(value.toDouble()) }
                }
                setProgress(0.5f); await { text("0.5×") != null }
                setProgress(3f); await { text("3.0×") != null }
                saveUiScreenshot("sensitivity-slider")
                tapNode(text("恢复 1.0×")!!)
                await { client.inputSettings.value.sensitivity == 1.0 }
                setProgress(1.7f)
                tapNode(text("完成")!!)
                await { text("触控板灵敏度") == null }
                // Exercise the real movement conversion without opening a socket or injecting PC input.
                scenario.onActivity { activity ->
                    val gainLock = client.javaClass.getDeclaredField("lock").apply { isAccessible = true }.get(client)!!
                    val connected = client.javaClass.getDeclaredField("connected").apply { isAccessible = true }
                    val x = client.javaClass.getDeclaredField("x").apply { isAccessible = true }
                    val y = client.javaClass.getDeclaredField("y").apply { isAccessible = true }
                    synchronized(gainLock) {
                        uiState(activity).value = ClientState(config = PcConfig(sensitivity = 0.1))
                        try {
                            connected.setBoolean(client, true)
                            for ((multiplier, expected) in listOf(0.5 to 10240L, 1.0 to 20480L, 3.0 to 61440L)) {
                                client.setSensitivity(multiplier)
                                val beforeX = x.getLong(client); val beforeY = y.getLong(client)
                                client.move(10.0, -10.0)
                                assertEquals(expected, x.getLong(client) - beforeX)
                                assertEquals(-expected, y.getLong(client) - beforeY)
                            }
                            uiState(activity).value = ClientState(config = PcConfig(sensitivity = 5.0))
                            assertEquals(3.0, client.inputSettings.value.sensitivity, 0.0)
                        } finally { connected.setBoolean(client, false) }
                    }
                    client.setSensitivity(1.7); client.setHaptics(false)
                }
                await { kotlinx.coroutines.runBlocking { store.loadInputSettings() } == DeviceInputSettings(1.7, false) }
            }
            DeviceActivity().use { scenario ->
                lateinit var client: TapClient
                scenario.onActivity { activity -> client = ViewModelProvider(activity)[TapViewModel::class.java].client }
                await { client.inputSettings.value == DeviceInputSettings(1.7, false) }
                fun nodes() = automation.rootInActiveWindow?.let(::accessibilityNodes).orEmpty()
                assertTrue(nodes().single { it.contentDescription?.startsWith("连接设置") == true }
                    .performAction(AccessibilityNodeInfo.ACTION_CLICK))
                var toggle: AccessibilityNodeInfo? = null
                await { toggle = nodes().firstOrNull { it.contentDescription?.toString() == "按键震动反馈" }; toggle != null }
                tapNode(nodes().single { it.text?.toString() == "测试震动" })
                await { nodes().any { it.text?.toString() == "请先开启按键震动反馈" } }
                toggle = nodes().single { it.contentDescription?.toString() == "按键震动反馈" }
                tapNode(toggle!!)
                await { client.inputSettings.value.haptics }
                assertTrue(nodes().any { it.text?.toString() == "测试震动" })
                tapNode(nodes().single { it.text?.toString() == "测试震动" })
                await { nodes().any { it.text?.toString() in listOf("已发送测试震动", "这台设备没有振动马达") } }
                saveUiScreenshot("key-feedback-settings")
                val offToggle = nodes().single { it.contentDescription?.toString() == "按键震动反馈" }
                tapNode(offToggle)
                await { !client.inputSettings.value.haptics }
                instrumentation.sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
                await { nodes().none { it.text?.toString() == "连接与设置" } }
                scenario.onActivity { client.forget() }
                SystemClock.sleep(100)
                assertEquals(DeviceInputSettings(1.7, false), kotlinx.coroutines.runBlocking { store.loadInputSettings() })
            }
        } finally { kotlinx.coroutines.runBlocking {
            store.saveInputSettings(original)
            store.updateCatalog { originalCatalog }
        } }
    }

    @Test fun keyFeedbackUsesVibratorServiceWithSystemTouchFeedbackOff() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val automation = instrumentation.uiAutomation
        fun shell(command: String): String = automation.executeShellCommand(command).use { descriptor ->
            android.os.ParcelFileDescriptor.AutoCloseInputStream(descriptor).bufferedReader().use { it.readText().trim() }
        }
        assumeTrue("system feedback settings are only changed on an isolated emulator", shell("getprop ro.kernel.qemu") == "1")
        assumeTrue(android.os.Build.VERSION.SDK_INT >= 33)
        val previous = listOf("haptic_feedback_enabled", "haptic_feedback_intensity").associateWith { shell("settings get system $it") }
        val context = instrumentation.targetContext
        assertEquals(android.content.pm.PackageManager.PERMISSION_GRANTED,
            context.checkSelfPermission(android.Manifest.permission.VIBRATE))
        val vibrator = context.getSystemService(android.os.VibratorManager::class.java).defaultVibrator
        assertTrue("validation emulator requires a simulated vibrator", vibrator.hasVibrator())
        fun records() = shell("dumpsys vibrator_manager").lineSequence()
            .filter { it.contains("com.yuncii.tapdeck") && it.contains("status: finished") && it.contains("Usage=PHYSICAL_EMULATION") }.toList()
        try {
            shell("settings put system haptic_feedback_enabled 1")
            shell("settings put system haptic_feedback_intensity 2")
            DeviceActivity().use { scenario ->
                val backend = AndroidFeedbackBackend(context)
                var enabled = true
                val controller = KeyFeedbackController({ enabled }, backend)
                assertEquals(FeedbackResult.Requested, controller.availability())
                for (kind in KeyFeedback.entries) {
                    val before = records()
                    scenario.onActivity { activity ->
                        assertEquals(FeedbackResult.Requested, activity.window.decorView.keyFeedback(kind, controller))
                    }
                    await { records() != before }
                    val dump = shell("dumpsys vibrator_manager")
                    assertTrue("feedback was not classified as physical emulation: $dump", dump.contains("PHYSICAL_EMULATION"))
                    SystemClock.sleep(150)
                }
                val beforeDisabled = records()
                enabled = false
                scenario.onActivity { activity ->
                    assertEquals(FeedbackResult.AppDisabled, activity.window.decorView.keyFeedback(KeyFeedback.Press, controller))
                }
                SystemClock.sleep(150)
                assertEquals("app-disabled feedback reached the vibrator", beforeDisabled, records())
                enabled = true
                shell("settings put system haptic_feedback_enabled 0")
                SystemClock.sleep(150)
                assertEquals(FeedbackResult.Requested, controller.availability())
                scenario.onActivity { activity ->
                    assertEquals(FeedbackResult.Requested, activity.window.decorView.keyFeedback(KeyFeedback.Press, controller))
                }
                await { records() != beforeDisabled }
                val beforeIntensityOff = records()
                shell("settings put system haptic_feedback_enabled 1")
                shell("settings put system haptic_feedback_intensity 0")
                SystemClock.sleep(150)
                assertEquals(FeedbackResult.Requested, controller.availability())
                scenario.onActivity { activity ->
                    assertEquals(FeedbackResult.Requested, activity.window.decorView.keyFeedback(KeyFeedback.LongPress, controller))
                }
                await { records() != beforeIntensityOff }
            }
        } finally {
            for ((key, value) in previous) shell(if (value == "null") "settings delete system $key" else "settings put system $key $value")
        }
    }

    @Test fun touchpadSettingsIconCancelsPendingClickAndDoesNotMoveMouse() {
        DeviceActivity().use { scenario ->
            val events = mutableListOf<String>(); var settings = 0
            lateinit var pad: TouchpadView
            scenario.onActivity { activity ->
                val sink = object : TouchSink {
                    override fun move(dx: Double, dy: Double) { events += "move" }
                    override fun scroll(dx: Double, dy: Double) { events += "scroll" }
                    override fun button(name: String, down: Boolean) { events += "$name:$down" }
                    override fun click(name: String) { events += "click" }
                }
                pad = TouchpadView(activity, sink).apply { connected = true; onSensitivitySettings = { settings++ } }
                activity.setContentView(pad)
            }
            val down = SystemClock.uptimeMillis()
            scenario.onActivity {
                for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                    val e = motion(down, action, listOf(pad.width / 2f to pad.height / 2f)); pad.dispatchTouchEvent(e); e.recycle()
                }
                assertTrue(events.isEmpty())
                val button = pad.findViewById<View>(R.id.touchpad_sensitivity_button)
                val point = (button.left + button.width / 2f) to (button.top + button.height / 2f)
                for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                    val e = motion(SystemClock.uptimeMillis(), action, listOf(point)); pad.dispatchTouchEvent(e); e.recycle()
                }
            }
            await { settings == 1 }
            SystemClock.sleep(350)
            assertTrue("settings icon emitted pointer input: $events", events.isEmpty())
        }
    }

    @Test fun shortcutFeedbackOncePerPressAndDisabledDoesNotTrigger() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val feedback = java.util.concurrent.CopyOnWriteArrayList<KeyFeedback>()
        val events = java.util.concurrent.CopyOnWriteArrayList<String>()
        val connected = mutableStateOf(true)
        val composedConnected = java.util.concurrent.atomic.AtomicBoolean(true)
        val stopRecording = mutableStateOf(false)
        val hold = ShortcutHold({ slot, _ -> events.add("down:$slot") }, { events.add("up") })
        DeviceActivity().use { scenario ->
            SystemClock.sleep(300)
            scenario.onActivity { activity ->
                val vm = ViewModelProvider(activity)[TapViewModel::class.java]
                vm.keyboardOn.value = false
                val viewport = activity.findViewById<View>(R.id.controller_regions)
                val scale = ControllerLayout.measure(viewport.width, viewport.height).scale
                activity.findViewById<ComposeView>(R.id.shortcut_region).setContent {
                    CompositionLocalProvider(LocalKeyFeedback provides { feedback.add(it) }) { MaterialTheme { CompactControls {
                        val online = connected.value
                        ShortcutButtons(ClientState(connected = online), hold, scale) { stopRecording.value }
                        androidx.compose.runtime.SideEffect { composedConnected.set(online) }
                    } } }
                }
            }
            var node: AccessibilityNodeInfo? = null
            await { node = instrumentation.uiAutomation.rootInActiveWindow?.let { root ->
                accessibilityNodes(root).firstOrNull { it.contentDescription?.startsWith("快捷键 1：") == true }
            }; node != null }
            val rect = Rect().also(node!!::getBoundsInScreen)
            fun press(duration: Long) {
                val down = SystemClock.uptimeMillis()
                for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                    val e = motion(down, action, listOf(rect.exactCenterX() to rect.exactCenterY()))
                    assertTrue(instrumentation.uiAutomation.injectInputEvent(e, true)); e.recycle()
                    if (action == MotionEvent.ACTION_DOWN) SystemClock.sleep(duration)
                }
                instrumentation.waitForIdleSync()
            }
            press(60); assertEquals(listOf(KeyFeedback.Press), feedback.toList())
            assertEquals(listOf("down:0", "up"), events.toList()); feedback.clear(); events.clear()
            press(650); assertEquals(listOf(KeyFeedback.Press), feedback.toList())
            assertEquals(listOf("down:0", "up"), events.toList()); feedback.clear(); events.clear()
            scenario.onActivity { stopRecording.value = true }
            press(60); assertEquals(listOf(KeyFeedback.Press), feedback.toList()); assertTrue(events.isEmpty()); feedback.clear()
            scenario.onActivity { connected.value = false }
            await { !composedConnected.get() }
            instrumentation.waitForIdleSync()
            press(60)
            assertTrue("disabled shortcut generated feedback: $feedback", feedback.isEmpty())
            assertTrue("disabled shortcut generated input: $events", events.isEmpty())
        }
    }

    private fun assertTouchpadLabelsFit(touch: TouchpadView) {
        val bitmap = Bitmap.createBitmap(touch.width, touch.height, Bitmap.Config.ARGB_8888)
        try {
            touch.draw(Canvas(bitmap))
            val pixels = IntArray(touch.width * touch.height)
            bitmap.getPixels(pixels, 0, touch.width, 0, 0, touch.width, touch.height)
            val button = touch.findViewById<View>(R.id.touchpad_sensitivity_button)
            val buttonBounds = Rect(button.left, button.top, button.right, button.bottom)
            assertTrue("settings button clipped", Rect(0, 0, touch.width, touch.height).contains(buttonBounds))
            // The new gray icon is not title text. Verify that hints do not occupy its target.
            for (y in buttonBounds.top until buttonBounds.bottom.coerceAtMost(touch.height - 2)) {
                for (x in buttonBounds.left until buttonBounds.right) {
                    val pixel = pixels[y * touch.width + x]
                    // Exclude the dark blue-gray divider at the bottom of this very short view.
                    assertFalse("hint overlaps settings button", Color.red(pixel) > 96 && Color.blue(pixel) > Color.red(pixel) + 2)
                }
            }
            fun rows(title: Boolean): Pair<Int, Int> {
                // Small-window text may consist entirely of antialiased pixels.
                // White title remains neutral; the blue-gray hint has B > R.
                fun isInk(index: Int): Boolean {
                    if (index / touch.width >= touch.height - 2) return false
                    if (buttonBounds.contains(index % touch.width, index / touch.width)) return false
                    val pixel = pixels[index]
                    val red = Color.red(pixel); val green = Color.green(pixel); val blue = Color.blue(pixel)
                    return if (title) red > 32 && red == green && green == blue else blue > 32 && blue > red + 2
                }
                val first = pixels.indices.firstOrNull { isInk(it) } ?: -1
                val last = pixels.indices.lastOrNull { isInk(it) } ?: -1
                assertTrue("touchpad label was not rendered", first >= 0 && last >= first)
                return first / touch.width to last / touch.width
            }
            val title = rows(true)
            val hint = rows(false)
            assertTrue("touchpad title clipped at top", title.first > 0)
            assertTrue("touchpad labels overlap", title.second < hint.first)
            assertTrue("touchpad hint clipped at bottom", hint.second < touch.height - 1)
        } finally { bitmap.recycle() }
    }

    @Test fun controllerModesAndShortcutVariants() {
        DeviceActivity().use { scenario ->
            SystemClock.sleep(500)
            var touchHeight = 0
            var panelTop = 0
            for (count in listOf(1, 4, 5, 8)) {
                scenario.onActivity { activity ->
                    val vm = ViewModelProvider(activity)[TapViewModel::class.java]
                    vm.keyboardOn.value = false
                    vm.selectedVoice.value = defaultVoiceProfiles()[if (count == 8) 0 else 1]
                    val slots = if (count == 5) setOf(0, 1, 2, 3, 7) else (0 until count).toSet()
                    uiState(activity).value = ClientState(peerName = "PC-77", connected = true, status = "已连接", rttMs = 5,
                        config = PcConfig(shortcuts = (0..7).map { Shortcut(if (it == 7) "很长的快捷键名称用于验证省略" else "快捷键 ${it + 1}", "Ctrl+F${it + 1}", it in slots) }))
                }
                SystemClock.sleep(200)
                var bounds = Rect()
                var shortcutScale = 1f
                scenario.onActivity { activity ->
                    val viewport = activity.findViewById<View>(R.id.controller_regions)
                    val m = ControllerLayout.measure(viewport.width, viewport.height)
                    val panel = activity.findViewById<ViewGroup>(R.id.control_panel)
                    val shortcut = activity.findViewById<View>(R.id.shortcut_region)
                    val touch = views(viewport).filterIsInstance<TouchpadView>().single()
                    val mic = views(viewport).filterIsInstance<MicBallView>().single()
                    assertEquals(m.touchpad, touch.height)
                    if (count == 1) {
                        assertTouchpadLabelsFit(touch)
                        touch.connected = false
                        try { assertTouchpadLabelsFit(touch) } finally { touch.connected = true }
                    }
                    assertEquals(m.panel, panel.height)
                    assertEquals(m.panelContent, shortcut.height + mic.height)
                    assertEquals(shortcut.height.toFloat(), mic.height.toFloat(), 1f)
                    assertEquals(touch.bottom, panel.top)
                    if (touchHeight == 0) { touchHeight = touch.height; panelTop = panel.top }
                    assertEquals(touchHeight, touch.height)
                    assertEquals(panelTop, panel.top)
                    val location = IntArray(2); shortcut.getLocationOnScreen(location)
                    bounds = Rect(location[0], location[1], location[0] + shortcut.width, location[1] + shortcut.height)
                    shortcutScale = m.scale
                    val g = VoiceGeometry(mic.width.toFloat(), mic.height.toFloat(), mic.width * m.scale)
                    val center = mic.ballCenter()
                    assertTrue(center.second - g.radius - g.halo >= g.modeHeight)
                    assertTrue(center.second + g.radius + g.halo <= g.footerTop)
                }
                await {
                    InstrumentationRegistry.getInstrumentation().uiAutomation.rootInActiveWindow?.let { root ->
                        accessibilityNodes(root).count { it.contentDescription?.startsWith("快捷键 ") == true } == count
                    } == true
                }
                val nodes = accessibilityNodes(InstrumentationRegistry.getInstrumentation().uiAutomation.rootInActiveWindow)
                    .filter { it.contentDescription?.startsWith("快捷键 ") == true }
                assertEquals(count, nodes.size)
                val rects = nodes.map { it.refresh(); Rect().also(it::getBoundsInScreen) }
                val expectedGap = bounds.width() * 0.01f
                val expectedVerticalGap = expectedGap * shortcutScale
                assertEquals("shortcut left margin", expectedGap, (rects.minOf { it.left } - bounds.left).toFloat(), 1.5f)
                assertEquals("shortcut right margin", expectedGap, (bounds.right - rects.maxOf { it.right }).toFloat(), 1.5f)
                assertEquals("shortcut bottom margin", expectedVerticalGap, (bounds.bottom - rects.maxOf { it.bottom }).toFloat(), 1.5f)
                val rowRects = rects.groupBy { it.top }.toSortedMap().values.map { row -> row.sortedBy { it.left } }
                for (row in rowRects) for ((left, right) in row.zipWithNext()) {
                    assertEquals("shortcut column gap", expectedGap, (right.left - left.right).toFloat(), 1.5f)
                }
                if (rowRects.size == 2) assertEquals("shortcut row gap", expectedVerticalGap,
                    (rowRects[1][0].top - rowRects[0][0].bottom).toFloat(), 1.5f)
                saveUiScreenshot("shortcuts-$count")
                assertEquals(if (count > 4) 2 else 1, rects.map { it.top }.distinct().size)
                rects.forEachIndexed { i, r ->
                    assertTrue("shortcut outside panel: $r / $bounds", bounds.contains(r))
                    for (j in 0 until i) assertFalse("$count shortcuts overlap: ${rects[j]} / $r", Rect.intersects(r, rects[j]))
                }
            }
            scenario.onActivity { activity -> ViewModelProvider(activity)[TapViewModel::class.java].keyboardOn.value = true }
            SystemClock.sleep(250)
            var keyboardBounds = Rect()
            var panelBounds = Rect()
            var keyboardScale = 1f
            scenario.onActivity { activity ->
                val viewport = activity.findViewById<View>(R.id.controller_regions)
                val keyboard = activity.findViewById<View>(R.id.keyboard_region)
                val panel = activity.findViewById<View>(R.id.control_panel)
                val m = ControllerLayout.measure(viewport.width, viewport.height)
                keyboardScale = m.scale
                assertEquals(m.panelContent, keyboard.height)
                assertEquals(panelTop, panel.top)
                assertEquals(touchHeight, views(viewport).filterIsInstance<TouchpadView>().single().height)
                val p = IntArray(2); keyboard.getLocationOnScreen(p)
                keyboardBounds = Rect(p[0], p[1], p[0] + keyboard.width, p[1] + keyboard.height)
                panel.getLocationOnScreen(p)
                panelBounds = Rect(p[0], p[1], p[0] + panel.width, p[1] + panel.height)
            }
            fun keyboardNodes(): List<AccessibilityNodeInfo> {
                val root = InstrumentationRegistry.getInstrumentation().uiAutomation.rootInActiveWindow ?: return emptyList()
                return accessibilityNodes(root).filter {
                    val d = it.contentDescription?.toString() ?: ""
                    d.contains("键，长按输入") || listOf("Shift：", "空格：", "退格：", "Ctrl：", "Shift+Enter：", "回车：").any { prefix -> d.startsWith(prefix) }
                }
            }
            await { keyboardNodes().size == 33 }
            val keys = keyboardNodes()
            assertEquals(33, keys.size)
            val rects = keys.map { Rect().also(it::getBoundsInScreen) }
            rects.forEachIndexed { i, r ->
                assertTrue("key outside keyboard: $r / $keyboardBounds", keyboardBounds.contains(r))
                for (j in 0 until i) assertFalse("keys overlap", Rect.intersects(r, rects[j]))
            }
            val w = keyboardBounds.width().toDouble()
            val rows = rects.groupBy { it.top }.toSortedMap().values.map { row -> row.sortedBy { it.left } }
            assertEquals(listOf(10, 9, 9, 5), rows.map { it.size })
            val widths = listOf(
                List(10) { 0.089 }, List(9) { 0.089 },
                listOf(0.1385) + List(7) { 0.089 } + 0.1385,
                listOf(0.188, 0.089, 0.3365, 0.1385, 0.188),
            )
            rows.forEachIndexed { rowIndex, row ->
                row.forEachIndexed { index, key ->
                    assertEquals("key width $rowIndex/$index", w * widths[rowIndex][index], key.width().toDouble(), 2.0)
                    assertEquals(row.first().bottom, key.bottom)
                    if (index > 0) assertEquals("horizontal gap", w * 0.01, (key.left - row[index - 1].right).toDouble(), 2.0)
                }
                val left = row.first().left - keyboardBounds.left
                val right = keyboardBounds.right - row.last().right
                assertEquals("centered row", left.toDouble(), right.toDouble(), 1.0)
                assertEquals("side margin", w * if (rowIndex == 1) 0.0595 else 0.01, left.toDouble(), 2.0)
                if (rowIndex > 0) assertEquals("vertical gap", w * 0.01 * keyboardScale, (row.first().top - rows[rowIndex - 1].first().bottom).toDouble(), 2.0)
            }
            assertEquals(keyboardBounds.top, rows.first().first().top)
            assertEquals(keyboardBounds.bottom, rows.last().first().bottom)
            assertEquals("top outer margin", w * 0.01 * keyboardScale, (keyboardBounds.top - panelBounds.top).toDouble(), 1.0)
            assertEquals("bottom outer margin", w * 0.01 * keyboardScale, (panelBounds.bottom - keyboardBounds.bottom).toDouble(), 1.0)
            saveUiScreenshot("keyboard")
        }
    }

    @Test fun launcherIconUsesPcColorsAndSafeAdaptiveLayers() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val info = context.packageManager.getApplicationInfo(context.packageName, 0)
        // PackageManager can select roundIcon according to the system's icon mask.
        assertTrue(info.icon == R.mipmap.ic_launcher || info.icon == R.mipmap.ic_launcher_round)
        val icon = context.packageManager.getApplicationIcon(info) as android.graphics.drawable.AdaptiveIconDrawable
        val normal = context.getDrawable(R.mipmap.ic_launcher) as android.graphics.drawable.AdaptiveIconDrawable
        val round = context.getDrawable(R.mipmap.ic_launcher_round) as android.graphics.drawable.AdaptiveIconDrawable
        val blue = Color.rgb(23, 92, 211)
        fun layer(drawable: android.graphics.drawable.Drawable): Bitmap = Bitmap.createBitmap(108, 108, Bitmap.Config.ARGB_8888).also {
            drawable.setBounds(0, 0, 108, 108); drawable.draw(Canvas(it))
        }
        for (adaptive in listOf(normal, round)) {
            val background = layer(adaptive.background)
            val foreground = layer(adaptive.foreground)
            try {
                assertEquals(blue, background.getPixel(0, 0))
                assertEquals(blue, background.getPixel(107, 107))
                var white = 0
                for (y in 0..107) for (x in 0..107) {
                    val color = foreground.getPixel(x, y)
                    if (x in 30..77 && y in 30..77) { assertEquals(Color.WHITE, color); white++ }
                    else assertEquals("foreground outside safe square", 0, Color.alpha(color))
                }
                assertEquals(48 * 48, white)
            } finally { background.recycle(); foreground.recycle() }
        }
        if (android.os.Build.VERSION.SDK_INT >= 33) for (adaptive in listOf(normal, round)) {
            val mono = layer(checkNotNull(adaptive.monochrome))
            try { assertEquals(Color.WHITE, mono.getPixel(54, 54)); assertEquals(0, Color.alpha(mono.getPixel(0, 0))) }
            finally { mono.recycle() }
        }
        val size = 256; val gap = 16
        val preview = Bitmap.createBitmap(size * 4 + gap * 5, size + 72, Bitmap.Config.ARGB_8888)
        try {
            val canvas = Canvas(preview); canvas.drawColor(Color.rgb(238, 238, 238))
            val labels = listOf("System", "Circle", "Rounded", "Monochrome")
            val paint = android.graphics.Paint(android.graphics.Paint.ANTI_ALIAS_FLAG).apply { color = Color.DKGRAY; textSize = 22f }
            labels.forEachIndexed { index, label ->
                val x = gap + index * (size + gap)
                canvas.drawText(label, x.toFloat(), 27f, paint)
                val saved = canvas.save(); canvas.translate(x.toFloat(), 48f)
                if (index == 0) {
                    icon.setBounds(0, 0, size, size); icon.draw(canvas)
                } else {
                    val mask = android.graphics.Path().apply {
                        if (index == 2) addRoundRect(0f, 0f, size.toFloat(), size.toFloat(), 64f, 64f, android.graphics.Path.Direction.CW)
                        else addCircle(size / 2f, size / 2f, size / 2f, android.graphics.Path.Direction.CW)
                    }
                    canvas.clipPath(mask)
                    canvas.drawColor(if (index == 3) Color.rgb(206, 216, 237) else blue)
                    canvas.scale(size / 72f, size / 72f); canvas.translate(-18f, -18f)
                    val foreground = (if (index == 3 && android.os.Build.VERSION.SDK_INT >= 33) icon.monochrome else icon.foreground)!!.mutate()
                    if (index == 3) foreground.setTint(Color.rgb(30, 47, 77))
                    foreground.setBounds(0, 0, 108, 108); foreground.draw(canvas)
                }
                canvas.restoreToCount(saved)
            }
            val label = InstrumentationRegistry.getArguments().getString("uiLabel") ?: "device"
            val directory = File(context.getExternalFilesDir(null), "ui-validation").apply { mkdirs() }
            File(directory, "$label-launcher-icon-masks.png").outputStream().use { preview.compress(Bitmap.CompressFormat.PNG, 100, it) }
        } finally { preview.recycle() }
    }

    @Test fun connectionIconReminderAndSettings() {
        val automation = InstrumentationRegistry.getInstrumentation().uiAutomation
        DeviceActivity().use { scenario ->
            SystemClock.sleep(500)
            var region = Rect()
            scenario.onActivity { activity ->
                uiState(activity).value = ClientState()
                val header = activity.findViewById<View>(R.id.connection_header)
                val p = IntArray(2); header.getLocationOnScreen(p)
                region = Rect(p[0] + (header.width * 0.875f).toInt(), p[1], p[0] + header.width, p[1] + header.height)
            }
            fun iconCenter(connected: Boolean): Float {
                val b = automation.takeScreenshot() ?: error("Screenshot unavailable")
                val target = if (connected) ControllerStyle.CONNECTED else ControllerStyle.DISCONNECTED
                var sum = 0L; var count = 0
                for (y in region.top until region.bottom) for (x in region.left until region.right) {
                    val c = b.getPixel(x, y)
                    if (kotlin.math.abs(Color.red(c) - Color.red(target)) < 12 &&
                        kotlin.math.abs(Color.green(c) - Color.green(target)) < 12 &&
                        kotlin.math.abs(Color.blue(c) - Color.blue(target)) < 12) { sum += y; count++ }
                }
                b.recycle()
                assertTrue("missing ${if (connected) "green" else "orange"} connection icon", count > 5)
                return sum.toFloat() / count
            }
            val centers = mutableListOf<Float>()
            // A tablet screenshot can take longer than one animation frame. Observe up
            // to two reminder cycles so a slow capture does not miss the whole jump.
            val deadline = SystemClock.elapsedRealtime() + 7000
            while (SystemClock.elapsedRealtime() < deadline) {
                centers += iconCenter(false)
                if (centers.max() - centers.min() > 2f) break
                SystemClock.sleep(45)
            }
            assertTrue("disconnected icon did not bounce: $centers", centers.max() - centers.min() > 2f)
            scenario.onActivity { activity -> uiState(activity).value = ClientState(connected = true, status = "已连接") }
            SystemClock.sleep(150)
            val green = iconCenter(true)
            SystemClock.sleep(500)
            assertEquals(green, iconCenter(true), 0.1f)
            assertEquals(centers.max(), green, 1f)
            val button = accessibilityNodes(automation.rootInActiveWindow).single { it.contentDescription?.startsWith("连接设置") == true }
            assertTrue(button.performAction(AccessibilityNodeInfo.ACTION_CLICK))
            SystemClock.sleep(250)
            await { automation.rootInActiveWindow?.let { root -> accessibilityNodes(root).any { it.text?.toString() == "连接与设置" } } == true }
            InstrumentationRegistry.getInstrumentation().sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
            scenario.moveToState(androidx.lifecycle.Lifecycle.State.CREATED)
            scenario.moveToState(androidx.lifecycle.Lifecycle.State.RESUMED)
        }
    }

    private class RecordingSocket : okhttp3.WebSocket {
        val messages = java.util.concurrent.CopyOnWriteArrayList<JsonObject>()
        override fun request() = okhttp3.Request.Builder().url("http://127.0.0.1/").build()
        override fun queueSize() = 0L
        override fun send(text: String): Boolean { messages += Json.parseToJsonElement(text).jsonObject; return true }
        override fun send(bytes: okio.ByteString) = true
        override fun close(code: Int, reason: String?) = true
        override fun cancel() {}
    }

    private fun withVoiceTestClient(block: (TapClient, RecordingSocket, MutableStateFlow<ClientState>) -> Unit) {
        val scope = kotlinx.coroutines.CoroutineScope(kotlinx.coroutines.SupervisorJob() + kotlinx.coroutines.Dispatchers.Default)
        val app = InstrumentationRegistry.getInstrumentation().targetContext.applicationContext as android.app.Application
        val client = TapClient(app, scope)
        val socket = RecordingSocket()
        client.javaClass.getDeclaredField("socket").apply { isAccessible = true }.set(client, socket)
        client.javaClass.getDeclaredField("connected").apply { isAccessible = true }.setBoolean(client, true)
        client.setForeground(true)
        @Suppress("UNCHECKED_CAST")
        val state = client.javaClass.getDeclaredField("mutable").apply { isAccessible = true }.get(client) as MutableStateFlow<ClientState>
        try { block(client, socket, state) } finally {
            client.close()
            scope.coroutineContext[kotlinx.coroutines.Job]?.cancel()
        }
    }

    @Test fun pairingRejectionStopsRetryAndIgnoresStaleFailure() {
        for (code in listOf("pairing_rejected", "pairing_expired")) withVoiceTestClient { client, _, state ->
            fun field(name: String) = client.javaClass.getDeclaredField(name).apply { isAccessible = true }
            field("connected").setBoolean(client, false)
            field("peer").set(client, Peer("127.0.0.1", 41443, 41080, "a".repeat(64)))
            state.value = state.value.copy(connected = false, pairing = "ABCD 1234", pairingConfirmed = true)
            val generation = field("generation").getLong(client)
            val before = state.value
            assertFalse(client.finishPairingAttempt(generation, "touchpad_error", "无关错误"))
            assertTrue(client.finishPairingAttempt(generation - 1, code, "陈旧请求"))
            assertEquals(before, state.value)
            assertTrue(client.finishPairingAttempt(generation, code, "请重新点击连接"))
            assertEquals("", state.value.pairing)
            assertFalse(state.value.pairingConfirmed)
            assertEquals("请重新点击连接", state.value.error)
            assertFalse(field("reconnect").getBoolean(client))
            assertNull(field("peer").get(client))
            val after = state.value
            client.javaClass.getDeclaredMethod("lost", java.lang.Long.TYPE, String::class.java).apply { isAccessible = true }
                .invoke(client, generation, "旧连接关闭")
            client.setForeground(false); client.setForeground(true)
            assertEquals(after, state.value)
            assertFalse((field("reconnectJob").get(client) as? kotlinx.coroutines.Job)?.isActive == true)
        }
    }

    @Test fun toggleVoiceStopsAndConsumesTouchpadClick() {
        withVoiceTestClient { client, socket, state ->
            fun types() = socket.messages.map { it.str("type") }
            for (phase in listOf("preparing", "transmitting")) {
                assertTrue(client.startMic("toggle"))
                val id = socket.messages.last().str("recording")
                state.value = state.value.copy(mic = phase, level = 0.8f)
                socket.messages.clear()
                client.click("left")
                assertEquals(listOf("mic_stop"), types())
                assertEquals(id, socket.messages.single().str("recording"))
                assertEquals("stopping", state.value.mic)
                assertEquals(0f, state.value.level)
                client.click("right")
                assertEquals("clicks while draining must not interrupt the input method", listOf("mic_stop"), types())
                client.finishMic("old-recording")
                assertEquals("stopping", state.value.mic)
                client.finishMic(id)
                assertEquals("idle", state.value.mic)
                assertEquals("", state.value.micMode)
                socket.messages.clear()
                client.click("left")
                assertEquals(listOf("mouse_button", "mouse_button"), types())
                assertTrue(socket.messages[0]["down"]!!.jsonPrimitive.boolean)
                assertFalse(socket.messages[1]["down"]!!.jsonPrimitive.boolean)
            }
            assertTrue(client.startMic("toggle"))
            val id = socket.messages.last().str("recording")
            socket.messages.clear()
            client.button("left", true)
            client.finishMic(id)
            client.button("left", true) // A repeated down cannot escape a consumed press.
            client.button("left", false)
            assertEquals(listOf("mic_stop"), types())
            assertTrue(client.startMic("hold"))
            val holdId = socket.messages.last().str("recording")
            socket.messages.clear()
            client.click("left")
            assertEquals("hold-to-talk must still allow mouse input", listOf("mouse_button", "mouse_button"), types())
            assertEquals("preparing", state.value.mic)
            client.finishMic(id)
            assertEquals("old stop cannot end a new recording", "preparing", state.value.mic)
            client.finishMic(holdId)
        }
    }

    @Test fun microphoneFramesRetainTheirOriginalSessionAndRecording() {
        withVoiceTestClient { client, socket, state ->
            assertTrue(client.startMic("hold"))
            val id = socket.messages.last().str("recording")
            val generation = client.javaClass.getDeclaredField("generation").apply { isAccessible = true }.getLong(client)
            client.microphoneFrame(generation - 1, id, ByteArray(960), 0, 0.8f)
            client.microphoneFrame(generation, "0000000000000001", ByteArray(960), 0, 0.8f)
            assertEquals(0f, state.value.level)
            client.microphoneFrame(generation, id, ByteArray(960), 0, 0.4f)
            assertEquals(0.4f, state.value.level)
            client.finishMic(id)
            client.microphoneFrame(generation, id, ByteArray(960), 0, 0.8f)
            assertEquals(0f, state.value.level)
        }
    }

    @Test fun voiceProfilesWireSnapshotAndSpaceGesture() {
        withVoiceTestClient { client, socket, state ->
            val profiles = defaultVoiceProfiles().map { it.copy(enabled = true) }
            state.value = state.value.copy(voiceProfilesSupported = true, config = PcConfig(revision = 17, voice = Voice(profiles = profiles)))
            // A held space uses the toggle profile's keys but remains a momentary phone gesture.
            assertTrue(client.startMic("hold", "voice-1"))
            val start = socket.messages.last()
            val id = start.str("recording")
            assertEquals("voice-1", start.str("profile_id")); assertEquals(17L, start.long("revision"))
            assertEquals("toggle", start.str("mode")); assertEquals("hold", state.value.micMode)
            socket.messages.clear()
            client.click("left")
            assertEquals(listOf("mouse_button", "mouse_button"), socket.messages.map { it.str("type") })
            state.value = state.value.copy(config = PcConfig(revision = 18, voice = Voice(profiles = profiles.map { it.copy(enabled = false, name = "改名", mode = "hold") })))
            assertEquals(profiles[0], state.value.activeVoiceProfile)
            client.stopMic()
            assertEquals("mic_stop", socket.messages.last().str("type"))
            assertEquals(id, socket.messages.last().str("recording"))
            assertFalse(client.startMic("hold", "voice-3"))
            client.finishMic("old-id")
            assertEquals("stopping", state.value.mic)
            client.finishMic(id)
            assertNull(state.value.activeVoiceProfile)
            assertFalse(client.startMic("hold", "voice-1"))
            state.value = state.value.copy(config = PcConfig(revision = 19, voice = Voice(profiles = profiles)))
            assertTrue(client.startMic("hold", "voice-3"))
            assertEquals("voice-3", socket.messages.last().str("profile_id"))
            client.finishMic(socket.messages.last().str("recording"), "语音配置已更新，请重试")
            assertEquals("idle", state.value.mic)
            // Older PC receives its original mode-only request.
            state.value = state.value.copy(voiceProfilesSupported = false, config = PcConfig())
            assertTrue(client.startMic("hold", "voice-1"))
            assertEquals("toggle", socket.messages.last().str("mode"))
            assertFalse(socket.messages.last().containsKey("profile_id"))
            assertFalse(socket.messages.last().containsKey("revision"))
            client.finishMic(socket.messages.last().str("recording"))
        }
    }

    @Test fun voiceProfileButtonsCycleDisableAndFreeze() {
        val store = PairStore(InstrumentationRegistry.getInstrumentation().targetContext)
        val originalCatalog = kotlinx.coroutines.runBlocking { store.loadCatalog() }
        val voicePeer = Peer("127.0.0.1", 41443, 41080, "ee".repeat(32), "语音测试电脑")
        kotlinx.coroutines.runBlocking { store.updateCatalog { it.upsert(voicePeer).select(voicePeer.id) } }
        try {
        val feedback = mutableListOf<KeyFeedback>()
        DeviceActivity().use { scenario ->
            SystemClock.sleep(500)
            lateinit var vm: TapViewModel
            lateinit var mic: MicBallView
            val all = defaultVoiceProfiles().map { it.copy(enabled = true) }
            scenario.onActivity { activity ->
                vm = ViewModelProvider(activity)[TapViewModel::class.java]
                vm.keyboardOn.value = false
                uiState(activity).value = ClientState(connected = true, selectedPeerId = voicePeer.id, status = "已连接", voiceProfilesSupported = true, config = PcConfig(voice = Voice(profiles = all)))
                mic = views(activity.window.decorView).filterIsInstance<MicBallView>().single()
                mic.onFeedback = { feedback.add(it) }
            }
            SystemClock.sleep(200)
            fun tapSwitch() = scenario.onActivity {
                val button = mic.javaClass.getDeclaredField("modeButton").apply { isAccessible = true }.get(mic) as android.graphics.RectF
                val time = SystemClock.uptimeMillis()
                for (action in listOf(MotionEvent.ACTION_DOWN, MotionEvent.ACTION_UP)) {
                    val event = MotionEvent.obtain(time, time + 20, action, button.centerX(), button.centerY(), 0)
                    mic.dispatchTouchEvent(event); event.recycle()
                }
            }
            val first = vm.selectedVoice.value!!.id
            val order = all.map { it.id }
            repeat(3) { index ->
                tapSwitch(); SystemClock.sleep(80)
                assertEquals(order[(order.indexOf(first) + index + 1) % 3], vm.selectedVoice.value!!.id)
                if (vm.selectedVoice.value!!.id != "voice-3") saveUiScreenshot("voice-${vm.selectedVoice.value!!.mode}-idle")
            }
            assertEquals(3, feedback.size)
            scenario.onActivity { activity ->
                uiState(activity).value = uiState(activity).value.copy(config = PcConfig(voice = Voice(profiles = all.map { it.copy(enabled = it.id != "voice-3") })))
            }
            SystemClock.sleep(100)
            val twoGroupStart = vm.selectedVoice.value!!.id
            repeat(2) { tapSwitch(); SystemClock.sleep(80) }
            assertEquals(twoGroupStart, vm.selectedVoice.value!!.id)
            assertEquals(5, feedback.size)
            for (profile in all.take(2)) for (phase in listOf("preparing", "transmitting", "stopping")) {
                scenario.onActivity { activity ->
                    val current = uiState(activity).value
                    uiState(activity).value = current.copy(mic = phase, micMode = profile.mode, activeVoiceProfile = profile, level = if (phase == "transmitting") 0.42f else 0f, config = current.config.copy(voice = Voice(profiles = all.map { it.copy(enabled = false) })))
                }
                SystemClock.sleep(80)
                assertEquals(profile.name, mic.profileName)
                assertEquals(profile.mode, mic.gestureMode)
                assertTrue(mic.available)
                assertFalse(mic.switchAvailable)
                tapSwitch(); assertEquals(5, feedback.size)
                saveUiScreenshot("voice-${profile.mode}-$phase")
            }
            scenario.onActivity { activity -> uiState(activity).value = uiState(activity).value.copy(mic = "idle", activeVoiceProfile = null) }
            SystemClock.sleep(120)
            assertEquals("语音未启用", mic.profileName)
            assertFalse(mic.available); assertFalse(mic.switchAvailable)
            tapSwitch(); assertEquals(5, feedback.size)
            saveUiScreenshot("voice-disabled")
            scenario.onActivity { activity ->
                uiState(activity).value = uiState(activity).value.copy(config = PcConfig(voice = Voice(profiles = all.map { it.copy(enabled = it.id == "voice-3", name = "WWWWWWWWWWWWWWWW") })))
            }
            SystemClock.sleep(120)
            assertEquals("voice-3", vm.selectedVoice.value!!.id)
            assertTrue(mic.available); assertFalse(mic.switchAvailable)
            tapSwitch(); assertEquals(5, feedback.size)
            scenario.onActivity { activity ->
                val button = mic.javaClass.getDeclaredField("modeButton").apply { isAccessible = true }.get(mic) as android.graphics.RectF
                assertTrue(button.left >= 0 && button.right <= mic.width)
            }
            saveUiScreenshot("voice-single-long-name")
            scenario.onActivity { activity ->
                uiState(activity).value = uiState(activity).value.copy(config = PcConfig(voice = Voice(profiles = all.map { it.copy(name = "中".repeat(8)) })))
            }
            SystemClock.sleep(120)
            assertTrue(mic.switchAvailable)
            saveUiScreenshot("voice-three-long-name")
            await { kotlinx.coroutines.runBlocking { PairStore(InstrumentationRegistry.getInstrumentation().targetContext).loadVoiceSelection().id } == "voice-3" }
        }
        DeviceActivity().use { scenario ->
            SystemClock.sleep(300)
            scenario.onActivity { activity ->
                uiState(activity).value = ClientState(connected = true, selectedPeerId = voicePeer.id, voiceProfilesSupported = true, config = PcConfig(voice = Voice(profiles = defaultVoiceProfiles().map { it.copy(enabled = true) })))
            }
            SystemClock.sleep(150)
            scenario.onActivity { activity -> assertEquals("voice-3", ViewModelProvider(activity)[TapViewModel::class.java].selectedVoice.value?.id) }
        }
        } finally { kotlinx.coroutines.runBlocking { store.updateCatalog { originalCatalog } } }
    }

    @Test fun pcVoiceStopReleasesRecorderAndIgnoresStaleReplies() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        instrumentation.uiAutomation.grantRuntimePermission(instrumentation.targetContext.packageName, android.Manifest.permission.RECORD_AUDIO)
        DeviceActivity().use {
            withVoiceTestClient { client, socket, state ->
                assertTrue(client.startMic("toggle"))
                val id = socket.messages.last().str("recording")
                // Exercise the real AudioRecord without creating a UDP audio session.
                client.javaClass.getDeclaredField("recordingRequested").apply { isAccessible = true }.setBoolean(client, false)
                val recorder = client.javaClass.getDeclaredField("recorder").apply { isAccessible = true }.get(client) as MicCapture
                recorder.start()
                val thread = recorder.javaClass.getDeclaredField("thread").apply { isAccessible = true }.get(recorder) as Thread
                state.value = state.value.copy(mic = "transmitting", level = 0.8f)
                assertTrue(thread.isAlive)
                client.finishMic("stale-recording")
                assertTrue(thread.isAlive)
                assertEquals("transmitting", state.value.mic)
                client.finishMic(id)
                await { !thread.isAlive }
                assertEquals("idle", state.value.mic)
                assertEquals("", state.value.micMode)
                assertEquals(0f, state.value.level)
                assertTrue(client.startMic("toggle"))
                val newId = socket.messages.last().str("recording")
                client.finishMic(id)
                assertEquals("preparing", state.value.mic)
                client.finishMic(newId, "测试录音错误")
                assertEquals("idle", state.value.mic)
                assertEquals("测试录音错误", state.value.error)
            }
        }
    }

    @Test fun pairingScanFillsWithoutConnectingAndPreservesAddressOnCancel() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val automation = instrumentation.uiAutomation
        val context = instrumentation.targetContext
        assumeTrue(context.packageManager.hasSystemFeature(android.content.pm.PackageManager.FEATURE_CAMERA_ANY))
        automation.grantRuntimePermission(context.packageName, android.Manifest.permission.CAMERA)
        val store = PairStore(context)
        val originalCatalog = kotlinx.coroutines.runBlocking { store.loadCatalog() }
        kotlinx.coroutines.runBlocking { store.clear() }
        var result = android.app.Instrumentation.ActivityResult(android.app.Activity.RESULT_OK,
            Intent().putExtra(com.google.zxing.client.android.Intents.Scan.RESULT, "http://10.23.45.67:41080/pair"))
        var scans = 0
        var openedProject = false
        val monitor = object : android.app.Instrumentation.ActivityMonitor() {
            override fun onStartActivity(intent: Intent): android.app.Instrumentation.ActivityResult? {
                if (intent.action == Intent.ACTION_VIEW && intent.dataString == "https://github.com/fly3457/TapDeck") {
                    openedProject = true
                    return android.app.Instrumentation.ActivityResult(android.app.Activity.RESULT_CANCELED, null)
                }
                if (intent.component?.className != "com.journeyapps.barcodescanner.CaptureActivity") return null
                scans++
                return result
            }
        }
        instrumentation.addMonitor(monitor)
        try {
            DeviceActivity().use { scenario ->
                lateinit var client: TapClient
                scenario.onActivity { activity -> client = ViewModelProvider(activity)[TapViewModel::class.java].client }
                fun nodes() = automation.rootInActiveWindow?.let(::accessibilityNodes).orEmpty()
                await { nodes().any { it.contentDescription?.startsWith("连接设置") == true } }
                assertTrue(nodes().single { it.contentDescription?.startsWith("连接设置") == true }.performAction(AccessibilityNodeInfo.ACTION_CLICK))
                await { nodes().any { it.text?.toString() == "github.com/fly3457/TapDeck" } }
                val link = nodes().single { it.text?.toString() == "github.com/fly3457/TapDeck" }
                assertTrue(generateSequence(link) { it.parent }.first { it.isClickable }.performAction(AccessibilityNodeInfo.ACTION_CLICK))
                await { openedProject }
                fun scan() {
                    var button: AccessibilityNodeInfo? = null
                    await {
                        button = nodes().firstOrNull { it.contentDescription?.toString() == "扫码填写 PC 配对网址" }
                        if (button == null) {
                            nodes().firstOrNull { it.isScrollable }?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_FORWARD)
                            instrumentation.waitForIdleSync()
                        }
                        button != null
                    }
                    val heading = nodes().single { it.text?.toString() == "输入PC连接窗口URL" }
                    val headingBounds = Rect().also { heading.getBoundsInScreen(it) }
                    val scanBounds = Rect().also { button!!.getBoundsInScreen(it) }
                    assertTrue("scanner must share the heading row", kotlin.math.abs(headingBounds.centerY() - scanBounds.centerY()) < scanBounds.height()/2)
                    assertTrue("scanner must follow heading", headingBounds.right <= scanBounds.left)
                    // Accessibility activation is stable while the scroll animation settles.
                    val action = generateSequence(button!!) { it.parent }.first { it.isClickable }
                    assertTrue(action.performAction(AccessibilityNodeInfo.ACTION_CLICK))
                    instrumentation.waitForIdleSync()
                }
                fun address() = nodes().single { it.isEditable }.text.toString()
                val before = client.state.value
                scan()
                await { address() == "http://10.23.45.67:41080/pair" }
                assertEquals("scanning must not start a connection", before, client.state.value)
                assertTrue(nodes().any { it.text?.toString() == "连接与设置" })
                result = android.app.Instrumentation.ActivityResult(android.app.Activity.RESULT_CANCELED, null)
                scan()
                await { scans == 2 }
                instrumentation.waitForIdleSync()
                assertEquals("http://10.23.45.67:41080/pair", address())
                result = android.app.Instrumentation.ActivityResult(android.app.Activity.RESULT_OK,
                    Intent().putExtra(com.google.zxing.client.android.Intents.Scan.RESULT, "https://example.com"))
                scan()
                await {
                    val visible = nodes().any { it.text?.startsWith("未识别到 PC 配对网址") == true }
                    if (!visible) {
                        nodes().firstOrNull { it.isScrollable }?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_FORWARD)
                        instrumentation.waitForIdleSync()
                    }
                    visible
                }
                assertEquals("http://10.23.45.67:41080/pair", address())
                assertEquals(before, client.state.value)
                saveUiScreenshot("pairing-scan-invalid")
                result = android.app.Instrumentation.ActivityResult(android.app.Activity.RESULT_OK,
                    Intent().putExtra(com.google.zxing.client.android.Intents.Scan.RESULT, "http://192.168.1.25:52080/pair"))
                scan()
                await { address() == "http://192.168.1.25:52080/pair" }
                assertFalse(nodes().any { it.text?.startsWith("未识别到 PC 配对网址") == true })
                assertEquals(before, client.state.value)
                saveUiScreenshot("pairing-scan-filled")
                // The management entry replaces the old single-computer forget action.
                scenario.onActivity { activity -> uiState(activity).value = before.copy(connected = true) }
                await {
                    val visible = nodes().any { it.text?.toString()?.startsWith("管理电脑（") == true }
                    if (!visible) {
                        nodes().firstOrNull { it.isScrollable }?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD)
                        instrumentation.waitForIdleSync()
                    }
                    visible
                }
                saveUiScreenshot("pairing-connected-actions")
            }
        } finally {
            instrumentation.removeMonitor(monitor)
            kotlinx.coroutines.runBlocking { store.updateCatalog { originalCatalog } }
        }
    }

    @Test fun keyboardShortLongVoiceAndDisposalRelease() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val events = java.util.Collections.synchronizedList(mutableListOf<String>())
        val feedback = java.util.Collections.synchronizedList(mutableListOf<KeyFeedback>())
        val hold = KeyHold({ events += "down $it" }, { events += "up $it" })
        val shown = mutableStateOf(true)
        val voice = mutableStateOf(false)
        var voiceStarts = 0; var voiceStops = 0
        DeviceActivity().use { scenario ->
            SystemClock.sleep(500)
            scenario.onActivity { activity -> ViewModelProvider(activity)[TapViewModel::class.java].keyboardOn.value = true }
            SystemClock.sleep(150)
            scenario.onActivity { activity ->
                val viewport = activity.findViewById<View>(R.id.controller_regions)
                val scale = ControllerLayout.measure(viewport.width, viewport.height).scale
                activity.findViewById<ComposeView>(R.id.keyboard_region).setContent {
                    CompositionLocalProvider(LocalKeyFeedback provides { feedback.add(it) }) { MaterialTheme { CompactControls {
                        if (shown.value) KeyboardView(connected = true, voiceActive = voice.value, scale = scale, hold = hold,
                            beginVoice = { voiceStarts++; voice.value = true; true },
                            stopVoice = { voiceStops++; voice.value = false })
                    } } }
                }
            }
            fun point(prefix: String): Pair<Float, Float> {
                var found: AccessibilityNodeInfo? = null
                await {
                    found = instrumentation.uiAutomation.rootInActiveWindow?.let { accessibilityNodes(it).firstOrNull { node -> node.contentDescription?.startsWith(prefix) == true } }
                    found != null
                }
                found!!.refresh()
                val rect = Rect().also(found!!::getBoundsInScreen)
                return rect.exactCenterX() to rect.exactCenterY()
            }
            fun inject(action: Int, p: Pair<Float, Float>, down: Long) {
                val e = motion(down, action, listOf(p)); assertTrue(instrumentation.uiAutomation.injectInputEvent(e, true)); e.recycle()
            }
            fun press(prefix: String, duration: Long) {
                val p = point(prefix); val down = SystemClock.uptimeMillis()
                val before = feedback.size
                inject(MotionEvent.ACTION_DOWN, p, down); SystemClock.sleep(duration); inject(MotionEvent.ACTION_UP, p, down)
                instrumentation.waitForIdleSync()
                assertEquals("wrong haptics for $prefix", before + if (duration >= KeyHold.LONG_PRESS_MS) 2 else 1, feedback.size)
            }
            press("Q 键", 70)
            assertEquals(listOf(KeyFeedback.Press), feedback.toList()); feedback.clear()
            assertEquals(listOf("down Q", "up Q"), events.toList()); events.clear()
            press("Q 键", 550)
            assertEquals(listOf(KeyFeedback.Press, KeyFeedback.LongPress), feedback.toList()); feedback.clear()
            assertEquals(listOf("down 1", "up 1"), events.toList()); events.clear()
            press("空格：", 70)
            assertEquals(listOf(KeyFeedback.Press), feedback.toList()); feedback.clear()
            assertEquals(listOf("down Space", "up Space"), events.toList()); events.clear()
            press("空格：", 550)
            assertEquals(listOf(KeyFeedback.Press, KeyFeedback.LongPress), feedback.toList()); feedback.clear()
            assertTrue(events.isEmpty()); assertEquals(1, voiceStarts); assertEquals(1, voiceStops)
            press("Ctrl：", 70)
            assertEquals(listOf("down LeftCtrl", "up LeftCtrl"), events.toList()); events.clear()
            press("Shift+Enter：", 70)
            assertEquals(listOf("down LeftShift+Enter", "up LeftShift+Enter"), events.toList()); events.clear()
            press("回车：", 70)
            assertEquals(listOf("down Enter", "up Enter"), events.toList()); events.clear()
            press("Shift：", 70)
            press("Q 键", 70)
            assertEquals(listOf("down LeftShift", "down LeftShift+Q", "up LeftShift+Q", "up LeftShift"), events.toList()); events.clear()
            val ctrl = point("Ctrl："); val letter = point("C 键")
            val chordDown = SystemClock.uptimeMillis()
            inject(MotionEvent.ACTION_DOWN, ctrl, chordDown); SystemClock.sleep(550)
            assertEquals(listOf("down LeftCtrl"), events.toList())
            fun multi(action: Int) {
                val event = motion(chordDown, action, listOf(ctrl, letter))
                assertTrue(instrumentation.uiAutomation.injectInputEvent(event, true)); event.recycle()
            }
            multi(MotionEvent.ACTION_POINTER_DOWN or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT)); SystemClock.sleep(70)
            multi(MotionEvent.ACTION_POINTER_UP or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT))
            instrumentation.waitForIdleSync()
            assertEquals(listOf("down LeftCtrl", "down C", "up C"), events.toList())
            inject(MotionEvent.ACTION_UP, ctrl, chordDown)
            instrumentation.waitForIdleSync()
            assertEquals(listOf("down LeftCtrl", "down C", "up C", "up LeftCtrl"), events.toList()); events.clear()
            val p = point("退格："); val down = SystemClock.uptimeMillis()
            inject(MotionEvent.ACTION_DOWN, p, down); SystemClock.sleep(550)
            assertEquals(listOf("down Backspace"), events.toList())
            scenario.onActivity { shown.value = false }
            await { events.toList() == listOf("down Backspace", "up Backspace") }
            inject(MotionEvent.ACTION_UP, p, down)
            SystemClock.sleep(450)
            assertEquals(2, events.size)
        }
    }

    @Test fun compactVoiceAndTouchpadPointersStayIndependent() {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val moved = AtomicInteger(); val scrolled = AtomicInteger(); val began = AtomicInteger(); val ended = AtomicInteger()
        val zoomed = AtomicInteger(); val swiped = AtomicInteger()
        val feedback = java.util.concurrent.CopyOnWriteArrayList<KeyFeedback>()
        val sink = object : TouchSink {
            override fun move(dx: Double, dy: Double) { moved.incrementAndGet() }
            override fun scroll(dx: Double, dy: Double) { scrolled.incrementAndGet() }
            override fun button(name: String, down: Boolean) {}
            override fun click(name: String) {}
            override fun zoom(steps: Int) { zoomed.addAndGet(steps) }
            override fun gesture(direction: String) { swiped.incrementAndGet(); assertEquals("down", direction) }
        }
        DeviceActivity().use { scenario ->
            SystemClock.sleep(500)
            scenario.onActivity { activity -> ViewModelProvider(activity)[TapViewModel::class.java].keyboardOn.value = false }
            SystemClock.sleep(150)
            lateinit var mic: MicBallView
            lateinit var pad: TouchpadView
            scenario.onActivity { activity ->
                val viewport = activity.findViewById<ViewGroup>(R.id.controller_regions)
                val originalPad = views(viewport).filterIsInstance<TouchpadView>().single()
                val originalMic = views(viewport).filterIsInstance<MicBallView>().single()
                val micParent = originalMic.parent as ViewGroup
                val padParams = originalPad.layoutParams; val micParams = originalMic.layoutParams
                viewport.removeView(originalPad); micParent.removeView(originalMic)
                pad = TouchpadView(activity, sink).apply { connected = true; verticalScale = originalPad.verticalScale }
                mic = MicBallView(activity, { began.incrementAndGet(); mic.post { mic.status = "transmitting" }; true }, { ended.incrementAndGet(); mic.status = "idle" }, saveMode = {
                    mic.gestureMode = if (mic.gestureMode == MicBallView.MODE_HOLD) MicBallView.MODE_TOGGLE else MicBallView.MODE_HOLD
                })
                    .apply { available = true; verticalScale = originalMic.verticalScale; onFeedback = { feedback.add(it) } }
                viewport.addView(pad, 1, padParams); micParent.addView(mic, micParams)
            }
            SystemClock.sleep(200)
            var micPoint = 0f to 0f; var padPoint = 0f to 0f
            scenario.onActivity {
                val p = IntArray(2); mic.getLocationOnScreen(p); val center = mic.ballCenter()
                micPoint = (p[0] + center.first) to (p[1] + center.second)
                pad.getLocationOnScreen(p); padPoint = (p[0] + pad.width / 3f) to (p[1] + pad.height / 3f)
            }
            val down = SystemClock.uptimeMillis()
            fun inject(action: Int, points: List<Pair<Float, Float>>) {
                val e = motion(down, action, points); assertTrue(instrumentation.uiAutomation.injectInputEvent(e, true)); e.recycle()
            }
            inject(MotionEvent.ACTION_DOWN, listOf(micPoint)); await { began.get() == 1 && mic.status == "transmitting" }
            assertEquals(listOf(KeyFeedback.Press, KeyFeedback.LongPress), feedback.toList())
            inject(MotionEvent.ACTION_POINTER_DOWN or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(micPoint, padPoint))
            repeat(3) { n -> inject(MotionEvent.ACTION_MOVE, listOf(micPoint, (padPoint.first + (n + 1) * 25) to padPoint.second)); SystemClock.sleep(25) }
            assertTrue(moved.get() > 0); assertEquals(0, scrolled.get()); assertEquals(0, ended.get())
            val a = (padPoint.first + 75) to padPoint.second
            val b = (padPoint.first + pad.width * 0.35f) to padPoint.second
            inject(MotionEvent.ACTION_POINTER_DOWN or (2 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(micPoint, a, b))
            val spreadA = (a.first - pad.width * 0.08f) to a.second
            val spreadB = (b.first + pad.width * 0.08f) to b.second
            inject(MotionEvent.ACTION_MOVE, listOf(micPoint, spreadA, spreadB))
            assertTrue("two touchpad fingers did not zoom while the microphone was held", zoomed.get() > 0)
            assertEquals("microphone finger incorrectly became a third touchpad finger", 0, swiped.get())
            val c = ((spreadA.first + spreadB.first) / 2) to padPoint.second
            inject(MotionEvent.ACTION_POINTER_DOWN or (3 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(micPoint, spreadA, spreadB, c))
            val distance = pad.resources.displayMetrics.density * 40
            val swipeA = spreadA.first to (spreadA.second + distance)
            val swipeB = spreadB.first to (spreadB.second + distance)
            val swipeC = c.first to (c.second + distance)
            inject(MotionEvent.ACTION_MOVE, listOf(micPoint, swipeA, swipeB, swipeC))
            assertEquals("three touchpad fingers must still work alongside the microphone", 1, swiped.get())
            assertEquals("transmitting", mic.status); assertEquals(0, ended.get()); assertEquals(0, scrolled.get())
            inject(MotionEvent.ACTION_POINTER_UP or (3 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(micPoint, swipeA, swipeB, swipeC))
            inject(MotionEvent.ACTION_POINTER_UP or (2 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(micPoint, swipeA, swipeB))
            inject(MotionEvent.ACTION_POINTER_UP or (1 shl MotionEvent.ACTION_POINTER_INDEX_SHIFT), listOf(micPoint, swipeA))
            inject(MotionEvent.ACTION_UP, listOf(micPoint)); await { ended.get() == 1 }
            assertEquals("release or other region generated extra haptics", listOf(KeyFeedback.Press, KeyFeedback.LongPress), feedback.toList())
            feedback.clear()
            assertEquals("idle", mic.status)
            var modePoint = 0f to 0f
            scenario.onActivity {
                val button = mic.javaClass.getDeclaredField("modeButton").apply { isAccessible = true }.get(mic) as android.graphics.RectF
                val p = IntArray(2); mic.getLocationOnScreen(p)
                modePoint = (p[0] + button.centerX()) to (p[1] + button.centerY())
            }
            fun tap(point: Pair<Float, Float>) {
                inject(MotionEvent.ACTION_DOWN, listOf(point)); SystemClock.sleep(40); inject(MotionEvent.ACTION_UP, listOf(point))
            }
            tap(modePoint); assertEquals(MicBallView.MODE_TOGGLE, mic.gestureMode)
            tap(micPoint); await { began.get() == 2 && mic.status == "transmitting" }
            tap(micPoint); await { ended.get() == 2 }; assertEquals("idle", mic.status)
            assertEquals(listOf(KeyFeedback.Press, KeyFeedback.Press, KeyFeedback.Press), feedback.toList())
            val position = mic.normalizedPosition()
            inject(MotionEvent.ACTION_DOWN, listOf(micPoint))
            inject(MotionEvent.ACTION_MOVE, listOf((micPoint.first + mic.width * 0.10f) to micPoint.second))
            inject(MotionEvent.ACTION_UP, listOf((micPoint.first + mic.width * 0.10f) to micPoint.second))
            assertTrue(mic.normalizedPosition().first > position.first)
            assertEquals(2, began.get()); assertEquals(2, ended.get())
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
