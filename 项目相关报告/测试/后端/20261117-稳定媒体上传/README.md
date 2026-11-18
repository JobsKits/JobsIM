# JobsIM 稳定媒体上传与原结果验证

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、对外合同与责任状态](#contract)
- [二、代码、围栏与拒复活](#implementation)
- [三、实际测试范围](#tests)
- [四、原件、实例与边界](#evidence)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

媒体上传支持稳定原请求：原 user／device／family／epoch 与名称、字节数、SHA-256 固定后，响应丢失可纯查原结果；同原体明确重试仍使用同一对象 UUID。准备中的原体保全在精确 staging 路径；文件完整发布并 fsync 后，才登记媒体对象和 committed 结果。已清理、已绑定、不同原体或已变化的身份不能用旧请求覆盖或重建原对象。

本批运行 `0a9751f3-83d2-4367-b824-6e13640f7d9d`：**8 个纯规则顶层测试通过、27 个数据库顶层测试通过、两个后端 cmd 编译通过，0 失败、0 跳过**。数据库组为新稳定上传 8 组与受影响的注销 19 组；纯规则含 7 组 WebM 与 1 组上传 manifest。使用真实 [**PostgreSQL**](https://www.postgresql.org/) 与生产角色处理器，RPC 为进程内连接；真实浏览器合成音轨原件已做实际分类及上传入口验证，未接真实 HTTPS、麦克风或原生设备。

## 一、<span id="contract">对外合同与责任状态</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

唯一接口合同见[稳定媒体上传合同](../../../../测试/后端/接口/media_upload/README.md)。`stable-upload-v1` 是技术协议字段，不建立产品需求的新版本。

| 入口 | 当前行为 |
| --- | --- |
| `GET /api/media/capabilities` | 通过当前实际普通身份核验后，给出协议标记、文件 64 MiB／整个请求 65 MiB 上限及原结果路径。 |
| `POST /api/media` | 有 nonce 时要求单文件与八个固定字段：`requestId/userId/deviceId/familyId/restoreEpoch/name/size/sha256`。重新计算真实字节摘要和类型，固定原责任；无 nonce 继续兼容原非幂等路径。 |
| `POST /api/media/upload-results` | 八字段短 JSON 纯查；404 不建单、不建对象，prepared 不发布／补写文件，committed 返回原对象，已退休或不可再用的原对象返回 410。 |
| 同原体明确重试 | 同上下文和元数据才可续接 prepared；固定对象 UUID，已有 final 只校验，不能替换 inode 或覆盖内容。committed 原件缺失时拒绝重建。 |

普通浏览器仍需原 BFF family、Origin 与 CSRF；原生仍需原普通凭据。身份服务从实际认证的会话确定 family、安全版本和 source generation，媒体服务不信任客户端自报 family。后继登录、恢复 epoch 或封禁不能继续原上传；没有新增跨 family 的历史查询授权，也不持久化凭据。

准备阶段仅在 `media.upload_requests` 保存不可变绑定与固定对象号，不提前插入 `media.objects`，避免让既有主备份把尚未发布的文件当作缺失原件。完整私有原体以不覆盖的 hard-link 保全为 `.upload-<objectId>`，文件和目录同步后保护最小 journal intent，再发布 final UUID；去掉 staging 并同步后，媒体对象与 committed 状态同事务提交。journal 未确认时保留 prepared，不交付 committed。

## 二、<span id="implementation">代码、围栏与拒复活</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 位置 | 职责 |
| --- | --- |
| [模型与迁移](../../../../代码/后端/Go/media_upload_model.go) | 原请求表、不可变 manifest 绑定、字段约束、响应 DTO 和 multipart 单值校验。`(scope_id,request_id)` 唯一，原对象号在 scope 内唯一；有 prepared 与 owner 清理索引。 |
| [上传与纯查](../../../../代码/后端/Go/media_upload.go) | 准备、原结果查询、实际文件完整性检查、no-replace 发布与事务提交；不把纯查变成续跑。 |
| [身份资格](../../../../代码/后端/Go/media_upload_identity.go) | 复用原普通授权规则，再按实际访问凭据解析当前原会话；只向媒体 owner 提供绑定信息。 |
| [最小安全事实](../../../../代码/后端/Go/media_upload_safety.go) | journal 仅含请求号、身份代际、对象号及不透明绑定摘要，不含文件名、原体、明文摘要或凭据；恢复缺失责任时只建 retired 拒绝记录，保留精确对象号。 |
| [准备原体清理](../../../../代码/后端/Go/media_upload_cleanup.go) | 按原 owner 有界清除精确 staging／未登记 final；拒绝非普通文件。登记对象继续走原 export-holder 排空与媒体 owner 生命周期。离线恢复清除退休及旧 prepared 的精确源。 |

共享入口仅作窄接线：[媒体路由与上传](../../../../代码/后端/Go/media.go)、[身份注册](../../../../代码/后端/Go/export_identity.go)、[迁移](../../../../代码/后端/Go/migrations.go)、[owner SH](../../../../代码/后端/Go/send_admission.go)／[网关 SH](../../../../代码/后端/Go/forward_media.go)、[journal 分发](../../../../代码/后端/Go/journal.go)、[安全重放](../../../../代码/后端/Go/operations.go)、[账户媒体擦除](../../../../代码/后端/Go/account_deletion_owner.go)与[离线恢复](../../../../代码/后端/Go/restore_runner.go)。已有 retention／迁移分支保留。

上传、能力和原结果三个路径在网关及媒体 owner 持有同一共享 admission 边界；账号安全变化、恢复和清理的排他边界先排空这些请求。准备原体清理每页 32 个，仍有剩余就返回待清理，不能提前确认 owner 完成；实际 33 原责任反例验证首轮未 ACK、第二轮全部精确清源后才完成。元数据擦除清掉重复名称、类型、大小和原始字节摘要，保留拒绝重放所需的最小绑定。

恢复重放若找不到本域原单，只建立带原对象号的 retired 责任，不创建 `media.objects`。离线清理在复制备份文件后，对原 staging／final 精确再清除；已有 committed 正常历史对象不因一个上传 intent 被重新生成或普遍删除。旧 epoch 的 prepared 原责任不会在新登录中续传。

## 三、<span id="tests">实际测试范围</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

[新上传反例](../../../../测试/后端/接口/media_upload_pg_test.go)和[真实容器用例](../../../../测试/后端/接口/media_webm_browser_test.go)覆盖以下内容：

| 新上传测试 | 已执行断言 |
| --- | --- |
| `TestMediaUploadManifestAndMinimalSafetyFields` | 规范字节数、大小与溢出拒绝；安全事实字段合法，且不含名称／原体。纯规则组。 |
| `TestMediaUploadPGOriginalResultAndStableRetry` | 纯 404 不建单或对象；查询、同体重试固定 ID；异名称、异字节和重绑定摘要拒绝，原文件不变。 |
| `TestMediaUploadPGPreparedJournalUnknownAndPublishedCrashResume` | journal 故障保持 prepared 与完整 staging；纯查不发布；模拟 final 已发布而数据库未提交后，明确重试续用原 inode／对象号。 |
| `TestMediaUploadPGCommittedMissingOrFlooredNeverRepublishes` | 原文件缺失、retention floor、owner floor 三个子项均拒绝重建。 |
| `TestMediaUploadPGSuccessorFamilyEpochAndBanCannotResumeOriginal` | 另一 family、重新绑定的同 nonce、新 epoch、旧凭据及封禁均不能接管 prepared。 |
| `TestMediaUploadPGPreparedCannotOverwriteRegisteredOrDifferentBytes` | 已登记在用对象、同 UUID 的不同文件两个子项均不被覆盖，原单不假完成。 |
| `TestMediaUploadPGReplayRetiresMissingRequestAndClearsExactUUIDs` | 缺记录的原 intent 重放只建 retired floor；staging／final 清除，邻居 UUID 保留，旧原体不能复活。 |
| `TestMediaUploadPGOwnerErasureWaitsForEveryPreparedPage` | 33 个 prepared staging／final 分页清除，首轮无完成 ACK，后续清完且重复元数据擦除后才完成。 |
| `TestMediaUploadPGActualBrowserContainerRemainsAudio` | 最新真实浏览器合成音轨经生产上传处理器仍为 `audio/webm`，实际 SHA 与原结果相同。 |

纯规则另实际运行 [WebM 原 6 组](../20261117-WebM音轨识别/README.md)及 `TestWebMActualBrowserContainerClassification`；没有借用先前不同输入的通过结果。数据库组还重新运行原注销 19 个顶层测试，覆盖受影响的 owner 清理、安全重放、媒体恢复与普通凭据拒绝。

| 阶段 | 顶层 PASS | 子项 PASS | 结果 |
| --- | ---: | ---: | --- |
| 纯规则与真实容器 | 8 | 44 | 52 个 PASS 事件，0 跳过／失败。 |
| 上传与受影响注销数据库组 | 27 | 13 | 40 个 PASS 事件，0 跳过／失败；其中 8 个新上传数据库组、19 个注销组。 |
| `jobsimd`／`jobsimops` | — | — | 两个目标实际 `go build` 退出 0，不是无测试文件的包级 skip。 |

合计 35 个不同顶层测试、57 个参数化子项，不能把 92 个 PASS 事件写成 92 个独立业务测试。所有组与构建均首轮通过；未用旧注销 19 项或纯规则替代本批新协议验证。

## 四、<span id="evidence">原件、实例与边界</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 原件 | 用途 |
| --- | --- |
| [输入 SHA256](./0a9751f3-83d2-4367-b824-6e13640f7d9d/输入SHA256.json)与[冻结源码](./0a9751f3-83d2-4367-b824-6e13640f7d9d/冻结源码/) | 134 项后端源码／模块文件、47 份递归外置测试及 1 份真实容器，共 182 项输入，执行前后 SHA 不变。47 份测试参与编译，不表示全部用例均执行。 |
| [纯规则 JSONL](./0a9751f3-83d2-4367-b824-6e13640f7d9d/01-纯规则与真实容器.jsonl)与[stderr](./0a9751f3-83d2-4367-b824-6e13640f7d9d/01-纯规则与真实容器.stderr.log) | 11:07:30 的实际分类与纯规则结果。 |
| [数据库 JSONL](./0a9751f3-83d2-4367-b824-6e13640f7d9d/06-上传与注销PG.jsonl)与[stderr](./0a9751f3-83d2-4367-b824-6e13640f7d9d/06-上传与注销PG.stderr.log) | 11:07:32～11:08:57 的上传与受影响注销结果。 |
| [原实例控制信息](./0a9751f3-83d2-4367-b824-6e13640f7d9d/02-owned-control.log)、[启前状态](./0a9751f3-83d2-4367-b824-6e13640f7d9d/03-before-start.log)与[启动后身份](./0a9751f3-83d2-4367-b824-6e13640f7d9d/05-owned-identity.log) | 核对与 a199e 原件相同的 system identifier、准确数据目录和持久写入选项；没有新 init。 |
| [停止回执](./0a9751f3-83d2-4367-b824-6e13640f7d9d/07-owned-stop.log) | 按本任务原自有集群准确 `-D` 停止，保留数据与日志。 |
| [两个 cmd 编译原件](./0a9751f3-83d2-4367-b824-6e13640f7d9d/08-两个cmd编译.jsonl)与[stderr](./0a9751f3-83d2-4367-b824-6e13640f7d9d/08-两个cmd编译.stderr.log) | 实际构建命令及退出码；产物在工程外应用支持目录。 |
| [结构化结果](./0a9751f3-83d2-4367-b824-6e13640f7d9d/结果.json)与[验证入口](./验证.mjs) | 阶段范围、输入、磁盘门禁、实例停止与完整汇总。 |

真实容器来自[最新浏览器原件](../../前端/Web/20261117-Web基础音频/03-原生浏览器-1794882303104-90890/浏览器/原生合成音轨.webm)，本批保留[固定输入副本](./0a9751f3-83d2-4367-b824-6e13640f7d9d/真实浏览器合成音轨.webm)，SHA-256 为 `3c7890c6485fcb0d914faf0c0449ae037619cdf059b36387e56f642e22b7edef`。它由真实浏览器 [**MediaRecorder**](https://www.w3.org/TR/mediastream-recording/) 与合成音轨生成，无真实麦克风；本批只读该原件，在后端做实际类型识别及生产处理器的进程内上传，不冒称真实网络上传或播放验收。

数据库仅复用 a199e 任务自有、原已停止的 `62381` 集群，system identifier 为 `7708939865112687365`，private passfile 权限 0600，`fsync=on`、`synchronous_commit=on`。只使用随机测试子库，未连接用户 `16543`，未启动用户应用、HTTP、NATS 或原生运行环境。私有凭据未复制到报告或冻结输入。

执行使用现有本地 [**Go**](https://go.dev/) 工具链及暖缓存，`GOMAXPROCS=2`、`-p=1`、`GOPROXY=off`、`GOTOOLCHAIN=local`；逐阶段超时与 500ms 磁盘监控均未触发。最低观测可用空间 **4,960,428,032 字节（约 4.62 GiB）**，高于 4 GiB 门禁。未清理缓存、既有库、SDK 或旧证据。

真实服务端 HTTPS、浏览器／Android 持久录音队列的进程重启与上传联调，以及真实麦克风和播放另由对应端继续验收；本报告不代表完整 I 基础版或全部验收项完成。此后源码变化时应按新 SHA 另跑，不能沿用本批快照结论。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
