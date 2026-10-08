package com.yuncii.tapdeck

import android.app.Application
import android.os.Build
import android.os.SystemClock
import android.util.Log
import android.widget.Toast
import java.net.DatagramPacket
import java.net.DatagramSocket
import java.net.InetAddress
import java.net.URI
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.security.MessageDigest
import java.security.SecureRandom
import java.security.cert.X509Certificate
import javax.net.ssl.SSLContext
import javax.net.ssl.X509TrustManager
import kotlinx.coroutines.*
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.serialization.json.*
import okhttp3.*

data class ClientState(val status: String = "未连接", val peerName: String = "TapDeck", val connected: Boolean = false, val pairing: String = "", val pairingConfirmed: Boolean = false, val config: PcConfig = PcConfig(), val mic: String = "idle", val micMode: String = "", val activeVoiceProfile: VoiceProfile? = null, val voiceProfilesSupported: Boolean = false, val level: Float = 0f, val error: String = "", val rttMs: Long = 0, val touchpad: TouchpadCapabilities = TouchpadCapabilities()) {
    fun voiceProfiles() = config.voiceProfiles(voiceProfilesSupported)
}
class TapClient(private val app: Application, private val scope: CoroutineScope) : TouchSink {
    private companion object {
        /** 自动重连的退避间隔（毫秒），最后一次会一直沿用。 */
        val RETRY_DELAYS = longArrayOf(1000, 2000, 4000, 8000, 15000, 30000)
        /** App 不在前台时的重试间隔，避免后台无谓地反复连接。 */
        const val IDLE_RETRY_MS = 30000L
    }
    private val store = PairStore(app)
    private val pairingPersistence = Mutex()
    private var revokedCredential: Peer? = null
    private val mutable = MutableStateFlow(ClientState())
    val state = mutable.asStateFlow()
    private val inputPreferences = DeviceInputPreferences(store::loadInputSettings, store::saveInputSettings) { error ->
        Log.w("TapDeck", "Device input settings could not be saved/loaded", error)
        mutable.update { it.copy(error = "设备设置读写失败，请重试") }
    }
    val inputSettings = inputPreferences.state
    fun setSensitivity(value: Double) { inputPreferences.update { it.copy(sensitivity = value) } }
    fun setHaptics(on: Boolean) { inputPreferences.update { it.copy(haptics = on) } }
    override val doubleClickMs: Int get() = state.value.touchpad.doubleClickMs
    private var lastTouchpadNotice = -3000L
    private val lock = Any()
    private var socket: WebSocket? = null
    private var udp: DatagramSocket? = null
    private var peer: Peer? = null
    private var target: InetAddress? = null
    private var udpPort = 0
    private var mouseCodec: UdpCodec? = null
    private var audioCodec: UdpCodec? = null
    private var generation = 0L
    private var desiredConnection = 0L
    private var epoch = 0
    private var x = 0L; private var y = 0L; private var sx = 0L; private var sy = 0L
    private var connected = false
    private var foreground = false
    @Suppress("DEPRECATION") private val wifiLock = runCatching {
        app.getSystemService(android.net.wifi.WifiManager::class.java)?.createWifiLock(if (Build.VERSION.SDK_INT >= 29) android.net.wifi.WifiManager.WIFI_MODE_FULL_LOW_LATENCY else android.net.wifi.WifiManager.WIFI_MODE_FULL_HIGH_PERF, "TapDeck:input")?.apply { setReferenceCounted(false) }
    }.getOrNull()
    private fun updateWifiLock() {
        runCatching { if (connected && foreground) { if (wifiLock?.isHeld == false) wifiLock.acquire() } else if (wifiLock?.isHeld == true) wifiLock.release() }
            .onFailure { Log.w("TapDeck", "Wi-Fi latency lock unavailable", it) }
    }
    fun setForeground(value: Boolean) = synchronized(lock) {
        val becameForeground = value && !foreground
        foreground = value
        updateWifiLock()
        // 回到前台仍未连接时立即重试，不必等退避计时。
        if (becameForeground && reconnect && peer != null && !connected) { retryAt = 0; scheduleReconnect() }
    }
    @Volatile private var lastResponse = 0L
    @Volatile private var reconnect = true
    private var heartbeat: Job? = null
    private var reconnectJob: Job? = null
    // 自动重连的退避状态：开机顺序、Wi-Fi 尚未就绪或电脑接收端未启动时都要继续尝试。
    private var retryCount = 0
    private var retryAt = 0L
    private var retryNotice = ""
    private var networkCallback: android.net.ConnectivityManager.NetworkCallback? = null
    private val movement = Channel<Pair<Long, Movement>>(Channel.CONFLATED)
    private val audio = Channel<ByteArray>(6)
    private var recording = ""
    private var recordingRequested = false
    private val voiceMouseButtons = mutableSetOf<String>()
    private val recorder = MicCapture(frame = { pcm, pos, level ->
        val id = synchronized(lock) { if (!recordingRequested || !connected) return@MicCapture; recording }
        if (id.isEmpty()) return@MicCapture
        val body = ByteBuffer.allocate(976).order(ByteOrder.LITTLE_ENDIAN).putLong(id.toULong(16).toLong()).putLong(pos).put(pcm).array()
        synchronized(lock) {
            if (!recordingRequested || recording != id) return@MicCapture
            audio.trySend(body)
            if (pos % 4800L == 0L) mutable.update { it.copy(level = level) }
        }
    }, onError = { reason -> stopMic(true); mutable.update { it.copy(error = reason) } })
    private val http = OkHttpClient.Builder().connectTimeout(5, java.util.concurrent.TimeUnit.SECONDS).readTimeout(5, java.util.concurrent.TimeUnit.SECONDS).followRedirects(false).followSslRedirects(false).build()
    init {
        scope.launch(Dispatchers.IO) { for ((gen, m) in movement) synchronized(lock) { if (connected && gen == generation) runCatching { val bytes = mouseCodec!!.seal(m.bytes()); udp!!.send(DatagramPacket(bytes, bytes.size, target, udpPort)) }.onFailure { Log.w("TapDeck", "mouse UDP failed", it) } } }
        scope.launch(Dispatchers.IO) { for (body in audio) synchronized(lock) { if (connected && recordingRequested && recording.isNotEmpty() && ByteBuffer.wrap(body).order(ByteOrder.LITTLE_ENDIAN).long == recording.toULong(16).toLong()) runCatching { val bytes = audioCodec!!.seal(body); udp!!.send(DatagramPacket(bytes, bytes.size, target, udpPort)) }.onFailure { Log.w("TapDeck", "audio UDP failed", it) } } }
        registerNetworkCallback()
    }
    private fun registerNetworkCallback() {
        val manager = app.getSystemService(android.net.ConnectivityManager::class.java) ?: return
        val callback = object : android.net.ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: android.net.Network) = wakeForReconnect("网络已连接")
            override fun onLost(network: android.net.Network) { }
        }
        runCatching { manager.registerDefaultNetworkCallback(callback); networkCallback = callback }
            .onFailure { Log.w("TapDeck", "network callback unavailable", it) }
    }
    /** Wi-Fi 状态变化时立即重试，不必等下一次退避。 */
    private fun wakeForReconnect(reason: String) {
        synchronized(lock) {
            if (!reconnect || peer == null || connected) return
            retryAt = 0
            retryCount = 0
            retryNotice = reason
            scheduleReconnect()
        }
    }
    /**
     * Keeps trying to reach the saved peer until it connects or the user acts.
     * The delay grows from 1 s to 30 s, and a foreground or network change cuts
     * it short. Must be called with [lock] held.
     */
    private fun scheduleReconnect() {
        val saved = peer ?: return
        if (!reconnect) return
        val timer = reconnectJob?.isActive == true
        if (timer && retryAt != 0L) return
        if (retryAt == 0L) retryAt = SystemClock.elapsedRealtime() + backoffDelay()
        val attempt = desiredConnection
        reconnectJob = scope.launch(Dispatchers.IO) {
            while (isActive) {
                val wait = synchronized(lock) { (retryAt - SystemClock.elapsedRealtime()).coerceAtLeast(0) }
                if (wait > 0) { delay(wait); continue }
                val current = synchronized(lock) { if (!reconnect || attempt != desiredConnection || connected) null else peer }
                if (current == null) return@launch
                connect(current, attempt)
                return@launch
            }
        }
    }
    private fun backoffDelay(): Long {
        if (!foreground) return IDLE_RETRY_MS
        val delay = RETRY_DELAYS[retryCount.coerceAtMost(RETRY_DELAYS.size - 1)]
        retryCount++
        return delay
    }
    suspend fun restore() {
        val attempt = synchronized(lock) { desiredConnection }
        store.load()?.let {
            if (Build.VERSION.SDK_INT >= 37 && app.applicationInfo.targetSdkVersion >= 37 && app.checkSelfPermission("android.permission.ACCESS_LOCAL_NETWORK") != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                mutable.update { it.copy(status = "请打开连接页授予局域网权限") }; return
            }
            synchronized(lock) { if (attempt != desiredConnection) return; peer = it; reconnect = true }
            connect(it, attempt)
            // 首次恢复失败（电脑还没开机、Wi-Fi 尚未就绪等）时继续按退避重试。
            synchronized(lock) { if (reconnect && !connected && peer != null) scheduleReconnect() }
        }
    }
    fun enter(raw: String) {
        val attempt = synchronized(lock) { ++desiredConnection }
        reconnectJob?.cancel(); reconnect = false; disconnect("正在连接")
        scope.launch(Dispatchers.IO) {
            try {
                val text = raw.trim()
                val p: Peer
                if (text.startsWith("tapdeck://")) {
                    val u = android.net.Uri.parse(text)
                    require(u.host == "pair") { "无效配对链接" }
                    require(u.getQueryParameter("v")?.toIntOrNull() == CONTROL_VERSION) { "协议版本不匹配，请同时升级 PC 和 Android 至 TapDeck 0.2" }
                    val host = u.getQueryParameter("host") ?: error("缺少地址")
                    p = Peer(host, u.getQueryParameter("wss")?.toIntOrNull() ?: 41443, u.getQueryParameter("http")?.toIntOrNull() ?: 41080, u.getQueryParameter("pin") ?: error("缺少服务器指纹"))
                } else {
                    val u = URI(if (text.contains("://")) text else "http://$text")
                    require(u.scheme == "http" && u.host != null && u.userInfo == null) { "请输入 PC 的 http 配对网址" }
                    val host = u.host
                    validateHost(host)
                    val port = if (u.port == -1) 41080 else u.port
                    val response = http.newCall(Request.Builder().url("http://$host:$port/api/pair-info").build()).execute()
                    val m = response.use { require(it.isSuccessful) { "电脑返回 ${it.code}" }; wireJson.parseToJsonElement(it.body!!.string()).jsonObject }
                    require(m.long("version") == CONTROL_VERSION.toLong()) { "协议版本不匹配，请同时升级 PC 和 Android 至 TapDeck 0.2" }
                    p = Peer(host, m.long("wss_port").toInt(), port, m.str("pin"), m.str("name", "电脑"))
                }
                validateHost(p.host); require(p.pin.matches(Regex("[0-9a-fA-F]{64}"))) { "服务器指纹无效" }; require(p.wssPort in 1024..65535 && p.httpPort in 1024..65535) { "端口无效" }
                val existing = pairingPersistence.withLock { store.load() }
                val usable = synchronized(lock) { existing?.takeUnless { saved -> revokedCredential?.sameCredential(saved) == true } }
                val target = p.withCredentialFrom(usable)
                synchronized(lock) { if (attempt != desiredConnection) return@launch; reconnect = true; retryCount = 0; retryAt = 0L; peer = target }
                connect(target, attempt)
            } catch (e: Exception) { mutable.update { it.copy(status = "连接失败", error = e.message ?: "连接失败") } }
        }
    }
    private fun validateHost(host: String) { val a = InetAddress.getByName(host); require(a is java.net.Inet4Address && (a.isSiteLocalAddress || a.isLoopbackAddress)) { "原型仅支持局域网 IPv4 地址" } }
    private suspend fun connect(p: Peer, attempt: Long) {
        val gen = synchronized(lock) { if (attempt != desiredConnection) return; generation++; generation }
        mutable.update { it.copy(status = "正在连接 ${p.host}", pairing = "", pairingConfirmed = false, error = "") }
        // Check restored peers too: older receivers reject hello silently.
        try {
            val version = withContext(Dispatchers.IO) {
                http.newCall(Request.Builder().url("http://${p.host}:${p.httpPort}/api/pair-info").build()).execute().use {
                    require(it.isSuccessful) { "电脑返回 ${it.code}" }
                    wireJson.parseToJsonElement(it.body!!.string()).jsonObject.long("version")
                }
            }
            if (synchronized(lock) { gen != generation || attempt != desiredConnection }) return
            if (version != CONTROL_VERSION.toLong()) {
                reconnect = false
                mutable.update { it.copy(status = "版本不匹配", error = "请同时升级 PC 和 Android 至 TapDeck 0.2") }
                return
            }
        } catch (e: CancellationException) { throw e } catch (e: Exception) { lost(gen, e.message ?: "连接失败"); return }
        val id = store.deviceId(); val clientNonce = ByteArray(32).also { SecureRandom().nextBytes(it) }
        val manager = object : X509TrustManager {
            override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
            override fun checkClientTrusted(chain: Array<X509Certificate>, authType: String) { throw java.security.cert.CertificateException("server only") }
            override fun checkServerTrusted(chain: Array<X509Certificate>, authType: String) {
                if (chain.isEmpty()) throw java.security.cert.CertificateException("empty chain")
                chain[0].checkValidity()
                val pin = MessageDigest.getInstance("SHA-256").digest(chain[0].publicKey.encoded).joinToString("") { "%02x".format(it.toInt() and 255) }
                if (!pin.equals(p.pin, true)) throw java.security.cert.CertificateException("服务器指纹不匹配，请重新配对")
            }
        }
        val tls = SSLContext.getInstance("TLS").apply { init(null, arrayOf(manager), SecureRandom()) }
        val client = http.newBuilder().sslSocketFactory(tls.socketFactory, manager).readTimeout(0, java.util.concurrent.TimeUnit.SECONDS).build()
        val ws = client.newWebSocket(Request.Builder().url("wss://${p.host}:${p.wssPort}/ws").build(), object : WebSocketListener() {
            override fun onOpen(ws: WebSocket, response: Response) {
                if (synchronized(lock) { gen != generation }) { ws.cancel(); return }
                sendOn(ws, message("hello", "version" to CONTROL_VERSION.j(), "device_id" to id.j(), "name" to "${Build.MANUFACTURER} ${Build.MODEL}".j(), "token" to p.token.j(), "client_nonce" to encode64(clientNonce).j()))
            }
            override fun onMessage(ws: WebSocket, text: String) {
                if (synchronized(lock) { gen != generation }) return
                lastResponse = SystemClock.elapsedRealtime()
                runCatching {
                    val m = wireJson.parseToJsonElement(text).jsonObject
                    when (m.str("type")) {
                        "pair_challenge" -> {
                            val code = comparisonCode(p.pin.lowercase(), clientNonce, decode64(m.str("server_nonce")))
                            require(code == m.str("code")) { "配对校验失败" }
                            // 手机上只要看到校验码，不用点确认：由 PC 端核对并允许。
                            // 仍然回一条 pair_confirm（老版本接收端需要它才会等待 PC 允许）。
                            mutable.update { it.copy(status = "等待电脑允许连接", pairing = code, pairingConfirmed = true) }
                            send(message("pair_confirm", "code" to code.j()))
                        }
                        "ready" -> {
                            val saved = p.copy(token = m.str("token").ifEmpty { p.token })
                            val config = wireJson.decodeFromJsonElement<PcConfig>(m["config"]!!).validate()
                            val profilesSupported = VOICE_PROFILES_FEATURE in m.touchpadCapabilities().features
                            config.voiceProfiles(profilesSupported)
                            synchronized(lock) {
                                if (gen != generation || attempt != desiredConnection) return
                                revokedCredential = null
                                udp?.close(); udp = DatagramSocket(); target = InetAddress.getByName(p.host); udpPort = m.long("udp_port").toInt()
                                val session = m.str("session").toULong(16).toLong()
                                mouseCodec = UdpCodec(decode64(m.str("mouse_key")), decode64(m.str("mouse_prefix")), session, 1)
                                audioCodec = UdpCodec(decode64(m.str("audio_key")), decode64(m.str("audio_prefix")), session, 2)
                                epoch = 0; x = 0; y = 0; sx = 0; sy = 0; connected = true; peer = saved
                                retryCount = 0; retryAt = 0L; retryNotice = ""
                                updateWifiLock()
                            }
                            scope.launch(Dispatchers.IO) { pairingPersistence.withLock { if (synchronized(lock) { gen == generation && attempt == desiredConnection }) store.save(saved) } }
                            mutable.update { it.copy(status = "已连接", peerName = p.name, connected = true, pairing = "", pairingConfirmed = false, config = config, error = "", touchpad = m.touchpadCapabilities(), voiceProfilesSupported = profilesSupported) }
                            heartbeat?.cancel(); heartbeat = scope.launch(Dispatchers.IO) {
                                while (isActive && synchronized(lock) { gen == generation && connected }) {
                                    if (SystemClock.elapsedRealtime() - lastResponse >= 1000) {
                                        ws.cancel(); lost(gen, "电脑心跳超时"); break
                                    }
                                    send(message("heartbeat", "tick" to SystemClock.elapsedRealtime().j())); delay(250)
                                }
                            }
                        }
                        "config" -> { val c = wireJson.decodeFromJsonElement<PcConfig>(m["config"]!!).validate(); c.voiceProfiles(mutable.value.voiceProfilesSupported); mutable.update { it.copy(config = c) } }
                        "heartbeat" -> mutable.update { it.copy(rttMs = (SystemClock.elapsedRealtime() - m.long("tick")).coerceAtLeast(0)) }
                        "mic_ready" -> synchronized(lock) {
                            if (m.str("recording") == recording && recordingRequested && foreground) {
                                try { recorder.start(); mutable.update { it.copy(mic = "transmitting", error = "") } } catch (e: Exception) { stopMic(true); mutable.update { it.copy(error = e.message ?: "录音失败") } }
                            } else send(message("mic_abort", "recording" to m.str("recording").j()))
                        }
                        "mic_error" -> finishMic(m.str("recording"), m.str("reason"))
                        "mic_stopped" -> finishMic(m.str("recording"))
                        "error" -> {
                            if (m.str("code") == "pairing_revoked") { revokePair(gen, m.str("reason")); return }
                            if (finishPairingAttempt(gen, m.str("code"), m.str("reason"))) return
                            if (m.str("code") == "version_mismatch") reconnect = false
                            mutable.update { it.copy(error = m.str("reason")) }
                            if (m.str("code") == "touchpad_error") touchpadNotice(m.str("reason"))
                        }
                    }
                }.onFailure { error -> mutable.update { it.copy(error = error.message ?: "协议错误") }; ws.cancel() }
            }
            override fun onFailure(ws: WebSocket, t: Throwable, response: Response?) { lost(gen, t.message ?: "连接已断开") }
            override fun onClosed(ws: WebSocket, code: Int, reason: String) { lost(gen, reason.ifEmpty { "连接已断开" }) }
            override fun onClosing(ws: WebSocket, code: Int, reason: String) { ws.close(code, reason) }
        })
        synchronized(lock) { if (gen == generation) socket = ws else ws.cancel() }
    }
    /**
     * 配对不再需要手机确认：校验码只用于 PC 端核对，收到后自动回一条 pair_confirm
     * （兼容老接收端）并进入「等待电脑允许连接」。保留此方法以便手动重发。
     */
    fun confirmPair() { val code = mutable.value.pairing; if (code.isNotEmpty()) { send(message("pair_confirm", "code" to code.j())); mutable.update { it.copy(pairingConfirmed = true, status = "等待电脑允许连接") } } }
    internal fun finishPairingAttempt(gen: Long, code: String, reason: String): Boolean {
        if (code != "pairing_rejected" && code != "pairing_expired") return false
        synchronized(lock) {
            if (gen != generation) return true
            desiredConnection++; reconnect = false; retryAt = 0L; retryCount = 0
            reconnectJob?.cancel(); peer = null
        }
        val status = if (code == "pairing_rejected") "电脑未允许连接" else "配对请求已过期"
        disconnect(status)
        mutable.update { it.copy(pairingConfirmed = false, error = reason.ifEmpty { "$status，请重新点击连接" }) }
        return true
    }
    private fun revokePair(gen: Long, reason: String) {
        val revoked = synchronized(lock) {
            if (gen != generation) return
            val old = peer
            revokedCredential = old
            desiredConnection++; reconnect = false; retryAt = 0L; retryCount = 0
            reconnectJob?.cancel(); peer = null
            old
        }
        disconnect("已解除配对")
        scope.launch { pairingPersistence.withLock { if (revoked != null && store.load()?.sameCredential(revoked) == true) store.clear() } }
        mutable.update { it.copy(error = reason.ifEmpty { "电脑已解除配对，请重新连接" }) }
    }
    private fun lost(gen: Long, reason: String) {
        synchronized(lock) { if (gen != generation) return; generation++; connected = false; updateWifiLock(); recordingRequested = false; recording = ""; socket?.cancel(); socket = null; udp?.close(); udp = null }
        recorder.stop(); heartbeat?.cancel()
        mutable.update { it.copy(connected = false, mic = "idle", micMode = "", activeVoiceProfile = null, level = 0f, status = "连接已断开", error = reason, pairing = "", touchpad = TouchpadCapabilities()) }
        synchronized(lock) {
            // 只要还有保存的配对就继续自动重连，不要求已经拿到 token。
            if (reconnect && peer != null) { retryAt = 0; scheduleReconnect() }
        }
    }
    fun disconnect(reason: String = "未连接") {
        stopMic(true); synchronized(lock) { generation++; connected = false; recordingRequested = false; recording = ""; voiceMouseButtons.clear(); updateWifiLock(); socket?.cancel(); socket = null; udp?.close(); udp = null }
        heartbeat?.cancel(); mutable.update { it.copy(connected = false, mic = "idle", micMode = "", activeVoiceProfile = null, level = 0f, pairing = "", status = reason, touchpad = TouchpadCapabilities()) }
    }
    fun forget() { synchronized(lock) { desiredConnection++; reconnect = false; retryAt = 0L; retryCount = 0; reconnectJob?.cancel(); peer = null }; disconnect(); scope.launch { pairingPersistence.withLock { store.clear() } } }
    private fun sendOn(ws: WebSocket, m: JsonObject) = ws.send(m.toString())
    private fun send(m: JsonObject): Boolean = socket?.let { sendOn(it, m) } ?: false
    override fun move(dx: Double, dy: Double): Unit = synchronized(lock) { if (!connected) return@synchronized; val gain = inputSettings.value.pointerGain; x += (dx * gain * 1024).toLong(); y += (dy * gain * 1024).toLong(); movement.trySend(generation to Movement(epoch, x, y, sx, sy)); Unit }
    override fun scroll(dx: Double, dy: Double): Unit = synchronized(lock) { if (!connected) return@synchronized; val units = scrollUnits(dx, dy, mutable.value.config.natural_scroll); sx += units.first; sy += units.second; movement.trySend(generation to Movement(epoch, x, y, sx, sy)); Unit }
    private fun barrier(type: String, vararg fields: Pair<String, JsonElement>): Boolean {
        if (!connected) return false
        if (!send(message(type, *fields, "epoch" to epoch.j(), "next_epoch" to (epoch + 1).j(), "x" to x.j(), "y" to y.j(), "scroll_x" to sx.j(), "scroll_y" to sy.j()))) {
            lost(generation, "触控指令发送失败"); return false
        }
        epoch++
        return true
    }
    override fun button(name: String, down: Boolean) {
        val finishVoice = synchronized(lock) {
            if (name in voiceMouseButtons) {
                if (!down) voiceMouseButtons.remove(name)
                return
            }
            val current = mutable.value
            if (down && current.micMode == "toggle" && current.mic in listOf("preparing", "transmitting", "stopping")) {
                voiceMouseButtons.add(name)
                true
            } else {
                barrier("mouse_button", "button" to name.j(), "down" to down.j())
                false
            }
        }
        // 与快捷键一致：这一轮按下/抬起仅用于结束单击录音，避免点击提前打断输入法。
        // recorder.stop() 会等待采音线程，必须放在连接锁外。
        if (finishVoice) stopMic()
    }
    override fun click(name: String) { button(name, true); button(name, false) }
    private fun touchpadNotice(reason: String) {
        mutable.update { it.copy(error = reason) }
        val now = SystemClock.elapsedRealtime()
        val show = synchronized(lock) { if (now - lastTouchpadNotice < 3000) false else { lastTouchpadNotice = now; true } }
        if (show) scope.launch(Dispatchers.Main) { Toast.makeText(app, reason, Toast.LENGTH_SHORT).show() }
    }
    private fun hasFeature(feature: String): Boolean {
        if (feature in state.value.touchpad.features) return true
        touchpadNotice("请将电脑端升级至 TapDeck 0.3.3 或更新版本，以使用双指缩放和三指手势")
        return false
    }
    override fun zoom(steps: Int): Unit = synchronized(lock) {
        if (!connected || !foreground || steps == 0 || !hasFeature(ZOOM_FEATURE)) return@synchronized
        var remaining = steps
        while (remaining != 0) {
            val batch = remaining.coerceIn(-4, 4)
            if (!barrier("zoom", "steps" to batch.j())) break
            remaining -= batch
        }
    }
    override fun gesture(direction: String): Unit = synchronized(lock) {
        if (!connected || !foreground || direction !in listOf("up", "down") || !hasFeature(GESTURE_FEATURE)) return@synchronized
        barrier("gesture", "action" to direction.j())
        Unit
    }
    fun shortcut(slot: Int) {
        val current = state.value
        if (current.connected && current.config.shortcuts.getOrNull(slot)?.enabled == true)
            send(message("shortcut", "slot" to slot.j(), "revision" to current.config.revision.j()))
    }

    /** 按住开始时调用：PC 保持组合键按下。 */
    fun shortcutHoldStart(slot: Int, hold: String) {
        val current = state.value
        if (current.connected && current.config.shortcuts.getOrNull(slot)?.enabled == true)
            send(message("shortcut_hold_start", "slot" to slot.j(), "revision" to current.config.revision.j(), "hold" to hold.j()))
    }

    /** 按住结束（或取消）时调用：PC 释放对应组合键。 */
    fun shortcutHoldStop(hold: String) {
        send(message("shortcut_hold_stop", "hold" to hold.j()))
    }

    /** 用于按住快捷键的唯一编号。 */
    fun newHoldToken(): String = "%016x".format(SecureRandom().nextLong())

    /** 全键盘：按下单个键（组合键文本，例如 `LeftShift+A`）。 */
    fun keyDown(chord: String): Boolean = key("key_down", chord)

    /** 全键盘：抬起单个键。 */
    fun keyUp(chord: String): Boolean = key("key_up", chord)

    private fun key(type: String, chord: String): Boolean {
        val current = state.value
        if (!current.connected || chord.isEmpty()) return false
        return send(message(type, "text" to chord.j(), "revision" to current.config.revision.j()))
    }
    // mode describes the phone gesture; the selected profile owns the PC hotkey type.
    fun startMic(mode: String = "hold", profileId: String? = null): Boolean = synchronized(lock) {
        if (!connected || !foreground || recordingRequested || recording.isNotEmpty() || mutable.value.mic != "idle" || mode !in listOf("hold", "toggle")) {
            Log.i("TapDeck", "startMic refused: connected=$connected foreground=$foreground requested=$recordingRequested recording=${recording.isNotEmpty()} mic=${mutable.value.mic} mode=$mode")
            return@synchronized false
        }
        val current = mutable.value
        val profiles = current.voiceProfiles()
        val profile = profiles.firstOrNull { it.enabled && if (profileId != null) it.id == profileId else it.mode == mode } ?: return@synchronized false
        var id: ULong
        do { id = ByteBuffer.wrap(ByteArray(8).also { SecureRandom().nextBytes(it) }).long.toULong() } while (id == 0uL)
        recording = id.toString(16).padStart(16, '0'); recordingRequested = true
        mutable.update { it.copy(mic = "preparing", micMode = mode, activeVoiceProfile = profile, error = "") }
        val fields = mutableListOf("recording" to recording.j(), "mode" to profile.mode.j())
        if (current.voiceProfilesSupported) { fields += "profile_id" to profile.id.j(); fields += "revision" to current.config.revision.j() }
        if (!send(message("mic_start", *fields.toTypedArray()))) {
            recordingRequested = false; recording = ""
            mutable.update { it.copy(mic = "idle", micMode = "", activeVoiceProfile = null, error = "控制连接不可用") }
            return@synchronized false
        }
        true
    }
    internal fun finishMic(id: String, reason: String? = null) {
        synchronized(lock) {
            if (id.isEmpty() || id != recording) return
            recordingRequested = false
            while (audio.tryReceive().isSuccess) { }
            mutable.update { it.copy(mic = "stopping", level = 0f) }
        }
        recorder.stop()
        synchronized(lock) {
            if (recording != id) return
            recording = ""
            mutable.update { it.copy(mic = "idle", micMode = "", activeVoiceProfile = null, level = 0f, error = reason ?: it.error) }
        }
    }
    fun stopMic(abort: Boolean = false) {
        val id = synchronized(lock) {
            if (recording.isEmpty()) { recordingRequested = false; return }
            if (!recordingRequested && !abort) return
            recordingRequested = false
            while (audio.tryReceive().isSuccess) { }
            mutable.update { it.copy(mic = "stopping", level = 0f) }
            recording
        }
        recorder.stop()
        synchronized(lock) {
            if (recording == id && connected)
                send(message(if (abort) "mic_abort" else "mic_stop", "recording" to id.j()))
        }
    }
    fun close() { inputPreferences.close(); reconnect = false; retryAt = 0L; reconnectJob?.cancel(); networkCallback?.let { runCatching { app.getSystemService(android.net.ConnectivityManager::class.java)?.unregisterNetworkCallback(it) } }; disconnect(); movement.close(); audio.close(); http.dispatcher.executorService.shutdown() }
}
