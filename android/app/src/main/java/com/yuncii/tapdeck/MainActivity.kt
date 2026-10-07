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
    init {
        viewModelScope.launch { client.restore() }
        viewModelScope.launch { ballPosition.value = store.loadBallPosition() }
    }
    fun saveBallPosition(x: Float, y: Float) {
        ballPosition.value = x to y
        viewModelScope.launch { store.saveBallPosition(x, y) }
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
    /** 语音模式开关：默认长按语音输入（圆形），打开后单击语音输入（方形）。 */
    private var voiceToggle by mutableStateOf(false)
    /** 全键盘：激活后下方快捷键区与语音区换成键盘区。 */
    private var keyboardMode by mutableStateOf(false)
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
                ConnectionHeader(
                    state,
                    voiceToggle,
                    keyboardMode,
                    onVoiceToggle = { voiceToggle = it; microphone?.gestureMode = if (it) MicBallView.MODE_TOGGLE else MicBallView.MODE_HOLD },
                    onKeyboardToggle = { on ->
                        keyboardMode = on
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
            val voiceRegion = MicBallView(context, ::beginMic, vm.client::stopMic, vm::saveBallPosition).also { view ->
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
                if (keyboardMode) KeyboardView(
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
                val hidden = if (keyboardMode) android.view.View.GONE else android.view.View.VISIBLE
                shortcutsView.visibility = hidden
                voiceView.visibility = hidden
                keyboardView.visibility = if (keyboardMode) android.view.View.VISIBLE else android.view.View.GONE
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
            if (state.pairing.isNotEmpty()) AlertDialog(onDismissRequest = { vm.client.forget() }, title = { Text("核对配对校验码") }, text = { Column { Text("请与 PC 设置窗口的校验码比较："); Spacer(Modifier.height(12.dp)); Text(state.pairing, fontSize = 19.sp); Spacer(Modifier.height(12.dp)); Text(if (state.pairingConfirmed) "等待电脑允许连接…" else "相同后点击确认，并在电脑允许连接。") } }, confirmButton = { TextButton(onClick = vm.client::confirmPair, enabled = !state.pairingConfirmed) { Text("与电脑一致") } }, dismissButton = { TextButton(onClick = vm.client::forget) { Text("取消") } })
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
    voiceToggle: Boolean,
    keyboardMode: Boolean,
    onVoiceToggle: (Boolean) -> Unit,
    onKeyboardToggle: (Boolean) -> Unit,
    onSettings: () -> Unit,
) {
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val scale = LocalDensity.current.fontScale
        // 状态区固定 10%：内容压成单行，避免两行文字把区域撑高、挤掉下面的区域。
        val titleSize = min(18f, maxHeight.value * 0.30f / scale).sp
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
            // 全键盘开关与语音输入方式开关互斥显示在同一位置，两者文字都与「连接」同号。
            if (keyboardMode) ModeSwitch("全键盘", true, titleSize) { onKeyboardToggle(false) }
            else {
                CompactSwitch(voiceToggle, titleSize, onVoiceToggle)
                // 两个开关之间留出与「连接」前一致的间隙，避免文字和开关挤在一起。
                Spacer(Modifier.width(16.dp))
                ModeSwitch("全键盘", false, titleSize) { onKeyboardToggle(true) }
            }
            Spacer(Modifier.width(16.dp))
            TextButton(onClick = onSettings, contentPadding = PaddingValues(horizontal = 8.dp, vertical = 0.dp)) { Text("连接", fontSize = titleSize, lineHeight = titleSize * 1.2f) }
        }
    }
}

/** 全键盘开关：文字 + 小开关，样式与语音输入方式开关一致。 */
@Composable private fun ModeSwitch(label: String, checked: Boolean, textSize: TextUnit, onChange: (Boolean) -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(label, fontSize = textSize, lineHeight = textSize * 1.2f, color = Color(0xFF3F4F60), maxLines = 1)
        Spacer(Modifier.width(6.dp))
        Box(
            Modifier
                .size(width = 38.dp, height = 22.dp)
                .clip(RoundedCornerShape(11.dp))
                .background(if (checked) Color(0xFF175CD3) else Color(0xFFCBD5E1))
                .clickable { onChange(!checked) }
                .semantics { contentDescription = if (checked) "全键盘已激活，点击返回快捷键与语音输入" else "切换到全键盘" },
        ) {
            Box(
                Modifier
                    .align(if (checked) Alignment.CenterEnd else Alignment.CenterStart)
                    .padding(horizontal = 3.dp)
                    .size(16.dp)
                    .clip(CircleShape)
                    .background(Color.White),
            )
        }
    }
}

/**
 * 语音输入方式的开关，放在状态区「连接」旁边：关＝长按语音输入（圆形控件），
 * 开＝单击语音输入（方形控件）。自绘以保证在 10% 高度里也只占很小一块。
 */
@Composable private fun CompactSwitch(checked: Boolean, textSize: TextUnit, onChange: (Boolean) -> Unit) {
    val track = if (checked) Color(0xFF175CD3) else Color(0xFFCBD5E1)
    val label = if (checked) "单击语音输入" else "长按语音输入"
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(label, fontSize = textSize, lineHeight = textSize * 1.2f, color = Color(0xFF3F4F60), maxLines = 1)
        Spacer(Modifier.width(6.dp))
        Box(
            Modifier
                .size(width = 38.dp, height = 22.dp)
                .clip(RoundedCornerShape(11.dp))
                .background(track)
                .clickable { onChange(!checked) }
                .semantics { contentDescription = if (checked) "语音输入方式：单击语音输入，点击切换为长按语音输入" else "语音输入方式：长按语音输入，点击切换为单击语音输入" },
        ) {
            Box(
                Modifier
                    .align(if (checked) Alignment.CenterEnd else Alignment.CenterStart)
                    .padding(horizontal = 3.dp)
                    .size(16.dp)
                    .clip(CircleShape)
                    .background(Color.White),
            )
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
