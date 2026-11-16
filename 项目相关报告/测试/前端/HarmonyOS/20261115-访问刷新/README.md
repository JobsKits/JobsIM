# 鸿蒙访问刷新限定验证

![Jobs出品，必属精品](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、入口、行为与默认值](#jobs-toc-heading-2)
- [二、持久化与生命周期](#jobs-toc-heading-3)
- [三、实际验证](#jobs-toc-heading-4)
- [四、仍待验收](#jobs-toc-heading-5)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮以[五端资产盘点](../../../../资产盘点/README.md)的单AT／认证刷新缺口为起点，接入完整凭据对、原意图及未知结果门禁。当前结果为**41项纯主机测试通过、应用HAP与29方法测试HAP编译通过**；没有鸿蒙设备安装、Hypium执行或真实鸿蒙HTTPS结果，不代表完整第一阶段验收。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、入口、行为与默认值 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

原登录／注册／恢复表单保留，登录页新增“使用当前证明重新验证”的确认入口和未知结果说明；三个新文案键覆盖原十语言。简中、跟随系统、字号1及通知关闭等默认值保持。服务端合同为AT15分钟、RT空闲30天、首次起绝对90天，客户端验证服务端期限，不自行延期。

JSON、文件上传／下载与头像统一经过AuthCoordinator；只有明确ACCESS_EXPIRED协调一次refresh。实时auth_refresh不支持则拒正式认证／业务，需新关联本人验证时还须auth_retirement_chain。同family成功刷新保持原设备及Outbox空间；重新认证得到新设备后，在RDB事务冻结旧空间再发布新身份，不搬运／自动重发旧队列。旧sendAdmission、roleGeneration、forward proof和原body不回填。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、持久化与生命周期 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

完整AT／RT、family、精确字符串代际、期限及连接／实例／本人／personal／设备／epoch，紧凑编码到一个AssetStore SECRET；UTF-8小于1024字节，单次OVERWRITE且不先删旧值。使用DEVICE_UNLOCKED与SYNC_TYPE.NEVER。RDB只存非秘密原requestId、用途、用户名、未知状态、family、完整退休闭包和公开交接点；密码／OTP只在内存，不写摘要猜测凭据。

初始认证在HTTP前写原意图，普通重试保持原ID／字段；无法重建原证明时，用户明确当前本人验证并关联完整旧→新请求闭包。未知register且账号未确认存在时沿原register。refresh结果未知或安全库失败立即阻断旧RT重用及自动业务；安全库已写而RDB提交中断，可从匹配原requestId／family／代际的完整凭据续交接，不重新refresh。

新设备namespace包含profile／部署／本人／personal／epoch／device。原AT、scope stamp和当前owner检查防晚请求结果清除新身份；退出、401、冷启动context均有原身份比较。新增第11个反例曾复现冷启动context晚到误清新设备，最小修复后通过。旧单AT不作为RT读取，原空间保留并冻结。

[华为Asset Store参考](https://developer.huawei.com/consumer/en/doc/doccenter-references/api/js-apis-asset/)及[能力概览](https://developer.huawei.com/consumer/en/doc/harmonyos-guides/asset-store-kit-overview/)与本地API24声明共同约束SECRET大小和alias操作。单次OVERWRITE不等于已证明设备断电原子性，也不构成AssetStore与RDB跨库事务；这些仍需原生设备故障测试。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、实际验证 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 范围 | 结果与证据 | 证明边界 |
| --- | --- | --- |
| 原纯规则 | 25/25，通过 | 既有消息／群／提及／公告／转发与十语规则 |
| 认证协调器注入 | 11/11，通过 | 并发刷新一次、未知不重用RT、安全库失败、原ID／完整闭包、晚失败／退出／context、新旧设备与中断交接；持久化及网络为替身 |
| Api注入 | 5/5，通过 | JSON／上传／下载／头像共用刷新、旧绑定请求前拒绝、晚成功不越namespace、未知写不重发、公有认证不附AT |
| 当前主机合计 | [41/41，0失败／跳过](./Host-1/Results.txt)；[对应源摘要](./Host-1/SourceManifest.json) | Node18.20.1＋同镜像TypeScript；Models／Coordinator严格类型检查0诊断；未执行系统服务 |
| 应用HAP | [Build-2](./Build-2/运行日志.txt)，63.670秒，exit0 | 真实ArkTS编译／打包，unsigned；字节码含AuthCoordinator |
| 外置测试HAP | [TestBuild-1](./TestBuild-1/运行日志.txt)，46.394秒，exit0 | 29个Hypium方法名全部进入字节码，**未执行**；四个新增方法只调用纯AuthRules |
| 包／源码 | [结果与产物摘要](./结果.json)、[当前源码SHA256](./源码摘要.json)、[Dist基线](../../../../../代码/前端/HarmonyOS/Dist/认证刷新基线.json) | 两包ZIP CRC完整，8个App源和外置测试源与两轮构建快照相符 |
| HDC | [Empty](./HDC目标.txt) | 无目标、无签名安装、无设备截图／HTTPS／系统服务结果 |

应用HAP SHA256：`dd7775def9b34bc78d08446e3b5bcaa5b94aae4c8d00cf08174469a45eb8bc31`。测试HAP SHA256：`986862148d6548288b7fde9013bde5cace0e1d6589a1a5f9e0fd8e115b1345b1`。正式分发位置仍是工程Dist；原两份HAP已先在用户持久Build目录归档并记录摘要，原报告／JSON／图片未覆盖。

首轮[原失败日志](./Build-1/运行日志.txt)记录00303168 SDK扫描错误；[后续真实缺库](./Build-1/SDK-probe.txt)为11201001／libsec_shared.dylib。root从同一用户指定libsdl ZIP补原元数据和动态库闭包后，Build-2通过。未切换下载源、修改SDK源码、安装全局库或降低TLS；ETag／CRC／SHA是完整性依据，不声称厂商签名验证。

构建保留平台API弃用、异常处理、测试资源同名与包版本格式告警，本轮未用改SDK或无关重构消除。相应源码位置见[代码地图](../../../../../代码/前端/HarmonyOS/代码地图/README.md)。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">四、仍待验收 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

设备签名安装、真实RCP HTTPS／CA拒绝、AssetStore锁屏及服务失败／原子替换、RDB强杀／断电耐久、两者交接崩溃窗口、系统后台／多Ability、全部十语／三主题／字号与API12兼容仍未验证。未自然等待15分钟／30天／90天；主机注入不替代期限、复用检测和服务端family隔离的原生联调。媒体本地25MiB与后端64MiB差异保留，没有借认证改动扩大媒体能力。

完整同步、资格ACK与清云、正式导出／迁移／注销等I缺口仍按[资产表](../../../../资产盘点/README.md)补齐。此次未动后端、其他端、旧服务、全局信任或代理；旧Caches路径未重建。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
