# <span id="前言">CLI 权威同步候选：Linux／Windows 交叉编译</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、结论与范围](#jobs-toc-heading-1)
- [二、实际构建与产物](#jobs-toc-heading-2)
- [三、资源与隔离](#jobs-toc-heading-3)
- [四、保留边界](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、结论与范围</span><!-- jobs-toc:heading-end -->

2026-11-16，现权威同步 CLI 候选在 MacOS arm64 宿主上完成 **Linux amd64 与 Windows amd64 的 Go 交叉编译，两者退出0**。CGO关闭，分别产出 ELF 与 PE32+；只读取文件格式和 Go build 元数据，**没有执行目标程序、目标测试或安全凭据库，不能标为 Windows／Linux 原生通过**。

19 项 Go／模块输入均与已通过 [MacOS 45 项本地回归](../../20261116-权威同步编译回归/20261116-113743-192d48/README.md)和 [固定57真实 HTTPS 10 项](../../20261116-权威同步HTTPS/20261116-120401-3cbd4e/README.md)的候选输入完全一致。没有 Unix API 等编译错误，不需要源码修复，也没有重跑不受影响的 MacOS／HTTPS。旧跨编译报告不追认为本轮结果；旧 binary／原报告均保留。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、实际构建与产物</span><!-- jobs-toc:heading-end -->

| 目标 | 耗时／退出码 | 文件格式 | SHA256 |
| --- | --- | --- | --- |
| Linux amd64 | 77.42s／0 | ELF 64-bit x86-64，静态链接 | `3709d4560f052302ad1279bc2ef7289a1389380f9b4171ca5d7692af7c3298f9` |
| Windows amd64 | 59.27s／0 | PE32+ console x86-64 | `45ad1e495b7f2cb13599dd3e96b8c6d97baf7e797900ff9223b39e7cca700d88` |

[Linux JSON](./linux.json)／[原日志](./linux.log)／[buildinfo](./linux-buildinfo.txt)、[Windows JSON](./windows.json)／[原日志](./windows.log)／[buildinfo](./windows-buildinfo.txt)分别保存退出状态、原命令、SHA和格式。buildinfo中的 GOOS／GOARCH／CGO_ENABLED 与各目标一致，文件 SHA 再读回一致。[结果汇总](./结果汇总.json)和 [19 项源 SHA／快照](./源码SHA256.json)绑定此轮，文档变化另封 [SHA](./文档SHA256.json)。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、资源与隔离</span><!-- jobs-toc:heading-end -->

使用既有 Go1.27.4，Linux完成后才编Windows。每步 `GOMAXPROCS=2`、`-p=1`、`CGO_ENABLED=0`、`GOTOOLCHAIN=local`、`GOPROXY=off`，复用同任务外置cache／已有模块源码，不安装依赖、不改变全局环境。

```text
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -p=1 -o <新Products/jobsim-linux-amd64> ./cmd/jobsim
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -p=1 -o <新Products/jobsim-windows-amd64.exe> ./cmd/jobsim
```

每步外层watchdog限180秒，4GiB磁盘硬门禁；最低观测空闲9.95GiB，两步均正常完成，没有把超时当通过。产物在 Application Support/JobsIM/Build/CLI 同任务的独立 `Products/Cross-20261116-123709-093a23/`，实际路径见 [配置](./配置.json)，不覆盖 MacOS binary。不运行API／PG、不启停服务、不调用vault、不修改SDK或第三方源码。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、保留边界</span><!-- jobs-toc:heading-end -->

这里只证明目标代码可编译链接；目标系统的文件锁／ACL／SQLite耐久性／路径／终端／系统安全库及网络行为未实跑，跨编译不替代这些原生证据。MacOS process-only真实路径也不证明OS安全库正向；qualifiedACK／after_receipt及完整第一阶段仍未由本轮验收。无提交、推送或全局导航改动。

<a id="🔚" href="#前言">返回首页</a>
