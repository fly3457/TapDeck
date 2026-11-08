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

enum class ConnectionPhase { Disconnected, Connecting, Pairing, Connected, NeedsPairing }

data class ClientState(val status: String = "未连接", val peerName: String = "TapDeck", val connected: Boolean = false, val pairing: String = "", val pairingConfirmed: Boolean = false, val config: PcConfig = PcConfig(), val mic: String = "idle", val micMode: String = "", val activeVoiceProfile: VoiceProfile? = null, val voiceProfilesSupported: Boolean = false, val level: Float = 0f, val error: String = "", val rttMs: Long = 0, val touchpad: TouchpadCapabilities = TouchpadCapabilities(), val selectedPeerId: String? = null, val connectionEpoch: Long = 0, val phase: ConnectionPhase = ConnectionPhase.Disconnected, val catalogReady: Boolean = false, val video: VideoState = VideoState()) {
    fun voiceProfiles() = config.voiceProfiles(voiceProfilesSupported)
    val videoSupported: Boolean get() = VIDEO_FEATURE in touchpad.features
    val canSwitch: Boolean get() = mic == "idle"
}
/** [notifications] 为空时（仪器测试）不接收电脑通知，也不进入后台心跳模式。 */
class TapClient(private val app: Application, private val scope: CoroutineScope, private val notifications: PcNotificationCenter? = null) : TouchSink {
    private companion object {
        /** 自动重连的退避间隔（毫秒），最后一次会一直沿用。 */
        val RETRY_DELAYS = longArrayOf(1000, 2000, 4000, 8000, 15000, 30000)
        /** App 不在前台时的重试间隔，避免后台无谓地反复连接。 */
        const val IDLE_RETRY_MS = 30000L
        /** 后台心跳模式：PC 每 20 秒发 keepalive，超过此时长没有任何消息才判定断线。 */
        const val BACKGROUND_TIMEOUT_MS = 75000L
    }
    private val store = PairStore(app)
    private val pairingPersistence = Mutex()
    private val revokedCredentials = mutableMapOf<String, Peer>()
    private val directory = MutableStateFlow(PeerCatalog())
    val peers = directory.asStateFlow()
    /** Called on the main thread BEFORE invalidating the old connection. */
    var beforeConnectionChange: (() -> Unit)? = null
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
    /** 证书固定的 TLS 客户端，视频连接复用它。 */
    private var pinnedClient: OkHttpClient? = null
    private var sessionHex = ""
    private var videoSocket: WebSocket? = null
    private var videoRequest = 0L
    /** 视频消息在 OkHttp 线程上回调，画面帧不经过主线程。 */
    @Volatile var videoSink: ((VideoMessage) -> Unit)? = null
    /** 与当前电脑的时钟差，供视频延迟统计把 PC 时刻换算到手机时钟；每次新连接清空。 */
    internal val clock = ClockSync()
    private var udp: DatagramSocket? = null
    @Volatile private var peer: Peer? = null
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
        applyLinkMode()
        // 回到前台仍未连接时立即重试，不必等退避计时。
        if (becameForeground && reconnect && peer != null && !connected && state.value.phase == ConnectionPhase.Disconnected) { retryAt = 0; scheduleReconnect() }
    }
    @Volatile private var lastResponse = 0L
    /** 已向 PC 声明后台模式（link_mode background）：停发 250ms 心跳，只回应 PC 的 keepalive。 */
    @Volatile private var linkBackground = false
    @Volatile private var reconnect = true
    private var heartbeat: Job? = null
    private var reconnectJob: Job? = null
    private var connectionJob: Job? = null
    private var bootstrapCall: Call? = null
    private var readyPending = -1L
    private var closed = false
    // 自动重连的退避状态：开机顺序、Wi-Fi 尚未就绪或电脑接收端未启动时都要继续尝试。
    private var retryCount = 0
    private var retryAt = 0L
    private var retryNotice = ""
    private var networkCallback: android.net.ConnectivityManager.NetworkCallback? = null
    private val movement = Channel<Pair<Long, Movement>>(Channel.CONFLATED)
    private val audio = Channel<Pair<Long, ByteArray>>(6)
    private var recording = ""
    private var recordingRequested = false
    private val voiceMouseButtons = mutableSetOf<String>()
    private val recorder = MicCapture(frame = { _, _, _ -> })
    internal fun microphoneFrame(gen: Long, id: String, pcm: ByteArray, pos: Long, level: Float) {
        synchronized(lock) {
            if (gen != generation || !connected || !recordingRequested || id.isEmpty() || id != recording) return
            val body = ByteBuffer.allocate(976).order(ByteOrder.LITTLE_ENDIAN)
                .putLong(id.toULong(16).toLong()).putLong(pos).put(pcm).array()
            audio.trySend(gen to body)
            if (pos % 4800L == 0L) mutable.update { it.copy(level = level) }
        }
    }
    private fun startRecorder(attempt: Long, gen: Long) {
        val id = recording
        recorder.start(
            frame = { pcm, pos, level -> microphoneFrame(gen, id, pcm, pos, level) },
            onError = { reason ->
                scope.launch(Dispatchers.Main.immediate) {
                    if (current(attempt, gen) && recording == id) {
                        stopMic(true)
                        mutable.update { it.copy(error = reason) }
                    }
                }
            },
        )
    }
    private val http = OkHttpClient.Builder().connectTimeout(5, java.util.concurrent.TimeUnit.SECONDS).readTimeout(5, java.util.concurrent.TimeUnit.SECONDS).followRedirects(false).followSslRedirects(false).build()
    init {
        scope.launch(Dispatchers.IO) { for ((gen, m) in movement) synchronized(lock) { if (connected && gen == generation) runCatching { val bytes = mouseCodec!!.seal(m.bytes()); udp!!.send(DatagramPacket(bytes, bytes.size, target, udpPort)) }.onFailure { Log.w("TapDeck", "mouse UDP failed", it) } } }
        scope.launch(Dispatchers.IO) { for ((gen, body) in audio) synchronized(lock) { if (connected && gen == generation && recordingRequested && recording.isNotEmpty() && ByteBuffer.wrap(body).order(ByteOrder.LITTLE_ENDIAN).long == recording.toULong(16).toLong()) runCatching { val bytes = audioCodec!!.seal(body); udp!!.send(DatagramPacket(bytes, bytes.size, target, udpPort)) }.onFailure { Log.w("TapDeck", "audio UDP failed", it) } } }
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
            if (!reconnect || peer == null || connected || state.value.phase != ConnectionPhase.Disconnected) return
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
        if (!reconnect || peer?.token.isNullOrEmpty() || closed) return
        val timer = reconnectJob?.isActive == true
        if (timer && retryAt != 0L) return
        if (retryAt == 0L) retryAt = SystemClock.elapsedRealtime() + backoffDelay()
        val attempt = desiredConnection
        reconnectJob?.cancel()
        reconnectJob = scope.launch(Dispatchers.Main) {
            while (isActive) {
                val wait = synchronized(lock) { (retryAt - SystemClock.elapsedRealtime()).coerceAtLeast(0) }
                if (wait > 0) { delay(wait); continue }
                val current = synchronized(lock) { if (!reconnect || attempt != desiredConnection || connected) null else peer }
                if (current == null) return@launch
                reconnectJob = null
                connectionJob = scope.launch { connectSafely(current, attempt) }
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
    private fun current(attempt: Long, gen: Long? = null) = synchronized(lock) {
        !closed && attempt == desiredConnection && (gen == null || gen == generation)
    }
    private suspend fun editPeers(attempt: Long? = null, change: (PeerCatalog) -> PeerCatalog): PeerCatalog =
        pairingPersistence.withLock {
            val next = withContext(Dispatchers.IO) {
                store.updateCatalog { old -> if (attempt != null && !current(attempt)) old else change(old) }
            }
            withContext(Dispatchers.Main.immediate) {
                directory.value = next
                mutable.update { state -> state.copy(catalogReady = true, peerName = next.find(state.selectedPeerId)?.displayName ?: state.peerName) }
            }
            next
        }
    private fun storageError(error: Exception) {
        Log.w("TapDeck", "Pairing directory could not be saved/loaded", error)
        mutable.update { it.copy(error = "电脑列表读写失败，原配对已保留，请重试") }
    }
    fun allowTargetChange(): Boolean {
        if (state.value.canSwitch) return true
        Toast.makeText(app, "请先结束语音输入", Toast.LENGTH_SHORT).show()
        return false
    }
    /** All target changes first clear the old input owners on the main thread. */
    private fun beginTargetChange(name: String = "TapDeck", id: String? = null): Long {
        connectionJob?.cancel()
        synchronized(lock) {
            desiredConnection++; reconnect = false; retryAt = 0L; retryCount = 0; reconnectJob?.cancel()
        }
        disconnect()
        peer = null
        mutable.update { it.copy(peerName = name, selectedPeerId = id, error = "") }
        return synchronized(lock) { desiredConnection }
    }
    suspend fun restore() = withContext(Dispatchers.Main.immediate) {
        val attempt = synchronized(lock) { desiredConnection }
        try {
            val saved = editPeers { it }.selected
            if (!current(attempt) || connected || saved == null) return@withContext
            mutable.update { it.copy(selectedPeerId = saved.id, peerName = saved.displayName) }
            if (saved.needsPairing) {
                mutable.update { it.copy(status = "需重新配对", phase = ConnectionPhase.NeedsPairing) }
                return@withContext
            }
            if (Build.VERSION.SDK_INT >= 37 && app.applicationInfo.targetSdkVersion >= 37 &&
                app.checkSelfPermission("android.permission.ACCESS_LOCAL_NETWORK") != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                mutable.update { it.copy(status = "请打开连接页授予局域网权限") }
                return@withContext
            }
            peer = usablePeer(saved.peer)
            reconnect = peer?.token?.isNotEmpty() == true
            if (reconnect) connectionJob = scope.launch { connectSafely(requireNotNull(peer), attempt) }
        } catch (e: CancellationException) { throw e } catch (e: Exception) { storageError(e) }
    }
    private fun usablePeer(value: Peer): Peer =
        if (revokedCredentials[value.id]?.sameCredential(value) == true) value.copy(token = "") else value

    fun selectPeer(id: String) {
        if (!allowTargetChange()) return
        val saved = directory.value.find(id) ?: return
        if (state.value.selectedPeerId == id && connected) return
        val attempt = beginTargetChange(saved.displayName, saved.id)
        val target = usablePeer(saved.peer)
        peer = target
        connectionJob = scope.launch(Dispatchers.Main.immediate) {
            try {
                editPeers(attempt) { it.select(id) }
                if (!current(attempt)) return@launch
                reconnect = target.token.isNotEmpty()
                connectSafely(target, attempt)
            } catch (e: CancellationException) { throw e } catch (e: Exception) {
                if (current(attempt)) { reconnect = false; storageError(e) }
            }
        }
    }
    fun renamePeer(id: String, alias: String) {
        scope.launch(Dispatchers.Main.immediate) {
            try { editPeers { it.rename(id, alias) } }
            catch (e: CancellationException) { throw e } catch (e: Exception) { storageError(e) }
        }
    }
    fun rememberVoice(id: String, epoch: Long, profileId: String) {
        if (state.value.selectedPeerId != id || state.value.connectionEpoch != epoch) return
        val credential = directory.value.find(id)?.peer ?: return
        scope.launch(Dispatchers.Main.immediate) {
            try {
                editPeers { catalog ->
                    if (catalog.find(id)?.peer?.sameCredential(credential) == true) catalog.voice(id, profileId) else catalog
                }
            } catch (e: CancellationException) { throw e } catch (e: Exception) { storageError(e) }
        }
    }
    fun forgetPeer(id: String) {
        notifications?.forget(id)
        val active = state.value.selectedPeerId == id || peer?.id == id
        if (active && !allowTargetChange()) return
        if (active) beginTargetChange()
        scope.launch(Dispatchers.Main.immediate) {
            try { editPeers { it.remove(id) } }
            catch (e: CancellationException) { throw e } catch (e: Exception) { storageError(e) }
        }
    }
    fun cancelPairing() {
        if (!allowTargetChange()) return
        val attempt = beginTargetChange()
        scope.launch(Dispatchers.Main.immediate) {
            try { editPeers(attempt) { it.select(null) } }
            catch (e: CancellationException) { throw e } catch (e: Exception) { storageError(e) }
        }
    }
    /** Compatibility for diagnostics: never clears other saved computers. */
    fun forget() { state.value.selectedPeerId?.let(::forgetPeer) ?: cancelPairing() }

    fun enter(raw: String) {
        if (!allowTargetChange()) return
        val attempt = beginTargetChange()
        mutable.update { it.copy(status = "正在读取电脑信息", phase = ConnectionPhase.Connecting) }
        connectionJob = scope.launch(Dispatchers.Main.immediate) {
            try {
                // Unfinished new pairings are deliberately not restored on startup.
                editPeers(attempt) { it.select(null) }
                if (!current(attempt)) return@launch
                val text = raw.trim()
                val target = withContext(Dispatchers.IO) {
                    val p = if (text.startsWith("tapdeck://")) {
                        val u = android.net.Uri.parse(text)
                        require(u.host == "pair") { "无效配对链接" }
                        require(u.getQueryParameter("v")?.toIntOrNull() == CONTROL_VERSION) { "协议版本不匹配，请升级 TapDeck" }
                        Peer(u.getQueryParameter("host") ?: error("缺少地址"),
                            u.getQueryParameter("wss")?.toIntOrNull() ?: 41443,
                            u.getQueryParameter("http")?.toIntOrNull() ?: 41080,
                            u.getQueryParameter("pin") ?: error("缺少服务器指纹"))
                    } else {
                        val u = URI(if (text.contains("://")) text else "http://$text")
                        require(u.scheme == "http" && u.host != null && u.userInfo == null) { "请输入 PC 的 http 配对网址" }
                        validateHost(u.host)
                        val port = if (u.port == -1) 41080 else u.port
                        require(port in 1024..65535) { "端口无效" }
                        val m = pairInfo(u.host, port, attempt)
                        require(m.long("version") == CONTROL_VERSION.toLong()) { "协议版本不匹配，请升级 TapDeck" }
                        Peer(u.host, m.long("wss_port").toInt(), port, m.str("pin"), m.str("name", "电脑"))
                    }
                    validateHost(p.host)
                    require(p.pin.matches(Regex("[0-9a-fA-F]{64}"))) { "服务器指纹无效" }
                    require(p.wssPort in 1024..65535 && p.httpPort in 1024..65535) { "端口无效" }
                    p
                }
                if (!current(attempt)) return@launch
                val existing = directory.value.find(target.id)
                val candidate = usablePeer(target.withCredentialFrom(existing?.peer))
                peer = candidate
                if (existing != null) {
                    editPeers(attempt) { it.select(existing.id) }
                    if (!current(attempt)) return@launch
                }
                mutable.update { it.copy(selectedPeerId = existing?.id, peerName = existing?.displayName ?: candidate.name) }
                reconnect = candidate.token.isNotEmpty()
                connectSafely(candidate, attempt)
            } catch (e: CancellationException) { throw e } catch (e: Exception) {
                if (current(attempt)) stopAttempt("连接失败", e.message ?: "连接失败")
            }
        }
    }
    private fun validateHost(host: String) {
        val a = InetAddress.getByName(host)
        require(a is java.net.Inet4Address && (a.isSiteLocalAddress || a.isLoopbackAddress)) { "仅支持局域网 IPv4 地址" }
    }
    private fun pairInfo(host: String, port: Int, attempt: Long, client: OkHttpClient = http): JsonObject {
        val call = client.newCall(Request.Builder().url("http://$host:$port/api/pair-info").build())
        synchronized(lock) {
            if (!current(attempt)) throw CancellationException("Target changed")
            bootstrapCall = call
        }
        return try {
            call.execute().use {
                require(it.isSuccessful) { "电脑返回 " + it.code }
                wireJson.parseToJsonElement(requireNotNull(it.body).string()).jsonObject
            }
        } finally { synchronized(lock) { if (bootstrapCall === call) bootstrapCall = null } }
    }
    /** 探测已知地址时缩短超时：换了网络后旧地址通常不可达，不必每个等满 5 秒。 */
    private val probeHttp by lazy { http.newBuilder().connectTimeout(1500, java.util.concurrent.TimeUnit.MILLISECONDS).readTimeout(2500, java.util.concurrent.TimeUnit.MILLISECONDS).build() }
    private class IdentityChanged : Exception("电脑身份已变化")

    /**
     * 依次探测这台电脑记住的所有地址（当前地址在前），返回第一个证书指纹一致的地址及其元数据。
     * 换 Wi-Fi / 网段后电脑 IP 不同：只要曾在该网络连过，就能自动连回。只有一个地址且被
     * 另一台电脑占用时仍按“身份已变化”处理；多个地址时跳过被占用的旧地址。
     */
    private fun locate(p: Peer, attempt: Long): Pair<Peer, JsonObject> {
        val candidates = (listOf(p) + (directory.value.find(p.id)?.candidates() ?: emptyList())).distinctBy { it.host + ":" + it.httpPort }
        var mismatch = false
        var failure: Exception? = null
        for (c in candidates) {
            try {
                validateHost(c.host)
                val m = pairInfo(c.host, c.httpPort, attempt, if (candidates.size > 1) probeHttp else http)
                if (!m.str("pin").equals(p.pin, true)) { mismatch = true; continue }
                val wss = m.long("wss_port").toInt().takeIf { it in 1024..65535 } ?: c.wssPort
                return c.copy(wssPort = wss) to m
            } catch (e: CancellationException) { throw e } catch (e: Exception) { failure = e }
        }
        if (mismatch && candidates.size == 1) throw IdentityChanged()
        if (candidates.size > 1) throw java.io.IOException("记住的 ${candidates.size} 个地址都没找到这台电脑；如果换了网络，请扫描电脑配对二维码添加新地址")
        throw failure ?: java.io.IOException("连接失败")
    }

    private suspend fun connectSafely(p: Peer, attempt: Long) {
        try { connect(p, attempt) }
        catch (e: CancellationException) { throw e }
        catch (e: Exception) {
            if (current(attempt)) {
                reconnect = false
                disconnect("连接失败")
                mutable.update { it.copy(error = e.message ?: "无法建立连接，请重试") }
            }
        }
    }
    private suspend fun connect(saved: Peer, attempt: Long) {
        val gen = synchronized(lock) { if (!current(attempt)) return; generation++; generation }
        clock.reset()
        mutable.update { it.copy(status = "正在连接", phase = ConnectionPhase.Connecting, connectionEpoch = gen, pairing = "", pairingConfirmed = false, error = "") }
        val metadata: JsonObject
        val p: Peer
        try {
            val located = withContext(Dispatchers.IO) { locate(saved, attempt) }
            if (!current(attempt, gen)) return
            p = located.first; metadata = located.second
            if (metadata.long("version") != CONTROL_VERSION.toLong()) {
                reconnect = false
                mutable.update { it.copy(status = "版本不匹配", phase = ConnectionPhase.Disconnected, error = "请升级 TapDeck 至兼容协议版本") }
                return
            }
            // HTTP discovery may refresh a name, but never replaces a trusted identity.
            if (!metadata.str("pin").equals(p.pin, true)) {
                reconnect = false
                mutable.update { it.copy(status = "电脑身份已变化", phase = ConnectionPhase.NeedsPairing, error = "请重新扫描该电脑配对网址并核对校验码") }
                return
            }
        } catch (e: CancellationException) { throw e } catch (e: IdentityChanged) {
            reconnect = false
            mutable.update { it.copy(status = "电脑身份已变化", phase = ConnectionPhase.NeedsPairing, error = "请重新扫描该电脑配对网址并核对校验码") }
            return
        } catch (e: Exception) { lost(gen, e.message ?: "连接失败"); return }
        val candidate = p.copy(name = metadata.str("name").ifBlank { p.name })
        val id = withContext(Dispatchers.IO) { store.deviceId() }
        if (!current(attempt, gen)) return
        val clientNonce = ByteArray(32).also { SecureRandom().nextBytes(it) }
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
        // 身份只认证书指纹（上面的 manager）：不再校验证书里的 IP，电脑换网络后旧接收端的证书没有新地址也能连上。
        val client = http.newBuilder().sslSocketFactory(tls.socketFactory, manager).hostnameVerifier { _, _ -> true }.readTimeout(0, java.util.concurrent.TimeUnit.SECONDS).build()
        val ws = client.newWebSocket(Request.Builder().url("wss://" + p.host + ":" + p.wssPort + "/ws").build(), object : WebSocketListener() {
            private fun dispatch(action: () -> Unit) {
                scope.launch(Dispatchers.Main.immediate) { if (current(attempt, gen)) action() }
            }
            override fun onOpen(ws: WebSocket, response: Response) {
                dispatch {
                    sendOn(ws, message("hello", "version" to CONTROL_VERSION.j(), "device_id" to id.j(),
                        "name" to (Build.MANUFACTURER + " " + Build.MODEL).j(), "token" to p.token.j(), "client_nonce" to encode64(clientNonce).j()))
                }
                if (!current(attempt, gen)) ws.cancel()
            }
            override fun onMessage(ws: WebSocket, text: String) = dispatch {
                lastResponse = SystemClock.elapsedRealtime()
                runCatching {
                    val m = wireJson.parseToJsonElement(text).jsonObject
                    when (m.str("type")) {
                        "pair_challenge" -> {
                            val code = comparisonCode(p.id, clientNonce, decode64(m.str("server_nonce")))
                            require(code == m.str("code")) { "配对校验失败" }
                            mutable.update { it.copy(status = "等待电脑允许连接", phase = ConnectionPhase.Pairing, pairing = code, pairingConfirmed = true) }
                            sendOn(ws, message("pair_confirm", "code" to code.j()))
                        }
                        "ready" -> acceptReady(candidate, attempt, gen, ws, m)
                        "config" -> {
                            val c = wireJson.decodeFromJsonElement<PcConfig>(m["config"]!!).validate()
                            c.voiceProfiles(mutable.value.voiceProfilesSupported)
                            mutable.update { it.copy(config = c) }
                        }
                        "heartbeat" -> {
                            mutable.update { it.copy(rttMs = (SystemClock.elapsedRealtime() - m.long("tick")).coerceAtLeast(0)) }
                            // PC 0.5.11 起回带墙钟，用于视频延迟统计的时钟对齐。
                            if (m["pc_us"] != null) clock.sample(m.long("tick") * 1000, SystemClock.elapsedRealtimeNanos() / 1000, m.long("pc_us"))
                        }
                        "keepalive" -> sendOn(ws, message("keepalive"))
                        "notifications" -> notifications?.receive(p.id, mutable.value.peerName, m.str("boot"),
                            wireJson.decodeFromJsonElement<List<WireNotification>>(m["items"] ?: JsonArray(emptyList())))
                        "mic_ready" -> {
                            if (m.str("recording") == recording && recordingRequested && foreground) {
                                try { startRecorder(attempt, gen); mutable.update { it.copy(mic = "transmitting", error = "") } }
                                catch (e: Exception) { stopMic(true); mutable.update { it.copy(error = e.message ?: "录音失败") } }
                            } else sendOn(ws, message("mic_abort", "recording" to m.str("recording").j()))
                        }
                        "mic_error" -> finishMic(m.str("recording"), m.str("reason"))
                        "mic_stopped" -> finishMic(m.str("recording"))
                        "video_ready" -> openVideo(gen, m)
                        "video_error" -> videoFailed(m.str("reason").ifEmpty { "无法查看电脑画面" })
                        "error" -> when {
                            m.str("code") == "pairing_revoked" -> revokePair(gen, m.str("reason"))
                            finishPairingAttempt(gen, m.str("code"), m.str("reason")) -> Unit
                            else -> {
                                if (m.str("code") == "version_mismatch") reconnect = false
                                mutable.update { it.copy(error = m.str("reason")) }
                                if (m.str("code") == "touchpad_error") touchpadNotice(m.str("reason"))
                            }
                        }
                    }
                }.onFailure { error -> lost(gen, error.message ?: "协议错误") }
            }
            override fun onFailure(ws: WebSocket, t: Throwable, response: Response?) = dispatch { lost(gen, t.message ?: "连接已断开") }
            override fun onClosed(ws: WebSocket, code: Int, reason: String) = dispatch { lost(gen, reason.ifEmpty { "连接已断开" }) }
            override fun onClosing(ws: WebSocket, code: Int, reason: String) { ws.close(code, reason) }
        })
        synchronized(lock) { if (current(attempt, gen)) { socket = ws; pinnedClient = client } else ws.cancel() }
    }
    private fun acceptReady(p: Peer, attempt: Long, gen: Long, ws: WebSocket, m: JsonObject) {
        if (readyPending == gen || connected) return
        val saved = p.copy(token = m.str("token").ifEmpty { p.token })
        require(saved.token.isNotEmpty()) { "电脑缺少配对凭据" }
        val config = wireJson.decodeFromJsonElement<PcConfig>(m["config"]!!).validate()
        val capabilities = m.touchpadCapabilities()
        val profilesSupported = VOICE_PROFILES_FEATURE in capabilities.features
        config.voiceProfiles(profilesSupported)
        val session = m.str("session").toULong(16).toLong()
        sessionHex = m.str("session")
        val mouse = UdpCodec(decode64(m.str("mouse_key")), decode64(m.str("mouse_prefix")), session, 1)
        val voice = UdpCodec(decode64(m.str("audio_key")), decode64(m.str("audio_prefix")), session, 2)
        val port = m.long("udp_port").toInt()
        require(port in 1024..65535) { "UDP 端口无效" }
        readyPending = gen
        mutable.update { it.copy(status = "正在保存配对", pairing = "", pairingConfirmed = false, config = config, voiceProfilesSupported = profilesSupported) }
        // Keep the captured socket alive during storage, without routing old heartbeats to a new target.
        heartbeat?.cancel()
        heartbeat = scope.launch(Dispatchers.Main) {
            while (isActive && current(attempt, gen)) {
                val background = linkBackground
                if (SystemClock.elapsedRealtime() - lastResponse >= if (background) BACKGROUND_TIMEOUT_MS else 1000) { lost(gen, "电脑心跳超时"); break }
                if (!background) sendOn(ws, message("heartbeat", "tick" to SystemClock.elapsedRealtime().j()))
                // 后台同样 250ms 检查一次：CPU 休眠时计时器不会唤醒设备，切回前台后心跳也能立即恢复。
                delay(250)
            }
        }
        scope.launch(Dispatchers.Main.immediate) {
            try {
                val address = withContext(Dispatchers.IO) { InetAddress.getByName(p.host) }
                val next = editPeers(attempt) { old -> if (current(attempt, gen)) old.upsert(saved).select(saved.id) else old }
                if (!current(attempt, gen)) return@launch
                synchronized(lock) {
                    revokedCredentials.remove(saved.id)
                    udp?.close(); udp = DatagramSocket(); target = address; udpPort = port
                    mouseCodec = mouse; audioCodec = voice
                    epoch = 0; x = 0; y = 0; sx = 0; sy = 0
                    connected = true; peer = saved; reconnect = true
                    retryCount = 0; retryAt = 0L; retryNotice = ""
                    updateWifiLock()
                    mutable.update { it.copy(status = "已连接", phase = ConnectionPhase.Connected, selectedPeerId = saved.id,
                        peerName = next.find(saved.id)!!.displayName, connected = true, error = "", touchpad = capabilities) }
                }
                if (NOTIFY_FEATURE in capabilities.features) syncNotifications(ws, saved.id, attempt, gen)
            } catch (e: CancellationException) { throw e } catch (e: Exception) {
                if (current(attempt, gen)) { stopAttempt("无法保存配对", "电脑列表读写失败，请重试"); storageError(e) }
            }
        }
    }
    /** 连接就绪后补收离线期间的通知，并按前后台状态声明心跳模式。 */
    private fun syncNotifications(ws: WebSocket, peerId: String, attempt: Long, gen: Long) {
        val center = notifications ?: return
        scope.launch(Dispatchers.Main.immediate) {
            val cursor = center.cursor(peerId)
            if (!current(attempt, gen)) return@launch
            sendOn(ws, message("notify_sync", "boot" to (cursor?.boot ?: "").j(), "after" to (cursor?.seq ?: 0L).j()))
            synchronized(lock) { applyLinkMode() }
        }
    }
    /**
     * 后台且开启“后台接收电脑通知”时请求 PC 改由其发起 keepalive，手机不必每 250ms 唤醒；
     * 回到前台恢复手机心跳。需持有 [lock]。
     */
    private fun applyLinkMode() {
        if (!connected) return
        val background = !foreground && notifications?.backgroundEnabled?.value == true && NOTIFY_FEATURE in state.value.touchpad.features
        if (background == linkBackground) return
        if (send(message("link_mode", "mode" to (if (background) "background" else "foreground").j()))) {
            linkBackground = background
            lastResponse = SystemClock.elapsedRealtime()
            // 切回前台立即补一次心跳，PC 的 1 秒超时从此刻重新计算。
            if (!background) send(message("heartbeat", "tick" to SystemClock.elapsedRealtime().j()))
        }
    }
    fun refreshLinkMode() = synchronized(lock) { applyLinkMode() }
    fun confirmPair() {
        val code = mutable.value.pairing
        if (code.isNotEmpty()) send(message("pair_confirm", "code" to code.j()))
    }
    private fun stopAttempt(status: String, reason: String) {
        synchronized(lock) { desiredConnection++; reconnect = false; retryAt = 0L; retryCount = 0; reconnectJob?.cancel() }
        disconnect(status)
        peer = null
        mutable.update { it.copy(selectedPeerId = null, peerName = "TapDeck", error = reason) }
        val attempt = synchronized(lock) { desiredConnection }
        scope.launch(Dispatchers.Main.immediate) {
            try { editPeers(attempt) { it.select(null) } }
            catch (e: CancellationException) { throw e } catch (e: Exception) { storageError(e) }
        }
    }
    internal fun finishPairingAttempt(gen: Long, code: String, reason: String): Boolean {
        if (code != "pairing_rejected" && code != "pairing_expired") return false
        if (synchronized(lock) { gen != generation }) return true
        val status = if (code == "pairing_rejected") "电脑未允许连接" else "配对请求已过期"
        stopAttempt(status, reason.ifEmpty { "$status，请重新点击连接" })
        return true
    }
    private fun revokePair(gen: Long, reason: String) {
        if (synchronized(lock) { gen != generation }) return
        val revoked = peer ?: return
        revokedCredentials[revoked.id] = revoked
        synchronized(lock) { desiredConnection++; reconnect = false; retryAt = 0L; retryCount = 0; reconnectJob?.cancel() }
        disconnect("需重新配对")
        peer = null
        directory.value = directory.value.revoke(revoked)
        mutable.update { it.copy(phase = ConnectionPhase.NeedsPairing, error = reason.ifEmpty { "电脑已解除配对，请重新连接" }) }
        scope.launch(Dispatchers.Main.immediate) {
            try { editPeers { it.revoke(revoked) } }
            catch (e: CancellationException) { throw e } catch (e: Exception) { storageError(e) }
        }
    }
    private fun lost(gen: Long, reason: String) {
        if (synchronized(lock) { gen != generation } || closed) return
        if (!reconnect || peer?.token.isNullOrEmpty()) { stopAttempt("连接失败", reason); return }
        disconnect("连接已断开，正在重试")
        mutable.update { it.copy(error = reason) }
        synchronized(lock) { retryAt = 0; scheduleReconnect() }
    }
    fun disconnect(reason: String = "未连接") {
        beforeConnectionChange?.invoke()
        stopMic(true)
        synchronized(lock) {
            generation++; connected = false; recordingRequested = false; recording = ""; voiceMouseButtons.clear()
            linkBackground = false
            updateWifiLock()
            bootstrapCall?.cancel(); bootstrapCall = null
            socket?.cancel(); socket = null
            closeVideoLocked()
            udp?.close(); udp = null
            mouseCodec = null; audioCodec = null
            while (movement.tryReceive().isSuccess) { }
            while (audio.tryReceive().isSuccess) { }
            readyPending = -1L
            mutable.update { it.copy(connected = false, phase = ConnectionPhase.Disconnected, connectionEpoch = generation,
                mic = "idle", micMode = "", activeVoiceProfile = null, level = 0f, pairing = "", pairingConfirmed = false,
                status = reason, config = PcConfig(), voiceProfilesSupported = false, touchpad = TouchpadCapabilities(), rttMs = 0, video = VideoState()) }
        }
        recorder.stop()
        heartbeat?.cancel()
    }
    private fun sendOn(ws: WebSocket, m: JsonObject) = ws.send(m.toString())
    private fun send(m: JsonObject): Boolean = socket?.let { sendOn(it, m) } ?: false
    /** UI callbacks retain their original session even if recomposition is delayed. */
    fun inputInSession(epoch: Long, action: () -> Unit) {
        // Callers and connection transitions share Main; never join the recorder while holding lock.
        if (synchronized(lock) { state.value.connected && state.value.connectionEpoch == epoch }) action()
    }
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

    /**
     * 视频模式：把 PC 光标移到画面上的 (x, y)（所捕获显示器的物理像素）并单击。
     * action 为 click、right_click、move（只移动光标；按住时为拖拽）、down、up。与鼠标按键共用分段屏障；单击语音录音中第一次点击只结束录音。
     */
    fun pointer(action: String, x: Int, y: Int) {
        val finishVoice = synchronized(lock) {
            if (!connected) return
            val current = mutable.value
            if ((action == "click" || action == "right_click" || action == "down") && current.micMode == "toggle" && current.mic in listOf("preparing", "transmitting", "stopping")) true
            else { barrier("pointer", "action" to action.j(), "px" to x.j(), "py" to y.j()); false }
        }
        if (finishVoice) stopMic()
    }
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
    /**
     * 请求查看电脑画面。maxWidth/maxHeight 为手机解码器上限；PC 回复 video_ready 后
     * 用同一证书固定的客户端另开 /video 连接，画面帧不经过控制连接。
     */
    fun startVideo(quality: VideoQuality, maxWidth: Int, maxHeight: Int, audio: Boolean = false): Boolean = synchronized(lock) {
        val current = mutable.value
        if (!connected || !foreground || !current.videoSupported) return@synchronized false
        closeVideoLocked()
        videoRequest++
        mutable.update { it.copy(video = VideoState(VideoPhase.Starting)) }
        if (!send(message("video_start", "quality" to quality.wire.j(), "max_width" to maxWidth.j(), "max_height" to maxHeight.j(), "fps" to 60.j(), "audio" to (audio && VIDEO_AUDIO_FEATURE in current.touchpad.features).j()))) {
            mutable.update { it.copy(video = VideoState(VideoPhase.Error, error = "控制连接不可用")) }
            return@synchronized false
        }
        true
    }

    /** 视频播放中打开或关闭电脑声音（接收端 0.5.15 起）。 */
    fun setVideoAudio(on: Boolean) = synchronized(lock) {
        if (connected && VIDEO_AUDIO_FEATURE in mutable.value.touchpad.features && mutable.value.video.phase != VideoPhase.Idle) send(message("video_audio", "audio" to on.j()))
    }

    fun stopVideo() = synchronized(lock) {
        val active = mutable.value.video.phase != VideoPhase.Idle
        closeVideoLocked()
        videoRequest++
        if (active && connected) send(message("video_stop"))
        mutable.update { it.copy(video = VideoState()) }
    }

    private fun openVideo(gen: Long, m: JsonObject) = synchronized(lock) {
        val p = peer
        val client = pinnedClient
        if (gen != generation || !connected || mutable.value.video.phase != VideoPhase.Starting || p == null || client == null) {
            send(message("video_stop")); return@synchronized
        }
        val request = videoRequest
        val hello = message("video_hello", "session" to sessionHex.j(), "stream" to m.str("stream").j(), "token" to m.str("token").j())
        fun live() = synchronized(lock) { gen == generation && request == videoRequest }
        videoSocket = client.newWebSocket(Request.Builder().url("wss://" + p.host + ":" + p.wssPort + "/video").build(), object : WebSocketListener() {
            override fun onOpen(ws: WebSocket, response: Response) { if (live()) ws.send(hello.toString()) else ws.cancel() }
            override fun onMessage(ws: WebSocket, text: String) {
                if (!live()) { ws.cancel(); return }
                val msg = runCatching { wireJson.parseToJsonElement(text).jsonObject }.getOrNull() ?: return
                when (msg.str("type")) {
                    "video_format" -> {
                        val format = VideoFormat.of(msg) ?: return videoFailed("电脑画面尺寸无效")
                        mutable.update { it.copy(video = VideoState(VideoPhase.Streaming, format)) }
                    }
                    "video_error" -> videoFailed(msg.str("reason").ifEmpty { "无法查看电脑画面" })
                }
            }
            override fun onMessage(ws: WebSocket, bytes: okio.ByteString) {
                if (!live()) { ws.cancel(); return }
                val msg = parseVideoMessage(bytes.toByteArray(), SystemClock.elapsedRealtimeNanos() / 1000) ?: return
                if (msg is VideoCursor && msg.visible != mutable.value.video.cursorVisible)
                    mutable.update { it.copy(video = it.video.copy(cursorVisible = msg.visible)) }
                videoSink?.invoke(msg)
            }
            override fun onFailure(ws: WebSocket, t: Throwable, response: Response?) { if (live()) videoFailed("视频连接已断开") }
            override fun onClosed(ws: WebSocket, code: Int, reason: String) { if (live() && mutable.value.video.phase != VideoPhase.Error) videoFailed("视频连接已断开") }
            override fun onClosing(ws: WebSocket, code: Int, reason: String) { ws.close(code, reason) }
        })
    }

    private fun videoFailed(reason: String) = synchronized(lock) {
        if (mutable.value.video.phase == VideoPhase.Idle) return@synchronized
        closeVideoLocked()
        videoRequest++
        mutable.update { it.copy(video = VideoState(VideoPhase.Error, error = reason)) }
    }

    private fun closeVideoLocked() {
        videoSocket?.cancel(); videoSocket = null
    }

    fun close() {
        inputPreferences.close()
        synchronized(lock) { closed = true; desiredConnection++; reconnect = false; retryAt = 0L }
        connectionJob?.cancel(); reconnectJob?.cancel()
        networkCallback?.let { runCatching { app.getSystemService(android.net.ConnectivityManager::class.java)?.unregisterNetworkCallback(it) } }
        disconnect()
        movement.close(); audio.close(); http.dispatcher.executorService.shutdown()
    }
}
