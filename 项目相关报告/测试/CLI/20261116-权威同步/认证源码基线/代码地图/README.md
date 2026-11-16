# CLI代码地图

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

只定位本CLI，不复制后端或其它端的源码。

新增[auth.go](../cmd/jobsim/auth.go)管理HTTP前初始意图／完整退休链、非秘密SQLite状态、单flight刷新、原family响应围栏及旧设备namespace冻结。`auth_vault_darwin/linux/windows.go`分别接[Swift Keychain助手](../原生助手/Keychain/README.md)、Secret Service和Credential Manager；`auth_lock_unix/windows.go`提供进程级持有的私有认证目录锁。随包构建见[Build.py](../Scripts/Build.py/README.md)，新增[认证故障测试](../../../../测试/CLI/auth_test.go)与[真实HTTPS协调器](../../../../测试/CLI/auth_https_test.go)由overlay按显式环境运行，系统安全库另取证。

## 一、<span id="entry">入口</span> <a href="#前言"><b>🔼</b></a> <a href="#🔚"><b>🔽</b></a>

[main.go](../cmd/jobsim/main.go)负责TLS连接、内存会话和命令编排；[store.go](../cmd/jobsim/store.go)负责SQLite schema3 增量迁移、身份隔离、WAL/FULL、原始 Outbox／独立冻结原因、历史／游标事务、版本门禁与原JSONL迁入；[local.go](../cmd/jobsim/local.go)负责原ID发送／核对、缓存回退、鉴权下载与文件保护；[governance.go](../cmd/jobsim/governance.go)负责原 request/reopen 操作持久恢复、对应 direct 冻结和原代际离群事务；[credentials.go](../cmd/jobsim/credentials.go)负责无回显密码／MFA／恢复码交互。[forward.go](../cmd/jobsim/forward.go)负责明确源版本、原引用／proof持久化、未知只读核对与明确重投、失效冻结、无正文缓存和固定消息附件代理；[collaboration.go](../cmd/jobsim/collaboration.go)负责搜索／收藏签名游标与精确源、结构化提及资格和原公告引用操作；依赖锁为`go.mod`／`go.sum`。外置[Outbox测试](../../../../测试/CLI/outbox_test.go)、[治理测试](../../../../测试/CLI/governance_test.go)及[搜索协作测试](../../../../测试/CLI/collaboration_test.go)由[CLI测试运行器](../../../../测试/CLI/运行测试.mjs/README.md)以临时overlay执行，不复制测试到生产源码。

<a id="🔚" href="#前言">返回首页</a>
