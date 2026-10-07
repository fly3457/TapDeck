package com.yuncii.tapdeck

import org.junit.Assert.*
import org.junit.Test

class PairingCredentialTest {
    private val saved = Peer("192.168.1.2", 41443, 41080, "ab".repeat(32), "PC", "existing-token")

    @Test fun upgradedAppReusesSavedCredentialFromWebLink() {
        val link = saved.copy(token = "", name = "电脑")
        assertEquals("existing-token", link.withCredentialFrom(saved).token)
    }

    @Test fun changedAddressStillRequiresSameTlsIdentity() {
        val moved = saved.copy(host = "192.168.1.9", wssPort = 42443, token = "", pin = saved.pin.uppercase())
        assertEquals("existing-token", moved.withCredentialFrom(saved).token)
        assertEquals(42443, moved.withCredentialFrom(saved).wssPort)
        assertEquals("", moved.copy(pin = "cd".repeat(32)).withCredentialFrom(saved).token)
        assertEquals("", moved.withCredentialFrom(null).token)
    }

    @Test fun revocationMatchesCredentialAcrossMetadataChanges() {
        assertTrue(saved.sameCredential(saved.copy(host = "192.168.1.9", name = "Renamed")))
        assertFalse(saved.sameCredential(saved.copy(token = "new-approved-token")))
        assertFalse(saved.sameCredential(saved.copy(pin = "cd".repeat(32))))
    }
}
