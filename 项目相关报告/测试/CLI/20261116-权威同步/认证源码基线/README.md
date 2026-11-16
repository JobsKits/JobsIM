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

对于前三类治理冻结，`reconcile 原clientMsgId` 只用当前身份和恢复代际下的历史 GET 分页寻找原 sender＋clientMsgId，不发送消息 POST、不中途覆盖其它历史。最多 1000 页、每页 200；未找到仍是未知，不能证明未提交，原行保持冻结。找到既成事实可确认原行；退群后的核对不会重新填回已撤的本机历史。恢复代际和服务端准入拒绝的既有对账规则继续保留，所有路径均不换原快照复活旧消息。

## 三、<span id="outbox">Outbox与范围</span> <a href="#前言"><b>🔼</b></a> <a href="#🔚"><b>🔽</b></a>

当前使用 pure-Go SQLite（modernc.org/sqlite 1.62.0、对应 libc 1.80.1），启动核验 WAL 与 synchronous=FULL。当前本地 schema3 从 schema1 增量升级：schema2 添加独立冻结原因与原治理操作表，schema3 添加公告原操作引用表；旧待发字节和摘要保留。待发送原载荷和草稿清理在同一事务提交后才发 HTTP；消息正文和连续游标同事务，字符串大 ID 和版本不转浮点数、不倒退。新认证目录绑定经验证地址／CA／deployment档案、用户与个人来源；消息namespace还绑定服务器实际deviceId和store_generation。重新认证取得新device前冻结旧设备／旧epoch及legacy队列，保留原ID／载荷／摘要，不迁移到新设备自动发送。系统凭据仍在而消息库缺失时也要求本人重新验证、退休旧family，不以空库继承旧设备资格。完整合格ACK和旧备份防回退登记仍依后端后续公开闭环验收。

`outbox`查看，`retry 原clientMsgId`重试；恢复代次变化冻结旧队列并保留旧历史，`reconcile 原clientMsgId`显式核对旧结果，始终携原代次和原ID，不重新生成。`cached 会话ID`、`draft 会话ID 内容`、`drafts`、`sync 会话ID`分别操作当前空间缓存、草稿与增量游标。服务器权限拒绝或恢复代次错误不会伪装成本地成功。

旧JSONL文件不会自动删除或重发；核实来源确属当前服务器后，可用 `import-outbox 原路径 | 当前deploymentId` 原子迁入，校验用户/部署/载荷及源摘要，重复迁入或损坏整批拒绝，原文件保留。普通数据目录默认为用户配置目录，可用 `--data-dir`显式指定。第三方原始许可和摘要见[清单](./第三方许可/清单.json)。

搜索协作切片的 21 组 SQLite／故障与 HTTPS 限定证据见[搜索协作报告](../../../项目相关报告/测试/CLI/20261115-搜索协作/README.md)，此前普通转发 17 组与治理链各保留原范围；本轮当前计数见下述刷新报告，不跨轮加算。完整测试入口见[CLI测试](../../../测试/CLI/README.md)，入口定位见[代码地图](./代码地图/README.md)。当前网络失败按原身份保留未知；未提供前端完整本地演示回退，真实API失败不假报发送成功。CLI是终端客户端，不提供图形主题／播放器／摄像头UI。完整快照、合格收讫回执、二维码及全部I范围仍须单独验收。

附件可 `upload 文件路径` → `send-media 会话ID image|audio|video|file 对象ID 说明`；`download 对象ID | 目标路径`鉴权下载，拒绝覆盖已有文件。`reply 会话ID 源消息ID 内容`、`message 源消息ID`核对内容版本后，`forward 会话ID 源消息ID 源内容版本`遵守服务端能力开关；`unpin`／`unfavorite`／`block`／`unblock`／`sessions`／`revoke`完成对应操作。`recover`在登录前输入恢复码及新密码且不自动登录；登录后`recovery-codes`要求当前密码，只在当次终端显示一次。管理员若收到MFA挑战再输入验证器验证码，验证码不进入普通配置或数据库。

每条新消息在严格入队前取得当前会话的 `sendAdmission` 不透明快照，之后重试／重开／显式原ID对账均使用原值；无快照的旧记录不从最新资格补齐。服务端返回 `SEND_ADMISSION_CHANGED` 后持久冻结，普通retry不再发，显式reconcile也不能换资格创建旧消息。每次发送／对账先核对当前实例的 `send_admission:true` 能力，旧服务不接收POST。资格读取失败时保留草稿，不把无法取得资格的新消息标为已入队。


[普通转发限定验证](../../../项目相关报告/测试/CLI/20261115-普通转发/README.md)覆盖原引用入队、两类未知结果、原proof冻结和转发缓存无正文。`download-forward 消息ID | 目标路径`每次重新鉴权下载；普通展示去除临时proof，转发离线缓存保持占位。明确下载的本机文件不承诺远程撤销。

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

<a id="🔚" href="#前言">返回首页</a>
