package com.yuncii.tapdeck

import androidx.compose.foundation.layout.*
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.*
import androidx.compose.runtime.*
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

@Composable
internal fun PeerManager(
    catalog: PeerCatalog,
    state: ClientState,
    rename: (String, String) -> Unit,
    forget: (String) -> Unit,
    close: () -> Unit,
) {
    var editing by remember { mutableStateOf<String?>(null) }
    var removing by remember { mutableStateOf<String?>(null) }
    var alias by remember { mutableStateOf("") }
    AlertDialog(onDismissRequest = close, title = { Text("管理电脑") }, text = {
        Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (catalog.peers.isEmpty()) Text("还没有已配对电脑，可从顶部列表添加。")
            catalog.peers.sortedBy { if (it.id == state.selectedPeerId) 0 else 1 }.forEach { pc ->
                Column {
                    Text(pc.displayName, style = MaterialTheme.typography.titleMedium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    if (pc.alias.isNotEmpty()) Text(pc.peer.name, style = MaterialTheme.typography.bodySmall)
                    Text(pc.address, style = MaterialTheme.typography.bodySmall)
                    Text(when {
                        pc.id == state.selectedPeerId -> "当前电脑 · ${state.status}"
                        pc.needsPairing -> "需重新配对"
                        else -> "已配对"
                    }, style = MaterialTheme.typography.bodySmall)
                    Row {
                        TextButton(onClick = { alias = pc.alias; editing = pc.id },
                            modifier = Modifier.semantics { contentDescription = "修改${pc.displayName}备注" }) { Text("备注名") }
                        TextButton(enabled = state.canSwitch || pc.id != state.selectedPeerId, onClick = { removing = pc.id },
                            modifier = Modifier.semantics { contentDescription = "忘记${pc.displayName}" }) { Text("忘记") }
                    }
                    HorizontalDivider()
                }
            }
            if (!state.canSwitch) Text("请先结束语音输入，再切换或忘记当前电脑。")
            if (state.error.isNotEmpty()) Text(state.error, color = MaterialTheme.colorScheme.error)
        }
    }, confirmButton = { TextButton(onClick = close) { Text("完成") } })
    catalog.find(editing)?.let { pc ->
        AlertDialog(onDismissRequest = { editing = null }, title = { Text("电脑备注名") }, text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(pc.peer.name + " · " + pc.address)
                OutlinedTextField(value = alias, onValueChange = { alias = it }, singleLine = true,
                    label = { Text("备注名") }, supportingText = { Text("留空恢复电脑系统名称") })
            }
        }, confirmButton = { TextButton(onClick = { rename(pc.id, alias); editing = null }) { Text("保存") } },
            dismissButton = { TextButton(onClick = { editing = null }) { Text("取消") } })
    }
    catalog.find(removing)?.let { pc ->
        AlertDialog(onDismissRequest = { removing = null }, title = { Text("忘记电脑") }, text = {
            Text("忘记“${pc.displayName}”（${pc.address}）的手机配对记录？再次连接需要在电脑上允许。")
        }, confirmButton = { TextButton(enabled = state.canSwitch || pc.id != state.selectedPeerId,
            onClick = { forget(pc.id); removing = null }) { Text("忘记这台电脑") } },
            dismissButton = { TextButton(onClick = { removing = null }) { Text("取消") } })
    }
}
