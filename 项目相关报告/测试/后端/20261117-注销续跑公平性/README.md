# JobsIM 注销续跑公平性与完成回执验证

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、当前行为与代码位置](#current-behavior)
- [二、原始红绿结果](#red-green)
- [三、冻结输入与可追溯原件](#frozen-inputs)
- [四、实例、磁盘门禁与验证边界](#environment-boundary)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

注销恢复按上次实际尝试时间轮转，持续失败的原责任保留原状态和截止时间，后续可推进责任仍能得到处理。注销完成并清除密码后，经独立管理员当前会话、MFA 与逐次审计，可重新签发仅查询原完成回执的 5 分钟证明；普通认证、旧密码、未消费恢复码与取消入口继续拒绝。

本报告对应运行 `a199e288-d70b-4682-878e-3fbecc4f5a1b`，于 2026-11-17 09:03～09:04（UTC+8）完成。原始记录确认修前 **3 项业务反例失败**，修后 **19 个顶层测试通过、0 跳过、0 失败**。这是 [**Go**](https://go.dev/) 后端在独立 [**PostgreSQL**](https://www.postgresql.org/) 实例中的定向验证，结果绑定[冻结输入](#frozen-inputs)，不代表整套基础版、真实 HTTPS 或原生端验收完成。

## 一、<span id="current-behavior">当前行为与代码位置</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 路径 | 已验证行为 |
| --- | --- |
| [注销迁移](../../../../代码/后端/Go/account_deletion.go) | `auth.account_deletions` 与 `governance.account_deletions` 均有非空 `last_attempt_at`，默认 `'-infinity'`，并有各自 schema 下的 `account_deletion_retry` 部分索引。 |
| [注销恢复 worker](../../../../代码/后端/Go/account_deletion_governance.go) | 按 `last_attempt_at,updated_at,id` 选取有界批次，在每次跨角色调用前持久化尝试时间。身份侧恢复 `prepared`；治理侧恢复 `prepared/cancel_pending/processing` 及已到截止时间的 `requested`。失败保留原责任，继续尝试批内其它责任，取消上下文时停止。 |
| [受限证明与管理员审计](../../../../代码/后端/Go/account_deletion_proof.go) | 完成状态只允许管理员审计路径签发 `query` 证明；绑定原用户、注销单、当前请求版本、安全版本和恢复 epoch，有效期 5 分钟。管理员申请 `cancel` 返回 `409 DELETION_CANCEL_CLOSED`；普通用户旧凭据仍不能签发证明。 |

身份侧索引为 `(last_attempt_at,updated_at,id) WHERE state='prepared'`；治理侧索引为相同列、条件 `state IN('prepared','cancel_pending','processing','requested')`。尝试时间只决定恢复排队，不改冷静期、请求版本、安全 journal 或清理资格；治理查询仍须筛掉尚未到期的 `requested`。

[公平性反例](../../../../测试/后端/接口/closure_audit_deletion_fairness_test.go)在身份、治理两个角色分别构造前三个原责任：前两个持续失败，第三个可推进。每轮批量 2、连续 3 轮后，第三个达到 `requested`；前两个仍保留 `prepared`、原截止时间、原 pending 操作及原安全版本，没有用丢弃失败单换取通过。

[完成回执反例](../../../../测试/后端/接口/closure_audit_completed_receipt_test.go)先执行真实注销完成并核实密码 hash 已清，再用另一名管理员的真实会话和已登记 MFA 签发查询证明。缺失 MFA 时拒绝且不落审计；证明到期后，新审计与新有效 MFA 可续签并查询同一完成结果。该测试以数据库调整到期时间模拟过期，没有等待真实 5 分钟。查询证明不能用于取消或普通会话；旧密码及注销前未消费的恢复码均返回 403，已完成状态和空密码 hash 保持不变。

## 二、<span id="red-green">原始红绿结果</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

修前运行在相同冻结副本中，仅将上述三个生产文件替换为保留的修改前原件，三个新增反例保持相同。已逐条读取[修前 JSONL 原件](./a199e288-d70b-4682-878e-3fbecc4f5a1b/04-修前反例.jsonl)，确认失败发生在预期业务断言，均非编译、环境门禁或连接失败。

| 反例 | 原始失败 | 修后 |
| --- | --- | --- |
| `TestClosureAuditDeletionCompletedReceiptCanRenewOnlyThroughAuditedAdmin` | 已完成账户的管理员首次查询证明预期 200，实际 `409 DATA_RIGHTS_CONTEXT_CHANGED`，尚未进入模拟到期与续签步骤。 | PASS |
| `TestClosureAuditDeletionIdentityRecoveryDoesNotStarveLaterIntent` | `two persistent original failures starved a later valid identity request`。 | PASS |
| `TestClosureAuditDeletionGovernanceRecoveryDoesNotStarveLaterIntent` | `two persistent original failures starved a later valid governance request`。 | PASS |

[修后 JSONL 原件](./a199e288-d70b-4682-878e-3fbecc4f5a1b/05-修后注销回归.jsonl)对应 `^Test(ClosureAuditDeletion|AccountDeletion)`。以下为准确的 19 个顶层测试范围：

| 测试 | 范围 |
| --- | --- |
| `TestAccountDeletionAdmissionRejectsOrdinaryCredentialLanes` | requested／processing／completed 普通凭据通道拒绝与不消费、不新发。 |
| `TestAccountDeletionAdmissionRetiresPreparedPasswordChange` | 已准备改密被注销门禁取代后退休。 |
| `TestAccountDeletionAdmissionHistoricalPasswordResultIsNotCurrent` | 历史改密结果不再宣称当前有效。 |
| `TestAccountDeletionAdmissionPreparedAdminResetCannotReopenTarget` | 管理员重置恢复前及预检查后不能重开注销目标。 |
| `TestAccountDeletionAdmissionOriginalAuthenticationDeliveryIsRejected` | login／registration／reauthentication 原认证结果不能交付旧凭据。 |
| `TestAccountDeletionPGRevokesFamiliesAndOriginalLookupNeverCreates` | 撤销会话族，原申请查询不创建新单。 |
| `TestAccountDeletionPGPurposeProofAndCancelPreserveIndependentDenials` | purpose 隔离，取消不解除独立处罚或恢复限制。 |
| `TestAccountDeletionPGCancelUnknownWaitsForJournalResult` | 取消 journal 结果未知时保留等待与登录栅栏。 |
| `TestAccountDeletionPGZeroCooldownCleansOwnerDataAndCannotCancel` | 零冷静期实际清理与取消关闭。 |
| `TestAccountDeletionPGRecoveryCodeIsOneUseAndPurposeCannotAuthorizeChat` | 恢复码一次性与证明不能授权聊天。 |
| `TestAccountDeletionPGSafetyCancelIntentCannotReopenAndCompletedCannotResurrect` | 取消 intent 不重开，已完成事实不复活。 |
| `TestAccountDeletionPGExpiredAndOldEpochProofCannotMutate` | 过期及旧 epoch 证明拒绝。 |
| `TestAccountDeletionPGOldVersionCanReverifyQueryButCannotRebaseCancellation` | 旧版本可重新核实查询，不能把取消重绑定到新版本。 |
| `TestAccountDeletionPGInitialJournalUnknownKeepsOriginalAndWorkerResumes` | 初始 journal 未知时保留原单并可恢复。 |
| `TestAccountDeletionPGMultipleOwnerPagesAndRestoredMediaStayErased` | 多页 owner 清理及恢复媒体保持擦除。 |
| `TestAccountDeletionPGUnqueuedLastPageReadDenial` | 尚未排到清理的末页已有账户 floor 拒读。 |
| `TestClosureAuditDeletionCompletedReceiptCanRenewOnlyThroughAuditedAdmin` | 完成回执受限续签及拒绝边界。 |
| `TestClosureAuditDeletionIdentityRecoveryDoesNotStarveLaterIntent` | 身份侧失败责任不饿死后续原单。 |
| `TestClosureAuditDeletionGovernanceRecoveryDoesNotStarveLaterIntent` | 治理侧失败责任不饿死后续原单。 |

日志另含普通准入 3 个状态、管理员重置 2 个时点及原认证交付 3 个入口，共 **8 个参数化子项**的 PASS。因此总计 **27 个 PASS 事件＝19 个顶层测试＋8 个子项**，不能称为 27 个独立业务测试。冻结的 41 份外置测试源码均参与编译输入，执行仍受上述正则限定，不能称为 41 份测试全部运行。[结构化结果](./a199e288-d70b-4682-878e-3fbecc4f5a1b/结果.json)记录修前退出码 1、修后退出码 0，均无门禁中断。

## 三、<span id="frozen-inputs">冻结输入与可追溯原件</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 原件 | 用途 |
| --- | --- |
| [修改前 SHA256](./修改前SHA256.json)与[注销迁移](./修改前-account_deletion.go)、[worker](./修改前-account_deletion_governance.go)、[证明](./修改前-account_deletion_proof.go) | 保存三个实际修改前生产原件。 |
| [输入 SHA256](./a199e288-d70b-4682-878e-3fbecc4f5a1b/输入SHA256.json) | 168 项冻结输入：后端目录 127 项源码／模块文件与 41 份外置测试源码。 |
| [修前冻结源码](./a199e288-d70b-4682-878e-3fbecc4f5a1b/修前源码/)与[替换 SHA256](./a199e288-d70b-4682-878e-3fbecc4f5a1b/修前替换SHA256.json) | 从修后冻结输入复制，仅换回三个生产文件，供红反例执行。 |
| [修后冻结源码](./a199e288-d70b-4682-878e-3fbecc4f5a1b/修后源码/) | 实际通过 19 个顶层测试的输入副本。 |
| [验证入口](./验证.mjs) | 解析项目相对位置，冻结输入后在副本执行，不修改生产模块；保留标准输出、标准错误与结果。 |
| [修前 stderr](./a199e288-d70b-4682-878e-3fbecc4f5a1b/04-修前反例.stderr.log)与[修后 stderr](./a199e288-d70b-4682-878e-3fbecc4f5a1b/05-修后注销回归.stderr.log) | 与 JSONL 原件配套保留，不能只摘取通过汇总。 |

运行结束记录 `changedLiveInputs=[]`，证明捕获到执行结束期间，168 项原工作树输入的 SHA256 未变。之后普通迁移能力独立继续开发，报告编写时 `sync_migration_validate.go` 已与该批冻结输入不同；上述三个注销生产文件与两份新增反例仍匹配。本文验证结论指向冻结副本，不把后续工作树改动追认为已验。

本轮开始时工程位于原 `GitHub/JobsIM` 工作树，08:55 左右整体迁移至 `GitHub/JobsProds/JobsIM`。旧路径消失后曾停止写入；核实新根及两份反例均完整后，在新根冻结并执行本批验证，未重建旧目录。这里保留迁移历史说明，项目内链接和验证入口仍以相对路径解析，不把旧开发机路径固化为运行依赖。

## 四、<span id="environment-boundary">实例、磁盘门禁与验证边界</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

实例由[初始化日志](./a199e288-d70b-4682-878e-3fbecc4f5a1b/01-initdb.log)与[启动日志](./a199e288-d70b-4682-878e-3fbecc4f5a1b/02-start.log)证明是本次新建的私有 [**PostgreSQL**](https://www.postgresql.org/) 实例，监听 `127.0.0.1:62381`；[实例身份原件](./a199e288-d70b-4682-878e-3fbecc4f5a1b/03-owned-cluster.log)核对独立 system identifier、准确数据目录、`fsync=on` 与 `synchronous_commit=on`。测试使用随机业务库和完整生产迁移；注销 fixture 的九个角色处理器通过进程内 RPC 相连，journal 与业务 schema 处于同一隔离实例。普通准入 fixture 的部分跨角色依赖为显式确认夹具。

测试完成后按该运行准确 `-D` 停止自有实例，[停止日志](./a199e288-d70b-4682-878e-3fbecc4f5a1b/06-stop.log)及结果中的 `ownedClusterStopped=true` 均已保留。没有连接用户默认实例执行测试或停止其它实例。私有数据目录、初始化凭据与 `pgpass` 保留在应用支持目录中；凭据目录权限 0700、凭据文件权限 0600。报告和项目冻结输入不复制秘密，不提供密码、Token、MFA 秘密或可复用证明。

本次红绿测试最低观测可用空间为 **4,576,174,080 字节（约 4.26 GiB）**，高于 4 GiB 门禁。19 项通过后，另拟执行的全包编译在启动前仅剩 **3,565,473,792 字节**，被[全包编译门禁原件](./a199e288-d70b-4682-878e-3fbecc4f5a1b/07-全包编译门禁.json)阻止，未启动 Go 编译；这不改变先前 19 项结果，也不构成全包编译通过。本报告整理没有重跑测试、重建缓存或清理数据。

该批未验证真实 HTTPS 网关、独立 journal 集群、独立双集群备份恢复、生产部署、原生界面或真机生命周期；恢复相关测试执行的是明确的重放／文件恢复钩子与夹具。结果不外推为整套保留策略、SDK、多端一致性或全部基础版功能已经验收。

本报告与本轮 Web 报告已增量登记精确目录路径，仓内过滤器的 `install`／`verify` 对156份受控文档通过。检查前后真实索引的 OID／mode／stage 及用户 flags 摘要一致，没有暂存、提交或推送；见[目录接入结果](./目录接入/结果.json)。这是本地目录形态与往返检查，不等于远端发布验收。

如需重现，由执行者先确认 `62381` 未被其它实例占用、本机工具链与现有构建缓存可用且空间满足门禁，再在本目录运行以下入口。脚本使用本地工具链和 `GOPROXY=off`，会创建新私有实例与新结果目录并停止自己启动的实例，不清理原证据或私有数据。

```shell
node ./验证.mjs
```

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
