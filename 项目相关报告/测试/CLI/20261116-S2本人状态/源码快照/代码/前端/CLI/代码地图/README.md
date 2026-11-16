# CLI代码地图

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

只定位本CLI，不复制后端或其它端的源码。

新增[auth.go](../cmd/jobsim/auth.go)管理HTTP前初始意图／完整退休链、非秘密SQLite状态、单flight刷新、原family响应围栏及旧设备namespace冻结。`auth_vault_darwin/linux/windows.go`分别接[Swift Keychain助手](../原生助手/Keychain/README.md)、Secret Service和Credential Manager；`auth_lock_unix/windows.go`提供进程级持有的私有认证目录锁。随包构建见[Build.py](../Scripts/Build.py/README.md)，新增[认证故障测试](../../../../测试/CLI/auth_test.go)与[真实HTTPS协调器](../../../../测试/CLI/auth_https_test.go)由overlay按显式环境运行，系统安全库另取证。

## 一、<span id="entry">入口</span> <a href="#前言"><b>🔼</b></a> <a href="#🔚"><b>🔽</b></a>

[main.go](../cmd/jobsim/main.go)负责TLS连接、内存会话和命令编排；[store.go](../cmd/jobsim/store.go)负责SQLite schema4 增量迁移（本地编译回归已验）、身份隔离、WAL/FULL、原始 Outbox／独立冻结原因、历史／游标事务、版本门禁与原JSONL迁入；[local.go](../cmd/jobsim/local.go)负责原ID发送／核对、缓存回退、鉴权下载与文件保护；[governance.go](../cmd/jobsim/governance.go)负责原 request/reopen 操作持久恢复、对应 direct 冻结和原代际离群事务；[credentials.go](../cmd/jobsim/credentials.go)负责无回显密码／MFA／恢复码交互。[forward.go](../cmd/jobsim/forward.go)负责明确源版本、原引用／proof持久化、未知只读核对与明确重投、失效冻结、无正文缓存和固定消息附件代理；[collaboration.go](../cmd/jobsim/collaboration.go)负责搜索／收藏签名游标与精确源、结构化提及资格和原公告引用操作；依赖锁为`go.mod`／`go.sum`。外置[Outbox测试](../../../../测试/CLI/outbox_test.go)、[治理测试](../../../../测试/CLI/governance_test.go)及[搜索协作测试](../../../../测试/CLI/collaboration_test.go)由[CLI测试运行器](../../../../测试/CLI/运行测试.mjs/README.md)以临时overlay执行，不复制测试到生产源码。

## 二、<span id="sync">权威同步源码检查点</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

此前 schema3 与认证报告保留其历史范围；本轮独立验证 schema4。当前源码 schema4 新增 [sync_protocol.go](../cmd/jobsim/sync_protocol.go)：schema1 绑定、原 bytes envelope／SHA、全页 manifest、分类水位和连续事件；clientMsgId 按 UTF-8 8～128 字节，不按 UUID 处理。[sync_store.go](../cmd/jobsim/sync_store.go)保存独立 UUID physical generation、登记／快照原意图、引用暂存、资格 hash、永久 global／清理队列及原结果事务 CAS；旧 Auth 32hex installation identity、文件名绑定不迁移。[sync_engine.go](../cmd/jobsim/sync_engine.go)接 `sync`／`sync-status`／`local-history`、实时能力、生命周期 guard、首次 changes、原路由只读查单与启动清理。`auth.go` 在 HTTP 完成时校验 guard，并按原 method／path／body 约束 Message global，错路由不得永久清理。

[sync_test.go](../../../../测试/CLI/sync_test.go)的 14 个顶层注入方法已与旧 31 项一起通过正式 overlay 运行器，Go1.27.4 MacOS arm64 构建通过；0 失败，真实 HTTPS／原生凭据库 2 项按范围跳过，无新依赖。[本轮编译回归](../../../../项目相关报告/测试/CLI/20261116-权威同步编译回归/20261116-113743-192d48/README.md)保存新产物 SHA、首轮失败和最终证据；[此前源码检查点](../../../../项目相关报告/测试/CLI/20261116-权威同步/README.md)保持原样。该本地轮的真实服务边界由下述专项补充，跨平台原生仍待验。

[sync-https.py](../../../../测试/CLI/sync-https.py/README.md)为标准库、真实 PTY／严格实例 CA 的固定57专项入口，需显式私密 seed、新 DataRoot 和报告目录；只新账号，不 SQL／启停服务。其[最终 10 项限定结果](../../../../项目相关报告/测试/CLI/20261116-权威同步HTTPS/20261116-120401-3cbd4e/README.md)已补真实 schema1／global／旧 holder 证据，跨平台原生、OS 安全库正向及完整 I 仍未验。

该候选的[Linux／Windows amd64 编译兼容](../../../../项目相关报告/测试/CLI/20261116-权威同步交叉编译/20261116-123709-093a23/README.md)独立取证，19 项 Go／模块输入相同；没有为了兼容修改同步源码。产物为 ELF／PE，只交叉构建和只读元数据检查，没有目标系统运行或 vault 验证。

<a id="🔚" href="#前言">返回首页</a>

## S2本人状态切片

[self_state.go](../cmd/jobsim/self_state.go)实现唯一S1→S2事务跃迁、按旧stream封存原control／events／refs／pages、严格本人DTO、font／view／read及独立文本云草稿命令。`self_operations`先持久完整原method／route／body／摘要，`resume-self`先纯查；prepared、retired与applied分开，原成功不覆盖本地草稿。`sync_protocol.go`按协议分离S2新kind及sync owner水位，`sync_engine.go`在当前flags支持时自动S2、新流全快照，已有S2不能降级。原namespace／物理UUID／Outbox与终态清理保持。

[self_state_test.go](../../../../测试/CLI/self_state_test.go)覆盖自动升级／事务回滚、owner引用与对象version分离、本人边界、默认空草稿和许可剔除、prepared原body续查／本地输入保护、纯查404零创建、原route／nonce、显式观察集合及视图恢复列表。证据及尚未真实HTTPS／目标OS运行的边界见[S2本轮](../../../../项目相关报告/测试/CLI/20261116-S2本人状态/README.md)，不修改旧45组／固定57／跨编译的历史含义。
