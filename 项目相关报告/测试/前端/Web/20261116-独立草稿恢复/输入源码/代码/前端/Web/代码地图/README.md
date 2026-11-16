# <span id="前言">JobsIM Web 代码地图</span>

![JobsIM](../Public/Logo.svg)

[toc]

---

## 一、入口与调用关系 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 文件 | 职责 | 主要调用 |
| --- | --- | --- |
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
| [api.ts](../Src/api.ts) | 同源请求、会话确认、CSRF、超时 | Go 网关 `/api` |
| [AuthSession.ts](../Src/AuthSession.ts) | 非秘密认证上下文、原 nonce 链及恢复门禁 | 原证明 RAM 核对、跨标签页生命周期锁、同家族单次刷新 |
| [PrivateMedia.tsx](../Src/PrivateMedia.tsx) | 带家族绑定的私有媒体字节 | Blob URL 生命周期、失权清理、本地 Logo 回退 |
| [storage.ts](../Src/storage.ts) | 严格 IndexedDB 事务与空间隔离 | 草稿、历史、Outbox |
| [StorageDatabase.ts](../Src/StorageDatabase.ts) | 共享原JobsIM v1／spaces物理数据库 | 复用既有存储，不新增数据库或迁移版本 |
| [GlobalMessages.ts](../Src/GlobalMessages.ts) | 持久global ledger与已知持有者清理 | 同来源／本人／personal围栏、保原Outbox、不单独产生同步责任 |
| [MessageFacts.ts](../Src/MessageFacts.ts) | 完整业务DTO终态观察和投影代际 | 原路由／原body标识绑定、RAM失效、存储失败限制与重试、媒体完成前重读ledger |
| [SyncProtocol.ts](../Src/SyncProtocol.ts) | 快照／增量原始字节和版本校验 | 原 envelope 摘要、完整页、精确位置、显式 redacted 连续范围 |
| [SyncStore.ts](../Src/SyncStore.ts) | 同一 IndexedDB 内的同步持久化 | 存储登记、暂存、guard CAS、游标原子提交、隔离持有者及 global 清理 |
| [SyncEngine.ts](../Src/SyncEngine.ts) | 权威同步网络编排 | WebLocks、原请求恢复、最终校验／首次增量、原 body 只读查单 proof |
| [SyncView.ts](../Src/SyncView.ts) | 投影转本地视图 | 已知 global 终态、本地未读、原队列保留、转发精确重验 |
| [SelfDraftRecovery.tsx](../Src/SelfDraftRecovery.tsx) | 设置内本人独立草稿恢复 | 私有候选CID、精确GET当前文字／recovery、复制版本重验、清空CAS、原单只读核对及family／mount围栏 |
| [types.ts](../Src/types.ts) | 服务端及本地缓存类型 | 字符串 ID／游标／恢复代次 |
| [demo.ts](../Src/demo.ts) | 独立本地演示数据 | 不参与真实发送 |
| [i18n.ts](../Src/i18n.ts) | 十种语言的完整键表 | 页面文案 |
| [styles.css](../Src/styles.css) | 主题、字号、响应式与焦点 | 本地字体及语义色 |
| [vite.config.ts](../vite.config.ts) | 构建及带证书校验的开发代理 | `Dist/`、Go 网关 |

## 二、发送与恢复路径 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

用户发送 → 固定 `clientMsgId` 与 `restoreEpoch` → 本地严格事务 → POST → 服务端消息替换本地待发送项。未知结果保留原标识；切换账户会停止旧队列。恢复代次变化会归档旧缓存、冻结旧 Outbox，重新认证并拉取历史。

权威同步路径：能力／原同步责任 → 原物理存储登记 → 全页 envelope 校验 → 最终权限校验 → 首次连续增量 → guard／原队列 proof 再核 → 同一严格事务保存视图、原队列及连续游标。任何失败保留原意图与可恢复数据，不发送 qualified ACK；现行范围和缺口见[工程说明](../README.md#web-authoritative-sync)。

需求对应[前端架构](../../../../需求/IM前端架构表.md/IM前端架构表.md)，API 对应[接口契约](../../../后端/Go/接口契约/README.md)，测试位于[工程外测试目录](../../../../测试/前端/Web/README.md)。

<a id="🔚" href="#前言">回到首页</a>
