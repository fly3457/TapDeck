# 一台 PC 连接多个控制端

一台 PC 最多同时连接 **五个 Android 控制端**。每台设备独立配对，PC 核对后允许；已有凭据免确认重连。同一设备重连替换旧会话，不重复占名额。

手机配对多台 PC 是另一种用法：每次只控制一台，见[多 PC 切换](multi-pc.md)。

## 共享行为

- 控制端共享鼠标和键盘，按收到的顺序执行；同时移动鼠标会相互影响。
- 配置保存后同步给所有在线控制端。
- 持有键、鼠标按钮和排队动作按会话归属。单台断线或解绑只清理它的输入，不释放其他设备的持有。
- 语音是单路录音，不混音；另一台需等待当前传音结束。
- 满五台时，新的请求收到 `too_many_clients`；断开一台后可继续接入。

PC“连接”页按设备 ID 区分同名设备，显示在线状态和最近连接时间，可单独或全部解除配对。失败的持久化不会撤销原授权。

## 开发验证

在 `windows` 目录运行模拟控制端：

```powershell
go run ./cmd/ctrlprobe -name Probe-A -seconds 60
go run ./cmd/ctrlprobe -seconds 0
```

它使用独立设备标识，首次仍需 PC 允许。协议与会话隔离见[控制协议](../protocol/README.md)，当前结果见[验证记录](verification.md)。下方保留旧设备管理阶段的实测。

<details>
<summary>旧设备管理版本的多控制端实测</summary>

## 4. 历史实测（设备管理更新前）

| 项目 | 结果 |
|---|---|
| 单台连接 | 手机（Sony XQ-DQ72）保持连接，状态行「已连接：Sony XQ-DQ72」 |
| 5 台同时在线 | 平板 + 4 个 `ctrlprobe`（各自不同设备编号、各自配对）→ 状态行「已连接 5 个控制端：Probe-1、Probe-2、Probe-3、Probe-4、Sony XQ-DQ72」，每个 `ctrlprobe` 各自拿到会话编号（如 `adccb393f8fe28b8`） |
| 第 6 台 | 立刻收到 `too_many_clients`：`被拒绝：接收端最多同时连接 5 个控制端，请先断开其中一个` |
| 释放名额 | 关闭其中一个会话后，新的控制端可以再配对接入 |
| 配对只需 PC 确认 | `ctrlprobe` 只发 `hello`、不发任何确认，PC 点「校验码一致，允许」后直接打印 `已建立会话：e3d00800147c89ad`；同机状态行变为「已连接 2 个控制端：PairLive、Sony XQ-DQ72」 |
| 配置广播 | `TestConfigUpdateReachesEverySession`：两个会话都收到 `config` |
| 路由 | `TestPacketsRouteToOwningSession`：未知会话 id 不匹配，两个 id 各自命中自己的会话 |
| 单元测试 | `TestMultipleControllersUpToLimit`、`TestSameDeviceReplacesItsSession`、`TestPairWithoutClientConfirmation` 等全部通过 |

</details>
