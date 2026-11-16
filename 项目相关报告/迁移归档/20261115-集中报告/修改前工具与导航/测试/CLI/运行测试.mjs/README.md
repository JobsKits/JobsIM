# <span id="前言">运行测试.mjs</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、用途与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

CLI SQLite 隔离、数据库重开、游标事务、schema 迁移及治理原操作／撤回故障回归。临时 overlay 引入 outbox_test.go、governance_test.go 和 collaboration_test.go，不复制到生产源码。从本目录运行 `node 运行测试.mjs`；工程与报告位置按脚本位置解析，无须固定工作目录。详细参数与前置见[测试说明](../README.md)。

依赖 Node.js 和现有 Go 工具链，不调用浏览器。CLI 单元入口用 `GO_BINARY` 指定 Go；联调入口还需已运行的隔离测试服务器、匹配的CA和测试配置。脚本不安装依赖、不修改系统代理或全局证书。

输出默认保存在上一级 `报告/`，可用 `JOBSIM_CLI_REPORT_DIR` 指定本轮独立目录，包含真实通过组数、原始 Go 日志和被测源码摘要，失败退出码原样保留。本入口仅使用临时 SQLite／独立故障 HTTP 服务，不访问真实账号、不下载依赖或改全局代理／证书；真实 HTTPS 联调由同级独立入口执行。

<a id="🔚" href="#前言">回到首页</a>
