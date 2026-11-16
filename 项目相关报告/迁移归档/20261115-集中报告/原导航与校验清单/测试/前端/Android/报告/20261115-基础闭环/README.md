# Android 原生基础闭环限定证据

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、真实结果](#result)
- [二、实际覆盖](#coverage)
- [三、原生截图](#screenshots)
- [四、边界](#limits)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

2026-11-15，复用已有专用 API37／Android17 ARM64 AVD `JobsIMReview_20261114_1424`（`emulator-5582`），未下载新镜像、未改系统 CA／全局代理。此记录保留发送准入快照围栏接入前的真实通过基线；后续源码／APK不能仅凭本报告宣称通过。完整 I 和 243 项仍未验收。

## 一、<span id="result">真实结果</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 项目 | 结果与证据 |
| --- | --- |
| APK | debug 构建成功；所测 SHA256／大小见 [验证摘要](./验证摘要.json)，最终 Dist 可能被后续围栏构建替换 |
| JVM | 6 项，0 失败／错误／跳过，[原始 XML](./JVM/TEST-com.jobs.im.IsolationTest.xml) |
| 仪器 | 8 项，0 失败／错误／跳过，[实际 XML](<./仪器结果/TEST-JobsIMReview_20261114_1424(AVD) - 17-_app-.xml>) |
| 原生 UI | 真实聊天、群成员／转让控件、设置；白天／黑夜／跟随系统各有截图，共 6 张含清理前诊断图 |
| 命令与版本 | [构建测试日志](./构建测试日志.txt)，[绑定源码／APK 指纹](./验证摘要.json) |

构建命令为工程 wrapper 的 `--offline --no-daemon :app:assembleDebug :app:testDebugUnitTest :app:connectedDebugAndroidTest`，明确指定专用设备和隔离 HTTPS 的 liveUrl／deploymentId／公开 CA；保留本测试包以提取原生截图。所有测试源码在工程外，由 sourceSets 引用，没有复制业务逻辑替代生产代码。

## 二、<span id="coverage">实际覆盖</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- Room：真实 bundled SQLite 与写连接 FULL，正式／演示隔离；schema2→4 非破坏迁移、原待发 payload／ID／epoch 保留；游标／消息事务回滚、超大十进制字符串、回执对账、草稿修订、未读去重／归零、数据库重开；匿名重新连接冻结同档案旧 epoch 的全部账号待发。
- 原生 ViewModel＋真实 HTTPS：本人入站申请与当前 requester、接受；本人关系 GET 与删除／拉黑／解黑／显式 reopen、新申请／新接受；群改名／转让／退群、旧 membershipGeneration 在重入后被拒绝，只读群不落新消息。
- 消息／头像／举报：在真实服务端已提交后由测试传输层丢弃响应，客户端保留原待发／原 requestId／原文件再按原操作重试；服务端只产生一项原消息／报告。旧头像结果 current=false 后重新 GET /me，不覆盖当前头像。
- 状态竞态：轮询与打开会话交错 4 轮，可见 key 唯一且不串会话；实际发送回执返回期间输入的新草稿保留；后台完成加载不清未读，回前台刷新后归零。
- 附件／账号：当前鉴权下载真实文件并核对字节／MIME、设备撤销拒旧会话、ViewModel 重建时 Keystore 密封会话复核恢复；常规编辑／反应／置顶／收藏／撤回保留原 LiveApiTest 覆盖。
- 通知／UI：实际系统通知仅当前命名空间，固定通用文字、无会话标题／消息正文；原生聊天、成员、设置及明暗／系统主题可达，已检查所保存截图的可读性。十语键完整由 JVM 检查，不等于翻译逐项人工验收。

## 三、<span id="screenshots">原生截图</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[聊天](./原生UI/01-Native-Chat.png) · [群成员](./原生UI/02-Native-Group.png) · [白天设置](./原生UI/03-Native-Settings.png) · [黑夜设置](./原生UI/04-Native-Dark-Settings.png) · [跟随系统](./原生UI/05-Native-System-Settings.png)

## 四、<span id="limits">边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

没有真机／公网可达、全部语言字号组合、真实系统 CA 选择器／头像选择器／Save As 提供者全流程、断电／灾备／清云／合格设备 ACK、完整账号删除和派生源生命周期验收；系统应用负责媒体打开，不是内置媒体播放器。原媒体上传丢响应尚无对账契约；轮询及本地通知依赖本应用进程存活，不是远程推送。发送准入快照的退出重入围栏由后续报告独立记录，不把本次通过当作该问题已修复。

原需求及早先证据保留；未提交、推送或签名发布。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
