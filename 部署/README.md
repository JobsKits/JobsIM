# <span id="前言">JobsIM 部署入口</span>

![JobsIM](../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、平台入口](#jobs-toc-heading-1)
- [二、边界与要求](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、平台入口 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 系统 | 脚本 | 用途 |
| --- | --- | --- |
| MacOS | [部署/MacOS](./MacOS/README.md) | 本机独立运行、停止与备份验证 |
| Linux | [部署/Linux](./Linux/README.md) | 自托管运行、停止与备份 |
| WindowsOS | [部署/WindowsOS](./WindowsOS/README.md) | PowerShell 原生入口 |

具体入口按平台说明检查既有 Go、PostgreSQL、Node、NATS 与证书。NATS JetStream 是第一阶段自托管依赖，不是推迟到后续标段的外部服务 SDK；缺工具时预检明确失败，不自动安装或降级关闭同步。运维数据、密钥、业务库、独立恢复日志库和 JetStream 均在工程外，不固定开发机目录；日志与停止操作只针对当前项目创建的进程。

MacOS 持久状态与已安装工具使用 `~/Library/Application Support/JobsIM`；本轮联调备份另置 `~/Documents/JobsIMBackups`。平台入口可通过 `JOBS_IM_BACKUP_DIR`／`JOBS_IM_RECOVERY_DIR` 指定独立持久目录；既有部署的归档与恢复密钥不自动搬移。`~/Library/Caches` 只放可重建的编译缓存、临时下载；不得存放运行数据库、实例身份、CA／私钥或唯一备份。业务与独立日志库仍须物理隔离；同机分目录备份不等于异地灾备。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、边界与要求 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

本地开发先采用同机拆进程／域的自托管结构；业务数据库与恢复日志数据库使用独立实例，备份恢复不得把独立日志一起回滚。备份应覆盖业务库、媒体及部署标识，恢复只在隔离实例演练并验证旧会话／旧游标被拒绝。

当前脚本从 Go 源码发现九个内部角色，另有同源网关；governance 复用 identity 数据库，含 sync 的域库共八个，不表示启动八套 PostgreSQL 集群。NATS 只监听本机 loopback，使用本实例私有 token／配置及持久 JetStream 目录；秘密不放进进程参数或日志。启停同时核对 PID、启动身份、工具和配置摘要，陌生进程不会被接管。脚本停止顺序为应用、本人 broker、本人数据库，保留全部运行数据。

平台脚本的加密备份范围增加已停止 broker 的数据及清单；它与 `jobsimops` 的业务快照／独立 journal 取证归档分别描述，不能把一份 PG dump 当成包含 broker 的完整恢复。三平台静态检查和 Mac 独立 NATS 认证、重启持久化、配置漂移及进程保护已验；八域数据库＋broker 的部署入口全链恢复、Linux／Windows 原生执行仍未验，见[限定报告](../项目相关报告/测试/部署/NATS/README.md)。后端独立同步恢复证据只证明其报告列明的单业务库测试环境。

Web 构建由网关同源提供，普通 API 请求优先；未部署后端时可立即进入明确标识的本地演示。演示操作不补发，素材随包提供，离线预览见[Web 说明](../代码/前端/Web/README.md)。

本期外部推送、RTC、地图、支付及强保护能力保持关闭。后续标段按[需求](../需求/README.md)接入；当前测试结果及已知缺口见[实现进度](../项目相关报告/实现进度/README.md)。

<a id="🔚" href="#前言">回到首页</a>
