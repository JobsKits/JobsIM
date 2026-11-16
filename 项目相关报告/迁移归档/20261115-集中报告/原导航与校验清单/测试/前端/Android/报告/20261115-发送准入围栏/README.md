# Android 发送准入围栏限定证据

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、当前状态](#status)
- [二、实际实现](#changes)
- [三、实际通过范围](#verified-proof)
- [四、边界](#limits)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本报告只覆盖 Android 原生待发绑定原资格的回归，不代表完整 I／243 项验收。此前 6 JVM＋8 仪器的真实通过基线及原生截图保留在[基础闭环](../20261115-基础闭环/README.md)。

## 一、<span id="status">当前状态</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

当前 debug APK 构建成功，6 JVM＋9 项 API37 ARM64 真实仪器全部通过，0 失败／错误／跳过。最后整组命令成功退出，耗时 33 秒；此前已过的 P0 证据与主题混色诊断图保留在 `主题修复前/`，不替代当前结果。

| 证据 | 位置 |
| --- | --- |
| 当前源码／APK 指纹 | [验证摘要](./验证摘要.json)，包含 37 份被测文件 SHA256、APK SHA256／大小 |
| JVM 6 项 | [实际 XML](./JVM/TEST-com.jobs.im.IsolationTest.xml) |
| 仪器 9 项 | [实际 XML](<./仪器结果/TEST-JobsIMReview_20261114_1424(AVD) - 17-_app-.xml>) |
| 构建和完整回归 | [Gradle 原始日志](./构建测试日志.txt) |
| 原生截图 | [聊天](./原生UI/01-Native-Chat.png)、[群成员](./原生UI/02-Native-Group.png)、[白天](./原生UI/03-Native-Settings.png)、[黑夜](./原生UI/04-Native-Dark-Settings.png)、[跟随系统](./原生UI/05-Native-System-Settings.png) |

逐图检查曾发现黑夜下顶部与导航仍混用浅色默认值；已为这些原生组件显式绑定当前主题语义色，并在当前 APK 重新跑完整 9 项后检查截图。该检查是限定页面的可读性验证，未外推全部设备／语言组合。

## 二、<span id="changes">实际实现</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- 新意图从可信 Conversation 原样保存本人 membershipGeneration／sendAdmission，与原 clientMsgId、原 restoreEpoch 和消息载荷一起先写入 Room，再发消息 HTTP。
- Room schema5：4→5 仅添加原快照字段、旧行保持空值，不用当前资格补旧队列，不换 ID／token 自动重发。
- 每次提交先读取 Room 的原行，必须仍为 pending 且完整快照相同；已冻结后拿旧内存 pending 对象调用也不发送。
- 提交前核对 bootstrap 的强制 send_admission=true；没有此标记时也不对缺快照的旧 ID 发 POST，避免旧服务创建原本未发送的消息。
- 新服务对原 ID 先按原完整 payload 对账；SEND_ADMISSION_CHANGED 将该行永久冻结。当前新资格仅用于用户明确创建的新消息，界面不提供冻结行的重试／编辑动作。

## 三、<span id="verified-proof">实际通过范围</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[AdmissionFenceTest.kt](../../Instrumentation/AdmissionFenceTest.kt) 使用真实 HTTPS、Room 与 Keystore。测试传输层仅模拟消息 HTTP 前离线和移除 bootstrap 能力标记；好友、群成员退出重入、新消息及幂等冲突均经真实后端。场景包含私聊删除好友后新申请／新接受、持久待发后错过群退出重入、ViewModel 重建只见新资格仍坚持原快照、原行冻结、旧回调禁止再发、旧空快照不补最新值、缺标记不 POST、当前新意图成功及同 ID 改 token 冲突。

## 四、<span id="limits">边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

未对旧二进制实际重放：缺能力标记由测试传输层模拟，验证没有消息 POST。旧服务上已经提交的无快照历史 ID，未在 Android 端完成跨升级对账演练；当前新服务的原 ID 幂等／改 token 冲突已真实验证。

复用专用 API37／Android17 ARM64 AVD，不等于真机／公网／断电／完整生命周期／清云或灾备验收。ViewModel 重建与数据库关闭重开不等于操作系统强杀／整进程恢复测试。原媒体上传丢响应的对账、后台远程推送、RTC、地图、支付、产品 SDK、E2EE 未在本报告交付。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
