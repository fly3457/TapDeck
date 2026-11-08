package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class PcAudioTest {
    @Test fun bufferPolicyStartsAfterPrimeAndDropsBacklog() {
        assertFalse("预缓冲不足 30ms 不开始播放", PcAudioBuffer.shouldStart(PcAudioBuffer.START_FRAMES - 1L))
        assertTrue(PcAudioBuffer.shouldStart(PcAudioBuffer.START_FRAMES.toLong()))
        assertFalse("60ms 以内照常写入", PcAudioBuffer.shouldDrop(PcAudioBuffer.MAX_FRAMES.toLong()))
        assertTrue("积压超过 60ms 丢弃新包", PcAudioBuffer.shouldDrop(PcAudioBuffer.MAX_FRAMES + 1L))
        assertEquals(1440, PcAudioBuffer.START_FRAMES)
        assertEquals(2880, PcAudioBuffer.MAX_FRAMES)
    }

    @Test fun parsesAudioMessage() {
        val pcm = ByteArray(1 + 1920).also { it[0] = 4; it[1] = 7 }
        val a = parseVideoMessage(pcm) as VideoAudio
        assertEquals(1920, a.size); assertEquals(1, a.offset); assertEquals(7.toByte(), a.data[a.offset])
        assertNull("不是整帧（4 字节）的数据忽略", parseVideoMessage(byteArrayOf(4, 1, 2, 3)))
        assertNull("空的声音消息忽略", parseVideoMessage(byteArrayOf(4)))
    }
}
