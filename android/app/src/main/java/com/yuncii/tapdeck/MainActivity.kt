package com.yuncii.tapdeck

import android.Manifest
import android.app.Application
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import android.widget.LinearLayout
import android.widget.FrameLayout
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.platform.ComposeView
import androidx.compose.ui.platform.ViewCompositionStrategy
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.combine
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import kotlin.math.min

object Regions { const val CONNECTION = 0.10f; const val TOUCHPAD = 0.40f; const val SHORTCUTS = 0.25f; const val MICROPHONE = 0.25f }
class TapViewModel(app: Application) : AndroidViewModel(app) {
    val client = TapClient(app, viewModelScope)
    private val store = PairStore(app)
    val ballPosition = MutableStateFlow<Pair<Float, Float>?>(null)
    /** 语音输入方式（长按 / 单击）与全键盘开关：记在本地，重启 App 后沿用。 */
    val voiceMode = MutableStateFlow(MicBallView.MODE_HOLD)
    val keyboardOn = MutableStateFlow(false)
    init {
        viewModelScope.launch { client.restore() }
        viewModelScope.launch { ballPosition.value = store.loadBallPosition() }
        viewModelScope.launch {
            val (mode, keyboard) = store.loadUiMode()
            voiceMode.value = mode
            keyboardOn.value = keyboard
        }
    }
    fun saveBallPosition(x: Float, y: Float) {
        ballPosition.value = x to y
        viewModelScope.launch { store.saveBallPosition(x, y) }
    }
    fun setVoiceMode(mode: String) {
        if (voiceMode.value == mode) return
        voiceMode.value = mode
        viewModelScope.launch { store.saveVoiceMode(mode) }
    }
    fun setKeyboardOn(on: Boolean) {
        if (keyboardOn.value == on) return
        keyboardOn.value = on
        viewModelScope.launch { store.saveKeyboardMode(on) }
    }
    override fun onCleared() { client.close() }
}
class MainActivity : ComponentActivity() {
    private val vm: TapViewModel by viewModels()
    private var touchpad: TouchpadView? = null
    private var microphone: MicBallView? = null
    private var deferredLink: String? = null
    private var scannedLink: Boolean = false
    private var settings by mutableStateOf(false)
    private var address by mutableStateOf("http://192.168.1.11:41080/pair")
    /** 语音模式默认长按语音输入；切换入口在语音区的按钮上，状态记在本地。 */
    private val voiceToggle: Boolean get() = vm.voiceMode.value == MicBallView.MODE_TOGGLE
    /** 快捷键的轻点 / 按住手势。 */
    private val shortcutHold by lazy { ShortcutHold({ slot, token -> vm.client.shortcutHoldStart(slot, token) }, { token -> vm.client.shortcutHoldStop(token) }) }
    /** 全键盘的按键状态。 */
    private val keyHold by lazy { KeyHold({ chord -> vm.client.keyDown(chord) }, { chord -> vm.client.keyUp(chord) }) }
    /** 切换全键盘 / 快捷键+语音两种下半区布局。 */
    private var modeViews: (() -> Unit)? = null
    private val microphonePermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted -> if (!granted) android.widget.Toast.makeText(this, "麦克风权限未授予，键鼠仍可使用", android.widget.Toast.LENGTH_LONG).show() }
    private val lanPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted -> if (granted) deferredLink?.let { vm.client.enter(it, scannedLink); deferredLink = null } else android.widget.Toast.makeText(this, "需要局域网权限才能连接电脑", android.widget.Toast.LENGTH_LONG).show() }
    private val scanner = registerForActivityResult(ScanContract()) { result -> result.contents?.let { enter(it, true) } }
    private val cameraPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted -> if (granted) scan() }
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState); fullscreen()
        fun compose(content: @Composable () -> Unit) = ComposeView(this).apply {
            setViewCompositionStrategy(ViewCompositionStrategy.DisposeOnViewTreeLifecycleDestroyed)
            setContent {
                MaterialTheme(colorScheme = lightColorScheme(primary = Color(0xFF175CD3), background = Color.White, surface = Color.White)) { content() }
            }
        }
        // One native parent routes simultaneous pointer IDs to different regions.
        // Keeping it outside AndroidView also exposes nested Compose buttons to
        // Android accessibility instead of collapsing the entire screen to one node.
        val regions = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            isMotionEventSplittingEnabled = true
            setBackgroundColor(android.graphics.Color.WHITE)
            fun region(view: android.view.View, weight: Float): android.view.View {
                addView(view, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, weight))
                return view
            }
            fun fixedRegion(view: android.view.View, fraction: Float): android.view.View {
                addView(view, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, (resources.displayMetrics.heightPixels * fraction).toInt()))
                return view
            }
            // 状态区固定为屏幕高度的 10%；其余区域按权重吃满剩余高度，
            // 键盘关闭时下半区由触控板(0.40) + 快捷键(0.25) + 语音(0.25) 填满。
            fixedRegion(compose {
                val state by vm.client.state.collectAsStateWithLifecycle()
                val keyboardOn by vm.keyboardOn.collectAsStateWithLifecycle()
                ConnectionHeader(
                    state,
                    keyboardOn,
                    onKeyboardToggle = { on ->
                        vm.setKeyboardOn(on)
                        // 立即切换下半区布局，不必等 onResume。
                        modeViews?.invoke()
                        // 退出全键盘时释放所有按键与修饰键；进入时结束可能正在进行的录音。
                        if (!on) keyHold.releaseAll() else if (vm.client.state.value.mic != "idle") vm.client.stopMic()
                    },
                    onSettings = { settings = true },
                )
            }, Regions.CONNECTION)
            region(TouchpadView(context, vm.client).also { touchpad = it }, Regions.TOUCHPAD)
            // 快捷键区 / 语音区 / 全键盘区共用下半部分：全键盘激活时前两者隐藏，
            // 键盘权重等于两者之和，因此两种模式下各区域高度一致。
            val shortcutsRegion = compose {
                val state by vm.client.state.collectAsStateWithLifecycle()
                ShortcutButtons(
                    state,
                    shortcutHold,
                    stopRecording = {
                        // 单击语音输入录音中：按下快捷键先结束录音，这一次不再发送按键。
                        val active = microphone?.gestureMode == MicBallView.MODE_TOGGLE && state.mic in listOf("preparing", "transmitting")
                        if (active) vm.client.stopMic()
                        active
                    },
                )
            }
            val voiceRegion = MicBallView(context, ::beginMic, vm.client::stopMic, vm::saveBallPosition, vm::setVoiceMode).also { view ->
                microphone = view
                view.gestureMode = if (voiceToggle) MicBallView.MODE_TOGGLE else MicBallView.MODE_HOLD
            }
            val voiceComposite = LinearLayout(context).apply {
                orientation = LinearLayout.VERTICAL
                isMotionEventSplittingEnabled = true
                addView(voiceRegion, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f))
            }
            val keyboardRegion = compose {
                val state by vm.client.state.collectAsStateWithLifecycle()
                val keyboardOn by vm.keyboardOn.collectAsStateWithLifecycle()
                if (keyboardOn) KeyboardView(
                    connected = state.connected,
                    voiceActive = state.mic == "preparing" || state.mic == "transmitting",
                    hold = keyHold,
                    beginVoice = ::beginMic,
                    stopVoice = { vm.client.stopMic() },
                )
            }
            val shortcutsView = region(shortcutsRegion, Regions.SHORTCUTS)
            val voiceView = region(voiceComposite, Regions.MICROPHONE)
            val keyboardView = region(keyboardRegion, Regions.SHORTCUTS + Regions.MICROPHONE)
            // 下半部分只有一组子视图是可见的：键盘区权重等于两个隐藏区域之和，
            // LinearLayout 按可见子视图的权重归一化，所以两种模式下各区域都吃满高度。
            // （此前的占位视图也带权重，导致键盘关闭时底部空出约三分之一。）
            modeViews = {
                val on = vm.keyboardOn.value
                val hidden = if (on) android.view.View.GONE else android.view.View.VISIBLE
                shortcutsView.visibility = hidden
                voiceView.visibility = hidden
                keyboardView.visibility = if (on) android.view.View.VISIBLE else android.view.View.GONE
            }
            modeViews?.invoke()
        }
        val dialogs = compose {
            val state by vm.client.state.collectAsStateWithLifecycle()
            if (settings) AlertDialog(onDismissRequest = { settings = false }, title = { Text("连接电脑") }, text = {
                Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text("输入 PC 设置窗口显示的配对网址，或粘贴完整配对信息。")
                    OutlinedTextField(value = address, onValueChange = { address = it }, label = { Text("PC 配对网址") }, singleLine = true, keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri))
                    if (packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)) TextButton(onClick = { if (checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) scan() else cameraPermission.launch(Manifest.permission.CAMERA) }) { Text("扫描电脑二维码") }
                    if (state.connected) TextButton(onClick = { vm.client.forget() }) { Text("忘记当前电脑") }
                }
            }, confirmButton = { TextButton(onClick = { enter(address); settings = false }) { Text("连接") } }, dismissButton = { TextButton(onClick = { settings = false }) { Text("关闭") } })
            if (state.pairing.isNotEmpty()) AlertDialog(onDismissRequest = { vm.client.forget() }, title = { Text("等待电脑允许连接") }, text = { Column { Text("请核对 PC 设置窗口里的校验码是否一致，一致后在电脑上点“允许连接”："); Spacer(Modifier.height(12.dp)); Text(state.pairing, fontSize = 19.sp); Spacer(Modifier.height(12.dp)); Text("电脑允许后会自动连上，手机上不需要额外操作。") } }, confirmButton = {}, dismissButton = { TextButton(onClick = vm.client::forget) { Text("取消") } })
        }
        val root = FrameLayout(this).apply {
            isMotionEventSplittingEnabled = true
            // 系统状态栏的留白放在最外层：四个区域的高度仍然严格按比例分配。
            setOnApplyWindowInsetsListener { view, insets ->
                val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars())
                val cutout = insets.getInsets(WindowInsetsCompat.Type.displayCutout())
                view.setPadding(maxOf(bars.left, cutout.left), maxOf(bars.top, cutout.top), maxOf(bars.right, cutout.right), maxOf(bars.bottom, cutout.bottom))
                insets
            }
            addView(regions, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
            // Dialogs own their windows; this composition needs no screen area.
            addView(dialogs, FrameLayout.LayoutParams(0, 0))
        }
        setContentView(root)
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                // 本地记录的模式是异步读出来的：读到以后要重新应用到界面。
                launch {
                    vm.keyboardOn.collect { modeViews?.invoke() }
                }
                launch {
                    vm.voiceMode.collect { mode -> microphone?.let { if (it.gestureMode != mode) it.gestureMode = mode } }
                }
                combine(vm.client.state, vm.ballPosition) { state, position -> state to position }.collect { (state, position) ->
                    touchpad?.connected = state.connected
                    microphone?.apply {
                        available = state.connected; mode = state.micMode; status = state.mic; level = state.level
                        // 单击语音输入开始录音时，撤销可能仍在按住的快捷键。
                        if (state.mic == "preparing" || state.mic == "transmitting") shortcutHold.cancelAll()
                        position?.let { restorePosition(it.first, it.second) }
                    }
                }
            }
        }
        intent?.data?.let { enter(it.toString()) }
    }
    private fun fullscreen() {
        // 保留系统状态栏（电池、时间等），只把布局延伸到状态栏后面，并按安全区留白。
        WindowCompat.setDecorFitsSystemWindows(window, false)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        if (Build.VERSION.SDK_INT >= 28) window.attributes = window.attributes.apply { layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES }
        WindowInsetsControllerCompat(window, window.decorView).apply {
            systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_DEFAULT
            show(WindowInsetsCompat.Type.systemBars())
        }
    }
    private fun enter(link: String, scanned: Boolean = false) {
        val permission = "android.permission.ACCESS_LOCAL_NETWORK"
        if (Build.VERSION.SDK_INT >= 37 && applicationInfo.targetSdkVersion >= 37 && checkSelfPermission(permission) != PackageManager.PERMISSION_GRANTED) { deferredLink = link; scannedLink = scanned; lanPermission.launch(permission) } else vm.client.enter(link, scanned)
    }
    private fun scan() { scanner.launch(ScanOptions().setDesiredBarcodeFormats(ScanOptions.QR_CODE).setPrompt("扫描 TapDeck 电脑端二维码").setBeepEnabled(false).setOrientationLocked(true)) }
    private fun beginMic(mode: String): Boolean {
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            android.util.Log.i("TapDeck", "beginMic $mode：未授予麦克风权限")
            microphonePermission.launch(Manifest.permission.RECORD_AUDIO)
            return false
        }
        val started = vm.client.startMic(mode)
        if (!started) android.util.Log.i("TapDeck", "beginMic $mode 未开始：${vm.client.state.value.mic}/${vm.client.state.value.error}")
        return started
    }
    override fun onNewIntent(intent: Intent) { super.onNewIntent(intent); setIntent(intent); intent.data?.let { enter(it.toString()) } }
    override fun onResume() { super.onResume(); fullscreen(); modeViews?.invoke() }
    override fun onStart() { super.onStart(); vm.client.setForeground(true) }
    override fun onStop() { touchpad?.cancel(); microphone?.cancel(); vm.client.stopMic(true); vm.client.setForeground(false); super.onStop() }
}

@Composable private fun ConnectionHeader(
    state: ClientState,
    keyboardOn: Boolean,
    onKeyboardToggle: (Boolean) -> Unit,
    onSettings: () -> Unit,
) {
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val scale = LocalDensity.current.fontScale
        // 状态区固定 10%：内容压成单行，避免两行文字把区域撑高、挤掉下面的区域。
        val titleSize = min(18f, maxHeight.value * 0.30f / scale).sp
        val iconBox = (30f.coerceAtMost(maxHeight.value * 0.66f)).dp
        Row(
            Modifier.fillMaxSize().padding(start = 12.dp, end = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = if (state.error.isNotEmpty()) "${state.peerName} · ${state.error}"
                else "${state.peerName} · ${state.status} · RTT ${state.rttMs} ms",
                modifier = Modifier.weight(1f),
                fontSize = titleSize, lineHeight = titleSize * 1.2f, maxLines = 1, overflow = TextOverflow.Ellipsis,
            )
            // 全键盘开关：Lucide keyboard 图标按钮，未激活 #AAA、激活 #000。
            IconButton(
                onClick = { onKeyboardToggle(!keyboardOn) },
                modifier = Modifier.size(iconBox).semantics {
                    contentDescription = if (keyboardOn) "全键盘已激活，点击返回快捷键与语音输入" else "切换到全键盘"
                },
            ) {
                Icon(
                    painter = painterResource(R.drawable.ic_lucide_keyboard),
                    contentDescription = null,
                    tint = if (keyboardOn) Color(0xFF000000) else Color(0xFFAAAAAA),
                    modifier = Modifier.size(iconBox * 0.8f),
                )
            }
            Spacer(Modifier.width(6.dp))
            TextButton(onClick = onSettings, contentPadding = PaddingValues(horizontal = 8.dp, vertical = 0.dp)) { Text("连接", fontSize = titleSize, lineHeight = titleSize * 1.2f) }
        }
    }
}

@Composable private fun ShortcutButtons(state: ClientState, hold: ShortcutHold, stopRecording: () -> Boolean) {
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val visible = state.config.visibleShortcuts()
        val rows = if (visible.size > 4) 2 else 1
        val columns = if (rows == 2) 4 else visible.size.coerceAtLeast(1)
        val scale = LocalDensity.current.fontScale
        val cellWidth = (maxWidth.value - 8f - (columns - 1) * 4f) / columns
        val cellHeight = (maxHeight.value - 8f - (rows - 1) * 4f) / rows
        val titleSize = min(17f, min(cellWidth * 0.26f, cellHeight * 0.30f) / scale).sp
        val chordSize = min(10f, min(cellWidth * 0.17f, cellHeight * 0.22f) / scale).sp
        Column(Modifier.fillMaxSize().padding(4.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            repeat(rows) { row ->
                Row(Modifier.weight(1f).fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                    repeat(columns) { column ->
                        val item = visible.getOrNull(row * columns + column)
                        if (item == null) Spacer(Modifier.weight(1f).fillMaxHeight())
                        else Box(
                            Modifier.weight(1f).fillMaxHeight()
                                .semantics { contentDescription = "快捷键 ${item.index + 1}：${item.value.label}" }
                                .shortcutPress(state.connected, item.index, hold, stopRecording),
                        ) {
                            // 自绘按钮外观：不用 Button，避免它消费抬手事件而收不到释放。
                            Surface(
                                modifier = Modifier.fillMaxSize(),
                                shape = MaterialTheme.shapes.small,
                                color = MaterialTheme.colorScheme.surface,
                                contentColor = MaterialTheme.colorScheme.primary,
                                border = BorderStroke(1.dp, if (state.connected) MaterialTheme.colorScheme.primary else Color(0xFFCBD5E1)),
                            ) {
                                Column(
                                    modifier = Modifier.fillMaxSize().padding(3.dp),
                                    horizontalAlignment = Alignment.CenterHorizontally,
                                    verticalArrangement = Arrangement.Center,
                                ) {
                                    Text(item.value.label, fontSize = titleSize, lineHeight = titleSize * 1.25f, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                    Text(item.value.chord, fontSize = chordSize, lineHeight = chordSize * 1.25f, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

/**
 * 快捷键的按下 / 抬起手势：按下即让 PC 保持组合键按下，松手立即释放。
 * 轻点就是一次「按下→抬起」，按住就是持续的按下状态，与实体键盘一致。
 * 单击语音输入录音进行中按下任意快捷键时，只结束录音，这一次不发送按键。
 */
class ShortcutHold(
    private val start: (Int, String) -> Unit,
    private val stop: (String) -> Unit,
) {
    private val active = mutableMapOf<Int, String>()

    fun press(slot: Int, recordingActive: Boolean) {
        if (active.containsKey(slot)) return
        if (recordingActive) return
        val token = "slot$slot-${System.nanoTime()}"
        active[slot] = token
        start(slot, token)
    }

    fun release(slot: Int) {
        val token = active.remove(slot) ?: return
        stop(token)
    }

    /** 开始录音时撤销仍按住的键，避免残留按下状态。 */
    fun cancelAll() {
        val pending = active.values.toList()
        active.clear()
        pending.forEach { stop(it) }
    }
}

private fun Modifier.shortcutPress(
    connected: Boolean,
    slot: Int,
    hold: ShortcutHold,
    stopRecording: () -> Boolean,
): Modifier = pointerInput(connected, slot) {
    awaitEachGesture {
        awaitFirstDown(requireUnconsumed = false)
        if (!connected) return@awaitEachGesture
        val wasRecording = stopRecording()
        hold.press(slot, wasRecording)
        waitForUpOrCancellation()
        hold.release(slot)
    }
}
