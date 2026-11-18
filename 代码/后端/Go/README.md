# JobsIM Go 后端

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、构建与运行](#run)
- [二、角色与数据所有权](#roles)
- [三、接口与当前范围](#interfaces)
- [四、备份与隔离恢复](#operations)
- [五、验证与剩余I缺口](#verification)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本工程是[现行需求](../../../需求/IM需求明细表.md/IM需求明细表.md)的首批可执行实现。使用[**Go**](https://go.dev/)及真实[**PostgreSQL**](https://www.postgresql.org/)，没有内存或SQLite服务端替代。它提供独立角色启动和开发联调；当前未达到完整I验收或强保护生产准入。

收讫链路当前区分正文描述和完整原件：[对象协议](./接口契约/README.md#durable-object-receipts)需新的文件耐久批准、同物理库既成正文事实及独立原件字节证明。实现／固定输入／待验证项见[中央检查点](../../../项目相关报告/测试/后端/20261116-完整对象收讫/README.md)；发送时冻结目标、累计ACK、编辑代际与云清理worker已实现，启用值由实际物理库批准、JetStream和独立删除事实就绪派生；未满足时仍关闭。当前验证及边界见[收讫清理](../../../项目相关报告/实现进度/20261116-剩余能力闭环/收讫清理.md)及[媒体物理清理](../../../项目相关报告/实现进度/20261116-剩余能力闭环/媒体收讫清理.md)。旧件只读白名单不补造期限；既成global按原job/event独立事实清理。正式本人导出已有独立后端验证，五端入口仍须接线，不能与收讫能力合并宣称基础版完成。

[逐store合格协商](./接口契约/合格收讫/README.md)使用独立`receipt-context/profiles/targets`和声明原单`storage-results`，不再借永恒false的旧`after_receipt`开关阻断实际资格。运行保护核当前message／journal的真实PG system identifier、fsync／full_page_writes／同步提交、相同epoch和JetStream发布／消费连接；同PG两库不能冒充独立journal。正文与原件分别批准，策略仅当前actor原store与相应实时保护齐备才能新建，已成原nonce仍先查原结果。rc1／ro1及结果沿原DG／manifest绑定，旧传输接口保持兼容false。源码和当前验证边界见[新协商报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/逐store合格收讫.md)，不自动批准任何生产端。

本人[主动改密](./接口契约/README.md#own-password-change)后端已接两条入口、原责任恢复和全family撤销，4纯／12真实PG及生产构建通过；[实际边界](../../../项目相关报告/测试/后端/20261116-本人主动改密/README.md)保留原Cookie反例失败及源冻结。五端产品入口、真实HTTPS与新改密独立恢复仍待验。

[稳定媒体上传](./接口契约/README.md#stable-media-upload)提供能力查询、固定nonce／原身份／真实SHA绑定、纯查原结果与明确重试，旧无nonce上传继续兼容。prepared原体、no-replace发布、owner有界擦除与恢复拒复活已接通；[本批验证](../../../项目相关报告/测试/后端/20261117-稳定媒体上传/README.md)覆盖真实浏览器容器的实际分类与进程内上传、隔离数据库反例及双cmd构建，真实HTTPS与两端持久队列重启仍另验，不与前序注销数量累计。

## 一、<span id="run">构建与运行</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

构建采用Go 1.27.4；依赖锁在`go.mod`和`go.sum`，公共SDK与模块缓存放在项目外。5个直接模块的版本、许可原文和来源见[第三方依赖清单](./第三方依赖/README.md)；x/sys由Windows文件锁直接使用，nats.go用于自托管JetStream指针事件。依赖选取核验入口：[Go发行](https://go.dev/dl/)、[pgx v5.13.0](https://github.com/jackc/pgx/releases/tag/v5.13.0)、[x/crypto](https://pkg.go.dev/golang.org/x/crypto@v0.59.0)、[x/text](https://pkg.go.dev/golang.org/x/text@v0.44.0)、[nats.go v1.55.0](https://github.com/nats-io/nats.go/releases/tag/v1.55.0)。版本锁表示本次采用，不代表以后无需复检。

恢复与普通迁移构建均保留实例4GiB磁盘余量。[**MacOS**](https://www.apple.com/macos/)／[**Linux**](https://www.kernel.org/)容量探针使用系统`/bin/df`，不依赖`PATH`中可能同名的用户命令；Windows沿用系统磁盘接口查询。容量无法确认、预算为负、余量不足或请求取消时保持拒绝，不以探针失败放宽门禁。验证入口与限定范围见[本轮回归](../../../项目相关报告/测试/回归/20261118-全项目回归/README.md)。

```shell
go build -o /选择的外置目录/jobsimd ./cmd/jobsimd
go build -o /选择的外置目录/jobsimops ./cmd/jobsimops
```

运行必须提供`JOBS_IM_DATABASE_URL`与`JOBS_IM_JOURNAL_DATABASE_URL`。后者必须是另一[**PostgreSQL**](https://www.postgresql.org/)集群；启动核对真实system_identifier，第二数据库或另一个schema不能冒充独立日志。检查fsync/full_page_writes并强制会话同步提交。连接串只经环境传入，不写源码、收据或正常日志。

```shell
jobsimd --service=all --addr 127.0.0.1:8443 \
  --state-dir /选择的外置私有数据目录 \
  --tls-cert /受控证书/server.pem --tls-key /受控证书/server.key \
  --web-dir /构建后的Web目录
```

`JOBS_IM_PUBLIC_URL`指定浏览器实际同源HTTPS入口，不能用API内部地址替代；Web静态产物只由gateway托管。`--web-dir`也可由`JOBS_IM_WEB_DIR`传入，支持SPA回退但不列目录、不开放隐藏文件／越界符号链接／未允许扩展；未知API保持JSON错误。TLS验证保持开启；受控本地CA须由客户端核对并单独信任，不静默安装系统全局CA。`--insecure-loopback`仅接受显式回环IP，用于本机测试，不能用于远程原生联调。

同步基础依赖是本机自托管[**NATS JetStream**](https://docs.nats.io/nats-concepts/jetstream)，采用[官方nats-server v2.15.3发行包](https://github.com/nats-io/nats-server/releases/tag/v2.15.3)及同版SHA256SUMS校验。先通过私有配置文件启动回环broker，持久store_dir放实例运行数据目录，不能放易清理Caches；Unix的server私有配置和后端tokenfile均0600且归当前用户；WindowsOS由真实owner／DACL限制为当前用户、SYSTEM及Administrators，不用FileMode假装权限。两份文件保存同一个随机token，不能把token写argv／URL／日志。后端读取`JOBS_IM_NATS_URL`与`JOBS_IM_NATS_TOKEN_FILE`，环境只传无秘密URL和文件路径。基本配置文件存储单副本、stream 7天／256MiB；worker精确核验既有配置，冲突拒绝不覆盖。`--service=all`增加独立sync角色；未配置总线时同步flags为false且接口503，不借HTTP补偿冒称已交付NATS。现三平台部署入口的预检／托管接入由部署模块单独验证。

```shell
nats-server -c /本实例私有目录/nats.conf
export JOBS_IM_NATS_URL=nats://127.0.0.1:4222
export JOBS_IM_NATS_TOKEN_FILE=/本实例私有目录/nats-token
```

## 二、<span id="roles">角色与数据所有权</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 角色 | 数据归属 | 默认内部端口 |
| --- | --- | --- |
| identity | auth：密码、设备、会话、恢复批次 | 8101 |
| directory | directory：展示资料、联系人、拉黑 | 8102 |
| conversation | conversation：会话、成员、历史资格 | 8103 |
| message | message：消息、事件序号及本批互动实现 | 8104 |
| media | media：私有文件字节及受控授权 | 8105 |
| control | control：真实能力查询、版本CAS及绑定强验证的管理写 | 8106 |
| journal | 独立集群journal：最小安全intent/result与恢复代际 | 8107 |
| governance | governance：最小举报；正式本人 ZIP、独立实验包及受控清理 | 8108 |
| sync | sync：设备库绑定、无正文来源事件、快照页及本人增量流 | 8109 |
| gateway | 无业务表；公开API代理与同源Web | `--addr` |

可分别以`--service=identity`等运行独立进程；`all`是开发编排，启动多个独立HTTP角色和数据库池，不共享跨schema业务事务。内部调用使用受鉴权HTTP且仅监听回环；未声称跨机mTLS与每角色独立密钥隔离已验收。`--internal-base-port`调整整组9个内部端口。各业务角色可用`JOBS_IM_IDENTITY_DATABASE_URL`等覆盖其数据库连接，治理域对应`JOBS_IM_GOVERNANCE_DATABASE_URL`、同步域对应`JOBS_IM_SYNC_DATABASE_URL`；独立日志固定使用journal专属环境变量。实际验过的备份布局是一个业务DB的独占owner schema＋另一个真实PG集群的journal，多业务DB聚合备份尚未实现。开发默认同一业务数据库不等于已验证最小权限部署。互动暂归message实现切片，独立interaction运行角色仍未交付。

发送准入围栏要求全部角色在同一主机共用同一个规范化本地StateDir；启动验证回环角色地址及OS文件锁竞争能力，关键内部RPC校验同路径证明，错目录拒绝。发信持共享锁至PG事务提交和结果读取，关系／成员撤权及新授予持排他锁；该锁跨同机角色的不同PG连接／数据库生效，没有借用跨数据库不共享的advisory锁。单一全局锁的吞吐、Linux／Windows原生行为及跨主机分布部署尚未验收，不支持静默切换到远程角色或无共享状态目录。

## 三、<span id="interfaces">接口与当前范围</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

完整路由与对象见[HTTP契约](./接口契约/README.md)。本批已写本地用户名／Argon2id密码、私有会话摘要、短期AEAD凭据再交付、Cookie／CSRF／浏览器确认、恢复码、联系人接受／拒绝／拉黑、明确收件人控制的入站门禁重开、群成员及转让、消息原ID幂等、事件游标、编辑CAS、回复、撤回、反应、共享置顶／个人收藏、授权过滤搜索、私有附件与权威昵称聚合。接收者不会凭猜到objectId取得未提交附件；互动消息版本／序号与事件同事务推进，不能用旧版本覆盖增量状态。

有覆盖清单的权威快照与增量由独立sync owner实现。owner事务锁行clock／纯引用Outbox、真实JetStream PubAck后推进、消费落盘origin后Ack；HTTP只补偿持久原指针，冲突／中段缺行拒绝推进。快照跨owner双采集证明共同cut，分页只存引用密文及固定manifest，正文逐次当前ACL／版本重投影；原字节envelope分别校refs、事件和动态body。设备／本地库绑定、完整页检查、final validate、原边界衔接、权限ABA及恢复epoch均分开，不把GET或canInstall当本地存储证明。固定schema1覆盖外的read／云草稿／偏好／完整peer资料流、qualified ACK、导出／迁移／注销继续按实际切片追踪；安装端仍需真实原子事务及即时权限重验。具体流程和错误见[同步契约](./接口契约/README.md#authoritative-sync)，测试方式见[同步测试](../../../测试/后端/接口/sync_snapshot.py/README.md)。

候选schema2增加本人read／稀疏观察／会话view和字号偏好，保持原device/store与S1独立stream；原body只读查单、真实message创建tuple和精确当前未读、独立manualUnread代际均有[20组PG验证](../../../项目相关报告/测试/后端/2026-11-16-S2真PG定向/README.md)及[8组双PG／HTTPS／JetStream验证](../../../项目相关报告/测试/后端/2026-11-16-S2双PG与真实同步/README.md)。实际125项JSON HTTP检查／334语义／17页，另3条零读取丢响应写及1条CAS预期冲突分列，不相加为产品项。69份生产源未因该验证改动；固定57和7ef域未升级，候选测试域已按精确身份停止，端侧schema2接入仍未验证。该8组证据当时尚未挂草稿；当前基础文本云草稿另按下一段与9.6的限定闭环验证，不将原候选解释为全I交付。字段见[9.5契约](./接口契约/README.md#sync-self-state)，复现见[S2测试](../../../测试/后端/接口/self_state_sync.py/README.md)。

基础文本云草稿已接公开GET／PUT／DELETE／原完整body只读查单、独立CAS及加密prepared intent、无正文journal floor、message holder与源／成员／账号／forward后代清理worker，并纳入S2指针同步。源失效只撤reply，不误删本人独立text；未发布退休text经显式recovery，不自动改正式Outbox。真实[草稿验证](../../../项目相关报告/测试/后端/20261116-云草稿/README.md)另含新加密backup／offline restore、旧clear／digest／reply floor拒复活和新epoch快照；final只比已验候选增加cloud_drafts能力标记，原矩阵与窄重验分列。字段见[9.6](./接口契约/README.md#cloud-text-drafts)，测试见[草稿测试](../../../测试/后端/接口/cloud_drafts.py/README.md)。云附件上传队列、正式导出、注销、普通迁移、qualified ACK及全I仍未因此完成。

`GET /api/contacts/{peerId}/relationship`只返回本人的拉黑／好友／入站门禁与当前重开代际元数据，供界面从服务端恢复真实状态；不读取或返回对方私人策略，不改变资料／头像／通信授权，也不创建默认关系行。

搜索先从conversation owner取本人完整有效读取范围／群加入下界，在message owner过滤后分页；不会先取全库100条再删掉他人命中。本人收藏由独立`GET /api/favorites`读取当前可见源引用，空列表200，不借空搜索词。两者返回`items/cursor/hasMore`，签名游标绑定本人／epoch／条件；逐条返回前重验源ACL。`GET /api/conversations/{id}`与`GET /api/messages/{id}`支持精确定位较老来源并再次鉴权，不能从旧列表直接恢复已失权正文。当前为PostgreSQL子串搜索，全文索引、拼音与大容量性能未验。

群角色授权另持roleGeneration：群主转出再转回会推进两位参与者各自角色代际，sendAdmission绑定该原代际，不能借后来返回的owner资格首次发送旧@all。公告新请求也冻结原roleGeneration，缺失／旧值拒绝未建操作；已完成旧摘要先查事实，不补旧body。bootstrap只读role_admission用于区分旧send_admission-only服务；旧无字段队列只读查原结果，新写需新意图。见[角色专项证据](../../../项目相关报告/测试/后端/角色准入代际实测.json)。

结构化提及仅来自已选成员ID，保存发送时目标资格代际，当前blocks抑制定向提醒而不删除共同群正文；群主@all有独立限频。群公告仅保存本群消息／内容版本／到期引用，当前源失效或失权时只回占位。`Message.contentVersion`独立于反应／置顶／个人收藏的展示版本，互动不会打掉群公告。bootstrap的mentions／announcements为只读实现标志，新发布仍复查父能力与群权限。冻结消息通过只读`POST .../message-results`核对原事实，未知404绝不首次创建；字段与默认配置见[接口契约](./接口契约/README.md)，实际范围见[提及公告报告](../../../项目相关报告/测试/后端/提及与公告实测.json)。

普通转发采用纯引用新分支，覆盖文本、图片／动图、音频、视频、文件及最多8层链；forwarding只读开启，旧text_forward保持false。Outbox冻结源读者proof＋contentVersion和目标原sendAdmission，未存／失效证明不借后来资格补齐；未知先查原body，用户明确重试才沿同ID／body重投。目标无需源群资格，服务端逐层核原转发者成员／安全／恢复代际，媒体经目标消息代理原字节而不下放源对象ID。编辑／撤回／源成员退出／账号安全撤权持久失效后代，message后台分批恢复pending，不能按超时租约假报排空完成。公共JSON及媒体流的共享围栏保持到响应结束，源内容写排他等待排空；同机范围与单全局锁吞吐限制不变。

forwardUnavailable是本次投影占位，临时依赖故障也可200返回；端侧清旧正文／sidecar但不永久锁存该boolean。恢复只能依据当前精确GET及本地版本／请求CAS，晚到历史或发送响应不能复活旧缓存。搜索／收藏未知整页503，确定失效则继续扫描；实验导出包含forward明确failed并清prefix，链级导出holder仍后置。完整字段、错误及恢复语义见[转发契约](./接口契约/README.md#普通引用式转发)，真实证据见[转发测试入口](../../../测试/后端/接口/forward_refs.py/README.md)。

管理员通过受控`bootstrap-admin`初始化TOTP，登录及高危写分别验证，验证码时间步单次消费，step-up绑定目标／载荷／版本／设备／原操作／安全代际。封禁／墓碑先停止新准入并执行跨owner清理；恢复墓碑保留处罚，旧会话不复活。受控密码重置只发600秒单次凭证，不设共享密码、不代登录、不重置MFA。静态头像走独立profile prepared／媒体真实解码裁剪／CAS完成，原操作事实可查；最小举报目前只持用户选定目标与理由，不复制聊天正文作为治理后门。

Web未登录register/login/recover使用同源Origin＋JSON＋`X-JobsIM-Client:web`，不先获取已登录CSRF，也未声称实现匿名挑战；成功SetCookie后先GET csrf再POST confirm。原生使用bearer无需浏览器确认。`GET /api/auth/context`提供真实设备ID和恢复代际；不返回token。`GET /api/me`保持flat User。

访问／刷新以只读auth_refresh标识新协议：AT默认15分钟，RT闲置30天、首次认证绝对90天。稳定family／session内单代原子消费，used摘要继续定位家族；重用只撤相应家族，独立设备不连坐。未知不重用旧RT，须本人重新验证关联撤旧；初始认证300秒交付单独实现，不用于refresh。旧7天单token迁移明确失效，不能被升级为RT。Web只有不透明BFF Cookie，上游凭据对密封在identity；access过期后通过受CSRF／Origin保护的refresh-context／refresh继续，浏览器JS不拿AT／RT。

Web已激活请求必须X-JobsIM-Auth-Family绑定原标签身份，Cookie被其他标签换账号时409 AUTH_CONTEXT_CHANGED，不能把另一账号资料写回旧库或误logout新家族。原生也按本地捕获的family／身份代次拒绝迟到刷新和业务重试。初始认证unknown使用previousRequestId关联本人原事实；多层未知在只读auth_retirement_chain下冻结previousRequestIds完整闭包，未建前序也先持久退休，迟到旧请求拒绝。退休日志独立重放，防旧快照复活未知认证链；identity后台清到期密文并恢复家族撤销责任，不按300秒删除非秘密映射。字段与错误详见[刷新合同](./接口契约/README.md#session-refresh-implementation)、[Web身份围栏](./接口契约/README.md#bff-refresh-implementation)，实际证据见[刷新测试](../../../测试/后端/接口/session_refresh.py/README.md)。

恢复代际为十进制字符串。历史游标是`epoch:sequence`；旧代际或非零旧裸游标返回409 `RESTORE_EPOCH_CHANGED`。发送需记录创建Outbox时的`restoreEpoch`；仅初始epoch=1兼容缺省。服务端先核对原幂等结果，对不存在的旧epoch发送拒绝创建；客户端不得换ID重发。新消息ID由实例秘密、原sender／clientMsgId稳定派生，附件保留不会因重试改成另一个待提交消息ID。

新消息还必须冻结Conversation的本人sendAdmission；群退重入及direct删友／拉黑后重新接受都会使旧快照失效。先按当前读取ACL核既成原ID，缺失／失效快照的未建消息返回`SEND_ADMISSION_CHANGED`永久冻结，不用最新资格修补旧队列。bootstrap／capabilities只读`send_admission=true`是端侧必须核验的安全能力，不能被管理员关闭。旧服务没有该标志时，不向它POST旧缺快照队列。

聊天图片原件限20MiB、文件总上传限64MiB。检查图片像素、GIF帧／时长／累计像素，MPEG4按真实音轨handler区分AAC/M4A与视频，不按扩展名或客户端MIME放行。头像当前仅静态标准JPEG/PNG≤2MiB／16MP，重编码512／128 JPEG并剥元数据，使用中头像不套聊天附件保留期；当前好友／共同有效会话可见且blocks优先拒绝，普通media接口不能旁路。E2EE、收讫删除、RTC、邮件、厂商推送及强保护不能被能力接口冒充开启。

正式本人数据导出后端已接`/api/me/data-exports`六条入口，最终候选`chat_export=true`：当前本人公开资料／设备元数据、静态头像、关系、获权会话／成员、本人view／read／已应用文本草稿／字号偏好、本人举报理由与公开状态及当前普通消息／附件；forward只出占位。服务端私有加密制品完整认证后经TLS交付可用ZIP，普通客户端不接收解密key；原完整body只读查单、源版本holder／独立journal、24小时／60秒期限、同步源排空与持久清理、恢复拒旧均有[独立真实证据](../../../项目相关报告/测试/后端/20261116-正式本人导出/README.md)。具体DTO与范围见[正式导出契约](./接口契约/README.md#personal-data-export)。五端正式导出入口仍需接线，不能据后端切片宣称完整I。

`--experimental-exports`继续单独控制旧受控聊天包协议，默认不注册五条实验入口，不由`chat_export`代替。实验只支持普通获权聊天与附件，包含forward明确失败；正式包的forward占位不表示链级源正文导出已交付。正式与实验purpose、任务原nonce、key交付和下载入口互相隔离；关闭实验仍保留旧责任清理。

## 四、<span id="operations">备份与隔离恢复</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

`jobsimops`是显式授权的离线维护入口，使用维护权限；不能把其全库备份／分owner恢复权限授予普通运行角色。备份使用真实pg_dump一致性快照，登记owner覆盖、快照LSN及独立日志through／chain anchor，核对私有对象字节、实例秘密与TLS材料，输出分块AES-GCM认证加密归档；LSN是记录的快照信息，不冒充已交付PITR链。独立日志及恢复密钥不打回旧备份。TLS目录必须有server.pem／server.key，实例专用ca.pem／ca.key存在时才纳入，不要求持有公信CA私钥。恢复密钥必须另存，不进入备份目录。失败不覆盖既有成功集。

`StateDir/Admission/Pending/`是提交前fsync的原事务标记，包含原XID／原消息摘要／原目标库绑定；不可作为临时文件清理。正常提交确认后删除并同步目录；未知或进程崩溃保留，后续准入必须先证明PG原事务committed／aborted且原消息事实一致，in-progress／不可证明／错库均拒绝。备份和隔离恢复取得排他围栏且拒绝未决标记，不把它们丢弃后伪装成完整备份。可在原实例、原message数据库上显式运行`jobsimops --mode reconcile-admissions --state-dir /本实例State`，只核原结果不重发消息；独立message库使用`JOBS_IM_MESSAGE_DATABASE_URL`，否则取业务URL。改连接串／迁移前先在原连接完成核对；人工更换库、坏标记和无法证明的旧XID保持门禁，不能按TTL或手动删标记宣称恢复完成。

```shell
jobsimops --mode backup --pg-bin /已验证的PG工具目录 \
  --state-dir /本实例State --tls-dir /本实例TLS \
  --file /备份目录/本次.jimbackup --key-file /单独保管目录/recovery.key
```

首次恢复接受停掉源角色、无源／目标运行连接、另一个空目标数据库和空目标State；同一稳定operation-id可续本操作独占的目标，不能换备份／数据库／State，也不覆盖无归属的非空目录。先验证完整认证归档和逐文件清单，再以独立journal操作记录只分配一次恢复epoch；pg_restore单事务成功但响应丢失后可依据本操作独占的目标checkpoint继续。重放核对快照anchor和完整日志tail，缺最新密码标recovery_required；缺较新编辑正文以contentUnavailable占位，不退回旧文。所有旧会话／step-up／重置凭证及短期交付失效。短期派生导出不恢复ready/旧key，离线旧source与target前缀均按原操作取消清理后才激活；失败保留隔离诊断与门禁。没有自动HA、跨故障域RPO0、PITR链或全资产灾备承诺。目标连接通过`JOBS_IM_RESTORE_DATABASE_URL`传入。

```shell
jobsimops --mode restore --offline --operation-id 原稳定维护操作ID \
  --pg-bin /已验证的PG工具目录 --state-dir /源State \
  --target-state-dir /新的空State \
  --file /备份目录/本次.jimbackup --key-file /单独保管目录/recovery.key
```

## 五、<span id="verification">验证与剩余I缺口</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

测试入口与证据见[后端测试](../../../测试/后端/README.md)，源码定位见[代码地图](./代码地图/README.md)。已有真实HTTPS／PG核心55、管理员与群墓碑49、重置凭证20、头像／最小举报51、备份／恢复27项HTTP断言及外置Go安全／新库迁移／CLI测试；导出另有91项公开实验生命周期和21项私有协议／文件栅栏证据。各报告范围不同且包含重复，不能相加为产品验收数，不能据此宣称完整I或243主项通过。

发送准入另有52项真实HTTPS断言及外置Go锁／原事务标记测试：退群重入、direct两轮撤权、旧摘要兼容、PG阻塞提交与撤权并发、请求取消、真实子进程崩溃后锁释放与原事务恢复。基础55项HTTP已按冻结原快照重新通过；这些证据不扩大灾备或分布生产承诺。

引用式转发新增185项HTTP／104语义、慢公共JSON与持久续跑218项HTTP／12语义、隔离真实备份恢复33项HTTP／16语义；外置Go核5类JSON响应持锁／取消／伪造证明及200临时占位同version恢复。最终候选构建另复跑核心55、分页279、当前role／提及／公告77与实验导出明确拒绝11项HTTP，历史报告未覆盖写回。报告范围有重叠，不能累加成产品验收项；每份证据记录其源码／二进制快照，后续修改只可沿实际验证范围解释。

尚未完成完整I的独立interaction部署、细粒度服务身份／最小权限、全部账号数据域覆盖与设备耐久收讫、完整群角色／禁言与独立治理接管、陌生文本消息请求与完整隐私矩阵、举报证据／申诉、MFA登记轮换及受控MFA恢复、五端正式导出接线与普通迁移、五端注销接线／本地与离线副本清理、全文拼音搜索、转发链级导出holder及全媒体跨引用物理清理。固定schema1同步覆盖不含read／云草稿／偏好及完整peer资料流；候选S2只补上述本人状态后端切片，五端接线、真实S2恢复、原子安装与存储资格仍须实际验证。消息收藏暂借共享展示版本，细分个人同步版本仍需补；跨owner旧准入工作的全量排空／精确历史重放及全部离线副本／备份覆盖也须继续验。当前不标生产准入完成，`safe_restore`／`strong_protection`继续关闭。


本人完整注销新增独立 `account_deletion*.go`：7天默认／0～30天配置的Governance冷静期，当前本人密码＋原nonce发起、匿名原体只查、5分钟用途绑定的受限密码／单次恢复码／管理员审计证明，取消与processing的同一行仲裁，以及message／media／directory／conversation／sync／governance最终清理。取消不解封、不恢复旧family；独立journal重放后仍不复活已完成注销或媒体字节。完整字段见[本人注销契约](./接口契约/README.md#own-account-deletion)。最终10项真实PG方法全过，含33源消息／33头像分页与恢复字节重清，RPC为进程内生产handler，journal在隔离PG独立schema；五端入口、真实HTTPS／设备、本地与离线副本另验，不能借此段认定全I交付。

<a id="🔚" href="#前言">返回首页</a>

原生八域CMS协调恢复提供 `deployment-restore-plan/deployment-restore`，保持current ledger一次epoch交接、原source拒普通启动与原单续作。隔离八域及最新事实恢复、目标规范Mac入口真实启动／原CA严格HTTPS、旧凭据／旧epoch拒绝、原媒体字节及停止保留已完成一轮主路径验证；不外推断点恢复、真实第二跳或Linux／Windows原生验证，详[恢复与精确证据](../../../项目相关报告/实现进度/20261116-剩余能力闭环/服务端恢复.md)。

普通个人端侧迁移使用独立 `sync_migration*.go/message_migration.go/migration_identity.go/migration_interest.go/migration_invalidation.go`：同scope/user/personal的源目标原device/store配对、服务端核原历史及原件、AES-GCM包与独立HTTPS重新认证取钥、目标整批validate和安装报告，源保留、无新消息／无Outbox／无资格继承。独立owner失效队列接源撤交付、媒体清理与恢复退休；迁移13＋注销／改密／媒体关联共26方法同轮全过，原parent冻结退派生转发包另1方法通过。poison仍503保原责任，但公平批次已证明后续合法holder可清理。字段见[普通迁移合同](./接口契约/普通迁移/README.md)，精确输入／已验／候验分列[普通迁移报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/普通端侧迁移.md)，不借后端测试证明五端本地耐久或全I完成。
