package com.yuncii.tapdeck

/**
 * 两端时钟对齐：心跳回应带 PC 墙钟（pc_us），按 NTP 思路取往返最短的一次样本，
 * 假设去回对称，误差不超过该次往返的一半（局域网通常几毫秒）。
 */
internal class ClockSync(private val keep: Int = 40) {
    private val samples = ArrayDeque<Pair<Long, Long>>() // (往返 µs, PC 时钟 − 手机时钟 µs)

    /** sentUs、recvUs 为手机单调时钟（elapsedRealtime，微秒），pcUs 为 PC 回应时的墙钟。 */
    fun sample(sentUs: Long, recvUs: Long, pcUs: Long) {
        val rtt = recvUs - sentUs
        if (rtt < 0 || pcUs <= 0) return
        samples.addLast(rtt to pcUs - (sentUs + rtt / 2))
        while (samples.size > keep) samples.removeFirst()
    }

    fun reset() = samples.clear()

    /** PC 墙钟换算为手机单调时钟；还没有样本时为 null。 */
    fun toLocal(pcUs: Long): Long? = samples.minByOrNull { it.first }?.let { pcUs - it.second }
}

/**
 * 视频端到端延迟：PC 截屏 → 编码输出 → 发送队列 → 网络 → 手机解码输出。不含手机屏幕合成与显示（约 1–2 帧）。
 * 每个画面帧前的帧时间消息给出 PC 侧两段耗时与发送时刻，手机记录到达与解码输出时刻，按 pts 对应。
 */
internal class VideoLatency(private val windowUs: Long = 1_000_000) {
    data class Sample(val totalUs: Long, val encodeUs: Long, val queueUs: Long, val networkUs: Long, val decodeUs: Long)

    private val timings = object : LinkedHashMap<Long, VideoTiming>() {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<Long, VideoTiming>?) = size > MAX_TRACKED
    }
    private val arrivals = object : LinkedHashMap<Long, Long>() {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<Long, Long>?) = size > MAX_TRACKED
    }
    private val recent = ArrayDeque<Pair<Long, Sample>>()

    fun timing(t: VideoTiming) { timings[t.ptsUs] = t }

    fun arrived(ptsUs: Long, atUs: Long) { if (ptsUs in timings) arrivals[ptsUs] = atUs }

    /** 解码器输出 pts 对应的帧时调用；缺少帧时间（旧版 PC）或尚未对齐时钟时返回 null。 */
    fun decoded(ptsUs: Long, atUs: Long, toLocal: (Long) -> Long?): Sample? {
        val t = timings.remove(ptsUs) ?: return null
        val arrived = arrivals.remove(ptsUs) ?: return null
        val sentLocal = toLocal(t.pcSendUs) ?: return null
        val network = (arrived - sentLocal).coerceAtLeast(0)
        val decode = (atUs - arrived).coerceAtLeast(0)
        val s = Sample(t.encodeUs + t.queueUs + network + decode, t.encodeUs, t.queueUs, network, decode)
        recent.addLast(atUs to s)
        while (recent.isNotEmpty() && atUs - recent.first().first > windowUs) recent.removeFirst()
        return s
    }

    fun reset() { timings.clear(); arrivals.clear(); recent.clear() }

    /** 最近一个窗口内各段的中位数；没有样本时为 null。 */
    fun median(): Sample? {
        if (recent.isEmpty()) return null
        fun mid(f: (Sample) -> Long) = recent.map { f(it.second) }.sorted()[recent.size / 2]
        return Sample(mid { it.totalUs }, mid { it.encodeUs }, mid { it.queueUs }, mid { it.networkUs }, mid { it.decodeUs })
    }

    private companion object { const val MAX_TRACKED = 240 }
}
