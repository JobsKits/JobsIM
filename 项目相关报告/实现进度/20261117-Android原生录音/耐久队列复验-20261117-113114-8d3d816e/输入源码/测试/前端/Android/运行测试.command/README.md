# <span id="前言">Android 测试运行器</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

入口为同目录 [运行测试.command](./运行测试.command)。实际模式、设备选择、副作用与证据见[上级测试说明](../README.md)。从任意工作目录传入 `check`、`unit`、`build`、`emulator`、`device`、`live-emulator` 或 `live-device`；默认 `check`，不会隐式安装App。需要原生 zsh、Java、Android SDK与工程 Gradle wrapper。

`live-*` 需要显式传入 `JOBS_IM_TEST_URL`、`JOBS_IM_DEPLOYMENT_ID` 和 `JOBS_IM_CA_FILE`，实际会在隔离服务器创建测试账户／群／报告及设备会话。默认服务参数缺失时会跳过 live 仪器，不应把跳过计作接口通过。宿主回环服务的单目标 adb reverse 与保留测试截图参数见上级说明。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
