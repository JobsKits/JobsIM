# <span id="前言">CLI schema1 本地编译与完整回归</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、结论与证据范围](#jobs-toc-heading-1)
- [二、失败、修复与最终验证](#jobs-toc-heading-2)
- [三、资源、命令与产物](#jobs-toc-heading-3)
- [四、仍需单独验证](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、结论与证据范围</span><!-- jobs-toc:heading-end -->

2026-11-16，MacOS arm64 的 Go1.27.4 编译通过，正式 overlay 运行器完成 **45 个顶层本地测试 PASS、0 FAIL、2 SKIP**（旧 31＋新 sync 14）。两项跳过明确要求真实 HTTPS realm 或 OS 安全库；本轮清除对应环境，不连接真实 API／PG、不启停服务。新二进制只放本轮 Application Support 隔离 Products；旧固定 binary、旧 31 项报告及 [source-only 检查点](../../20261116-权威同步/README.md)原样保留，24 项原认证输入 SHA 一致。

[结构化汇总](./结果汇总.json)、[最终运行器结果](./06-final-runner/SQLite结果.json)、[完整最终日志](./06-final-runner/SQLite测试.log)、[当前源 SHA 与对应快照](./源码SHA256.json)共同绑定此轮。真实 schema1 服务、Windows／Linux 原生、安全库正向及完整第一阶段未验收。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、失败、修复与最终验证</span><!-- jobs-toc:heading-end -->

| 步骤 | 结果 | 限定解释 |
| --- | --- | --- |
| 01 初始 build | PASS，29.1s | 单独冷编译 cache，新 binary |
| 02 Node24 入口 | SIGKILL -9，6.45s，零输出 | 原因未证实；不是 Go test 通过／失败，不改全局 runtime |
| 03 直接同 overlay | 38 PASS／7 FAIL／2 SKIP，10.08s | 新 14 项全部通过；7 个旧测试因原 DTO／查单路由／schema 夹具不符新合同而失败 |
| 04 修复后完整直接 overlay | 45 PASS／0 FAIL／2 SKIP，7.04s | 旧、新本地集全跑 |
| 05 最终新 binary build | PASS，3.01s | 包含缺原准入只读保护修复 |
| 06 正式 Node18 runner | 45 PASS／0 FAIL／2 SKIP，3.01s | 现成镜像工具链 Node18.20.1，进程显式路径，不改全局环境 |
| 07 新 binary `-h` | 退出0 | flag 解析即返回，无 HTTP／认证／凭据库调用 |

首轮日志与 JSON 均保留。首失败步骤未另封所有旧夹具原字节 SHA，不为该步骤补造可复现声明；最终完整源码快照及运行器内 SHA 只绑定最终通过轮。

旧夹具更新为完整业务 Message（UUID 消息／会话／sender、contentVersion、显式 deleted、type），旧 history GET 查单改为纯只读 `message-results` 的原 body，区分 readonly POST 与创建 POST，schema 断言从 3 到 4，恢复 epoch 变化改为零 HTTP。数值大 ID／版本缓存测试仍保留其原目的，不把 opaque clientMsgId UUID 化，不放松生产 DTO 校验。

同时修复一处实际客户端边界：[retry](../../../../../代码/前端/CLI/cmd/jobsim/local.go)对于旧行缺原 sendAdmission，即使只读返回 404，也只冻结并保留原 body／hash，不首次消息 POST、不取新资格。原 admission 测试已断言缺值路径创建 POST 次数不变。原 frozen 行只能只读核对，不被用户 retry 自动复活。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、资源、命令与产物</span><!-- jobs-toc:heading-end -->

每步 GOMAXPROCS=2、`-p=1`，CGO=0、GOTOOLCHAIN=local、GOPROXY=off；依赖只复用已有模块 cache，没有下载或改变源码依赖。编译 cache／临时文件／新 binary 位于 `Application Support/JobsIM/Build/CLI/20261116-113743-192d48`，逻辑合计约 227MiB。外层 watchdog 每秒观测本轮进程，180 秒上限、磁盘 4GiB 硬门禁；最低观测空闲 13.42GiB，没有超时／门禁中止。未宣称测量了精确峰值 RSS。

```text
go build -p=1 -o <本轮Products/jobsim-final> ./cmd/jobsim
go test -p=1 -overlay <本轮overlay.json> -timeout 120s -count=1 -v ./...
<现成Node18.20.1> 测试/CLI/运行测试.mjs/运行测试.mjs
```

运行器支持 `GO_BINARY`、`JOBSIM_CLI_REPORT_DIR` 和 `JOBSIM_CLI_TEST_TIMEOUT`；本轮确实使用隔离 cache、显式新报告目录和 120s 内部 timeout。步骤原命令及路径见各 JSON，不将该轮外部 watchdog 冒充运行器内置常驻监控。

最终新 binary SHA256：`18738c34228b3318a7a68c8eb34854cb9c3a84fc0351d405a69d16d365f12929`。产物路径见 [05 build JSON](./05-final-build.json)，不写回旧发行文件。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、仍需单独验证</span><!-- jobs-toc:heading-end -->

本地注入 HTTP／SQLite 成功不能证明真实服务快照、NATS、ACL 并发或恢复闭环已通过；后续须独立严格 CA、新账号／profile／DataRoot、真实 TTY 取证。OS 安全库正向不在本轮；Mac DPK entitlement 原缺口保留，临时内存会话不冒充 securestore。qualifiedACK／after_receipt 仍关闭，服务端已读／云草稿偏好／完整迁移与全部第一阶段仍未验收。跨 device 清理为持久意图＋逐库事务，不宣称跨 SQLite 原子；不明旧 namespace 隔离，用户外部下载不远程擦除。

只改 Jobs 自有 CLI、其外置测试／运行器及就近文档；无 backend／其他端／部署／全局导航改动，无提交推送。

<a id="🔚" href="#前言">返回首页</a>
