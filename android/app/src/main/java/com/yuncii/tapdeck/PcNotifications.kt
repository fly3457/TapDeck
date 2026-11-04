package com.yuncii.tapdeck

import kotlinx.serialization.Serializable

/** PC 声明此能力时转发 Windows 系统通知，并接受 link_mode 后台心跳。 */
const val NOTIFY_FEATURE = "notify"

/** PC 发来的一条通知（`notifications.items[]`）。time 为 Unix 毫秒。 */
@Serializable data class WireNotification(val seq: Long, val app: String = "", val title: String = "", val body: String = "", val time: Long = 0)

/** 手机保存的电脑通知；同一台 PC 的 (boot, seq) 唯一。 */
@Serializable data class PcNotification(
    val peerId: String,
    val peerName: String = "",
    val boot: String,
    val seq: Long,
    val app: String = "",
    val title: String = "",
    val body: String = "",
    val time: Long = 0,
    val read: Boolean = false,
) {
    val key: String get() = "$peerId/$boot/$seq"
    /** 系统通知与列表的标题：应用名 · 通知标题，任一为空时只显示另一项。 */
    val heading: String get() = listOf(app, title).filter { it.isNotBlank() }.joinToString(" · ").ifEmpty { "电脑通知" }
}

/** 每台 PC 已收到的最后一条通知：PC 重启后 boot 变化，seq 重新计数。 */
@Serializable data class NotifyCursor(val boot: String, val seq: Long)

/** 手机端的电脑通知收件箱：最近 [LIMIT] 条通知与各 PC 的补发游标。 */
@Serializable data class NotificationInbox(
    val items: List<PcNotification> = emptyList(),
    val cursors: Map<String, NotifyCursor> = emptyMap(),
) {
    val unread: Int get() = items.count { !it.read }

    fun cursor(peerId: String): NotifyCursor? = cursors[peerId]

    /**
     * 合并 PC 推送或补发的通知，返回新收件箱与本次新增项（按 seq 升序）。
     * 同一 boot 内只接受比游标新的 seq；boot 变化表示 PC 已重启，全部视为新通知。
     */
    fun accept(peerId: String, peerName: String, boot: String, incoming: List<WireNotification>): Pair<NotificationInbox, List<PcNotification>> {
        if (boot.isEmpty()) return this to emptyList()
        val cursor = cursors[peerId]?.takeIf { it.boot == boot }
        val known = items.asSequence().filter { it.peerId == peerId && it.boot == boot }.map { it.seq }.toSet()
        val fresh = incoming.asSequence()
            .filter { it.seq > (cursor?.seq ?: 0) && it.seq !in known }
            .distinctBy { it.seq }
            .sortedBy { it.seq }
            .map { PcNotification(peerId, peerName, boot, it.seq, it.app, it.title, it.body, it.time) }
            .toList()
        val last = maxOf(cursor?.seq ?: 0, incoming.maxOfOrNull { it.seq } ?: 0)
        val merged = (fresh.reversed() + items).sortedByDescending { it.time }.take(LIMIT)
        return copy(items = merged, cursors = cursors + (peerId to NotifyCursor(boot, last))) to fresh
    }

    fun markRead(): NotificationInbox = if (unread == 0) this else copy(items = items.map { if (it.read) it else it.copy(read = true) })

    /** 清空列表但保留游标，避免下次连接时 PC 再补发同样的通知。 */
    fun clear(): NotificationInbox = copy(items = emptyList())

    /** 忘记电脑时删除它的通知与游标。 */
    fun forget(peerId: String): NotificationInbox = copy(items = items.filter { it.peerId != peerId }, cursors = cursors - peerId)

    fun validated(): NotificationInbox = copy(items = items.distinctBy { it.key }.sortedByDescending { it.time }.take(LIMIT))

    companion object {
        const val LIMIT = 100
        /** 首次连接或 PC 重启后补发的旧通知只进列表；只有这段时间内的才弹系统通知。 */
        const val ALERT_WINDOW_MS = 10 * 60 * 1000L
        /** 一次最多弹出的系统通知条数，其余只计入列表。 */
        const val ALERT_LIMIT = 5

        /** 从本次新增项中挑出需要弹系统通知的（最近且最多 [ALERT_LIMIT] 条）。 */
        fun alertable(fresh: List<PcNotification>, now: Long): List<PcNotification> =
            fresh.filter { now - it.time < ALERT_WINDOW_MS }.takeLast(ALERT_LIMIT)
    }
}
