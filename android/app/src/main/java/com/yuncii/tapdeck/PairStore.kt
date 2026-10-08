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
import kotlinx.serialization.encodeToString

private val Context.dataStore by preferencesDataStore("tapdeck")
class PairStore(private val context: Context) {
    private val peerKey = stringPreferencesKey("protected_peer")
    private val catalogKey = stringPreferencesKey("protected_peers_v1")
    private val idKey = stringPreferencesKey("device_id")
    private val ballX = floatPreferencesKey("voice_ball_x")
    private val ballY = floatPreferencesKey("voice_ball_y")
    private val voiceModeKey = stringPreferencesKey("voice_mode")
    private val voiceProfileKey = stringPreferencesKey("voice_profile_id")
    private val keyboardKey = booleanPreferencesKey("keyboard_mode")
    private val sensitivityKey = doublePreferencesKey("touchpad_sensitivity_multiplier")
    private val hapticsKey = booleanPreferencesKey("key_haptics")
    private fun key(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (ks.getKey("tapdeck-pair", null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder("tapdeck-pair", KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT).setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }
    suspend fun deviceId(): String {
        val saved = context.dataStore.edit { if (it[idKey] == null) it[idKey] = UUID.randomUUID().toString() }
        return requireNotNull(saved[idKey])
    }
    private fun decrypt(raw: String): String {
        val b = decode64(raw)
        require(b.size >= 28) { "配对数据损坏" }
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, b.copyOfRange(0, 12)))
        return cipher.doFinal(b.copyOfRange(12, b.size)).toString(Charsets.UTF_8)
    }
    private fun encrypt(text: String): String {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key())
        return encode64(cipher.iv + cipher.doFinal(text.toByteArray(Charsets.UTF_8)))
    }
    /** Read/migrate/write in ONE DataStore transaction, including across PairStore instances.
     * Decode, encryption or disk failure leaves the original data untouched. */
    suspend fun updateCatalog(change: (PeerCatalog) -> PeerCatalog): PeerCatalog {
        var result = PeerCatalog()
        context.dataStore.edit { prefs ->
            val raw = prefs[catalogKey]
            val current = if (raw != null) wireJson.decodeFromString<PeerCatalog>(decrypt(raw)).validated() else
                PeerCatalog.migrate(prefs[peerKey]?.let { wireJson.decodeFromString<Peer>(decrypt(it)) }, prefs[voiceProfileKey], prefs[voiceModeKey])
            result = change(current).validated()
            if (raw == null || result != current) prefs[catalogKey] = encrypt(wireJson.encodeToString(result))
            prefs.remove(peerKey)
            prefs.remove(voiceProfileKey)
            prefs.remove(voiceModeKey)
        }
        return result
    }
    suspend fun loadCatalog() = updateCatalog { it }
    suspend fun load(): Peer? = loadCatalog().selected?.peer
    suspend fun save(peer: Peer) { updateCatalog { it.upsert(peer).select(peer.id) } }
    suspend fun clear() { updateCatalog { catalog -> catalog.selectedId?.let(catalog::remove) ?: catalog } }
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

    suspend fun loadVoiceSelection(): VoiceSelection {
        val selected = loadCatalog().selected
        return VoiceSelection(selected?.voiceProfileId, selected?.legacyVoiceMode)
    }

    suspend fun saveVoiceProfile(id: String) {
        updateCatalog { catalog -> catalog.selectedId?.let { catalog.voice(it, id) } ?: catalog }
    }

    suspend fun saveKeyboardMode(on: Boolean) {
        context.dataStore.edit { it[keyboardKey] = on }
    }

    suspend fun loadInputSettings(): DeviceInputSettings {
        val p = context.dataStore.data.first()
        return DeviceInputSettings(p[sensitivityKey] ?: 1.0, p[hapticsKey] ?: true).normalized()
    }

    suspend fun saveInputSettings(settings: DeviceInputSettings) {
        val value = settings.normalized()
        context.dataStore.edit { it[sensitivityKey] = value.sensitivity; it[hapticsKey] = value.haptics }
    }
}
