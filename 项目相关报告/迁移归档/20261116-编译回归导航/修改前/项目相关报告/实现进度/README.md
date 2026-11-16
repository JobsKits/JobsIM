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

当前认证基线已封存：Android [10 仪器＋6 JVM](../测试/前端/Android/20261115-访问刷新/README.md)；iOS [12 本机＋4 HTTPS](../测试/前端/iOS/2026-11-15-认证刷新与家族隔离/测试摘要.json)；CLI [31 本地 PASS＋2 SKIP、真实协调器／TTY各1组](../测试/CLI/20261115-刷新家族/README.md)；Web [16 模拟 host＋8 真实 GUI](../测试/前端/Web/20261115-认证输入保护)，当时构建为 `index-CnD80aSu`，七份源码及生产包已按原 SHA 归档，认证固定服务为 7ef；后续同步源码不追认这份基线。Android/CLI 的 AT 截止为自有 family SQL 时钟夹具，iOS 为客户端过期状态注入，都没有自然等待 15 分钟。

Harmony 新 auth 已落盘，原 stamp/token 围栏修复后 41/41 纯主机规则通过（既有25＋协调器11＋Api5）、语法转译 0 错误；本轮 Build-1 因 SDK 缺库失败保留，镜像补齐后 Build-2 应用 HAP 已通过 CompileArkTS／打包（未签名）；29 方法 test-HAP 46.4 秒编译通过，HDC [Empty]、原生设备待验。CLI 正式 macOS DPK 保存缺 entitlement，真实 TTY 是用户明确允许后的限程会话；Windows／Linux仅交叉编译。每次 App 修改同任务同步[资产盘点](../资产盘点/README.md)，不以源码存在代替全端通过。后端权威同步 schema1 固定构建 57a867 已通过 100 HTTP／1282 语义／32 页真实专项，包含 NATS 停机补偿与重连；独立恢复另记原始失败及成功续验，不把包装失败隐去。Web当前DXdl9E2N的真实同步10组与认证8组已过，本地各模块结果分列；iOS／Android／CLI权威消费为源码在建，Harmony待接。qualified ACK／after_receipt、服务端 read／draft／prefs 及完整 I 仍未交付。

2026-11-16接续：Web生产包`index-DXdl9E2N`已通过真实同步10组、真实认证8组；新业务终态9组、持久终态6组、原队列10组与认证协议16组分别通过，详见[固定检查点](../测试/前端/Web/20261116-权威同步检查点/README.md)。辅助查单503由浏览器注入，其后的真实global仍提交、原队列未改；这些数量不相加。iOS当前69份应用／本地Pod Swift＋测试文件parser通过，18项注入方法未运行；Android Room9及4JVM＋9仪器测试为源码未验；CLI权威消费已封25项输入和14项测试源码。三端均未有新同步构建或联网证据，Harmony仍待接权威同步。后端schema2新read／view／prefs仅源码，草稿尚未公开接线，均未编译实跑；固定57不变。磁盘约2.4GiB，仅完成有界小型Web构建与测试，原生大构建继续受4GiB门禁限制。

下面保留各业务切片的独立历史证据；认证与同步分别按上述固定构建及报告核对。

| 工程 | 已执行的真实验证 | 当前继续工作 |
| --- | --- | --- |
| [Go后端](../../代码/后端/Go/README.md) | 核心55、Safety27及既有治理／头像举报；原发送准入52真实HTTPS与OS锁／未知提交；搜索收藏279真实HTTPS；提及公告181 HTTPS＋42语义；角色准入46 HTTPS＋27语义；普通转发首轮187 HTTPS＋104语义 | 原引用转发已在独立测试域验证文本／媒体／撤权；公网慢JSON排空与最终既有回归继续补验。完整导出仍实验，强保护／收讫清理保持关闭 |
| [Web](../../代码/前端/Web/README.md) | 构建、十语三主题／字号、桌面手机及严格Outbox；既有真实HTTPS全链复跑通过。新增分页、原提及名单、block重投影、公告内容版本／原操作及角色循环；普通转发两类未知／原proof／撤源清正文缓存、晚响应单调及503／200临时占位同version恢复实测通过 | 转发已在最终公网JSON／媒体锁版复跑通过；最新认证见上方 8 GUI＋16 host；完整同步／导出与全部I尚未验收。独立转发域的静态Dist由测试同源路由提供，未冒充该域静态部署 |
| [iOS](../../代码/前端/iOS/README.md) | 原30/30、4/4与保留脚本1/1；原准入专项；搜索收藏5/5（4数据/API＋1原生UI）；提及公告5/5＋真实对端1/1；普通转发／实例围栏最终8/8（7数据/API＋1原生UI）及真实对端1/1通过 | 转发当前8＋peer证据及新增3项并发投影补充均已封存，数量不相加；最新认证 12＋4 已过（客户端过期注入）；27.2 Files、真机、完整同步／收讫／迁移备份待验。旧raw自动轮换已如实登记 |
| [Android](../../代码/前端/Android/README.md) | 原6 JVM＋9仪器0失败0跳过；搜索／协作新基线6 JVM＋8定向仪器及真实深色UI已封存，数量不相加 | Room7普通转发最终6 JVM＋10定向仪器通过，另单条200占位恢复通过且产品APK字节不变；最新访问刷新 10 仪器＋6 JVM 已过；真机和全部I未验 |
| [HarmonyOS](../../代码/前端/HarmonyOS/README.md) | libsdl镜像CLT6.1.1.280／API24／Hvigor6.24.2；提及／公告／角色及原引用转发应用HAP、含25项Hypium的测试HAP编译通过，25项host规则实际通过 | 新 auth 41 纯主机规则过，Build-2 应用 HAP 已过、29 方法 test-HAP 已编译、未设备执行；HDC无目标，未签名包未安装；ArkUI、RDB耐久、系统预览、设备Hypium与鸿蒙真实HTTPS未验，不外推其他端证据 |
| [CLI](../../代码/前端/CLI/README.md) | pure-Go SQLite WAL/FULL；原转发17组事务／故障、真实HTTPS治理与转发已归档；搜索协作另有21组SQLite／故障及8条真实HTTPS链路，数量不相加 | 搜索收藏分页、结构化提及与公告原操作已在清理前验证；新续期 31 本地＋2SKIP、真实协调器／TTY各1组；系统库缺口、完整快照／合格收讫与全部I继续 |
| [部署](../../部署/README.md) | Mac独立双PG／原7库备份，新增governance复用业务库独占schema、严格TLS双账户、加密备份验真、成对启停／重开／SIGINT回收；Linux/Windows语法和平台阻断 | NATS成对自托管入口已接，Mac有界broker／鉴权／重启实测通过；9owner＋gateway共10逻辑单元。Linux/Windows仅语法／静态，完整8域库＋broker备份恢复、目标机／容量／升级等待验；不接管外部进程或全局安装 |

原发信资格回归优先项：后端以同机共享StateDir的OS共享／排他锁协调跨业务库提交与撤权，并保留未知提交标记核原事务；五端保存原快照，禁止以新资格重放旧意图。服务端、端侧存储和真实联网结果分别登记，跨主机分布式围栏与吞吐尚未验收。

第一阶段最短后续合同依次为完整快照／增量、合格 ACK 与收讫策略、正式导出、账号注销、普通端迁移；详见[资产盘点优先序](../资产盘点/README.md#phase-one-slices)。这些仍是 I 缺口，不改排后期。完整群治理、隐私、申诉、MFA 管理等既有 I 事项仍逐项对照需求验收。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、接续与验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[HTTP契约](../../代码/后端/Go/接口契约/README.md)是当前可执行接口说明；完整产品目标见[需求总纲](../../需求/IM需求明细表.md/IM需求明细表.md)。独立日志与隔离恢复已有局部真实证据，管理员强验证、资产治理及其余基础细节继续闭环。不能将当前代码切片称为全部I完成；原243主验收项仍未验收。SDK／支付／地图／RTC／E2EE专项不提前接入。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、额度接续 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

用户授权本任务仅在额度耗尽后使用一次可用重置。当前会话仅有查询接口，界面工具明确禁止操作Codex自身；助手未执行兑换。中断后快照曾显示额度恢复、剩余次数变化，但这不能证明谁触发、是否兑换或是否平台自动恢复；此前“本任务的一次已经使用”的推断已撤回，实际兑换历史待核实。严格按全局公约先耗尽再用，不提前兑换，也不在次数归属不明时擅自再消耗。额度提示不替代软件主任务。

<a id="🔚" href="#前言">回到首页</a>
