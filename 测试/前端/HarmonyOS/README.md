# JobsIM 鸿蒙原生测试

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [当前结果](#jobs-toc-heading-1)
- [使用](#jobs-toc-heading-2)
- [用例与未验边界](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">当前结果</span><!-- jobs-toc:heading-end -->

当前S2切片35／35主机反例（25基础＋10 S2，不累加旧24／25轮）、41／41旧规则通过；最终应用和37方法源码测试HAP编译通过，未运行Hypium／安装设备／实际鸿蒙HTTPS。[S2证据](../../../项目相关报告/测试/前端/HarmonyOS/20261116-S2本人状态/README.md)和[当前S2 Dist基线](../../../代码/前端/HarmonyOS/Dist/S2本人状态基线.json)保原S1包与输入。以下S1／认证数字仍为各自历史范围，不追认新源或新包。

2026-11-16 schema1同步切片已通过24项新主机反例与41项旧规则回归，应用及32方法源码测试HAP离线编译通过。32项没有在设备执行；RDB Store主机用真实SQLite适配，不冒充Harmony NAPI。[本轮证据](../../../项目相关报告/测试/前端/HarmonyOS/20261116-权威同步/README.md)保留首轮18／2、两次ArkTS失败和最终输入／产物SHA；当前Dist绑定[同步基线](../../../代码/前端/HarmonyOS/Dist/权威同步基线.json)。下述访问刷新29条／41主机仍为其独立历史范围，旧HAP已另存。

2026-11-15访问刷新切片已通过**真实应用HAP与外置29方法测试HAP构建**，均为未签名包；41项纯主机测试实际通过（原规则25＋认证协调器11＋Api注入5），0失败／跳过。未执行模拟器／真机安装、Hypium或真实鸿蒙HTTPS。工具来自用户指定libsdl镜像：CLT6.1.1.280、SDK6.1.1.125／API24、Hvigor6.24.2、Node18.20.1。29条不能写成设备测试通过。

[访问刷新限定证据](../../../项目相关报告/测试/前端/HarmonyOS/20261115-访问刷新/README.md)保存该轮构建、主机结果、源码与产物摘要，以及首次SDK缺件失败；[认证历史基线](../../../代码/前端/HarmonyOS/Dist/认证刷新基线.json)记录该轮unsigned HAP，原包字节另存本轮上一认证Dist目录。旧转发25规则／25方法HAP及原报告保持独立历史含义。

[本轮提及公告与分页证据](../../../项目相关报告/测试/前端/HarmonyOS/20261115-提及公告与分页/README.md)提供成功日志、源码快照清单及HAP／Ark字节码摘要。更早缺SDK、官网取包和构建失败日志保留供核查，当前SDK阻塞已由镜像解除，不再要求登录华为下载门户。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">使用</span><!-- jobs-toc:heading-end -->

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

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">用例与未验边界</span><!-- jobs-toc:heading-end -->

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
