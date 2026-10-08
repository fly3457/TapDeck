# 全键盘

点击手机顶部键盘图标，将底部快捷键与语音区切换成全键盘，再点恢复。模式保存在本机；长按空格使用当前语音配置，松手结束。

## 键位

下方主键位短按输入，上方灰色副键位长按输入。

| 行 | 主键位 | 副键位 |
|---|---|---|
| 1 | `q w e r t y u i o p` | `1 2 3 4 5 6 7 8 9 0` |
| 2 | `a s d f g h j k l` | `- / : ; ( ) ~ ' "` |
| 3 | Shift、`z x c v b n m`、退格 | Shift、`@ - # & ? ! …`、退格 |
| 4 | Ctrl、`.`、空格、Shift+Enter、Enter | Ctrl、`,`、语音、Shift+Enter、Enter |

- 字母、数字和符号：短按主键一次，长按 400 ms 后只输入副键一次。
- Shift：短按单次大写，长按锁定，再短按解除。
- Ctrl、退格、Enter、Shift+Enter：短按一次，长按持续按住；Ctrl 可搭配另一手指的按键。
- 空格：短按空格，长按语音。语音组未启用时不开始录音。
- 切换电脑、断线、后台或退出键盘时释放持有键与修饰键。

## 间隙与尺寸

0.3.18 起，**1% 键间隙只用于视觉分隔**。左右相邻键各占间隙一半，行间隙由上下行平分；中线归右侧／下方。一次触摸只命中一个键，长按与多指行为相同。四周留白和第二行两侧的居中留白保持。

| 键面 | 视口宽度占比 |
|---|---:|
| 普通键 | 8.9% |
| Shift / 退格 / Shift+Enter | 13.85% |
| Ctrl / Enter | 18.8% |
| 空格 | 33.65% |
| 横向视觉间隙及两侧留白 | 1% |

第二行九键居中；其余行铺满两侧留白之间的区域。四行共享底部 60% 视口宽度的内容高度，上下留白由原生父容器提供。短窗口只缩小纵向尺寸，视觉间隙、键面样式和字号规则见[Android 布局](android-ui.md)。

`KeyboardGeometry.measure` 计算键面，`touchBounds` 将相邻触控范围延伸到同一中线；外层处理手势，内层绘制原尺寸键面。累计边界取整，避免像素误差留下空隙或重叠。

## 输入范围与验证

中文由 PC 输入法处理。上档符号通过 Shift 组合发送，效果取决于 PC 键盘布局；`…` 使用 Unicode 软件发送，强制 HID 模式不支持，建议“自动”。协议仍为 v2，键按下／释放按会话归属，见[特殊按键](special-keys.md)和[控制协议](../protocol/README.md)。

JVM 覆盖几何边界、取整和极小尺寸；设备测试覆盖全部内部间隙、精确中线、长按副键、多指 Ctrl+C、断线清理以及五种布局。当前结果与截图见[验证记录](verification.md)。

<details>
<summary>原型全键盘真机验证（旧版尺寸）</summary>

## 历史实测（0.3.1 样式调整以前，ONYX Tab8C / Android 11）

用 `cmd/keywatch`（轮询 `GetAsyncKeyState`，与焦点窗口和输入法无关）观察 PC 实际收到的按键：

| 项目 | 结果 |
|---|---|
| 区域高度 | 关闭键盘：状态区 + 触控板 + 快捷键区 + 语音区占满 1404×1872；打开键盘：同一块下半区换成键盘，底部不再留空 |
| 键宽（截图逐像素量取，屏宽 1404） | 常规键 118 px＝8.4%、间隙 21 px＝1.5%、`[Shift]`/`[Backspace]` 188 px＝13.4%、`[alt]`/`[Enter]` 258 px＝18.4%、`[Shift+Enter]` 189 px＝13.5%、`[空格]` 469 px＝33.4%；1/3/4 行右边缘都在 x≈1390（两侧各留 10 px）；2 行 9 键居中排布、两侧各留 ≈72 px |
| 字母 | 点 2 行 `k` → `K` 一次；按住 `k` 1.4 s → **只出现 `Quote`（`'`）一次**，没有先冒 `K`、也没有连发 |
| 空格 | 轻点 → `Space` 一次（49–55 ms）；按住 1.6 s → 不出现空格，PC 端 `RightAlt`（本机语音长按热键）被按住 1.5 s 后随松手释放，同时空格键变蓝 |
| Shift 长按锁定 | 按住 Shift（1.1 s）→ `LeftShift` 一直保持按下；随后按 `a` 只发 `LeftShift+A`，字母抬起后 `LeftShift` 不释放；再短按 Shift → `LeftShift` 释放；锁定时键面为蓝色 |
| 退格 | 轻点 → `Backspace` 按下 / 抬起 51 ms；按住 1.2 s → `Backspace` 一直保持按下，松手释放 |
| 回车 | 轻点 → `Enter` 56 ms |
| `Shift+Enter` | 轻点 9 ms；按住 1.1 s 期间 `Shift` 与 `Enter` 一直保持按下 |
| 键面 | 常规键白底、`[Shift]`/`[Backspace]`/`[alt]`/`[Shift+Enter]`/`[Enter]` 为 `#CCC` 灰底；2 行整行居中；副键位与主键位都居中、字号加大；按下与传音时整键变蓝 |
| 单元测试 | `go test ./...` 含 `TestKeyboardKeyStatesAreForwarded`、`TestUnicodeOnlyKeyFallsBackToSendInput`、`TestPunctuationKeysUseHID`、`TestUnicodeOnlyKey`；`gradlew testDebugUnitTest` 含 `KeyHoldTest` 8 项（短按主键位一次、长按副键位一次、Shift 单次与锁定、长按键按住） |

上表全部为真机（ONYX Tab8C / Android 11 + `dist/TapDeck.exe`）实测。

</details>
