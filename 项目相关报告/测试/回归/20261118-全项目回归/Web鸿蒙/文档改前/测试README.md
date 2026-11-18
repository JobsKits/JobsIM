# JobsIM 鸿蒙原生测试

[toc]

## 当前结果

本人主动改密新增[14条协议／SQLite仓库主机反例](./Unit/RunPassword.mjs/README.md)，原认证入口新增两条未知暂停与后来登录围栏，当前43项；OhosTest新增4条纯规则，使当前方法数46，原生编译与设备执行分开记录。[改密实现报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS改密.md)保存源码与产物摘要，旧42方法报告保持原义。

[当前全项目回归](../../../项目相关报告/测试/回归/20261116-全项目回归/Android-HarmonyOS.md)分别执行认证41、S2同步35与导出18项主机反例，均通过。当前应用与42方法源码测试HAP真实编译通过、无签名且未安装；旧37方法Dist保持原SHA。认证首轮36过／5失败是Api新依赖漏入测试闭包，应用首轮失败是两个ArkTS编译约束，原始失败日志均保留。主机依赖、输出及命令见[Unit说明](./Unit/README.md)。下文旧计数保留对应历史范围，不据它们证明当前设备行为。

当前S2切片35／35主机反例（25基础＋10 S2，不累加旧24／25轮）、41／41旧规则通过；最终应用和37方法源码测试HAP编译通过，未运行Hypium／安装设备／实际鸿蒙HTTPS。[S2证据](../../../项目相关报告/测试/前端/HarmonyOS/20261116-S2本人状态/README.md)和[当前S2 Dist基线](../../../代码/前端/HarmonyOS/Dist/S2本人状态基线.json)保原S1包与输入。以下S1／认证数字仍为各自历史范围，不追认新源或新包。

2026-11-16 schema1同步切片已通过24项新主机反例与41项旧规则回归，应用及32方法源码测试HAP离线编译通过。32项没有在设备执行；RDB Store主机用真实SQLite适配，不冒充Harmony NAPI。[本轮证据](../../../项目相关报告/测试/前端/HarmonyOS/20261116-权威同步/README.md)保留首轮18／2、两次ArkTS失败和最终输入／产物SHA；当前Dist绑定[同步基线](../../../代码/前端/HarmonyOS/Dist/权威同步基线.json)。下述访问刷新29条／41主机仍为其独立历史范围，旧HAP已另存。

2026-11-15访问刷新切片已通过**真实应用HAP与外置29方法测试HAP构建**，均为未签名包；41项纯主机测试实际通过（原规则25＋认证协调器11＋Api注入5），0失败／跳过。未执行模拟器／真机安装、Hypium或真实鸿蒙HTTPS。工具来自用户指定libsdl镜像：CLT6.1.1.280、SDK6.1.1.125／API24、Hvigor6.24.2、Node18.20.1。29条不能写成设备测试通过。

[访问刷新限定证据](../../../项目相关报告/测试/前端/HarmonyOS/20261115-访问刷新/README.md)保存该轮构建、主机结果、源码与产物摘要，以及首次SDK缺件失败；[认证历史基线](../../../代码/前端/HarmonyOS/Dist/认证刷新基线.json)记录该轮unsigned HAP，原包字节另存本轮上一认证Dist目录。旧转发25规则／25方法HAP及原报告保持独立历史含义。

[本轮提及公告与分页证据](../../../项目相关报告/测试/前端/HarmonyOS/20261115-提及公告与分页/README.md)提供成功日志、源码快照清单及HAP／Ark字节码摘要。更早缺SDK、官网取包和构建失败日志保留供核查，当前SDK阻塞已由镜像解除，不再要求登录华为下载门户。

## 使用

在仓库根执行[同名自包含入口](./运行测试.command/运行测试.command)：

```sh
测试/前端/HarmonyOS/运行测试.command/运行测试.command check
测试/前端/HarmonyOS/运行测试.command/运行测试.command build
测试/前端/HarmonyOS/运行测试.command/运行测试.command test-build
测试/前端/HarmonyOS/运行测试.command/运行测试.command emulator
测试/前端/HarmonyOS/运行测试.command/运行测试.command device
测试/前端/HarmonyOS/【MacOS】运行规则测试.command/【MacOS】运行规则测试.command
```

先运行[工具链准备入口](../../../代码/前端/HarmonyOS/工具链/README.md)。`JOBS_IM_HARMONY_CLT_HOME`／`JOBS_IM_HARMONY_CACHE`可指定源码外缓存；缺件仅从libsdl镜像取得，旧健康缓存保留实际来源，不修改全局环境、代理或系统CA。入口直接使用CLT内置插件和Node，`--offline`避免Hvigor包装器安装；本地Hypium依赖来自同镜像SDK，意外网络查询也不回退其他registry。临时Python下载取证脚本不是产品依赖。

官方构建器拒绝中文工程路径，入口在ASCII用户缓存生成本轮源码快照、标准ohosTest布局和必要的本地依赖元数据。原测试物理源仍在本目录OhosTest；只在临时副本调整相对import及依赖位置。`source-manifest.json`记录原始／暂存文件SHA256，HAP导出工程`Dist/`。日志位于工程根 `项目相关报告/测试/前端/HarmonyOS/时间目录/`。构建失败保留真实退出码，缺工具／无设备／签名缺失返回非零，不生成假的成功标记。

emulator／device需要已启动并授权的目标及适用开发签名；当前未为截图额外下载模拟器。多个设备使用fzf方向键／回车选择、Esc取消，非交互可用`HDC_TARGET`指定已枚举设备。`HDC_COMMAND`可指定已有HDC。模式与设备类型不匹配会拒绝，不把模拟器记为真机。入口只安装已签名测试HAP，运行`aa test`并要求可信完成计数；未签名包不会强行安装。签名材料和目标设备信任仍是独立前置条件，不因下载源改为镜像而取消。

## 用例与未验边界

[正式本人导出](../../../项目相关报告/测试/前端/HarmonyOS/20261116-正式本人导出/README.md)新增[RunExport](./Unit/RunExport.mjs)和[18组目标源码](./Unit/Export.test.cjs)：首轮15组／纯类型通过；后补流刷新／文件保存适配与页面生命周期输入后，4GiB门禁拦在执行前。Python SQLite只验证实际ExportStore原单／CAS／物理UUID逻辑，RCP为注入；新Node文件／系统选择器适配2项未执行，更不代表Harmony NAPI、真实系统提供方、HTTPS或保存成功。新增5 Hypium纯规则使当前源共42方法，本切片尚未构建应用／测试HAP；旧37编译结果保持原义。

外置Hypium当前29项：原25项可信连接、演示／正式、账户设备与恢复代次隔离，pending本人入站筛选，只读群、原群资格代际，大整数游标、恢复会话核对，认证用途、原发送准入、收藏／搜索、结构化提及、公告和转发规则；新增完整凭据对编码、精确refresh代际、新设备空间隔离和初始关联闭包4项。这里只证明测试源码原生编译且29方法名进入HAP；未执行设备Hypium，测试包不调用原生认证协调器／安全库／RDB。

[主机规则入口](./【MacOS】运行规则测试.command/README.md)使用同镜像SDK已有Node与TypeScript转译当前自有纯规则，Node内置test实际25/25通过，记录见本轮证据；它不执行原生UI、RDB、通知、文件预览或AssetStore。

[认证主机入口](./Unit/RunAuth.mjs)使用同镜像Node执行，源码以自身位置解析仓库；构建中间产物写入用户持久目录`Application Support/JobsIM/Build/Harmony`。该轮41/41包括原25规则、AuthCoordinator单次刷新／未知不重用旧RT／安全库失败／原初始载荷／完整闭包／晚失败／退出／冷启动交接11项，以及JSON／媒体／头像共用刷新、旧绑定拒绝、晚响应隔离、transport未知不重发、公有认证不附AT5项。RCP、AssetStore和RDB均为注入替身，不能外推原生服务通过。

[同步主机入口](./Unit/RunSync.mjs)同样只使用已有镜像Node／TypeScript及系统Python SQLite，直接执行实际SyncProtocol／Engine／Store转译结果；私有SQLite WAL／FULL由[桥接器](./Unit/SyncSqlite.py)提供，不连接任何JobsIM或Postgres。原结果、namespace／guard、终态清理和全页事务反例见[24项源码](./Unit/Sync.test.cjs)。临时数据库不会回灌正式RDB或作为设备证明。

本轮新增已实现／已编译的联系人、群管理、历史分页／前台同步、草稿／本机未读、原生凭据／设备会话、头像原文件原操作、举报及鉴权附件打开，详见[工程当前能力](../../../代码/前端/HarmonyOS/README.md)。其真实鸿蒙网络联调、原结果丢响应、权限／恢复竞态和UI仍未设备验证；不再把这些已落盘组写成尚无源码，也不把它们写成验收完成。

剩余I和设备门禁包括：完整本人导出／受控导入迁移、收讫资格ACK与清云、容量／无障碍、所有十语／三主题／字号与API12设备组合、系统跟随／权限、RDB耐久、AssetStore锁屏及重启／卸载、通知和媒体预览／播放。签名安装、模拟器与真机均未验；前台轮询不是后台厂商推送，本机未读不是跨端已读。完整243项不据测试数量或HAP构建标为通过。

[原发信资格补齐报告](../../../项目相关报告/测试/前端/HarmonyOS/20261115-原发信资格/README.md)：应用／13用例测试HAP构建与12项主机规则通过；设备及实际鸿蒙HTTP仍未执行。


最新③切片见[提及公告与分页报告](../../../项目相关报告/测试/前端/HarmonyOS/20261115-提及公告与分页/README.md)。22项Hypium仍是已编译未设备执行；HDC只读枚举为`[Empty]`，未签名包未安装，未进行ArkUI截图或鸿蒙真实HTTPS联调。旧13用例HAP及旧报告原样保留，不把新的22用例源码覆盖成旧证据的实际执行结果。

[原角色资格专项](../../../项目相关报告/测试/前端/HarmonyOS/20261115-原角色资格/README.md)：23项主机规则执行、应用及23用例测试HAP编译通过；设备与真实鸿蒙HTTP未验。

[普通转发限定证据](../../../项目相关报告/测试/前端/HarmonyOS/20261115-普通转发/README.md)：25项主机规则及应用／25用例测试HAP构建通过，设备与真实鸿蒙HTTPS仍未执行。


本轮追加能力撤回与恢复代际反例：旧原单允许匿名纯查询；全部续跑必须重新通过能力门禁，404 未建立原单须由原家族 AT 创建。更新验证为鸿蒙15项主机通过、两端应用与测试包重新编译；Android新增第5项 Room 反例仅已编译，未执行设备测试。


新增[注销host入口](./Unit/RunDeletion.mjs/README.md)与[12项反例](./Unit/Deletion.test.cjs/README.md)，实际生产service＋SQLite桥接通过；认证host现45项、改密host16项通过。Hypium增加注销／拒绝码4项，总50方法仅真实编译并在ABC核名，未执行设备。见[注销冻结报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS注销.md)。


新增[群控主机入口](./Unit/RunGroup.mjs/README.md)与[协议／RDB反例](./Unit/Group.test.cjs/README.md)，当前20项通过；注销12项增加群控原责任精确账号清理反例仍通过。Hypium追加群控4项，当前54方法测试HAP真实编译，设备未执行。来源快照、首轮ArkTS Record字面量失败及修后包见[群控报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS群控.md)，不把主机适配算原生设备验证。

2026-11-17后增取消门禁源码：unknown取消先纯查询，同原cancelVersion才允许用新证明明确续同nonce；旧缺版本记录与查询新版都保持只读，不静默补资格。十语新增先查／冻结说明。新增Harmony4主机反例（目标16）、Android4 Room方法（目标9）及Hypium3方法（目标57）未运行／未编译，受低于4GiB磁盘门禁限制；旧APK/HAP及先前12主机结果不追认该增量。见`项目相关报告/实现进度/20261116-剩余能力闭环/注销证据/取消原版本新增源码待验.json`。


2026-11-17普通个人历史迁移已新增源码入口、原请求耐久、配对／发送／只查／接收／撤包、独立可读历史与原件保存；40键×10语言沿现有三主题。当前新增UI及文件／global清理尚未编译或设备运行；Android首次33 JVM只覆盖其归档的前期草稿和录音5项，新迁移6 JVM／Harmony13 host均待执行。原APK/HAP不追认新源码。协议、文件位置及分层验证见[两端普通迁移报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS普通迁移.md)。

取消增量另有[Harmony16项实际host](../../../项目相关报告/实现进度/20261116-剩余能力闭环/注销证据/HarmonyHost-fMtui1/结果.json)；该轮输入冻结不包含后续迁移表清理增量，Android9 Room／Hypium57仍未在新包执行。


当前迁移／逐store收讫最终：Android150输入、54 JVM通过，双APK9660aa38／7b3f14b4构建并设备读回一致；105仪器逐项核齐，本地102为86通过／16明确Assume跳过，同包另3真实HTTPS通过（S2／草稿／UI），0失败。迁移3、注销9和Receipt真实Room／文件4亦独立16／16通过。Harmony迁移22／receipt23／sync36 host，70输入双HAP5ecddaa4／7f71c4d5成功，63 Hypium仅编译。修复可信OS root别名、同physical probe与迟到global，移动端按本次bytes＋64MiB reserve判断；宿主4GiB门禁保持。默认不开自动收讫、不授生产资格，未知只查原单；真实迁移／receipt HTTPS、管理员矩阵、物理断电及Harmony设备另验。详见[逐store限定报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS逐store收讫.md)。
