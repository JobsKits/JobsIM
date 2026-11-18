# <span id="前言">JobsIM MacOS 原生部署与反部署</span>

![JobsIM](../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前交付与入口](#entry)
- [二、环境与外置资源](#environment)
- [三、首次连接与生命周期](#connection)
- [四、加密备份与恢复边界](#backup)
- [五、自托管 NATS JetStream](#nats)

</details>
<!-- jobs-toc:end -->

## 一、<span id="entry">当前交付与入口</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

此入口在普通用户下原生构建[**Go**](https://go.dev/)与WebDist，创建两个物理独立的 [**PostgreSQL**](https://www.postgresql.org/)18 集群：业务集群内按 identity、directory、conversation、message、media、control、sync 分七库七用户；journal 位于第二集群。网关同源提供 HTTPS 与 WebDist。当前是可运行开发切片，不能作为生产容量、完整管理后台、强保护或 RPO0 验收。

- [双击部署（快捷入口）](./【MacOS】双击部署（快捷入口）.command)：自述确认后经fzf选择本地部署或服务器模式；关闭运行终端将结束本轮服务和两集群。
- [双击反部署（快捷入口）](./【MacOS】双击反部署（快捷入口）.command)：先选择仅停止保留或进入清理，再选择默认备份／显式清零，最后独立回车确认。
- [Deployment部署源码](./Deployment/【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command)与[Undeployment反部署源码](./Undeployment/【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command)各自内嵌生命周期，不依赖外置状态函数库；动作包括 `check / plan / status / run / undeploy / backup / stop`。
- [代码地图](./代码地图/README.md)、[测试与实际记录](../../测试/部署/MacOS/README.md)、[后端契约](../../代码/后端/Go/接口契约/README.md)。

```sh
./Deployment/【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command --check
./【MacOS】双击部署（快捷入口）.command
./【MacOS】双击反部署（快捷入口）.command
```

`check / plan / status` 不联网、不运行工具探针，不创建实例、生成密钥、安装工具或启停资源。`--mode`只供只读计划，正常部署由fzf选择。`--yes`仅跳过普通开场，不能跳部署模式、公开范围或最终清理确认；`--state`指定源码外私密目录，路径含空格可用，目录及祖先不能为链接，不接管陌生非空目录。`stop --no-backup`是仅停保留的运维动作，不代表清理。

## 二、<span id="environment">环境与外置资源</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

基础工具包括nc、[**Go**](https://go.dev/)1.27、[**PostgreSQL**](https://www.postgresql.org/)18完整bin、[**Node.js**](https://nodejs.org/)／npm、支持CMS AES256-GCM的[**OpenSSL**](https://openssl.org/)、curl与lsof。正常部署查询官方元数据，核对Go、[**fzf**](https://github.com/junegunn/fzf)、Node、[**NATS**](https://nats.io/)及数据库稳定目标，明确选择临时穿透后另核[**cloudflared**](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/)；版本和所需功能都通过才复用。选择器缺失可按[引导锁](../deploy-tools.lock.tsv)校验准备，最终采用仍重新查目标。业务依赖仅用外置清单副本报告更新，超时明确标检查未完成并按原锁构建，不批量升级源码依赖。没有可验证原生数据库包时要求合格候选，不自动跨主版本迁移既有集群。

工具从用户机器工具区、显式路径与PATH发现，健康共享工具保留；必要下载校验摘要后进入本轮私有会话，不调用Homebrew／sudo或更改系统安装、证书信任及Go全局配置。可指定 `JOBS_IM_GO`、`JOBS_IM_PG_BIN`、`JOBS_IM_OPENSSL`、`JOBS_IM_FZF_BIN`、`JOBS_IM_CLOUDFLARED_BIN`；显式候选不合格则保留报错，不静默换用。Web依赖须已准备，缺失时构建失败并回收本轮进程，不自动 `npm install`。诊断目录由运行终端显示，取消可能保留本轮诊断与已核验引导缓存。

默认 state 是用户 Application Support 下由实际项目路径哈希区分的独立实例目录，不进入源码。`Application/` 存部署身份、应用秘密和媒体；`MainDatabase/`、`JournalDatabase/` 存物理数据库；`TLS/` 存实例 CA 与服务端密钥；`Secrets/` 存操作员／域数据库密码；`Processes/`、`owner`、`receipt.txt` 为归属与进程收据；`Logs/` 为本实例日志。文件使用私密 umask，实例目录700。不得把这些文件提交到 Git 或回显。

| 可配置变量 | 默认／含义 |
| --- | --- |
| `JOBS_IM_DEPLOY_STATE` | 外置实例目录，命令行 `--state` 优先 |
| `JOBS_IM_HTTPS_PORT` | 9443，网关只绑定127.0.0.1 |
| `JOBS_IM_PG_PORT` / `JOBS_IM_JOURNAL_PORT` | 16543 / 16544，数据库只绑定127.0.0.1 |
| `JOBS_IM_INTERNAL_PORT` | 18101，按Go源码动态发现角色，当前连续九个内部端口 |
| `JOBS_IM_PUBLIC_HOST` | 本地证书SAN主机；服务器模式从显式HTTPS origin取得主机，仍只监听回环 |
| `JOBS_IM_PUBLIC_URL` | 服务器模式必须明确提供HTTPS origin；本地Quick取得真实网址后绑定实际BFF Origin |
| `JOBS_IM_TLS_CERT` / `JOBS_IM_TLS_KEY` / `JOBS_IM_TLS_CA` | 服务器模式的真实SAN证书、匹配私钥和信任链，不覆盖不匹配的旧TLS |
| `JOBS_IM_BACKUP_DIR` | `~/Documents/JobsIMBackups/实例名`，必须位于 state 之外 |
| `JOBS_IM_RECOVERY_DIR` | state 同级 RecoveryKeys/实例名，必须与数据、加密归档分开保管 |

端口冲突会拒绝运行，不杀占用者。两个 [**PostgreSQL**](https://www.postgresql.org/) 集群仅监听回环 TCP，所有管理、备份和应用连接均显式使用 `127.0.0.1`；不创建 Unix socket，因此实例目录包含中文、空格或较长路径时不受 MacOS Unix socket 路径上限影响。既有 `Sockets/` 目录保留，不迁移数据库或修改已有密码。

同项目重开前先加密备份，再在相同数据上启动，保留部署身份；不会重置密码、覆盖已有 CA 或静默接管未知数据库。源码路径从脚本自身解析，无启动工作目录要求。移动项目会改变默认实例定位；需要继续旧实例时显式指定原 `--state`，核对归属后受控迁移，不能手改归属收据冒充接管。

## 三、<span id="connection">首次连接与生命周期</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

启动输出用户URL、deploymentId、信任信息及不含口令/令牌的 `connection.json`；管理网页尚未交付，不编造后台URL。本地模式默认仅本机，不准备cloudflared；可选择临时穿透并独立回车确认公开范围，严格核本机HTTPS，再用本实例CA校验Quick origin；取得真实网址后重启同一本轮Go绑定实际PublicURL，PG、NATS和实例身份不重新初始化。公网健康未通过不报告公网成功；穿透工具、连接失败或晚退时撤销旧公网URL/二维码，严格把同一本轮Go绑定回loopback Origin，保持本机服务。菜单提供重试（重新确认公开）、诊断和停止，不降TLS或重建数据。Quick描述使用系统信任，私有origin指纹另列 `originCaSha256`，不冒充公网证书根。服务器模式不建穿透，需要明确URL及SAN证书/私钥/CA；解析、反向代理、防火墙和公网可达性由部署方配置，本机健康不代替生产验收。

[离线二维码编码器](../公共组件/地址二维码/README.md)在本机生成HTML／SVG／PNG与终端显示，不调用外部二维码服务。终端实际尺寸从TTY读取；足够时用黑白背景方格显示完整二维码，先留屏供查看，按回车再打开网页菜单。窗口过窄／过矮或`NO_COLOR`／纯文本终端无法保证黑白极性时，自动用本机预览打开本轮自己生成的PNG；URL和HTML始终保留，不修改全局配色。后台fzf菜单可反复打开用户地址和扫码页，Esc收起菜单、回车重开；浏览器或二维码失败保留完整URL，业务继续。二维码在地址失效与退出时过期，回环地址明确仅本机，不能用手机访问127.0.0.1。非TTY不等待输入、不自动打开图片。

部署／反部署选择与运行中的网页菜单均使用终端备用屏全屏绘制；调整窗口尺寸时只重绘菜单，退出选择后归还原终端屏幕。Esc只收起网页菜单，回车重开；Ctrl+C由菜单明确传回中断标记，再沿原控制器收尾本轮资源，不能把选择器的普通取消误当停止。既有工具先以只读过滤校验全屏和按键动作能力，不升级机器上的fzf。菜单缩放、选择和中断的限定证据见[专项报告](../../项目相关报告/测试/部署/MacOS/20261118-终端菜单缩放/README.md)。

已打开的部署控制器不会重新载入修改后的脚本；Esc收起再回车仍使用该会话内的原函数。源码修正从下一次正常启动生效，本操作不会自动重启当前服务。旧会话画面残留时可先Esc收起，再使用Terminal的⌘K清屏，回车重开；这仅清理当前显示，继续缩放仍可能出现旧表现。暂停菜单并非Shell提示符，不要向其中输入`clear`当作命令执行。

由可信部署终端取得并核对 CA／指纹，原生端只在对应连接档案内信任该 CA，仍校验 SAN、主机名与有效期。Web 必须按目标系统／浏览器受控信任本地 CA，或改用公信 HTTPS；未信任即阻断，不能绕过证书警告。脚本不修改系统全局信任。自签 CA 私钥属于本实例维护者，CA 公钥可交给客户端核对；反部署后需由维护者按受控记录手动清理曾安装的浏览器／系统信任，不自动删除机器全局证书。

TLS 首次生成先写实例内私有 `.TLS-pending.*` 目录，补齐 CA 的签证用途／SKI 与服务端 SAN、serverAuth、KeyUsage、SKI／AKI，严格验证通过后才提交。失败保留 pending 与私有日志，不把半成品放入生效的 `TLS/`；已有有效材料逐字节复用，残缺或未知材料保留并阻断。旧证书未通过严格验证时不自动换根：先备份并核对 CA 指纹、有效期与扩展，CA 合格才可按原 CA 受控重签服务端证书；CA 本身不合格时需要另行安排客户端信任迁移，不能仅重签服务端便声称修复。

运行终端保持前台；Ctrl+C／TERM／退出会回收本轮匹配的服务与两集群。停止时核对 PID、启动时间、命令路径、项目收据和物理集群标识，拒绝强杀陌生进程。并发事务锁不会自动删除；遇到锁应先核对持锁进程，不能据此删除数据。事务失败时保留日志、密钥和数据供诊断。

## 四、<span id="backup">加密备份与恢复边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

`backup` 与默认 `stop` 都会暂停本实例，完成后保持停止。停止后按当前源码发现的八个物理域库逐个 `pg_dump --format=custom`，检查每份 `pg_restore --list`，与部署身份、应用／媒体、TLS 和数据库秘密一起经 OpenSSL CMS AES256-GCM 加密。解密私钥保存在 state 之外的 RecoveryKeys，归档不包含这把解密私钥；清单与 SHA256 校验结果随归档保存。备份后执行解密目录验真，删除的只是本次确切临时文件。

备份包含独立 journal 的导出以供取证／重建，但**不允许将当前 journal 随业务库回滚**。目前没有自动恢复／升级迁移／回滚向导；加密归档可解密、dump 可列目录，仅证明归档结构有效，不等于数据库恢复、媒体完整性、恢复后旧 epoch／会话拒绝或跨机恢复已通过。恢复应在隔离环境按后端受控恢复合同继续实现并验证；不要直接覆盖当前业务目录或独立 journal。此入口运行账号兼具其域数据库建表／迁移权限，尚未宣称生产级迁移账号与运行账号分离。

反部署先以fzf选择“仅停止并保留全部资料”（默认）或“进入反部署清理”。清理层默认“数据库数据备份”，可明确选“数据库数据清零”；最终展示准确范围并独立读取回车，非空或EOF取消，`--yes`不旁路。备份生成state与源码之外的加密归档，解密验真并核原摘要后才清理；备份失败、来源变化或归档被替换时保留源与原备份，不重新备份后默默清零。

只删除原点标为本实例新建且身份相符的业务库、程序、媒体及运行资源。独立 `JournalDatabase`、`Application/instance.json`及其父目录、`Secrets`、`TLS`、外置备份、恢复私钥和归属审计默认保留；销毁独立恢复事实及匹配钥须另行授权，backup／clear都不能扩大到此范围。清理成功仍报告 `partial`，不称完整还原部署前环境；复用或未知资源保留，缺有效原点的旧部署不能追认清理资格。

持本实例事务锁，仅停PID/启动时间/命令与物理集群身份相符的进程。冷态快照核设备/inode、文件SHA和目录全集，链接、未知顶层及并发变更阻断；只逐项unlink/rmdir，不递归清空state。断点在 `.Deployment/undeploy`，续跑只处理原计划，不能重备份或重新初始化扩大范围。部署发现断点或受保护遗留即拒绝新建业务库。只有匹配当前实例的本机reference才移除，共享工具、源码及用户全局信任不处理。

governance占用第八个内部端口（base+7），其独占schema映射到identity数据库并纳入identity.dump，不额外建同名数据库。Go安全恢复工具的单业务库演练不覆盖多业务库聚合恢复，当前没有跨库自动恢复向导。

## 五、<span id="nats">自托管 NATS JetStream</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

NATS按可验证稳定目标优先复用健康既有工具，必要准备先校验来源及SHA；不可验证就明确失败，不调用全局包管理器或关闭同步冒充成功。`check / plan / status`只读源码角色、路径与收据，不运行版本探针、生成token/配置/JetStream或启停broker。

| 变量 | 默认与限制 |
| --- | --- |
| `JOBS_IM_NATS_BIN` | 既有可执行文件；默认查询 PATH，MacOS 还可发现唯一用户受管 NATS 分发 |
| `JOBS_IM_NATS_PORT` | 4222，只监听 `127.0.0.1`，必须与所有动态角色／PG／HTTPS端口不同 |
| `JOBS_IM_NATS_URL` | `nats://127.0.0.1:4222`，必须与本次端口一致；不接管外部broker，不含认证信息 |
| `JOBS_IM_NATS_TOKEN_FILE` | `Secrets/nats.token`，只能指定本实例Secrets内普通文件；值不进argv／日志／环境 |
| `JOBS_IM_NATS_MAX_FILE_BYTES` | 536870912（512MiB），可显式调大；后端单实例stream上限256MiB，额外空间仍需磁盘预算 |
| `JOBS_IM_NATS_MAX_MEMORY_BYTES` | 33554432（32MiB），可显式调大；不是整个宿主资源限制 |

配置 `Secrets/nats.conf` 与 token 一并私有保存，broker只接受 `-c 配置路径`。持久数据为本实例 `Application/JetStream/`，从不写入Caches／临时目录；既有数据缺token、配置不一致、PID／启动时间／工具／配置摘要不匹配均保留并拒绝接管。端口被占不按名称或端口杀进程。启动先验真配置、认证PING与JetStream标志，再启动Go；停止先应用，后broker，数据保持。

当前Go入口发现9个内部角色（identity、directory、conversation、message、media、control、journal、governance、sync）＋gateway；内部端口按源码顺序动态预检，当前是base至base+8。governance schema位于identity库，sync使用独立库，共8个物理业务／日志域数据库。Go进程仅接收无秘密NATS URL和token文件路径；后端自行建立带部署身份的JetStream流与durable消费者，不将HTTP补偿回放当成NATS替代。

默认备份先停应用与broker，归档明确包含全部域dump、独立journal导出、`Application/JetStream`、私有NATS配置／token及broker范围清单；缺原收据、未证实已停或配置漂移时拒绝标完整。broker归档不是两份PG dump能替代的，解密目录验真也不等于JetStream／跨库恢复演练；恢复在隔离环境核验，不能覆盖当前journal或用旧broker事件绕过当前owner权限／epoch。

官方依据：[NATS配置](https://docs.nats.io/reference/config/)、[运行JetStream](https://github.com/nats-io/nats.docs/blob/master/running-a-nats-service/running/README.md)。实际采用版本及其官方目标由每次正常部署查询，不把固定版本或历史专项当作任意时间的最新版。

候选八域CMS协调恢复使用 `jobsimops --mode deployment-restore-plan|deployment-restore --deployment-spec <私有spec>`；路径相对spec解析，实际写入须 `--offline`。目标原入口只读 `restore-layout.conf`取端口，`restore.pending`时拒启动。最新current ledger交接、旧broker隔离重建及当前未完成实测边界见[恢复候选](../../项目相关报告/实现进度/20261116-剩余能力闭环/服务端恢复.md)，不能据此将`safe_restore`标为已验。

<a id="🔚" href="#前言">回到首页</a>
