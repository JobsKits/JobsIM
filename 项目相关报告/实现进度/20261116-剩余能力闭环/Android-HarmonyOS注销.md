# JobsIM Android／HarmonyOS 本人注销闭环

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、源码已实现](#jobs-toc-heading-2)
- [二、实际已验](#jobs-toc-heading-3)
- [三、输出包与复现](#jobs-toc-heading-4)
- [四、待核边界](#jobs-toc-heading-5)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

2026-11-17 两端设置现有可用的本人注销入口，登录时发起、退出后继续核原结果／受限查询／冷静期取消。Android25项JVM、鸿蒙注销12项主机、改密16项主机、认证45项主机全部通过；应用／测试APK、应用／50方法测试HAP均真实编译。没有连接Android或鸿蒙设备，没有执行Room instrumentation／Hypium，也未借host适配称设备通过。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、源码已实现 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

- Android设置→[OwnDeletionScreen](../../../代码/前端/Android/app/src/main/java/com/jobs/im/OwnDeletionScreen.kt)，鸿蒙设置→[Index.deletionPage](../../../代码/前端/HarmonyOS/entry/src/main/ets/pages/Index.ets)，均保留三主题、十语、明确首次／取消确认、空态／重载、20行分页及绝对冷静截止。不是演示注销；当前实例支持且当前本人登录后才能首次提交。
- [Android协议与责任](../../../代码/前端/Android/app/src/main/java/com/jobs/im/OwnDeletion.kt)／[鸿蒙协议](../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/Deletion.ets)先持久非秘密原nonce、profile／origin／deployment／user／device／family／epoch／namespace，匿名查原体，只有用户明确重试且当前能力／原epoch仍在才续跑；查404首次创建必须原家族AT，后继家族不能代发。密码、OTP、恢复码与5分钟proof仅在内存，不写Room／RDB、普通日志或无钥摘要。
- 发送前先保存unknown并封普通API／RT；冷启动先读责任，原family未知时只留原证明核对通道。已确认请求撤本地原family，迟到响应不清后来登录者。后台／离页清空输入与敏感操作上下文；最终状态或取消不会复活旧family，正常使用重新登录。
- 原结果、purpose=query证明及purpose=cancel证明相互区分。取消nonce在mutation前耐久保存，未知时保留同一nonce，先查原申请的cancelRequestId；取消确认冻结所见requestVersion，不悄悄刷新重基。结果强校原申请／deletionId／epoch及十进制版本单调性，拒绝替换身份。
- [Android本地完成清理](../../../代码/前端/Android/app/src/main/java/com/jobs/im/OwnDeletionCleanup.kt)及[鸿蒙RDB清理](../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/DeletionStore.ets)仅在服务器completed后按原profile／deployment／user前缀清本人全部epoch的消息／待发／草稿／sync视图／快照／操作数据，保公共注销责任及原family暂停标记。先清有归属索引的精确附件、头像原件／缓存与导出暂存，再删索引；失败可在重启／再查询时沿completed责任重做，其他账号空间保留。
- 鸿蒙导出暂存新增原namespace子目录，头像缓存逐文件登记本人归属；文件删除只接受受控应用目录的原UUID路径。外部系统保存的ZIP与用户原选文件不会由本地清理擅自删除。服务端范围见[注销后端报告](./注销后端.md)。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、实际已验 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 层级 | 当前执行结果 | 不代替的验证 |
| --- | --- | --- |
| Android JVM | 25项，0失败／错误／跳过；含注销4项、改密5项 | 不执行Android UI／Room原生事务 |
| Android原生编译 | app APK与test APK通过；新增注销5个Room方法、改密6个Room方法均在DEX核到 | instrumentation未运行 |
| Harmony注销host | 12项真实生产service＋现行仓库SQLite桥接通过，含秘密不落库、pause marker同事务、CAS／重开、仅本人空间清理 | RDB为系统SQLite适配，文件清理port是测试适配 |
| Harmony认证host | 45项通过，含注销marker跨冷启动阻止普通bearer／RT并不影响后继family | 无HUKS／真实设备／HTTPS |
| Harmony改密host | 16项通过；新增依赖403不能解除prepared gate | 独立于设备方法执行 |
| Harmony原生编译 | 应用HAP＋50方法test HAP成功，全部测试名在ABC核到；unsigned | Hypium未执行、未签名分发 |
| 注销后端 | 另10个真实PG顶层方法全过、跑前／后／当前SHA一致 | RPC进程内；未称双集群／真实HTTPS |

两端同时修了复核发现：任何400／401／403／429不能一概视作未建。只有fresh首次提交的明确前置code白名单才能rejected；prepared后依赖403继续unknown、保持普通业务门禁。原改密Room测试数据库名原先不满足现有`JobsIM-test-`约束，已改正；仍只称编译，不称该反例已在设备执行。

最终文件审查另补Android `AvatarSources/<namespace hash>`原头像目录清理及第5个Room方法断言；磁盘自然回升后，12秒最窄增量app／test APK重新编译通过，未重复运行未改的JVM。旧冻结保留，当前四包与源码重新一致。

原始日志、源快照、60项鸿蒙原生输入逐项匹配、APK／HAP CRC、DEX／ABC测试名及未改Dist哈希见[最终冻结](./注销证据/最终冻结.json)。历史改密冻结与Dist保持；新包集中在本轮证据，没有覆盖旧分发包。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、输出包与复现 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 包 | SHA-256 |
| --- | --- |
| [Android应用](./注销证据/Android头像原件增量/JobsIM-debug.apk) | `5305969bd6d9904464f5cbdbaa70cb6da3937dd965c8c8913522fbe770c33ebc` |
| [Android测试](./注销证据/Android头像原件增量/JobsIM-debug-androidTest.apk) | `2346e30f39a6b5d906226b3bbada9d0b3fdd23a3feb83390355ffe6ba39ebcb2` |
| [Harmony应用](./注销证据/Harmony最终-default/entry-default-unsigned.hap) | `93d64ebc486ac1747f1a72998ca3456e737bff04e764b03af9055d650b5cb4c0` |
| [Harmony测试](./注销证据/Harmony最终-ohosTest/entry-ohosTest-unsigned.hap) | `87c9970820eb54920cecc9e123a15946fc04d9b50d69aa662f82ca42cd015b1e` |

```shell
node 测试/前端/HarmonyOS/Unit/RunDeletion.mjs/RunDeletion.mjs
node 测试/前端/HarmonyOS/Unit/RunPassword.mjs/RunPassword.mjs
node 测试/前端/HarmonyOS/Unit/RunAuth.mjs
```

Android使用现有离线Gradle、单worker／1536MiB JVM：`:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest`。鸿蒙沿现有镜像CLT分开`build_hap default`／`build_hap ohosTest`，ASCII暂存的原路径和source-manifest保留当轮日志；未调用覆盖Dist的发布步骤。最终构建完成后磁盘降至4GiB门禁以下，停止新增重型验证，继续文档／源码工作；没有删缓存、第三方或用户数据。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">四、待核边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

实际端内登录→注销→退出→查原单／受限查询→取消→重新登录，以及设备冷启动、真实安全存储和文件fsync均待设备执行。仅原创建完整上下文可在completed后重查；当前凭据已物理清理后不能跨浏览器／新设备新签proof。此前已签未到5分钟的query proof可观察终态。已有旧版未登记归属的孤立头像缓存须在设备兼容迁移中核对，不能仅凭UUID文件名推断本人而删其他账号缓存；本轮只验证现有持久索引与新逐文件归属。外部已保存文件、关机设备及历史离线备份各有独立责任；不由服务端completed或本轮编译推断已经全副本擦除。

没有提交或推送。后续群控／成员分页／邀请与普通迁移继续由主任务分工，不把本报告称全部I交付。

<a id="🔚" href="#前言">返回前言</a>

历史源码阶段的后增取消围栏（当时待核）：两端现在持久独立cancelVersion；unknown取消不签新cancel proof、不发mutation，先纯查同原version仍requested才能prepared，再明确用新证明续同nonce。旧缺version不补，查询新版本多次也不重基；proof query结果绑定其原id/version。Room／RDB CAS不准更改既有cancel tuple。十语16键，新增host／Room／Hypium反例均未运行或重编；此前包保持历史范围，详[待验摘要](./注销证据/取消原版本新增源码待验.json)。

2026-11-17当前Android修后150输入候选在私有API37 arm64 AVD实际执行注销9方法全部通过，含取消unknown／原cancelVersion、全账号范围擦除及后继围栏；修前旧冻结包的OS root别名导致本机最终擦除1红已保留并修复。完整当前102本机方法86通过／16明确跳过，另3S2真实HTTPS通过；并非真实注销API已验。Harmony16host及当前70输入HAP／63方法编译分列，设备未验。见[最终逐方法记录](./普通迁移两端证据/Android独立AVD-20261117-153544-d52437/最终105逐方法实际核对.json)。
