# <span id="前言">运行测试.mjs</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、用途与运行](#jobs-toc-heading-1)
- [本人注销](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、用途与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

CLI SQLite 隔离、数据库重开、游标事务、schema 迁移及治理原操作／撤回故障回归。临时 overlay 自动引入测试目录中全部普通 `*_test.go` 文件，当前15份／139个顶层方法，不复制到生产源码；新增同类测试无需手工维护名单。范围含S2本人状态／草稿、正文／对象及逐store收讫、正式导出、主动改密、注销、群治理和普通迁移。从本目录运行 `node 运行测试.mjs`；工程与报告位置按脚本位置解析，无须固定工作目录。详细参数与前置见[测试说明](../README.md)。

当前完整入口实跑137 PASS／0 FAIL／2环境SKIP，139个顶层方法与15份源码逐名称完全对齐。真实HTTPS认证协调器与系统秘密存储诊断仍因未提供显式环境跳过；[本轮证据](../../../项目相关报告/测试/回归/20261118-全项目回归/README.md)保留原名单漏挂22个方法及补齐前后的原始日志。

依赖 Node.js 和现有 Go 工具链，不调用浏览器。CLI 单元入口用 `GO_BINARY` 指定 Go；联调入口还需已运行的隔离测试服务器、匹配的CA和测试配置。脚本不安装依赖、不修改系统代理或全局证书。

输出默认保存在工程根 `项目相关报告/测试/CLI/`，可用 `JOBSIM_CLI_REPORT_DIR` 指定本轮独立目录，包含真实通过组数、原始 Go 日志和被测源码摘要，失败退出码原样保留。本入口仅使用临时 SQLite／独立故障 HTTP 服务，不访问真实账号、不下载依赖或改全局代理／证书；真实 HTTPS 联调由同级独立入口执行。


默认报告现直接写入工程根 `项目相关报告/测试/CLI/执行记录/本轮唯一目录/`，Node 使用时间与 PID，Python 使用 UUID，避免覆盖迁入的历史固定文件。原有显式报告参数／环境变量优先；请选择新目录。本轮只核对独立执行目录表达式和脚本语法，没有重新运行业务测试或真实接口。

20261116 的[原源码检查点](../../../项目相关报告/测试/CLI/20261116-权威同步/README.md)不回写。资源恢复后的[完整本地回归](../../../项目相关报告/测试/CLI/20261116-权威同步编译回归/20261116-113743-192d48/README.md)由本入口实际完成 45 PASS／0 FAIL／2 需显式 HTTPS／系统库环境的 SKIP。Go 内部超时默认 120 秒，`JOBSIM_CLI_TEST_TIMEOUT` 接受正整数的 ms／s／m 时长；独立外层 watchdog 和 4GiB 磁盘门禁由本轮编译验证协调器提供，不冒充运行器的常驻资源监管。当前未做真实 API 或 OS 安全库验证。

正文收讫后续已将`self_state_test.go`、`receipt_test.go`加入实际overlay，当前完整[67组本地结果](../../../项目相关报告/测试/CLI/20261116-正文收讫/README.md)与上述旧45组各按输入独立保存；真实服务及系统安全库仍需显式入口，不因该运行器通过而追认。

历史11份输入注册99个顶层方法；[当时源码回归](../../../项目相关报告/测试/回归/20261116-全项目回归/Web-CLI.md)执行97 PASS／0 FAIL／2环境SKIP，源码方法与原日志集合无缺项或多项。统计识别提示后粘行的顶层结果，排除带 `/` 的子测试；跳过项分别是真实HTTPS协调器和系统安全库诊断，不能算作通过。历史45／67／80／88等报告仅证明各自输入，保留原件。


## <span id="jobs-toc-heading-2" data-jobs-toc="v2">本人注销</span><!-- jobs-toc:heading-end -->

登录后`account-delete`确认申请；`account-deletions`、`deletion-reconcile REQUEST_ID`、`deletion-retry REQUEST_ID`、`deletion-query REQUEST_ID`、`deletion-cancel REQUEST_ID`和`deletion-cancel-retry REQUEST_ID`也可从启动参数进入本机原任务。口令／恢复码／OTP只经TTY读取，禁止秘密参数；原nonce／family／物理库／epoch和cancel版本公开责任WAL／FULL耐久。未知先查，明确续原单；受限proof只RAM，completed精确清原本人所有本机store及托管ReceiptObjects，其他profile／user和外部文件保留。

注销专项当时实跑全runner115通过／0失败／2环境跳过、注销11方法通过（含一次恢复码unknown取消），host二进制已构建；随后main.go将未保存默认URL改为9443，当时因4GiB磁盘门禁未重编。该历史限制不作为当前源码测试状态，当前回归见本页前文。详见[独立限定报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/CLI注销.md)，不把HTTP／vault适配当真实HTTPS／安全库或TTY操作通过。

<a id="🔚" href="#前言">回到首页</a>
