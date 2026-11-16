# Go 后端代码地图

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、入口与模块](#files)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本地图只定位Go后端工程，独立CLI见[CLI地图](../../../前端/CLI/代码地图/README.md)。

## 一、<span id="files">入口与模块</span> <a href="#前言"><b>🔼</b></a> <a href="#🔚"><b>🔽</b></a>

| 文件 | 职责 |
| --- | --- |
| [jobsimd](../cmd/jobsimd/main.go) | 角色编排、TLS、信号回收与环境参数 |
| [jobsimops](../cmd/jobsimops/main.go) | 显式备份／隔离恢复命令 |
| [server.go](../server.go) | 独立角色池、内部HTTP、gateway |
| [migrations.go](../migrations.go) | 按域可执行迁移与实例归属检查 |
| [identity.go](../identity.go)／[security.go](../security.go) | 身份、会话、CSRF、Argon2id及秘密再交付 |
| [session_refresh_model.go](../session_refresh_model.go) | 新旧会话迁移、AT／RT／family与历史约束、初始认证映射、密封BFF交付与退休日志定位 |
| [session_refresh.go](../session_refresh.go) | 单代事务消费／重用撤family、不可续绝对截止、identity后台过期与撤销责任 |
| [session_auth_retirement.go](../session_auth_retirement.go) | 新本人证明的完整未知前序闭包、保留退休边、同owner原family回收及迟到请求围栏 |
| [session_bff.go](../session_bff.go) | 不透明Cookie解析、AT过期后的刷新上下文／CSRF、原family header与初始浏览器确认 |
| [password_change.go](../password_change.go)／[password_change_worker.go](../password_change_worker.go) | 本人当前密码与管理员TOTP验证、密封替换哈希原责任、全family撤销、纯查与限流、原操作恢复及独立epoch门禁；历史回放不清新Cookie |
| [mfa.go](../mfa.go)／[admin_identity.go](../admin_identity.go)／[reset_grant.go](../reset_grant.go) | 受控管理员初始化、单次TOTP、绑定step-up、账户状态与短期密码重置 |
| [directory.go](../directory.go) | 资料、联系人、请求代际及拉黑 |
| [request_gate.go](../request_gate.go) | 收件人显式重开入站门禁、稳定原操作结果与当前资格区分 |
| [contact_relationship.go](../contact_relationship.go) | 已鉴权用户的本人关系状态DTO／只读查询，不投射peer私人策略或新授权 |
| [conversation.go](../conversation.go) | 群／私聊与当前成员资格 |
| [owner_lifecycle.go](../owner_lifecycle.go) | 群主墓碑后的owner_unavailable只读及作者擦除边界 |
| [message.go](../message.go) | 原ID幂等、游标、编辑及互动 |
| [message_lifecycle.go](../message_lifecycle.go) | 消息操作命名空间、恢复缺新编辑正文的占位字段 |
| [mentions_announcements.go](../mentions_announcements.go) | 发送时成员代际提及、blocks投影、引用式公告与原操作事实 |
| [forward_projection.go](../forward_projection.go) | 纯引用转发、读者原proof、逐层源资格／内容／恢复代际投影，临时占位不等同永久ref状态 |
| [forward_revocation.go](../forward_revocation.go) | owner pending交接、持久后代invalidating、holder排空与分批事件，message后台仅恢复未决工作 |
| [forward_media.go](../forward_media.go) | 目标消息鉴权原媒体代理、Range／hash；公开JSON与流完整响应围栏及内部method/path持锁证明 |
| [send_admission.go](../send_admission.go) | 本人原发送快照、独立direct授权代际、同机跨角色OS提交围栏、原事务持久标记及显式维护对账 |
| [search.go](../search.go) | owner权威读取范围先筛选后分页、本人收藏源引用、签名游标及单会话／原消息定位复验 |
| [sync_protocol.go](../sync_protocol.go)／[sync_service.go](../sync_service.go) | 纯引用DTO、设备库绑定、跨owner共同cut、快照manifest／分页及本人签名增量流 |
| [sync_outbox.go](../sync_outbox.go)／[sync_owner.go](../sync_owner.go) | 本域事务clock／权限代际／指针触发器、完整keyset capture、动态当前权限投影及raw body envelope |
| [sync_nats.go](../sync_nats.go)／[sync_origin.go](../sync_origin.go) | 自托管JetStream PubAck／事务后Ack、断线重试、HTTP与总线共用原事实冲突拒绝 |
| [sync_private_unix.go](../sync_private_unix.go)／[sync_private_windows.go](../sync_private_windows.go) | 总线Token文件的Unix owner／0600与Windows真实owner／DACL门禁，不用跨平台FileMode假扮ACL |
| [sync_validation.go](../sync_validation.go)／[sync_classification.go](../sync_classification.go) | 安装前完整清单／两轮原refs复验、历史合法成员仅global墓碑；不签发ACK或恢复云正文 |
| [sync_receipt_policy.go](../sync_receipt_policy.go)／[sync_receipts.go](../sync_receipts.go) | 部署批准的原生构建与验证报告、物理库资格、原正文挑战及幂等正文事实；旧SQLite合同不升级文件资格 |
| [sync_object_receipts.go](../sync_object_receipts.go)／[media_receipts.go](../media_receipts.go) | 同库既成正文事实为父的独立原件ro1、真实文件摘要、原单查核及本域锁后时限门禁；不授予完整目标／删除资格 |
| [self_read.go](../self_read.go)／[self_unread.go](../self_unread.go)／[message_read_points.go](../message_read_points.go) | 本人真实观察并集、创建tuple洞／历史起点、精确当前未读与原body查单，不假充对外已读或收讫 |
| [self_views.go](../self_views.go)／[source_preferences.go](../source_preferences.go)／[sync_self.go](../sync_self.go) | 独立view／manualUnread代际、字号偏好与schema2当前本人metadata投影 |
| [self_draft.go](../self_draft.go)／[self_draft_floor.go](../self_draft_floor.go) | 本人独立文本云草稿、原body只读结果、加密prepared intent／后台续跑、CAS与不可回退floor；不改正式Outbox |
| [self_draft_safety.go](../self_draft_safety.go)／[self_draft_sync.go](../self_draft_sync.go) | typed无正文journal／恢复拒旧、reply失效保ownedText及S2 draft指针触发器 |
| [draft_reply.go](../draft_reply.go)／[draft_reply_holders.go](../draft_reply_holders.go)／[draft_reply_revocation.go](../draft_reply_revocation.go) | dr1原读者资格、prepared／active／revoking holder、原selector账本与源／成员／principal／forward后代回调排空 |
| [media.go](../media.go)／[media_format.go](../media_format.go) | 私有字节、消息授权与容器识别 |
| [avatar.go](../avatar.go)／[avatar_media.go](../avatar_media.go)／[asset_migrations.go](../asset_migrations.go) | 头像owner prepared／媒体规范化与原结果、最小举报模型 |
| [reports.go](../reports.go) | 当前源授权、本人举报与强验证处置 |
| [journal.go](../journal.go)／[operations.go](../operations.go) | 独立最小日志、认证加密归档和恢复门禁 |
| [restore_runner.go](../restore_runner.go) | 独占恢复操作记录、同epoch断点续跑、日志覆盖与源／目标派生资产清理 |
| [control.go](../control.go)／[web.go](../web.go) | 实际能力查询与同源静态产物 |
| [export.go](../export.go)／[export_worker.go](../export_worker.go) | 显式实验导出协议、分块加密、worker／reader栅栏与关闭后的清理责任 |
| [message_export.go](../message_export.go)／[media_export.go](../media_export.go)／[export_identity.go](../export_identity.go) | 正文读取前holder登记、消息／账号／设备／成员准入冻结与跨owner排空 |
| [export_restore.go](../export_restore.go) | 派生导出旧代际拒旧与恢复取消，不恢复旧ready/key |
| [personal_export.go](../personal_export.go)／[personal_export_worker.go](../personal_export_worker.go) | 正式本人任务、完整原body只读查单、固定引用清单、私有ZIP构建／清理与恢复拒旧；旧实验purpose隔离 |
| [personal_export_owner.go](../personal_export_owner.go)／[personal_export_safety.go](../personal_export_safety.go) | 五类metadata源owner的prepared／active holder、源事务版本与revoking／cleanup账本、无正文typed journal；report只读本人理由／公开状态 |
| [personal_export_media.go](../personal_export_media.go)／[personal_export_crypto.go](../personal_export_crypto.go) | 本人静态头像独立用途holder与精确字节；完整密文／final／digest先验后TLS明文ZIP交付，key不外送 |
| [personal_export_fence.go](../personal_export_fence.go) | 公共源写EX到提交后排空再flush小JSON／Cookie，原已成nonce续清理；治理同owner可继承实时锁scope，不嵌套文件锁 |
| [Unix文件锁](../export_lock_unix.go)／[Windows文件锁](../export_lock_windows.go) | 进程崩溃释放、带context的排它writer／共享reader文件栅栏 |
| [第三方许可清单](../第三方依赖/README.md) | 直接／传递模块版本、上游来源与原文保留 |

外置测试、运行参数、当前限制及证据统一见[工程说明](../README.md)。

<a id="🔚" href="#前言">返回首页</a>
