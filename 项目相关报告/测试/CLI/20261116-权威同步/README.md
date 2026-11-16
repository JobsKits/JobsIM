# <span id="前言">CLI 权威快照与增量：源码检查点</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、状态与原件保护](#jobs-toc-heading-1)
- [二、入口与默认行为](#jobs-toc-heading-2)
- [三、持久化与生命周期](#jobs-toc-heading-3)
- [四、待执行验证与边界](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、状态与原件保护</span><!-- jobs-toc:heading-end -->

2026-11-16：本轮仅完成源码、文档与 gofmt 语法解析；**未 Go 编译／类型检查、未执行测试、未连接真实 API、未备份／恢复或启停服务**。可用磁盘约 2.3 GiB，遵守本轮低空间门禁。原认证固定 binary 及 31 组旧证据没有更新，不能用其结果追认新源码。

此前 [24 项认证输入](./认证源归档.json)在 `认证源码基线/` 全部逐字节 SHA 复核一致。当前 [源码 SHA](./源码SHA256.json)和 [机器检查点](./源码检查点.json)独立封存；新增 14 个顶层 TestSync 方法仅为待执行源码，子场景不另算通过数量。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、入口与默认行为</span><!-- jobs-toc:heading-end -->

| 入口 | 当前源码行为／默认值 | 位置 |
| --- | --- | --- |
| `sync`／`sync 会话ID` | schema1；无责任先核实时 authoritative_sync／sync_jetstream；责任耐久后缺能力不回旧轮询 | [sync_engine.go](../../../../代码/前端/CLI/cmd/jobsim/sync_engine.go) |
| `conversations`／`history 会话ID` | 能力启用后走权威库；全局串行 sync，旧 history cursor 不冒充 stream cursor | 同上 |
| `sync-status` | 仅本地绑定／位置／限制，无 HTTP；qualifiedAck、afterReceipt false | 同上 |
| `local-history 会话ID` | 明确只读、只限同来源同 epoch 曾验证且 hash 相等的普通副本；不授予云端／媒体访问 | [sync_store.go](../../../../代码/前端/CLI/cmd/jobsim/sync_store.go) |
| `reconcile`／自动 sync 查单 | 只读原会话路由＋原完整 body；完整 DTO／本人 CID／conv＋原 hash CAS 才确认 | [local.go](../../../../代码/前端/CLI/cmd/jobsim/local.go) |
| `retry` | 用户明确动作先只读查；仅 404、未冻结及当前保护均有效可重投原 ID／body／资格；不补新 proof | 同上 |
| HTTP 业务响应 | 精确 route／过滤条件绑定后才接受 deleted global；guard 防迟到；401／epoch／实例异常退休原身份 | [auth.go](../../../../代码/前端/CLI/cmd/jobsim/auth.go) |

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、持久化与生命周期</span><!-- jobs-toc:heading-end -->

本地 schema4 增加 sync 引用、原页 envelope、origin 去重、终态、清理意图、普通合法历史资格与只读收讫 receipt。原 profile installation-id／Auth StoreGeneration 仍 32hex；Auth MessageStore、namespace 文件名和旧 Outbox payload／route／hash 不变。同步另在同一 SQLite 保存 canonical UUID physical generation，原登记 requestId 和 snapshot request 在 HTTP 前耐久；登记后缺 UUID 拒绝，不重造登记绕过后端 409。

全 manifest／页引用／投影／事件先核原 bytes SHA 再比结构、字符串精确 counter 和版本，缺页／未知不装正文。末次 validate 分类水位不低于原值，首次 changes 权限确认后才同 SQLite 事务提交 projection、连续 cursor、origin 和 guard；暂存只有引用／摘要，forward 投影正文和 proof、公告顶层 message 不入持久缓存。原发送 Outbox 的原 proof 属不可变意图，不被投影清洗改写。

普通合法旧副本按 eligibility version／hash 保留为隔离只读历史；身份待核、恢复变化及未验证旧库拒读。global 为永久终态，低版本也先擦除高版本缓存；终态和 cleanup intent 同事务，启动／每轮无需 HTTP drain。跨旧 device 库先核同可信 profile／部署／本人／personal／epoch 和原 identity hash 文件名，不明旧库保持隔离。跨库清理逐库提交、失败留意图，**不声称跨 SQLite／文件原子性**；用户显式下载到外部的文件不承诺远程擦除。

同步同 clientMsgId 永不 ACK。只读原结果需完整 Message（含 contentVersion、显式 deleted、精确版本／序号）、本人 sender／原 CID／原会话，并在提交内重比 payload_hash 与 guard；可用 forward 结果可证明原成功，sync 持久层仅接受 forward 占位。503／超时／坏 200 为零 proof，不阻挡独立已验证 global；原身份／epoch／store 变化仍拒整体。未能以原成功事实关联的 unresolved Outbox 保留原字节，不能靠猜 CID 消队列或删除原正文。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、待执行验证与边界</span><!-- jobs-toc:heading-end -->

新增 [sync_test.go](../../../../测试/CLI/sync_test.go)已接 [overlay 运行器](../../../../测试/CLI/运行测试.mjs/README.md)，14 个顶层方法包含：UUID／旧 Auth 分离、重开原意图、原 bytes／缺页、分类／权限／guard、查单 404／503／坏 200 不 ACK 且 global 可装、完整 proof／hash CAS、低版本 global、不复活、普通保留 hash、能力失效、counter／原 envelope、SQL 回滚、跨 device 清理恢复／旧库隔离、已登记 physical UUID 缺失。其内还包括无能力不登记责任、错原路由不得 global、缺 contentVersion／deleted 拒 proof、可用 forward 业务查单与 placeholder 投影分离。

本轮仅 gofmt 的 Go 语法解析为 0 且 `-l` 无输出；这不验证类型、SQL、事务、网络或平台能力。旧单测／HTTPS mock 未适配或新源码的类型／运行问题仍可能存在，资源恢复后须先编译并执行完整旧＋新注入回归，再用新独立目录验证真实 CA／服务 schema1、并发 ACL／恢复／晚响应、OS 安全库及跨库故障。Windows／Linux 与 Mac DPK entitlement 均不由本轮声明通过。

qualifiedACK／after_receipt 仍关闭；服务端已读、云草稿／偏好、正式迁移及完整第一阶段仍未验收。未改 backend、其他 App、部署、全局文档／资产表、第三方源码；无提交／推送。

<a id="🔚" href="#前言">返回首页</a>
