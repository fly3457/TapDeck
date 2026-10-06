package com.yuncii.tapdeck

import java.nio.ByteBuffer
import java.nio.ByteOrder
import javax.crypto.Cipher
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.*
import java.security.MessageDigest
import java.util.Base64

val wireJson = Json { ignoreUnknownKeys = true; encodeDefaults = true }
const val CONTROL_VERSION = 2
fun message(type: String, vararg fields: Pair<String, JsonElement>) = buildJsonObject { put("type", type); fields.forEach { put(it.first, it.second) } }
fun String.j() = JsonPrimitive(this)
fun Int.j() = JsonPrimitive(this)
fun Long.j() = JsonPrimitive(this)
fun Boolean.j() = JsonPrimitive(this)
fun JsonObject.str(key: String, fallback: String = "") = this[key]?.jsonPrimitive?.contentOrNull ?: fallback
fun JsonObject.long(key: String, fallback: Long = 0) = this[key]?.jsonPrimitive?.longOrNull ?: fallback
fun decode64(s: String): ByteArray = Base64.getUrlDecoder().decode(s)
fun encode64(b: ByteArray): String = Base64.getUrlEncoder().withoutPadding().encodeToString(b)

@Serializable data class Shortcut(val label: String, val chord: String, val enabled: Boolean = false)
@Serializable data class Voice(val hold_key: String = "", val toggle_start_key: String = "", val toggle_stop_key: String = "", val stop_delay_ms: Int = 200)
@Serializable data class PcConfig(val revision: Long = 1, val shortcuts: List<Shortcut> = listOf(Shortcut("复制", "Ctrl+C", true), Shortcut("粘贴", "Ctrl+V", true), Shortcut("撤销", "Ctrl+Z", true), Shortcut("回车", "Enter", true)) + (5..8).map { Shortcut("快捷键 $it", "") }, val voice: Voice = Voice(), val sensitivity: Double = 1.5, val natural_scroll: Boolean = true) {
    fun validate(): PcConfig {
        require(shortcuts.size == 8 && shortcuts.any { it.enabled } && shortcuts.filter { it.enabled }.all { it.label.isNotBlank() && it.chord.isNotBlank() }) { "快捷键配置无效" }
        return this
    }
    fun visibleShortcuts(): List<IndexedValue<Shortcut>> = shortcuts.withIndex().filter { it.value.enabled }
}
data class Movement(val epoch: Int, val x: Long, val y: Long, val sx: Long, val sy: Long) {
    fun bytes(): ByteArray = ByteBuffer.allocate(36).order(ByteOrder.LITTLE_ENDIAN).putInt(epoch).putLong(x).putLong(y).putLong(sx).putLong(sy).array()
}
class UdpCodec(key: ByteArray, private val prefix: ByteArray, private val session: Long, private val kind: Int) {
    private val key = SecretKeySpec(key, "AES")
    private val cipher = Cipher.getInstance("AES/GCM/NoPadding")
    private var sequence = 0L
    @Synchronized fun seal(body: ByteArray): ByteArray {
        check(sequence != -1L) { "UDP sequence exhausted" }
        val seq = sequence++
        val header = ByteBuffer.allocate(24).order(ByteOrder.LITTLE_ENDIAN).put("TDK1".toByteArray()).put(kind.toByte()).put(ByteArray(3)).putLong(session).putLong(seq).array()
        val nonce = ByteBuffer.allocate(12).order(ByteOrder.LITTLE_ENDIAN).put(prefix).putLong(seq).array()
        cipher.init(Cipher.ENCRYPT_MODE, key, GCMParameterSpec(128, nonce)); cipher.updateAAD(header)
        return header + cipher.doFinal(body)
    }
}
fun comparisonCode(pin: String, client: ByteArray, server: ByteArray): String {
    val p = pin.chunked(2).map { it.toInt(16).toByte() }.toByteArray()
    val hash = MessageDigest.getInstance("SHA-256").digest("tapdeck-pair-v1".toByteArray() + p + client + server)
    return hash.take(16).joinToString("") { "%02X".format(it.toInt() and 255) }.chunked(4).joinToString(" ")
}
