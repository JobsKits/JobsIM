# JobsIM WindowsOS PowerShell 原生入口

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [当前能力与边界](#jobs-toc-heading-1)
- [工具、数据库与连接](#jobs-toc-heading-2)
- [备份、密钥和恢复](#jobs-toc-heading-3)
- [自托管 NATS JetStream](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">当前能力与边界</span><!-- jobs-toc:heading-end -->

此入口为 PowerShell7.4+ 原生程序，构建 Go／WebDist、生成实例专用 TLS、启动同源网关；停止后可把八个预置角色数据库和本实例媒体／身份／TLS加密备份。**当前没有 Windows 目标机验收。** 本机 PowerShell7.7 仅完成解析、帮助和非 Windows 拒绝检查，不代表构建、ACL、Ctrl+C、网络或备份在 Windows 上成功。

与 MacOS／Linux 的双集群初始化入口不同，这个切片**要求操作者预先准备业务 PostgreSQL18 和物理独立的 journal PostgreSQL18**、八个域数据库及各自非超级用户账号。它不安装、创建、停止、删除或接管外部 PG 服务。后端启动会验证 journal 的物理集群标识；不能用同一集群改库名代替独立 journal。数据库角色须已具备所属域 schema 迁移与必要 `pg_control_system()` 检查权限，不将未知账号提权。

- [运行快捷入口](./【WindowsOS】快捷入口_部署运行.cmd)、[停止备份快捷入口](./【WindowsOS】快捷入口_停止备份.cmd)。
- 自包含实现：[部署运行.ps1](./部署运行.ps1/部署运行.ps1)、[停止备份.ps1](./停止备份.ps1/停止备份.ps1)。支持 `check / plan / status / run / stop / backup`，无参实现只读／停止，快捷运行会明确选择 run。
- [代码地图](./代码地图/README.md)、[测试边界](../../测试/部署/README.md)、[后端契约](../../代码/后端/Go/接口契约/README.md)。

```powershell
pwsh -NoProfile -File ./部署运行.ps1/部署运行.ps1 check
pwsh -NoProfile -File ./部署运行.ps1/部署运行.ps1 run
# 新格式收据可向本入口独立进程组发CTRL_BREAK，先应用后NATS；旧收据需原终端正常退出。
pwsh -NoProfile -File ./停止备份.ps1/停止备份.ps1 stop
```

变更操作回车确认；自动化显式 `-Yes`。`-State` 或 `JOBS_IM_DEPLOY_STATE` 选择外置私密实例目录；默认当前用户 LocalApplicationData/JobsIM/Instances/项目路径哈希。只接受本项目／当前用户 SID 收据，不接管非空无收据目录、junction、符号链接或复用 PID。state 使用仅当前用户的私有继承 ACL；权限设置失败即阻断。`check / plan / status` 不创建目录／密钥，不启停资源。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">工具、数据库与连接</span><!-- jobs-toc:heading-end -->

准备既有nats-server.exe、Go1.27、PostgreSQL18 的 pg_dump／pg_restore、Node22.12+／npm、curl.exe、tar.exe、支持 CMS AES256-GCM 的 OpenSSL。工具从 PATH 或 `JOBS_IM_GO / JOBS_IM_PG_BIN / JOBS_IM_OPENSSL` 取得，不修改全局环境、不自动安装依赖、不放宽 PowerShell 执行策略；目标机器的签名／执行策略由操作者按既有组织政策配置。

在**当前运行进程环境**分别配置 `JOBS_IM_IDENTITY_DATABASE_URL`、`JOBS_IM_DIRECTORY_DATABASE_URL`、`JOBS_IM_CONVERSATION_DATABASE_URL`、`JOBS_IM_MESSAGE_DATABASE_URL`、`JOBS_IM_MEDIA_DATABASE_URL`、`JOBS_IM_CONTROL_DATABASE_URL`、`JOBS_IM_SYNC_DATABASE_URL`、`JOBS_IM_JOURNAL_DATABASE_URL`。连接串由维护者的私密管理工具注入，不写入脚本、README、参数日志或共享终端记录。备份进程把 URI 转成其专属 libpq 环境，密码不放命令行；支持标准 PostgreSQL URI 和 sslmode／sslrootcert／sslcert／sslkey／connect_timeout 参数，其余参数明确拒绝，不静默改变连接语义。

`JOBS_IM_HTTPS_PORT` 默认9443；`JOBS_IM_INTERNAL_PORT` 默认18101，按Go源码当前连续九个。只监听127.0.0.1，外部发布和系统服务托管尚未交付；端口占用时拒绝接管。Go／npm 的依赖事先准备，构建失败不会报告成功。运行前保留既有数据并对已创建实例加密备份，保持部署身份不变。

严格 TLS bootstrap 通过后输出真实用户 URL、deploymentId、CA SHA256 和不含秘密的 connection.json。管理写入口和二维码工具未接入，不记作验收通过。由可信运行终端取得 CA／指纹核对；原生端仅当前连接档案信任，继续校验主机名、SAN 和到期；Web 受控安装本地 CA 或使用公信 HTTPS，未信任即阻断，不绕证书告警。脚本不修改系统全局 CA；维护者另行记录并清理曾安装的信任材料。

运行需要真实Windows控制台。新进程由内嵌Win32桥创建独立进程组，控制助手只向核验PID／启动时刻／工具摘要的组发送CTRL_BREAK，先应用后NATS；不使用全局CTRL_C、taskkill或Stop-Process。旧收据没有独立组证明时要求回原运行终端正常退出。外部PG继续由其维护者管理。当前只在Mac完成PowerShell解析及C#桥编译，Windows控制台、ACL、关窗／跨终端停止仍未原生验证。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">备份、密钥和恢复</span><!-- jobs-toc:heading-end -->

数据库维护者须确认该实例八域没有其他写入服务；默认 stop／backup 在本后端已停止后逐库导出八份 custom dump、检查 pg_restore 清单，把 Application（部署身份／媒体）和 TLS 一起 CMS AES256-GCM 加密。解密目录验真、SHA256 和清单随归档存放。恢复私钥默认在 state 同级 RecoveryKeys/实例名，归档默认在 state 同级 Backups/实例名；也可通过 JOBS_IM_RECOVERY_DIR／JOBS_IM_BACKUP_DIR 指定，二者不得位于 state 内。恢复钥应离线另存；PG 外部角色和凭据需由数据库维护者单独保存，本归档不能代替其恢复。

不会自动清空数据、证书、密钥、备份或外部资源。备份包含 journal 导出，但不自动覆盖当前 journal，不能与业务库一起回滚。尚未实现安全恢复／升级迁移／回滚向导，也未在 Windows 验证数据库还原；归档可列目录不等同完整恢复演练，更不代表生产容量、强保护或 RPO0 已通过。

当前新增governance占用第八个内部端口（base+7），本入口将其独占schema映射到identity数据库，纳入identity.dump，不新增数据库或迁移既有数据布局。Go安全恢复工具的单业务库演练证据不覆盖本入口的多业务库聚合恢复；当前仍未交付跨库自动恢复。

Windows独立进程组依据[微软控制台信号文档](https://learn.microsoft.com/en-us/windows/console/generateconsolectrlevent)；NATS的`--signal`在Windows仅面向服务，脚本不借此控制名为nats-server的既有系统服务。[NATS2.15.3源码](https://github.com/nats-io/nats-server/blob/v2.15.3/server/signal_windows.go)。秘密文件设置当前用户独占ACL，不以Unix0600位冒充Windows保护；后端Windows安全文件检查及整条原生链另验。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">自托管 NATS JetStream</span><!-- jobs-toc:heading-end -->

默认需要已有 `nats-server`，缺失时检查明确失败，提示实际查找位置；不自动调用 Homebrew／apt／winget、下载或降级关闭同步能力。`check / plan / status` 只读工具版本、源码角色、收据，不生成 token／配置／JetStream，不连接或启停 broker。

| 变量 | 默认与限制 |
| --- | --- |
| `JOBS_IM_NATS_BIN` | 既有可执行文件；默认查询 PATH，MacOS 还可发现唯一用户受管 NATS 分发 |
| `JOBS_IM_NATS_PORT` | 4222，只监听 `127.0.0.1`，必须与所有动态角色／PG／HTTPS端口不同 |
| `JOBS_IM_NATS_URL` | `nats://127.0.0.1:4222`，必须与本次端口一致；不接管外部broker，不含认证信息 |
| `JOBS_IM_NATS_TOKEN_FILE` | `Secrets/nats.token`，只能指定本实例Secrets内普通文件；值不进argv／日志／环境 |
| `JOBS_IM_NATS_MAX_FILE_BYTES` | 536870912（512MiB），可显式调大；后端单实例stream上限256MiB，额外空间仍需磁盘预算 |
| `JOBS_IM_NATS_MAX_MEMORY_BYTES` | 33554432（32MiB），可显式调大；不是整个宿主资源限制 |

配置 `Secrets/nats.conf` 与 token 一并私有保存，broker只接受 `-c 配置路径`。持久数据为本实例 `Application/JetStream/`，从不写入Caches／临时目录；既有数据缺token、配置不一致、PID／启动时间／工具／配置摘要不匹配均保留并拒绝接管。端口被占不按名称或端口杀进程。启动先验真配置、认证PING与JetStream标志，再启动Go；停止先应用，后broker，数据保持。

当前Go入口发现9个内部角色（identity、directory、conversation、message、media、control、journal、governance、sync）＋gateway；内部端口按源码顺序动态预检，当前是base至base+8。沿用governance schema位于identity库的布局，新增sync独立库；当前共8个物理业务／日志域数据库，不靠旧六库清单遗漏sync。Go进程仅接收无秘密NATS URL和token文件路径；后端自行建立带部署身份的JetStream流与durable消费者，不将HTTP补偿回放当成NATS替代。

默认备份先停应用与broker，归档明确包含全部域dump、独立journal导出、`Application/JetStream`、私有NATS配置／token及broker范围清单；缺原收据、未证实已停或配置漂移时拒绝标完整。broker归档不是两份PG dump能替代的，解密目录验真也不等于JetStream／跨库恢复演练；恢复在隔离环境核验，不能覆盖当前journal或用旧broker事件绕过当前owner权限／epoch。

官方依据：[NATS配置](https://docs.nats.io/reference/config/)、[运行JetStream](https://github.com/nats-io/nats.docs/blob/master/running-a-nats-service/running/README.md)。本轮工具专项使用已按官方release校验SHA256的NATS2.15.3，不把它声明为任意时间的最新版。
