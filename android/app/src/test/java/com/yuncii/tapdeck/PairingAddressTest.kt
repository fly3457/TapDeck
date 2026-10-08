package com.yuncii.tapdeck

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PairingAddressTest {
    @Test fun acceptsPcPairingUrlsAndTrimsSurroundingWhitespace() {
        for (url in listOf("http://192.168.1.11:41080/pair", "http://10.0.0.5:52080/pair", "http://tapdeck.local/pair/")) {
            assertEquals(url, pairingAddressFromQr(" \n$url\r\n "))
        }
    }

    @Test fun rejectsUnrelatedQrCodesAndMalformedUrls() {
        for (text in listOf("", "hello", "WIFI:T:WPA;S:Wifi;;", "https://example.com/pair", "http://192.168.1.11:41080/apk",
            "http://192.168.1.11:41080/pair?next=other", "http://192.168.1.11:41080/pair#fragment", "http:///pair",
            "http://user:password@192.168.1.11:41080/pair", "http://192.168.1.11:0/pair", "http://192.168.1.11:65536/pair",
            "http://192.168.1.11:invalid/pair", "http://192.168.1.11:41080/pa ir")) {
            assertNull(text, pairingAddressFromQr(text))
        }
    }
}
