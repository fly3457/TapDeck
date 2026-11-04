package com.yuncii.tapdeck

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.*
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Popup
import androidx.compose.ui.window.PopupProperties
import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale

/** 今天的通知只显示时间，更早的带上日期。 */
internal fun notificationTime(time: Long, now: Long = System.currentTimeMillis()): String {
    if (time <= 0) return ""
    val day = SimpleDateFormat("yyyyMMdd", Locale.getDefault())
    val pattern = if (day.format(Date(time)) == day.format(Date(now))) "HH:mm" else "M月d日 HH:mm"
    return SimpleDateFormat(pattern, Locale.getDefault()).format(Date(time))
}

/** 电脑通知列表：按时间倒序，可清空。 */
@Composable internal fun NotificationList(items: List<PcNotification>, onClear: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(onDismissRequest = onDismiss, title = { Text("电脑通知") }, text = {
        if (items.isEmpty()) Text("还没有收到电脑通知。\n电脑上的 Windows 系统通知会转发到这里。", style = MaterialTheme.typography.bodyMedium)
        else LazyColumn(Modifier.heightIn(max = 460.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            items(items, key = { it.key }) { n ->
                Column(Modifier.fillMaxWidth()) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Text(n.heading, Modifier.weight(1f), fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        Text(notificationTime(n.time), style = MaterialTheme.typography.bodySmall, color = Color(0xFF667085))
                    }
                    if (n.body.isNotBlank()) Text(n.body, style = MaterialTheme.typography.bodyMedium, maxLines = 6, overflow = TextOverflow.Ellipsis)
                    if (n.peerName.isNotBlank()) Text(n.peerName, style = MaterialTheme.typography.bodySmall, color = Color(0xFF667085))
                }
            }
        }
    }, confirmButton = { TextButton(onClick = onDismiss) { Text("关闭") } },
        dismissButton = { if (items.isNotEmpty()) TextButton(onClick = onClear) { Text("清空") } })
}

/** 前台收到新通知时在顶部短暂显示；点击打开列表，不拦截横幅以外的触摸。 */
@Composable internal fun NotificationBanner(n: PcNotification, onOpen: () -> Unit) {
    Popup(alignment = Alignment.TopCenter, properties = PopupProperties(focusable = false)) {
        Column(
            Modifier.statusBarsPadding().padding(horizontal = 12.dp, vertical = 8.dp).fillMaxWidth()
                .shadow(6.dp, RoundedCornerShape(12.dp)).clip(RoundedCornerShape(12.dp))
                .background(Color.White).clickable(role = Role.Button, onClick = onOpen)
                .padding(horizontal = 14.dp, vertical = 10.dp),
        ) {
            Text(n.heading, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
            if (n.body.isNotBlank()) Text(n.body, style = MaterialTheme.typography.bodyMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
        }
    }
}
