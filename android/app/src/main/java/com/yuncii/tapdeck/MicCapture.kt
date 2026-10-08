package com.yuncii.tapdeck

import android.annotation.SuppressLint
import android.media.AudioFormat
import android.media.AudioRecord
import android.media.MediaRecorder
import android.os.Process
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.math.sqrt

class MicCapture(private val frame: (ByteArray, Long, Float) -> Unit, private val onError: (String) -> Unit = {}) {
    private val running = AtomicBoolean(false)
    @Volatile private var active: AtomicBoolean? = null
    @Volatile private var record: AudioRecord? = null
    @Volatile private var thread: Thread? = null
    /** Snapshot callbacks per capture thread, so a delayed old frame/error keeps its owner. */
    @SuppressLint("MissingPermission") @Synchronized fun start(
        frame: (ByteArray, Long, Float) -> Unit = this.frame,
        onError: (String) -> Unit = this.onError,
    ) {
        if (running.get()) return
        val minimum = AudioRecord.getMinBufferSize(48000, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT)
        check(minimum > 0) { "设备不支持 48 kHz 单声道录音" }
        val r = AudioRecord.Builder().setAudioSource(MediaRecorder.AudioSource.VOICE_RECOGNITION).setAudioFormat(AudioFormat.Builder().setSampleRate(48000).setChannelMask(AudioFormat.CHANNEL_IN_MONO).setEncoding(AudioFormat.ENCODING_PCM_16BIT).build()).setBufferSizeInBytes(maxOf(minimum, 3840)).build()
        try {
            check(r.state == AudioRecord.STATE_INITIALIZED) { "麦克风初始化失败" }
            r.startRecording(); check(r.recordingState == AudioRecord.RECORDSTATE_RECORDING) { "麦克风不可用" }
        } catch (error: Exception) { r.release(); throw error }
        val captureActive = AtomicBoolean(true)
        active = captureActive; record = r; running.set(true)
        thread = Thread({
            Process.setThreadPriority(Process.THREAD_PRIORITY_AUDIO)
            val samples = ShortArray(480); val bytes = ByteArray(960); var pos = 0L
            try { while (captureActive.get()) {
                var offset = 0
                while (captureActive.get() && offset < 480) {
                    val n = r.read(samples, offset, 480 - offset, AudioRecord.READ_BLOCKING)
                    if (n <= 0) { if (captureActive.get()) error("麦克风读取失败 ($n)"); return@Thread }; offset += n
                }
                if (!captureActive.get()) break
                var power = 0.0
                for (i in samples.indices) { val v = samples[i].toInt(); bytes[2 * i] = v.toByte(); bytes[2 * i + 1] = (v shr 8).toByte(); power += v.toDouble() * v }
                frame(bytes.copyOf(), pos, (sqrt(power / 480) / 32768).toFloat()); pos += 480
            } } catch (error: Exception) { if (captureActive.get()) onError(error.message ?: "录音意外停止") }
            finally { runCatching { r.stop() }; r.release(); if (record === r) { record = null; running.set(false) } }
        }, "TapDeck-Audio").apply { start() }
    }
    fun stop() { active?.set(false); running.set(false); val r = record; runCatching { r?.stop() }; val t = thread; if (t !== Thread.currentThread()) t?.join(150); thread = null }
}
