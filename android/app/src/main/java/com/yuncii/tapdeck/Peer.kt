package com.yuncii.tapdeck

import kotlinx.serialization.Serializable

@Serializable
data class Peer(val host: String, val wssPort: Int, val httpPort: Int, val pin: String, val name: String = "电脑", val token: String = "") {
    // TLS identity owns the credential, so a LAN address or port change can
    // still reconnect to the same computer after validating its certificate.
    fun withCredentialFrom(saved: Peer?): Peer =
        if (saved != null && pin.equals(saved.pin, true)) copy(token = saved.token) else this

    fun sameCredential(other: Peer): Boolean = pin.equals(other.pin, true) && token == other.token
}
