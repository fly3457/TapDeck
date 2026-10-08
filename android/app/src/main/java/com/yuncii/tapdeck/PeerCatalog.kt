package com.yuncii.tapdeck

import kotlinx.serialization.Serializable

/** The certificate identity, not an address or a display name, owns a pairing. */
@Serializable
data class SavedPeer(
    val peer: Peer,
    val alias: String = "",
    val voiceProfileId: String? = null,
    val legacyVoiceMode: String? = null,
) {
    val id: String get() = peer.id
    val displayName: String get() = alias.ifBlank { peer.name.ifBlank { "电脑" } }
    val needsPairing: Boolean get() = peer.token.isEmpty()
    val address: String get() = "${peer.host}:${peer.httpPort}"
}

@Serializable
data class PeerCatalog(
    val schemaVersion: Int = 1,
    val peers: List<SavedPeer> = emptyList(),
    val selectedId: String? = null,
) {
    fun find(id: String?) = peers.firstOrNull { it.id == id }
    val selected: SavedPeer? get() = find(selectedId)
    fun ordered() = peers.sortedBy { if (it.id == selectedId) 0 else 1 }

    fun validated(): PeerCatalog {
        require(schemaVersion == 1) { "不支持的电脑列表版本" }
        require(peers.all { it.peer.pin.matches(Regex("[0-9a-fA-F]{64}")) }) { "电脑指纹无效" }
        require(peers.all { it.peer.host.isNotBlank() && it.peer.httpPort in 1024..65535 && it.peer.wssPort in 1024..65535 }) { "电脑地址或端口无效" }
        require(peers.map { it.id }.distinct().size == peers.size) { "电脑身份重复" }
        require(selectedId == null || selected != null) { "选中的电脑不存在" }
        return this
    }

    fun select(id: String?): PeerCatalog {
        require(id == null || find(id) != null) { "电脑已不在列表中" }
        return copy(selectedId = id)
    }

    fun upsert(peer: Peer): PeerCatalog {
        val old = find(peer.id)
        val saved = (old ?: SavedPeer(peer)).copy(peer = peer.copy(pin = peer.id))
        return copy(peers = if (old == null) peers + saved else peers.map { if (it.id == saved.id) saved else it })
    }

    fun rename(id: String, alias: String) = copy(peers = peers.map { if (it.id == id) it.copy(alias = alias.trim()) else it })
    fun voice(id: String, profile: String) = copy(peers = peers.map {
        if (it.id == id) it.copy(voiceProfileId = profile, legacyVoiceMode = null) else it
    })
    fun remove(id: String) = copy(peers = peers.filterNot { it.id == id }, selectedId = selectedId?.takeUnless { it == id })
    fun revoke(credential: Peer) = copy(peers = peers.map {
        if (it.peer.sameCredential(credential)) it.copy(peer = it.peer.copy(token = "")) else it
    })

    companion object {
        fun migrate(peer: Peer?, voiceId: String?, legacyMode: String?): PeerCatalog = if (peer == null) PeerCatalog() else
            PeerCatalog(peers = listOf(SavedPeer(peer.copy(pin = peer.id), voiceProfileId = voiceId, legacyVoiceMode = legacyMode)), selectedId = peer.id).validated()
    }
}
