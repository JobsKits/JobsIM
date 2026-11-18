# JobsIM · Android 与 HarmonyOS 回归

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

2026-11-16 对照[现行需求](../../../../需求/IM需求明细表.md/IM需求明细表.md)、[五端资产](../../../资产盘点/README.md)及当前源码，回归 [**Android**](https://developer.android.com/) 与 [**HarmonyOS**](https://developer.huawei.com/consumer/cn/) 的认证、消息、同步、本人状态和导出链路。结论是**当前版本仍未完成完整 I**；本报告修复两个鸿蒙生产编译阻断、一处认证测试依赖遗漏及 Android 导出任务的原生语境文案。

精确执行结果、71 项源码摘要、产物摘要及旧 Dist 校验见[结果与源摘要](./Android-HarmonyOS证据/结果与源摘要.json)。未使用正式账户／消息或现有部署，未安装 SDK、未清理缓存、未覆盖旧 Dist，未执行提交或推送。ADB 枚举无设备，HDC 为 `[Empty]`；本轮没有设备安装、Hypium 执行、真实 HTTPS 或系统保存选择器结果。

## 一、复现、修复与结果 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 优先级 | 当前问题及影响 | 修复与验证 |
| --- | --- | --- |
| P1 | 鸿蒙正式导出源码无法构建应用：`ExportFiles.save` 的 catch 末尾调用 `never` helper 未被 ArkTS 识别为必有返回；页面 `exportState` 的无类型对象字面量违反 ArkTS 约束。主机导出 18 项通过不能发现这些原生编译错误 | 异常路径明确 `throw`，状态映射使用穷尽 switch。[首轮实际失败](./Android-HarmonyOS证据/Harmony应用构建/构建.log)退出 255；修后[应用编译](./Android-HarmonyOS证据/Harmony应用终验/构建.log)及[42 方法测试 HAP 编译](./Android-HarmonyOS证据/Harmony测试终验/构建.log)均退出 0 |
| P2 | 新导出接入后 `Api` 增加 `ExportProtocol` 运行时依赖，认证主机入口没有转译其依赖闭包，fixture 又从测试源目录解析相对 require，当前 41 项实际 36 过／5 失败 | 转译 ExportProtocol／SyncProtocol／SelfProtocol，使用编译文件的 `createRequire` 解析同目录依赖，仍只替换原生网络与文件端口。[原失败](./Android-HarmonyOS证据/Harmony认证首轮/Results.txt)保留；[修后 41／41](./Android-HarmonyOS证据/Harmony认证复验/Results.txt)，0 失败／跳过 |
| P3 | Android 原生导出页十语的任务标题及本机记录说明仍写“浏览器／站点数据”，误导用户理解任务保存位置与清除边界 | 仅替换 `exportLocalList`／`exportLocalOnly` 两键，复用同源设备语境译文，并纠正两端德语遗留的浏览器语境；十语键完整性、16 项 JVM、应用与测试 APK 编译通过。没有改变任务持久化、请求或取消行为 |

相关源码：[鸿蒙文件保存](../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/ExportFiles.ets)、[鸿蒙页面](../../../../代码/前端/HarmonyOS/entry/src/main/ets/pages/Index.ets)、[认证入口](../../../../测试/前端/HarmonyOS/Unit/RunAuth.mjs)、[API 测试适配](../../../../测试/前端/HarmonyOS/Unit/AuthApi.test.cjs)、[Android 十语](../../../../代码/前端/Android/app/src/main/java/com/jobs/im/PersonalExportStrings.kt)。

## 二、当前业务链路及未完项 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 链路 | 源码已实现 | 本轮实际已验 | 待核／仍缺能力 |
| --- | --- | --- | --- |
| 可信连接、注册／登录／恢复 | Android `ChatModel`／`SessionCoordinator`／`NativeSession`；鸿蒙 `Index`／`AuthCoordinator`／`NativeServices`。实例 CA 与身份隔离，AT／RT 整对安全保存，unknown 原请求及家族围栏 | Android JVM 中 URL／空间规则；鸿蒙认证 41 主机反例；两端当前包编译 | 本轮无设备安全库、锁屏／进程重启、原生 HTTPS 或双账户业务；不套用旧 APK／HAP 证据 |
| 会话、文本／附件、群与消息操作 | Android `ChatModel` 及各原生 screen；鸿蒙 `Index`／`Api`。正式与演示隔离，原操作 ID，单聊／建群、基本成员治理、回复／编辑／撤回／反应／置顶／收藏、引用转发 | Android 原有隔离 JVM；鸿蒙规则中成员代际、原资格、公告／转发反例；当前包编译 | 不等于实际设备收发／后台与断网恢复。完整群禁言／解散等治理仍未闭环，系统查看器不等于内置录音录像或播放器 |
| 权威同步、已读／视图／云草稿 | 两端 `SyncEngine`／`SyncStore` 及本人状态服务。完整页校验后提交，原未决消息只查单，global 清理；S2 保存原 CAS／nonce，采纳草稿核本机原快照 | 鸿蒙当前 S2／同步 35／35 主机 SQLite 适配；Android 当前 JVM 中 wire／隔离规则 | Android Room 仪器本轮未执行；鸿蒙 SQLite 适配不是 RDB NAPI。两端显式只接 `transport_only`，拒绝 qualified ACK／afterReceiptEnabled；完整收讫目标、累计 ACK 与清云不在本次源码完成范围 |
| 正式本人导出 | 原创建／取消意图耐久、原 body 查单／明确重试、短票只 RAM、私有 ZIP 校验与文件同步后交系统选择器；本机列表有空态／重载 | 鸿蒙 18／18 主机反例，包含流刷新及文件／选择器适配；Android 5 导出 JVM（属于当前 16）；两端当前应用及测试包编译，鸿蒙 42 方法名全部进入测试字节码 | 没有原生系统文件提供方、真实导出 HTTPS、外部保存或断电证明；42 方法仅编译，不是 Hypium 执行 |
| 本人主动改密、账号注销、普通端迁移 | 本次扫描两端当前目录，未找到相应原生页面／服务路由接线；恢复密码不等于主动改密，JSON／正式导出不等于受控导入迁移 | 无相应本轮端侧用例 | 属于完整 I 的明确产品缺口，不能凭后端或 CLI 已有实现标记两端完成 |
| 主题、十语、字号与无障碍 | 两端有白天／黑夜／跟随系统、十语、三档字号，导出继承原状态 | 当前键表完整性与两端编译；鸿蒙 literal UI label 主机检查 | 没有设备实际渲染矩阵、系统主题变更、可访问性、API 最低版本或容量验收 |

每一行的源码、主机、原生编译与设备结果分开；不将本轮测试数量相加成产品验收数。外部渠道、厂商推送、RTC、支付、产品 SDK 与 E2EE 仍按原后续标段处理，没有把基础缺口改排到后期。

## 三、实际验证与可重跑命令 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 验证 | 当前结果与证据 |
| --- | --- |
| Android 离线 JVM 与应用／测试 APK | `:app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest` 退出 0；16 测试、0 失败／错误／跳过。[XML 与两个 APK](./Android-HarmonyOS证据/Android/) |
| 鸿蒙认证 | 首轮 36／41，修后 41／41，原失败及源清单保留 |
| 鸿蒙 S2／同步 | 35／35，0 失败／跳过。[原始结果](./Android-HarmonyOS证据/Harmony同步/Results.txt)；Python SQLite 桥接不是 RDB NAPI |
| 鸿蒙导出 | 修前 18／18，生产编译修后仍 18／18，0 失败／跳过。[修后结果](./Android-HarmonyOS证据/Harmony导出终验/原始结果.log)；Node 文件／选择器、RCP 注入不是设备服务 |
| 鸿蒙原生构建 | 应用修前退出 255，最终词条源应用 65 秒／测试 55 秒成功；两个 HAP 无签名。全部 42 个源方法名在 `ets/modules.abc` 可核对，未执行这些测试 |
| 包完整性与旧基线 | 两 APK／两 HAP 的 ZIP CRC 检查通过；Android e295、鸿蒙 S2 应用 3db7／测试 d8c7 均保持旧摘要 |

在仓库根运行当前主机入口：

```shell
node 测试/前端/HarmonyOS/Unit/RunAuth.mjs
node 测试/前端/HarmonyOS/Unit/RunSync.mjs
node 测试/前端/HarmonyOS/Unit/RunExport.mjs
```

在 Android 工程目录，先按[工程运行说明](../../../../代码/前端/Android/README.md#前言)使用已有 JDK／SDK，再执行：

```shell
./gradlew --offline --no-daemon --max-workers=1 --console=plain \
  '-Dorg.gradle.jvmargs=-Xmx1536m -XX:ActiveProcessorCount=2 -Dfile.encoding=UTF-8' \
  -Pkotlin.compiler.execution.strategy=in-process \
  :app:testDebugUnitTest :app:assembleDebug :app:assembleDebugAndroidTest
```

鸿蒙原生验证复用现有[测试入口](../../../../测试/前端/HarmonyOS/运行测试.command/运行测试.command)的 `find_tools`／`build_hap`，只加载函数定义、显式提供仓库与当前证据目录，分别构建 `default`／`ohosTest`；跳过 `run_tests` 中复制到 Dist 的步骤。完整构建输出和所用 ASCII 快照位置逐项保留，不更改工具链或第三方源。原生警告仍有 API 弃用与可抛异常提示，不将编译成功称为这些运行路径均已验收。

## 四、包与后续验收入口 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 当前独立产物 | SHA256 |
| --- | --- |
| [Android 应用](./Android-HarmonyOS证据/Android/JobsIM-debug.apk) | `3e72d039ce870e0efeeb76a872f8e64c26cf7a76503f64630a184a76abb96108` |
| [Android 测试](./Android-HarmonyOS证据/Android/JobsIM-debug-androidTest.apk) | `76826edf7a3d38a209b56c8503c08b572aa31e5c25d63c29dffeada4d134ed06` |
| [鸿蒙应用](./Android-HarmonyOS证据/Harmony应用终验/entry-default-unsigned.hap) | `ceb9df450edb9e42941e6a7ae767aabfe3083eb4ee74e21a23b79d47af6a99ee` |
| [鸿蒙测试](./Android-HarmonyOS证据/Harmony测试终验/entry-ohosTest-unsigned.hap) | `9c5eec59b2ba46911d4c343e5a502e49010a7730f1695473ab971556270b5586` |

下一验收应先补两端独立设备的认证／真实收发、断网与恢复、系统保存及生命周期，再按原需求补主动改密、注销、迁移与完整群治理，收讫资格与云清理继续保持服务端能力门禁。签名与设备信任由实际目标建立；本轮不绕过这些前置条件。

<a id="🔚" href="#前言">返回前言</a>
