# HarmonyOS 代码地图

| 入口 | 职责 |
| --- | --- |
| [EntryAbility](../entry/src/main/ets/entryability/EntryAbility.ets) | Stage Ability 与窗口入口 |
| [Index](../entry/src/main/ets/pages/Index.ets) | ArkUI 原生页面、身份与连接、CA核对、聊天／分页来源定位、提及选员、群公告原操作及设置 |
| [Models](../entry/src/main/ets/Data/Models.ets) | HTTP对象、可信连接档案、签名查询路径、原提及／公告意图；完整认证凭据对、非秘密交接对象与精确代际规则 |
| [AuthCoordinator](../entry/src/main/ets/Data/AuthCoordinator.ets) | 实时协议门禁、单次refresh协调、初始原意图及完整退休链、unknown停止、全局安全库变更串行及冷启动交接 |
| [Store](../entry/src/main/ets/Data/Store.ets) | 系统RDB、正式／演示分库、消息／草稿／游标／未读／原操作；写串行、认证公开交接与旧设备Outbox冻结同事务 |
| [SyncProtocol](../entry/src/main/ets/Data/SyncProtocol.ets) | schema1原envelope字节SHA与结构验证、manifest／page／delta、精确计数与完整Message证明 |
| [SyncEngine](../entry/src/main/ets/Data/SyncEngine.ets) | 原注册／快照意图、全页＋validate＋首次changes编排；single-flight／身份guard、只读原体查单，绝不自动创建消息 |
| [SyncStore](../entry/src/main/ets/Data/SyncStore.ets) | 每库物理UUID、同RDB原体／游标／投影事务、终态账本、合法普通副本与跨持有者cleanup意图 |
| [SyncText](../entry/src/main/ets/Data/SyncText.ets) | 同步受限与本地只读入口十语词条 |
| [SelfProtocol](../entry/src/main/ets/Data/SelfProtocol.ets) | S2本人scope／version、读点／视图／字体／独立文本草稿及原结果校验，server op UUID不等于client requestId |
| [SelfState](../entry/src/main/ets/Data/SelfState.ets) | 同RDB原method／route／body／hash意图，fresh da1／dr1、prepared／retired、完成后纯查current、显式本地采纳CAS |
| [SelfText](../entry/src/main/ets/Data/SelfText.ets) | S2入口25控件及操作状态十语，不把协议枚举当用户状态标签 |
| [ExportProtocol](../entry/src/main/ets/Data/ExportProtocol.ets) | 正式本人任务／原单／短票严格解析，安全文件名和有界ZIP目录校验 |
| [ExportStore](../entry/src/main/ets/Data/ExportStore.ets) | RDB原create／cancel body及hash，namespace＋family／物理UUID与更新CAS |
| [ExportService](../entry/src/main/ets/Data/ExportService.ets) | 六API原单纯查、显式同体续、取消降级收尾、当前ready票和保存重核 |
| [ExportFiles](../entry/src/main/ets/Data/ExportFiles.ets) | 私有流文件／SHA／file与parent fsync、DocumentViewPicker.save及取消／失败清理 |
| [ExportText](../entry/src/main/ets/Data/ExportText.ets) | 正式导出41键完整十语、本设备任务与未保存提示 |
| [Api](../entry/src/main/ets/Data/Api.ets) | 严格RCP HTTPS、结构化错误、JSON／媒体／头像同认证协调；原绑定检查拒绝晚响应 |
| [NativeServices](../entry/src/main/ets/Data/NativeServices.ets) | 单条AssetStore完整凭据对与大小门禁、无正文通知与用户显式系统外部预览 |
| [Strings](../entry/src/main/ets/Data/Strings.ets) | 十语控件词条 |
| [项目配置](../build-profile.json5) | 产品目标、模块与构建模式 |
| [模块配置](../entry/src/main/module.json5) | Ability、设备范围、INTERNET 权限和页面资源 |
| [外置测试](../../../../测试/前端/HarmonyOS/README.md) | 原生Hypium源码与HAP、41项实际主机规则、设备模式及未验边界；旧切片计数和证据独立保留 |

只定位本原生工程；当前已取得镜像SDK应用／测试HAP编译证据，设备未验，详见[工程 README](../README.md)。

[正式本人导出切片](../../../../项目相关报告/测试/前端/HarmonyOS/20261116-正式本人导出/README.md)是新原生子视图与独立原单模块；原15项host通过，后补18目标门禁未执行，新5 Hypium／当前42方法尚未新HAP编译。旧HAP不能覆盖本轮，Python SQLite／RCP和文件适配均不是设备NAPI。

[工具链](../工具链/README.md)管理镜像实际配套与用户级缓存准备；构建和测试入口支持ASCII源码快照，当前成功构建与保留的历史失败均见测试记录。


提及与公告没有新增Web容器：仍为同一ArkUI入口的原生子视图，消息来源使用contentVersion，原操作只保存引用。最新[限定证据](../../../../项目相关报告/测试/前端/HarmonyOS/20261115-提及公告与分页/README.md)分别记录主机规则、两类HAP及设备／HTTPS未验，不将本机提及读点当作跨端ACK。

[访问刷新证据](../../../../项目相关报告/测试/前端/HarmonyOS/20261115-访问刷新/README.md)记录同family刷新、新设备空间交接、晚context反例与安全库失败注入；纯主机验证不替代鸿蒙AssetStore、RDB、网络或设备生命周期测试。

[schema1同步证据](../../../../项目相关报告/测试/前端/HarmonyOS/20261116-权威同步/README.md)保留首轮18／2与首两次ArkTS失败、24项新主机／41项回归、应用及32源码测试HAP；无HDC设备或鸿蒙HTTPS通过声明。正式入口为Index前台5秒／刷新→SyncEngine，后台取消；原history路径不会在已有同步责任后回退。

[S2本人状态证据](../../../../项目相关报告/测试/前端/HarmonyOS/20261116-S2本人状态/README.md)封35主机（含S1修后25）、41旧规则、最终应用及37方法测试HAP。Index接设置／会话子页、可见ID显式观察、排序／静音／手动未读和独立草稿输入；SyncEngine自动协商2，SyncStore唯一事务封旧流，不换物理UUID或原Outbox。没有设备／实际鸿蒙HTTPS运行结果；HAP只证明编译。
