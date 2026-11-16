# <span id="前言">运行测试.mjs</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、用途与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

CLI SQLite 隔离、数据库重开、游标事务、schema 迁移及治理原操作／撤回故障回归。临时 overlay 引入 outbox_test.go、governance_test.go、collaboration_test.go、auth_test.go、auth_https_test.go 和 sync_test.go，不复制到生产源码。从本目录运行 `node 运行测试.mjs`；工程与报告位置按脚本位置解析，无须固定工作目录。详细参数与前置见[测试说明](../README.md)。

依赖 Node.js 和现有 Go 工具链，不调用浏览器。CLI 单元入口用 `GO_BINARY` 指定 Go；联调入口还需已运行的隔离测试服务器、匹配的CA和测试配置。脚本不安装依赖、不修改系统代理或全局证书。

输出默认保存在工程根 `项目相关报告/测试/CLI/`，可用 `JOBSIM_CLI_REPORT_DIR` 指定本轮独立目录，包含真实通过组数、原始 Go 日志和被测源码摘要，失败退出码原样保留。本入口仅使用临时 SQLite／独立故障 HTTP 服务，不访问真实账号、不下载依赖或改全局代理／证书；真实 HTTPS 联调由同级独立入口执行。


默认报告现直接写入工程根 `项目相关报告/测试/CLI/执行记录/本轮唯一目录/`，Node 使用时间与 PID，Python 使用 UUID，避免覆盖迁入的历史固定文件。原有显式报告参数／环境变量优先；请选择新目录。本轮只核对独立执行目录表达式和脚本语法，没有重新运行业务测试或真实接口。

20261116 的[原源码检查点](../../../项目相关报告/测试/CLI/20261116-权威同步/README.md)不回写。资源恢复后的[完整本地回归](../../../项目相关报告/测试/CLI/20261116-权威同步编译回归/20261116-113743-192d48/README.md)由本入口实际完成 45 PASS／0 FAIL／2 需显式 HTTPS／系统库环境的 SKIP。Go 内部超时默认 120 秒，`JOBSIM_CLI_TEST_TIMEOUT` 接受正整数的 ms／s／m 时长；独立外层 watchdog 和 4GiB 磁盘门禁由本轮编译验证协调器提供，不冒充运行器的常驻资源监管。当前未做真实 API 或 OS 安全库验证。

<a id="🔚" href="#前言">回到首页</a>
