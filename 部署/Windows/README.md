# <span id="前言">JobsIM Windows 原生部署</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、成对入口与操作](#entries)
- [二、原生数据库与工具](#database)
- [三、两种真实运行模式](#modes)
- [四、访问、扫码与生命周期](#access)
- [五、反部署与原点保护](#inverse)
- [六、备份与运维子动作](#backup)
- [七、验证边界](#verification)

</details>
<!-- jobs-toc:end -->

---

## 一、<span id="entries">成对入口与操作</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本平台使用 [PowerShell 7.4+](https://learn.microsoft.com/powershell/) 原生入口，提供“本地部署（默认仅本机，临时穿透另选另确认）”和“服务器 HTTPS”两条路线。原生 [Go](https://go.dev/)、[PostgreSQL](https://www.postgresql.org/) 与 [NATS JetStream](https://docs.nats.io/nats-concepts/jetstream) 为当前运行路线；不要求 WSL、Docker 或 TiDB。源码、SDK、实例数据、备份与恢复钥分别管理，不安装系统服务或修改防火墙／全局代理／系统 CA。

| 入口 | 用途 |
| --- | --- |
| [双击部署](./【Windows】双击部署（快捷入口）.cmd) | 唯一发现 `Deployment/` 的同名 PowerShell 实施包，默认进入交互部署 |
| [双击反部署](./【Windows】双击反部署（快捷入口）.cmd) | 唯一发现 `Undeployment/` 的配对实现，默认先选“仅停止保留”或“进入清理” |
| [部署实现](./Deployment/【Windows】部署.ps1/【Windows】部署.ps1) | 自包含工具准备、模式、数据库、TLS、构建、进程与原点 |
| [反部署实现](./Undeployment/【Windows】反部署.ps1/【Windows】反部署.ps1) | 自包含同一生命周期内核，默认反部署菜单；不借停止备份冒充逆操作 |
| [代码地图](./代码地图/README.md) | 资源、动作与函数定位 |
| [隔离测试](../../测试/部署/Windows/README.md) | 宿主执行与 Windows 原生未验边界 |

从本目录运行，参数与退出码由快捷入口透传；不依赖调用工作目录：

```powershell
& './Deployment/【Windows】部署.ps1/【Windows】部署.ps1' -Check
& './Deployment/【Windows】部署.ps1/【Windows】部署.ps1' -Plan -Mode server
& './Deployment/【Windows】部署.ps1/【Windows】部署.ps1'
& './Undeployment/【Windows】反部署.ps1/【Windows】反部署.ps1'
```

`check / plan / status / help` 为只读结构与计划检查，不联网、不启动工具、不写缓存／密钥／锁。真实动作只在 Windows 执行；`-Mode` 只用于计划，不能绕过正常运行的模式菜单。`-State` 指定明确的外置实例，默认在当前用户 LocalApplicationData 的 `JobsIM/Instances/<项目路径摘要>/local` 或 `server`。两模式默认独立目录；同时运行须另配不冲突端口。

## 二、<span id="database">原生数据库与工具</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

默认由 fzf 选择“本实例原生双 PG 自动准备”：发现 `JOBS_IM_PG_BIN`、PATH 的 initdb、Program Files 的 PostgreSQL 安装，或拖入已安装的 PG18 `bin` 目录。脚本核验原生架构与版本后，在本实例建立物理独立的业务／journal 集群，生成随机管理员和八域非超级用户、数据库及必要的 `pg_control_system()` 查询资格。无需手填八个 URL；治理独占 schema 沿用 identity 库。连接资料仅进入实例私有 profile；角色密码通过 psql stdin 输入，管理员证明通过私有环境与 pwfile 使用，不进参数／报告。

Windows PG 原生成品没有可验证官方摘要时，不下载未经核验的 EDB 包；**缺健康 PG18 工具会明确阻断，不能称为干净系统一键部署。** PG 官方当前稳定目标与项目现支持的 18.x 分开展示，不自动跨 major 升级现有数据库。共享 PostgreSQL 安装不纳入本项目删除范围。

高级路线可明确选择已有外部 PG。仅该路线要求维护者在当前进程注入八域 `JOBS_IM_<ROLE>_DATABASE_URL`，role 为 identity、directory、conversation、message、media、control、sync、journal；业务与 journal 必须是不同物理集群。连接串不写命令行／源码／共享记录，不自动建库、提权、DROP 或接管外部服务。

部署模式确定后，公共[依赖版本组件](../公共组件/依赖版本/README.md)核对 Go、fzf、Node LTS、NATS 及可选 cloudflared 的官方当前稳定目标。只有实际版本达标、原生 PE 架构相符的健康候选才复用；否则按官方 SHA 下载到当前用户 `JobsIM/ManagedTools/<项目摘要>/` 的版本目录，不覆盖显式或系统程序。Go／fzf 引导使用[锁清单](../deploy-tools.lock.tsv)，选定模式后仍重新查当前稳定元数据。网络类故障退避重试，坏来源／摘要／架构立即停止；已有依赖不重装，旧版本及无收据缓存不盲删。

OpenSSL 必须支持 CMS AES256-GCM；还需系统 tar／curl 与 PG 的 pg_dump／pg_restore／initdb／postgres／pg_ctl／psql。工程 npm 依赖按源码锁预先准备，业务库不自动升级；构建在选定 Node 的 npm CLI 上执行。Go／Web 可运行产物写入实例 `Bin/`／`WebDist/`，不覆盖源码 Dist。下载按Content-Length／实际写入字节预留，ZIP按实际展开长度检查；备份查询各域实际库大小，按源文件／dump大小为tar、CMS、解密副本计算额外空间。各阶段保留4GiB，负预算／溢出／低空间阻断；这条宿主测试保留线不等于Windows最低容量承诺。实际最低内存／CPU／并发容量尚未测定，不能由原生路线名称推导资源达标。

## 三、<span id="modes">两种真实运行模式</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本地模式默认仅本机，先核严格 HTTPS 健康，不准备或启动 cloudflared。明确选临时公网并单独确认后，才核其工具并启动本次 Cloudflare Quick Tunnel。隧道使用空会话配置、清空本子进程 TUNNEL 变量，明确指定本机 CA 和 `localhost` SAN 校验；不使用 `no-tls-verify`。菜单独立确认公开临时访问，只允许合成资料；任何得到 URL 的人可到达登录入口，JobsIM 自身账号与 CSRF 认证仍必须完成。

取得本次真实公网 HTTPS URL 后，优雅重启本次 Go 并把 `JOBS_IM_PUBLIC_URL` 固定为该 origin，保留部署 ID、数据和 NATS；公网 bootstrap 必须再次通过，防止只打印网址却令 Web 同源认证不可用。临时隧道最长等待45秒；准备失败、无地址或晚退均撤换旧公网二维码／描述，精确停止本次隧道，将本次Go绑定回严格loopback origin并重新验健康，保留本机与NATS和数据；菜单可重试、诊断或停止，不复用外部隧道，不降TLS。已是健康loopback时不重启其Go。公网可用性、不同运营商及原生 App 全流程仍需实际验证。

服务器模式使用独立 state，必须提供 `JOBS_IM_SERVER_URL`、`JOBS_IM_SERVER_CA_FILE`、`JOBS_IM_SERVER_CERT_FILE`、`JOBS_IM_SERVER_KEY_FILE`。URL 为 HTTPS origin，端口须与监听一致，不含用户信息／路径／查询／片段。脚本校验 CA 链、服务端用途、DNS SAN、有效期及公私钥匹配，再把原件逐字复制到实例；原外部证书／私钥不更改，已有不匹配材料不覆盖／换根。默认监听 `0.0.0.0`，可用 `JOBS_IM_SERVER_BIND` 指定明确 IP；仅健康探针将域名解析到回环，不改系统 hosts。域名解析、主机采购、防火墙与系统服务托管由部署者另行落实，不生成假公网验收。

| 配置 | 默认／边界 |
| --- | --- |
| `JOBS_IM_HTTPS_PORT` | 9443；服务器 URL 端口须匹配 |
| `JOBS_IM_INTERNAL_PORT` | 18101；按 Go 源码动态发现9个角色，使用 base～base+8 |
| `JOBS_IM_BUSINESS_PG_PORT`／`JOBS_IM_JOURNAL_PG_PORT` | 托管路线17432／17433，两个物理集群，不接管占用端口 |
| `JOBS_IM_NATS_PORT` | 4222，仅回环；不能与 HTTPS／内部角色／PG 重复 |
| `JOBS_IM_NATS_URL` | 须精确匹配本次回环端口，不含秘密 |
| `JOBS_IM_NATS_MAX_FILE_BYTES` | 至少512MiB，默认512MiB |
| `JOBS_IM_NATS_MAX_MEMORY_BYTES` | 至少32MiB，默认32MiB；不是宿主完整资源限额 |
| 工具覆盖 | `JOBS_IM_GO`、`JOBS_IM_FZF_BIN`、`JOBS_IM_NATS_BIN`、`JOBS_IM_CLOUDFLARED_BIN`、`JOBS_IM_PG_BIN`、`JOBS_IM_OPENSSL`；只作用于当前任务 |

NATS 秘密仅在实例 `Secrets/`，只通过配置路径启动；JetStream 位于 `Application/JetStream/`，不放 Caches。启动核工具／配置摘要和认证 PING／JetStream，关停先应用、后 broker、再本实例 PG。共享 broker／系统服务和未知端口不接管。

## 四、<span id="access">访问、扫码与生命周期</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

启动后显示同一真实 origin，Web、原生与 CLI 的 API 位于其 `/api`；没有独立后台网页，不虚构后台 URL。fzf 可打开用户网页或本地扫码页，Esc仅收起菜单，回车／O再打开；服务保持运行。离线二维码由[公共组件](../公共组件/地址二维码/README.md)本地生成 HTML／SVG／PNG，只包含当前非秘密 URL。控制台读取实际宽高并核VT能力，尺寸足够时显示黑白背景方格；窗口过窄／过矮、NO_COLOR或不能固定黑白时，请求打开本轮本地PNG。无交互桌面时明确保留PNG与完整URL，不声称已显示可扫码图，不修改全局配色。失败保留 URL，正常退出使旧页面失效。回环二维码仅本机可用，生成成功不等于手机可达；Windows图形预览实际效果需目标机验证。

本机自签 CA 只用于对应回环连接档案，继续校验主机名、SAN、有效期和部署身份，不安装系统全局 CA。Quick 公网描述使用 `tlsTrust=system`，由系统信任的边缘证书保护，实例 CA 指纹另标为 `originCaSha256`，不能拿回环 CA 当作公网证书。服务器直连描述保留 `caSha256` 并标明实例 CA 信任；新公网描述的端侧导入尚未验证，不能据此宣称五端已适配。

本次原生程序在独立进程组及 Job Object 中启动，收据绑定 PID、启动时刻、程序路径／SHA、PG数据目录或 NATS配置SHA。Ctrl+C／正常关闭只回收本次进程，保留数据；父宿主消失时 Job 兜底回收。再次启动先精确停止并等待同项目旧宿主封存，未知 PID、旧无进程组证明、工具漂移或占用端口均保留并拒绝接管，不按名称强杀。控制台、真实关窗、ACL及完整进程树行为须在 Windows 目标机验收。

## 五、<span id="inverse">反部署与原点保护</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

反部署先由 fzf 选择“仅停止并保留”（默认）或“进入清理”。清理再选“数据库／文件数据备份”（默认，验证后清理本项目源）或“直接清零”。最后确认默认取消，选择执行后再以独立空行回车复核，任意非空输入或EOF取消；不要求手敲 YES 或实例短语。`-Yes` 只跳过普通介绍确认，不能跳过数据策略和最后键盘复核；`-NoBackup` 仅能用于 stop，不能把反部署扩大为清零。

首次新／空 state 在任何资源写入前记录 `origin.json`，区分原不存在和原有空目录。会话结束、应用／NATS／托管 PG 均停止后封存逐文件 SHA、Windows对象ID／所有者SID／链接数、目录及使用的共享工具路径。反部署持实例外独占锁，仅按清单逐个普通文件／空目录撤回：程序、WebDist、媒体、NATS持久副本、专属业务 PG 和实例生成资料；原有空目录保留。共享 SDK／工具、源码、外部证书原件均不删除。新建但由本地／服务器共同复用的机器工具缓存保留，来源收据记录其范围。

旧非空实例没有原点仍可按原 PID 收据停止／备份，但不能“补登记为己有”后删除。未知顶层项、后改文件、链接／junction或其祖先、不同用户 SID、未封存状态、活动反部署、并发写入、备份／解密钥变化均阻断，保留原源和证据。删除前逐步比对剩余快照，已完成部分写入 `undo.json`，重试沿原策略／原备份核对，不悄悄重新备份后清零。

**独立恢复 journal、`Application/instance.json` 中部署身份及32字节实例secret、TLS／Secrets／owner与原点记录、既有备份／离线集及匹配恢复钥默认保留，普通清零也不删除。** `Application` 父目录只为容纳身份文件而保留，清单内其它业务／媒体仍实际清理。 托管 journal 在 `Postgres/journal*`，其余清单内业务资源可撤回；外部路线的全部 PG 均无本项目删除权。存在这些保留项时审计明确“部分反部署”并返回退出码2，不能声称数据已全部清零或环境完全回原点。需销毁保留对象须另行授权和专项流程；保留 journal 的已反部署 state 不直接重新初始化，重新部署使用新 state并按恢复合同处理旧恢复域。

## 六、<span id="backup">备份与运维子动作</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

`stop`／`backup` 是保留型运维动作，不等同反部署。默认 stop 停止后备份，`stop -NoBackup` 仅停止保留。八域 custom dump 经 pg_restore 清单检查，本项目文件、TLS、NATS持久数据／秘密、必要连接配置及原点一起 CMS AES256-GCM 加密到 state 外。托管 PG 只为 dump 启动本次已核验集群，归档物理文件前完成停止／检查点；非 PG 源SHA不得改变，匹配钥和配置文件不豁免竞争检查。解密载荷须与原载荷SHA逐字一致，并明确含broker清单、8域非空dump／list、配置和实例身份，拒绝重复／越界条目后再登记归档SHA；备份失败保留源和现场，不能继续清理。

恢复私钥默认放在 state 同级 `RecoveryKeys/<实例名>`，加密归档放 `Backups/<实例名>`，可分别用 `JOBS_IM_RECOVERY_DIR`／`JOBS_IM_BACKUP_DIR` 指定；两者不得位于 state 内或互相包含。只允许当前用户的 ACL，设置失败即阻断。外部 PG 维护者须确认本实例各域无其它写者；本入口不代其停服务或备份全服务器权限。加密与解密SHA通过不等同跨库／journal／JetStream隔离恢复演练，未提供覆盖当前journal的自动恢复向导。

## 七、<span id="verification">验证边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[Windows宿主隔离证据](../../项目相关报告/测试/部署/20261118-成对部署反部署/WindowsOS/summary.json)按实际输入记录：PowerShell解析、两实现一致性、C#桥编译、原点逐文件逆操作、取消、未知／后改／链接、独占锁、失败／中断续行、journal及恢复材料保留、Quick严格origin参数、真实OpenSSL加解密和服务器SAN／密钥校验。宿主夹具明确替换SID／ACL及进程探针，PG dump在相应用例中为注入，不伪装真实Windows数据库。

Windows原生下载／架构与版本探针、ACL、双PG初始化／SQL、控制台／Job／关窗、真实备份恢复、双击菜单、Quick公网及手机读写均待目标环境验收。没有可控Windows目标时，不对用户实例执行部署／反部署来替代验证。

清理清单同时绑定 Windows 文件对象 ID、所有者 SID、链接数与内容 SHA；state／目录同字节换壳、归属变化和多硬链接均阻断。旧清单缺对象身份时保留现场，不自动补领删除权。当前宿主测试使用实际 macOS inode／uid／链接数替身及原生桥编译；Windows FILE_ID_INFO／ACL API 的目标机执行仍待验。

<a id="🔚" href="#前言">返回前言</a>
