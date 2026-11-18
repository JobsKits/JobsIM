# <span id="前言">JobsIM Web 代码地图</span>

![JobsIM](../Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、入口与调用关系](#jobs-toc-heading-1)
- [二、发送与恢复路径](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、入口与调用关系 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 文件 | 职责 | 主要调用 |
| --- | --- |
| [useVisibleViewport.ts](../Src/useVisibleViewport.ts) | 桌面／手机可见视口、键盘／焦点与捏合处理；卸载清理监听，读取按实际可见交集裁剪 | --- |
| [main.tsx](../Src/main.tsx) | 浏览器入口 | App、CSS |
| [App.tsx](../Src/App.tsx) | 会话、联系人、设置和登录交互 | api、storage、demo、i18n |
| [AdminPanel.tsx](../Src/AdminPanel.tsx) | 管理员账号、举报处理与功能开关 | MFA step-up、目标／版本绑定、原操作对账 |
| [ReportPanel.tsx](../Src/ReportPanel.tsx) | 举报提交、本人记录、原操作持久恢复 | 独立namespace／epoch、未知结果原ID、当前权限列表 |
| [AnnouncementPanel.tsx](../Src/AnnouncementPanel.tsx) | 群公告当前引用／原操作 | 内容版本、过期占位、CAS与原请求持久化 |
| [MentionPicker.tsx](../Src/MentionPicker.tsx) | 显式选员与owner全体提及 | 草稿／Outbox原名单、服务端资格与通知投影 |
| [MessageLookup.tsx](../Src/MessageLookup.tsx) | 服务端搜索／本人收藏分页 | 原查询游标、源重新鉴权定位、完整空态和重载 |
| [PeerControls.tsx](../Src/PeerControls.tsx) | 本人好友／拉黑／入站申请治理 | 权威只读关系、原申请ID持久化、旧队列冻结 |
| [GroupControls.tsx](../Src/GroupControls.tsx) | 群改名／移除／转让／退出 | 原成员代际、只读保护、本地旧队列冻结 |
| [forward.ts](../Src/forward.ts) | 原引用发送与投影失效 | 不复制正文／媒体ID、固定消息附件路径、未知时清投影 |
| [api.ts](../Src/api.ts) | 同源请求、会话确认、CSRF、超时 | Go 网关 `/api`；等待刷新与过期后续期均核原快照，不将旧body绑定后继family |
| [AuthSession.ts](../Src/AuthSession.ts) | 非秘密认证上下文、原 nonce 链及恢复门禁 | 原证明 RAM 核对、跨标签页生命周期锁、同家族单次刷新 |
| [PasswordChangePanel.tsx](../Src/PasswordChangePanel.tsx) | 设置内本人改密及原单恢复表单 | 十语／三主题、管理员OTP、明确确认、字段即时清空及完整空态 |
| [PasswordChangeActions.ts](../Src/PasswordChangeActions.ts) | 改密专用网络编排 | 历史纯查omit凭据、原family首次授权、CSRF与双层WebLocks、未知／迟到围栏 |
| [PasswordChangeStore.ts](../Src/PasswordChangeStore.ts) | 独立非秘密原责任与冷启动门禁 | 原nonce／身份先严格IDB提交，无密码／OTP／请求hash；不清后来family的namespace |
| [PasswordChangeProtocol.ts](../Src/PasswordChangeProtocol.ts) | 原身份／结果与NFC证明校验 | 精确代际字符串、显著空格保留、响应字段白名单 |
| [PasswordChangeLabels.ts](../Src/PasswordChangeLabels.ts) | 本人改密十语言文案 | 表单、风险确认、恢复提示及各原请求状态 |
| [AccountDeletionPanel.tsx](../Src/AccountDeletionPanel.tsx) | 设置与登录前的注销／查询／取消界面 | 十语三主题、明确确认、完整空态／重载及凭据即时清空 |
| [AccountDeletionActions.ts](../Src/AccountDeletionActions.ts) | 原申请与用途隔离的数据权利服务 | 原family首写，匿名只查／明确续尾，RAM query/cancel proof，原取消nonce及迟到身份围栏 |
| [AccountDeletionStore.ts](../Src/AccountDeletionStore.ts) | 注销非秘密责任与冷family门禁 | 严格IDB、精确字符串版本、取消未知先查、无密码／恢复码／OTP／proof |
| [AccountDeletionProtocol.ts](../Src/AccountDeletionProtocol.ts) | 原请求、结果和受限证明白名单 | 当前版本／用途／有效期核验；query允许服务器当前版本，cancel仍精确 |
| [AccountDeletionLabels.ts](../Src/AccountDeletionLabels.ts) | 注销与数据权利十语文案 | 原责任、恢复码消费、服务器期限与终态分别解释 |
| [GroupGovernancePanel.tsx](../Src/GroupGovernancePanel.tsx) | 群策略／成员治理／邀请审批及独立恢复 | 所见快照确认、目标复制、分页、申请人当前公开姓名、RAM OTP与复制前当前邀请重验 |
| [GroupJoinPanel.tsx](../Src/GroupJoinPanel.tsx) | 新会话的同码入群与本人旧群原单恢复 | 纯查零POST、明确同nonce重试、冷启动重填同码、即时清空token及跨登录原单只读 |
| [GroupGovernanceActions.ts](../Src/GroupGovernanceActions.ts) | 群当前读取与原操作编排 | 同实例／family／device／epoch围栏、实时能力、actor／target原代际、RAM step-up |
| [GroupGovernanceStore.ts](../Src/GroupGovernanceStore.ts) | 本人群操作严格IDB责任 | 按origin／deployment／user索引；原family／namespace不变，无邀请码／OTP／授权ID／hash |
| [GroupGovernanceProtocol.ts](../Src/GroupGovernanceProtocol.ts) | 群DTO／分页／标准分享码校验 | 精确大代际、结果与目标绑定、token投影隔离、重复游标拒绝 |
| [GroupGovernanceLabels.ts](../Src/GroupGovernanceLabels.ts) | 群治理十语文案 | 管理／独立恢复／邀请／入群／未知责任与显式确认 |
| [MigrationPanel.tsx](../Src/MigrationPanel.tsx) | 设置普通迁移入口 | 配对、选择、密文文件、原请求、独立历史；关闭／后台销毁RAM凭据 |
| [MigrationActions.ts](../Src/MigrationActions.ts) | 当前本人迁移协议 | 真实physical store、原nonce纯查／明确重试、独立取钥、整批核验；已报告不重签钥 |
| [MigrationStore.ts](../Src/MigrationStore.ts) | 非秘密原责任 | family／device／store／选择与目标固定，普通内容摘要，秘密及秘密hash拒存 |
| [MigrationProtocol.ts](../Src/MigrationProtocol.ts) | 源输入与边界 | 当前原JSON白名单、字符串大版本、标准Base64／Base64URL及有界原件 |
| [MigrationArchive.ts](../Src/MigrationArchive.ts) | 加密包严格校验 | 原manifest／message JSON字节SHA、重复key／毒字段拒绝、AES-GCM与完整原件 |
| [MigrationHistoryStore.ts](../Src/MigrationHistoryStore.ts) | 私有暂存与原子历史 | 密文不含钥、guard／membership／global再核、去重保新、global清Blob；forward正文不落库 |
| [MigrationLabels.ts](../Src/MigrationLabels.ts) | 普通迁移十语文案 | 配对／源／目标／原责任／实际本机提交与服务器reported分开 |
| [PrivateMedia.tsx](../Src/PrivateMedia.tsx) | 带家族绑定的私有媒体字节 | Blob URL 生命周期、失权清理、本地 Logo 回退 |
| [AudioRecorderPanel.tsx](../Src/AudioRecorderPanel.tsx) | 输入区原生录音／停止试听／取消／明确发送 | 原AuthSnapshot与会话owner、权限等待迟到关闭、隐藏／卸载中断、RAM预览失败恢复 |
| [AudioRecorderController.ts](../Src/AudioRecorderController.ts) | 原生录音资源状态机 | 显式许可、实际MIME探测、最终dataavailable、短录／时长／字节界限和全部track清理 |
| [MessagePlayback.tsx](../Src/MessagePlayback.tsx) | 原消息当前权限复验、倍速与本机续点 | 原身份／CV／字节摘要、原生play/pause顺序、global／失权撤URL，不持久forward |
| [PlaybackCheckpointProtocol.ts](../Src/PlaybackCheckpointProtocol.ts) | 位置白名单及单调版本规则 | 256条上限、旧版本／不同摘要拒旧、精确forget、本机heard不回退 |
| [PlaybackCheckpointStore.ts](../Src/PlaybackCheckpointStore.ts) | 同库严格IDB续点 | 独立非秘密metadata键、global与auth-active／auth-frozen同事务门禁 |
| [AudioMessageLabels.ts](../Src/AudioMessageLabels.ts) | 录音与播放十语 | 许可／中断／试听／重试和本机续点状态 |
| [AttachmentSendPanel.tsx](../Src/AttachmentSendPanel.tsx) | 输入区附件原单列表 | 十语／三主题、空态／重载／更多、纯查／明确继续、导出／确认本机停止和清理待办 |
| [AttachmentSendActions.ts](../Src/AttachmentSendActions.ts) | 稳定上传与原消息恢复编排 | 实时能力、原8字段nonce查单、同bytes明确重试、旧family零HTTP，不回退旧上传 |
| [AttachmentSendStore.ts](../Src/AttachmentSendStore.ts) | 原Blob与原Outbox严格事务 | 32项／64 MiB同scope、linked保Blob、接受后释放、取消精确冻结、最终发布锁与floor门禁 |
| [AttachmentSendProtocol.ts](../Src/AttachmentSendProtocol.ts) | 原责任／manifest／收据白名单 | 单语音8 MiB、prepared非可用收据、物理holder键绑定、完整会话／client／本人global映射 |
| [AttachmentErasureProtocol.ts](../Src/AttachmentErasureProtocol.ts) | 独立最小拒旧事实 | user／conversation／client／server四字段、原单GC后仍拒复活、坏floor保留并要求修复 |
| [AttachmentSendLabels.ts](../Src/AttachmentSendLabels.ts) | 附件耐久原单十语 | 冻结、未知、接受、本机取消、旧文件导出与清理不足状态 |
| [storage.ts](../Src/storage.ts) | 严格 IndexedDB 事务与空间隔离 | 草稿、历史、Outbox |
| [StorageDatabase.ts](../Src/StorageDatabase.ts) | 共享原JobsIM v1／spaces物理数据库 | 复用既有存储，不新增数据库或迁移版本 |
| [GlobalMessages.ts](../Src/GlobalMessages.ts) | 持久global ledger与已知持有者清理 | 同来源／本人／personal围栏、保原Outbox、不单独产生同步责任 |
| [MessageFacts.ts](../Src/MessageFacts.ts) | 完整业务DTO终态观察和投影代际 | 原路由／原body标识绑定、RAM失效、存储失败限制与重试、媒体完成前重读ledger |
| [SyncProtocol.ts](../Src/SyncProtocol.ts) | 快照／增量原始字节和版本校验 | 原 envelope 摘要、完整页、精确位置、显式 redacted 连续范围 |
| [SyncStore.ts](../Src/SyncStore.ts) | 同一 IndexedDB 内的同步持久化 | 存储登记、暂存、guard CAS、游标原子提交、隔离持有者及 global 清理 |
| [SyncEngine.ts](../Src/SyncEngine.ts) | 权威同步网络编排 | WebLocks、原请求恢复、最终校验／首次增量、原 body 只读查单 proof |
| [ReceiptContext.ts](../Src/ReceiptContext.ts)／[ReceiptPanel.tsx](../Src/ReceiptPanel.tsx)／[ReceiptLabels.ts](../Src/ReceiptLabels.ts) | 逐物理库收讫协商与只读事实 | 服务器保护与本端耐久分别判断；原六字段storage-results、当前目标DG／manifest；Web仍transport_only，不声明native资格或发送ACK |
| [SyncView.ts](../Src/SyncView.ts) | 投影转本地视图 | 已知 global 终态、本地未读、原队列保留、转发精确重验 |
| [SelfDraftRecovery.tsx](../Src/SelfDraftRecovery.tsx) | 设置内本人独立草稿恢复 | 私有候选CID、精确GET当前文字／recovery、复制版本重验、清空CAS、原单只读核对及family／mount围栏 |
| [types.ts](../Src/types.ts) | 服务端及本地缓存类型 | 字符串 ID／游标／恢复代次 |
| [demo.ts](../Src/demo.ts) | 独立本地演示数据 | 不参与真实发送 |
| [i18n.ts](../Src/i18n.ts) | 十种语言的完整键表 | 页面文案 |
| [styles.css](../Src/styles.css) | 主题、字号、响应式与焦点 | 本地字体及语义色 |
| [vite.config.ts](../vite.config.ts) | 构建及带证书校验的开发代理 | `Dist/`、Go 网关 |

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、发送与恢复路径 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

用户发送 → 固定 `clientMsgId` 与 `restoreEpoch` → 本地严格事务 → POST → 服务端消息替换本地待发送项。未知结果保留原标识；切换账户会停止旧队列。恢复代次变化会归档旧缓存、冻结旧 Outbox，重新认证并拉取历史。

附件／录音明确发送 → 严格IDB保存原Blob和固定manifest → 实时稳定上传能力／纯查原结果 → 明确同体上传 → 收据先落库 → 原Outbox与enqueued同事务（保Blob）→ 原消息接受后精确释放。重启不自动发原件；取消与最终创建共用责任锁，同tx冻结精确原队列并清文件；global／completed清理和独立最小floor防晚到复活。前序41项真实IDB／Chrome、三项有效修前反例、5张原单截图及旧CCq包独立保留；其后Cleanup／ReceiptContext两Src增量已有14纯规则、13声明兼容＋3注销查询清理、新类型与统一`index-B3U1AnNU.js`成品，82产品输入／7Public／11产物对应齐，未重跑或追认新源41全矩阵。分别见[附件报告](../../../../项目相关报告/测试/前端/Web/20261117-附件耐久发送/README.md)；另轮双tab认证失败未归因、五次专项未复现，保留该限制。

共享transport在`waitForRefresh`和已有身份的`refreshSession`等待后再次执行原快照检查；相同family轮换保持原身份revision，换登录则停止旧请求，错误为`AUTH_REQUEST_ABORTED`。改密／注销的历史专用匿名查单仍走各自服务，不借此普通请求恢复授权。本轮29组群专项、认证16／改密22／注销23／导出19和类型／私有构建的精确输入分别见[围栏报告](../../../../项目相关报告/测试/前端/Web/20261117-原身份等待围栏/README.md)。

权威同步路径：能力／原同步责任 → 原物理存储登记 → 全页 envelope 校验 → 最终权限校验 → 首次连续增量 → guard／原队列 proof 再核 → 同一严格事务保存视图、原队列及连续游标。任何失败保留原意图与可恢复数据，不发送 qualified ACK；现行范围和缺口见[工程说明](../README.md#web-authoritative-sync)。

需求对应[前端架构](../../../../需求/IM前端架构表.md/IM前端架构表.md)，API 对应[接口契约](../../../后端/Go/接口契约/README.md)，测试位于[工程外测试目录](../../../../测试/前端/Web/README.md)。

<a id="🔚" href="#前言">回到首页</a>
