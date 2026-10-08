package com.yuncii.tapdeck

import kotlinx.serialization.encodeToString
import org.junit.Assert.*
import org.junit.Test

class PeerCatalogTest {
    private val a = Peer("192.168.1.2", 41443, 41080, "ab".repeat(32), "PC", "token-a")
    private val b = a.copy(pin = "cd".repeat(32), token = "token-b", host = "192.168.1.3")

    @Test fun legacyMigrationKeepsIdentityCredentialAndVoicePreference() {
        val old = a.copy(pin = a.pin.uppercase())
        val catalog = PeerCatalog.migrate(old, "voice-2", "hold")
        assertEquals(a.id, catalog.selectedId)
        assertEquals(a, catalog.selected!!.peer)
        assertEquals("voice-2", catalog.selected!!.voiceProfileId)
        assertEquals("hold", catalog.selected!!.legacyVoiceMode)
        assertEquals(PeerCatalog(), PeerCatalog.migrate(null, "voice-2", "hold"))
        assertEquals(catalog, wireJson.decodeFromString<PeerCatalog>(wireJson.encodeToString(catalog)).validated())
    }
    @Test fun sameIdentityMovesAddressWithoutLosingAliasVoiceOrPosition() {
        val initial = PeerCatalog().upsert(a).upsert(b).rename(a.id, "  办公电脑  ").voice(a.id, "voice-2").select(b.id)
        val next = initial.upsert(a.copy(host = "192.168.1.10", pin = a.pin.uppercase(), name = "Renamed", token = "fresh"))
        assertEquals(listOf(a.id, b.id), next.peers.map { it.id })
        assertEquals(listOf(b.id, a.id), next.ordered().map { it.id })
        assertEquals("办公电脑", next.find(a.id)!!.displayName)
        assertEquals("voice-2", next.find(a.id)!!.voiceProfileId)
        assertEquals("fresh", next.find(a.id)!!.peer.token)
        assertEquals("Renamed", next.rename(a.id, " ").find(a.id)!!.displayName)
        assertEquals(b, next.find(b.id)!!.peer)
    }
    @Test fun sameNameAndAddressDoNotMergeIdentities() {
        val second = b.copy(host = a.host)
        val catalog = PeerCatalog().upsert(a).upsert(second)
        assertEquals(2, catalog.peers.size)
        assertEquals("", second.copy(token = "").withCredentialFrom(a).token)
        assertEquals("token-a", a.copy(host = "192.168.1.9", token = "").withCredentialFrom(catalog.find(a.id)?.peer).token)
    }
    @Test fun revokingOrForgettingOneComputerNeverChangesAnother() {
        val catalog = PeerCatalog().upsert(a).upsert(b).voice(a.id, "voice-2").voice(b.id, "voice-1").rename(a.id, "家里").select(b.id)
        val revoked = catalog.revoke(a)
        assertEquals("家里", revoked.find(a.id)!!.displayName)
        assertTrue(revoked.find(a.id)!!.needsPairing)
        assertEquals(catalog.find(b.id), revoked.find(b.id))
        assertEquals(b.id, revoked.selectedId)
        val repaired = revoked.upsert(a.copy(token = "approved-again"))
        assertEquals(repaired, repaired.revoke(a))
        assertEquals(b.id, catalog.remove(a.id).selectedId)
        assertNull(catalog.remove(b.id).selectedId)
        assertEquals(listOf(a.id), catalog.remove(b.id).peers.map { it.id })
    }
    @Test fun selectionAndTemporaryPairingPreserveAllRecords() {
        val initial = PeerCatalog().upsert(a).upsert(b)
        val roundTrip = initial.select(a.id).select(b.id).select(a.id).select(null)
        assertEquals(initial.peers, roundTrip.peers)
        assertNull(roundTrip.selected)
        assertThrows(IllegalArgumentException::class.java) { initial.select("missing") }
        assertThrows(IllegalArgumentException::class.java) { initial.copy(schemaVersion = 9).validated() }
        assertThrows(IllegalArgumentException::class.java) { initial.copy(peers = initial.peers + initial.peers[0]).validated() }
        assertThrows(IllegalArgumentException::class.java) { PeerCatalog().upsert(a.copy(httpPort = 80)).validated() }
    }
}
