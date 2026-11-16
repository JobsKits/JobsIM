# <span id="前言">Android 代码地图</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

| 入口 | 职责 |
| --- | --- |
| [MainActivity.kt](../app/src/main/java/com/jobs/im/MainActivity.kt) | Compose 页面、导航、身份／连接表单、消息动作、原生附件选择、主题与字号 |
| [ChatModel.kt](../app/src/main/java/com/jobs/im/ChatModel.kt) | API 流程、账号代际守卫、分页轮询／草稿未读、原子可见列表、原 ID Outbox／头像／举报重试、引用式转发与权威重投影 |
| [Store.kt](../app/src/main/java/com/jobs/im/Store.kt) | schema9／可靠原件8→9迁移已验（旧认证APK为8），同协程driver writer／增量迁移、bundled SQLite/WAL/FULL、消息游标事务、草稿修订／结构化提及、原操作及发送准入冻结、连接／账号／epoch／演示隔离 |
| [SyncProtocol.kt](../app/src/main/java/com/jobs/im/SyncProtocol.kt) | 原始 ref／body／event bytes SHA、完整页／版本与绑定验证、连续增量／redacted闭区间、opaque client ID；基础4JVM及原生注入通过，真实HTTPS待验 |
| [SyncStore.kt](../app/src/main/java/com/jobs/im/SyncStore.kt) | Room 登记／stage／retained／global终态／持久guard，原完整请求 proof CAS 与消息／cursor 同事务；原生注入／V8迁移通过，真实HTTPS待验 |
| [SyncEngine.kt](../app/src/main/java/com/jobs/im/SyncEngine.kt) | 原 snapshot nonce恢复、全页／final validate／首次changes、只读Outbox查单与失败隔离；原生注入通过，真实HTTPS待验 |
| [SelfState.kt](../app/src/main/java/com/jobs/im/SelfState.kt) | S2 本人 preferences／view／read／稀疏 observation／draft 严格 DTO、原意图与结果证明、和 cursor 同 Room writer 的投影；本轮候选待验证 |
| [SelfStateModel.kt](../app/src/main/java/com/jobs/im/SelfStateModel.kt) | 新本人操作前实时能力门禁、原 CAS／成员代际／nonce、未知只读查单和显式原体重试、同身份 reset／迟到响应围栏；云草稿原 da1／dr1 捕获 |
| [SelfStateUi.kt](../app/src/main/java/com/jobs/im/SelfStateUi.kt) | 原生同步状态面板、精确／本机计数分离、独立手动未读与 view 操作、草稿显式载入及恢复 |
| [SelfStateStrings.kt](../app/src/main/java/com/jobs/im/SelfStateStrings.kt) | 本人状态／草稿新增十语词条；沿用三主题与系统无障碍字号 |
| [PersonalExportWire.kt](../app/src/main/java/com/jobs/im/PersonalExportWire.kt)／[Store](../app/src/main/java/com/jobs/im/PersonalExportStore.kt) | 正式导出严格metadata／原create与cancel body／SHA，Room9原family／device／物理UUID事务围栏；无token／票／ZIP入库 |
| [PersonalExportModel.kt](../app/src/main/java/com/jobs/im/PersonalExportModel.kt) | 六API、原单只读核对／明确原体重试、能力撤回取消收尾、RAM短票与保存前重验、迟到身份拒绝 |
| [PersonalExportFiles.kt](../app/src/main/java/com/jobs/im/PersonalExportFiles.kt) | 鉴权流式私有ZIP、1GiB／32MiB目录限制、完整目录／SHA、file与parent fsync、取消断流及自有孤儿清理 |
| [PersonalExportScreen.kt](../app/src/main/java/com/jobs/im/PersonalExportScreen.kt)／[Strings](../app/src/main/java/com/jobs/im/PersonalExportStrings.kt) | 原生任务页、空态／重载／取消确认、系统CreateDocument，取消选择不冒充保存；十语三主题，当前验证见独立报告 |
| [SyncGuardApi.kt](../app/src/main/java/com/jobs/im/SyncGuardApi.kt) | 新同步责任禁止旧poll、请求前后持久guard与生命周期绑定、精确消息写缓存marker；原生注入通过，完整App生命周期待验 |
| [SyncCleanup.kt](../app/src/main/java/com/jobs/im/SyncCleanup.kt) | 已验证global的耐久媒体清理；retained映回原缓存namespace，失败保留清理意图，重启／空增量续清；原生受控中断／清理重试通过 |
| [Transport.kt](../app/src/main/java/com/jobs/im/Transport.kt) | HTTPS JSON／multipart、禁止重定向、实例 CA 与 Keystore 整对凭据原子密封 |
| [NativeSession.kt](../app/src/main/java/com/jobs/im/NativeSession.kt) | 严格会话绑定、原认证非秘密意图与完整前序链、unknown及安全提交回执 |
| [SessionCoordinator.kt](../app/src/main/java/com/jobs/im/SessionCoordinator.kt) | 档案级单航班旋转、持久保存先于发布、迟到身份围栏、原业务body重试、旧协议只读 |
| [SessionStrings.kt](../app/src/main/java/com/jobs/im/SessionStrings.kt) | 原认证重试／重新验证入口的完整十语词条 |
| [Domain.kt](../app/src/main/java/com/jobs/im/Domain.kt) | 会话／成员／请求／本人关系／报告数据与字符串序号消息解析 |
| [ManagementScreens.kt](../app/src/main/java/com/jobs/im/ManagementScreens.kt) | 请求收件箱、本人关系、群治理、头像／举报／设备原生面板与完整空态 |
| [MessageDiscovery.kt](../app/src/main/java/com/jobs/im/MessageDiscovery.kt) | 服务端搜索／收藏游标、查询与身份围栏、失效原文清理 |
| [DiscoveryScreens.kt](../app/src/main/java/com/jobs/im/DiscoveryScreens.kt) | 原生搜索／收藏分页、空态／重载、精确打开原文 |
| [AnnouncementModel.kt](../app/src/main/java/com/jobs/im/AnnouncementModel.kt) | 公告实时源投影、原操作持久化／只读核对、成员与角色资格快照 |
| [CollaborationScreens.kt](../app/src/main/java/com/jobs/im/CollaborationScreens.kt) | 当前成员提及、公告源选择／有效期／发布／清除 |
| [FeatureStrings.kt](../app/src/main/java/com/jobs/im/FeatureStrings.kt) | 搜索与群协作新增十语词条 |
| [Forwarding.kt](../app/src/main/java/com/jobs/im/Forwarding.kt) | 原转发 body 校验、明确资格拒绝、不可用投影清理与同源固定附件路径 |
| [ForwardScreens.kt](../app/src/main/java/com/jobs/im/ForwardScreens.kt) | 原生来源预览／更新后重选与目标会话选择 |
| [Assets.kt](../app/src/main/java/com/jobs/im/Assets.kt) | 有界文件读取与真实图片格式识别 |
| [LocalAlerts.kt](../app/src/main/java/com/jobs/im/LocalAlerts.kt) | 默认关闭的本地通知、权限／命名空间门禁与隐藏正文 |
| [ProfileAvatar.kt](../app/src/main/java/com/jobs/im/ProfileAvatar.kt) | 本地 Logo 兜底与鉴权头像异步显示 |
| [ExtraStrings.kt](../app/src/main/java/com/jobs/im/ExtraStrings.kt) | 管理／隐私／错误状态新增十语词条 |
| [Strings.kt](../app/src/main/java/com/jobs/im/Strings.kt) | 十语控件词条与完整性检查 |
| [构建配置](../app/build.gradle.kts) | SDK、依赖、Compose/KSP 和外置 Unit／Instrumentation 源目录 |
| [外置测试](../../../../测试/前端/Android/README.md) | 逻辑、Room 隔离、模拟器／真机运行与证据 |

范围和限制见[工程 README](../README.md)。不在此维护后端或其它端源码地图。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
