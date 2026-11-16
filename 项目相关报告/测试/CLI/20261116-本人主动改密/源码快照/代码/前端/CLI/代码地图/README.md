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




## S2本人状态切片

[self_state.go](../cmd/jobsim/self_state.go)实现唯一S1→S2事务跃迁、按旧stream封存原control／events／refs／pages、严格本人DTO、font／view／read及独立文本云草稿命令。`self_operations`先持久完整原method／route／body／摘要，`resume-self`先纯查；prepared、retired与applied分开，原成功不覆盖本地草稿。`sync_protocol.go`按协议分离S2新kind及sync owner水位，`sync_engine.go`在当前flags支持时自动S2、新流全快照，已有S2不能降级。原namespace／物理UUID／Outbox与终态清理保持。

[self_state_test.go](../../../../测试/CLI/self_state_test.go)覆盖自动升级／事务回滚、owner引用与对象version分离、本人边界、默认空草稿和许可剔除、prepared原body续查／本地输入保护、纯查404零创建、原route／nonce、显式观察集合及视图恢复列表。证据及尚未真实HTTPS／目标OS运行的边界见[S2本轮](../../../../项目相关报告/测试/CLI/20261116-S2本人状态/README.md)，不修改旧45组／固定57／跨编译的历史含义。

## 三、<span id="content-receipts">独立正文收讫</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[receipt.go](../cmd/jobsim/receipt.go)负责实时能力／存储声明、原bodyEnvelope SHA与普通文本DTO、rc1上下文的本地一致性检查、同SQLite事务正文＋receipt_outbox、完整原body纯查及显式同请求续跑。`sync_store.syncMigrate`增量创建收据表，不换schema4／physical UUID／认证32hex。[receipt_test.go](../../../../测试/CLI/receipt_test.go)原12组和67组／Mac构建的[正文报告](../../../../项目相关报告/测试/CLI/20261116-正文收讫/README.md)保持历史范围；当前对象增量见下节。批准项不是runtime attestation，累计目标范围／删除均未开放，无真实收讫HTTPS证明。

## 四、<span id="object-receipts">完整附件收讫</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[receipt_objects.go](../cmd/jobsim/receipt_objects.go)实现独立ro1／全文件count及SHA／父正文已成资格、本地导入或鉴权原件下载、file及目录fsync、同FULL事务`receipt_objects`＋`receipt_outbox`。`receipt.go`新增receive-message普通媒体描述，保原receive-text；原对象body、nonce、qualification、ro1和相对落点不自动补绑。storageContract／objectQualified向后兼容，旧仅正文合同不升级。

[receipt_cleanup.go](../cmd/jobsim/receipt_cleanup.go)只清私有托管对象：global事务留清理意图并冻结原pending，启动／同步drain跨可信旧库续跑。retained／unknown不误删，外部原件不碰。下载前FULL存preparation原context／内容／对象元数据，SQLite写围栏内独占建文件；完整fsync后同事务描述＋ACK原意图并删prepare。崩溃按原路径清理，晚到不复活、不广扫。13对象组＋完整80本地及eb42候选见[附件报告](../../../../项目相关报告/测试/CLI/20261116-附件收讫/README.md)，无真实API／PG。

## 五、<span id="personal-data-export">正式本人导出</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[data_export.go](../cmd/jobsim/data_export.go)负责七命令、原create／cancel body的data_exports持久化、family／physical／epoch绑定、原单纯查及显式同体续跑；cap撤回保查／取消。syncMigrate增量建表，不换schema4。

[data_export_file.go](../cmd/jobsim/data_export_file.go)负责RAM短票、鉴权流／拒redirect、ZIP完整性／SHA／fsync、流后源与本地身份围栏及不覆盖发布。auth.go只扩既有下载协调器的可选短票header。失败清本次临时inode，强杀残留私有part不自动发布／广扫。

[data_export_test.go](../../../../测试/CLI/data_export_test.go)新增8组SQLite／注入HTTP与文件反例，原失败／全量及候选分别见[本轮报告](../../../../项目相关报告/测试/CLI/20261116-正式本人导出/README.md)，未证明真实API／PG／目标OS或系统vault正向。

## 六、<span id="own-password-change">本人主动改密</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[password_change.go](../cmd/jobsim/password_change.go)负责四命令、Authentication SQLite非秘密原责任、TTY证明及无自动刷新HTTP专用路径；cap只门禁首次授权，原结果永匿名纯查，显式retry仅同原完整family／token／namespace可首建。顶层resume只开同账号元数据锁，不读取vault／激活家族。auth.go挂unknown的启动restore gate和原family已停退出；只停匹配原family及物理库，旧结果不碰后来登录。

[password_change_test.go](../../../../测试/CLI/password_change_test.go)8顶层组及OTP延迟两子例、逐原单／身份／SQL／fake vault反例见[本轮报告](../../../../项目相关报告/测试/CLI/20261116-本人主动改密/README.md)。最终注册99＝97 PASS＋2环境SKIP，原raw无FAIL／Go退出0，新Mac候选71500985；旧96／7f98和计数器95原件保留，不跨轮累加，无真实API／TTY／OS安全库正向。

<a id="🔚" href="#前言">返回首页</a>
