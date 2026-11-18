# JobsIM 基础 HTTP 实现契约

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

当前代码切片的接口，全部使用 `/api`。单一现行需求仍是产品目标；本文只记录已实现或本轮正在实现的接口，不能据此宣称全量验收。ID、序号、游标均为字符串；时间为UTC RFC3339。

## 一、公共对象

`User={id,username,displayName,avatarURL?,avatarThumbnailURL?,profileVersion?,state?}`；会话内本人对象还含deviceId／restoreEpoch。头像URL须当前鉴权下载；没有头像不生成公开对象地址。群成员额外含role、membershipGeneration及roleGeneration。

`Conversation={id,kind,title,ownerId,memberIds:[],updatedAt,cursor,state,readOnly,membershipGeneration,roleGeneration,sendAdmission}`；kind为direct/group/self。membershipGeneration是本人当前成员资格的十进制字符串；roleGeneration是本人当前角色授权代际的十进制字符串，所有kind均返回。群主转让只推进原／新owner的roleGeneration，membershipGeneration和joinedAt不变；同owner转让及改名不推进。sendAdmission是本人当前发信准入的不透明快照（当前实现为47字符`sa1.`标识串），没有当前发信资格时为空。不得解析其中私人策略或将其他成员的快照借用给本人。当前目录cursor保留`0`，权威消息水位从history.cursor获取。owner_unavailable群可读既有获权历史，不能恢复普通群治理写。

`Message={id,conversationId,senderId,clientMsgId,type,text,replyToId?,attachmentId?,createdAt,updatedAt,version,contentVersion,sequence,creationEpoch,creationSequence,deleted,contentUnavailable?,reactions:[],pinned,favorite,mentionedUserIds?,mentionAll?,mentionedMe,forwarded?,forwardUnavailable?,forwardAdmission?,attachmentURL?}`。回复保持原消息ID；撤回保留占位。`sequence`为消息事件序号；编辑／撤回／反应／置顶／收藏的展示版本与事件同事务推进，增量返回当前权威消息状态并按ID去重，cursor只按返回水位推进。恢复缺少较新编辑正文时，独立content floor拒绝旧文，返回contentUnavailable=true且正文空，客户端显示内容不可用占位。

错误为`{error:{code,message},requestId}`，HTTP 400/401/403/404/409/410/413/429/503区分参数、身份、权限、缺失、冲突、大小、限速和依赖失败。不存在或未授权的对象不泄漏正文。

## 二、身份与会话

- `GET /api/bootstrap` → `{deploymentId,name,features,protocolVersion,restoreEpoch}`；restoreEpoch为十进制字符串。
- bootstrap和capabilities的只读`features.send_admission=true`表示服务端执行严格原准入快照校验，管理员不能关闭。旧服务没有该标志时，客户端停止新发送及旧缺快照队列的POST对账，避免旧服务首次创建旧意图。
- `POST /api/auth/register|login`，JSON `{username,password,requestId,deviceName,otp?}` → 下述AuthResult。管理员登录必须当前TOTP。客户端持稳定requestId；相同ID异参409。注册用户名为3～32 ASCII字母数字下划线，小写认证键；密码NFC后15～128码点、最多512 UTF8字节，Argon2id。
- 原生和CLI使用`Authorization: Bearer <token>`。Web发送`X-JobsIM-Client:web`并使用`credentials:include`；返回HttpOnly Cookie，JSON不含bearer。
- 未登录register/login/recover不先取CSRF；Web要求同源Origin、application/json及X-JobsIM-Client:web，不开放跨源CORS。当前未实现匿名前置挑战，不把它写成已交付。
- 登录SetCookie后 `GET /api/auth/csrf` → `{csrfToken}`；Web所有写请求发送`X-CSRF-Token`，并检查同源Origin；跨源不自动开放。之后`POST /api/auth/confirm {}`带CSRF，确认前Cookie不能访问聊天。
- `POST /api/auth/logout`；`GET /api/me`返回flat User；`GET /api/auth/context`返回`{user,deviceId,restoreEpoch,familyId,refreshGeneration,accessExpiresAt,idleExpiresAt,absoluteExpiresAt,sourceContextId}`供客户端恢复真实命名空间，不返回token／refreshToken；`GET /api/auth/sessions`；`DELETE /api/auth/sessions/{deviceId}`。登录响应user也包含restoreEpoch；客户端Outbox冻结原epoch，不因重连或刷新重写。
- `PATCH /api/me` `{displayName}` → User。昵称由directory域权威保存，用户搜索／联系人／会话成员通过受鉴权域RPC读取当前昵称。
- `POST /api/auth/recovery-codes` `{password,requestId}` → `{codes,batchId}`，只当次生成返回原码；重复查询只返脱敏状态。`POST /api/auth/recover` `{username,code,password,requestId}` → `{status}`，单次消费、改密和撤旧会话；恢复不自动登录。

### 2.1、<span id="session-refresh-implementation">访问／刷新家族与未知结果</span>

只读内建`features.auth_refresh=true`标识本协议。AuthResult为`{token?,refreshToken?,user,deviceId,restoreEpoch,familyId,refreshGeneration,accessExpiresAt,idleExpiresAt,absoluteExpiresAt,sourceContextId}`；原生token就是访问凭据，默认15分钟，refresh闲置30天、首次认证绝对90天，期限为UTC RFC3339字符串，generation为精确十进制字符串。旋转保持session／family／device，闲置与access均不超过原绝对截止。新认证另建家族；普通刷新不改变账号securityVersion或已发转发引用的账号资格。当前只实现personal来源，服务端冻结其source_generation，未接入II宿主来源／grant。

- 原生／CLI `POST /api/auth/refresh` `{refreshToken,requestId,familyId,refreshGeneration}`，不依赖旧AT有效；成功返回完整新AuthResult。RT随机摘要只用于定位；事务固定先锁user再锁session，原历史标used、新代active、当前摘要／代际／期限同事务推进，每session唯一active。旧access在旋转后也不再是当前摘要，客户端只能在原账户／family的同一协调操作内切换凭据。
- 每个原refresh只消费一次，**没有重交付窗口**。已消费旧摘要再现401 `REFRESH_REUSE_DETECTED`，原子撤其family及当前AT；其他独立family不误注销。未知token401 `REFRESH_INVALID`；闲置／绝对到期401 `REFRESH_EXPIRED`；撤权、安全／恢复／来源代际失效401 `REFRESH_REAUTH_REQUIRED`。只有业务401 `ACCESS_EXPIRED`可以触发一次协调刷新；其他401不盲目续登。503／传输失败／无法解析／新凭据对安全存储失败保留unknown，禁止自动重用旧RT。
- 刷新前持久非秘密family／原操作unknown标记，成功新AT／RT对按平台安全库原子保存后再发布。请求和401重试绑定调用捕获的账号／family／实例代次；换账号后不能借新账号的凭据重试旧业务。原恢复epoch与原Outbox body保持不变。RT、密码、OTP不入普通消息DB／日志／URL；旧摘要保留映射，当前没有自动删除这些安全责任记录的GC。

`POST /api/auth/reauthenticate` `{username,password,otp?,previousFamilyId?,previousRequestId?,previousRequestIds?,requestId,deviceName}`是新的本人验证，至少携原family或前序请求。先验当前密码及管理员MFA，再在同user锁下核同实例／personal owner：known previousFamily仅撤本人的该家族；previousRequest定位原login／registration／reauthentication记录直接绑定的session／family，不能凭ID冒认或取得凭据。其他owner 403 `PREVIOUS_FAMILY_INVALID`／`PREVIOUS_AUTHENTICATION_INVALID`；映射不能证明503 `PREVIOUS_AUTHENTICATION_UNRESOLVED`。原请求映射、旧摘要及退休fence不随300秒密文清理而删除。

多层未知由只读`features.auth_retirement_chain=true`标识完整退休闭包实现。新显式恢复意图的`previousRequestIds`为旧→新、最多64项的完整非秘密前序链，键长8～128字节，不重复、不包含本次requestId；scalar previousRequestId可重复列表末项，未在列表中则追加末尾。例R0已成但响应丢、R1关联R0却从未到达，R2必须冻结`[R0,R1]`，原子退休未建R1并撤R0既成family。已退休边R0→R1要求闭包同时覆盖后方R1，否则409 `PREVIOUS_AUTHENTICATION_SUPERSEDED`；保留原边，不改写独立日志事实。只撤闭包内同owner原family及显式previousFamily，合法独立设备不受影响。旧auth_refresh但没有chain标志的服务不能承担多层闭包；不得给已发body补列表，须保留原意图沿原ID／原body继续，或等受控恢复。

初始认证自身仍有独立300秒AEAD交付，摘要包含规范化password／OTP、Web/native用途和原previous字段；同ID必须原body，不换OTP后冒充重交付。超过窗口401 `DELIVERY_EXPIRED`，旋转／撤权使旧交付失效；初始密文到期、Web确认或刷新后清空。若秘密不再在内存、需要新有效OTP或原初始结果未知，则用户重新输入证明，以新关联reauth意图携完整未知前序链，先受控撤旧再发新认证，不无关联换ID建会话。本人证明有效且闭包完整时无需先等待DELIVERY_EXPIRED；客户端也可采用只在服务器明确该错误后才新建后继的保守流程。普通400／401不能证明同requestId从未在先前未知尝试中建family，非秘密未知责任不能据此自动丢弃。

previousRequest尚未创建时，已验证的**现有账号**也可先持久退休自己的旧意图再建关联后继。原认证新建前同时核业务表与独立journal退休事实，迟到未建旧请求401 `AUTHENTICATION_RETIRED`，不能首建孤立family；旧前序已由另一后继取代409 `PREVIOUS_AUTHENTICATION_SUPERSEDED`，沿客户端保存的后继意图续，不分叉。退休最小user／requestKey／replacementKey记独立日志并恢复重放，无密码／token；日志intent已成而业务事务回滚也拒旧。原register完全未达且账号不存在时，不能自称密码reauth造号，仍须原register ID重试；本地经过300秒不能证明服务器已建账号。

旧schema只有7天单token的session迁移时明确撤销并清旧交付，不续长、不中途升级成RT，须重新认证。账号恢复／安全撤权拒全部原账号凭据；logout／选定设备／旧RT重用只撤相应逻辑设备家族。identity后台保留到期／撤销清理责任及独立日志，普通family关闭不撤销已经发送的独立转发引用。实现借鉴[刷新旋转与重用检测原则](https://www.rfc-editor.org/rfc/rfc9700.html#section-4.14.2)，本项目是本地账号协议，不据此宣称完整OAuth服务或全部DBC-05已验收。

### 2.2、<span id="bff-refresh-implementation">Web BFF 与多标签身份围栏</span>

浏览器只有`__Host-jobsim_session`不透明`Secure; HttpOnly; SameSite=Lax; Path=/`、无Domain Cookie。它不是上游AT／RT；identity的auth.bff_sessions密封保存上游凭据对，并绑定family／generation／安全／恢复／来源及原截止。Web AuthResult剔除token／refreshToken，JS／LocalStorage／IndexedDB不持上游凭据。首次登录仍awaiting浏览器确认：GET `/api/auth/csrf`，POST `/api/auth/confirm {}`带CSRF＋精确Origin；仅初始CSRF／confirm允许握手缺family，logout不享这个例外。

激活后的所有Web业务GET／写、context／CSRF／媒体／头像／下载都带调用捕获的`X-JobsIM-Auth-Family:<familyId>`，后端对当前Cookie再验。跨标签Cookie换成Bob而旧页仍是Alice时，缺失／不符返回409 `AUTH_CONTEXT_CHANGED`，不清Cookie、不撤新Bob家族，不用Bob响应写Alice缓存或以Bob重试Alice请求。资源须经带此header的鉴权fetch，再显示受控Blob，不把凭据塞URL或开放资源例外。该header不是认证材料，Cookie／CSRF／Origin仍必需。仅首次没有本地已激活／unknown上下文时，可GET context或refresh-context带`X-JobsIM-Context-Learn:1`学习；旧上下文错误后不能自动去掉family再学习新Cookie。

- `GET /api/auth/refresh-context` 发送`X-JobsIM-Client:web`及原family header，AT过期但BFF仍有效时返回非秘密`{familyId,refreshGeneration,accessExpiresAt,idleExpiresAt,absoluteExpiresAt,sourceContextId,restoreEpoch,csrfToken}`。它不清unknown、不返回User或上游凭据；旧页不能因为看见新generation就恢复。
- Web `POST /api/auth/refresh` 仅`{requestId,familyId,refreshGeneration}`＋Cookie／CSRF／精确Origin／原family header；传refreshToken反而400 `BROWSER_REFRESH_SECRET_REJECTED`。BFF所用原generation、RT消费、新密文同事务，旧标签命令不借最新密文再旋转。不同family先409 AUTH_CONTEXT_CHANGED，不误撤新登录；同family旧generation或重复refresh ID则409 REFRESH_REAUTH_REQUIRED，撤该family并清密文，要求重新验证。
- BFF重启只能读取原密封状态，不签新凭据或续绝对截止；未确认401 BROWSER_CONFIRMATION_REQUIRED，到期／撤权／恢复等401 REFRESH_REAUTH_REQUIRED。响应未知、JS侧持久意图或凭据处理失败时停止自动刷新／业务，经本人验证关联撤旧；普通refresh不能套初始300秒交付。多标签只协调一个刷新；其他页等待该原协调结果，不能用旧RT或过期generation再发。

### 2.3、<span id="own-password-change">本人主动改密</span>

只读内建`features.own_password_change=true`表示后端提供本协议，客户端接线与实际验证分别登记。首次`POST /api/auth/password-change`提交`{requestId,username,deviceId,restoreEpoch,currentPassword,newPassword,otp?}`，须当前已登录本人、原设备及当前恢复代际；Web仍需原family／CSRF／同源Origin。新密码沿用NFC后15～128码点／最多512字节规则，拒绝复用当前密码；管理员另消费当前有效TOTP，不更换或清除MFA。

返回`{requestId,status,restoreEpoch,credentialGeneration,securityVersion,current,requiresNewLogin}`，代际均十进制字符串。`status=pending|changed|retired`；已完成历史结果的`current`只有在当前账号／安全代际／业务和独立日志epoch均核齐时为true，日志不可核或恢复门禁未开返回503。安全提交后必须重新登录，不发AT／RT或自动代登录。

`POST /api/auth/password-change-results`接受上述规范化原密码对、原nonce／用户名／设备／epoch，仅查原单：未知404 `PASSWORD_CHANGE_OPERATION_NOT_FOUND`，原证明不匹配403 `PASSWORD_CHANGE_REJECTED`；不首建、不重投、不恢复后台责任、不追加journal或改会话，仅更新每IP每分钟20次的限流计数。原操作`POST password-change`每IP20次／分钟，新建每本人5次／分钟；过限429。已成原单可在旧会话已撤销后凭原证明查询或显式同单重试，原OTP仅授权首次创建，不进入持久业务意图，查单不要求重放过期验证码。两条入口均保持JSON／同源Web标识要求，不以新登录的凭据续投旧意图；历史回放不清后来登录的Cookie。

identity先持久`prepared`原责任及绑定实例／本人／设备／epoch的AEAD密封替换Argon2id哈希，再写独立最小`auth.invalidate_user` intent、跨owner源排空，最后在本域事务中推进凭据／安全代际、撤销全部旧family、RT、BFF与初始凭据交付，停用旧恢复码批次及重置凭证；不解封或记录原密码。提交后清除替换密文，journal result／导出清理完成才标`completed`。后台按有界批次续原责任，不要求再持有密码；旧epoch／已被其他安全操作取代则退休原责任，不能安装旧哈希、解除封禁或降低安全floor。客户端只持久非秘密原nonce与身份上下文，密码对／OTP留内存，重启后由本人重新输入原证明；不得持久无钥密码摘要或把unknown当失败换ID。

源码与4组纯规则／真实PG专项的实际范围见[改密检查点](../../../../项目相关报告/测试/后端/20261116-本人主动改密/README.md)。各端产品入口、真实HTTPS、独立双集群恢复另验，不能借当前后端切片称全部账号功能完成。

普通认证及恢复路径还要求`deletion_state='none'`；requested／processing／completed都不能新发登录、旋转RT、解析BFF、恢复密码或授正式导出principal。已准备未提交的主动改密在注销状态出现后退休，已完成原单仍可核历史但current=false。原[门禁增量](../../../../项目相关报告/测试/后端/20261116-注销普通准入门禁/README.md)仅证明普通准入；后续完整注销协议见[本人注销与受限数据权利](#own-account-deletion)，实际端侧和执行证据单独登记。

### 2.4、<span id="own-account-deletion">本人注销与受限数据权利</span>

只读内建 `features.own_account_deletion=true` 表示实现本协议，不能代替五端接线／真实设备验收。全部公开入口归 Identity，Web JSON、原family／CSRF／同源 Origin 要求沿现有认证约定。原申请／结果／受限证明／查询／取消各自每IP每分钟15次限流；管理员审计证明另需当前管理员会话及MFA。

- 首次 `POST /api/auth/deletion-requests` 体为 `{requestId,username,deviceId,restoreEpoch,password,otp?}`：原nonce为8～128字符，当前本人AT／原设备／epoch＋当前可信密码；管理员需TOTP。密码NFC规范化，只在内存验证，持久原单仅存有钥摘要，OTP不进入业务摘要。先持久原责任和普通访问围栏，Governance保存数据库绝对冷静截止，独立journal请求intent保护后跨owner撤导出／转发，Identity本域标requested、递增security_version、撤全部family／RT／BFF／短期交付。返回已确认请求需要重新登录时，注销状态本身继续拒绝普通登录。
- `POST /api/auth/deletion-results` 接受同一原体，可省OTP；匿名只查原申请，不创建、不续跑、不改变会话。未知404 `ACCOUNT_DELETION_NOT_FOUND`，原证明不匹配403 `DATA_RIGHTS_REJECTED`。原创建提交未知时保留同nonce／body责任，只有用户明确重试才再POST原申请；新家族AT不能借给原单。已存在原单允许匿名同体续跑，历史回复不清后来登录Cookie。
- 公共结果为 `{deletionId,requestId,status,requestVersion,restoreEpoch,cancelUntil,cancelRequestId,requiresNewLogin:true}`；版本与epoch均十进制字符串。状态为 `pending/requested/cancel_pending/processing/completed/cancelled/retired`；原责任尚未获冷静截止或旧未保护代际退休时cancelUntil为null，否则为原ISO绝对时刻。默认冷静期7天，`--account-deletion-cooldown-days`／`JOBS_IM_ACCOUNT_DELETION_COOLDOWN_DAYS`允许0～30天，0表示立即取得清理资格，不表示先跳过撤权或journal。
- 无普通会话时，`POST /api/auth/data-rights/proofs` 体 `{username,deletionId,requestVersion,purpose,password?,recoveryCode?,otp?}`，purpose严格为`query`或`cancel`，密码／恢复码二选一。密码须当前正常可信凭据，管理员密码证明另需MFA；恢复码只消费当前有效批次，消费写独立journal避免旧备份复用。响应 `{proof,deletionId,requestVersion,purpose,expiresAt,restoreEpoch}`，5分钟不透明proof只在内存，绑定实例／本人／原申请／版本／用途／当前安全与恢复代际。proof不能用于聊天、refresh、解封或恢复墓碑。
- `POST /api/auth/data-rights/query {proof}` 仅接受query用途；`POST /api/auth/data-rights/cancel {proof,requestId}`仅接受cancel用途，requestId是持久非秘密原取消nonce。Governance同一行锁／row_version仲裁：仅requested且DBnow<cancelUntil→cancel_pending；worker仅截止后requested→processing。任一方获资格，另一方不能跨越；processing/completed不可取消。取消未知保持cancel_pending，沿原nonce查原申请结果里的cancelRequestId，不换操作假报成功。旧proof仅可重查同一个已经成立的取消，不能借安全版本变化取消新责任。
- Identity只在原deletion_request_id且仍requested时清该注销门禁，递增security_version；保留banned／tombstoned、recovery_state及其他撤权／期限。旧session始终无效；journal result、跨owner排空和确认完成后Governance才显示cancelled。取消后正常使用须重新登录并满足其余权限。
- 受控管理员核验入口 `POST /api/admin/users/{id}/data-rights-proofs {requestId,deletionId,requestVersion,purpose,reason,otp}` 要求当前管理员、有效MFA与10～512字节审计理由，持久操作者／目标／原申请／用途／理由；它只发同一受限proof，不签普通会话、解封或重设密码。注销completed且原密码／恢复凭据已清除后，独立正常管理员仍可按原申请和当前精确版本审计签发5分钟query证明，过期须重新MFA和独立审计；completed签cancel返回409 `DELETION_CANCEL_CLOSED`。被注销本人旧密码／恢复码不能据此重新取得证明。
- processing后各owner只凭Governance当前原版本与已到数据库截止执行：message复用持久global擦除队列、源／转发／草稿／导出排空与删除引用；media按精确owner及原UUID对象删除附件、未提交上传和头像字节并fsync；directory清资料与私人偏好、移除联系人；conversation退出成员，失效群主转owner_unavailable只读而不阻塞本人注销；sync清本人快照／流／本地库注册／view／read／草稿；governance仅保最小审计事实。全部owner确认后再清当前密码／MFA秘密和短期交付，保用户名占用键及最小安全记录。
- 请求、取消结果和owner擦除均有typed最小journal事实；取消intent不能降闸，completed不能恢复。Identity／Governance后台按持久last_attempt_at公平选择有界续跑批次，永久失败的早期责任保留原deadline／状态并让出后续批次，不阻塞其它申请。隔离恢复先重放门禁／内容floor，再删除已保护owner的精确恢复媒体文件、退休旧群治理grant，最后才开放新epoch。客户端仍须分别执行本地数据与global回归，服务端完成不代表已擦除关机设备或历史离线备份。

源码为 `account_deletion*.go`，外置真实PG用例为 `测试/后端/接口/account_deletion_pg_test.go`；最终10项真实隔离PG方法全部通过，含33消息／33头像分页、恢复字节重清及未知责任续跑；RPC为进程内生产handler，journal在隔离测试PG独立schema，不称真实HTTPS或独立双集群恢复。证据见[注销后端报告](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/注销后端.md)。

2026-11-17追加[公平续跑与完成回执回归](../../../../项目相关报告/测试/后端/20261117-注销续跑公平性/README.md)：新增3反例修前全部失败，冻结修后输入19项注销相关方法全部通过／无跳过，包含准入、原PG用例、末页拒读及新增反例；不与前序轮次累计。独立新PG已停止，未连接用户实例；角色间RPC仍是进程内生产handler。

## 三、会话与消息

- `GET /api/conversations` → `{items:[Conversation]}`。
- `GET /api/conversations/{id}` → Conversation；本人当前有效成员读取，支持定位列表前100条以外的合法来源，不凭旧搜索结果恢复资格。
- `POST /api/conversations` `{kind,title,memberIds,requestId?}` → Conversation。
- `GET /api/conversations/{id}/members` → `{items:[User+{role,membershipGeneration,roleGeneration}]}`；墓碑成员仅返回稳定身份占位／state，不能回填正常展示资料。
- `POST /api/conversations/{id}/members` `{userId}`；`DELETE .../members/{userId}?membershipGeneration=<原代际>`；`PATCH /api/conversations/{id}`只接`{title}`，旧`ownerId`转让入口400拒绝。退群／踢出必须保留原资格代际，缺值400、跨重入代际409。转让、角色、禁言、策略、解散及独立恢复使用[群治理原单协议](./群治理/README.md)，不借后来的owner资格补旧操作。群主须转让后退出；owner_unavailable不会因账号恢复自动解除，恢复需独立operator grant、当前管理员及绑定原操作的step-up。
- `GET /api/conversations/{id}/messages?after=<cursor>&limit=100` → `{items:[Message],cursor}`。history.cursor的精确格式为`epoch:sequence`，两个分量均为十进制字符串；仅首次从头请求可传`0`，不得把整个游标解析成UInt64或只持久sequence。旧epoch返回HTTP409、`error.code=RESTORE_EPOCH_CHANGED`，客户端冻结旧Outbox并重新核对身份与服务端历史。
- `POST /api/conversations/{id}/messages` 普通分支传`{clientMsgId,type,text,restoreEpoch,sendAdmission,replyToId?,attachmentId?,mentionedUserIds?,mentionAll?}`，转发分支见下文，均返回Message。客户端在新意图入本地事务时冻结原sendAdmission／本人membershipGeneration，发送、重启及未知结果核对均保持原ID／原载荷，不能给旧队列补新快照。服务端先验当前读取ACL，再沿原sender＋clientMsgId核对完整原摘要；已成功事实仍受当前读取ACL，群重入可能不能读原成员期正文。同ID变更快照也属于`IDEMPOTENCY_CONFLICT`。未建原ID缺快照或快照失效返回HTTP409 `SEND_ADMISSION_CHANGED`，该意图永久冻结，重新获权不自动重发。旧无字段的既成消息保持原摘要兼容，缺字段未建消息严格拒绝。不存在的旧restoreEpoch仍返回RESTORE_EPOCH_CHANGED，不可换ID自动重发。

- `POST /api/conversations/{id}/message-results` 接收发送时完整原JSON，返回已提交的当前Message；沿当前readACL与本人sender／原clientMsgId／原conversationId／完整摘要核对。未知ID为404 `MESSAGE_RESULT_NOT_FOUND`，异参409 `IDEMPOTENCY_CONFLICT`，此接口绝不创建消息。冻结／主动取消／撤权前后待核对队列使用这个只读路由；不能把创建型POST messages当成通用查询，因为本地先冻结但撤权尚未到服务端时，原准入仍可能有效。旧服务404也不能降级到创建型路由。只读核对无需把缺失的旧sendAdmission补成新值。

当前围栏限定同一主机、同一个规范化本地StateDir内的独立角色进程：发信持操作系统共享锁直到真实PG提交完成；删友／拉黑／接受／入退群及群只读状态变更持排他锁。目录发信授权代际独立于私密入站申请gate，撤权和新接受推进，unblock／reopen本身不授予发信。消息提交前fsync原事务标记；未知、取消或崩溃遗留先核PG原事务状态和原消息摘要，未证实则503 `ADMISSION_RECOVERY_REQUIRED`，不按超时清理。角色启动验证回环地址及OS锁能力，关键内部RPC校验规范化StateDir证明，错目录503拒绝；不把此实现当成跨主机分布锁或吞吐达标。
- `PATCH /api/messages/{id}` `{text,expectedVersion,requestId?}` → Message。
- `GET /api/messages/{id}` → Message；重新核当前源读取ACL，撤回返回既有删除占位；群退出／重入的旧成员期历史按既有规则拒绝。搜索／收藏定位前重新读会话与原消息，不直接回显旧列表快照。
- `DELETE /api/messages/{id}` → Message；仅作者或有权群治理者，撤回留占位。
- `POST /api/messages/{id}/reactions` `{emoji}`；`DELETE .../reactions/{emoji}`。
- `PUT|DELETE /api/messages/{id}/pin`；`PUT|DELETE /api/messages/{id}/favorite`。
- `GET /api/search?q=...&conversationId?&cursor?&limit?` → `{items:[Message],cursor,hasMore}`，q为1～256 UTF8字节，非空；`GET /api/favorites?conversationId?&cursor?&limit?`使用相同分页对象，无需搜索词，只读本人收藏的当前源引用。源撤回、失权或旧群成员期不能从收藏找回正文，不复制正文作为独立收藏快照。
- 两种列表均默认limit=50、范围1～100，初次cursor省略或`0`，后续原样回传不透明签名游标；按源createdAt／ID降序，cursor绑定本人、恢复代际、列表类型、q及conversationId，跨账户／条件／坏签名400 INVALID_CURSOR，旧恢复代际409 RESTORE_EPOCH_CHANGED。先从conversation owner获取本人完整有效读取范围和各群加入下界，再在message owner按该范围过滤后LIMIT，不用公共会话列表前100条或全库前100命中充当授权范围；每条返回前仍重验当前源ACL。并发编辑／撤权导致不再匹配或失权时继续扫描补足页面，接口不是固定时间快照，新增内容从首屏刷新可见。hasMore=false表示本次当前范围已扫描完，不承诺跨请求权限或数据不变。

`features.role_admission=true`为只读内建角色围栏标志，表示sendAdmission同时绑定本人roleGeneration；角色名称不是代际。owner→member→owner不能恢复旧token，普通未建旧意图也保守返回SEND_ADMISSION_CHANGED；当前readACL下已提交原ID不受新写资格检查阻断。旧send_admission-only服务尚无该保护，客户端@all新发送与原未成重放须明确检查role_admission；普通消息／单独成员提及仍按既有send_admission门禁。只读message-results与公告原operation查询不要求角色写能力，也绝不降级为创建型发送。

### 普通引用式转发

`features.forwarding=true`是只读内建实现标志，旧`text_forward`保持false。支持普通text／image／audio／video／file及最多8层转发，新增写仍复查messages／media父能力和目标发信资格。旧服务缺forwarding时不提交这个新分支。消息读取给当前读者可用的`forwardAdmission`，为不透明`fa1.`证明，绑定本人、源消息／contentVersion、源会话原成员代际、本人securityVersion及restoreEpoch；不含正文，也不授予目标收件人源会话权限。

转发新意图先持久原JSON：`{clientMsgId,type:'forward',forwardOfId,forwardSourceVersion,forwardAdmission,restoreEpoch,sendAdmission}`。forwardSourceVersion取原选择消息的contentVersion；禁止夹带text／attachmentId／replyToId／mentionedUserIds／mentionAll复制正文或提醒资格。不得为旧意图补最新证明／版本／目标token。未知先用message-results查原完整body；404表示没有已提交事实，只有用户明确重试原发送才可沿相同ID和原body重投创建型POST，仍重新验证原证明。冻结、取消或已明确资格失败的队列仅只读核对，不自动重发。新分支字段以omitempty追加，普通旧发送摘要兼容。

服务端message行仅存type=forward、空正文／空attachmentId；独立forward_refs保存原source ID／contentVersion、转发者原成员代际／securityVersion／epoch及链深度／状态。每次读取先验目标当前readACL，再逐层重验原转发者及来源当前资格；目标收件人不必加入源群。有效响应`forwarded=true`，type为最终text／image／audio／video／file，正文仅当前投影；媒体原attachmentId不下放目标。任何层失效时`forwardUnavailable=true`且text／attachmentId／attachmentURL／forwardAdmission均空。源编辑／撤回、原转发者退出源成员期或账号安全撤权会永久失效受影响引用，重入／重新获权／旧备份恢复不会使旧引用复活；普通logout或单设备撤销本身不撤销已发引用。

`forwardUnavailable`只表示**本次当前投影不可用**，可由临时依赖未知产生，不能作为端侧永久锁存标志。精确GET、history、已成发送及message-results可200返回空占位且不推进内容版本；依赖恢复后精确GET可在同一version恢复投影。客户端应清正文／附件／派生缓存，按单调version及本地请求次序／CAS接收结果，不能让晚到历史或发送响应覆盖较新占位，也不能凭同version普通历史解除本地未知；只在当前精确GET权威重投影后恢复。永久失效由服务端持久ref状态及内容版本／事件保证，不能把新发送的403／409资格拒绝与已成引用的投影boolean混同。搜索／收藏有分页要求，遇到未能证明的转发资格整页503；已确定失效引用则继续扫描，不让旧正文命中或挤掉合法结果。

- `GET /api/messages/{id}/attachment`（含HEAD／Range）只接受目标消息当前鉴权及完整有效链。经受鉴权内部media协议核原sourceMessage／chat_attachment purpose／storage ID后代理原字节，不复制对象、不暴露原对象授权。返回Content-Type、文件名、长度／Range、ETag及`X-Content-SHA256`／`X-Content-Size`；依赖未知503、确定不可用404 `FORWARD_ATTACHMENT_UNAVAILABLE`，不得回退旧本地文件。
- 包含Message的公开JSON和转发媒体流，由gateway从资格判断一直持同机共享围栏到响应写完／客户端取消；内容改写持排他围栏。外来内部证明header一律清除，只有gateway在实际持锁后签发method／path／模式绑定证明，内部角色复核以免重复嵌套持锁。内部media读取也核相同规范化StateDir证明。慢客户端可阻塞本机内容撤权，取消后排空；这不是端侧耐久ACK或跨主机分布式锁。
- 清理先提交owner pending，持久登记直系及后代invalidating任务，再排空源及派生holder，分批推进unavailable／version／contentVersion／事件。超过单次100条或排空未完503 `FORWARD_CLEANUP_PENDING`，原源动作不能假报完成；message后台恢复未决任务和owner pending，操作者沿原源操作续跑。当前公开edit／recall／退出源群／账号安全路径已接；独立可信revoke-source钩子不等于新增公开云历史清空功能。

新意图错误：400 `INVALID_FORWARD`／`FORWARD_PROTOCOL_REQUIRED`／`FORWARD_DEPTH_LIMIT`；409 `FORWARD_ADMISSION_CHANGED`／`FORWARD_SOURCE_CHANGED`／`FORWARD_SOURCE_UNAVAILABLE`；源成员期明确403 `NOT_A_MEMBER`／`HISTORY_NOT_ALLOWED`；临时不可证明503 `FORWARD_QUALIFICATION_UNAVAILABLE`。明确原证明／资格失败保留原body并冻结，503保留未知，不换ID或证明。真实范围见[普通转发测试](../../../../测试/后端/接口/forward_refs.py/README.md)。

### 结构化提及与群公告

bootstrap／capabilities的`features.mentions=true`及`features.announcements=true`为只读内建实现标志，不能经管理员切换。新结构化提及仍检查groups及消息发送的messages父能力，公告新发布检查groups／messages；既有获权读取、原结果核对和清除引用不因关闭新能力而旁路身份／群权限，也不阻止必要清理。

`mentionedUserIds`是选定的当前群成员ID列表，不能重复、不能指向非成员；不能与`mentionAll=true`同时提交。普通文本中的`@昵称`没有提醒资格。两字段以omitempty加入原发送摘要，省略时保持旧摘要兼容；修改提及也属于同ID异参。服务端在发送事务中保存每个目标当时的membershipGeneration，@all只允许当前active群owner，不能由客户端伪造成员列表。`Message.mentionedMe`逐次按当前收件人／当前成员代际／双向blocks投影；发送者不提醒自己，撤回或contentUnavailable不返回有效提及。群重入不能继承离开前资格；共同群正文仍按已有ACL可读，拉黑仅抑制针对性提醒。编辑保留原消息身份与原提及资格，不新增mention记录；前台／本地通知按原messageId去重，不能把新version当作新提及提醒。

频控是当前工程默认：显式提及最多20人，每发送者／群60秒最多20条带显式提及消息；@all独立60秒最多2条。原ID已提交重试不重复计数，失败事务不消费次数。部署环境可分别设置`JOBS_IM_MENTION_MAX_TARGETS`（1～200）、`JOBS_IM_MENTION_WINDOW_SECONDS`（1～86400）、`JOBS_IM_MENTION_MEMBER_LIMIT`及`JOBS_IM_MENTION_ALL_LIMIT`（各1～10000），各角色保持相同实例配置；非法值拒绝提及处理503，不静默忽略。不把默认值写成用户自行选定的群策略；当前未提供群级策略管理UI。

- `GET /api/conversations/{id}/announcement` → `{conversationId,version,state,messageId?,sourceVersion?,expiresAt?,updatedAt?,canManage,message?}`，初始version=`"0"`、state=empty；state取empty／active／unavailable／expired。只有active包含经当前资格与精确来源版本重验的Message；其他状态没有正文或附件对象。owner_unavailable仍按已有获权范围读，但canManage=false。
- `PUT /api/conversations/{id}/announcement` `{messageId,sourceVersion,expiresAt?,expectedVersion,membershipGeneration,requestId,roleGeneration}`；`DELETE`同路径只传`{expectedVersion,membershipGeneration,requestId,roleGeneration}`。只允许当前active群owner；新操作核原本人成员／角色代际与公告CAS版本，旧代际409 `MEMBERSHIP_GENERATION_CHANGED`、旧公告409 `ANNOUNCEMENT_VERSION_CONFLICT`，只读群409 `GROUP_OWNER_UNAVAILABLE`。新未建请求缺失／过期roleGeneration均409 `QUALIFICATION_CHANGED`。roleGeneration以omitempty追加到原摘要，已完成旧无字段请求先按当前readACL与完整原摘要返回事实；不能给旧pending补最新值。新公告写及原未成重放还须核role_admission，只有原操作查询不要求该写flag。expiresAt为未来RFC3339时间，省略表示不设到期。
- 写响应及`GET /api/conversations/{id}/announcement/operations/{requestId}`均为`{operationId,status:'applied',result,current}`。result是原提交公告元数据，不存来源正文；其state／canManage为提交时事实，不能当作最新可见性或治理资格。丢响应沿原requestId及原完整payload查询／重试，不改expectedVersion／membershipGeneration后冒充同一操作；原ID异参409。后续公告更新后，原事实保留但current=false，不回滚后来引用。完成后重新GET当前公告；原操作查询仍要求当前群读取ACL。

公告只保存本群源messageId／sourceVersion／到期引用，不复制聊天正文。每次GET校验当前群加入下界、源当前精确contentVersion、未撤回、正文可用及来源未冻结；源变化只回占位，需群主新操作明确刷新引用。公告sourceVersion绑定Message.contentVersion（十进制字符串，旧行初始1），仅正文编辑／撤回／正文不可用等内容变化递增；反应／置顶／个人收藏只推进Message.version，不使公告失效。客户端兼容旧服务取source.contentVersion存在时优先使用，旧版没有该字段时才用source.version；新服务只校验contentVersion。公告父群行锁内的源RPC不回调持锁群；发布／清除、成员／关系变更及消息内容写由同主机OS围栏串行，公告读取持共享围栏，不宣称跨主机原子或完整strong-profile公告灾备已验收。

## 四、联系人、媒体与运维

- `GET /api/users?q=` → `{items:[User]}`；`GET /api/contacts` → `{items:[User],requests:[{id,requesterId,recipientId,state,requester?:User}]}`。申请人资料从identity／directory当前权威资料批量聚合；仅当前有效入站申请、当前好友或本人可返回，blocks／墓碑／失效资格不回填正常旧资料。requester缺失时显示不可用占位，不能保留上一响应里的旧资料；头像仍遵守既有好友／共同有效会话可见权限，不因陌生申请自动放宽。收件箱只把pending且recipientId为当前本人的项作为可接受申请。
- `GET /api/contacts/{peerId}/relationship` → `{peerId,blockedByMe,friend,inboundGate:'open'|'denied',generation,inboundReopened}`。仅查询已鉴权用户自己对peer的持久关系／入站策略，不返回对方的blocks或私人门禁，不含资料、头像地址、正文或MFA材料，也不扩大现有读取／通信权限。generation为十进制字符串；`'0'`表示尚无本人持久gate行、入站采用默认open，GET不创建行。inboundReopened仅当本人当前open代际确有对应的成功reopen操作才为true，不把默认open或旧操作成功当成当前已重开。合法但未知peer UUID返回同样的本人默认状态，不通过此接口探测账号存在性；缺身份401，无效／本人peer ID为400。现有写接口参数保持原样，写入仍各自重新鉴权。
- `POST /api/contacts/requests` `{userId,requestId?}` → `{id,state}`；`POST /api/contacts/requests/{id}/accept|reject`；`DELETE /api/contacts/{userId}`。
- `PUT|DELETE /api/blocks/{userId}`，双方私聊新发送阻断；解黑不重放旧申请。
- `POST /api/contacts/{peerId}/reopen` `{requestId}` → `{status:'opened',direction:'inbound',peerId,generation,requestId,current}`。这是当前收件人明确允许peer重新发入站申请，只改本人的gate；双方任一block仍存在则拒绝新操作。重复原ID只返原代际，current区分原成功与后来再次关门；旧pending终结、旧accepted／原申请ID不复活。收件人先reopen，发送人用新requestId申请，再由收件人新接受；不能拿发送人的账号去重开对方许可。
- <span id="stable-media-upload">稳定媒体上传</span>：`POST /api/media` 保留 multipart `file`，可选稳定原请求；`GET /api/media/capabilities` 返回 `stable-upload-v1` 能力与大小边界，`POST /api/media/upload-results` 只查原责任，404不建单、prepared不发布文件。同原身份／原metadata／真实bytes SHA的明确重试固定objectId，旧无nonce调用继续兼容但不具幂等保证；`GET /api/media/{id}`仍经当前权威授权，不公开对象目录。唯一完整DTO、错误与两端恢复语义见[稳定上传合同](../../../../测试/后端/接口/media_upload/README.md)，实际范围见[稳定上传验证](../../../../项目相关报告/测试/后端/20261117-稳定媒体上传/README.md)。
- `GET /api/capabilities` → `{features:{...},versions:{...}}`；管理员`PUT /api/capabilities/{name}` `{enabled,expectedVersion,requestId,stepUpId}`，仅开放已实现且可变的能力，不能开启after_receipt/E2EE等未交付项。
- `GET /api/health` → `{status,service}`；管理员`GET /api/admin/status`报告真实依赖、所选保护档与未验收项。备份为受控运维命令，不返回数据库连接串或秘密。

## 五、独立角色

单Go工程提供`--service=gateway|identity|directory|conversation|message|media|control|journal|governance|sync`独立启动；开发`--service=all`在同进程拉起独立HTTP角色、每角色数据库池和schema。内部端口从base起依次为identity至sync，共9个；journal使用另一个真实PG集群。角色通过受鉴权内部HTTP调用，不JOIN其他域私表、不共享事务。此开发切片尚未等于已验收微服务部署，跨机mTLS、最小权限、完整日志恢复、强保护等须后续真实验收。

`GET /api/me/export`仍仅导出本人账号／联系人／有效会话目录元数据，不含聊天正文、Cookie、token、密码哈希、恢复码或E2EE秘密；正式本人包另见[第十一节](#personal-data-export)。引用式转发与实验导出是独立能力，不能因转发可用便宣称链级导出holder已完成。

## 六、管理员强验证与凭据重置

首位管理员只通过`jobsimops --mode bootstrap-admin`在本人受控机器初始化，显式提供私密TOTP登记文件和密码环境变量。普通注册不占用admin/root等保留名，不交付共享默认密码，不把管理员重置等同于登录目标账号。

- `POST /api/auth/step-up` `{action,targetId,requestId,otp,payload}` → `{authorizationId,expiresAt}`。TOTP六位／30秒／±1窗口，每步只成功一次；新证明120秒。payload是将要提交的原业务JSON对象；服务端剔除requestId／stepUpId后规范化并计算带钥摘要，客户端不自制无钥密码摘要。绑定管理员／设备、动作、目标、原操作、版本、安全代际与restore epoch。
- action为`capability.update`、`account.state`、`account.reset_grant`、`report.resolve`。首次消费需在有效期内；已持久消费的同一原操作可在当前身份与安全代际仍有效时继续其prepared交接，不能换目标或载荷。已完成操作沿原requestId查询／重试原结果。
- `GET /api/admin/users?q=` → `{items:[AdminUser]}`。AdminUser含`id,username,displayName,state,version,securityVersion,admin,recoveryState,stateBeforeTombstone,banUntil,deletionState`。
- `PATCH /api/admin/users/{id}/state` `{state,reason,expectedVersion,requestId,stepUpId,permanent?,banUntil?}`；state为active/banned/tombstoned。封禁需明确永久或未来截止；当前解封仍须明确管理员操作，不把已记录期限冒充自动解封。恢复墓碑保留原封禁，旧会话始终无效。`GET /api/admin/operations/{requestId}`查询原结果。
- `POST /api/admin/users/{id}/reset-grants` `{reason,expectedVersion,requestId,stepUpId}` → `{id,userId,credential?,credentialAvailable,expiresAt,status,version,requestId}`。密码重置凭证为600秒、单次、绑定用户／用途／安全代际／恢复epoch，颁发即撤旧会话并拒绝旧密码登录。原凭证只首次返回；丢响应后查询不再回显，必须经新的强验证操作重发。
- `POST /api/auth/reset-password` `{username,grant,password,requestId}` → `{status}`，提交本人新密码、消费凭证并再次撤旧会话，不自动登录，不改变封禁，不重置MFA；不能拿账号恢复码或重置凭证互换用途。

## 七、静态头像与最小举报治理

- `POST /api/me/avatar`，multipart `file`＋`requestId`＋`expectedVersion=profileVersion`。当前仅标准静态JPEG/PNG、≤2MiB／16MP，拒动图和未实现解码格式。真实解码、中心裁剪、剥元数据，生成512及128 JPEG。响应`{user,operationId,status,current,result}`；status为applied/prepared/cancelled/superseded。
- `GET /api/me/avatar/operations/{requestId}`查询原完成事实；后续头像覆盖后仍保持原result且current=false。`GET /api/avatars/{id}`要求当前资料引用及本人／当前好友／共同有效会话可见资格，双向blocks优先拒绝。头像purpose与聊天附件独立，`/api/media/{头像id}`不能旁路；使用中头像不套聊天附件保留期。
- `POST /api/reports` `{targetType:'message'|'user'|'conversation',targetId,reason,requestId}` → `Report={id,reporterId,targetType,targetId,reason,state,version,createdAt,updatedAt}`。对消息／会话重验当前源访问，原ID异参409。
- `GET /api/reports`只列本人；`GET /api/admin/reports`列治理范围，两者响应`{items,includesChatBodies:false}`。管理员`PATCH /api/admin/reports/{id}` `{state:'reviewed'|'dismissed'|'action_required',reason,expectedVersion,requestId,stepUpId}`；`GET /api/admin/report-operations/{requestId}`查原结果。

这是目标＋理由的最小举报切片；完整证据选择／附件／处置进度／申诉、群头像和完整资料导出仍未交付，不能据此宣称完整I验收。

## 八、受控导出实验

只有显式启动`--experimental-exports`才开放本节实验路由与构建器；实验包范围为可获权普通聊天及真实附件，不能替代[正式本人数据导出](#personal-data-export)。`chat_export`现在声明正式入口，不开启或改写实验协议。实验关闭后保留受控清理组件继续排空此前责任。

`POST /api/me/exports {requestId}` → HTTP202 `{job,decryptionKey,keyReturned,format}`，原ID重试200但不回显key；`GET /api/me/exports/{id}`查状态；`POST .../cancel {requestId}`；`POST .../download-ticket {}` → `{downloadToken,expiresAt}`；`GET .../download`同时发送当前会话和`X-JobsIM-Download-Token`。独立源holder确认必须先于正文读取；编辑／撤回／退群／账号及设备撤权先冻结准入、撤任务并等旧worker/reader文件锁排空、清完整prefix后再完成源动作。短期派生制品不从备份恢复ready或旧key，恢复强制取消后重新授权新任务。实际公开源钩子、竞态和恢复仍在验证，不能将实验协议测试当作全部I或安全恢复门禁通过。

实验包只要包含forward就明确拒绝构建，任务failed且`failureCode='FORWARD_EXPORT_UNSUPPORTED'`，清理完整prefix；不能略过转发行后发布不完整包，也不能仅登记目标holder便复制链上源正文。manifest／register／read三个内部入口均拒这个分支。普通消息导出剔除临时forwardAdmission许可字段，不把源资格证明制成长期制品。链级正文holder尚未实现；正式包对forward仅提供明确占位。

## 九、<span id="authoritative-sync">权威快照与增量同步底座</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

这是有明确coverage的同步协议，不把会话history分页当全账户同步。`features.authoritative_sync`及`features.sync_jetstream`仅在部署配置自托管[**NATS JetStream**](https://docs.nats.io/nats-concepts/jetstream)后为true，不能由管理员关闭；未配置时公共sync接口503 `SYNC_NOT_CONFIGURED`。总线是I基础依赖，HTTP owner回放只作受鉴权补偿。该切片不代表端侧存储已合格，不发收讫ACK，`after_receipt`继续false。

schema1实际覆盖本人公开账号／设备元数据／本人profile、本人联系人与私人关系／申请、当前有效会话及成员ID、当前可读消息／互动／公告、消息附件引用及历史合法成员的最小global墓碑；该schema没有服务端已读、云草稿、服务端偏好及完整peer资料流，附件字节独立下载。候选schema2增量见[9.5本人状态](#sync-self-state)，不改变固定57／旧schema1覆盖；云草稿、完整账号注销、正式导出、普通迁移、qualified ACK等I功能仍须继续交付。`coverage.included/excluded`为机器可读范围，`classification='explicit_known_only'`、`absence='unknown'`；缺行不推断global或cloud_only。

### 9.1、<span id="sync-registration">设备与流绑定</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

- `PUT /api/sync/device {requestId,storeGeneration}`。storeGeneration为当前真实本地库创建时保存的小写UUID，requestId稳定8～128字节；user／device／personal来源／恢复epoch均取当前认证，不信客户端自报。响应含`scopeId,principalId,sourceContextId,deviceId,storeGeneration,restoreEpoch,streamId,qualification:'transport_only',qualifiedAck:false`。
- 同一认证device／epoch不能改storeGeneration。空库或替换库必须经本人新认证取得新device，不从旧安全库pair继承游标；409 `STORE_GENERATION_CHANGED`保留旧责任。换设备可以读取同本人／来源／epoch的流，但签名边界仍绑定各自device／store。
- `GET /api/sync/context`返回上述上下文及`head,minimumPosition:'0',permissionGeneration,authorizedScopes,coverage,snapshotRequired:true,transport:{kind:'nats_jetstream',connected,httpReplay:'compensation'}`。这是能力与当前范围，不证明任何端已安装快照；不得据此发ACK。
- sourceContextId当前只实现`personal`。边界精确身份为scope／source／principal／restoreEpoch／stream，设备token还绑定device／store。所有counter按十进制字符串处理，不能转换为浮点数。

### 9.2、<span id="sync-snapshot">完整页清单与安装顺序</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

`POST /api/sync/snapshots {requestId,storeGeneration,schemaVersion:'1',pageSize}`，pageSize为1～100。首次201，原ID／原body重复200同一manifest，不续期；异参409 `IDEMPOTENCY_CONFLICT`。manifest为：

```json
{
  "snapshotId":"UUID", "scopeId":"UUID", "principalId":"UUID",
  "sourceContextId":"personal", "deviceId":"UUID", "storeGeneration":"UUID",
  "restoreEpoch":"1", "streamId":"UUID", "schemaVersion":"1",
  "boundary":"123", "boundaryToken":"opaque signed cursor",
  "ownerWatermarks":{"auth":"10","directory":"20","conversation":"30","message":"40"},
  "permissionGeneration":"opaque monotonic-factor digest",
  "classificationWatermark":"40", "coverage":{},
  "pageManifest":[{"page":0,"count":50,"digest":"sha256 hex"}],
  "expiresAt":"UTC RFC3339"
}
```

快照有效1800秒，当前基本资源界限为200000个引用，超出明确503 `SYNC_CAPACITY_EXCEEDED`，不返回截断manifest。每个owner以真实数据keyset完整采集、所有页同watermark，全部capture结束后再读每个单调clock；三轮仍变化503 `SYNC_SNAPSHOT_BUSY`。clock在业务事务内锁行递增并同事务写Outbox，未提交低序不能迟到。个人流行锁覆盖采集至snapshot／边界提交，另端不能把边界推进到快照未包含的后续消息。多owner通过API取得共同稳定切点，不假称跨数据库分布事务。

`GET /api/sync/snapshots/{id}/pages/{page}`只允许当前本人／device／epoch／store。返回`{snapshotId,page,refs,referenceEnvelope,pageDigest,items,permissionGeneration,restoreEpoch,installable,requiresFinalValidation:true,requiresAtomicInstall:true,qualifiedAck:false}`。ref为`{owner,kind,id,revision,revisionEpoch,subjectUserId?,conversationId?,targetUserId?,state}`，不含正文／附件字节／密码／token／密文正文。referenceEnvelope是原refs JSON的base64url字节；**先对解码字节SHA256核manifest/pageDigest，再解析并深比较refs与items[].ref**，不得换一组未验证的ref或依赖跨语言重新序列化。

每个item是当前重新鉴权的`{ref,state,currentRevision,currentRevisionEpoch,revisionMatch,bodyDigest?,bodyEnvelope?,object?}`。state为available／global／unavailable／redacted／unknown；只有存在object时附bodyEnvelope，即该object原JSON字节的base64url，bodyDigest只哈希这些原字节。端侧核摘要、解析bodyEnvelope并深比较object后才用于展示／落盘；它不属于固定pageDigest，不能把当前新正文冒充原页版本。无body的未知／失权项不保留旧bodyEnvelope。任一revisionMatch=false、unknown或redacted时installable=false，禁止安装该边界。forwardUnavailable仍是可恢复当前投影，不能由客户端永久锁存。

`POST /api/sync/snapshots/{id}/validate {pageDigests:[按manifest次序完整提供]}`只作安装前复核。服务端核原归属、截止、全部页存在、密文AAD／摘要／数量／引用白名单；两轮按owner核原refs版本及当前epoch、前后核permissionGeneration。源版本变更／权限ABA为409 `SNAPSHOT_INVALIDATED`，缺／错／乱序页清单400 `SNAPSHOT_MANIFEST_INCOMPLETE`。无关新append不废原引用，通过boundary后的delta接续。成功为：

```json
{"canInstall":true,"boundaryToken":"原manifest同一个token","restoreEpoch":"1",
 "permissionGeneration":"原manifest同一个值","classificationWatermark":"当前已检查水位",
 "requiresLocalAtomicInstall":true,"qualifiedAck":false}
```

- 先把完整manifest和各页放入独立staging；校所有envelope、页数／顺序／数量／摘要／schema／epoch／permission，缺页或失败保留旧库与staging，不推进原游标，不把未出现的本地记录删除。
- 再做final validate。它不证明客户端已持久化，也不能消除响应返回到本地commit间的撤权竞态。端侧持调用捕获的实例／来源／账号／device／store／family及permission围栏，迟到响应不得覆盖后来登录或新代际。
- 最后在真实本地事务中合并投影和不可逆删除事实，并与**原boundaryToken**原子安装；事务未提交不更新活动游标。安装后立即从该token调用changes，先核当前权限再展示新cloud投影；发现新权限／恢复代际时限制受影响内容、按当前规则归档合法本地普通副本并重基线。global事实仍须清除，不能按缺失推断云删除类别。

### 9.3、<span id="sync-increments">增量、重放与删除分类</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

`GET /api/sync/changes?cursor=<boundaryToken>&limit=100`，limit为1～500。cursor是不透明签名值，必须原样耐久保存，不能用history的epoch:sequence或拆成浮点数。返回`{items,cursor,position,head,hasMore,restoreEpoch,permissionGeneration,authorizedScopes,snapshotRequired,qualifiedAck:false}`。每个owner原origin只导入一次，用户流origin唯一与head同事务分配；重放不新占游标。

- 可见项为`{kind:'reference',position,originEventId,eventDigest,eventEnvelope,ref,projection}`。eventEnvelope为原`{position,originEventId,ref}` JSON的base64url，SHA256核eventDigest后深比较公开字段；projection当前body单独按bodyEnvelope验证。事件不因正文后来变化重新签发origin或改eventDigest。
- 已分配但当前失权项为`{kind:'redacted',from,to,rangeDigest}`，明确覆盖闭区间，不暴露禁见对象ID／来源／origin。客户端把引用事件或明确跳过区间、origin去重事实和新cursor在同一事务提交，不能静默越洞。
- permissionGeneration由各owner本人的单调权限计数构成，退群再加入、拉黑再解黑不会回到旧值。旧边界遇变更始终snapshotRequired=true，不能只把新permission写回旧cursor洗白；应处理必要global清理、限制已失权cloud投影并取新快照。authenticated GET不等于已阅，不发qualified ACK。
- `POST /api/sync/classifications {messageIds:[1..100个UUID]}`用于本地已有记录逐项分类，返回`{items:[{messageId,classification,restoreEpoch}],missing:'unknown',cloudOnlyInferred:false}`。已知recall是global，当前可读普通内容available，缺失／无法证明／仅失去云权限均unknown。历史成员按原消息创建时合法membership_period证明才可得最小global墓碑；新快照也采集这些历史合法且已deleted的最小refs，即使不再有原群云读取资格，也不会因跳过旧cursor而遗漏该global事实；不会恢复旧群云正文、媒体或新发信资格，猜陌生ID也不能获得墓碑事实。

新Message附`creationEpoch,creationSequence`，在真实INSERT保存原创建tuple，编辑／互动／恢复不改。旧行无可靠证据为`unknown`，不伪造为当前epoch；原有sequence仍是当前消息事件序号。mutable ref的revisionEpoch按当前instance restore epoch重基线，新E1低version可覆盖旧E0高version；旧epoch边界409 `RESTORE_EPOCH_CHANGED`，旧设备与旧Outbox不迁移或改ID重发。

### 9.4、<span id="sync-transport">持久事件与总线边界</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

各owner只写本schema的sync_clock／sync_object_versions／sync_permission_versions／sync_outbox；sync角色只写自身schema。Outbox和NATS消息仅有来源／对象引用／版本／最小动作，无明文、E2EE密文正文或附件字节。固定scope的JetStream使用文件存储、单副本、7天／256MiB，消费器单durable；这不是强保护RPO0。

发布端收到有效PubAck才推进本域sync_publication，不删Outbox；未知沿原origin重投。sync消费事务严格校schema／scope／owner／epoch／位点／原ref，origins唯一落盘后才Ack。HTTP owner回放与NATS共用同一个原事实校验：重复必须完全相等，同origin异ref／同位点异origin503 `SYNC_ORIGIN_CONFLICT`；顺序缺口503 `SYNC_OWNER_INCOMPLETE`，事务回滚且不推进导入水位。总线断开仍保留PG责任，由HTTP补偿接续；恢复总线后重复不新分配用户游标。配置／subject冲突失败关闭，不覆盖其他stream，公网TLS不能关闭证书验证。

公共sync JSON同样持同机共享准入围栏直到响应完成；来源内容／权限排他动作等待排空。当前仍是同主机规范化StateDir，不宣称跨主机分布原子性或已完成端侧耐久资格。恢复后的新stream／epoch强制重新注册／取快照；旧总线消息不能在新epoch授予旧资格。

### 9.5、<span id="sync-self-state">schema2本人读进度、会话视图与字号偏好</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

这是当前候选的后端限定切片，已在独立双物理PG／真实HTTPS／JetStream域完成[8组定向验证](../../../../项目相关报告/测试/后端/2026-11-16-S2双PG与真实同步/README.md)，不表示五端已接线、完整I通过或固定57服务已升级。该8组报告执行时云草稿尚未挂；当前基础文本云草稿闭环见[9.6](#cloud-text-drafts)，不追认历史输入已验证新功能；`after_receipt`继续false。

**协商只选择同步协议。** 注册可增加`schemaVersion:'2'`；context／changes／classifications以`?schemaVersion=2`选择该注册，snapshot body同样冻结`schemaVersion:'2'`。省略或`'1'`沿旧schema1；其注册响应和签名cursor不强塞新的schema字段，coverage及事件过滤保持原范围。S2与S1是同user／source／epoch下的不同stream；物理`scope/source/user/device/restoreEpoch/storeGeneration`绑定独立于schema，不能以升级schema绕过空库409，也不改变可信连接profile、聊天namespace或认证device。旧cursor不能补字段伪装新schema。

S2新增`preferences`（directory owner）、`conversation_view/read_state/read_observation`（sync owner）。本人状态与owner clock／Outbox同事务提交；sync自身publisher和consumer分开，origins／streams／events没有源捕获触发器，避免导入回灌。capture和hydration沿原envelope／双采集／final validate规则；快照read投影固定`countStatus:'not_computed'`，不把采集之后实时计算的未读数塞入原cut。

| 入口 | 原始请求／响应 |
| --- | --- |
| `GET /api/me/preferences` | `{scopeId,sourceContextId:'personal',userId,revisionEpoch,version,fontPreset,explicit}`；目前只同步`fontPreset:'small'|'standard'|'large'`，不宣称主题／语言或peer隐私开关已同步 |
| `PATCH /api/me/preferences` | `{requestId,revisionEpoch,expectedVersion,fontPreset}` → `{operationId,status:'applied',current,result:{revisionEpoch,version,fontPreset}}` |
| `POST /api/me/preferences-results` | 完整原PATCH body，只核已成原事实；未知404 `PREFERENCE_OPERATION_NOT_FOUND`，不会创建。`GET /api/me/preferences/operations/{requestId}?revisionEpoch=<原epoch>`另供原操作元数据查询，不代替原body摘要证明 |
| `GET /api/conversations/{id}/view` | 本人`{id,scopeId,sourceContextId,userId,conversationId,revisionEpoch,version,pinnedRank,mutedUntil,archived,hidden,manualUnread,manualUnreadGeneration,manualUnreadMarkedAt,manualUnreadFrom}`；没有记录时返回v0默认，不写库 |
| `PATCH /api/conversations/{id}/view` | `{requestId,revisionEpoch,expectedVersion,changes:{...}}`；支持`pinnedRank:null|0..999999`、`mutedUntil:null|UTC RFC3339`、`archived/hidden/manualUnread:boolean`。显式布尔null拒绝；省略字段保持原值 |
| `POST /api/conversations/{id}/view-results` | 完整原PATCH body → `{operationId,status:'applied',current,result:<原view>}`；未知404 `VIEW_OPERATION_NOT_FOUND`，不创建 |
| `GET /api/conversations/{id}/read` | 当前有云读取资格时返回本人read视图，含精确字符串`unreadCount/countSourceWatermark`及`countStatus:'exact_current_source'`；不是对方已读回执 |
| `PUT /api/conversations/{id}/read` | `{requestId,revisionEpoch,membershipGeneration,observedMessageIds:[0..200个不重复UUID]}` → `{operationId,status:'applied',current,result:{revisionEpoch,version,membershipGeneration,readPoint:{epoch,sequence},observedMessageIds,countStatus:'not_computed',continuityStatus,hasMoreEligible,hasUnorderedEligible}}` |
| `POST /api/conversations/{id}/read-results` | 完整原PUT body，仅查原成功事实；未知404 `READ_OPERATION_NOT_FOUND`，不校成新观察、不推进读点 |

所有epoch／version／generation／计数／creation tuple都保持规范十进制字符串，requestId稳定8～128字节。实例、本人和personal来源由当前认证取值，不接受body改user/source。偏好／view使用CAS；read按真实观察并集合并并在本地事务重试CAS，不接受客户端自报“服务端最大sequence就是已读”。实际创建tuple、当前membership／历史起点和每批message owner水位必须相符；未观察的eligible洞阻止prefix前移，legacy未知不造顺序，自发／已撤回／入群前不可读消息不冒充未读。已被证实连续prefix覆盖的普通观察会压缩，并发布最小removed指针。

`manualUnread`是独立用户意图：显式设置时必须带`changes.expectedManualUnreadGeneration`，重复标记也推进自身generation；旧代际清除409 `MANUAL_UNREAD_CHANGED`，普通read观察不会自动清它。可选`manualUnreadFromMessageId`只在主动标记未读时接受当前可读真实消息，不保存引用正文。view的hidden／archive不是消息删除或云授权变更；移动／Web端如何应用静音、排序与字号仍须各端接线验证。

写意图先耐久保存原body／nonce／epoch／成员代际，未知先用对应`*-results`原body查单。原ID异body409 `IDEMPOTENCY_CONFLICT`；旧成功事实仍回原result，`current=false`不得覆盖后来的当前值；未知404不推断成功、不自动换ID。未成旧epoch409 `RESTORE_EPOCH_CHANGED`，read未成旧成员代际409 `QUALIFICATION_CHANGED`；偏好／view CAS冲突为`PREFERENCE_VERSION_CONFLICT/VIEW_VERSION_CONFLICT`。read源连续变化、元数据缺损或超过本机稀疏观察界限明确503 `READ_SUMMARY_BUSY/READ_SUMMARY_INTEGRITY_FAILED/READ_CAPACITY_EXCEEDED`，不降为近似数；保留原意图及未确认显示状态。

离群后公共read接口仍拒云资格；S2可投影已合法保留的本人metadata，read视图`writable=false`，旧观察`currentlyApplicable=false`，不会授回正文／媒体或把旧成员读点继承为新成员已读。原epoch元数据保留显示值但当前effective版本从0重基线；真实备份恢复后的S2连续性尚未在本次Phase B重跑。完整peer已读公开开关、附件字节、端侧原子安装及qualified ACK继续明确排除；基础文本云草稿另按9.6交付，云附件上传队列不在该云草稿范围。

### 9.6、<span id="cloud-text-drafts">基础文本云草稿与回复引用</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

当前只同步本人独立输入的UTF-8文本（≤16384字节）及同会话回复引用，附件待上传字节／本地队列仍留本端，不伪装为云端可迁移文件。当前来源只有认证派生的personal；不接受body自报user/source。`features.cloud_drafts`在当前实现配置自托管NATS后为true，不由管理员改写；配置标记不替代真实NATS就绪或端侧存储资格。实际验证见[云草稿报告](../../../../项目相关报告/测试/后端/20261116-云草稿/README.md)。

- `GET /api/conversations/{id}/draft`返回`{id,scopeId,sourceContextId,userId,conversationId,revisionEpoch,version,contentState:'empty|owned|unavailable',ownedText:string|null,replyState:'none|available|unavailable',reply:null|{messageId,sourceVersion},draftAdmission,writable,pendingOperationId?,recovery?:{operationId,ownedText}}`。默认无记录为v0／empty，不写库。draft版本与会话view版本独立，所有epoch/version保持规范十进制字符串。
- 新PUT的完整原body为`{requestId,revisionEpoch,expectedVersion,draftAdmission,ownedText,reply:null|{messageId,sourceVersion,replyAdmission}}`；`ownedText`必须显式字符串（允许空字符串），不能缺省或null。PUT是完整替换而非字段补丁；sourceVersion取Message.contentVersion，replyAdmission取当前精确GET消息的`Message.replyAdmission`（dr1），不得用forwardAdmission代替。回复只保存ID／版本／原资格及holder，不保存quote正文。
- `DELETE /api/conversations/{id}/draft`完整原body为`{requestId,revisionEpoch,expectedVersion,draftAdmission}`，显式清空本人草稿及旧未发布正文。`draftAdmission`（da1）来自精确GET草稿，绑定本人security／原成员与关系准入／恢复epoch，区别于消息回复dr1。requestId稳定8～128字节；旧proof不能补成最新proof重投旧意图。
- PUT／DELETE响应为`{operationId,status:'prepared|applied|retired',current,result:{revisionEpoch,version,contentState,replyState}}`。**HTTP200不单独表示保存成功，只有status=applied才是原草稿正提交；prepared仍在执行，retired禁止沿原意图发布。** 结果不携旧ownedText或quote，current=false不能回滚后来草稿。
- `POST /api/conversations/{id}/draft-results {method:'PUT|DELETE',body:<完整原body>}`仅查原事实。未知404 `DRAFT_OPERATION_NOT_FOUND`绝不创建，原ID异方法／异body／异会话409 `IDEMPOTENCY_CONFLICT`。未知先耐久保留原body／nonce并查单；允许同原ID／原body重试原PUT／DELETE收尾，不能自动改reply、proof、epoch或换ID。

新意图先在sync本地事务持久化加密intent／原摘要和版本槽，随后准备message owner的持久holder、保护无正文独立journal floor、promote原holder，再在同一sync事务核floor／pending／epoch并正提交owned ciphertext与原引用。没有本地PG事务跨owner RPC持锁。并发同草稿CAS只一方提交；409 `DRAFT_VERSION_CONFLICT`或`DRAFT_OPERATION_PENDING`要求先核当前／原操作，不能按客户端时间覆盖。

源编辑、global撤回、成员离开与账号安全失效先把相关holder持久标记revoking，并在sync回调确认后才cleared；来源消息／forward祖先的后代引用也纳入。原回调、selector摘要、prepared intent与denial floor均可续跑，worker只重试未完成责任，不靠TTL宣告清完。旧holder release不能抢掉revoking责任。外层public SH／来源动作EX覆盖至响应结束，内部同StateDir证明的RPC不嵌套加锁。

**独立文本与源引用分开。** 已正提交草稿的源失效只去reply资格／预览，ownedText保留且草稿版本推进；离群后仍可读本人已存文本并显式DELETE清理，writable=false禁止新PUT／回复授权，不恢复云历史。source暂时未知同样不扣住本人文本；replyState是当前投影，不由客户端永久锁存。若回复资格在新意图尚未正提交时失效，原操作可200返回retired；本人新输入保留在GET.recovery，用户可明确采纳独立文本、移除／重选引用后按最新GET/CAS创建新意图。recovery不会自动覆盖当前输入或自动发送。已进正式消息Outbox的原body一律不因草稿更新而改写、重绑proof或改ID补发。

独立journal只记录scope／本人／会话／对象／epoch／版本／owned-content HMAC／typed action与replyState／原operation，不含正文、密文正文、quote、AT／RT或临时许可。set／clear／reply_unavailable floor重放只阻旧发布：清空阻备份旧text复活；当前digest无相应备份正文时为unavailable，不回退另一个旧text；匹配的本人cipher仍可保留。恢复gate退休全部旧prepared／active reply授权和旧epoch正准备，新epoch有效版本从0重基线；旧未成body409 `RESTORE_EPOCH_CHANGED`，不能靠重入／新登录／改epoch修复旧授予。已成原操作只回原metadata，current=false不恢复旧正文。

草稿通过schema2的`kind:'draft'/owner:'sync'`指针进入Outbox／JetStream／快照／增量；S1不接收此kind。snapshot投影不带da1，变更前仍须精确GET草稿取新意图proof。实际客户端本地冲突保护、显示及端侧S2安装仍须各端验证，不据此签发收讫ACK；after_receipt仍false。

## 十、<span id="durable-content-receipts">存储批准与独立正文收讫</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

当前新增协议以`features.durable_receipt_protocol`声明，在配置JetStream的候选上可用；默认没有批准存储，原`sync/device`仍返回`transport_only/qualifiedAck:false`。启动参数`--receipt-policy-file`或`JOBS_IM_RECEIPT_POLICY_FILE`读取外置`{profiles:[{id,buildDigest,verificationDigest,storageContract:'native_sqlite_full_v1'}]}`；精确构建及存储测试报告SHA由部署者批准，不能把该记录当作远程硬盘证明或二进制运行身份认证。空策略继续拒绝资格升级，浏览器缓存不能自报合格。

| 入口 | 请求与结果 |
| --- | --- |
| `PUT /api/sync/storage` | `{requestId,storeGeneration,revisionEpoch,profileId,buildDigest,verificationDigest}`；只能声明已登记的当前物理库及精确批准项。返回`{result,current,storage,deletionEligible:false,afterReceiptEnabled:<当前批准物理库＋实际传输与删除事实就绪>}`，原请求异参409；换批准项产生新qualificationId，同项原身份不变 |
| `GET /api/sync/storage?schemaVersion=2` | 当前本人／device／epoch／物理库的`storage:{qualificationId,storeGeneration,restoreEpoch,profileId,buildDigest,verificationDigest,storageContract,qualified,objectQualified}`；未声明为false，不写新资格 |
| `POST /api/sync/receipt-challenges` | `{messageId,schemaVersion?}`；当前合格声明且仍能完整读取普通消息才返回原`bodyEnvelope/bodyDigest`、内容版本和时限一小时的原挑战。绑定实例／本人／device／物理库／epoch／qualification／内容与权限代际；引用转发另需完整source-holder合同，当前拒绝 |
| `POST /api/sync/receipts` | `{requestId,storeGeneration,revisionEpoch,qualificationId,kind:'message_content',challenge,bodyDigest}`；原生端必须先校验原字节，并在本地耐久事务提交正文与原回执意图。服务端验证原挑战、当前内容版本与权限、存储批准，并在SQL事务持久原请求摘要及结果 |
| `POST /api/sync/receipt-results` | 与上一行完全相同的原body，只查询已成原事实；未知404 `RECEIPT_OPERATION_NOT_FOUND`，异参409 `IDEMPOTENCY_CONFLICT`，绝不创建新回执或更换原proof |

结果的`storageQualifiedAtCommit:true`仅表示提交时部署批准项相符；`contentVersion/restoreEpoch/bodyDigest`是原内容事实，后续编辑不会改写。旧结果不确认新版或新设备，策略撤销不撤回已记录的历史事实。未提交旧epoch、换库／device、旧qualification、旧内容、权限ABA及过期挑战分别明确拒绝；调用方保留原意图，不自动补新proof或换nonce。

以上正文回执保持`objectComplete:false,targetRangeComplete:false,deletionEligible:false`，不能确认附件完整字节。原4组纯规则／3组真SQL证据见[正文检查点](../../../../项目相关报告/测试/后端/20261116-收讫与草稿实现/README.md)；当前CLI正文receipt_outbox的67本地验证另见[端侧报告](../../../../项目相关报告/测试/CLI/20261116-正文收讫/README.md)，不追认原后端轮次已测试该客户端。冻结目标、累计确认、编辑代际与保留worker已接入；单次receipt原结果中的false不冒充整范围完成。启用门槛与本轮证据见[当前收讫清理](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/收讫清理.md)。

### 10.1、<span id="durable-object-receipts">独立完整对象收讫</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

新增只读实现能力`features.durable_object_receipt_protocol`。部署批准的`storageContract:'native_sqlite_full_file_fsync_v1'`须以精确构建和验证报告覆盖原件文件fsync、父目录fsync及SQLite FULL事务；旧`native_sqlite_full_v1`没有对象资格。合同持久绑定qualification，修改批准合同不会自动升级旧声明，显式新声明旋转资格；原请求仍只回原结果。该批准不能替代实际客户端耐久证据。

| 入口 | 原请求与结果 |
| --- | --- |
| `POST /api/sync/object-receipt-challenges` | `{parentReceiptId,schemaVersion?}`，parentReceiptId为同本人／设备／物理库／epoch／qualification的**已提交正文回执requestId**。重新核当前内容版本与权限、原件绑定和实际文件SHA后返回`challenge:'ro1.…',parentReceiptId,messageId,conversationId,contentVersion,restoreEpoch,storeGeneration,qualificationId,bodyDigest,object:{objectId,sizeBytes,sha256,contentType},expiresAt,requiresDurableOriginalObject:true`；sizeBytes为规范十进制字符串，当前最多64 MiB |
| `POST /api/sync/object-receipts` | `{requestId,storeGeneration,revisionEpoch,qualificationId,kind:'object_received',parentReceiptId,challenge,objectId,sizeBytes,objectDigest}`。客户端必须完整下载原件并校字节数／SHA、文件及目录fsync后，以FULL本地事务保存对象描述及原receipt_outbox，再提交原body。URL、预览和旧rc1不能代替ro1；转发引用暂拒 |
| `POST /api/sync/object-receipt-results` | 同上一行完整原body，只查既成结果；未知404 `OBJECT_RECEIPT_OPERATION_NOT_FOUND`，异参409 `IDEMPOTENCY_CONFLICT`，不创建事实。已成结果在proof过期／批准撤销后仍按原版本返回；未知意图不能自动换nonce／proof重建 |

对象结果的`objectComplete:true`只指该设备、物理库及资格对该原件的历史确认，`targetRangeComplete/deletionEligible:false`。提交前再次检查原时限，等待本域行锁跨过截止不能确认。正文、对象字节与原证明不复制入服务端事件或回执结果；原件GET／HEAD完整响应持SH，源变更EX需等待流排空。源码及实际输入／通过／磁盘中断边界见[对象收讫检查点](../../../../项目相关报告/测试/后端/20261116-完整对象收讫/README.md)，不外推真实端落盘、HTTPS或删除策略完成。

## 十一、<span id="personal-data-export">正式本人数据导出</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

当前后端已通过独立真实验证，`features.chat_export=true`为只读实现能力；与五条旧实验入口及`--experimental-exports`分别管理。只支持认证派生的当前本人／实例／personal来源，不接受请求自报其他owner或source。证据、原失败和精确输入见[正式导出报告](../../../../项目相关报告/测试/后端/20261116-正式本人导出/README.md)，不外推五端入口、全I或243项验收。

| 入口 | 原意图与结果 |
| --- | --- |
| `POST /api/me/data-exports` | `{requestId,restoreEpoch}`；两项均字符串，nonce稳定8～128字节，epoch规范十进制；新建202，原完整body同nonce重试200，返回下述任务视图；每本人24小时最多3任务，失败也计入，不静默换ID |
| `POST /api/me/data-export-results` | 与创建完全相同的原body，仅查已成任务；未知404 `EXPORT_OPERATION_NOT_FOUND`，不新建。异body或跨实验purpose复用409 `IDEMPOTENCY_CONFLICT` |
| `GET /api/me/data-exports/{id}` | 当前本人任务视图；不是任务ID持有即授权，其他本人404。恢复后允许核对旧已成元数据，但不能重新获得交付 |
| `POST /api/me/data-exports/{id}/cancel` | `{requestId}`为独立稳定取消nonce，不能误用含restoreEpoch的创建body；状态先拒交付，writer／reader排空、完整prefix与sealed key清理完成后才返回清理完成 |
| `POST /api/me/data-exports/{id}/download-ticket` | `{}`→`{downloadToken,expiresAt,format:'zip'}`；须当前ready、当前身份／全部源仍获权且版本／期限相符；默认60秒，不晚于任务截止 |
| `GET /api/me/data-exports/{id}/download` | 当前认证＋`X-JobsIM-Download-Token`，Web另守原BFF family／CSRF合同；经受控gateway交付`application/zip`／`no-store`，不返回解密key |

任务视图为`{job,format:'zip',scope:'personal_current',capturedAt?,coverage:[],exclusions:[]}`。公开`job`为`{id,artifactId,restoreEpoch,buildGeneration,state,expiresAt,cleanupState,failureCode?}`，代际均十进制字符串、公开时间UTC RFC3339；`userId/deviceId/permissionGeneration`是服务端内部绑定，不在HTTP JSON返回，客户端不能要求这些隐藏字段。state为`requested/building/ready/cancelled/expired/failed`；原任务结果不证明当前仍可下载，必须重新查当前状态／票据。`cleanupState='source_handoff_pending'`表示还有最终持久交接，不能显示已清完。创建成功响应丢失时沿原完整body只读查单；404仍保留原意图，可显式同ID同body重试，绝不猜测新nonce。

固定capture覆盖当前本人公开账号字段、公开设备元数据、本人profile与静态头像、联系人／申请／拉黑等私有关系、当前获权会话及成员代际、公告纯引用、本人view／read及稀疏观察、已应用ownedText草稿与reply纯引用、来源字号偏好、本人举报理由／公开状态、当前普通消息和真实图片／动图／音频／视频／文件。举报只读reports公开reporter字段，不含管理员操作reason、他人report或聊天证据正文。容量超过明确20,000引用／owner、40,000总引用、单metadata对象512KiB、消息10,000条或制品1GiB时明确失败，不发布静默截断包。

ZIP内有`account-export.json`、`manifest.json`、`Data/<owner>/<kind>/<id>.json`、普通消息及安全UUID附件路径，原文件名只放metadata。普通forward明确占位，零来源正文、源附件URL或临时许可。密码／恢复码／ATRT／Cookie／E2EE秘密、客户端Outbox／原发送恢复库、pending草稿恢复text和所有fa1／dr1／da1／sendAdmission等临时授予不进入制品；普通包不承担迁移恢复会话或密钥。附件精确字节、MIME、长度与hash验证，不以别名或扩展名冒充。

每个源owner在读正文前持久prepared holder→独立typed journal→active；register／read／check均绑定原job／artifact／本人／epoch／build／源版本和期限。governance的本人report使用独立单调版本表及同事务触发器，不新增同步stream或把管理员私密理由装进源。正式源写的EX围栏先排空旧下载，源PG事务把相关holder设revoking并写cleanup账本，事务释放后跨owner清包／final journal ACK完成，才flush有界小JSON及Set-Cookie。失败503 `EXPORT_CLEANUP_PENDING`必须保留原请求；已成原nonce重试也继续旧账本，不因没有新trigger漏过收尾。同服务RPC只继承实际仍存活、method/path/规范StateDir绑定的私有锁scope；外部伪造header不免service认证或文件锁，过期scope拒调用。

制品默认24小时，封装key仅以实例key密封留服务端至合法清理；下载在源SH／artifact SH内先完整验证密文长度／SHA、每块AEAD和final、无尾随字节，再从原不可变文件第二遍解密经TLS交付，零明文落盘暂存。损坏／截断409 `EXPORT_INTEGRITY_FAILED`发生在任何ZIP正文之前；错票／到期票403 `EXPORT_DOWNLOAD_AUTH_REQUIRED`。开始后的取消／到期／网络断开只留下客户端不完整ZIP，不往流中追加JSON。源版本变化／global／撤权、任务取消／到期都会拒交付并清理；unknown holder拒绝下载，worker仅恢复持久未决责任，不靠TTL宣布清完。普通RT轮换不改变已捕获公开设备版本，不误撤包。

恢复只重建拒旧事实，全部旧epoch任务取消、sealed key／ticket清空、完整prefix与metadata／media holder退休；旧create body仍可查metadata，旧未成epoch409 `RESTORE_EPOCH_CHANGED`。已交付到用户控制的外部ZIP不由服务端声称远程抹除。收讫+n及cloud_only任务已接入相同源撤交付和期限收紧边界；原生批准、实际传输与独立事实未就绪时after_receipt仍false，不能借独立24小时包延寿。

## 十一、普通个人端侧迁移

独立同实例／同账号／personal迁移采用配对、加密capsule、另一次已认证HTTPS重新认证取钥和目标整批联网validate；不会新建服务器消息或迁移凭据／Outbox／qualified ACK。源撤权、原版本变化、global、截止与恢复均退旧交付，失败保源。完整请求、公开manifest原字节信封及AES-GCM格式见[普通迁移合同](./普通迁移/README.md)，当前源码与实际验证边界见[实现报告](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/普通端侧迁移.md)。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
