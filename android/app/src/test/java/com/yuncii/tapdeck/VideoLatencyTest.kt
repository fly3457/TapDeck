package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test
import java.nio.ByteBuffer
import java.nio.ByteOrder

class VideoLatencyTest {
    @Test fun clockSyncUsesShortestRoundTrip() {
        val c = ClockSync()
        assertNull(c.toLocal(1_000))
        // PC 时钟比手机快 5 秒。往返 40ms 的样本误差大，往返 2ms 的样本应被采用。
        val offset = 5_000_000L
        c.sample(sentUs = 100_000, recvUs = 140_000, pcUs = 100_000 + 30_000 + offset)
        c.sample(sentUs = 200_000, recvUs = 202_000, pcUs = 201_000 + offset)
        assertEquals(1_000_000L, c.toLocal(1_000_000 + offset))
        c.reset()
        assertNull(c.toLocal(1_000))
    }

    @Test fun clockSyncIsSafeAcrossThreads() {
        // 0.5.11–0.5.16 的崩溃：网络线程写样本、解码线程读样本同时进行，抛 ConcurrentModificationException。
        val c = ClockSync(keep = 8)
        val stop = java.util.concurrent.atomic.AtomicBoolean(false)
        val writer = Thread { var i = 0L; while (!stop.get()) { c.sample(i, i + 1_000 + i % 7, 5_000_000 + i); i++ } }
        writer.start()
        try { repeat(200_000) { c.toLocal(1_000_000) } } finally { stop.set(true); writer.join() }
        assertNotNull(c.toLocal(1_000_000))
    }

    @Test fun latencyAddsAllSegments() {
        val l = VideoLatency()
        val toLocal: (Long) -> Long? = { it - 5_000_000 } // PC 时钟比手机快 5 秒
        l.timing(VideoTiming(ptsUs = 16_000, encodeUs = 6_000, queueUs = 1_000, pcSendUs = 5_100_000))
        l.arrived(16_000, atUs = 108_000) // 发送后 8ms 到达
        val s = l.decoded(16_000, atUs = 120_000, toLocal)!!
        assertEquals(6_000L, s.encodeUs); assertEquals(1_000L, s.queueUs)
        assertEquals(8_000L, s.networkUs); assertEquals(12_000L, s.decodeUs)
        assertEquals(27_000L, s.totalUs)
        assertEquals(27_000L, l.median()!!.totalUs)
    }

    @Test fun framesWithoutTimingOrClockAreSkipped() {
        val l = VideoLatency()
        l.arrived(16_000, 1_000) // 旧版 PC：没有帧时间
        assertNull(l.decoded(16_000, 2_000) { it })
        l.timing(VideoTiming(33_000, 0, 0, 10))
        l.arrived(33_000, 20)
        assertNull("时钟未对齐时不出数", l.decoded(33_000, 30) { null })
        assertNull(l.median())
    }

    @Test fun medianIgnoresSpikesAndOldSamples() {
        val l = VideoLatency(windowUs = 1_000_000)
        fun frame(pts: Long, decodedAt: Long, total: Long) {
            l.timing(VideoTiming(pts, 0, 0, decodedAt - total))
            l.arrived(pts, decodedAt - total)
            l.decoded(pts, decodedAt) { it }
        }
        frame(1, 100_000, 500_000) // 窗口外的旧样本
        for (i in 0 until 9) frame(10L + i, 2_000_000L + i * 16_000, 40_000)
        frame(100, 2_200_000, 300_000) // 单帧尖峰
        assertEquals(40_000L, l.median()!!.totalUs)
    }

    @Test fun parsesTimingMessage() {
        val b = ByteBuffer.allocate(25).order(ByteOrder.LITTLE_ENDIAN)
        b.put(3).putLong(1_500_000).putInt(7_000).putInt(3_000).putLong(1_700_000_000_000_000)
        val t = parseVideoMessage(b.array()) as VideoTiming
        assertEquals(VideoTiming(1_500_000, 7_000, 3_000, 1_700_000_000_000_000), t)
        assertNull("太短的帧时间消息忽略", parseVideoMessage(ByteArray(10).also { it[0] = 3 }))
        val f = parseVideoMessage(byteArrayOf(1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0x65), arrivedUs = 42) as VideoFrame
        assertEquals(42L, f.arrivedUs)
    }
}
