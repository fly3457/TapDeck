package com.yuncii.tapdeck

import java.net.URI

/** PC 二维码只负责填写配对网址，连接与授权仍由原有配对流程处理。 */
internal fun pairingAddressFromQr(contents: String): String? {
    val text = contents.trim()
    val uri = runCatching { URI(text) }.getOrNull() ?: return null
    if (uri.scheme != "http" || uri.host.isNullOrEmpty() || uri.userInfo != null) return null
    if (uri.port != -1 && uri.port !in 1024..65535) return null
    if (uri.path?.trimEnd('/') != "/pair" || uri.rawQuery != null || uri.rawFragment != null) return null
    return text
}
