# 安装与配对

支持 Windows 11 x64、Android 8 / API 26 及以上。手机与 PC 需处于可互通的局域网。安装包见 [0.3.19 Release](https://github.com/fly3457/TapDeck/releases/tag/v0.3.19)。Android APK 为不可调试的 release 构建，现有官方版可直接覆盖升级，保留配对和手机设置。

## 连接

1. PC 运行 `TapDeck-0.3.19.exe`，打开“连接”页。
2. 手机安装 `TapDeck-0.3.19.apk`。也可在浏览器打开 PC 连接网址，从网页下载 EXE 内置 APK。
3. 手机“连接与设置”扫码填写或手动输入网址，再点击“连接”。
4. 比较手机与 PC 的校验码，在 PC 弹窗选择允许。关闭／Esc 等同拒绝，已有配对不会被覆盖。

扫码只填写网址，不会自动连接；拒绝相机权限时仍能手动输入。已配对电脑可免确认重连。从手机顶部列表添加、切换或管理电脑，详见[多 PC 配对](multi-pc.md)。

## 语音环境

推荐[豆包输入法 Windows 版](https://ime.doubao.com/pc)。TapDeck 传送声音并触发输入法热键，识别文字由输入法完成。

- **虚拟键盘 FakerInput**：首次使用豆包等输入法的语音热键时，在“快捷键 → 键盘环境”点“安装 / 修复虚拟键盘”。它让输入法接收虚拟硬件按键；只用普通控制时可不装，自动模式可使用软件按键。
- **虚拟声卡 VB-CABLE**：需要把手机声音作为 PC 麦克风时，在“语音 → 语音输入环境”安装。按照官方向导重启 Windows；不传音或已有其他音频路由时可不装。
- TapDeck 输出选 **CABLE Input**；系统音频输入或输入法麦克风选 **CABLE Output**。“系统音频输入设置”打开 Windows“声音 → 录制”，程序不自动更改默认麦克风。
- 在“语音快捷键设置”启用所需组，将热键设成输入法的语音快捷键。留空只传音；左右 Ctrl / Shift / Alt 应与输入法一致。
- 点击“保存并同步配置”，让 PC 目标输入框获得焦点，在手机点击“开始”或按住圆形控件说话。首次录音需允许麦克风权限。

感谢 [FakerInput](https://github.com/Ryochan7/FakerInput) 和 [VB-Audio / VB-CABLE](https://vb-audio.com/Cable/)。VB-CABLE 是 donationware，原包许可和官网入口均保留。更多说明见[虚拟键盘](virtual-keyboard.md)、[豆包热键排查](voice-hotkey.md)和[第三方声明](../THIRD_PARTY_NOTICES.md)。

## 设置与更新

PC“连接”“关于”即时处理配对和临时输入测试；“快捷键”“语音”“设置与状态”共用“保存并同步配置”，切换页面保留编辑。手机灵敏度、震动、键盘模式及控件位置保存在本机。

更新时退出旧托盘程序，运行新版 EXE，覆盖安装 APK。当前版本为接收端 **0.3.18**、Android **0.3.18 / code 21**，包名和签名保持；旧配对、配置及每台 PC 的语音选择保留。开启自启后移动 EXE 或更换版本文件名，请重新登记路径，见[自动连接](auto-connect.md)。

PC“连接”页可按设备 ID 单独撤销授权，手机会保留电脑并提示重新配对。手机“忘记”只移除本机记录。迁移与写入失败保留原数据，详见[多 PC](multi-pc.md)和[控制协议](../protocol/README.md)。

## 交付与验收

Windows 主程序、诊断版、HID 工具和 APK 使用带版本号的文件名。EXE 内嵌本次构建的 APK 与原版驱动安装包；诊断版支持 `--version`、`--apk-info`、`--keyboard-status`、`--cable-status`。

构建和发布按[版本规则](versioning.md)执行。当前测试、截图、包哈希与实机待验收项见[验证记录](verification.md)。
