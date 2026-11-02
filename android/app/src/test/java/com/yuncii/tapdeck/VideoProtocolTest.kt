package com.yuncii.tapdeck

import java.nio.ByteBuffer
import java.nio.ByteOrder
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.*
import org.junit.Test

class VideoProtocolTest {
    @Test fun parsesFramesAndCursor() {
        val frame = ByteBuffer.allocate(14).order(ByteOrder.LITTLE_ENDIAN).put(1).put(3).putLong(16000).put(byteArrayOf(0, 0, 1, 0x67)).array()
        val f = parseVideoMessage(frame) as VideoFrame
        assertTrue(f.key); assertTrue(f.config); assertEquals(16000L, f.ptsUs); assertEquals(4, f.size); assertEquals(0x67.toByte(), f.data[f.offset + 3])
        val inter = parseVideoMessage(byteArrayOf(1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 9)) as VideoFrame
        assertFalse(inter.key); assertFalse(inter.config)
        val cursor = ByteBuffer.allocate(10).order(ByteOrder.LITTLE_ENDIAN).put(2).putInt(-5).putInt(1079).put(1).array()
        assertEquals(VideoCursor(-5, 1079, true), parseVideoMessage(cursor))
    }

    @Test fun rejectsTruncatedOrUnknownMessages() {
        assertNull(parseVideoMessage(byteArrayOf()))
        assertNull(parseVideoMessage(byteArrayOf(1, 0, 0)))
        assertNull(parseVideoMessage(ByteArray(10).also { it[0] = 1 }))
        assertNull(parseVideoMessage(byteArrayOf(2, 0, 0)))
        assertNull(parseVideoMessage(byteArrayOf(9, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0)))
    }

    @Test fun formatValidatesDimensions() {
        val ok = wireJson.parseToJsonElement("""{"width":1920,"height":1080,"screen_width":2560,"screen_height":1440,"fps":60,"encoder":"h264_nvenc"}""").jsonObject
        assertEquals(VideoFormat(1920, 1080, 2560, 1440, 60, "h264_nvenc"), VideoFormat.of(ok))
        val bad = wireJson.parseToJsonElement("""{"width":0,"height":1080,"screen_width":2560,"screen_height":1440}""").jsonObject
        assertNull(VideoFormat.of(bad))
    }

    @Test fun qualityFallsBackToSmooth() {
        assertEquals(VideoQuality.Clear, VideoQuality.of("clear"))
        assertEquals(VideoQuality.Smooth, VideoQuality.of(null))
        assertEquals(VideoQuality.Smooth, VideoQuality.of("4k"))
    }
}
