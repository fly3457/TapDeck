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
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalDensity
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

object Regions { const val CONNECTION = 0.10f; const val TOUCHPAD = 0.50f; const val SHORTCUTS = 0.20f; const val MICROPHONE = 0.20f }
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
            fun region(view: android.view.View, weight: Float) = addView(view, LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, 0, weight))
            region(compose {
                val state by vm.client.state.collectAsStateWithLifecycle()
                ConnectionHeader(state) { settings = true }
            }, Regions.CONNECTION)
            region(TouchpadView(context, vm.client).also { touchpad = it }, Regions.TOUCHPAD)
            region(compose {
                val state by vm.client.state.collectAsStateWithLifecycle()
                ShortcutButtons(state, vm.client::shortcut)
            }, Regions.SHORTCUTS)
            region(MicBallView(context, ::beginMic, vm.client::stopMic, vm::saveBallPosition).also { microphone = it }, Regions.MICROPHONE)
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
        setContentView(FrameLayout(this).apply {
            isMotionEventSplittingEnabled = true
            addView(regions, FrameLayout.LayoutParams(FrameLayout.LayoutParams.MATCH_PARENT, FrameLayout.LayoutParams.MATCH_PARENT))
            // Dialogs own their windows; this composition needs no screen area.
            addView(dialogs, FrameLayout.LayoutParams(0, 0))
        })
        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                combine(vm.client.state, vm.ballPosition) { state, position -> state to position }.collect { (state, position) ->
                    touchpad?.connected = state.connected
                    microphone?.apply {
                        available = state.connected; mode = state.micMode; status = state.mic; level = state.level
                        position?.let { restorePosition(it.first, it.second) }
                    }
                }
            }
        }
        intent?.data?.let { enter(it.toString()) }
    }
    private fun fullscreen() {
        WindowCompat.setDecorFitsSystemWindows(window, false)
        window.addFlags(WindowManager.LayoutParams.FLAG_KEEP_SCREEN_ON)
        if (Build.VERSION.SDK_INT >= 28) window.attributes = window.attributes.apply { layoutInDisplayCutoutMode = WindowManager.LayoutParams.LAYOUT_IN_DISPLAY_CUTOUT_MODE_SHORT_EDGES }
        WindowInsetsControllerCompat(window, window.decorView).apply { systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE; hide(WindowInsetsCompat.Type.systemBars()) }
    }
    private fun enter(link: String, scanned: Boolean = false) {
        val permission = "android.permission.ACCESS_LOCAL_NETWORK"
        if (Build.VERSION.SDK_INT >= 37 && applicationInfo.targetSdkVersion >= 37 && checkSelfPermission(permission) != PackageManager.PERMISSION_GRANTED) { deferredLink = link; scannedLink = scanned; lanPermission.launch(permission) } else vm.client.enter(link, scanned)
    }
    private fun scan() { scanner.launch(ScanOptions().setDesiredBarcodeFormats(ScanOptions.QR_CODE).setPrompt("扫描 TapDeck 电脑端二维码").setBeepEnabled(false).setOrientationLocked(true)) }
    private fun beginMic(mode: String): Boolean {
        if (checkSelfPermission(Manifest.permission.RECORD_AUDIO) != PackageManager.PERMISSION_GRANTED) {
            microphonePermission.launch(Manifest.permission.RECORD_AUDIO)
            return false
        }
        return vm.client.startMic(mode)
    }
    override fun onNewIntent(intent: Intent) { super.onNewIntent(intent); setIntent(intent); intent.data?.let { enter(it.toString()) } }
    override fun onResume() { super.onResume(); fullscreen() }
    override fun onStart() { super.onStart(); vm.client.setForeground(true) }
    override fun onStop() { touchpad?.cancel(); microphone?.cancel(); vm.client.stopMic(true); vm.client.setForeground(false); super.onStop() }
}

@Composable private fun ConnectionHeader(state: ClientState, settings: () -> Unit) {
    BoxWithConstraints(Modifier.fillMaxSize()) {
        val scale = LocalDensity.current.fontScale
        val titleSize = min(19f, maxHeight.value * 0.30f / scale).sp
        val statusSize = min(12f, maxHeight.value * 0.23f / scale).sp
        Row(Modifier.fillMaxSize().windowInsetsPadding(WindowInsets.displayCutout.only(WindowInsetsSides.Top + WindowInsetsSides.Horizontal)).padding(horizontal = 12.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(state.peerName, fontSize = titleSize, lineHeight = titleSize * 1.25f, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(if (state.error.isNotEmpty()) state.error else "${state.status} · RTT ${state.rttMs} ms", fontSize = statusSize, lineHeight = statusSize * 1.25f, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            TextButton(onClick = settings, contentPadding = PaddingValues(horizontal = 8.dp, vertical = 0.dp)) { Text("连接", fontSize = titleSize, lineHeight = titleSize * 1.25f) }
        }
    }
}

@Composable private fun ShortcutButtons(state: ClientState, press: (Int) -> Unit) {
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
                        else OutlinedButton(
                            onClick = { press(item.index) }, enabled = state.connected,
                            modifier = Modifier.weight(1f).fillMaxHeight().semantics { contentDescription = "快捷键 ${item.index + 1}：${item.value.label}" },
                            contentPadding = PaddingValues(3.dp), shape = MaterialTheme.shapes.small,
                        ) {
                            Column(horizontalAlignment = Alignment.CenterHorizontally) {
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
