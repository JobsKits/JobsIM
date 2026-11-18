# <span id="前言">JobsIM 部署入口</span>

![JobsIM](../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、平台入口](#systems)
- [二、边界与要求](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## 一、<span id="systems">平台入口</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 系统 | 平台说明 | 成对入口 |
| --- | --- | --- |
| MacOS | [MacOS](./MacOS/README.md) | 本层双击部署／反部署，唯一实现分置Deployment／Undeployment |
| Windows | [Windows](./Windows/README.md) | 原生PowerShell与成对CMD快捷入口 |
| Ubuntu | [Ubuntu](./Ubuntu/README.md) | 独立Bash包及成对desktop快捷入口 |
| Debian | [Debian](./Debian/README.md) | 独立Bash包及成对desktop快捷入口 |
| RockyLinux | [RockyLinux](./RockyLinux/README.md) | 独立Bash包及成对desktop快捷入口 |
| AlibabaCloudLinux | [AlibabaCloudLinux](./AlibabaCloudLinux/README.md) | 独立Bash包及成对desktop快捷入口 |

最快运行：Mac打开[双击部署快捷入口](./MacOS/【MacOS】双击部署（快捷入口）.command)，阅读自述后回车，按方向键选择本地或服务器模式。本地可选仅本机／临时穿透（按平台当前选项），服务器需要真实地址及匹配证书；完成后从打开网页菜单进入，不要求用户手拼URL。正常运行终端需保持打开。Windows与Linux见各自平台说明、系统工具条件和桌面信任条件。

**部署与反部署成对使用。** 停止当前会话属于运维；反部署打开[Mac反部署快捷入口](./MacOS/【MacOS】双击反部署（快捷入口）.command)或对应平台入口，先选仅停保留／进入清理，再选默认备份后清理或明确清零，并独立复核范围。`--yes`／`-Yes`不授权跳过清理确认，空选／取消／EOF保持原件。停止、备份仍是原生脚本子动作，不能代替反部署。

各平台的核心生命周期自包含，公共[官方版本解析](./公共组件/依赖版本/README.md)和[离线二维码](./公共组件/地址二维码/README.md)按独立辅助职责共享，工具引导锁为[deploy-tools.lock.tsv](./deploy-tools.lock.tsv)。Go后端源码编译为各系统原生产物；Node只用于Web构建，不作为网关静态页的常驻服务，不额外要求Docker／Lima／WSL。

具体入口按平台说明检查既有Go、PostgreSQL、Node、NATS与证书，健康工具优先复用；具备可验证原生包时在工程外受管理位置准备，不升级系统全局环境。Windows缺少健康PostgreSQL 18工具时明确提示先决条件，不下载来源未核验的数据库包。NATS JetStream是第一阶段自托管依赖，不是推迟到后续标段的外部服务SDK；准备失败明确停止相关步骤，不降级关闭同步。运维数据、密钥、业务库、独立恢复日志库和 JetStream 均在工程外，不固定开发机目录；日志与停止操作只针对当前项目创建的进程。

MacOS 持久状态与已安装工具使用 `~/Library/Application Support/JobsIM`；本轮联调备份另置 `~/Documents/JobsIMBackups`。平台入口可通过 `JOBS_IM_BACKUP_DIR`／`JOBS_IM_RECOVERY_DIR` 指定独立持久目录；既有部署的归档与恢复密钥不自动搬移。`~/Library/Caches` 只放可重建的编译缓存、临时下载；不得存放运行数据库、实例身份、CA／私钥或唯一备份。业务与独立日志库仍须物理隔离；同机分目录备份不等于异地灾备。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、边界与要求 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

本地开发先采用同机拆进程／域的自托管结构；业务数据库与恢复日志数据库使用独立实例，备份恢复不得把独立日志一起回滚。备份应覆盖业务库、媒体及部署标识，恢复只在隔离实例演练并验证旧会话／旧游标被拒绝。

当前脚本从 Go 源码发现九个内部角色，另有同源网关；governance 复用 identity 数据库，含 sync 的域库共八个，不表示启动八套 PostgreSQL 集群。NATS 只监听本机 loopback，使用本实例私有 token／配置及持久 JetStream 目录；秘密不放进进程参数或日志。启停同时核对 PID、启动身份、工具和配置摘要，陌生进程不会被接管。脚本停止顺序为应用、本人 broker、本人数据库，保留全部运行数据。

平台脚本的加密备份包含已停止 broker 的数据及清单；它与 `jobsimops` 的业务快照／独立 journal 取证归档分别描述，不能把一份 PG dump 当成包含 broker 的完整恢复。当前脚本、Mac受控真实实例、Linux隔离夹具、Windows主机函数与待目标机项见[部署符合性与验证](../项目相关报告/测试/部署/20261118-成对部署反部署/README.md)，按实际输入与环境分别列证。Linux／Windows原生完整流程、六系统聚合恢复与真实手机扫码不得由语法或其他平台结果外推；[既有NATS限定证据](../项目相关报告/测试/部署/NATS/README.md)仍只证明自身冻结输入。后端独立同步恢复证据只证明其报告列明的单业务库测试环境。

Web同时提供桌面与手机H5，由网关同源提供，普通 API 请求优先；未部署后端时可立即进入明确标识的本地演示。演示操作不补发，素材随包提供，离线预览见[Web 说明](../代码/前端/Web/README.md)。

反部署依据created／reused原点、设备／inode或等价平台身份及当前快照逐项撤回，仅清理有证明的本实例新建资源。缺原点、未知对象、链接／junction、并发改变或验证失败时保留并报告未完成。独立journal、匹配实例身份／凭据与TLS材料、既有备份及恢复钥受保护，部分完成明确报告，不能宣称整机恢复；旧无清单实例也不追认删除资格。

本期外部推送、RTC、地图、支付及强保护能力保持关闭。后续标段按[需求](../需求/README.md)接入；当前测试结果及已知缺口见[实现进度](../项目相关报告/实现进度/README.md)。

<a id="🔚" href="#前言">回到首页</a>
