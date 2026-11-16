# 基础剩余能力核对

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、已有基础，避免重复实现](#present)
- [二、群：确需补后端的三组能力](#group)
  - [2.1、群管理员、禁言、全员禁言与慢速](#jobs-toc-heading-4)
  - [2.2、解散与owner\_unavailable独立接管](#jobs-toc-heading-5)
  - [2.3、邀请、入群申请与原链接退休](#jobs-toc-heading-6)
- [三、隐私与陌生准入：已有block，不等于完整许可矩阵](#privacy)
- [四、举报申诉与处罚期限：部分已实现，闭环仍缺](#governance)
- [五、管理员MFA管理与权限审批](#mfa)
- [六、其余已核实I断点，避免误借已有近似能力](#other-identity)
- [七、建议接续顺序与验证要求](#handoff)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

2026-11-16只读核对现行[主需求](../../../需求/IM需求明细表.md/IM需求明细表.md)、[243主项验收表](../../../需求/IM功能验收表.md/IM功能验收表.md)与当前Jobs自有Go业务源码。未改源码／需求／阶段／能力开关，未执行API、SQL、Go构建或测试；“未验收”不作为缺代码的证据。源码可在后续任务继续变更，本轮输入SHA见[审阅输入](./审阅输入SHA.json)。

本轮排除已由owner推进的正文／对象／targets／累计ACK／retention、正式导出、本人注销和普通迁移，不另建产品版本或降排I。结论不是完整243项枚举或通过声明。各端是否已有UI不在本轮验证范围；“后端已有”只表示有可追踪业务路径，仍须对应真实回归。

## 一、<span id="present">已有基础，避免重复实现</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 业务 | 已有路径与代码证据 | 不应混称为已交付的部分 |
| --- | --- | --- |
| 好友／拉黑／重开 | [directory.go](../../../代码/后端/Go/directory.go)142～150行注册好友、接受拒绝、删除、block／unblock、reopen；341～400行同关系锁接受并建双方friend；[contact_relationship.go](../../../代码/后端/Go/contact_relationship.go)19～42行仅返回本人门禁 | contacts/requests是好友申请；接受会建立friend，不是“陌生消息请求仅接受会话” |
| 群基础管理 | [conversation.go](../../../代码/后端/Go/conversation.go)169～260行建群／自聊，312～370行加人，372～453行按原member generation踢／退出，455～510行改名／转让；[owner_lifecycle.go](../../../代码/后端/Go/owner_lifecycle.go)8～52行已有墓碑群主导致只读 | 不是完整群管理员／禁言／解散／邀请审批或独立接管 |
| 举报与管理处置 | [reports.go](../../../代码/后端/Go/reports.go)38～62行注册提交、本人／管理员列表、管理员处置与原operation查询；64～132行核目标、限频和幂等，215～271行绑定step-up／CAS／prepared结果 | 不是没有举报接口；现处置只是工单状态，非处罚执行／申诉闭环 |
| 管理员强验证 | [mfa.go](../../../代码/后端/Go/mfa.go)55～103行TOTP时间步防重放＋journal；116～142行绑定设备／动作／原请求／payload／安全代际；146～232行公开step-up和内部check／consume | 验证已有因子，不代表因子登记／轮换／丢失恢复／权限审批管理完成 |
| 管理员处罚／恢复／重置 | [admin_identity.go](../../../代码/后端/Go/admin_identity.go)77～255行已有封禁／墓碑／恢复、原操作和跨owner撤权；[reset_grant.go](../../../代码/后端/Go/reset_grant.go)21行起有受控密码重置资格；[identity.go](../../../代码/后端/Go/identity.go)884～995行严格区分恢复码和reset grant并撤旧会话 | 密码重置不能取代MFA因子恢复；记录banUntil不等于期限任务已执行 |

静态头像、公告／结构化@、普通转发、本人font／view／read／文本草稿、搜索收藏均已有独立Go模块，不在本轮重复列为“没有后端”。本人read不等于peer公开已读，私人view.mutedUntil也不是群禁言。

## 二、<span id="group">群：确需补后端的三组能力</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

### <span id="jobs-toc-heading-4" data-jobs-toc="v2">2.1、群管理员、禁言、全员禁言与慢速 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

对应FE-043、BE-011／021、T-06／11／47；[验收表](../../../需求/IM功能验收表.md/IM功能验收表.md)128行明确I，[主需求](../../../需求/IM需求明细表.md/IM需求明细表.md)558及564行要求角色矩阵、按用户慢速及原待发重验。

当前`/internal/member`只按active／owner_unavailable及direct好友判断发信（conversation.go53～85行），群没有禁言或慢速判定。实际创建／转让仅owner／member，所有群治理仍检查ownerId；PATCH DTO只有title和ownerId（460～463行）。全Go出现的muted_until属于本人通知view，不限制群发言。

可落地路径：扩展conversation本域的角色／发言策略及其独立单调代际；父群锁内幂等发布，发消息最终准入、成员／公告／@权限统一重验。复用`sendAdmission`（[send_admission.go](../../../代码/后端/Go/send_admission.go)368～381行）的opaque资格，把群发言策略代际纳入，不用membershipGeneration／joinedAt承载禁言而误砍阅读历史。禁言→解除和角色转出→转回不能复活旧未提交意图；慢速按user而非device串行并保留原已成ID读取优先。角色例外及期限应有可读DTO，不能只关按钮。

### <span id="jobs-toc-heading-5" data-jobs-toc="v2">2.2、解散与owner_unavailable独立接管 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

对应FE-041／043、T-06／22／47／50、ARCH-05／10；主需求561～566行与验收表126～128行。现有removeMember明确拒绝群主直接退出（408～409／432～433行），updateConversation仅转让；没有公开解散业务或接管操作。owner_lifecycle已有进入只读的安全路径，但所有普通治理写要求state=active且actor为当前owner，不能从该受限状态执行获权接管。

可落地路径：在conversation新增稳定原operation／expected群版本的解散与独立接管流程。复用`protect`、现有同机准入EX围栏及`freezeMembershipExportsAt`（[export_identity.go](../../../代码/后端/Go/export_identity.go)69行起）的跨owner撤权／排空；prepared先阻新发信／加入，再完整交接成员资格、邀请、forward／draft／导出持有者与同步元数据。解散撤云访问并产生可信原因，不把每个原成员合法本地历史标global。接管须独立治理权限、step-up、审计和原结果查询，不能因普通管理员admin=true自动获群内容或静默抢群主。

现有改名／转让DTO没有原requestId、expectedVersion或原role generation；它们已能执行，不应重写为“缺转让”，但新增管理闭环须补原操作／CAS，不能让响应未知的旧页面指令在角色ABA后当新操作执行。

### <span id="jobs-toc-heading-6" data-jobs-toc="v2">2.3、邀请、入群申请与原链接退休 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

对应FE-042，验收表127行为I。现addMember只接受userId、要求owner与既有friend，并立即写active成员；没有入群申请、邀请nonce／过期／次数／撤销／禁入或批准状态机。已有membership_periods与新generation只能提供加入后的历史边界，不能替代邀请授权。

可落地路径：在conversation owner持久邀请／申请与原批准operation，沿现父群锁、200人上限和membership_periods创建成员；同一批准只产生一次新成员期。被踢、解散、邀请撤销使原邀请／未完成批准代际失效，重加必须新批准；复用当前成员代际／历史ACL及同步Outbox，不从最新好友关系补旧邀请许可。

## 三、<span id="privacy">隐私与陌生准入：已有block，不等于完整许可矩阵</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

对应FE-012～018、BE-003／004／008／021、T-06／11／47／53；验收表97～103行分别为I，通讯录发现FE-015仍II。

- **陌生文本消息请求缺业务路径。** createConversation205～212行要求friend；`/internal/member`75～79行也只允许friend发direct。目录关系RPC只返回blocked／friends／permissionGeneration（directory.go61～75行）。当前没有“受限文本请求→接受该会话但不成为好友”对象、配额、接受／拒绝与目标会话准入事实。不能拿contacts申请页或reopen门禁当作该功能。
- **允许名单与按媒体／受众隐私缺模型及处理。** [source_preferences.go](../../../代码/后端/Go/source_preferences.go)17～46行的实际表／DTO只有fontPreset等版本元数据；没有隐私策略。users搜索（identity.go538～562行）按username前缀查active，再聚合profile，没有用户可发现性选择；允许名单、谁可加群及文本／图片／音频／文件许可未进入上述relationship判定。
- **好友私有备注标签、群昵称／简介仍缺写读模型。** 实际contacts表是owner／peer／state／gate／generation（[migrations.go](../../../代码/后端/Go/migrations.go)23～25行）；PATCH me只改displayName（directory.go92～118行），群PATCH只有title／ownerId，members DTO只有全局User＋role／代际（conversation.go276～308行）。不宜宣称现profile改名等于私有备注或群内昵称。
- **好友申请取消／到期也未闭合。** 现路由只有创建、接收方accept／reject、reopen；contact_requests实际表没有截止，不能从前端删行当作持久取消或过期拒旧。

可落地路径：保留现`relationLock`、block双向阻断、request_generation／permission_generation、原operation和私有relationship DTO。新增本人版本化privacy选择与明确允许名单；统一目录授权RPC给出当前动作许可／受众，并把新的权限代际纳入opaque sendAdmission。陌生消息请求建立独立会话许可与受限正文流，接受仅授该会话，不能调用resolveContact自动建双方friend。拒绝／撤销／block及解除后的ABA继续沿现单调门禁；未接受不预载附件、在线态或公开已读，所有直接API也重验。扩展sync own metadata和导出字段前保持最小化，不向peer泄露本人策略或备注。

## 四、<span id="governance">举报申诉与处罚期限：部分已实现，闭环仍缺</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

对应FE-005／082、BE-021／033、SEC-09、T-11／39；验收表90／167行及主需求223／228行要求原因、期限、申诉与审计。

现Report DTO仅目标／本人理由／state／version／时间（reports.go13～25行），resolveReport只更新reviewed／dismissed／action_required（255行），管理员处置reason存report_operations，未进入本人Report响应。没有独立申诉对象／本人申诉接口、处罚关联与受控复核结果；消息举报只验证当前源可读（275行起），没有用户主动选定证据的独立受限留存／删除策略。不能为补证据直接复制全会话正文或把管理列表当全私聊监控。

**限时封禁尚缺到期执行。** adminAccountState接受并写banUntil（admin_identity.go96／105／192／255行），但当前所有Go业务中没有按该截止将处罚推进到期的worker／鉴权重评估；authorize及login仍要求state=active（identity.go234／382行）。已有StartSessionWorkers只做凭据过期／撤销，230行仍将所有banned账户的family撤销。因此到点仍被封禁，是后端行为缺口，不只是界面倒计时。

可落地路径：复用reports／report_operations原nonce／CAS／step-up，将工单处置与独立处罚operation建立可审计关联；添加本人申诉与分权复核、仅本人可读的必要处置理由／期限，不向双方公开举报者材料。封禁用户需要受限验证／申诉入口，不能要求先恢复普通聊天权限。到期任务按原处罚ID／版本／截止串行，journal保护状态与安全代际，恢复后仍需新认证，不能解除独立墓碑或复活旧family。独立证据仅采用户选定对象，授权、保留期和源撤销各自明确。

## 五、<span id="mfa">管理员MFA管理与权限审批</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

现有TOTP校验／step-up不应重复开发，但因子生命周期确需补后端。mfa.go唯一写入因子的路径是BootstrapAdministrator（350行）；它在已有任意admin时直接拒绝初始化（334～339行），不是轮换／恢复已有管理员的入口。未见登记确认、轮换、撤销／丢失恢复、MFA专用恢复码的公开或受控运维业务；`revoked_at`只有读取判定，没有因子撤销状态机。

主需求230及530行明确MFA恢复不解除处罚、账号恢复码与MFA用途分开。实际恢复码批次只有user／generation／active（migrations.go16～18行），identity.recoverWithCredential仅账号码或password_reset grant，不能将其别名当作MFA恢复。单一admin布尔（[types.go](../../../代码/后端/Go/types.go)25行）也无法表达FE-082的审批／最小权限和独立群治理权；所有admin操作目前共用这个布尔。

可落地路径：复用`consumeMFA`／`checkStepUpTx`／admin_operations、journal与security generation，新增本人或独立授权操作者的因子prepare／确认／轮换／撤销／受控恢复原operation；新因子确认前不撤唯一旧有效因子，恢复后撤相关旧证明并要求新认证。MFA恢复材料按独立purpose／generation持久化，不改密码、不代授一般管理员step-up。角色授予／撤销另有动作权限与审计，不能放宽bootstrap覆盖已有身份或用reset grant代替MFA证明。实体模型已有设计可复用，落DDL时保留现账号恢复历史。

## 六、<span id="other-identity">其余已核实I断点，避免误借已有近似能力</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 条目 | 当前真实情况 | 复用与最小后续路径 |
| --- | --- | --- |
| 本人主动改密 | 主需求530行要求与恢复码／管理员重置分别授权；现identity.go803～809行的改密请求只能以code或grant验证，884～897行没有当前密码证明支路 | 新增已登录本人＋当前密码的专用原operation，复用密码规范／哈希、pending拒新准入、freezeUserExports、凭据与security代际、撤全family及原结果；不消耗恢复码或赋予管理员reset能力 |
| 可信设备／账号扫码授权FE-008 | sessions列表／撤销已存在；POST auth/confirm仅当前Cookie的confirmBrowser（identity.go140～147行），不是新设备一次性挑战／旧设备主动批准；jobsimops也只有bootstrap-admin等运维模式 | 新建短期单次device-link challenge／批准／取消／凭据交付，绑定待登录端与当前本人授权；可复用初始认证交付窗／原nonce／设备family与撤销，不把公开实例连接二维码当账号授权二维码 |
| 在线／正在输入FE-018与对外已读FE-036 | 当前同步coverage明确排除peer_presence／peer_public_read_receipts（[sync_service.go](../../../代码/后端/Go/sync_service.go)220行），全Go无presence／typing／公开read可见性处理。本人read／精确unread及manualUnread已实现 | 另建按当前受众／隐私过滤的短期在线／输入态；对外已读按用户聚合、可关闭／大群降级，不把设备数当人数，不混为正文耐久ACK；先补隐私矩阵，不以本人S2同步外推peer公开能力 |
| 私人消息置顶FE-035 | [migrations.go](../../../代码/后端/Go/migrations.go)38行为单message_id主键的共享pins；[message.go](../../../代码/后端/Go/message.go)569～618行仅该pin接口，群要求owner，删除按message_id，没有user维度私人消息pin | 现共享群pin已存在；另补个人message pin的user复合键／scope授权／本人同步，保群pin角色路径。不要拿conversation_views.pinnedRank当个人消息pin，也不让一人取消另一人私人置顶 |

这些条目是源码明确存在的能力边界，不是本轮实测失败；不能用缺勾选推断，也不能未经授权改为II。复杂频道／企业／机器人、支付／钱包、RTC／外部push、公开SDK继续按原阶段，不挤入本次缺口。

## 七、<span id="handoff">建议接续顺序与验证要求</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

先补群发信策略／群生命周期，再补目录隐私与陌生会话许可，随后接处罚／申诉和MFA管理；本人主动改密、可信设备及私人消息pin可按独立owner小切片推进。每片先给五端稳定DTO、真实能力开关与原结果查询，后写客户端；新handler必须同步gateway分发、SH／EX围栏、owner同步／恢复及原准入代际，不能只加路由。

保留已有已成消息按当前read ACL查原事实；旧未成意图不能因新许可重新开放就借新token首建。群禁言不撤历史阅读，解散／退群不冒充global；申诉不自动解封，MFA恢复不改处罚或通用会话。新增矩阵应实际覆盖两账号／多端、原nonce丢响应、权限ABA、期限／重启、恢复拒旧和UI／直接API一致；本报告没有执行这些测试。

<a id="🔚" href="#前言">回到首页</a>
