# JobsIM Go CLI

![JobsIM](../Web/Public/Logo.svg)

[toc]

---

## 🔥 <span id="前言">前言</span>

本CLI调用同一[后端契约](../../后端/Go/接口契约/README.md)。使用[**Go**](https://go.dev/)1.27.4，终端密码输入不回显，不把密码放命令行参数或普通配置。当前新增访问／刷新协调与三系统安全库源码，验证状态见下文；旧历史报告不外推至此增量。

## 一、<span id="use">使用</span> <a href="#前言"><b>🔼</b></a> <a href="#🔚"><b>🔽</b></a>

```shell
python3 Scripts/Build.py/Build.py --go <已核验Go> --output <新外置发行目录> --sign-identity <已有授权签名身份> --application-id <实际获权标识>
jobsim --server https://实例入口 --ca /该实例受控CA.pem login
```

公信HTTPS无需`--ca`，指定CA只作用本客户端，仍验证主机名与有效期。仅回环HTTP开发可显式`--insecure-loopback`，不存在跳过TLS验证选项。`register`为本地注册入口，成功进入交互会话。输入用户名、密码后可用`users`、`contacts`、`request 用户ID`、`accept 申请ID`、`direct 用户ID`、`group 群名 | 成员ID`、`conversations`、`history 会话ID 游标`、`send 会话ID 文本`、`edit 消息ID 预期版本 文本`、`react 消息ID 表情`、`retract 消息ID`、`pin 消息ID`、`favorite 消息ID`、`search 关键词`、`upload 文件路径`、`logout`和`quit`。

## 二、<span id="governance">联系人与群管理</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

以下命令在登录后的 `jobsim>` 提示符输入；`|` 是该交互命令的分隔符，不是 Shell 管道。

| 命令 | 输入与行为 |
| --- | --- |
| `relationship 用户ID` | 只读取本人对该人的好友／拉黑／入站申请门禁，不返回对方私人策略 |
| `request 用户ID`、`reopen 用户ID` | 先持久保存原 requestId／载荷／摘要／恢复代际，再 POST；已有同类同人的 pending 操作沿用原 ID；完成后再次输入代表明确的新意图 |
| `operations`、`resume 原操作ID` | 查看本账户原操作并显式恢复；已完成只显示原事实和当前本人关系，不再次 POST；已知失败或恢复代际变化冻结的操作不自动重放 |
| `accept 申请ID`、`reject 申请ID` | 处理本人入站申请；`reopen` 仅允许新的入站申请，仍需发送人新申请及本人新接受 |
| `remove-contact 用户ID` | 先在 SQLite 事务冻结此人的对应 direct 待发，再删除好友；不影响其他私聊或群待发，不等于拉黑 |
| `block 用户ID`、`unblock 用户ID` | 拉黑前同样冻结对应 direct 待发；解黑不解冻旧待发、不自动重开申请或恢复好友 |
| `members 会话ID` | 读取当前成员及每人的 membershipGeneration |
| `add-member 会话ID 用户ID` | 按后端权限加人；未知结果先用 members 核对，不自动重放 |
| `remove-member 会话ID 用户ID 原成员代际` | 必须明确提供该成员的原代际，不用最新代际替换旧请求；移除本人执行与 leave 相同的本地撤回 |
| `rename 会话ID 新群名`、`transfer 会话ID 新群主ID` | 分别只发送 title／ownerId 字段；群主须先转让再离群 |
| `leave 会话ID 本人原成员代际` | 先核对当前群快照仍是该原代际，再事务冻结该群待发、撤本机该群历史／游标；私人草稿保留。旧离群意图遇重入新代际时不触碰新的本地数据 |

好友申请和重新开放申请的原操作会跨进程恢复；成功事实与当前关系分开显示，旧“曾开放”不会冒充当前已开放。网络失败／响应无法解析保留 pending；明确 4xx（429 除外）记失败，用户核对关系后才能发起新的明确意图；后台不重试。实例或恢复代际变化冻结旧操作。

群改名、转让、成员变更、删除好友、拉黑和退群的写请求每次命令只发送一次；未知结果不能推断服务端已执行。此时本地冻结／退群撤回仍保留，先通过 relationship／members／conversations 核对当前事实。冻结原因独立保存为 CONTACT_REMOVED、PEER_BLOCKED、CONVERSATION_LEFT、SEND_ADMISSION_CHANGED 或 RESTORE_EPOCH_CHANGED，原消息 payload／sendAdmission／ID／摘要均保留。

当前源码将 `reconcile 原clientMsgId` 统一接到原会话路由的只读 `message-results`，始终发送原完整 body，核对完整 Message、本人的 sender／clientMsgId／conversationId，并在 SQLite 事务内重比原 payload_hash 后确认。同步流或历史里的相同 clientMsgId 不确认 Outbox。404、409、网络未知或坏 200 保留原行；冻结消息不重建、不补最新资格。治理旧报告的历史分页查找仅证明当时固定 binary，不能追认为本轮源码已实跑。

## 三、<span id="outbox">Outbox与范围</span> <a href="#前言"><b>🔼</b></a> <a href="#🔚"><b>🔽</b></a>

当前使用 pure-Go SQLite（modernc.org/sqlite 1.62.0、对应 libc 1.80.1），启动核验 WAL 与 synchronous=FULL。当前源码本地 schema4 从 schema1 增量升级：schema2 添加独立冻结原因与原治理操作表，schema3 添加公告原操作引用表，schema4 添加权威同步／清理表（已完成本地编译回归，真实服务另验）；旧待发字节和摘要保留。待发送原载荷和草稿清理在同一事务提交后才发 HTTP；消息正文和连续游标同事务，字符串大 ID 和版本不转浮点数、不倒退。新认证目录绑定经验证地址／CA／deployment档案、用户与个人来源；消息namespace还绑定服务器实际deviceId和store_generation。重新认证取得新device前冻结旧设备／旧epoch及legacy队列，保留原ID／载荷／摘要，不迁移到新设备自动发送。系统凭据仍在而消息库缺失时也要求本人重新验证、退休旧family，不以空库继承旧设备资格。完整合格ACK和旧备份防回退登记仍依后端后续公开闭环验收。

`outbox`查看，`retry 原clientMsgId`重试；恢复代次变化冻结旧队列并保留旧历史，`reconcile 原clientMsgId`显式核对旧结果，始终携原代次和原ID，不重新生成。`cached 会话ID`、`draft 会话ID 内容`、`drafts`、`sync 会话ID`分别操作当前空间缓存、草稿与增量游标。服务器权限拒绝或恢复代次错误不会伪装成本地成功。

旧JSONL文件不会自动删除或重发；核实来源确属当前服务器后，可用 `import-outbox 原路径 | 当前deploymentId` 原子迁入，校验用户/部署/载荷及源摘要，重复迁入或损坏整批拒绝，原文件保留。普通数据目录默认为用户配置目录，可用 `--data-dir`显式指定。第三方原始许可和摘要见[清单](./第三方许可/清单.json)。

搜索协作切片的 21 组 SQLite／故障与 HTTPS 限定证据见[搜索协作报告](../../../项目相关报告/测试/CLI/20261115-搜索协作/README.md)，此前普通转发 17 组与治理链各保留原范围；本轮当前计数见下述刷新报告，不跨轮加算。完整测试入口见[CLI测试](../../../测试/CLI/README.md)，入口定位见[代码地图](./代码地图/README.md)。当前网络失败按原身份保留未知；未提供前端完整本地演示回退，真实API失败不假报发送成功。CLI是终端客户端，不提供图形主题／播放器／摄像头UI。权威快照已完成本地及固定57专项，合格收讫回执、二维码及全部I范围仍须单独验收。

附件可 `upload 文件路径` → `send-media 会话ID image|audio|video|file 对象ID 说明`；`download 对象ID | 目标路径`鉴权下载，拒绝覆盖已有文件。`reply 会话ID 源消息ID 内容`、`message 源消息ID`核对内容版本后，`forward 会话ID 源消息ID 源内容版本`遵守服务端能力开关；`unpin`／`unfavorite`／`block`／`unblock`／`sessions`／`revoke`完成对应操作。`recover`在登录前输入恢复码及新密码且不自动登录；登录后`recovery-codes`要求当前密码，只在当次终端显示一次。管理员若收到MFA挑战再输入验证器验证码，验证码不进入普通配置或数据库。

每条新消息在严格入队前取得当前会话的 `sendAdmission` 不透明快照，之后重试／重开／显式原ID对账均使用原值；无快照的旧记录不从最新资格补齐。服务端返回 `SEND_ADMISSION_CHANGED` 后持久冻结，普通retry不再发，显式reconcile也不能换资格创建旧消息。每次发送／对账先核对当前实例的 `send_admission:true` 能力，旧服务不接收POST。资格读取失败时保留草稿，不把无法取得资格的新消息标为已入队。


[普通转发限定验证](../../../项目相关报告/测试/CLI/20261115-普通转发/README.md)覆盖原引用入队、两类未知结果、原proof冻结和转发缓存无正文。`download-forward 消息ID | 目标路径`每次重新鉴权下载；普通展示去除临时proof，转发离线缓存保持占位。明确下载的本机文件不承诺远程撤销。

## schema2本人状态与云草稿

普通`sync`在当前实例明确支持`authoritative_sync`／`sync_jetstream`／`sync_self_state`时自动选择协议schema2，不要求用户先理解schema。S1旧责任的control、引用、事件和页暂存在同一SQLite事务按旧stream封存，新流从完整快照开始；原物理UUID、认证32hex绑定、namespace、Outbox原字节、终态和清理责任保留。已有S2遇能力消失拒绝降级；`sync-schema 2`供显式诊断，不能变成空库继承旧设备资格的入口。继续使用本地数据库schema4，新增`sync_stream_archive`／`self_operations`，协议schema号不等于数据库schema号。

| 命令 | 入口、默认与生命周期 |
| --- | --- |
| `font`／`font small\|standard\|large` | 读写本人云字号偏好，默认standard；原CAS和nonce先入SQLite。CLI输出该偏好，不修改终端模拟器字体 |
| `view 会话ID`／`view 会话ID JSON_CHANGES` | 查看或修改pinnedRank、mutedUntil、archived、hidden、manualUnread；手动未读捕获独立generation，普通read不清它。nullable字段接受显式null，布尔null拒绝 |
| `conversations`／`conversations all` | S2按本人置顶排序，默认隐藏hidden／archived；all保留可恢复入口，返回ownViews及字号元数据，不把隐藏当作消息删除 |
| `read 会话ID`／`read 会话ID 已观察消息ID[,ID...]` | 前者精确GET本人当前计数，后者最多200真实观察ID并捕获原成员代际；仅显示history不会自动提交已读，不能用最大sequence代替观察 |
| `cloud-draft 会话ID` | 精确GET本人独立文本、回复状态及恢复文本；临时da1／dr1不输出，不把S2快照中的引用当新写许可 |
| `cloud-draft-set 会话ID \|文本` | 新完整PUT，仅保存本人独立输入及reply=null；16384 UTF-8字节上限，分隔符后文本原样入原body |
| `cloud-draft-reply 会话ID 消息ID 所见contentVersion \|文本` | 精确GET原消息取dr1；版本变化要求重新选择，原da1／dr1与ID先耐久保存，不复制quote正文 |
| `cloud-draft-clear 会话ID` | 原DELETE意图；离群后仍依服务端许可清理本人旧文本，不取得新PUT或聊天媒体权限 |
| `cloud-draft-adopt 会话ID 所见草稿版本 \|当前本地原文` | 用户明确采纳云文本到普通本地draft，事务重比当前本地原文；不同即拒，绝不静默覆盖输入。空本地草稿可省略分隔符 |
| `cloud-draft-recover 会话ID 恢复operationId \|当前本地原文` | 明确采纳retired操作留下的本人独立文本，同样按本地原文CAS；不恢复旧回复授权或发送消息 |
| `self-operations`／`resume-self 原操作ID` | 列本人原操作元数据；恢复先只读原body的*-results，404／prepared仅在此次明确动作沿原method／route／body续跑一次。prepared不是成功，retired冻结；current=false不回滚当前值 |

云草稿需实时`cloud_drafts`。原nonce、epoch、CAS、成员代际和临时许可持久在当前私有SQLite原操作中，未知不换ID／proof。普通`draft`仍是本地独立输入，服务器投影或旧成功不会覆盖它；正式消息Outbox也不因草稿更新改变。snapshot的owner引用revision与对象自身version分别校验，不错误要求相等；全manifest／原字节SHA／finalvalidate＋首次changes安装、global及完整消息只读证明继续沿原同步合同。

`resume-self`使用客户端原`requestId`（`self-operations`的id）；read／draft的服务端`operationId`是独立UUID，不能互相替代。结果显示两者，原body和摘要保持。已完成的显式恢复也只读重核当前标记，404不重建，不能把旧缓存的`current:true`当成当前状态。普通显示与消息缓存剔除fa1／dr1／da1，待完成原操作仍保存其不可替换的原proof。

当前15a4c053候选的同输入本地55组通过（旧45＋新10），0失败，主机轮真实HTTPS／OS vault两项按范围跳过；MacOS arm64构建及独立真实S2 TTY 13／13通过，包含已完成原操作只读重核current=false。前两失败、587de旧13和最终新13各保原输入，见[独立本轮记录](../../../项目相关报告/测试/CLI/20261116-S2本人状态/README.md)。未跑Linux／Windows新源码或OS安全库正向；旧S1真实服务和交叉编译不追认为此增量。qualifiedACK／after_receipt仍没有实现。

| 局部资产入口 | 代码位置／持久化差异 | 当前验证 |
| --- | --- | --- |
| 自动S2／会话列表 | sync_engine＋sync_store／self_state；同SQLite事务封存旧stream，保原身份／Outbox／终态，隐藏可用all恢复 | 55本地＋本轮13真实TTY限定覆盖 |
| font／view／read | self_state原CAS／manual代际／真实观察集合；font只云偏好，不改终端字体，history不隐式观察 | 本人偏好隔离、隐藏恢复、真实洞与manualUnread独立实际通过 |
| cloud-draft／采纳／恢复 | self_operations原method／route／body／hash；dr1／da1不进普通展示／缓存，独立本地输入不被旧结果覆盖 | 文本、回复、global仅失效reply、清除及本地CAS实际通过；retired恢复文本仅本地源规则，未做本轮真故障注入 |
| self-operations／resume-self | requestId与server operationId分开，prepared／retired／applied分开，完成后也纯查当前标记 | 55本地与13真实链包含current=false；未注入真实丢响应／同family重启 |

## 四、<span id="collaboration">搜索、收藏与群协作</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 命令 | 输入与结果 |
| --- | --- |
| `search 查询词`、`search 查询词 \| 会话ID` | 查询 1～256 UTF-8 字节；新搜索清旧游标，默认每页 50 |
| `favorites`、`favorites 会话ID` | 本人收藏的实时源引用，同样每页 50 |
| `more` | 原样发送本次列表的不透明签名游标，保留原查询／会话／本人／实例与恢复代际；不能临时改条件 |
| `open-result 消息ID` | 仅选择本次列表结果，精确 GET 会话及原消息后展示当前授权正文，不受目录前 100 条影响；失权清查询状态，不回显旧列表正文 |
| `mention 会话ID 成员ID,成员ID \| 文本` | 最多 20 个不重复的当前成员；普通文字中的 @名字不建立提醒资格 |
| `mention 会话ID all \| 文本` | 仅当前群主；新写核 mentions／role_admission，原名单／原准入／原角色快照先入 SQLite，重试不补最新值 |
| `announcement 会话ID` | 读取当前公告；只有 active 包含当前源正文 |
| `announce 会话ID 源消息ID 源内容版本 none或1d或7d` | 先用 `message 源消息ID` 查看 contentVersion，再明确选源；重新取源、群成员／角色代际和当前公告 CAS；只持久化原引用，不复制正文 |
| `clear-announcement 会话ID` | 当前群主明确清除，原 CAS／资格／requestId 同样先入库 |
| `announcement-operations`、`resume-announcement 原操作ID` | 查看本机原操作或明确恢复；先 GET 原 operation，未知时沿原完整 body 写，已成事实再 GET 当前公告；旧 current=false 不回滚当前引用 |

`|` 是登录后的 CLI 命令分隔符。列表正文只在本次实时输出，不另存搜索／收藏正文或游标；临时 forwardAdmission 从普通展示递归去除，既有转发离线缓存继续只保留占位。已经输出到终端或用户明确下载的文件不承诺远程擦除。

成员提及需要实时 mentions／send_admission，@所有人还需要 role_admission 和原角色快照；旧缺保护只能只读原结果。明确成员／资格拒绝永久冻结，429／暂时策略失败保留原意图。公告原操作的 requestId、CAS、成员／角色代际、源 contentVersion 和期限保持不变；未知不能被新操作覆盖，已知失效被冻结后才允许用户另建新意图。恢复代际变化冻结旧操作。真实 HTTPS 与受控丢响应／SQL 故障的边界分别见报告，不将局部测试视为完整 I 验收。

## 五、<span id="refresh">刷新与系统安全凭据</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

默认使用[系统凭据适配](./原生助手/Keychain/README.md)：macOS随包自有Swift Keychain助手；Windows Generic Credential Manager／本机持久；Linux系统Secret Service的secret-tool。AT／RT及精确family／generation／UTC期限作为单项安全记录原子替换，普通SQLite只存非秘密初始意图、完整退休链、family和unknown／待撤销责任。Go仍CGO关闭；既有x/sys现为直接使用，未引入新第三方包装库。缺助手、错误签名／摘要、锁定或系统拒绝不写明文文件、环境变量或参数；交互终端可明确输入yes选择本进程临时模式，无人值守失败。

同家族只协调一次刷新，先耐久unknown再发原RT，安全库替换及非秘密交接完成后才释放请求；仅明确ACCESS_EXPIRED可触发一次，upload／download也用同一协调器。未知／解析失败／安全库写失败保持停止，不复用旧RT、不借初始300秒交付；晚到响应不能进入另一family。初始请求ID在HTTP前保存，默认沿原意图续；需要当前密码／新OTP时显式启动`reauthenticate`，新意图冻结完整旧→新previousRequestIds且实时检查auth_retirement_chain。未知注册不能因本地超时推断账号已建，须原register ID或服务器明确DELIVERY_EXPIRED后受控恢复。

quit／logout立即停本端，尽力删除该安全存储会话条目并撤原family；临时会话没有写入OS条目，只停其运行内存与服务器家族。离线或响应未知保留非秘密待撤销，重启本人新证明先退休旧family。普通SQLite写失败也不能短路安全库删除和在线撤销。当前[刷新报告](../../../项目相关报告/测试/CLI/20261115-刷新家族/README.md)分别记录31组本机测试、真实HTTPS协调器和实际TTY限程会话；Mac系统库正向写未通过，Win／Linux仅amd64跨编译。正式签名／应用授权与目标系统服务仍须分别验证，不把构建或受限模式写成三系统／全I验收。

## 六、<span id="authoritative-sync">权威快照与增量</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

最初的[源码检查点](../../../项目相关报告/测试/CLI/20261116-权威同步/README.md)原样保留。资源恢复后的[本地编译回归](../../../项目相关报告/测试/CLI/20261116-权威同步编译回归/20261116-113743-192d48/README.md)已完成 MacOS arm64 Go 构建及正式 overlay 运行器：45 个顶层本地测试通过（旧 31＋新 sync 14），0 失败，真实 HTTPS／原生凭据库 2 项按本轮范围跳过。仅新隔离 binary，不更新旧发行产物；该本地轮未执行真实服务；后续固定57专项见本节末尾，跨系统原生仍未验证。

`sync` 当前按实时能力安全选择schema2，未支持S2且尚无S2责任时保持schema1；已有S2不降级。`sync-status`只读本机元数据，`sync 会话ID`／`history 会话ID`展示已安装的权威历史。不承担责任时先核实时authoritative_sync／sync_jetstream；旧服务拒绝本次同步，不写新责任。首次持久承担后，能力关闭、缺页或未知不得退回旧轮询。原profile installation 32hex、Auth MessageStore、namespace文件名保持不变；同步另在每个SQLite保存独立canonical UUID physical generation，登记后缺失不能以新UUID绕过原设备围栏。

登记原 requestId、快照原请求在 HTTP 前耐久保存。全 manifest 及 referenceEnvelope／bodyEnvelope／eventEnvelope 先校验原 bytes SHA，再比结构、精确计数和版本；缺页／未知不装正文。末次 validate 分类水位不得低于原水位，首次 changes 权限确认后，projection／连续游标／origin 去重／guard CAS 同一 SQLite 事务安装。暂存页只有引用／摘要；forward 和公告不持久化投影正文或临时 proof，原 Outbox 完整发送意图仍原样保留。

`cached` 及普通导出受当前限制。`local-history 会话ID` 为明确只读入口，仅开放同可信 profile／deployment／本人／personal／epoch 下曾验证且正文 hash 相等的普通副本，不授予云媒体权限。身份待核、恢复代际变化、未验证旧库隔离。global 是永久终态，低版本也先清旧正文；清理意图同事务保存，启动／每轮离线 drain 同来源旧 device 库，失败保留意图。不明旧 hash 或异 epoch 库不猜归属；跨库／文件清理不是跨 SQLite 原子事务，显式下载的外部文件不承诺远程擦除。

原 Outbox 仅完整原路由／原 body 的只读成功事实才确认。辅助查单 503／坏 200 是零 proof，不阻挡已验证的独立 global 安装；自动 sync／reconcile 不创建消息。`retry` 是用户明确动作：先纯查，仅明确 404、当前身份／epoch／库／能力仍有效且原行未冻结时，沿原 ID／body／资格重投，不补新 proof。qualifiedAck／after_receipt 仍为 false；服务端已读、云草稿／偏好、完整迁移及 Mac DPK entitlement 不由本轮源码声明验收。

本地回归补强：旧行缺少原 sendAdmission 时，查单 404 也只冻结，绝不首次消息 POST；原 body／hash 不改。旧测试夹具按完整真实 DTO（UUID 主体、contentVersion、deleted、type）、只读查单路由和 schema4 更新，保留首轮失败日志，不放宽生产校验。运行器默认 Go 测试超时 120 秒，可用 `JOBSIM_CLI_TEST_TIMEOUT` 选择明确时长；真实 HTTPS／系统库仍需显式授权环境。

[固定57真实 HTTPS／TTY 专项](../../../项目相关报告/测试/CLI/20261116-权威同步HTTPS/20261116-120401-3cbd4e/README.md)另通过 10 项限定检查：双向消息／权威同步、本人未提交原意图与 peer 真同 CID 成功分离、global 不消其他原队列、新 device／旧库清理、退群云读拒绝而普通合法本地副本保留，以及后续 global 清历史 holder。坏媒体实际为 403 OBJECT_NOT_OWNED，明确拒绝不标未知。两账号用真实终端明确选择 process-only，不证明 OS 安全库正向；未拦截 CLI HTTP 计数，不宣称零创建 POST。本轮固定服务、原账号、PG／配置均未改。

同一权威同步候选另完成[Linux／Windows amd64 离线交叉编译](../../../项目相关报告/测试/CLI/20261116-权威同步交叉编译/20261116-123709-093a23/README.md)，CGO=0；19 项 Go／模块输入与上述 MacOS／HTTPS binary 相同，无源码改动。只验证 ELF／PE 产物及编译兼容，未运行目标系统程序或凭据库，不能称 Windows／Linux 原生通过。

## 七、<span id="content-receipts">独立正文收讫</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

该切片使用[后端第十节](../../后端/Go/接口契约/README.md#durable-content-receipts)，需当前实例实时声明`durable_receipt_protocol`、`authoritative_sync`和`sync_jetstream`。先正常`sync`登记当前物理库；默认未获存储批准，不自报合格。保持本地schema4、原认证32hex绑定及独立physical UUID，不换device或旧队列命名空间。

| 命令 | 行为与持久化 |
| --- | --- |
| `storage-status` | GET当前device／epoch／physical UUID声明；未批准可返回`qualified:false`和空qualificationId，不生成资格 |
| `storage-declare PROFILE_ID BUILD_SHA256 VERIFICATION_SHA256` | 显式声明部署者已批准的构建／存储验证报告。核本地可执行文件SHA与WAL／FULL，原PUT body／nonce先入`storage_declarations`再HTTP；已有未收口声明须沿原ID续跑 |
| `receive-text 消息ID` | 当前普通纯文本challenge核原bodyEnvelope字节SHA、完整Message、scope／本人／device／store／epoch／qualification／permission及有效期；同SQLite事务保存正文与`receipt_outbox`原nonce／body／摘要，再查原结果及尝试首写 |
| `receipts` | 只列原请求ID、类型、消息ID、epoch和状态，不打印正文、rc1、原body或临时许可 |
| `reconcile-receipt 原请求ID` | 普通正文始终以完整原body调用只读receipt-results；404／503／坏200保留原意图，不创建收据。声明没有独立results接口，只GET当前观察，不冒认原PUT已成功 |
| `retry-receipt 原请求ID` | 本次用户明确动作，先查正文原结果；仅对应404、仍pending及原资格／内容／期限／本地门禁都有效，才沿原body写一次。声明同样只重投原PUT，不换nonce |

正文和收据意图达到同一WAL／FULL事务提交点前不发送回执，SQL失败整体回滚。重启后原意图可查／续；不会自动刷新旧challenge或将旧qualification／epoch补成新值。过期未成意图冻结但仍可只读查询历史已成事实；已完成收据遇404不重建。普通同步的相同clientMsgId与正文收讫都不确认正式发送Outbox，也不推进累计收讫位置或把history当已阅。global删除清正文后，原收据只留无正文tuple／摘要与事实，不能凭旧请求复活内容。

部署批准项和本地文件SHA检查不等于运行时认证、远程硬盘／断电证明或OS安全库正向验收。当前仅普通纯文本，转发／附件拒绝；所有收据结果仍要求`objectComplete:false`、`targetRangeComplete:false`、`deletionEligible:false`，`after_receipt`关闭。没有全对象、冻结目标范围、合格累计ACK或云端唯一副本删除完成声明。

本轮67顶层本地PASS／0FAIL／2项显式环境跳过（含新增12组SQLite／注入HTTP）及MacOS arm64新候选构建见[正文收讫证据](../../../项目相关报告/测试/CLI/20261116-正文收讫/README.md)。未连接真实收讫服务／E2、未实际配置部署批准策略、未做目标OS或设备断电测试；15a4历史55＋13和旧binary不覆盖。

| 局部资产入口 | 默认／代码位置／生命周期 | 当前验证与差异 |
| --- | --- | --- |
| storage-status／declare | 默认transport-only；receipt.go核实时flags与既有UUID，storage_declarations原body耐久后PUT；无results时GET仅当前观察 | 本轮SQLite／HTTP替身及构建；未实际后端批准，不冒充attestation |
| receive-text | receiptValidateChallenge原字节SHA及tuple；stageReceipt同事务正文＋receipt_outbox；不保存第二份正文或改发送Outbox | 提交前HTTP检查、故障回滚、原体重开、身份／guard／global／过期反例通过 |
| receipts／reconcile／retry | 本人原库原请求；自动／普通核对纯查，显式retry才原body写，完成／冻结不凭404新建 | 原nonce／资格不替换，未知不成功，正文事实不等于对象／targets完成；真实HTTPS仍待验 |

<a id="🔚" href="#前言">返回首页</a>
