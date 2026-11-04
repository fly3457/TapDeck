package com.yuncii.tapdeck

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class PcNotificationsTest {
    private fun wire(seq: Long, time: Long = seq * 1000) = WireNotification(seq, "微信", "张三 $seq", "内容 $seq", time)

    @Test fun acceptsOnlyNewerSeqWithinSameBoot() {
        val (first, fresh) = NotificationInbox().accept("pc", "书房", "boot-a", listOf(wire(1), wire(2)))
        assertEquals(listOf(1L, 2L), fresh.map { it.seq })
        assertEquals(NotifyCursor("boot-a", 2), first.cursor("pc"))
        assertEquals(2, first.unread)
        // 补发重复项与旧 seq 不再计入。
        val (second, again) = first.accept("pc", "书房", "boot-a", listOf(wire(2), wire(3), wire(3)))
        assertEquals(listOf(3L), again.map { it.seq })
        assertEquals(listOf(3L, 2L, 1L), second.items.map { it.seq })
        assertEquals(NotifyCursor("boot-a", 3), second.cursor("pc"))
    }

    @Test fun newBootRestartsSequence() {
        val (first, _) = NotificationInbox().accept("pc", "书房", "boot-a", listOf(wire(5)))
        val (second, fresh) = first.accept("pc", "书房", "boot-b", listOf(wire(1, time = 9000)))
        assertEquals(listOf(1L), fresh.map { it.seq })
        assertEquals(NotifyCursor("boot-b", 1), second.cursor("pc"))
        assertEquals(2, second.items.size)
    }

    @Test fun peersAreIndependentAndForgettable() {
        val (a, _) = NotificationInbox().accept("pc1", "A", "x", listOf(wire(1)))
        val (b, fresh) = a.accept("pc2", "B", "x", listOf(wire(1)))
        assertEquals(1, fresh.size)
        val forgotten = b.forget("pc1")
        assertEquals(listOf("pc2"), forgotten.items.map { it.peerId })
        assertEquals(null, forgotten.cursor("pc1"))
    }

    @Test fun limitReadClearAndValidation() {
        val many = (1L..150L).map { wire(it) }
        val (inbox, fresh) = NotificationInbox().accept("pc", "A", "x", many)
        assertEquals(150, fresh.size)
        assertEquals(NotificationInbox.LIMIT, inbox.items.size)
        assertEquals(150L, inbox.items.first().seq)
        val read = inbox.markRead()
        assertEquals(0, read.unread)
        val cleared = read.clear()
        assertTrue(cleared.items.isEmpty())
        // 清空后游标保留：PC 再补发同一批不会重新出现。
        assertTrue(cleared.accept("pc", "A", "x", many).second.isEmpty())
        val duplicated = NotificationInbox(items = inbox.items + inbox.items).validated()
        assertEquals(NotificationInbox.LIMIT, duplicated.items.size)
    }

    @Test fun alertsOnlyRecentAndCapped() {
        val now = 100 * 60 * 1000L
        val fresh = (1L..8L).map { PcNotification("pc", "A", "x", it, time = now - 1000 * it) } +
            PcNotification("pc", "A", "x", 9, time = now - NotificationInbox.ALERT_WINDOW_MS - 1)
        val alert = NotificationInbox.alertable(fresh, now)
        assertEquals(NotificationInbox.ALERT_LIMIT, alert.size)
        assertTrue(alert.none { it.seq == 9L })
        assertEquals("微信 · 张三", PcNotification("pc", boot = "x", seq = 1, app = "微信", title = "张三").heading)
        assertEquals("张三", PcNotification("pc", boot = "x", seq = 1, title = "张三").heading)
        assertEquals("电脑通知", PcNotification("pc", boot = "x", seq = 1).heading)
    }

    @Test fun emptyBootIgnored() {
        val (inbox, fresh) = NotificationInbox().accept("pc", "A", "", listOf(wire(1)))
        assertTrue(fresh.isEmpty() && inbox.items.isEmpty())
    }
}
