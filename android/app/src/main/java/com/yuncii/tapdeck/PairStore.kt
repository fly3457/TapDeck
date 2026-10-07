package com.yuncii.tapdeck

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.datastore.preferences.core.*
import androidx.datastore.preferences.preferencesDataStore
import java.security.KeyStore
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import kotlinx.coroutines.flow.first
import kotlinx.serialization.Serializable
import kotlinx.serialization.encodeToString

private val Context.dataStore by preferencesDataStore("tapdeck")
@Serializable data class Peer(val host: String, val wssPort: Int, val httpPort: Int, val pin: String, val name: String = "电脑", val token: String = "")
class PairStore(private val context: Context) {
    private val peerKey = stringPreferencesKey("protected_peer")
    private val idKey = stringPreferencesKey("device_id")
    private val ballX = floatPreferencesKey("voice_ball_x")
    private val ballY = floatPreferencesKey("voice_ball_y")
    private val voiceModeKey = stringPreferencesKey("voice_mode")
    private val keyboardKey = booleanPreferencesKey("keyboard_mode")
    private fun key(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (ks.getKey("tapdeck-pair", null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder("tapdeck-pair", KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT).setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }
    suspend fun deviceId(): String { val p = context.dataStore.data.first(); p[idKey]?.let { return it }; val id = UUID.randomUUID().toString(); context.dataStore.edit { it[idKey] = id }; return id }
    suspend fun load(): Peer? { val raw = context.dataStore.data.first()[peerKey] ?: return null; return runCatching { val b = decode64(raw); val cipher = Cipher.getInstance("AES/GCM/NoPadding"); cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, b.copyOfRange(0, 12))); wireJson.decodeFromString<Peer>(String(cipher.doFinal(b.copyOfRange(12, b.size)))) }.getOrNull() }
    suspend fun save(peer: Peer) { val c = Cipher.getInstance("AES/GCM/NoPadding"); c.init(Cipher.ENCRYPT_MODE, key()); val b = c.iv + c.doFinal(wireJson.encodeToString(peer).toByteArray()); context.dataStore.edit { it[peerKey] = encode64(b) } }
    suspend fun clear() { context.dataStore.edit { it.remove(peerKey) } }
    suspend fun loadBallPosition(): Pair<Float, Float> {
        val p = context.dataStore.data.first()
        fun safe(v: Float?) = v?.takeIf { it.isFinite() }?.coerceIn(0f, 1f) ?: 0.5f
        return safe(p[ballX]) to safe(p[ballY])
    }
    suspend fun saveBallPosition(x: Float, y: Float) {
        context.dataStore.edit { it[ballX] = x.coerceIn(0f, 1f); it[ballY] = y.coerceIn(0f, 1f) }
    }

    /** 语音输入方式（"hold" 长按 / "toggle" 单击）与全键盘开关，重启 App 后继续沿用。 */
    suspend fun loadUiMode(): Pair<String, Boolean> {
        val p = context.dataStore.data.first()
        val mode = p[voiceModeKey]?.takeIf { it == MicBallView.MODE_TOGGLE || it == MicBallView.MODE_HOLD } ?: MicBallView.MODE_HOLD
        return mode to (p[keyboardKey] ?: false)
    }

    suspend fun saveVoiceMode(mode: String) {
        context.dataStore.edit { it[voiceModeKey] = if (mode == MicBallView.MODE_TOGGLE) MicBallView.MODE_TOGGLE else MicBallView.MODE_HOLD }
    }

    suspend fun saveKeyboardMode(on: Boolean) {
        context.dataStore.edit { it[keyboardKey] = on }
    }
}
