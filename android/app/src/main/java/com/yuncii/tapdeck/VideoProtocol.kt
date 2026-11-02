package com.yuncii.tapdeck

import java.nio.ByteBuffer
import java.nio.ByteOrder
import kotlinx.serialization.json.JsonObject

const val VIDEO_FEATURE = "video"

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
class VideoFrame(val data: ByteArray, val offset: Int, val key: Boolean, val config: Boolean, val ptsUs: Long) : VideoMessage {
    val size get() = data.size - offset
}
data class VideoCursor(val x: Int, val y: Int, val visible: Boolean) : VideoMessage

/** 解析 /video 的二进制消息：1=画面帧（flags、int64 pts 微秒、Annex-B），2=光标。未知类型返回 null。 */
fun parseVideoMessage(b: ByteArray): VideoMessage? {
    if (b.isEmpty()) return null
    val buffer = ByteBuffer.wrap(b).order(ByteOrder.LITTLE_ENDIAN)
    return when (b[0].toInt()) {
        1 -> if (b.size > 10) VideoFrame(b, 10, b[1].toInt() and 1 != 0, b[1].toInt() and 2 != 0, buffer.getLong(2)) else null
        2 -> if (b.size >= 10) VideoCursor(buffer.getInt(1), buffer.getInt(5), b[9].toInt() != 0) else null
        else -> null
    }
}
