package com.yuncii.tapdeck

import android.media.MediaCodec
import android.media.MediaFormat
import android.os.Build
import android.os.Handler
import android.os.HandlerThread
import android.os.SystemClock
import android.util.Log
import android.view.Surface
import java.util.ArrayDeque

/**
 * MediaCodec 硬件 H.264 解码，直接输出到 Surface（scrcpy / Moonlight 同类做法）：
 * 异步回调模式，输出缓冲一到立即渲染；输入积压时丢到下一个关键帧，保证延迟不累积。
 *
 * 公开方法可在任意线程调用；解码器状态只在专用线程上读写，避免回调与 stop() 互相等待。
 */
class VideoDecoder {
    private val thread = HandlerThread("TapDeck-video").apply { start() }
    private val handler = Handler(thread.looper)
    private var codec: MediaCodec? = null
    private var surface: Surface? = null
    private var width = 0
    private var height = 0
    private var config: VideoFrame? = null
    private val pending = ArrayDeque<VideoFrame>()
    private val inputs = ArrayDeque<Int>()
    private var needKey = true
    private var frames = 0
    private var windowStart = SystemClock.elapsedRealtime()
    /** 最近一秒实际渲染的帧数，用于状态按钮。 */
    @Volatile var fps = 0; private set
    /** 在解码线程上回调。 */
    var onError: (String) -> Unit = {}

    fun setSurface(value: Surface?) = handler.post { if (surface !== value) { surface = value; restart() } }

    /** Surface 销毁回调中必须同步停止解码器，之后系统才会回收 Surface。 */
    fun detachSurfaceBlocking() {
        val done = java.util.concurrent.CountDownLatch(1)
        handler.post { surface = null; stopCodec(); done.countDown() }
        done.await(500, java.util.concurrent.TimeUnit.MILLISECONDS)
    }

    fun setFormat(w: Int, h: Int) = handler.post { if (w != width || h != height) { width = w; height = h; restart() } }

    fun submit(frame: VideoFrame) = handler.post {
        if (frame.config) {
            config = frame
            pending.clear(); pending.add(frame); needKey = true
        } else {
            if (needKey && !frame.key) return@post
            // 解码跟不上（或 Surface 尚未就绪）：清空旧帧，等下一个关键帧。
            if (pending.count { !it.config } >= MAX_PENDING) {
                pending.removeAll { !it.config }
                needKey = true
                if (!frame.key) return@post
            }
            needKey = false
            pending.add(frame)
        }
        feed()
    }

    /** 新的视频流：丢弃旧配置，等待新的 SPS/PPS。 */
    fun reset() = handler.post { config = null; pending.clear(); needKey = true; stopCodec() }

    fun release() {
        handler.post { stopCodec() }
        thread.quitSafely()
    }

    private fun restart() {
        stopCodec()
        val target = surface ?: return
        if (width <= 0 || height <= 0 || !target.isValid) return
        codec = try { create(target, true) } catch (e: Exception) {
            Log.w("TapDeck", "低延迟解码配置失败，改用默认参数", e)
            try { create(target, false) } catch (e2: Exception) {
                Log.w("TapDeck", "H.264 解码器不可用", e2)
                onError("手机无法解码视频：" + (e2.message ?: "解码器错误"))
                null
            }
        }
        // 新解码器必须从配置帧和关键帧开始。
        pending.clear(); needKey = true
        config?.let { pending.add(it) }
    }

    private fun create(target: Surface, lowLatency: Boolean): MediaCodec {
        val format = MediaFormat.createVideoFormat(MediaFormat.MIMETYPE_VIDEO_AVC, width, height).apply {
            setInteger(MediaFormat.KEY_MAX_INPUT_SIZE, width * height)
            if (lowLatency) {
                if (Build.VERSION.SDK_INT >= 30) setInteger(MediaFormat.KEY_LOW_LATENCY, 1)
                setInteger(MediaFormat.KEY_PRIORITY, 0)
                setInteger(MediaFormat.KEY_OPERATING_RATE, 120)
                // 厂商私有低延迟开关（Moonlight 同款），不支持的解码器会忽略。
                setInteger("vendor.qti-ext-dec-low-latency.enable", 1)
                setInteger("vendor.hisi-ext-low-latency-video-dec.video-scene-for-low-latency-req", 1)
                setInteger("vendor.rtc-ext-dec-low-latency.enable", 1)
            }
        }
        val c = MediaCodec.createDecoderByType(MediaFormat.MIMETYPE_VIDEO_AVC)
        try {
            c.setCallback(object : MediaCodec.Callback() {
                override fun onInputBufferAvailable(mc: MediaCodec, index: Int) {
                    if (codec !== mc) return
                    inputs.add(index); feed()
                }
                override fun onOutputBufferAvailable(mc: MediaCodec, index: Int, info: MediaCodec.BufferInfo) {
                    if (codec !== mc) return
                    try { mc.releaseOutputBuffer(index, true) } catch (_: IllegalStateException) { return }
                    countFrame()
                }
                override fun onError(mc: MediaCodec, e: MediaCodec.CodecException) {
                    Log.w("TapDeck", "解码错误", e)
                    if (codec === mc) restart()
                }
                override fun onOutputFormatChanged(mc: MediaCodec, format: MediaFormat) {}
            }, handler)
            c.configure(format, target, null, 0)
            c.start()
        } catch (e: Exception) {
            c.release(); throw e
        }
        inputs.clear()
        return c
    }

    private fun feed() {
        val c = codec ?: return
        while (inputs.isNotEmpty() && pending.isNotEmpty()) {
            val index = inputs.peek()!!
            val frame = pending.poll()!!
            try {
                val buffer = c.getInputBuffer(index) ?: return
                if (frame.size > buffer.capacity()) { pending.removeAll { !it.config }; needKey = true; continue }
                inputs.poll()
                buffer.clear(); buffer.put(frame.data, frame.offset, frame.size)
                val flags = (if (frame.config) MediaCodec.BUFFER_FLAG_CODEC_CONFIG else 0) or (if (frame.key) MediaCodec.BUFFER_FLAG_KEY_FRAME else 0)
                c.queueInputBuffer(index, 0, frame.size, frame.ptsUs, flags)
            } catch (e: IllegalStateException) {
                Log.w("TapDeck", "解码器状态异常", e); return
            }
        }
    }

    private fun countFrame() {
        val now = SystemClock.elapsedRealtime()
        frames++
        if (now - windowStart >= 1000) { fps = (frames * 1000 / (now - windowStart)).toInt(); frames = 0; windowStart = now }
    }

    private fun stopCodec() {
        val c = codec ?: return
        codec = null
        inputs.clear()
        runCatching { c.stop() }
        runCatching { c.release() }
        fps = 0
    }

    private companion object { const val MAX_PENDING = 4 }
}
