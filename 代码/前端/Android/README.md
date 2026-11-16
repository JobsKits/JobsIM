# <span id="前言">JobsIM Android 原生工程</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前能力](#jobs-toc-heading-1)
- [二、构建与运行](#jobs-toc-heading-2)
- [三、服务与数据](#jobs-toc-heading-3)
  - [3.1、权威同步限定验证](#jobs-toc-heading-4)
- [四、验证边界](#jobs-toc-heading-5)
- [五、本人状态与云草稿接入（本轮候选）](#self-state)
- [六、正式本人导出（本轮候选）](#personal-export)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、当前能力 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[**Kotlin**](https://kotlinlang.org/)、[**Jetpack Compose**](https://developer.android.com/compose) 与 [**Room**](https://developer.android.com/training/data-storage/room) 原生客户端，采用暖白／墨黑和松绿。连接服务器与本地演示均有入口，设置完整提供十语、白天／黑夜／跟随系统和三档字号。

| 入口 | 当前操作与结果 |
| --- | --- |
| 连接与账号 | 核对部署 ID、导入实例 CA／比对指纹，注册／登录／MFA 挑战、恢复密码；AT／RT整对原子密封、单航班刷新；未知认证原请求恢复或显式重新验证，冷启动重新核验实例及设备空间 |
| 联系人 | 搜索、添加；只展示本人收到的 pending 申请，显示当前申请人并接受／拒绝；本人关系详情支持删除、拉黑／解黑及显式重新开放本人入站申请，解黑不会自动重开 |
| 会话与群 | 单聊／建群、成员列表、群名、加人、按原 membershipGeneration 移除／转让／退群；只读群禁新写，权限仍由后端重验 |
| 消息与历史 | 文字与文件选择上传、回复、编辑、撤回、反应、置顶、收藏；服务端搜索／本人收藏支持原样不透明游标翻页，选择结果重新鉴权会话和原消息并精确定位；每轮每会话最多读取 10 页、每页 200，持久游标后续继续；前台约每 5 秒轮询 |
| 引用式转发 | 精确 GET 重新鉴权来源，正文版本已变时展示当前原文并要求重新选择；目标会话重新取原准入，七字段原 body 先入 Room。未知先查原结果，404 仍表示未知，只有明确点击才沿原 ID／原 proof 重试。源失效显示占位，不复制纯文本伪装转发 |
| 群协作 | 群内选择当前成员提及，群主可明确 @所有人；提及草稿持久化。群公告引用实时消息正文，支持不过期／一天／七天、发布／清除与未知结果原操作核对；正文变更后不显示旧公告内容 |
| 本地状态与本人同步 | Room 原子保存消息／游标／未读与待发，原 clientMsgId 重试；每会话持久草稿。S2仅观察实际可见 ID、独立手动未读／会话显示／字号与云草稿接线见[本轮候选](#self-state)；旧S1仅本机读数。刷新不回滚较新消息或新输入 |
| 头像与举报 | 静态 PNG／JPEG 头像先保存原文件、原 requestId 和版本，再发布／按原操作查询重试；历史结果 current=false 时重新取本人资料；消息／用户／会话举报及本人报告列表 |
| 附件与设备 | 附件经当前鉴权下载，可交给系统应用打开或通过系统 Save As 另存；查看并撤销设备、生成本人恢复码 |
| 正式本人导出 | 设置→独立任务页，原创建／取消body耐久、纯查单与明确重试、RAM短票及私有ZIP→系统CreateDocument；本轮仅构建／16 JVM通过，5原生方法仅编译，系统保存／HTTPS待验 |
| 通知 | 默认关闭，可经系统权限授权开启进程内本地通知；通知固定隐藏会话名和消息正文，不依赖第三方推送 |

这是第一阶段的端侧实现切片，不代表完整 I 五端或 243 项验收。客户端有界读取附件最多 64 MiB、头像 2 MiB，服务端仍可按实际策略拒绝；系统查看器承担媒体查看，不声称内置播放器／录音录像、服务器消息事件推送、后台远程通知、RTC、地图、支付、产品 SDK 或 E2EE。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、构建与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

源码锁使用 AGP `9.3.1`、Gradle `9.5.0`、Kotlin Compose Compiler `2.4.20`、Compose BOM `2026.09.00`、Room `2.8.5` 与 sqlite-bundled `2.7.1`，minSdk26／target37。当前机器复用了用户级 Android SDK/API37、JDK21 和 Gradle；源码与安装包不包含 SDK。此构建锁与需求内后续目标 AGP9.4／Gradle9.6分别记录，不能称已验同版。

```sh
./gradlew :app:assembleDebug :app:testDebugUnitTest
```

Android Studio 打开本目录，选择已准备的 API26+ 设备或模拟器。SDK 从 `ANDROID_HOME`／`ANDROID_SDK_ROOT` 或 IDE 的本机配置取得；不要把本机路径提交至源码。Gradle 下载缓存位于用户级目录，`.gradle/` 是工具必要的本工程元数据，`app/build/` 为可再生产物；测试入口会把 APK 导出到 `Dist/JobsIM-debug.apk`。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、服务与数据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

先从可信部署终端取得 HTTPS 地址、部署 ID 与证书／CA 信息，点击“连接服务器”核对身份后注册或登录。禁止 HTTP、userinfo、秘密查询参数和跨域重定向。局域网本地 CA 可通过系统文件选择器导入，再与可信部署终端比对展示的 SHA256 指纹并明确确认；信任只影响当前实例的 `SSLContext`，仍校验主机名、SAN 和到期时间，不修改系统信任。

会话令牌不进入 Room、普通偏好或日志，使用 Android Keystore AES-GCM 密封到排除备份的私有目录；冷启动先验证原部署 ID／restoreEpoch，再以 /auth/context 和 /me 恢复本人身份；验证失败不会进入另一账号。正式与演示分别使用 `JobsIM-live.db`／`JobsIM-demo.db`，以可信连接档案、部署及账户隔离；演示消息不进入正式 Outbox。正式写入先落 pending，并持久绑定创建时 restoreEpoch、本人 membershipGeneration 和不透明 sendAdmission，再用原 clientMsgId／原完整 payload 提交；超时保留待发。新意图仅取可信 Conversation 快照，旧待发绝不补最新资格；先验证 bootstrap.send_admission=true，避免向旧服务提交缺快照队列。SEND_ADMISSION_CHANGED 永久冻结原行，不换 ID 或 token 自动重发；已提交原 ID 只按服务端当前读取资格对账。bootstrap恢复代际改变时冻结旧待发、隔离旧缓存并要求重新认证，不换ID补发。已验认证 APK 为 Room schema8；当前同步源码为 schema9（构建及独立V8→9真实迁移通过）。原迁移链：1→2 添加原 epoch，2→3 添加字符串序号／不可用占位、会话状态与原操作表，3→4 添加草稿修订号，4→5 添加原成员代际／sendAdmission（旧行留空），5→6 添加结构化提及／提及草稿和独立正文版本（旧正文版本留空兼容读取），6→7 添加引用式转发及原完整意图字段（旧行不补凭据），7→8 添加 native_scopes 原子设备空间交接及晚到旧写入冻结；新源码8→9添加同步记录、消息持久guard、按对象ID清理索引并遮蔽旧received forward正文，不改原pending／frozen payload；不使用破坏性迁移。真实 schema2→8 升级、数据库关闭重开、超大字符串游标／版本、原消息回执对账及未读去重已有独立模拟器验证；完整灾备恢复仍须专项演练。Room 使用 bundled SQLite、WAL 与 `synchronous=FULL`；这是实现配置，断电、完整设备收讫与清云资格仍须专项验收。


原生刷新须服务端 `auth_refresh=true`。完整 AT／RT／family／字符串代际／期限／来源／设备／实例绑定以 Keystore AES-GCM 密封并通过 AtomicFile 同步、原子替换；仅安全库和 Room 设备空间交接都完成后发布凭据。普通偏好只持非秘密原认证 requestId／用户名／前序链，以及刷新 unknown／提交回执，不存密码、OTP或秘密摘要。只有业务401 `ACCESS_EXPIRED`触发同档案单航班刷新；原业务完整 body／操作ID不改。刷新丢响应、503／解析未知或安全保存失败为持久 unknown，不自动重用旧RT。显式重新验证输入当前证明，新的关联意图冻结完整 `previousRequestIds`；多层退休须实时 `auth_retirement_chain=true`，已发原body不补字段，不凭本机经过300秒改nonce。

新协议空间绑定 profile／部署／user／personal来源／epoch／device(session)。刷新保持同device空间；新认证或重新验证换device，事务冻结旧Outbox，晚到旧空间enqueue也只能冻结并保留原payload及草稿，不自动搬移或发送。安全库已存但数据库交接未完可在重启后凭提交回执续接。旧单AT不升级成RT；无刷新能力的旧服务只保留原AT只读／显式退出及登录，已有新协议未知意图不能降级为旧登录绕过。新信任实例必须重新核对自己的CA／部署身份，不复用已清理环境的凭据。

转发新写须实时 `forwarding=true` 及 `send_admission=true`；旧缺原 body／proof 的记录冻结，不补最新资格。明确成员／历史读取拒绝、源／资格版本变化永久冻结；503 保留未知。打开会话、刷新和重回前台时精确重投影缓存的转发消息，暂时无法核验也清正文／URL，不能只依赖消息事件；不可用来源的本机缓存附件一并清理。媒体只从当前实例同主机／端口的固定 `/api/messages/{forwardMsgId}/attachment` 鉴权路径读取，不暴露源 attachmentId。正式本人导出入口见[本轮候选](#personal-export)，forward只出占位，不提供临时source proof导出；当前构建不代表真实交付已验。

搜索／收藏游标绑定原账户、epoch、查询与会话条件；切换条件清空旧分页，晚到回包不能串入新账户。结果选择走精确 GET，不依赖会话目录前 100 条；退出后重入只能看到新资格允许的原文，拒绝时清除对应旧预览。群公告只保存原 requestId／原版本／成员与角色代际再写入；角色反复转让不会给旧待发补新资格，QUALIFICATION_CHANGED 永久冻结。@所有人和公告新写须实时 role_admission=true；旧服务或旧缺角色快照只能只读核对原结果。

演示为用户明确进入的独立空间，不把真实接口异常伪装为已送达。网络故障保留正式历史和 Outbox；恢复后重新加载服务端数据。切换实例会隔离上下文；后台权限始终由服务端判定，客户端动作失败不会先伪改正式状态。

接口唯一来源：[后端契约](../../后端/Go/接口契约/README.md)。组件定位：[代码地图](./代码地图/README.md)。测试源码与运行入口：[Android 测试](../../../测试/前端/Android/README.md)。

### <span id="jobs-toc-heading-4" data-jobs-toc="v2">3.1、权威同步限定验证</span><!-- jobs-toc:heading-end -->

当前源码为 schema9 与原生 snapshot／changes 消费；本轮独立产品包SHA为978b9f9c54e4fa22b8adbc92211610f10cb8238bf297c83bfa4517de922f7d08，仅归档在中央构建子项。下方canonical e295认证APK保持原字节。页面进入、刷新及原有五秒轮询，在实时 `authoritative_sync=true` 或本空间已持有同步责任时走新协议；缺能力、缺页、未知投影或身份变化不会回到旧 history 轮询。未完成首次安装前隐藏 cloud 视图，用户可用原刷新入口重试。真正无同步责任的旧服务保留既有只读路径，显式演示空间独立。

Room 的 `sync_records` 按 profile／部署／本人／personal来源／epoch／server device 隔离登记原 storeGeneration、登记 requestId、原 snapshot request、完整 manifest、页引用与原始 envelope／SHA、origin 去重、连续 cursor、分类水位和持久 guard。分页完整核验、final validate 及首批 changes 当前权限确认后，当前消息、对象投影、未读、原 Outbox 查单证明与 cursor 在同一 Room 事务提交。后续增量每轮至多500条连续事件，再沿原 cursor 继续；这不是合格设备 ACK，`qualifiedAck`／`after_receipt` 仍关闭。

未知保留原 requestId、暂存引用及隔离区旧普通正文，不把缺项当删除；stage 不存正文，受控 forward 正文与 proof 只在 RAM 精确 GET 后展示。schema8→9 同时遮蔽旧 received forward 正文，保留原待发 payload。明确 global 在本 profile／部署／本人下遍历当前、旧 device／epoch 与 retained holder 清正文／附件缓存，并永久拒绝可用回包复活；临时不可用仅为占位。普通发送的成功响应和只读原完整 body＋route `message-results` 才可提供 ACK 证明，Room 提交时再比原意图；同步见到相同 opaque clientMsgId（UTF8 8–128字节，非 UUID）不能消队列。辅助查单503／网络未知／不完整200为零证明，不阻塞已验证 global 提交，也不创建消息。global另在同事务持久排队清理，启动及后续空增量仍续清本机媒体／通知；文件删除失败保留意图，retained映回原缓存路径。未知Outbox的原始body不因无关联的global被改写或虚称擦除。

本轮应用／测试APK构建通过，JVM10项（新增4＋原6）通过；仪器10项（8同步＋1旧Api身份＋1可靠原schema8迁移）通过，均无失败／跳过。V8原件三方SHA一致，使用独立UUID数据库验证原列／原payload逐字保留、received forward遮蔽及无同步资格假继承。自有事务统一由同协程BundledSQLite IMMEDIATE writer承载，不回退系统driver。完整真实HTTPS、高容量、UI及平台组合仍待验证。[本轮检查点与原认证输入归档](../../../项目相关报告/测试/前端/Android/20261116-权威同步/README.md)单独记录，不将旧6＋10结果套用到新源码。服务端 read／cloud drafts／偏好、合格 ACK、正式导出／账号注销／端迁移仍为 I 阶段未闭环项。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">四、验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

当前 canonical `Dist/JobsIM-debug.apk` 已更新为访问刷新通过包，独立基线为 `Dist/Baselines/20261115-native-auth-refresh-debug.apk`，SHA256 `e295688485e0caa73932670e48da227eae804108416625b62fba5b0e01cee8b3`；访问刷新当时安装读回相同。本轮专用模拟器另安装978中间包，两包与独立报告逐字节匹配。认证历史为6 JVM＋10仪器，旧转发／搜索等报告各对应自己的独立基线，计数不跨轮相加。访问／刷新家族证据见 [访问刷新限定报告](../../../项目相关报告/测试/前端/Android/20261115-访问刷新/README.md)。引用式转发及当前受影响回归见 [引用转发限定证据](../../../项目相关报告/测试/前端/Android/20261115-引用转发/README.md)。此前搜索／收藏、提及／公告及角色资格围栏的定向构建见 [搜索协作限定证据](../../../项目相关报告/测试/前端/Android/20261115-搜索协作/README.md)。既有发送准入围栏的构建及回归见 [发送准入限定证据](../../../项目相关报告/测试/前端/Android/20261115-发送准入围栏/README.md)，其前一轮原生功能及截图保存在 [基础闭环基线](../../../项目相关报告/测试/前端/Android/20261115-基础闭环/README.md)。这些历史记录绑定各自归档输入和 APK 指纹；后续同步源码不追认旧包验证，原报告保留其当时适用范围。未真机验收、未签名发布；完整账号删除／派生源生命周期、合格设备 ACK、清云、受控迁移／导出及安全恢复尚未闭环。媒体原文件上传丢响应的对账协议尚未提供；本地缓存不等同于权威备份，后台进程退出后轮询与本地通知停止。

官方依据：[AGP9.3 支持矩阵](https://developer.android.com/build/releases/agp-9-3-0-release-notes)、[Room](https://developer.android.com/jetpack/androidx/releases/room)。设计采用记录见 [PRODUCT](./PRODUCT.md) 与 [DESIGN](./DESIGN.md)。

## 五、<span id="self-state">本人状态与云草稿接入（本轮候选）</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本轮源码继续使用 Room9 的 `sync_records`，没有改写旧 schema9 或借协议升级重建数据库。S2 需要实时 `sync_self_state=true`，登记明确携带 `schemaVersion:'2'`，响应、coverage、manifest 与 sync owner 水位逐项核验；同一个物理 storeGeneration 不变，原 S1 control 独立归档，S2 使用独立登记 nonce／stream／cursor。已有 S2 责任后能力缺失会限制云视图，不退回 S1 或旧 poll。没有该标识的旧服务保持 S1，不向固定旧实例探测登记 S2。

| 原生入口 | 默认与行为 | 持久化／生命周期及源码 |
| --- | --- | --- |
| 会话“我的同步状态” | 仅本人阅读进度、服务端精确未读及本机缓存计数，明确分开；旧成员读点显示为保留 metadata | `SelfState.kt` 严格字符串 counter／principal／来源／epoch；投影和连续 cursor 同 Room writer 事务 |
| 屏幕真实可见消息 | 停留后仅提交当前可读真实 ID，不以最大序号宣告整段已读；未见洞和手动未读不被清除 | `MainActivity.kt` 可见列表观察；`SelfStateModel.kt` 按成员 generation 冻结原观察，原结果确认后只移除相应本机未读 ID |
| 会话显示操作与列表筛选 | 手动未读单独 generation；置顶／一天静音／归档／隐藏；列表保留显示归档和隐藏会话入口 | view CAS 原请求入 `self-intent`；静音实际抑制本地通知；隐藏／归档不改变正文权限 |
| 设置字号 | 小／标准／大接本人偏好；主题／语言仍本机，不宣称云同步 | 活跃 S2 用当前 GET／CAS；失联保原意图；未认证及旧服务沿用本机偏好，不自动上传旧值 |
| 云草稿面板 | 另需 `cloud_drafts=true`；显式保存、载入、清除及独立文字恢复，不自动覆盖编辑框／自动发送 | 原独立文字、CAS、da1 与 dr1 原许可入原意图；草稿投影仅引用，不存 quote／临时许可。载入时精确核原消息，输入修订变化拒迟到替换；正式消息 Outbox 不改 |
| 待确认操作 | 首次先耐久保存原 route／method／body／nonce；未知先原 body 只读 `*-results` | 404 不当成功，只有用户明确操作且查无结果才同 ID／原体重试；CAS／资格拒绝冻结原记录；`current=false` 不覆盖当前值，之后精确 GET 当前。reset／切账号／实例／成员／epoch 有围栏 |

本轮实现与验证记录见[独立 S2 报告](../../../项目相关报告/测试/前端/Android/20261116-S2本人状态/README.md)。本轮产品5774已编译，JVM11／同包原生23／新E2真实HTTPS与UI3分别通过，六个新family明确关闭。5张原帧已查看，过渡帧不作为稳定终态视觉证明；物理设备、全部语言／主题组合、完整 I 及 qualified ACK 仍未验收。严格支持RFC3339数值时区但不改原body字符串；旧认证／S1报告不追认新源码。

## 六、<span id="personal-export">正式本人导出（本轮候选）</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

设置页增加原生“导出本人数据”入口和独立任务页面，继承十语、三主题、字号及系统返回。列表明确只展示本机这份Room空间记录；完整空态与重载始终可达，不用本地JSON或演示ZIP替代正式包。创建前实时核实例／epoch／`chat_export`，原create与独立cancel body／nonce／SHA先入Room9 `sync_records`，绑定当前family／device／已有物理storeGeneration。未知只读原完整body查单；只有用户明确重试且查无结果才原ID同体提交，不改正式消息Outbox。能力撤回仍可核对与取消原任务，禁止新建／下载。

`SessionApi`与`SyncGuardApi`的新增流式下载保持原认证／身份围栏，AT／RT与下载票不进Room，票只走RAM header。ZIP写入排除系统备份的私有临时目录，最多1GiB、目录最多32MiB；完整目录校验及文件／父目录fsync后才调用系统CreateDocument。保存前再次重验当前权限、私有文件长度／SHA与完整ZIP，再写所选描述符并同步。取消选择明确“未保存”，失败尝试删除本轮新文档并提示检查不完整文件，不冒充外部存储或断电验收。外部已保存ZIP不受后续服务器清理控制。

关闭页面／切换身份会中止流和清本轮私有文件；进程重启只清本模块自有命名的孤儿暂存，不删除用户外部文件，当前活跃文件受进程内登记保护。本轮主／测试包构建及16 JVM（含5新导出反例）通过，5原生注入方法仅编译；App1588c450与test76826edf只在[独立报告](../../../项目相关报告/测试/前端/Android/20261116-正式本人导出/README.md)归档，未替换e295／5774旧证据。未安装或启动AVD，系统保存／目录fsync及真实HTTPS仍待验，完整I与243项状态未改。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
