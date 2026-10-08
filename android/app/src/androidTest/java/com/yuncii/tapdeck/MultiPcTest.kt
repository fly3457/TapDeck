package com.yuncii.tapdeck

import android.app.Application
import android.content.Context
import android.content.Intent
import android.graphics.Bitmap
import android.os.SystemClock
import android.view.View
import android.view.accessibility.AccessibilityNodeInfo
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.*
import androidx.lifecycle.ViewModelProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import kotlinx.coroutines.*
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.*
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class MultiPcTest {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val context = instrumentation.targetContext
    private val store = PairStore(context)
    private val a = Peer("127.0.0.1", 42443, 42080, "ab".repeat(32), "同名电脑", "test-a")
    private val b = a.copy(pin = "cd".repeat(32), httpPort = 43080, token = "test-b")
    private fun main(action: () -> Unit) { instrumentation.runOnMainSync(action); instrumentation.waitForIdleSync() }
    private fun await(ms: Long = 12000, condition: () -> Boolean) {
        val deadline = SystemClock.elapsedRealtime() + ms
        while (!condition() && SystemClock.elapsedRealtime() < deadline) SystemClock.sleep(25)
        assertTrue("condition timed out after $ms ms", condition())
    }
    @Suppress("UNCHECKED_CAST")
    private fun preferences(): DataStore<Preferences> = Class.forName("com.yuncii.tapdeck.PairStoreKt")
        .getDeclaredMethod("getDataStore", Context::class.java).apply { isAccessible = true }.invoke(null, context) as DataStore<Preferences>
    private fun isolated(block: () -> Unit) {
        val emulator = instrumentation.uiAutomation.executeShellCommand("getprop ro.kernel.qemu").use {
            android.os.ParcelFileDescriptor.AutoCloseInputStream(it).readBytes().toString(Charsets.UTF_8).trim()
        }
        assumeTrue("isolated emulator required", emulator == "1")
        val data = preferences()
        val original = runBlocking { data.data.first() }
        try {
            runBlocking { data.edit { it.remove(stringPreferencesKey("protected_peers_v1")); it.remove(stringPreferencesKey("protected_peer")) } }
            block()
        } finally { runBlocking { data.updateData { original } } }
    }
    private fun activity(block: (MainActivity, TapViewModel) -> Unit) {
        val screen = instrumentation.startActivitySync(Intent(context, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) as MainActivity
        lateinit var vm: TapViewModel
        main { vm = ViewModelProvider(screen)[TapViewModel::class.java] }
        await { vm.client.state.value.catalogReady && vm.voiceSelectionReady }
        try { block(screen, vm) } finally { main { screen.finish() } }
    }
    @Suppress("UNCHECKED_CAST")
    private fun state(client: TapClient) = client.javaClass.getDeclaredField("mutable").apply { isAccessible = true }.get(client) as MutableStateFlow<ClientState>
    private fun nodes(root: AccessibilityNodeInfo?): List<AccessibilityNodeInfo> = if (root == null) emptyList() else
        listOf(root) + (0 until root.childCount).flatMap { nodes(root.getChild(it)) }
    private fun click(label: String) {
        var target: AccessibilityNodeInfo? = null
        await {
            val visible = nodes(instrumentation.uiAutomation.rootInActiveWindow)
            target = visible.firstOrNull { it.isVisibleToUser && (it.text?.toString() == label || it.contentDescription?.toString()?.startsWith(label) == true) }
            if (target == null) {
                visible.firstOrNull { it.isScrollable }?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_FORWARD)
                SystemClock.sleep(100)
            }
            target != null
        }
        while (target != null && !target!!.isClickable) target = target!!.parent
        assertNotNull("no clickable parent: $label", target)
        assertTrue("not clickable: $label", target!!.performAction(AccessibilityNodeInfo.ACTION_CLICK))
        instrumentation.waitForIdleSync()
        SystemClock.sleep(350)
    }
    private fun shot(name: String) {
        instrumentation.waitForIdleSync()
        SystemClock.sleep(200)
        val label = InstrumentationRegistry.getArguments().getString("uiLabel") ?: "phone-100"
        val file = File(context.getExternalFilesDir(null), "ui-validation/$label-multipc-$name.png")
        file.parentFile!!.mkdirs()
        val bitmap = instrumentation.uiAutomation.takeScreenshot()!!
        file.outputStream().use { bitmap.compress(Bitmap.CompressFormat.PNG, 100, it) }
        bitmap.recycle()
    }

    @Test fun encryptedMigrationAndFailedTransactionsKeepOriginalData() = isolated {
        val data = preferences()
        val legacyKey = stringPreferencesKey("protected_peer")
        val catalogKey = stringPreferencesKey("protected_peers_v1")
        val encrypt = PairStore::class.java.getDeclaredMethod("encrypt", String::class.java).apply { isAccessible = true }
        val protected = encrypt.invoke(store, wireJson.encodeToString(a)) as String
        val deviceId = runBlocking { store.deviceId() }
        runBlocking {
            data.edit { it[legacyKey] = protected; it[stringPreferencesKey("voice_profile_id")] = "voice-2"; it[doublePreferencesKey("touchpad_sensitivity_multiplier")] = 1.7 }
            val migrated = store.loadCatalog()
            assertEquals(a, migrated.selected!!.peer)
            assertEquals("voice-2", migrated.selected!!.voiceProfileId)
            assertEquals(deviceId, store.deviceId())
            assertEquals(1.7, store.loadInputSettings().sensitivity, 0.001)
            assertNull(data.data.first()[legacyKey])
            val encrypted = data.data.first()[catalogKey]!!
            assertFalse(encrypted.contains("test-a"))
            assertThrows(IllegalStateException::class.java) { runBlocking { store.updateCatalog { error("simulated write transaction failure") } } }
            assertEquals(encrypted, data.data.first()[catalogKey])
            assertEquals(migrated, PairStore(context).loadCatalog())
            data.edit { it[catalogKey] = "corrupt" }
            assertTrue(runCatching { store.loadCatalog() }.isFailure)
            assertEquals("corrupt", data.data.first()[catalogKey])
            data.edit { it.remove(catalogKey); it[legacyKey] = "corrupt-legacy" }
            assertTrue(runCatching { store.loadCatalog() }.isFailure)
            assertEquals("corrupt-legacy", data.data.first()[legacyKey])
            assertNull(data.data.first()[catalogKey])
        }
    }

    @Test fun pickerManagementAndRecordingRestrictions() = isolated {
        activity { screen, vm ->
            InstrumentationRegistry.getArguments().getString("uiHeight")?.toIntOrNull()?.let { height ->
                main { screen.findViewById<View>(R.id.controller_regions).apply { layoutParams.height = height; requestLayout() } }
            }
            click("切换电脑")
            await { nodes(instrumentation.uiAutomation.rootInActiveWindow).any { it.text?.toString() == "还没有已配对电脑" } }
            shot("empty")
            instrumentation.sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
            runBlocking { store.updateCatalog { it.upsert(a).rename(a.id, "办公电脑") } }
            main { vm.client.cancelPairing() }
            await { vm.client.peers.value.peers.size == 1 }
            click("切换电脑"); shot("one")
            instrumentation.sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
            runBlocking { store.updateCatalog { it.upsert(b).rename(b.id, "客厅电脑名字很长用来验证省略显示") } }
            main { vm.client.cancelPairing() }
            await { vm.client.peers.value.peers.size == 2 }
            main { state(vm.client).value = ClientState(connected = true, status = "已连接", peerName = "办公电脑", selectedPeerId = a.id, catalogReady = true, rttMs = 6) }
            click("切换电脑"); shot("two")
            click("管理电脑")
            shot("manage")
            click("修改办公电脑备注")
            val field = nodes(instrumentation.uiAutomation.rootInActiveWindow).first { it.isEditable }
            assertTrue(field.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, android.os.Bundle().apply { putCharSequence(AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE, "书房") }))
            click("保存")
            await { vm.client.peers.value.find(a.id)?.alias == "书房" }
            click("完成")
            var scans = 0
            val scanMonitor = object : android.app.Instrumentation.ActivityMonitor() {
                override fun onStartActivity(intent: Intent): android.app.Instrumentation.ActivityResult? {
                    if (intent.component?.className != "com.journeyapps.barcodescanner.CaptureActivity") return null
                    scans++
                    return android.app.Instrumentation.ActivityResult(android.app.Activity.RESULT_CANCELED, null)
                }
            }
            instrumentation.uiAutomation.grantRuntimePermission(context.packageName, android.Manifest.permission.CAMERA)
            instrumentation.addMonitor(scanMonitor)
            try {
                for (phase in listOf("preparing", "transmitting", "stopping")) {
                    main { state(vm.client).value = state(vm.client).value.copy(mic = phase) }
                    click("切换电脑")
                    await { nodes(instrumentation.uiAutomation.rootInActiveWindow).any { it.text?.toString() == "请先结束语音输入" } }
                    val before = vm.client.state.value
                    main {
                        vm.client.selectPeer(b.id); vm.client.enter("http://127.0.0.1:43080/pair"); vm.client.forgetPeer(a.id); vm.client.cancelPairing()
                        screen.javaClass.getDeclaredMethod("scanPairingAddress").apply { isAccessible = true }.invoke(screen)
                        val link = Intent(Intent.ACTION_VIEW, android.net.Uri.parse("tapdeck://pair?v=2&host=127.0.0.1&pin=${b.pin}"))
                        screen.javaClass.getDeclaredMethod("onNewIntent", Intent::class.java).apply { isAccessible = true }.invoke(screen, link)
                    }
                    assertEquals(before, vm.client.state.value)
                    assertEquals("recording must not launch the scanner", 0, scans)
                    assertNull("refused links must not replay on recreation", screen.intent.data)
                    if (phase == "transmitting") shot("recording-disabled")
                    instrumentation.sendKeyDownUpSync(android.view.KeyEvent.KEYCODE_BACK)
                }
            } finally { instrumentation.removeMonitor(scanMonitor) }
            main { state(vm.client).value = state(vm.client).value.copy(mic = "idle") }
            click("切换电脑"); click("管理电脑"); click("忘记客厅电脑")
            click("忘记这台电脑")
            await { vm.client.peers.value.peers.size == 1 }
            assertTrue(vm.client.state.value.connected)
            assertEquals(a.id, vm.client.state.value.selectedPeerId)
            click("完成")
            runBlocking { store.updateCatalog { original ->
                (1..10).fold(original) { catalog, i -> catalog.upsert(a.copy(pin = i.toString(16).padStart(64, '0'), name = "测试电脑 $i")) }
            } }
            main { vm.client.renamePeer(a.id, "书房") }
            await { vm.client.peers.value.peers.size == 11 }
            click("切换电脑"); shot("many")
            repeat(4) {
                nodes(instrumentation.uiAutomation.rootInActiveWindow).firstOrNull { it.isScrollable }
                    ?.performAction(AccessibilityNodeInfo.ACTION_SCROLL_FORWARD)
                SystemClock.sleep(200)
            }
            click("管理电脑"); click("完成")
        }
    }

    @Test fun receiversSwitchReconnectRevokeAndReject() = isolated {
        val args = InstrumentationRegistry.getArguments()
        val urlA = args.getString("pcA")
        val urlB = args.getString("pcB")
        val urlC = args.getString("pcC")
        val admin = args.getString("pcAdmin")
        assumeTrue("three independent PC fixtures required", urlA != null && urlB != null && urlC != null && admin != null)
        val http = OkHttpClient()
        fun request(path: String, post: Boolean = false): String {
            val builder = Request.Builder().url(admin!! + path)
            if (post) builder.post(ByteArray(0).toRequestBody())
            return http.newCall(builder.build()).execute().use { check(it.isSuccessful); it.body!!.string() }
        }
        fun snapshot() = wireJson.parseToJsonElement(request("/state")).jsonArray
        fun control(action: String, pc: Int) { request("/control?action=$action&pc=$pc", true) }
        activity { screen, vm ->
            val client = vm.client
            fun connected(id: String? = null) = client.state.value.connected && (id == null || client.state.value.selectedPeerId == id)
            main { client.enter(urlA!!) }
            await { connected() }
            val idA = client.state.value.selectedPeerId!!
            await { vm.selectedVoice.value != null }
            main { vm.cycleVoiceProfile() }
            await { client.peers.value.find(idA)?.voiceProfileId == "voice-2" }
            main { client.renamePeer(idA, "办公电脑"); client.enter(urlB!!) }
            await { connected() && client.state.value.selectedPeerId != idA }
            val idB = client.state.value.selectedPeerId!!
            assertEquals(2, client.peers.value.peers.size)
            assertEquals("PC2 按键", client.state.value.config.shortcuts[0].label)
            main { client.enter(urlC!!) }
            await { connected() && client.state.value.selectedPeerId !in listOf(idA, idB) }
            val idC = client.state.value.selectedPeerId!!
            assertEquals(3, client.peers.value.peers.size)
            assertEquals("PC3 按键", client.state.value.config.shortcuts[0].label)
            main { client.selectPeer(idA) }
            await { connected(idA) && vm.selectedVoice.value?.id == "voice-2" }
            assertEquals("办公电脑", client.state.value.peerName)
            assertEquals("PC1 按键", client.state.value.config.shortcuts[0].label)
            await { snapshot()[0].jsonObject.long("approvals") == 1L && snapshot()[1].jsonObject.long("connected") == 0L }
            val oldEpoch = client.state.value.connectionEpoch
            main { vm.setKeyboardOn(true) }
            SystemClock.sleep(200)
            val held = screen.javaClass.getDeclaredField("keyHold").apply { isAccessible = true }.get(screen) as KeyHold
            main { held.lockShift(); held.holdDown("LeftCtrl"); client.button("left", true); client.shortcutHoldStart(0, "fixture-hold") }
            await { snapshot()[0].jsonObject.long("held") >= 2 }
            main { client.selectPeer(idB) }
            await { connected(idB) && snapshot()[0].jsonObject.long("held") == 0L && snapshot()[0].jsonObject.long("connected") == 0L }
            val bEvents = snapshot()[1].jsonObject.long("events")
            val aEvents = snapshot()[0].jsonObject.long("events")
            main { client.inputInSession(oldEpoch) { client.keyDown("A") }; held.dualShort("B"); held.holdUp("LeftCtrl") }
            SystemClock.sleep(150)
            assertEquals(bEvents, snapshot()[1].jsonObject.long("events"))
            main { client.keyDown("C"); client.keyUp("C"); client.move(5.0, 2.0) }
            await { snapshot()[1].jsonObject.long("events") > bEvents && snapshot()[1].jsonObject.long("mouse_packets") > 0 }
            assertEquals(aEvents, snapshot()[0].jsonObject.long("events"))
            main { client.selectPeer(idA); client.selectPeer(idB); client.selectPeer(idA) }
            await { connected(idA) }
            SystemClock.sleep(300)
            assertEquals(idA, client.state.value.selectedPeerId)
            main { client.selectPeer(idA); client.selectPeer(idB); client.selectPeer(idC) }
            await { connected(idC) && snapshot().take(2).all { it.jsonObject.long("connected") == 0L } }
            assertEquals(idC, runBlocking { store.loadCatalog().selectedId })
            assertEquals("PC3 按键", client.state.value.config.shortcuts[0].label)
            main { client.forgetPeer(idC) }
            await { client.peers.value.find(idC) == null && snapshot()[2].jsonObject.long("connected") == 0L }
            assertNull(client.state.value.selectedPeerId)
            assertFalse(client.state.value.connected)
            control("stop", 1)
            main { client.selectPeer(idB) }
            await { client.state.value.status.contains("重试") }
            assertEquals(idB, client.state.value.selectedPeerId)
            assertEquals(idB, runBlocking { store.loadCatalog().selectedId })
            assertEquals(0L, snapshot()[0].jsonObject.long("connected"))
            control("start", 1)
            await(20000) { connected(idB) }
            main { client.selectPeer(idA) }
            await { connected(idA) }
            val oldToken = client.peers.value.find(idA)!!.peer.token
            control("revoke", 0)
            await { client.state.value.phase == ConnectionPhase.NeedsPairing && client.peers.value.find(idA)!!.needsPairing }
            SystemClock.sleep(1200)
            assertEquals(1L, snapshot()[0].jsonObject.long("approvals"))
            assertFalse(client.peers.value.find(idB)!!.needsPairing)
            val restoredScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
            val restored = TapClient(context.applicationContext as Application, restoredScope)
            try {
                runBlocking { restored.restore() }
                assertEquals(ConnectionPhase.NeedsPairing, restored.state.value.phase)
                assertFalse(restored.state.value.connected)
                assertEquals(idA, restored.state.value.selectedPeerId)
            } finally { main { restored.close(); restoredScope.cancel() } }
            main { client.selectPeer(idB) }
            await { connected(idB) }
            main { client.selectPeer(idA) }
            await { connected(idA) }
            assertNotEquals(oldToken, client.peers.value.find(idA)!!.peer.token)
            assertEquals("办公电脑", client.state.value.peerName)
            main { client.forgetPeer(idB) }
            await { client.peers.value.find(idB) == null }
            for (mode in listOf("reject", "wait")) {
                control(mode, 1)
                main { client.enter(urlB!!) }
                await { client.state.value.phase == ConnectionPhase.Disconnected && client.state.value.error.isNotEmpty() }
                assertEquals(listOf(idA), client.peers.value.peers.map { it.id })
                assertNull(client.state.value.selectedPeerId)
            }
            main { client.enter(urlB!!) }
            await { client.state.value.pairing.isNotEmpty() }
            main { client.cancelPairing() }
            await { runBlocking { store.loadCatalog().selectedId } == null }
            assertEquals(1, client.peers.value.peers.size)
            main { client.selectPeer(idA) }
            await { connected(idA) }
            shot("live-connected")
        }
        // A fresh ViewModel restores only the selected, trusted computer without approval.
        activity { _, vm ->
            await { vm.client.state.value.connected }
            assertEquals("办公电脑", vm.client.state.value.peerName)
            assertEquals(2L, snapshot()[0].jsonObject.long("approvals"))
        }
        http.dispatcher.executorService.shutdown()
        http.connectionPool.evictAll()
    }
}
