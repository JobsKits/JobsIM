# JobsIM 鸿蒙原生工程

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [当前实现](#jobs-toc-heading-1)
- [构建与运行](#jobs-toc-heading-2)
- [信任与数据边界](#jobs-toc-heading-3)
- [权威同步schema1](#jobs-toc-heading-4)
  - [schema2本人状态](#jobs-toc-heading-5)
- [认证刷新与未知结果](#jobs-toc-heading-6)
- [正式本人导出](#jobs-toc-heading-7)
- [尚未验收与剩余I边界](#jobs-toc-heading-8)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">当前实现</span><!-- jobs-toc:heading-end -->

ArkTS／ArkUI Stage＋系统RDB原生客户端，保持暖白、墨黑和松绿；设置包含十语、白天／黑夜／跟随系统与持久字号。当前S2本人状态已接原生页面、实际应用及37方法测试HAP构建；35项同步／本人状态主机反例和41项旧规则通过。没有HDC设备或真实鸿蒙HTTPS结果，不等于完整I验收。

- 联系人：本人pending入站申请与当前requester资料；缺资料显示不可用，不沿用旧响应。接受／拒绝、搜索／建聊、删除、拉黑／解黑、本人显式reopen；原未决请求ID持久保留，解黑不复活旧申请。屏蔽列表明确只是本机操作记录，服务端仍最终判定双向门禁。
- 群：成员与资格代际、改名、添加／移除、转让、退出；可从联系人／成员选择，保留ID入口。移除重试只使用原membershipGeneration；只读群不新发送或提交普通治理写，危险动作核对目标后确认。
- 消息：文字和25MiB内媒体文件上传，回复／编辑／撤回、反应、置顶／收藏；内容不可用占位。正式消息接权威快照／连续增量，当前能力支持时自动schema2，否则无S2责任时保schema1，前台每5秒拉取一页；原history游标与旧副本保留但不再作为正式安装入口。普通聊天徽标仍为本机观察，S2另显示本人服务端读取进度和独立手动未读，不是对方公开已读或收讫资格证明。
- 提及：从当前群成员原生列表选择，群主可选全体；仅文字`@昵称`不取得提及资格。草稿与原Outbox持久mentionedUserIds／mentionAll，发送重试不换ID／原资格。RDB提及回执按原消息ID去重，与权威投影和连续cursor同事务；首次基线先标为已观察，不因旧编辑重新提醒。打开会话清除本机提及未读；后续投影mentionedMe=false／失权／global清理，解黑不复活已观察提醒。不是对方公开已读或qualified ACK。
- 群公告：本群消息可作为来源，当前群主选择无期／1小时／1天／7天，确认前捕获原contentVersion／公告CAS／本人成员代际。完整原请求先持久，再GET原operation，缺失才按原PUT／DELETE body续跑；完成后重新GET当前公告，current=false不回滚。待办仅存消息ID／版本／到期引用，不存来源正文或附件；失效／过期不显示旧正文。明确拒绝的原操作可确认放弃，超时／503等未知结果保持待核。新入口需当前实例mentions／announcements明确支持。
- 搜索／收藏：独立分页接口及原样签名游标，收藏不发空搜索词；打开来源重取当前会话与消息，支持公共会话前100条以外的单项定位。冻结队列可调用只读message-results核对既成事实；404保留原记录且绝不转向创建型POST messages。
- 账号：严格区分登录OTP与账号恢复码；完整AT／RT凭据对绑定可信连接、实例、本人、personal来源、设备和restoreEpoch，仅进单个AssetStore SECRET。冷启动先校验已核对CA与实时auth_refresh，再恢复安全库／RDB交接点并校验auth/context；设备会话列表／撤销及原ID恢复码生成保留。冷启动无法在线核验时不恢复正式身份，不把演示替代正式缓存。
- 头像：本人原JPEG／PNG文件先写私有文件并fsync，持久SHA256、requestId及profileVersion；未知结果先查原操作，不重选文件／新ID；current=false以当前me为准，不恢复旧头像。服务端负责静态格式、2MiB／16MP与转换限制。
- 举报：消息／用户／会话入口、目标理由、原操作持久重试与本人列表。当前后端列表最多100项，没有虚构分页或聊天正文证据接口。
- 附件：每次打开前重新通过当前会话鉴权下载，私有缓存文件使用本地生成名称。显式调用系统外部预览，格式不支持时显示失败，不声称应用内安全播放器或已交出内容可回收。
- 本地通知：用户主动授权／开启，只发JobsIM与十语通用“新消息”，无发送人或正文。仅依赖前台同步时发现的新消息；退后台停止轮询，不冒充远程厂商推送。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">构建与运行</span><!-- jobs-toc:heading-end -->

使用用户指定[libsdl镜像工具链](./工具链/README.md)：CLT6.1.1.280、SDK6.1.1.125／API24、Hvigor6.24.2、Node18.20.1；最低兼容API12。源码只保留配置与锁，SDK、Node、缓存均在工程外，不修改系统全局环境、CA或代理。健康已有缓存复用，不切回官网／其它registry间接补工具。

```sh
../../../测试/前端/HarmonyOS/运行测试.command/运行测试.command check
../../../测试/前端/HarmonyOS/运行测试.command/运行测试.command build
../../../测试/前端/HarmonyOS/运行测试.command/运行测试.command test-build
../../../测试/前端/HarmonyOS/【MacOS】运行规则测试.command/【MacOS】运行规则测试.command
```

官方构建器拒绝中文路径，入口在用户缓存ASCII快照构建，保留源码／暂存SHA256清单；外置测试仍唯一位于`测试/前端/HarmonyOS/OhosTest`，只在快照调整相对引用和镜像内置测试依赖元数据。应用／测试HAP导出`Dist/`，当前均unsigned。签名材料、目标授权及设备执行单独准备，不因更换下载源而豁免。

认证纯规则另有入口，使用同镜像已有Node／TypeScript，不安装依赖：

```sh
"$HOME/Library/Application Support/JobsIM/Toolchains/HarmonyMirror/ArkTS-6.1.1.280/command-line-tools/tool/node/bin/node" ../../../测试/前端/HarmonyOS/Unit/RunAuth.mjs
```

[测试入口与最新证据](../../../测试/前端/HarmonyOS/README.md)区分实际主机规则、HAP构建和未执行的设备用例。没有目标设备结果时，不宣称ArkUI、RDB、AssetStore或通知已实机通过。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">信任与数据边界</span><!-- jobs-toc:heading-end -->

输入的待核服务地址与当前可信地址分离，编辑表单不会携带旧bearer请求新地址。新连接先核对可信部署终端给出的deploymentId及CA指纹；只在当前实例RCP会话使用系统信任或核对后的私有CA，仍校验主机名／有效期，显式禁止自动重定向，不安装全局根证书。冷启动重新核对已存CA字节与指纹后才读取令牌。

正式与演示为独立RDB；消息、草稿和游标按可信连接档案／实例／账户／来源／设备／epoch隔离。成功refresh保留原family、设备和空间；重新本人验证取得新设备时，先在RDB事务冻结旧设备待发，再发布新身份，不自动搬运或提交旧队列。待发持久原clientMsgId和创建时epoch／准入快照，恢复变化冻结原队列，不换ID自动重发。目录资格消失或同步未知时关闭云正文入口，合法普通本地副本隔离保留；只有明确global分类永久清理所有已核同来源持有者。未知不等于cloud_only或全局删除。账号级头像／举报／关系操作另按同可信档案、本人账户和epoch持久，重新认证后可核对原操作，不复用旧会话秘密。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">权威同步schema1</span><!-- jobs-toc:heading-end -->

默认仍为`personal`来源。每个正式RDB另持久独立UUID作为物理storeGeneration，不复用认证设备或旧namespace；注册原requestId在HTTP前写入。当前可信实例必须同时明确`authoritative_sync`和`sync_jetstream`，已有责任后能力消失不会退回旧history轮询。

全部manifest页的原referenceEnvelope／bodyEnvelope／eventEnvelope先核原字节SHA256，再核外层结构、完整页数、精确十进制计数及连续位置。快照最后validate的分类水位不得低于原水位，首次changes权限确认后才在同一RDB事务安装正文、去重事实和连续游标；页面只暂存引用，不暂存正文。当前单轮快照正文设64MiB内存上限，超出明确拒装并保留原意图，没有声称已完成全量容量验收。

| 入口 | 默认与持久化 | 生命周期／边界 |
| --- | --- | --- |
| 前台／刷新／打开会话 | Index→SyncEngine；单轮一页、5秒定时，同轮single-flight | 页面隐藏或Ability退后台取消当前轮并推进本地guard；迟到身份、设备、epoch或guard不发布 |
| 快照及增量 | SyncStore复用LocalStore写串行；同RDB事务保存投影、事件事实、游标 | 原nonce与快照ID可续；缺页、未知投影、权限ABA和原始摘要矛盾均拒装 |
| 待发／核对原结果 | 原route＋完整原body＋SHA独立持久；仅只读message-results | 同clientMsgId从不确认；完整200与当前原体CAS一致才销队列，404／409／坏200／辅助503均零证明；自动同步绝不创建消息 |
| 本地历史 | “本地历史”显式入口；仅同可信身份／来源／epoch已登记合法普通副本 | 标明已离群／本地只读、禁止发送及云媒体；身份或恢复待核保持隔离，转发只存无正文占位 |
| 全局清理 | scope终态账本和缓存／通知清理意图同事务持久 | 跨已核同账户设备／epoch清正文、保留副本和索引；失败保意图，下次启动／同步drain；未知旧结构隔离不提升资格 |
| 搜索／公告／媒体 | 搜索和公告来源只在当前RAM，公告持久投影剔除来源message；附件私有路径 | 精确GET与下载绑定原guard，global不被迟到回调复活；用户已显式交出给系统的文件不声称远程回收 |

[S1独立证据](../../../项目相关报告/测试/前端/HarmonyOS/20261116-权威同步/README.md)保留原24／41、32方法HAP及失败；后续第25项确认owner引用revision独立于Message.version。该兼容修复现已纳入下述S2新HAP，旧包不追认。主机Store用实际SQLite适配器，不是Harmony RDB NAPI；设备及真实鸿蒙HTTPS未验，qualifiedACK与after_receipt仍未实现。

### <span id="jobs-toc-heading-5" data-jobs-toc="v2">schema2本人状态</span><!-- jobs-toc:heading-end -->

普通同步实时检查`sync_self_state`后自动安全选择schema2；S1控制记录、引用、事件和页暂存按旧stream在同RDB事务归档，新流先全快照，原认证namespace／设备与每库物理UUID／Outbox／终态／清理意图不迁移或重发。已有S2不能因能力消失降级。协议schema2与RDB的SecurityLevel.S2不是同一个概念。

| 入口 | 默认／持久化／生命周期 | 代码与当前验证 |
| --- | --- | --- |
| 设置→本人同步状态、云字号 | small／standard／large云偏好；默认standard，local slider仍是本机临时可读性设置。只在新云版本时应用，按本人来源隔离 | Index／SelfState.font；35主机及应用HAP编译，设备未验 |
| 会话→本人设置 | 当前CAS／manualUnreadGeneration；置顶排序、隐藏／归档、“显示全部”恢复入口，静音只影响本机新通知 | SelfProtocol／SelfState.changeView；原body先入同RDB，不以隐藏删消息 |
| “将已显示消息标为已读” | 仅当前前台可见≥50%的有效对端消息ID，用户明确提交≤200个及原成员代际；history／同步不自动观察 | onVisibleAreaChange→observe；精确字符串读点／计数，仅本人元数据，不是对方回执或ACK |
| 本人云草稿／所选回复 | 本人独立文本≤16384 UTF-8字节；当前da1，回复精确GET取dr1并核所见contentVersion，原nonce／CAS／proof入self_operations | SelfState.saveDraft；普通消息／投影不持久临时许可，绝不复制quote正文 |
| 采纳云文本／恢复文本 | 云更新不覆盖输入；明确采纳先核所见云版本／恢复op，重比整个本地draft与生命周期后同RDB事务换文本 | SelfState.adopt；clear仅清云端，原正式消息Outbox不改 |
| 待完成操作／继续原操作 | requestId与read／draft独立server operationId分开；先原body纯查，明确动作才原样续；prepared待核、retired冻结，已完成也纯查current | SelfState／self_operations；404／坏200不造成功，不换ID／proof |
| 旧会话本人草稿列表 | 只有本人metadata，缺当前会话时以明确只读个人条目打开；可查看／清旧文本，不取得聊天历史／附件／发送权 | Index.openOwnDraft；没有伪造有效member或云会话记录 |

[本轮S2证据](../../../项目相关报告/测试/前端/HarmonyOS/20261116-S2本人状态/README.md)分别封存35／35主机（25基础＋10 S2）、41／41旧规则和37项仅编译Hypium。新HAP包含S1引用版本修复；HDC、真实RDB／AssetStore／可见性回调、十语／主题实际渲染与鸿蒙HTTPS均未设备验证，不能用其它端的真服务结果补算。

RDB设置synchronous=FULL只是配置，设备断电和完整收讫／清云资格仍待验证。原生AssetStore使用DEVICE_UNLOCKED与SYNC_TYPE.NEVER，不设置卸载后保留；完整凭据对紧凑编码并限制UTF-8小于1024字节，单次OVERWRITE替换同一alias，失败不拆分成两个秘密条目、不先删旧值。只有明确未命中返回空，服务错误不静默覆盖。令牌不进RDB、普通偏好或日志；单次调用不等于已证明设备断电原子性或AssetStore与RDB跨库事务。

## <span id="jobs-toc-heading-6" data-jobs-toc="v2">认证刷新与未知结果</span><!-- jobs-toc:heading-end -->

服务端合同为AT15分钟、RT空闲30天、首次认证起绝对90天；客户端保存并验证服务端UTC期限，不用本地计时器自行延长。JSON、文件上传、下载和头像使用同一AuthCoordinator：只有明确ACCESS_EXPIRED协调一次refresh，当前family并发请求共用同一刷新任务。成功的新凭据对先写安全库，再完成RDB公开交接记录，之后才对业务可见；scope／原AT检查防止晚401、冷启动context或退出结果清除新设备。

refresh超时、响应丢失或安全库写失败后，持久非秘密family／原generation／requestId与unknown门禁，停止自动业务和旧RT重用。用户可用当前密码／OTP明确本人验证，沿已保存previousFamilyId及完整previousRequestIds闭包建立新认证；实时auth_retirement_chain缺失时拒绝新关联恢复。初始login／register／reauth在HTTP之前保存非秘密原意图；普通重试保持原ID与原字段，证明只在内存，不能重建旧证明时由用户明确选择新关联认证。未知register且账号未确认存在时坚持原register，不凭本地超时改为造号型reauth。

旧单AT安全条目不视作RT；原空间保留并冻结，新凭据按personal来源新空间落地。安全库已写、RDB交接未完成时，冷启动只能从匹配原requestId／family／代际的完整凭据恢复交接，不再消费旧RT。设备上的锁屏、进程崩溃／断电和AssetStore服务故障仍待签名设备测试。

## <span id="jobs-toc-heading-7" data-jobs-toc="v2">正式本人导出 <span id="harmony-personal-export"></span></span><!-- jobs-toc:heading-end -->

设置内已接原生导出页／本设备任务记录、十语三主题及完整Logo空态／重载。`ExportProtocol／ExportService／ExportStore`以当前namespace／family和已有RDB物理UUID持久原create／cancel完整body、hash与nonce，先纯查原单，明确fresh404后由用户显式同体重试；cap撤回仍能核对和取消，不新建或下载。原正式消息Outbox及聊天正文不复制到本模块，票／ATRT／制品key不进RDB。

`Api.dataExport／ExportFiles`接同源鉴权流→私有文件→有界ZIP／服务端安全文件名／长度／SHA→文件与父目录fsync→系统DocumentViewPicker.save。保存前重取当前ready与新RAM短票，目标写失败不报成功；取消选择明确未保存，跨身份／page晚响应拒绝，退出清私有暂存。上限1GiB／目录32MiB是本端限制，不虚称其它端可绕过。

[本轮报告](../../../项目相关报告/测试/前端/HarmonyOS/20261116-正式本人导出/README.md)封原15／15主机注入；后补18目标被4GiB门禁拦下、新5 Hypium仅源码。应用／测试HAP、真实RDB／文件NAPI／系统选择器、鸿蒙HTTPS与实际保存均未验，旧37方法HAP不覆盖新42方法源码。

## <span id="jobs-toc-heading-8" data-jobs-toc="v2">尚未验收与剩余I边界</span><!-- jobs-toc:heading-end -->

本轮没有签名、模拟器／真机执行或真实鸿蒙后端联调。[访问刷新限定证据](../../../项目相关报告/测试/前端/HarmonyOS/20261115-访问刷新/README.md)区分41项实际纯主机测试、应用／测试HAP构建和未执行的设备用例，保留各旧切片原计数与原件。仍需设备核对生命周期、CA拒绝路径、所有语言／字号／三主题、通知、媒体打开／播放、AssetStore、RDB耐久和原结果丢响应／恢复竞态。完整本人数据导出／受控导入迁移、资格ACK与清云、全部I容量及无障碍等仍未完成，不能把更多源码和HAP算成全I或243项验收。

正式本人导出已有上述原生源码及限定主机证据，仍待本轮HAP／设备／真实网络；受控导入迁移、资格ACK与清云等未完成范围保持不变。厂商远程推送、RTC、地图、支付、产品SDK和E2EE按后续标段接入。单一现行需求仍是完整产品目标。

[后端契约](../../后端/Go/接口契约/README.md) · [代码地图](./代码地图/README.md) · [最新限定证据](../../../项目相关报告/测试/前端/HarmonyOS/20261115-提及公告与分页/README.md) · [产品记录](./PRODUCT.md) · [设计记录](./DESIGN.md)。

消息原发信资格绑定及旧记录保护见[专项证据](../../../项目相关报告/测试/前端/HarmonyOS/20261115-原发信资格/README.md)。原资格失效冻结原记录；媒体上传不改用后来会话资格，旧服务不接收POST。设备上的RDB／HTTP组合仍待验证。

[原角色资格专项](../../../项目相关报告/测试/前端/HarmonyOS/20261115-原角色资格/README.md)：23项主机规则执行、应用及23用例测试HAP编译通过；设备与真实鸿蒙HTTP未验。

普通转发已接原生源版本重选、原始引用入队、未知结果纯查／明确重投、转发投影清理与固定消息附件代理；[本轮25规则／HAP证据](../../../项目相关报告/测试/前端/HarmonyOS/20261115-普通转发/README.md)不代表鸿蒙设备运行。
