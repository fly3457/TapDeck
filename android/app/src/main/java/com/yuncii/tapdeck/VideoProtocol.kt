package com.yuncii.tapdeck

import java.nio.ByteBuffer
import java.nio.ByteOrder
import kotlinx.serialization.json.JsonObject

const val VIDEO_FEATURE = "video"
/** 接收端 0.5.15 起：视频流可同时转发电脑声音。 */
const val VIDEO_AUDIO_FEATURE = "video_audio"

/** 手机可选画质档位；名称与 PC 端 video.Quality 一致。 */
enum class VideoQuality(val wire: String, val label: String, val detail: String) {
    Smooth("smooth", "流畅", "1080p 60fps"),
    Clear("clear", "清晰", "原生分辨率，最高 1440p 60fps"),
    Saver("saver", "省流", "720p 60fps");
    companion object { fun of(wire: String?) = entries.firstOrNull { it.wire == wire } ?: Smooth }
}

enum class VideoPhase { Idle, Starting, Streaming, Error }

data class VideoFormat(val width: Int, val height: Int, val screenWidth: Int, val screenHeight: Int, val fps: Int, val encoder: String) {
    companion object {
        fun of(m: JsonObject): VideoFormat? {
            val f = VideoFormat(m.long("width").toInt(), m.long("height").toInt(), m.long("screen_width").toInt(), m.long("screen_height").toInt(), m.long("fps", 60).toInt(), m.str("encoder"))
            return f.takeIf { it.width in 2..8192 && it.height in 2..8192 && it.screenWidth in 2..16384 && it.screenHeight in 2..16384 }
        }
    }
}

data class VideoState(val phase: VideoPhase = VideoPhase.Idle, val format: VideoFormat? = null, val error: String = "", val cursorVisible: Boolean = true)

sealed interface VideoMessage
/** arrivedUs 为收到该帧时的手机单调时钟（微秒），用于延迟统计。 */
class VideoFrame(val data: ByteArray, val offset: Int, val key: Boolean, val config: Boolean, val ptsUs: Long, val arrivedUs: Long = 0) : VideoMessage {
    val size get() = data.size - offset
}
data class VideoCursor(val x: Int, val y: Int, val visible: Boolean) : VideoMessage
/** 画面帧之前的帧时间（PC 0.5.11 起）：截屏到编码输出、发送队列等待（微秒）与发送时刻（PC 墙钟 Unix 微秒）。 */
data class VideoTiming(val ptsUs: Long, val encodeUs: Long, val queueUs: Long, val pcSendUs: Long) : VideoMessage
/** 电脑声音：48 kHz、立体声、16 位小端 PCM，data 从 offset 起，每包约 10 ms。 */
class VideoAudio(val data: ByteArray, val offset: Int) : VideoMessage {
    val size get() = data.size - offset
}

/** 解析 /video 的二进制消息：1=画面帧（flags、int64 pts 微秒、Annex-B），2=光标，3=帧时间，4=电脑声音。未知类型返回 null。 */
fun parseVideoMessage(b: ByteArray, arrivedUs: Long = 0): VideoMessage? {
    if (b.isEmpty()) return null
    val buffer = ByteBuffer.wrap(b).order(ByteOrder.LITTLE_ENDIAN)
    return when (b[0].toInt()) {
        1 -> if (b.size > 10) VideoFrame(b, 10, b[1].toInt() and 1 != 0, b[1].toInt() and 2 != 0, buffer.getLong(2), arrivedUs) else null
        3 -> if (b.size >= 25) VideoTiming(buffer.getLong(1), buffer.getInt(9).toLong() and 0xffffffffL, buffer.getInt(13).toLong() and 0xffffffffL, buffer.getLong(17)) else null
        4 -> if (b.size > 1 && (b.size - 1) % 4 == 0) VideoAudio(b, 1) else null
        2 -> if (b.size >= 10) VideoCursor(buffer.getInt(1), buffer.getInt(5), b[9].toInt() != 0) else null
        else -> null
    }
}
