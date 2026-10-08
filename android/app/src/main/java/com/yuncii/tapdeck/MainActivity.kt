package com.yuncii.tapdeck

import android.Manifest
import android.app.Application
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import android.view.View
import android.view.WindowManager
import android.widget.LinearLayout
import android.widget.FrameLayout
import androidx.activity.ComponentActivity
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.FastOutSlowInEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.gestures.awaitEachGesture
import androidx.compose.foundation.gestures.awaitFirstDown
import androidx.compose.foundation.gestures.waitForUpOrCancellation
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.disabled
import androidx.compose.ui.semantics.onClick
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextOverflow
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
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.combine
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import kotlin.math.min

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
    private var settings by mutableStateOf(false)
    private var sensitivitySettings by mutableStateOf(false)
    private val feedbackController by lazy { KeyFeedbackController({ vm.client.inputSettings.value.haptics }, AndroidFeedbackBackend(applicationContext)) }
    private var feedbackAvailability by mutableStateOf(FeedbackResult.Requested)
    private var feedbackTestMessage by mutableStateOf("")
    private var pageResumed by mutableStateOf(false)
    private var address by mutableStateOf("http://192.168.1.11:41080/pair")
    private var pairingScanError by mutableStateOf("")
    /** 语音模式默认长按语音输入；切换入口在语音区的按钮上，状态记在本地。 */
    private val voiceToggle: Boolean get() = vm.voiceMode.value == MicBallView.MODE_TOGGLE
    /** 快捷键的轻点 / 按住手势。 */
    private val shortcutHold by lazy { ShortcutHold({ slot, token -> vm.client.shortcutHoldStart(slot, token) }, { token -> vm.client.shortcutHoldStop(token) }) }
    /** 全键盘的按键状态。 */
    private val keyHold by lazy { KeyHold({ chord -> vm.client.keyDown(chord) }, { chord -> vm.client.keyUp(chord) }) }
    /** 切换全键盘 / 快捷键+语音两种下半区布局。 */
    private var modeViews: (() -> Unit)? = null
    private val microphonePermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted -> if (!granted) android.widget.Toast.makeText(this, "麦克风权限未授予，键鼠仍可使用", android.widget.Toast.LENGTH_LONG).show() }
    private val lanPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted -> if (granted) deferredLink?.let { vm.client.enter(it); deferredLink = null } else android.widget.Toast.makeText(this, "需要局域网权限才能连接电脑", android.widget.Toast.LENGTH_LONG).show() }
    private val pairingScanner = registerForActivityResult(ScanContract()) { result ->
        settings = true
        result.contents?.let { contents ->
            val scannedAddress = pairingAddressFromQr(contents)
            if (scannedAddress != null) {
                address = scannedAddress
                pairingScanError = ""
            } else {
                pairingScanError = "未识别到 PC 配对网址，请扫描 PC 设置窗口中的配对二维码"
            }
        }
    }
    private val cameraPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) launchPairingScanner()
        else pairingScanError = "未获得相机权限，可在系统设置中允许后重试，或手动输入配对网址"
    }
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState); fullscreen()
        if (savedInstanceState != null) {
            settings = savedInstanceState.getBoolean("connection_settings")
            address = savedInstanceState.getString("pairing_address", address)
            pairingScanError = savedInstanceState.getString("pairing_scan_error", "")
        }
        fun compose(compact: Boolean = true, content: @Composable () -> Unit) = ComposeView(this).apply {
            setViewCompositionStrategy(ViewCompositionStrategy.DisposeOnViewTreeLifecycleDestroyed)
            val feedbackView = this
            setContent {
                CompositionLocalProvider(LocalKeyFeedback provides { kind ->
                    feedbackView.keyFeedback(kind, feedbackController)
                }) {
                    MaterialTheme(colorScheme = lightColorScheme(primary = Color(0xFF175CD3), background = Color.White, surface = Color.White)) {
                        if (compact) CompactControls(content) else content()
                    }
                }
            }
        }
        // One native parent routes simultaneous pointer IDs to different regions.
        // Keeping it outside AndroidView also exposes nested Compose buttons to
        // Android accessibility instead of collapsing the entire screen to one node.
        val dimensions = mutableStateOf(ControllerLayout.measure(0, 0))
        lateinit var headerView: View
        lateinit var panelView: FrameLayout
        val regions = object : LinearLayout(this) {
            override fun onMeasure(widthMeasureSpec: Int, heightMeasureSpec: Int) {
                val measured = ControllerLayout.measure(MeasureSpec.getSize(widthMeasureSpec), MeasureSpec.getSize(heightMeasureSpec))
                headerView.layoutParams.height = measured.header
                panelView.layoutParams.height = measured.panel
                panelView.setPadding(0, measured.panelPadding, 0, measured.panelPadding)
                touchpad?.verticalScale = measured.scale
                microphone?.verticalScale = measured.scale
                if (dimensions.value != measured) dimensions.value = measured
                super.onMeasure(widthMeasureSpec, heightMeasureSpec)
            }
        }.apply {
            id = R.id.controller_regions
            orientation = LinearLayout.VERTICAL
            isMotionEventSplittingEnabled = true
            setBackgroundColor(android.graphics.Color.WHITE)
            headerView = compose {
                val state by vm.client.state.collectAsStateWithLifecycle()
                val keyboardOn by vm.keyboardOn.collectAsStateWithLifecycle()
                ConnectionHeader(
                    state,
                    keyboardOn,
                    scale = dimensions.value.scale,
                    reminderEnabled = pageResumed && !settings && !sensitivitySettings && state.pairing.isEmpty() && !state.connected,
                    onKeyboardToggle = { on ->
                        touchpad?.cancel()
                        vm.setKeyboardOn(on)
                        // 立即切换下半区布局，不必等 onResume。
                        modeViews?.invoke()
                        // 退出全键盘时释放所有按键与修饰键；进入时结束可能正在进行的录音。
                        if (!on) keyHold.releaseAll() else if (vm.client.state.value.mic != "idle") vm.client.stopMic()
                    },
                    onSettings = { touchpad?.cancel(); feedbackAvailability = feedbackController.availability(); feedbackTestMessage = ""; settings = true },
                )
            }.apply { id = R.id.connection_header }
            addView(headerView, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0))
            // Only the touchpad has a weight: it fills everything between header and panel.
            addView(TouchpadView(context, vm.client).also { view ->
                touchpad = view
                view.onSensitivitySettings = { view.cancel(); sensitivitySettings = true }
            },
                LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f))
            val shortcutsRegion = compose {
                val state by vm.client.state.collectAsStateWithLifecycle()
                ShortcutButtons(
                    state,
                    shortcutHold,
                    scale = dimensions.value.scale,
                    stopRecording = {
                        // 单击语音输入录音中：按下快捷键先结束录音，这一次不再发送按键。
                        val active = microphone?.gestureMode == MicBallView.MODE_TOGGLE && vm.client.state.value.mic in listOf("preparing", "transmitting")
                        if (active) vm.client.stopMic()
                        active
                    },
                )
            }.apply { id = R.id.shortcut_region }
            val voiceRegion = MicBallView(context, ::beginMic, vm.client::stopMic, vm::saveBallPosition, vm::setVoiceMode).also { view ->
                microphone = view
                view.onFeedback = { view.keyFeedback(it, feedbackController) }
                view.gestureMode = if (voiceToggle) MicBallView.MODE_TOGGLE else MicBallView.MODE_HOLD
            }
            val ordinaryPanel = LinearLayout(context).apply {
                orientation = LinearLayout.VERTICAL
                isMotionEventSplittingEnabled = true
                addView(shortcutsRegion, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f))
                addView(voiceRegion, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f))
            }
            val keyboardRegion = compose {
                val state by vm.client.state.collectAsStateWithLifecycle()
                val keyboardOn by vm.keyboardOn.collectAsStateWithLifecycle()
                if (keyboardOn) KeyboardView(
                    connected = state.connected,
                    voiceActive = state.mic == "preparing" || state.mic == "transmitting",
                    scale = dimensions.value.scale,
                    hold = keyHold,
                    beginVoice = ::beginMic,
                    stopVoice = { vm.client.stopMic() },
                )
            }.apply { id = R.id.keyboard_region }
            panelView = FrameLayout(context).apply {
                id = R.id.control_panel
                isMotionEventSplittingEnabled = true
                setBackgroundColor(ControllerStyle.PANEL)
                addView(ordinaryPanel, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
                addView(keyboardRegion, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
            }
            addView(panelView, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0))
            modeViews = {
                val on = vm.keyboardOn.value
                if (on) shortcutHold.cancelAll()
                ordinaryPanel.visibility = if (on) View.GONE else View.VISIBLE
                keyboardRegion.visibility = if (on) View.VISIBLE else View.GONE
            }
            modeViews?.invoke()
        }
        val dialogs = compose(compact = false) {
            val state by vm.client.state.collectAsStateWithLifecycle()
            val inputSettings by vm.client.inputSettings.collectAsStateWithLifecycle()
            if (sensitivitySettings) SensitivitySettings(inputSettings, vm.client::setSensitivity) { sensitivitySettings = false }
            if (settings) AlertDialog(onDismissRequest = { settings = false }, title = {
                Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                    Text("连接与设备设置")
                    Text("TapDeck ${BuildConfig.VERSION_NAME}（${BuildConfig.VERSION_CODE}）", style = MaterialTheme.typography.bodySmall)
                }
            }, text = {
                Column(Modifier.verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                        Text("按键震动反馈", Modifier.weight(1f))
                        Switch(checked = inputSettings.haptics, onCheckedChange = { on ->
                            vm.client.setHaptics(on)
                            feedbackTestMessage = ""
                            if (on) window.decorView.keyFeedback(KeyFeedback.Press, feedbackController)
                        },
                            modifier = Modifier.semantics { contentDescription = "按键震动反馈" })
                    }
                    Button(onClick = {
                        feedbackAvailability = feedbackController.availability()
                        feedbackTestMessage = when (window.decorView.keyFeedback(KeyFeedback.Press, feedbackController)) {
                            FeedbackResult.Requested -> "已发送测试震动"
                            FeedbackResult.AppDisabled -> "请先开启按键震动反馈"
                            FeedbackResult.NoVibrator -> "这台设备没有振动马达"
                            FeedbackResult.Failed -> "未能触发震动，请检查手机振动设置"
                        }
                    }, contentPadding = PaddingValues(horizontal = 10.dp, vertical = 8.dp)) { Text("测试震动", maxLines = 1) }
                    if (feedbackTestMessage.isNotEmpty()) Text(feedbackTestMessage, style = MaterialTheme.typography.bodySmall)
                    if (feedbackAvailability == FeedbackResult.NoVibrator && feedbackTestMessage != "这台设备没有振动马达") {
                        Text("这台设备没有振动马达", style = MaterialTheme.typography.bodySmall)
                    }
                    HorizontalDivider()
                    Text("输入PC设置窗口显示的配对网址：")
                    OutlinedTextField(value = address, onValueChange = { address = it; pairingScanError = "" },
                        modifier = Modifier.fillMaxWidth(), label = { Text("PC 配对网址") }, singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri))
                    if (pairingScanError.isNotEmpty()) Text(pairingScanError, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                    Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                        if (state.connected) TextButton(onClick = { vm.client.forget() }) { Text("忘记当前电脑") }
                        Spacer(Modifier.weight(1f))
                        IconButton(onClick = ::scanPairingAddress) {
                            Icon(painterResource(R.drawable.ic_lucide_scan_line), contentDescription = "扫码填写 PC 配对网址")
                        }
                    }
                }
            }, confirmButton = { TextButton(onClick = { enter(address); settings = false }) { Text("连接") } }, dismissButton = { TextButton(onClick = { settings = false }) { Text("关闭") } })
            if (state.pairing.isNotEmpty()) AlertDialog(onDismissRequest = { vm.client.forget() }, title = { Text("等待电脑允许连接") }, text = { Column { Text("请核对 PC 设置窗口里的校验码是否一致，一致后在电脑上点“允许连接”："); Spacer(Modifier.height(12.dp)); Text(state.pairing, fontSize = 19.sp); Spacer(Modifier.height(12.dp)); Text("电脑允许后会自动连上，手机上不需要额外操作。") } }, confirmButton = {}, dismissButton = { TextButton(onClick = vm.client::forget) { Text("取消") } })
        }
        val root = FrameLayout(this).apply {
            isMotionEventSplittingEnabled = true
            // Native regions are measured inside these system-bar/cutout safe insets.
            setOnApplyWindowInsetsListener { view, insets ->
                val safe = WindowInsetsCompat.toWindowInsetsCompat(insets, view)
                val bars = safe.getInsets(WindowInsetsCompat.Type.systemBars())
                val cutout = safe.getInsets(WindowInsetsCompat.Type.displayCutout())
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
    private fun scanPairingAddress() {
        pairingScanError = ""
        if (!packageManager.hasSystemFeature(PackageManager.FEATURE_CAMERA_ANY)) {
            pairingScanError = "这台设备没有相机，请手动输入配对网址"
        } else if (checkSelfPermission(Manifest.permission.CAMERA) == PackageManager.PERMISSION_GRANTED) {
            launchPairingScanner()
        } else {
            cameraPermission.launch(Manifest.permission.CAMERA)
        }
    }
    private fun launchPairingScanner() {
        pairingScanner.launch(ScanOptions()
            .setDesiredBarcodeFormats(ScanOptions.QR_CODE)
            .setPrompt("扫描 PC 设置窗口中的配对二维码")
            .setOrientationLocked(false)
            .setBeepEnabled(false))
    }
    override fun onSaveInstanceState(outState: Bundle) {
        outState.putBoolean("connection_settings", settings)
        outState.putString("pairing_address", address)
        outState.putString("pairing_scan_error", pairingScanError)
        super.onSaveInstanceState(outState)
    }
    private fun enter(link: String) {
        val permission = "android.permission.ACCESS_LOCAL_NETWORK"
        if (Build.VERSION.SDK_INT >= 37 && applicationInfo.targetSdkVersion >= 37 && checkSelfPermission(permission) != PackageManager.PERMISSION_GRANTED) { deferredLink = link; lanPermission.launch(permission) } else vm.client.enter(link)
    }
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
    override fun onResume() {
        super.onResume(); pageResumed = true; fullscreen(); modeViews?.invoke()
        if (settings) { feedbackAvailability = feedbackController.availability(); feedbackTestMessage = "" }
    }
    override fun onPause() { pageResumed = false; super.onPause() }
    override fun onStart() { super.onStart(); vm.client.setForeground(true) }
    override fun onStop() { touchpad?.cancel(); microphone?.cancel(); vm.client.stopMic(true); vm.client.setForeground(false); super.onStop() }
}

@Composable private fun SensitivitySettings(settings: DeviceInputSettings, change: (Double) -> Unit, close: () -> Unit) {
    AlertDialog(onDismissRequest = close, title = { Text("触控板灵敏度") }, text = {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text("1.0× 为默认速度，仅影响鼠标移动。")
            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                Slider(value = settings.sensitivity.toFloat(), onValueChange = { change(it.toDouble()) },
                    valueRange = DeviceInputSettings.MIN.toFloat()..DeviceInputSettings.MAX.toFloat(), steps = 24,
                    modifier = Modifier.weight(1f).semantics { contentDescription = "触控板灵敏度滑块" })
                Spacer(Modifier.width(12.dp))
                Text(settings.sensitivityLabel, modifier = Modifier.width(52.dp))
            }
        }
    }, confirmButton = { TextButton(onClick = close) { Text("完成") } },
        dismissButton = { TextButton(onClick = { change(1.0) }) { Text("恢复 1.0×") } })
}

@Composable private fun ConnectionHeader(
    state: ClientState,
    keyboardOn: Boolean,
    scale: Float,
    reminderEnabled: Boolean,
    onKeyboardToggle: (Boolean) -> Unit,
    onSettings: () -> Unit,
) {
    val feedback = LocalKeyFeedback.current
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val width = maxWidth
        val titleSize = widthFont(width, 0.032f, scale)
        val iconSize = width * (0.055f * scale)
        val jump = remember { Animatable(0f) }
        val amplitude = with(LocalDensity.current) { (width * (ConnectionReminder.AMPLITUDE * scale)).toPx() }
        LaunchedEffect(reminderEnabled) {
            jump.snapTo(0f)
            if (reminderEnabled) ConnectionReminder.run { target, duration ->
                jump.animateTo(target, tween(durationMillis = duration, easing = FastOutSlowInEasing))
            }
        }
        Row(
            Modifier.fillMaxSize().background(Color.White).padding(start = width * 0.02f, end = width * 0.01f),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                text = if (state.error.isNotEmpty()) "${state.peerName} · ${state.error}"
                else if (state.connected) "${state.peerName} · ${state.status} · RTT ${state.rttMs} ms"
                else "${state.peerName} · ${state.status}",
                modifier = Modifier.weight(1f),
                color = Color(ControllerStyle.LABEL),
                fontSize = titleSize, lineHeight = titleSize * 1.2f, maxLines = 1, overflow = TextOverflow.Ellipsis,
            )
            // Explicit click bounds avoid Material's minimum size enlarging a 0.10W header.
            Box(
                Modifier.width(width * 0.10f).fillMaxHeight()
                    .clip(RoundedCornerShape(width * ControllerStyle.CORNER))
                    .clickable(role = Role.Button) { feedback(KeyFeedback.Press); onKeyboardToggle(!keyboardOn) }
                    .semantics { contentDescription = if (keyboardOn) "全键盘已激活，点击返回快捷键与语音输入" else "切换到全键盘" },
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    painter = painterResource(R.drawable.ic_lucide_keyboard),
                    contentDescription = null,
                    tint = if (keyboardOn) Color(0xFF000000) else Color(0xFFAAAAAA),
                    modifier = Modifier.size(iconSize),
                )
            }
            Spacer(Modifier.width(width * 0.01f))
            Box(
                Modifier.width(width * 0.10f).fillMaxHeight()
                    .clip(RoundedCornerShape(width * ControllerStyle.CORNER))
                    .clickable(role = Role.Button, onClick = onSettings)
                    .semantics { contentDescription = "连接设置与手机震动设置，${if (state.connected) "已连接" else "未连接"}" },
                contentAlignment = Alignment.Center,
            ) {
                Icon(
                    painter = painterResource(R.drawable.ic_lucide_plug),
                    contentDescription = null,
                    tint = Color(if (state.connected) ControllerStyle.CONNECTED else ControllerStyle.DISCONNECTED),
                    modifier = Modifier.size(iconSize).graphicsLayer {
                        translationY = if (reminderEnabled) jump.value * amplitude else 0f
                    },
                )
            }
        }
    }
}

@Composable internal fun ShortcutButtons(state: ClientState, hold: ShortcutHold, scale: Float, stopRecording: () -> Boolean) {
    val feedback by rememberUpdatedState(LocalKeyFeedback.current)
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val visible = state.config.visibleShortcuts()
        val rows = if (visible.size > 4) 2 else 1
        val columns = if (rows == 2) 4 else visible.size.coerceAtLeast(1)
        val width = maxWidth
        val side = width * KeyboardGeometry.SIDE.toFloat()
        val gap = width * KeyboardGeometry.GAP.toFloat()
        val rowGap = gap * scale
        val cellWidth = (width.value - side.value * 2 - (columns - 1) * gap.value) / columns
        // 上外边距复用原生面板已有的 1% 留白，避免与全键盘相比多算一次。
        val cellHeight = (maxHeight.value - rowGap.value * rows) / rows
        val titleSize = with(LocalDensity.current) {
            min(width.value * 0.035f * scale, min(cellWidth * 0.26f, cellHeight * 0.32f)).coerceAtLeast(0f).dp.toSp()
        }
        val chordSize = with(LocalDensity.current) {
            min(width.value * 0.022f * scale, min(cellWidth * 0.17f, cellHeight * 0.22f)).coerceAtLeast(0f).dp.toSp()
        }
        Column(Modifier.fillMaxSize().background(Color(ControllerStyle.PANEL)).padding(start = side, end = side, bottom = rowGap),
            verticalArrangement = Arrangement.spacedBy(rowGap)) {
            repeat(rows) { row ->
                Row(Modifier.weight(1f).fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(gap)) {
                    repeat(columns) { column ->
                        val item = visible.getOrNull(row * columns + column)
                        if (item == null) Spacer(Modifier.weight(1f).fillMaxHeight())
                        else {
                            var pressed by remember(state.connected, item.index) { mutableStateOf(false) }
                            Box(
                                Modifier.weight(1f).fillMaxHeight()
                                    .semantics(mergeDescendants = true) {
                                        contentDescription = "快捷键 ${item.index + 1}：${item.value.label}"
                                        role = Role.Button
                                        if (!state.connected) disabled()
                                        onClick {
                                            if (!state.connected) false else {
                                                feedback(KeyFeedback.Press)
                                                hold.press(item.index, stopRecording())
                                                hold.release(item.index)
                                                true
                                            }
                                        }
                                    }
                                    .shortcutPress(state.connected, item.index, hold, stopRecording, { feedback(it) }) { pressed = it },
                            ) {
                                // 自绘按钮外观：不用 Button，避免它消费抬手事件而收不到释放。
                                KeySurface(
                                    modifier = Modifier.fillMaxSize(),
                                    viewportWidth = width,
                                    active = pressed,
                                ) {
                                    Column(
                                        modifier = Modifier.fillMaxSize().padding(width * (0.005f * scale)),
                                        horizontalAlignment = Alignment.CenterHorizontally,
                                        verticalArrangement = Arrangement.Center,
                                    ) {
                                        Text(item.value.label, color = if (pressed) Color.White else Color(ControllerStyle.LABEL).copy(alpha = if (state.connected) 1f else 0.55f), fontSize = titleSize, lineHeight = titleSize * 1.2f, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                        Text(item.value.chord, color = if (pressed) Color.White.copy(alpha = 0.9f) else Color(ControllerStyle.SECONDARY), fontSize = chordSize, lineHeight = chordSize * 1.2f, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                    }
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
    feedback: (KeyFeedback) -> Unit,
    onPressed: (Boolean) -> Unit,
): Modifier = pointerInput(connected, slot) {
    awaitEachGesture {
        awaitFirstDown(requireUnconsumed = false)
        if (!connected) return@awaitEachGesture
        try {
            feedback(KeyFeedback.Press)
            val wasRecording = stopRecording()
            onPressed(!wasRecording)
            hold.press(slot, wasRecording)
            waitForUpOrCancellation()
        } finally {
            hold.release(slot)
            onPressed(false)
        }
    }
}
