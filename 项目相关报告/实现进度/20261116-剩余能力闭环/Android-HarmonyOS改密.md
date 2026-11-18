# JobsIM · Android 与鸿蒙本人主动改密

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、入口与操作合同](#jobs-toc-heading-2)
- [二、实际实现位置](#jobs-toc-heading-3)
- [三、实际验证与限制](#jobs-toc-heading-4)
- [四、后续衔接](#jobs-toc-heading-5)
- [五、能力撤回追加回归](#jobs-toc-heading-6)
- [六、明确前置拒绝码回归](#jobs-toc-heading-7)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

2026-11-17 落地[本人主动改密协议](../../../代码/后端/Go/接口契约/README.md#own-password-change)的两端原生入口。这里交付实际页面、原责任服务、持久化与旧家族围栏；设备、真实 HTTPS 与全项目剩余能力分别验收，不能把本切片写成全部 I 完成。前一轮回归中的“两个原生端没有改密接线”属于当时范围，本报告记录当前源码与独立证据。

冻结源码、构建日志、测试 XML／原始输出、独立安装包及 SHA256 见[改密证据目录](./Android-HarmonyOS改密证据/)。旧 Dist 与原回归报告不覆盖，没有安装工具、清理环境、使用实际用户账号或提交／推送。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、入口与操作合同 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 项目 | 当前行为 |
| --- | --- |
| 原生入口 | 两端“设置→修改本人密码”。设置在未登录时也可进入，继续查看本机旧任务并重输原密码对核对结果；新建必须当前本人已登录且服务端 `own_password_change=true` |
| 证明与确认 | 当前密码、新密码、确认新密码及管理员可选验证器代码。密码按 NFC 规范化，新密码15～128码点／最多512字节，不得复用当前密码；明确确认全设备须重新登录后才开始 |
| 创建 | 先以现有认证协调器核当前上下文，允许同family正常AT过期刷新；再固定当前实例、本人、设备、family、epoch与nonce，保存非秘密原责任，匿名查原结果；明确原单404才使用匹配原family的AT创建 |
| 未确认 | 第一次创建POST前已持久attempt／unknown并暂停旧family普通业务。网络、解析、503或错误证明保留原请求；页面／重启不自动重发或换nonce |
| 核对／明确重试 | 查单永远不带AT或OTP，使用原用户名／设备／epoch／nonce及本人重输的原规范化密码对。只有明确点击重试才重投原单；已有pending责任以匿名原证明续作，不借后来登录的family或AT |
| 完成 | 只保存白名单公开回执及字符串凭据／安全代际，不接收新登录凭据。仅清除仍匹配原family的本机安全凭据与会话视图；新登录的其他family不被旧结果退出。changed旧family在冷启动仍被阻止，用户使用新密码重新登录 |
| 界面与生命周期 | 22个改密标签完整十语；沿用白天／黑夜／跟随系统、字号及原生控件。任务20项逐页、空态与重载。输入不使用可恢复页面状态，确认调用后清表单，离开页面／后台清敏感输入；迟到回包核原身份／页面生命周期 |

原密码对与OTP只在临时RAM中组装HTTP请求；普通库不存原密码、替换密码、OTP、AT／RT、原密码请求体或可离线猜测的密码／请求摘要。可持久的“原body责任”是原非秘密字段和nonce，恢复时由本人重输原密码对重建证明；不会把后来状态补入旧请求。平台运行时的不可变字符串由正常内存管理回收，Android额外清空本轮字符数组，不宣称能清零全部系统／输入法副本。

首次且从未重试的创建POST遇明确400／401／403／429才把原任务标为rejected并解除该未提交门禁；原unknown查单403不能据此解除暂停或创建新请求。原消息Outbox与历史不会被复制、重写或自动迁移发送。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、实际实现位置 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 端 | 源码与持久化 |
| --- | --- |
| [**Android**](https://developer.android.com/) | [OwnPassword](../../../代码/前端/Android/app/src/main/java/com/jobs/im/OwnPassword.kt)负责输入／结果校验、Room `sync_records` 原责任CAS、匿名查询与明确重试；[原生页](../../../代码/前端/Android/app/src/main/java/com/jobs/im/OwnPasswordScreen.kt)使用Compose Dialog／安全密码键盘；[十语](../../../代码/前端/Android/app/src/main/java/com/jobs/im/OwnPasswordText.kt)。`ChatModel`的普通API／轮询及SessionCoordinator捕获owner包含改密暂停，restore先读原责任；changed旧family不恢复成普通登录 |
| [**HarmonyOS**](https://developer.huawei.com/consumer/cn/) | [PasswordChange](../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/PasswordChange.ets)是平台中立协议服务；[PasswordStore](../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/PasswordStore.ets)在原RDB内保存非秘密责任，原责任与auth scope的family暂停marker在同一事务提交；[PasswordText](../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/PasswordText.ets)及`Index.passwordPage`接原生页面。`AuthCoordinator.before／refresh／restore`检查持久marker，旧晚到业务结果也因stamp变化被拒绝 |

两端信任仍由已有实例档案与CA策略承接，不改变系统CA、全局代理或正式认证合同；改密原结果不通过普通会自动刷新／重新认证的业务客户端发送。第一次准备与后续查询不互相借用新family。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、实际验证与限制 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 验证 | 实际范围 |
| --- | --- |
| Android JVM | 20项实际通过、0失败／错误／跳过；其中4项新增覆盖NFC／码点／边界、禁止复用、unknown／pending／changed旧family暂停、诊断脱敏与十语完整性 |
| Android应用／测试包 | 当前应用及测试APK真实构建；[新增4项Room仪器源码](../../../测试/前端/Android/Instrumentation/OwnPasswordLifecycleTest.kt)验证原责任重建／秘密不落库、只读查单、错误证明不解除暂停、不借新family及迟到身份，当前仅编译，未设备执行 |
| 鸿蒙协议／仓库 | [14项主机反例](../../../测试/前端/HarmonyOS/Unit/RunPassword.mjs/README.md)实际通过，含实际PasswordStore转译代码＋系统SQLite适配的CAS／重开／原责任与暂停marker同事务；RCP为注入，不是RDB NAPI或真实HTTPS |
| 鸿蒙认证 | 当前43项实际主机通过，新增两项专门验证unknown暂停普通AT／refresh、冷启动不发业务context、不删除原证明与后来独立family不受旧marker影响 |
| 鸿蒙原生编译 | 当前应用及46方法测试HAP真实编译，无签名；新增4个Hypium方法名进入测试字节码。未执行这些设备测试，不借旧42方法HAP算作当前通过 |
| 输入与产物核对 | 当前源摘要、原生暂存输入、两APK／两HAP摘要与ZIP CRC分别核验；旧e295与鸿蒙3db7／d8c7 Dist保留 |

在仓库根运行主机改密与认证回归：

```shell
node 测试/前端/HarmonyOS/Unit/RunPassword.mjs/RunPassword.mjs
node 测试/前端/HarmonyOS/Unit/RunAuth.mjs
```

Android使用既有JBR／SDK，离线单worker运行 `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest`；鸿蒙复用既有镜像SDK和原生构建入口的ASCII快照流程，单独构建default／ohosTest，跳过覆盖Dist。每次构建／执行写独立证据子项，失败与中间结果保留。低于4GiB暂停新增重型验证，源码工作继续；本轮未清理用户环境。

当前ADB无目标、HDC为`[Empty]`。实际设备的安全库、原生RDB、输入与系统生命周期、旧AT已过期时的真实刷新、管理员TOTP、服务端全family撤销后原生重新登录、断网及双集群恢复仍需真实链路验证。本轮没有用纯主机／编译结果冒充这些设备结果。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">四、后续衔接 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

改密源码及限定验证完成后继续账号注销／受限数据权利查询取消、完整群治理和普通端迁移；它们分别等待完整后端合同与端侧接线，不能用本改密入口或恢复密码替代。合格ACK／累计目标与清云继续保持服务端保护门禁。五端资产表由主代理统一更新，避免并发覆盖。



## <span id="jobs-toc-heading-6" data-jobs-toc="v2">五、能力撤回追加回归 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

交叉复核确认：旧原单 pending 或结果404时，原代码会在能力撤回／恢复代际变化后继续匿名 mutation；结果404的旧原单也可能在后继家族下匿名创建。现两端统一在全部 mutation 前要求当前能力与原代际匹配，404创建必须持原本人／设备／namespace／family的 AT。匿名纯查询继续可用。

鸿蒙新增实际服务反例覆盖能力撤回／epoch改变 × pending／404，15项主机通过；Android新增第5个 Room instrumentation 反例已编译。Android20项 JVM 仍通过，应用／测试 APK、鸿蒙应用／46方法测试 HAP 已重新编译；设备仍未执行。原冻结与四个旧包完整保留，新候选的源码摘要、CRC、四包SHA与原始日志见[追加冻结](./Android-HarmonyOS改密证据/能力撤回修复/冻结结果.json)。


## <span id="jobs-toc-heading-7" data-jobs-toc="v2">六、明确前置拒绝码回归 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

再次交叉复核修正400／401／403／429的泛化：仅fresh首次提交的400 INVALID_PASSWORD_CHANGE、403 PASSWORD_CHANGE_REJECTED、429 RATE_LIMITED可判未建，prepared后依赖403继续unknown。Harmony增加生产service反例后16项host通过；Android增加第5项JVM与第6项Room方法，后者仍只编译。旧Room测试专用DB名不满足现行JobsIM-test-约束，已修正但未称设备通过。新候选与注销同包验证，完整25 JVM／16改密host／45认证host及应用、测试包冻结见[两端注销报告](./Android-HarmonyOS注销.md)，旧四包冻结不冒充当前源码。

<a id="🔚" href="#前言">返回前言</a>

2026-11-17真实Android补证：Android当前6个OwnPasswordLifecycle原生方法全部实际通过，接口为受控注入，不能冒称主动改密真实HTTPS／管理员TOTP；Harmony改密16host按原冻结范围成立。同一当前9660aa38／7b3f14b4双包的[105逐方法记录](./普通迁移两端证据/Android独立AVD-20261117-153544-d52437/最终105逐方法实际核对.json)列出86本地通过／16明确跳过及3独立S2 HTTPS；当前源未借旧包累加计数，私有5580已停并保数据。
