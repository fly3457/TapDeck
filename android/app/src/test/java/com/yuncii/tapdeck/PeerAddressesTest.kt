package com.yuncii.tapdeck

import kotlinx.serialization.encodeToString
import org.junit.Assert.*
import org.junit.Test

/** 同一台电脑在不同 Wi-Fi / 网段下的多个地址。 */
class PeerAddressesTest {
    private val home = Peer("192.168.1.2", 41443, 41080, "ab".repeat(32), "PC", "token")

    @Test fun newAddressBecomesCurrentAndOldOnesAreKept() {
        val office = home.copy(host = "10.0.8.15")
        val catalog = PeerCatalog().upsert(home).upsert(office)
        val saved = catalog.find(home.id)!!
        assertEquals("10.0.8.15", saved.peer.host)
        assertEquals(listOf(PeerAddress("192.168.1.2", 41080, 41443)), saved.addresses)
        assertEquals(listOf("10.0.8.15", "192.168.1.2"), saved.candidates().map { it.host })
        // 回到家里的网络：家里的地址回到最前，办公室地址保留。
        val back = catalog.upsert(home).find(home.id)!!
        assertEquals(listOf("192.168.1.2", "10.0.8.15"), back.candidates().map { it.host })
        assertEquals(1, back.addresses.size)
        // 凭据、名称与地址一起保留在各候选里。
        assertTrue(back.candidates().all { it.token == "token" && it.pin == home.id })
    }

    @Test fun sameAddressIsNotDuplicatedAndListIsCapped() {
        var catalog = PeerCatalog().upsert(home).upsert(home)
        assertTrue(catalog.find(home.id)!!.addresses.isEmpty())
        for (i in 1..20) catalog = catalog.upsert(home.copy(host = "10.0.0.$i"))
        val saved = catalog.validated().find(home.id)!!
        assertEquals(PeerCatalog.MAX_ADDRESSES, saved.candidates().size)
        assertEquals("10.0.0.20", saved.peer.host)
        assertEquals("10.0.0.19", saved.addresses.first().host)
        // 端口变化视为不同地址。
        val moved = PeerCatalog().upsert(home).upsert(home.copy(httpPort = 42080)).find(home.id)!!
        assertEquals(2, moved.candidates().size)
    }

    @Test fun legacyCatalogWithoutAddressesStillDecodes() {
        val legacy = """{"schemaVersion":1,"peers":[{"peer":{"host":"192.168.1.2","wssPort":41443,"httpPort":41080,"pin":"${"ab".repeat(32)}","name":"PC","token":"t"}}],"selectedId":"${"ab".repeat(32)}"}"""
        val catalog = wireJson.decodeFromString<PeerCatalog>(legacy).validated()
        assertEquals(listOf("192.168.1.2"), catalog.selected!!.candidates().map { it.host })
        val withAddresses = catalog.upsert(home.copy(host = "10.1.1.1"))
        assertEquals(withAddresses, wireJson.decodeFromString<PeerCatalog>(wireJson.encodeToString(withAddresses)).validated())
    }

    @Test fun invalidStoredAddressIsRejected() {
        val bad = PeerCatalog(peers = listOf(SavedPeer(home, addresses = listOf(PeerAddress("", 41080, 41443)))))
        assertThrows(IllegalArgumentException::class.java) { bad.validated() }
    }
}
