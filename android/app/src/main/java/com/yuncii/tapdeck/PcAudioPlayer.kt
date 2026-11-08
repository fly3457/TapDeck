package com.yuncii.tapdeck

import android.media.AudioAttributes
import android.media.AudioFormat
import android.media.AudioTrack
import android.os.Handler
import android.os.HandlerThread
import android.util.Log

/**
 * 电脑声音的缓冲策略（纯逻辑，便于测试）：先攒够 [START_FRAMES] 再开始播放，避免开头断续；
 * 已缓冲超过 [MAX_FRAMES] 时丢弃新包，让声音追上画面，不会越拖越晚。
 */
internal object PcAudioBuffer {
    const val RATE = 48000
    const val CHANNELS = 2
    const val BYTES_PER_FRAME = CHANNELS * 2
    /** 约 30 ms：开始播放前的预缓冲。 */
    const val START_FRAMES = RATE * 30 / 1000
    /** 约 150 ms：积压上限，超过即丢包。 */
    const val MAX_FRAMES = RATE * 150 / 1000

    fun shouldDrop(bufferedFrames: Long): Boolean = bufferedFrames > MAX_FRAMES
    fun shouldStart(bufferedFrames: Long): Boolean = bufferedFrames >= START_FRAMES
}

/**
 * 播放视频模式中转发来的电脑声音：AudioTrack 低延迟模式，在专用线程上写入。
 * 静音（开关关闭、手机正在语音输入）时释放 AudioTrack，立即停止出声。
 */
class PcAudioPlayer {
    private val thread = HandlerThread("TapDeck-pc-audio").apply { start() }
    private val handler = Handler(thread.looper)
    private var track: AudioTrack? = null
    private var written = 0L

    /** 为真时丢弃收到的声音；手机语音输入期间置位，避免手机喇叭的声音被麦克风录回电脑。 */
    @Volatile var muted = false
        set(v) { field = v; if (v) stop() }

    fun submit(a: VideoAudio) = handler.post { play(a) }

    /** 停止并释放 AudioTrack（退出视频模式、关闭声音、切到后台）。 */
    fun stop() { handler.post { releaseTrack() } }

    fun release() { handler.post { releaseTrack() }; thread.quitSafely() }

    private fun play(a: VideoAudio) {
        if (muted || a.size <= 0) return
        val t = track ?: create() ?: return
        val buffered = written - (t.playbackHeadPosition.toLong() and 0xffffffffL)
        if (PcAudioBuffer.shouldDrop(buffered)) return
        val n = t.write(a.data, a.offset, a.size, AudioTrack.WRITE_NON_BLOCKING)
        if (n > 0) written += n / PcAudioBuffer.BYTES_PER_FRAME
        if (t.playState != AudioTrack.PLAYSTATE_PLAYING && PcAudioBuffer.shouldStart(written - (t.playbackHeadPosition.toLong() and 0xffffffffL)))
            runCatching { t.play() }
    }

    private fun create(): AudioTrack? = runCatching {
        val channel = AudioFormat.CHANNEL_OUT_STEREO
        val encoding = AudioFormat.ENCODING_PCM_16BIT
        val min = AudioTrack.getMinBufferSize(PcAudioBuffer.RATE, channel, encoding)
        // 缓冲容量要能放下积压上限，实际延迟由上面的丢包策略控制。
        val size = maxOf(min, (PcAudioBuffer.MAX_FRAMES + PcAudioBuffer.START_FRAMES) * PcAudioBuffer.BYTES_PER_FRAME)
        AudioTrack.Builder()
            .setAudioAttributes(AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_MEDIA).setContentType(AudioAttributes.CONTENT_TYPE_MOVIE).build())
            .setAudioFormat(AudioFormat.Builder().setSampleRate(PcAudioBuffer.RATE).setChannelMask(channel).setEncoding(encoding).build())
            .setBufferSizeInBytes(size)
            .setTransferMode(AudioTrack.MODE_STREAM)
            .setPerformanceMode(AudioTrack.PERFORMANCE_MODE_LOW_LATENCY)
            .build()
    }.onFailure { Log.w("TapDeck", "电脑声音播放器创建失败", it) }.getOrNull()?.also { track = it; written = 0 }

    private fun releaseTrack() {
        val t = track ?: return
        track = null
        written = 0
        runCatching { t.pause(); t.flush(); t.release() }
    }
}
