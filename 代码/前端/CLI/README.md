# JobsIM Go CLI

![JobsIM](../Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、使用](#use)
- [二、联系人与群管理](#governance)
- [三、Outbox与范围](#outbox)
- [四、schema2本人状态与云草稿](#schema2%E6%9C%AC%E4%BA%BA%E7%8A%B6%E6%80%81%E4%B8%8E%E4%BA%91%E8%8D%89%E7%A8%BF)
- [五、搜索、收藏与群协作](#collaboration)
- [六、刷新与系统安全凭据](#refresh)
- [七、权威快照与增量](#authoritative-sync)
- [八、独立正文收讫](#content-receipts)
  - [当前物理库的合格收讫协商](#jobs-toc-heading-10)
- [九、完整附件收讫](#object-receipts)
- [十、正式本人数据导出](#personal-data-export)
- [十一、本人主动改密](#own-password-change)
- [十二、本人注销](#own-account-deletion)
- [十三、普通端侧迁移增量](#personal-migration)

</details>
<!-- jobs-toc:end -->

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
| `rename 会话ID 新群名` | 只发送title字段；转让使用下方带完整原版本和代际的命令 |
| `transfer 会话ID 新群主ID 群版本 本人成员代际 本人角色代际 目标成员代际 目标角色代际` | 固定原观察及nonce后再写；拒绝不带版本的旧转让形式，群主须先转让再离群 |
| `leave 会话ID 本人原成员代际` | 先核对当前群快照仍是该原代际，再事务冻结该群待发、撤本机该群历史／游标；私人草稿保留。旧离群意图遇重入新代际时不触碰新的本地数据 |

好友申请和重新开放申请的原操作会跨进程恢复；成功事实与当前关系分开显示，旧“曾开放”不会冒充当前已开放。网络失败／响应无法解析保留 pending；明确 4xx（429 除外）记失败，用户核对关系后才能发起新的明确意图；后台不重试。实例或恢复代际变化冻结旧操作。

群改名、转让、成员变更、删除好友、拉黑和退群的写请求每次命令只发送一次；未知结果不能推断服务端已执行。此时本地冻结／退群撤回仍保留，先通过 relationship／members／conversations 核对当前事实。冻结原因独立保存为 CONTACT_REMOVED、PEER_BLOCKED、CONVERSATION_LEFT、SEND_ADMISSION_CHANGED 或 RESTORE_EPOCH_CHANGED，原消息 payload／sendAdmission／ID／摘要均保留。

群治理还提供`group-policy 会话ID`、`group-change 会话ID 群版本 本人成员代际 本人角色代际 settings|mute|role|dissolve | JSON_CHANGES`，以及`group-invites`／`group-applications`分页、`create-invite`／`revoke-invite`／`group-approve`／`group-reject`。`invite-code 会话ID 邀请ID`只在终端显示当前邀请码，`join-group 原邀请码`固定原部署及邀请；邀请token、OTP和step-up证明仅RAM，不入SQLite。`group-recover 会话ID 现有继任者ID 群版本 目标成员代际`要求独立本地运营授权与本次真实MFA，不借普通群主转让权限。

`group_intents`先耐久保存原nonce／route／完整body／family，再发写请求；`group-operations`列原责任，`group-check 原操作ID`只查原结果，`group-retry 原操作ID`显式沿原单续跑。后继family和缺family的legacy记录不创建旧操作，回执必须匹配类型化目标。当前[group_governance_test.go](../../../测试/CLI/group_governance_test.go)5个顶层专项均包含在[完整135 PASS／0 FAIL／2环境SKIP](../../../项目相关报告/实现进度/20261116-剩余能力闭环/逐store收讫证据/CLI-升级门禁最终-20261117-142600/完整本地结果.json)内，覆盖恢复MFA仅RAM、原许可／nonce纯查、邀请码不落库、SQL失败零写及后继family／错误typed目标拒绝。HTTP为注入，真实群治理HTTPS／管理员MFA及目标系统秘密库另验。

当前源码将 `reconcile 原clientMsgId` 统一接到原会话路由的只读 `message-results`，始终发送原完整 body，核对完整 Message、本人的 sender／clientMsgId／conversationId，并在 SQLite 事务内重比原 payload_hash 后确认。同步流或历史里的相同 clientMsgId 不确认 Outbox。404、409、网络未知或坏 200 保留原行；冻结消息不重建、不补最新资格。治理旧报告的历史分页查找仅证明当时固定 binary，不能追认为本轮源码已实跑。

## 三、<span id="outbox">Outbox与范围</span> <a href="#前言"><b>🔼</b></a> <a href="#🔚"><b>🔽</b></a>

当前使用 pure-Go SQLite（modernc.org/sqlite 1.62.0、对应 libc 1.80.1），启动核验 WAL 与 synchronous=FULL。当前源码本地 schema4 从 schema1 增量升级：schema2 添加独立冻结原因与原治理操作表，schema3 添加公告原操作引用表，schema4 添加权威同步／清理表（已完成本地编译回归，真实服务另验）；旧待发字节和摘要保留。待发送原载荷和草稿清理在同一事务提交后才发 HTTP；消息正文和连续游标同事务，字符串大 ID 和版本不转浮点数、不倒退。新认证目录绑定经验证地址／CA／deployment档案、用户与个人来源；消息namespace还绑定服务器实际deviceId和store_generation。重新认证取得新device前冻结旧设备／旧epoch及legacy队列，保留原ID／载荷／摘要，不迁移到新设备自动发送。系统凭据仍在而消息库缺失时也要求本人重新验证、退休旧family，不以空库继承旧设备资格。完整合格ACK和旧备份防回退登记仍依后端后续公开闭环验收。

`outbox`查看，`retry 原clientMsgId`重试；恢复代次变化冻结旧队列并保留旧历史，`reconcile 原clientMsgId`显式核对旧结果，始终携原代次和原ID，不重新生成。`cached 会话ID`、`draft 会话ID 内容`、`drafts`、`sync 会话ID`分别操作当前空间缓存、草稿与增量游标。服务器权限拒绝或恢复代次错误不会伪装成本地成功。

旧JSONL文件不会自动删除或重发；核实来源确属当前服务器后，可用 `import-outbox 原路径 | 当前deploymentId` 原子迁入，校验用户/部署/载荷及源摘要，重复迁入或损坏整批拒绝，原文件保留。普通数据目录默认为用户配置目录，可用 `--data-dir`显式指定。第三方原始许可和摘要见[清单](./第三方许可/清单.json)。

搜索协作切片的 21 组 SQLite／故障与 HTTPS 限定证据见[搜索协作报告](../../../项目相关报告/测试/CLI/20261115-搜索协作/README.md)，此前普通转发 17 组与治理链各保留原范围；本轮当前计数见下述刷新报告，不跨轮加算。完整测试入口见[CLI测试](../../../测试/CLI/README.md)，入口定位见[代码地图](./代码地图/README.md)。当前网络失败按原身份保留未知；未提供前端完整本地演示回退，真实API失败不假报发送成功。CLI是终端客户端，不提供图形主题／播放器／摄像头UI。权威快照已完成本地及固定57专项，合格收讫回执、二维码及全部I范围仍须单独验收。

附件可 `upload 文件路径` → `send-media 会话ID image|audio|video|file 对象ID 说明`；`download 对象ID | 目标路径`鉴权下载，拒绝覆盖已有文件。`reply 会话ID 源消息ID 内容`、`message 源消息ID`核对内容版本后，`forward 会话ID 源消息ID 源内容版本`遵守服务端能力开关；`unpin`／`unfavorite`／`block`／`unblock`／`sessions`／`revoke`完成对应操作。`recover`在登录前输入恢复码及新密码且不自动登录；登录后`recovery-codes`要求当前密码，只在当次终端显示一次。管理员若收到MFA挑战再输入验证器验证码，验证码不进入普通配置或数据库。

每条新消息在严格入队前取得当前会话的 `sendAdmission` 不透明快照，之后重试／重开／显式原ID对账均使用原值；无快照的旧记录不从最新资格补齐。服务端返回 `SEND_ADMISSION_CHANGED` 后持久冻结，普通retry不再发，显式reconcile也不能换资格创建旧消息。每次发送／对账先核对当前实例的 `send_admission:true` 能力，旧服务不接收POST。资格读取失败时保留草稿，不把无法取得资格的新消息标为已入队。


[普通转发限定验证](../../../项目相关报告/测试/CLI/20261115-普通转发/README.md)覆盖原引用入队、两类未知结果、原proof冻结和转发缓存无正文。`download-forward 消息ID | 目标路径`每次重新鉴权下载；普通展示去除临时proof，转发离线缓存保持占位。明确下载的本机文件不承诺远程撤销。

## 四、<span id="schema2本人状态与云草稿">schema2本人状态与云草稿</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

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

## 五、<span id="collaboration">搜索、收藏与群协作</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

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

## 六、<span id="refresh">刷新与系统安全凭据</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

默认使用[系统凭据适配](./原生助手/Keychain/README.md)：macOS随包自有Swift Keychain助手；Windows Generic Credential Manager／本机持久；Linux系统Secret Service的secret-tool。AT／RT及精确family／generation／UTC期限作为单项安全记录原子替换，普通SQLite只存非秘密初始意图、完整退休链、family和unknown／待撤销责任。Go仍CGO关闭；既有x/sys现为直接使用，未引入新第三方包装库。缺助手、错误签名／摘要、锁定或系统拒绝不写明文文件、环境变量或参数；交互终端可明确输入yes选择本进程临时模式，无人值守失败。

同家族只协调一次刷新，先耐久unknown再发原RT，安全库替换及非秘密交接完成后才释放请求；仅明确ACCESS_EXPIRED可触发一次，upload／download也用同一协调器。未知／解析失败／安全库写失败保持停止，不复用旧RT、不借初始300秒交付；晚到响应不能进入另一family。初始请求ID在HTTP前保存，默认沿原意图续；需要当前密码／新OTP时显式启动`reauthenticate`，新意图冻结完整旧→新previousRequestIds且实时检查auth_retirement_chain。未知注册不能因本地超时推断账号已建，须原register ID或服务器明确DELIVERY_EXPIRED后受控恢复。

quit／logout立即停本端，尽力删除该安全存储会话条目并撤原family；临时会话没有写入OS条目，只停其运行内存与服务器家族。离线或响应未知保留非秘密待撤销，重启本人新证明先退休旧family。普通SQLite写失败也不能短路安全库删除和在线撤销。当前[刷新报告](../../../项目相关报告/测试/CLI/20261115-刷新家族/README.md)分别记录31组本机测试、真实HTTPS协调器和实际TTY限程会话；Mac系统库正向写未通过，Win／Linux仅amd64跨编译。正式签名／应用授权与目标系统服务仍须分别验证，不把构建或受限模式写成三系统／全I验收。

## 七、<span id="authoritative-sync">权威快照与增量</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

最初的[源码检查点](../../../项目相关报告/测试/CLI/20261116-权威同步/README.md)原样保留。资源恢复后的[本地编译回归](../../../项目相关报告/测试/CLI/20261116-权威同步编译回归/20261116-113743-192d48/README.md)已完成 MacOS arm64 Go 构建及正式 overlay 运行器：45 个顶层本地测试通过（旧 31＋新 sync 14），0 失败，真实 HTTPS／原生凭据库 2 项按本轮范围跳过。仅新隔离 binary，不更新旧发行产物；该本地轮未执行真实服务；后续固定57专项见本节末尾，跨系统原生仍未验证。

`sync` 当前按实时能力安全选择schema2，未支持S2且尚无S2责任时保持schema1；已有S2不降级。`sync-status`只读本机元数据，`sync 会话ID`／`history 会话ID`展示已安装的权威历史。不承担责任时先核实时authoritative_sync／sync_jetstream；旧服务拒绝本次同步，不写新责任。首次持久承担后，能力关闭、缺页或未知不得退回旧轮询。原profile installation 32hex、Auth MessageStore、namespace文件名保持不变；同步另在每个SQLite保存独立canonical UUID physical generation，登记后缺失不能以新UUID绕过原设备围栏。

登记原 requestId、快照原请求在 HTTP 前耐久保存。全 manifest 及 referenceEnvelope／bodyEnvelope／eventEnvelope 先校验原 bytes SHA，再比结构、精确计数和版本；缺页／未知不装正文。末次 validate 分类水位不得低于原水位，首次 changes 权限确认后，projection／连续游标／origin 去重／guard CAS 同一 SQLite 事务安装。暂存页只有引用／摘要；forward 和公告不持久化投影正文或临时 proof，原 Outbox 完整发送意图仍原样保留。

`cached` 及普通导出受当前限制。`local-history 会话ID` 为明确只读入口，仅开放同可信 profile／deployment／本人／personal／epoch 下曾验证且正文 hash 相等的普通副本，不授予云媒体权限。身份待核、恢复代际变化、未验证旧库隔离。global 是永久终态，低版本也先清旧正文；清理意图同事务保存，启动／每轮离线 drain 同来源旧 device 库，失败保留意图。不明旧 hash 或异 epoch 库不猜归属；跨库／文件清理不是跨 SQLite 原子事务，显式下载的外部文件不承诺远程擦除。

原 Outbox 仅完整原路由／原 body 的只读成功事实才确认。辅助查单 503／坏 200 是零 proof，不阻挡已验证的独立 global 安装；自动 sync／reconcile 不创建消息。`retry` 是用户明确动作：先纯查，仅明确 404、当前身份／epoch／库／能力仍有效且原行未冻结时，沿原 ID／body／资格重投，不补新 proof。qualifiedAck／after_receipt 仍为 false；服务端已读、云草稿／偏好、完整迁移及 Mac DPK entitlement 不由本轮源码声明验收。

本地回归补强：旧行缺少原 sendAdmission 时，查单 404 也只冻结，绝不首次消息 POST；原 body／hash 不改。旧测试夹具按完整真实 DTO（UUID 主体、contentVersion、deleted、type）、只读查单路由和 schema4 更新，保留首轮失败日志，不放宽生产校验。运行器默认 Go 测试超时 120 秒，可用 `JOBSIM_CLI_TEST_TIMEOUT` 选择明确时长；真实 HTTPS／系统库仍需显式授权环境。

[固定57真实 HTTPS／TTY 专项](../../../项目相关报告/测试/CLI/20261116-权威同步HTTPS/20261116-120401-3cbd4e/README.md)另通过 10 项限定检查：双向消息／权威同步、本人未提交原意图与 peer 真同 CID 成功分离、global 不消其他原队列、新 device／旧库清理、退群云读拒绝而普通合法本地副本保留，以及后续 global 清历史 holder。坏媒体实际为 403 OBJECT_NOT_OWNED，明确拒绝不标未知。两账号用真实终端明确选择 process-only，不证明 OS 安全库正向；未拦截 CLI HTTP 计数，不宣称零创建 POST。本轮固定服务、原账号、PG／配置均未改。

同一权威同步候选另完成[Linux／Windows amd64 离线交叉编译](../../../项目相关报告/测试/CLI/20261116-权威同步交叉编译/20261116-123709-093a23/README.md)，CGO=0；19 项 Go／模块输入与上述 MacOS／HTTPS binary 相同，无源码改动。只验证 ELF／PE 产物及编译兼容，未运行目标系统程序或凭据库，不能称 Windows／Linux 原生通过。

## 八、<span id="content-receipts">独立正文收讫</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

该切片使用[后端第十节](../../后端/Go/接口契约/README.md#durable-content-receipts)，需当前实例实时声明`durable_receipt_protocol`、`authoritative_sync`和`sync_jetstream`。先正常`sync`登记当前物理库；默认未获存储批准，不自报合格。保持本地schema4、原认证32hex绑定及独立physical UUID，不换device或旧队列命名空间。

| 命令 | 行为与持久化 |
| --- | --- |
| `storage-status` | GET当前device／epoch／physical UUID声明；未批准可返回`qualified:false`和空qualificationId，不生成资格 |
| `storage-declare PROFILE_ID BUILD_SHA256 VERIFICATION_SHA256` | 显式声明部署者已批准的构建／存储验证报告。核本地可执行文件SHA与WAL／FULL，原PUT body／nonce先入`storage_declarations`再HTTP；已有未收口声明须沿原ID续跑 |
| `receive-text 消息ID` | 当前普通纯文本challenge核原bodyEnvelope字节SHA、完整Message、scope／本人／device／store／epoch／qualification／permission及有效期；同SQLite事务保存正文与`receipt_outbox`原nonce／body／摘要，再查原结果及尝试首写 |
| `receipts` | 只列原请求ID、类型、消息ID、epoch和状态，不打印正文、rc1、原body或临时许可 |
| `reconcile-receipt 原请求ID` | 正文始终以完整原body调用只读receipt-results；新协商服务的声明先纯查storage-results，404／503／坏200保留原意图，不创建收据或声明。旧服务声明仅GET当前观察，不冒认原PUT已成功 |
| `retry-receipt 原请求ID` | 本次用户明确动作，先查原结果；仅对应404、仍pending及原资格／内容／期限／本地门禁都有效，才沿原body写一次。声明保留原六字段／nonce，新协商下重核部署批准与当前库探针 |

正文和收据意图达到同一WAL／FULL事务提交点前不发送回执，SQL失败整体回滚。重启后原意图可查／续；不会自动刷新旧challenge或将旧qualification／epoch补成新值。过期未成意图冻结但仍可只读查询历史已成事实；已完成收据遇404不重建。普通同步的相同clientMsgId与正文收讫都不确认正式发送Outbox，也不推进累计收讫位置或把history当已阅。global删除清正文后，原收据只留无正文tuple／摘要与事实，不能凭旧请求复活内容。

部署批准项和本地文件SHA检查不等于运行时认证、远程硬盘／断电证明或OS安全库正向验收。正文收据仍要求`objectComplete:false`、`targetRangeComplete:false`、`deletionEligible:false`；原bootstrap的`after_receipt`保持false，新协商的逐store能力单独核验。普通媒体描述／完整原对象收据见[附件收讫](#object-receipts)，原67项证据不扩大为累计目标或云端删除验收。

### <span id="jobs-toc-heading-10" data-jobs-toc="v2">当前物理库的合格收讫协商</span><!-- jobs-toc:heading-end -->

新增[qualified-receipts-v1契约](../../后端/Go/接口契约/合格收讫/README.md)保留旧bootstrap／sync context／storage顶层false，另取当前store的资格及服务端实时保护。`receipt_negotiation.go`核本人／设备／physical UUID／epoch／schema与正文、原件能力的分别一致性；未批准的普通库继续transport_only和cloud_history。当前build通过部署批准的精确报告后才能声明，服务端profiles列表本身不是本机通过证据。新声明除核可执行SHA与WAL／FULL，还做当前物理库的实际FULL探针；file合同增加随机私有文件的完整读回、文件和目录fsync，探针不生成消息ACK。

| 命令 | 行为、持久化与生命周期 |
| --- | --- |
| `receipt-context` | 只读当前库qualifiedAck、正文／原件开关和独立PG／JetStream保护，当前身份或guard变化拒迟到结果；不改原同步游标或资格 |
| `receipt-profiles` | 本机实际可执行SHA查询部署已批准的精确profile／报告／合同；空列表不自动批准，不声明 |
| `receipt-target 消息ID` | 只读本人原目标／原DG和manifest摘要／正文与原件既成事实；自己ACK不等于所有人完成，不列他人设备 |
| `retention 会话ID` | 读取当前保留策略与CAS版本，默认cloud_history |
| `retention-set 会话ID 所见版本 \| 完整POLICY_JSON` | 先核逐store相应能力；明确n（含0）、对象独立延迟及receiptScope，原nonce／CAS／完整策略在SQLite FULL提交后才PUT，作用于新消息 |
| `retention-operations`／`retention-retry 原请求ID` | 只列本人原责任；未知保pending，显式retry只发原nonce／CAS／JSON一次，已完成只显示原结果。已知版本／权限冲突保失败事实，不能换最新CAS复活旧单 |

正文rc1与原件ro1保留并成对核验`deliveryGeneration/manifestDigest`，原件必须沿父正文的同一目标tuple；原结果缺失或更换该对值保留pending。旧无manifest证明允许空对，不给旧消息补目标。原bodyEnvelope和原摘要在同一正文／receipt意图FULL事务独立存入receipt_bodies，global同事务删除，注销清整个本人库；outbox不增加第二份正文。它只为合法cloud_only普通迁移保留精确旧字节，不重编码猜原SHA，也不继承源资格。迁移先纯查原content／object结果，按原QID历史查询与messageCurrent核DG-MG；批准后来撤回不抹原qualifiedAtCommit，无原目标不补进清理范围，legacy无DG还须原完整permission及fresh明确cloud_only，原始字节未保留的旧记录不能反造。

当前[逐store收讫报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/逐store合格收讫.md)已封52输入、候选测试二进制与完整本地135 PASS／0 FAIL／2环境SKIP，含新增12方法及普通迁移10方法；实际darwin/arm64可执行候选及--help另验，.test与产品SHA分记；本机SQLite／fsync真实，HTTP明确为注入。后端同轮独立双PG／JetStream正反另通过3方法及59相关PG，不冒充CLI已连接该服务或目标系统／断电证明。首轮global夹具133／1／2原证据保留，修夹具未改生产删除逻辑或放松断言。未给生产a4配置任何批准项。

本轮67顶层本地PASS／0FAIL／2项显式环境跳过（含新增12组SQLite／注入HTTP）及MacOS arm64新候选构建见[正文收讫证据](../../../项目相关报告/测试/CLI/20261116-正文收讫/README.md)。未连接真实收讫服务／E2、未实际配置部署批准策略、未做目标OS或设备断电测试；15a4历史55＋13和旧binary不覆盖。

| 局部资产入口 | 默认／代码位置／生命周期 | 当前验证与差异 |
| --- | --- | --- |
| storage-status／declare | 默认transport-only；receipt.go核实时flags与既有UUID，storage_declarations原body耐久后PUT；无results时GET仅当前观察 | 本轮SQLite／HTTP替身及构建；未实际后端批准，不冒充attestation |
| receive-text | receiptValidateChallenge原字节SHA及tuple；stageReceipt同事务正文＋receipt_outbox；不保存第二份正文或改发送Outbox | 提交前HTTP检查、故障回滚、原体重开、身份／guard／global／过期反例通过 |
| receipts／reconcile／retry | 本人原库原请求；自动／普通核对纯查，显式retry才原body写，完成／冻结不凭404新建 | 原nonce／资格不替换，未知不成功，正文事实不等于对象／targets完成；真实HTTPS仍待验 |

## 九、<span id="object-receipts">完整附件收讫</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

默认附件资格为false。部署方必须批准当前CLI构建及验证报告的`native_sqlite_full_file_fsync_v1`存储合同，当前`storage-status`明确返回`qualified:true`、`objectQualified:true`和相同`storageContract`后，才可取得附件挑战。旧`native_sqlite_full_v1`或旧服务省略新增字段仅保正文资格；重新构建／批准新合同后，旧qualification的父收据不能自动继承，须用户明确在新资格下取得正文收据。操作还需实时`durable_object_receipt_protocol`，绝不凭协议存在自证耐久或打开删除。

| 命令 | 行为与原单 |
| --- | --- |
| `receive-message 消息ID` | 普通文本／图片／音频／视频／文件的消息描述正文收讫；核完整原bodyEnvelope／SHA，同事务正文＋message_content原意图。不会把附件描述当成附件bytes，转发仍拒绝 |
| `receive-object 已成父收据ID` | 同device／physical UUID／epoch／当前qualification且已recorded的父正文收据，取得独立ro1后，用现有鉴权下载`/api/media/{objectId}`完整原件，不下载缩略图或forward引用 |
| `receive-object-file 已成父收据ID 本地路径` | 本地路径为命令余下原文，可含空格；原件按完整字节数／SHA导入私有文件。不移动、覆盖或删除用户原文件 |
| `receipt-file-status 原对象收据ID` | 本地重新核描述、完整文件SHA／字节数和耐久调用，分别显示历史serverResultRecorded和当前localFileVerified；不取新挑战／资格，不发送收据。已成事实不保证后来文件仍存在 |
| `receipts`／`reconcile-receipt`／`retry-receipt` | 列表增加对象ID和相对文件路径，不打印原ro1／body。对象始终先完整原body纯查object-receipt-results；只有fresh首写或此次显式retry、对应404且原意图仍pending，才重核原文件和原资格沿原body写一次 |

托管文件放当前SQLite相邻的`ReceiptObjects/{epoch}/{requestId}.bin`，记录可迁移的相对路径。下载前先FULL提交`receipt_object_preparations`，绑定原scope／本人／device／physical UUID／epoch／message／内容版本／对象SHA／相对路径及原ro1；它只承担文件清理责任，不是ACK。SQLite写围栏内核原准备记录／guard后独占创建文件，取消后不重新创建或rename。完整原文件count／SHA、file fsync及目录fsync成功后，同一WAL／FULL事务写`receipt_objects`描述与`receipt_outbox`，并删除preparation；outbox不复制聊天正文或文件bytes。文件到SQL不是跨介质原子：失败或崩溃保留可追踪preparation／清理任务；启动精确退休准备记录并删其私有路径，不从未成文件猜ACK，不扫描用户文件。

重启保原body和文件落点，查询404／503／坏200不当成功；文件缺失、同长改写、替换或symlink时不重发ACK，也不补下载／新挑战。仍可纯查历史已成收据。未知和普通合法retained只隔离云访问并保既有文件；已知global在同SQL事务留下文件清理意图并冻结未成对象收据，启动／同步drain精确删除私有托管副本并fsync目录，失败保清理责任。跨可信旧device库沿原scope身份核验处理，未知旧库不猜归属；用户显式外部下载和导入源文件不承诺远程删除。

对象收据成功只表示独立原对象历史事实，可以是`objectComplete:true`；targetRangeComplete／deletionEligible／after_receipt仍false，不确认消息Outbox／累计位置／已阅。当前preparation修订13对象组＋旧集完整80 PASS／0FAIL／2环境跳过及MacOS候选eb42a9e1见[附件证据](../../../项目相关报告/测试/CLI/20261116-附件收讫/README.md)。原79／39506f及source-only检查点原样保留；没有真实API／PG、目标OS或断电证明，不追认旧67及15a4历史。

| 局部资产 | 默认／持久化／生命周期 | 本轮差异与验证 |
| --- | --- | --- |
| receive-message | receipt.go扩展普通媒体描述，原receive-text仍严格纯文本；默认不表示objectComplete | 完整DTO／原字节SHA、forward／不完整媒体拒绝，本地HTTP替身 |
| receive-object／file | receipt_objects.go；显式file合同，file／dir fsync后FULL事务描述＋原意图 | hash／count／替换／原件保留／父资格／过期／原单未知及重开，79集限定覆盖 |
| receipt-file-status／global清理 | receipt_cleanup.go＋sync_store真实global钩子／启动drain；当前本地文件状态与历史事实分开 | retained／unknown不删、global后不得新ACK、持久清理重开，外部原件不删 |
| object preparations | 下载前原context／ro1／描述耐久，取消与文件创建共享SQLite写围栏；最终ACK事务移除prepare，启动精确清未成文件 | 13对象组＋完整80本地及eb42 Mac候选通过；无真实API／PG |



## 十、<span id="personal-data-export">正式本人数据导出</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

独立接入[正式后端第十一节](../../后端/Go/接口契约/README.md#personal-data-export)，不从本地history／retained或旧实验接口拼包。当前scope／本人／personal／device／epoch／连接profile／physical UUID／session family必须匹配。create／download需实时`chat_export:true`；cap撤回后仍允许原任务纯查与取消收尾。

| 命令 | 行为 |
| --- | --- |
| `data-export` | 新本人任务；完整创建body先WAL／FULL落SQLite，先纯查原单，仅fresh明确404才写一次。有未知原单不静默换nonce |
| `data-exports` | 当前family原请求ID／job ID、最后观察与取消待收尾状态；旧ready观察不证明现在可下载 |
| `data-export-reconcile 原请求ID` | 完整原body只读查单，404／503／坏响应保原意图，不新建 |
| `data-export-retry 原请求ID` | 明确同体重试；只有对应404、未绑定已成job／未取消且原身份有效才原nonce写一次 |
| `data-export-status 原请求ID` | 已绑定job精确GET，未绑定只纯查；source_handoff_pending不显示清完 |
| `data-export-cancel 原请求ID` | 独立稳定取消body先耐久；明确动作先读当前状态，未清完沿相同cancel body续跑，不误用创建body |
| `data-export-download 原请求ID 目标路径` | 当前ready／published及有效期限下申请RAM短票，经鉴权header取ZIP，不进URL／日志／DB；目标存在或被竞争创建都拒绝 |

`data_exports`只保存原创建／取消body、摘要、原身份／family、job关联及剔除未知字段的任务视图，不存票据／正文／ZIP。RT轮换不改变family；重新登录到新device／family不接管旧原单。只读查单不承担迁移、恢复会话或密钥。

ZIP流写目标目录0600私有`.part`，完整count／SHA、ZIP结束记录及每项CRC／安全路径验证后file及目录fsync；再核实时cap、同artifact／build当前ready与本地family／SQLite写围栏，最后no-replace原子hardlink发布及目录fsync。不支持hardlink的文件系统明确失败，不换成覆盖rename。普通失败只移除本次inode的临时名；强杀可能留下用户目标目录的私有`.part`，不自动发布或广扫。发布后的用户ZIP不承诺远程擦除。客户端本地SHA是完整文件摘要，后端未给明文ZIP预期SHA，不能称已比对远端ZIP SHA。

本CLI ZIP流及解压CRC校验总量各限1GiB，超限明确失败、不截断。服务端源SH负责流的当前授权，流后精确状态重验拒已知撤权；服务端状态与本地文件系统没有分布式原子提交，不声称阻止检查完成后的独立源动作。晚到family／epoch／换库结果不得发布。

| 本端资产 | 默认／持久化／生命周期 | 当前验证 |
| --- | --- | --- |
| 原单／状态／取消 | data_export.go；data_exports先耐久原body和family，unknown纯查，显式同体续跑 | 新8组实际SQLite／注入HTTP，原单重开／SQL失败／cap撤回／身份反例 |
| 下载 | data_export_file.go＋auth.go；RAM票据拒redirect，ZIP／fsync／不覆盖发布，失败清本次part | 完整ZIP、MIME／截断／源撤权／晚family／目标竞争；未连接实际API或目标OS |

[本轮报告](../../../项目相关报告/测试/CLI/20261116-正式本人导出/README.md)已保首类型失败、最终同输入88 PASS／0FAIL／2环境跳过及MacOS arm64新候选13cd2438；不借后端／Web真矩阵追认CLI真实HTTPS，原80／79／67候选及历史均保原件。

## 十一、<span id="own-password-change">本人主动改密</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

使用[后端2.3合同](../../后端/Go/接口契约/README.md#own-password-change)，新建需当前实例实时`own_password_change:true`与原已登录本人；密码验证／管理员TOTP由服务端执行，不使用恢复码替代。新密码由服务端按NFC规则校验；CLI不trim／截断，只作UTF8、字节上限和二次输入一致性检查。证明只由TTY读取，不接密码参数，不持久密码对／OTP／无钥密码请求hash。

| 入口 | 行为与原责任 |
| --- | --- |
| 登录后 `password-change` | 捕获原family／profile／本人／device／epoch／physical UUID／namespace，非秘密nonce责任先写Authentication SQLite WAL／FULL；提示当前／新密码及确认，先匿名纯查原结果，再以原捕获AT首写，不自动refresh |
| `password-changes` | 列本人原nonce、设备／epoch、状态与最后结果；没有可用proof或密码查看功能 |
| `password-change-reconcile 原请求ID` | 重新输入原规范化密码对，始终匿名原结果纯查，404／429／未知不创建／重投，不触发业务刷新 |
| `password-change-retry 原请求ID` | 用户明确原单续跑；先匿名纯查，已prepared仅匿名续尾。首请求未到达而仍为原捕获family时，可在重核cap／token／完整namespace后附原AT首次授权；缺原上下文则匿名，401／404继续保责任，不借后来family |
| 顶层 `jobsim ... password-change-reconcile 原请求ID`／`password-change-retry 原请求ID`／`password-changes` | 按同原server／CA／deployment形成profile，提示用户名后只打开该账户非秘密元数据锁，不读vault、不登录或激活family；重启由本人重输原证明，顶层重试为匿名，不能授权尚未见过的新操作 |

`password_change_intents`仅记录公开nonce、用户名、原family／设备／epoch／profile／物理库／相对namespace、尝试次数及结果元数据，没有密码、OTP、密码hash或请求digest。证明送出前先持久unknown并停止原family普通业务；崩溃在原意图与auth状态交接之间时，启动restore gate仍拒自动恢复旧凭据。明确首次拒绝与未知分别保留，未知不换nonce。OTP提示结束后重核cap及原完整上下文，发前再核相同token；改变即拒发送，不借新family。

成功或已安全提交需新登录：只停匹配原family的本地凭据，删除安全库前核其中仍是该原family，并仅退休同原identity／physical的消息空间。旧结果不清后来登录的family、vault项或新namespace；服务端不发AT／RT，客户端不自动代登录。`pending`不当作全部完成，`current:false`不回滚当前账号状态。

[本轮记录](../../../项目相关报告/测试/CLI/20261116-本人主动改密/README.md)：最终源码注册99顶层方法，与原raw唯一PASS／SKIP名称集合完全一致，实际97 PASS／0FAIL／2环境跳过，Go退出0；MacOS arm64新候选71500985构建通过。旧提示与Go PASS粘行导致计数器少报，原95／96 JSON及旧7f98不改，另有计数复核。无真实改密HTTPS、真实TTY交互或OS安全库正向，两个跳过不由旧证据补齐。

| 本端资产 | 默认／持久化／生命周期 | 验证范围 |
| --- | --- | --- |
| 主动改密／原单 | password_change.go；默认不发起；Authentication SQLite仅非秘密责任，证明仅TTY内存 | 新8顶层组含SQL失败、无密码／hash、原单重开与原身份门禁；网络均注入 |
| 显式原单重试 | 先匿名纯查；仅原family／token／device／epoch／profile／namespace／physical仍匹配才首次授权，OTP后再核 | pre-delivery丢请求后原家族首建、后继家族无AT、OTP期间换family／cap拒写 |
| 停旧凭据／restore gate | unknown先停普通业务；只删除原family安全库项／退休原物理namespace，旧结果不碰后继 | 真SQLite交接与fake vault反例，不冒充真实系统库或设备验证 |

当前[完整本地回归](../../../项目相关报告/测试/回归/20261116-全项目回归/Web-CLI.md)重新执行11份外置测试、99个顶层方法：97 PASS／0 FAIL／2环境SKIP，方法集合与源码逐项相等。真实HTTPS协调器与原生安全库诊断仍未运行；本地重跑不扩大正式导出、主动改密或对象收讫的真实服务验证范围。


## 十二、<span id="own-account-deletion">本人注销</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

登录后`account-delete`确认申请；`account-deletions`、`deletion-reconcile REQUEST_ID`、`deletion-retry REQUEST_ID`、`deletion-query REQUEST_ID`、`deletion-cancel REQUEST_ID`和`deletion-cancel-retry REQUEST_ID`也可从启动参数进入本机原任务。口令／恢复码／OTP只经TTY读取，禁止秘密参数；原nonce／family／物理库／epoch和cancel版本公开责任WAL／FULL耐久。未知先查，明确续原单；受限proof只RAM，completed精确清原本人所有本机store及托管ReceiptObjects，其他profile／user和外部文件保留。

最后实跑全runner115通过／0失败／2环境跳过、注销11方法通过（含一次恢复码unknown取消），host二进制已构建；其后main.go仅将未保存默认URL改为9443，此默认值增量未重编。当前磁盘4GiB门禁不运行新测试／编译。详见[独立限定报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/CLI注销.md)，不把HTTP／vault适配当真实HTTPS／安全库或TTY操作通过。


## 十三、<span id="personal-migration">普通端侧迁移增量</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

新增`migration-*`配对、创建、目标导入、原单查询／显式重试／取消、独立历史及完整原件保存入口；选择文件只列原ID／contentVersion／objectMode。密码／OTP／proof／key只留本次RAM，默认保源且不授予合格ACK，失败不换原nonce。原件和JSON原片段经长度／SHA／GCM核验，目标FULL整批发布不改游标／Outbox，global优先清stage和独立副本。当前仅候选编译，7个新增方法尚未执行，真实双设备和新成品待验；[入口、边界与验证](../../../项目相关报告/实现进度/20261116-剩余能力闭环/CLI普通迁移.md)。

<a id="🔚" href="#前言">返回首页</a>
