# <span id="前言">JobsIM 第一阶段实现状态</span>

![JobsIM](../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前主任务](#jobs-toc-heading-1)
- [二、已落盘与进行中](#jobs-toc-heading-2)
- [三、接续与验证边界](#jobs-toc-heading-3)
- [四、额度接续](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、当前主任务 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

用户已授权开始第一阶段代码，优先原生五端。编码前原有内容已按用户要求整体归入需求目录：原七份文档、数据库、xlsx与所有配图均保留，需求目录的README承接原根README，根README改为工程入口。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、已落盘与进行中 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

> 2026-11-16 当前检查点；只记录对应断言，不代表全 I 验收。报告集中在本目录的测试分区，资产表同步源码、实际验证及待核状态。

环境接续：用户确认过 `clean`，其清空了用户 Cache；历史源码、报告与冻结包保留。新工具／运行域使用工程外 Application Support、备份另置 Documents；新身份与 CA 是重新初始化，不是恢复旧数据。清理前证据仍按原构建解释；下面的新认证基线是在新持久环境分别执行，不能跨轮相加。

历史访问刷新基线已封存：Android [10 仪器＋6 JVM](../测试/前端/Android/20261115-访问刷新/README.md)；iOS [12 本机＋4 HTTPS](../测试/前端/iOS/2026-11-15-认证刷新与家族隔离/测试摘要.json)；CLI [31 本地 PASS＋2 SKIP、真实协调器／TTY各1组](../测试/CLI/20261115-刷新家族/README.md)；Web [16 模拟 host＋8 真实 GUI](../测试/前端/Web/20261115-认证输入保护)，当时构建为 `index-CnD80aSu`，七份源码及生产包已按原 SHA 归档，认证固定服务为 7ef；后续同步源码不追认这份基线。Android/CLI 的 AT 截止为自有 family SQL 时钟夹具，iOS 为客户端过期状态注入，都没有自然等待 15 分钟。

历史auth／S1检查点：Harmony stamp/token围栏修复后41/41纯主机规则通过（既有25＋协调器11＋Api5），Build-1缺SDK库失败保留，Build-2应用HAP与29方法test-HAP仅编译、未设备执行；后续S2双HAP／37方法见下文，不把29当当前总数。CLI正式macOS DPK保存缺entitlement，真实TTY是用户明确允许后的限程会话；Windows／Linux仅交叉编译。后端schema1固定57a867有100 HTTP／1282语义／32页、NATS补偿重连及独立恢复证据；旧Web DXdl9E2N有同步10／认证8，各自固定输入保留。它们不覆盖后来的S2或云草稿代码，也不代表Harmony当前未接同步。每次App修改同任务同步[资产盘点](../资产盘点/README.md)。

前序S1→S2接续记录（历史输入）：Android曾过10 JVM／10仪器含V8→9迁移；iOS曾过18同步注入／12原认证本地；CLI曾过45本地／2跳过及schema1真实PTY10。原始编译、driver与夹具失败保留，不拿旧包追认。后端S2首阶段16纯规则、20真PG、8双PG／NATS／HTTPS专项分列（125项JSON HTTP、334语义、17页；额外socket／CAS／health另列），该PhaseB专用服务已撤收；固定57／7ef没有据此改变，后续独立云草稿及客户端E2恢复域不属于这次撤收。

当前S2增量按[资产盘点](../资产盘点/README.md)分轮收口：Android5774有11 JVM／23原生／3真实HTTPS与UI；iOS固定30原生、新sim真接口／完整UI失败及六family精确撤销补证按原报告保留，新31原S2与10导出共41已在固定89e／94a1包实际本地通过，不借旧包。Harmony35host／41旧规则、双HAP／37方法仅编译。旧Web D5真11／本地26／模拟认证16，新独立草稿恢复10注入／构建各自封存。CLI旧15a4真13／本地55、12ac正文67及395对象79均保原件；新对象eb42为80本地／0失败／2环境跳过及Mac构建，完整准备记录／global与迟到围栏已执行；正式导出13cd为88本地／0失败／2环境跳过及Mac构建。这些均不相加为需求完成数，CLI新两轮没有真实API／PG。

服务端[云草稿](../测试/后端/20261116-云草稿/README.md)含公开S2与独立真实恢复；[正式导出2b58](../测试/后端/20261116-正式本人导出/README.md)当前获权数据／全媒体、源排空／取消／恢复拒旧已有独立限定证据，最终完整媒体27 HTTP／13语义。客户端新[Web导出](../测试/前端/Web/20261116-正式本人导出/README.md)全工程TS类型、18 Chrome注入和a53afe46生产JS通过，未真实HTTPS／最终用户保存；[CLI导出](../测试/CLI/20261116-正式本人导出/README.md)13cd有88本地／Mac构建，未真实服务；[iOS导出](../测试/前端/iOS/20261116-正式本人导出/README.md)App89e02736／test94a1cbb8完整build-for-testing通过、123输入与归档逐字节核齐，31原S2＋10导出共41实际本地通过、0失败／跳过，本端既有sim已正常关闭；无API／Files保存；原Swift6隔离失败保留；[Android导出](../测试/前端/Android/20261116-正式本人导出/README.md)App1588c450／test76826edf构建及16 JVM（5新）通过，5原生仅编译，无安装／AVD／API；Harmony导出源已接，首15host通过、追加18目标未验，HAP／设备未验。[对象9f3b](../测试/后端/20261116-完整对象收讫/README.md)有7纯／9真PG、类型／构建及media SH排空，依赖owner显式夹具，无HTTPS／端落盘；targets／累计ACK／保留worker未完，after_receipt=false。

本人主动改密新切片：[最终执行原件](../测试/后端/20261116-本人主动改密/执行记录/2026-11-16T12-41-43-795Z-ff2d05/结果.json)为4纯规则／12真PG，生产auth与journal处理器同随机隔离PG库，message／governance为显式进程内确认夹具；没有真实HTTPS、独立恢复或原生客户端证明，[62d3构建](../测试/后端/20261116-本人主动改密/生产构建.json)与[94项输入归档](../测试/后端/20261116-本人主动改密/最终输入SHA256.json)已过。[password_change.go](../../代码/后端/Go/password_change.go)和[worker](../../代码/后端/Go/password_change_worker.go)已落，当前源码默认`own_password_change=true`，原结果纯查／未知续跑、成功撤全families且不新建登录；CLI7150已接并通过97本地／Mac构建，其余四端入口待接，不把恢复改密当普通本人主动改密。

注销续写先完成[普通准入门禁](../测试/后端/20261116-注销普通准入门禁/README.md)：现有deletion_state只有none才可普通登录／AT／BFF／RT／恢复／重置／导出principal／新改密。七文件增量有4纯／25PG通过事件（原12改密＋5新组及8子项）与两个cmd构建，源／依赖为显式进程夹具；没有公开注销API、受限data_rights、取消或到期清理worker，不能称注销已完成。62d3旧包不追认这次源补。

客户端新检查点：[CLI本人主动改密](../测试/CLI/20261116-本人主动改密/README.md)7150有97本地／0失败／2环境跳过及Mac构建，99顶层名称与原raw集合核齐；原95计数器输出保留、未改旧96轮，无真实HTTPS／TTY／系统库。Harmony正式导出当前源已接；[首15host和后补18目标](../测试/前端/HarmonyOS/20261116-正式本人导出/README.md)分列，后者被4GiB门禁拦下，本轮HAP／设备／系统保存未验。其余四端主动改密仍待接，全部I未完成。

下面保留各业务切片的独立历史证据；认证与同步分别按上述固定构建及报告核对。

| 工程 | 已执行的真实验证 | 当前继续工作 |
| --- | --- | --- |
| [Go后端](../../代码/后端/Go/README.md) | 核心55、Safety27、准入52／搜索279／提及公告181＋42／角色46＋27／转发187＋104各自历史保留；正式导出2b58／对象9f3b及主动改密4纯＋12真PG分别有当前限定证据，不累计 | 改密62d3构建及94输入已封存；HTTPS／独立恢复／五端接线继续；auth／journal是真处理器，message／governance改密依赖显式进程内确认夹具。targets／累计ACK／保留worker未闭环，after_receipt及强保护未开启 |
| [Web](../../代码/前端/Web/README.md) | 旧D5 S2真11／本地26／模拟16分别保留；新独立恢复10注入／构建；正式导出18 Chrome注入、TS类型、a53afe46生产JS通过 | 正式导出真实HTTPS／最终用户保存、新恢复正式Settings联调仍待验；旧D5／DXdl不追认新包。主动改密及完整收讫客户端待接 |
| [iOS](../../代码/前端/iOS/README.md) | 原搜索收藏5、提及公告5＋peer、转发8＋peer及并发投影3各自封存；新S2界面30原生通过，不混算原认证12＋4 | 新sim真实接口已跑、完整UI因夹具交接和磁盘门禁未通过；六family已实际撤销，新公开档案源修与31项目标实际本地通过／跨进程完整UI未过；clean末轮authenticate失败／账户0补证独立保留，该补证不作为family已关闭证明。原27.2 Files阻断、真机、完整收讫及迁移另验；正式导出App89e02736／test94a1cbb8与41目标完整编译通过，123输入核齐，41实际本地通过／本端既有sim已关闭，无导出完整UI／真实HTTPS／系统保存证明，主动改密待接，旧raw轮换已登记 |
| [Android](../../代码/前端/Android/README.md) | 原转发／搜索协作／访问刷新各轮独立保留；当前S2 5774有11 JVM／23原生／3真HTTPS与UI，6family退出已验 | 已通过的S2范围不再列未接；正式导出主／测试APK及16 JVM通过，5 Room／模型原生方法仅编译；系统保存／文件与目录fsync／真实HTTPS待验。完整对象／targets／保留删除、主动改密／真机与其余I继续；未借旧5774产品通过新导出，原截图过渡帧不充当终态证据 |
| [HarmonyOS](../../代码/前端/HarmonyOS/README.md) | libsdl镜像CLT／API24与原提及／转发切片保留；当前S2 35host／41旧规则，App02／Test02双HAP与37方法仅编译 | 同步／本人read／view／字号／云草稿已源码接线，HDC无目标；ArkUI、RDB NAPI／AssetStore、设备Hypium和真实鸿蒙HTTPS未验，正式导出源已接，首15host与追加18待验分列；新5 Hypium仅源码、42方法未编译，主动改密未接，不借其它端证据 |
| [CLI](../../代码/前端/CLI/README.md) | 旧15a4真TTY13／本地55及旧12ac正文67／395对象79原件保留；新[对象eb42](../测试/CLI/20261116-附件收讫/README.md)80本地／Mac构建；新[正式导出13cd](../测试/CLI/20261116-正式本人导出/README.md)88本地／Mac构建，均0失败／2环境跳过 | 当前两轮均实际SQLite／文件fsync与注入HTTP，未真实批准／导出API／PG。系统库正向、目标OS、收讫targets与保留删除、真实导出和主动改密7150有97本地／Mac构建，真实API／TTY／系统库仍待验，不借旧15a4真13 |
| [部署](../../部署/README.md) | Mac独立双PG／原7库备份，新增governance复用业务库独占schema、严格TLS双账户、加密备份验真、成对启停／重开／SIGINT回收；Linux/Windows语法和平台阻断 | NATS成对自托管入口已接，Mac有界broker／鉴权／重启实测通过；9owner＋gateway共10逻辑单元。Linux/Windows仅语法／静态，完整8域库＋broker备份恢复、目标机／容量／升级等待验；不接管外部进程或全局安装 |

原发信资格回归优先项：后端以同机共享StateDir的OS共享／排他锁协调跨业务库提交与撤权，并保留未知提交标记核原事务；五端保存原快照，禁止以新资格重放旧意图。服务端、端侧存储和真实联网结果分别登记，跨主机分布式围栏与吞吐尚未验收。

第一阶段后续按[资产盘点优先序](../资产盘点/README.md#phase-one-slices)继续S2未覆盖流与逐端真实、对象／冻结targets／累计ACK／保留、正式导出五端真实交付、主动改密客户端及真实恢复、账号注销与普通迁移。已实际通过的CLI80／88与Web导出18／TS／构建不再记为待编译；iOS导出已编译及41本地通过，真实HTTPS／系统保存仍未验，Android导出16 JVM／主及测试APK已过但5原生仅编译、Harmony导出源已接，首15host通过、追加18目标未验，HAP／设备未验仍分别列明。完整群治理、隐私、申诉及MFA管理等既有I继续对照需求，不以此排序延期或删减。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、接续与验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[HTTP契约](../../代码/后端/Go/接口契约/README.md)是当前可执行接口说明；完整产品目标见[需求总纲](../../需求/IM需求明细表.md/IM需求明细表.md)。独立日志与隔离恢复已有局部真实证据，管理员强验证、资产治理及其余基础细节继续闭环。不能将当前代码切片称为全部I完成；原243主验收项仍未验收。SDK／支付／地图／RTC／E2EE专项不提前接入。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、额度接续 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

用户授权本任务仅在额度耗尽后使用一次可用重置。当前会话仅有查询接口，界面工具明确禁止操作Codex自身；助手未执行兑换。中断后快照曾显示额度恢复、剩余次数变化，但这不能证明谁触发、是否兑换或是否平台自动恢复；此前“本任务的一次已经使用”的推断已撤回，实际兑换历史待核实。严格按全局公约先耗尽再用，不提前兑换，也不在次数归属不明时擅自再消耗。额度提示不替代软件主任务。

<a id="🔚" href="#前言">回到首页</a>
