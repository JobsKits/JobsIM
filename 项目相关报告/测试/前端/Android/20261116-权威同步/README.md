# <span id="前言">Android 权威同步限定验证</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前范围与证据](#jobs-toc-heading-1)
- [二、入口、默认值与持久化](#jobs-toc-heading-2)
- [三、源码与下一验证](#jobs-toc-heading-3)
- [四、当前固定包与原始结果](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、当前范围与证据</span><!-- jobs-toc:heading-end -->

当前原生 Kotlin／Room schema9应用及测试APK构建通过。**JVM10项与仪器10项分列通过，均0失败／错误／跳过；未运行真实HTTPS或原生UI。** 只复用本任务emulator-5582，不创建／擦除AVD、不清原用户库；没有请求后端API／SQL／重启。构建与安装均保留4GiB硬门禁，旧Dist/e295两包未覆盖。

[本地检查记录](./本地检查.json)保留编译前13个源码／测试SHA与检查边界，不追认后来构建。此前独立审阅未覆盖最终补丁；本页只记随后真实构建／原生测试的实际范围。

旧认证输入在 [认证输入基线](./认证输入基线/归档核验.json)留存38份原件及 [原验证摘要](./认证输入基线/原验证摘要.json)。canonical 与独立认证 APK 均保持 `e295688485e0caa73932670e48da227eae804108416625b62fba5b0e01cee8b3`，对应当时6 JVM＋10仪器，不能外推当前新同步源码。旧转发／搜索报告及原包未覆盖。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、入口、默认值与持久化</span><!-- jobs-toc:heading-end -->

| 维度 | 当前源码行为 | 状态 |
| --- | --- | --- |
| 入口／默认 | 页面进入、刷新、既有五秒轮询；实时 `authoritative_sync` 或已持有责任时走原生引擎；无新 UI 开关 | 构建通过；完整UI／真实HTTPS待验 |
| 首次安装 | 原UUID登记 → 原snapshot request → 全页原bytes SHA／绑定 → final validation → 首批changes权限确认 → Room原子安装 | 待验证 |
| 增量／未读 | 连续cursor，显式redacted闭区间；至多500条／轮，原origin去重；首批不当全历史未读，当前阅读会话不增加本地未读 | 待验证；非服务端read状态 |
| 存储 | `sync_records` 保存控制／manifest／页原引用envelope和SHA／事件元数据／对象投影／未读；消息和cursor同事务；草稿与原Outbox不搬 | schema9已KSP；可靠原件V8→9真实迁移通过 |
| 未知／生命周期 | 原nonce与stage保留；旧普通正文仅单一retained隔离；持久guard及family／device／epoch／namespace拒绝晚提交；缺能力不退旧poll | 原生注入通过；完整App生命周期待验 |
| global／forward | global跨当前／旧device／epoch／retained永久终态；受控forward仅RAM精确GET，持久库只占位，旧received forward迁移遮蔽 | 原生global／retained／清理重试通过；UI／真实鉴权媒体待验 |
| Outbox确认 | 同clientID不ACK；原完整body＋route只读message-results 200，再在Room事务比原体清原行；503／transport unknown／坏200零proof且不挡独立global | 原生注入通过；无创建式补发 |
| 通知／设置 | 延用十语／三主题、本地通知默认关闭且正文隐藏；仅已经ready的后续增量可通知 | 原默认保留，新接线待验 |

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、源码与下一验证</span><!-- jobs-toc:heading-end -->

- [工程入口与边界](../../../../../代码/前端/Android/README.md)、[代码地图](../../../../../代码/前端/Android/代码地图/README.md)。
- [SyncProtocol](../../../../../代码/前端/Android/app/src/main/java/com/jobs/im/SyncProtocol.kt)、[SyncStore](../../../../../代码/前端/Android/app/src/main/java/com/jobs/im/SyncStore.kt)、[SyncEngine](../../../../../代码/前端/Android/app/src/main/java/com/jobs/im/SyncEngine.kt)、[SyncGuardApi](../../../../../代码/前端/Android/app/src/main/java/com/jobs/im/SyncGuardApi.kt)、[SyncCleanup](../../../../../代码/前端/Android/app/src/main/java/com/jobs/im/SyncCleanup.kt)。
- [外置测试入口](../../../../../测试/前端/Android/README.md)：当前JVM10项（4新＋6旧），仪器10项（8同步＋1旧Api family＋1可靠V8迁移）；最终10项整组重跑，不把旧9＋新1重复累计。后续真实HTTPS及UI另行协调，不新下载镜像。

未知Outbox没有原body查单关联，原始payload仍完整保留；不声称清除了无法关联的原始正文。

服务端read／cloud drafts／偏好、qualified ACK／after_receipt、正式导出、账号注销与端迁移均仍为 I 阶段缺口；本轮不改变原243验收项状态。当前包是限定验证中间包，未替换canonical，仍不等于完整I可交付。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、当前固定包与原始结果</span><!-- jobs-toc:heading-end -->

| 证据 | 实际范围 |
| --- | --- |
| [最终构建](./Build-20261116-121304-27317-f957c0/构建结果.json) | 60输入原件／SHA；应用978、新测试6b0；Room9 schema；JVM10沿同978产品先前实际执行结果，本次Gradle UP-TO-DATE复用 |
| [JVM实际执行轮](./Build-20261116-115754-14667-083e7d/构建结果.json) | 6 Isolation＋4 SyncWire，0失败／错误／跳过 |
| [最终10仪器](./Instrument-20261116-121415-27662-967574/仪器结果.json)／[原始输出](./Instrument-20261116-121415-27662-967574/仪器原始结果.txt) | 8权威同步＋1旧Api family＋1原V8迁移，OK(10 tests)，0失败／跳过；安装拉回SHA相符 |
| [首次2过7失败](./Instrument-20261116-115355-12632-e08937/仪器结果.json) | 旧Room兼容事务入口不支持BundledSQLiteDriver；自有ChatDatabase改为同协程IMMEDIATE writer，保留原失败 |
| [迁移夹具9过1失败](./Instrument-20261116-121131-25011-faa935/仪器结果.json) | 原JSON省略无索引entity的indices键，fixture强取失败，未进入迁移；仅修夹具，原schema8不动 |

产品APK SHA256：`978b9f9c54e4fa22b8adbc92211610f10cb8238bf297c83bfa4517de922f7d08`。新测试APK：`6b0c5b6793f8127f45e5e460994308d4a47acea1b6fbba7df691fba1253f236f`。两包均在最终构建子项，旧Dist及认证独立包仍e295。

V8原件SHA：`c2d08158cfa211cc6300c45a83ae192ff3cc8739bdc9f2f729e0b30d43683404`，仓内／e295输入归档／旧清单三方相同；只打入test APK，产品包无该asset。迁移测试按原createSql／索引／setupQueries建独立V8库，经既有Migration8→9打开，逐列及所有TEXT UTF8比较。普通历史、草稿、pending forward原体、frozen状态、原操作及native_scope保持；received forward按既定九字段遮蔽。新sync记录起初为空、guard为空，无旧资格继承；显式新登记仍无stream／cursor、ready=false，关闭重开继续读回。

[最终只读归档核验](./最终校验.json)另列原件／源码／包一致性。本轮受控API均为进程内注入；未声称真实NATS／HTTPS、媒体流撤权、完整通知／多语言／主题UI、断电或合格ACK通过。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
