# JobsIM Linux 原生运行与停止备份

[toc]

## 当前交付与入口

**未在 Linux 目标机执行构建、启动、停止或备份验收。** 本机只完成 Bash 语法及非 Linux 宿主拒绝检查。此入口是独立原生 Bash4+ 程序，不调用 MacOS 脚本。

此入口在普通用户下原生构建 Go 与 WebDist，创建两个物理独立的 PostgreSQL18 集群：业务集群内按 identity、directory、conversation、message、media、control、sync 分七库七用户；journal 位于第二集群。网关同源提供 HTTPS 与 WebDist。当前是可运行开发切片，不能作为生产容量、完整管理后台、强保护或 RPO0 验收。

- [快捷运行](./【Linux】快捷入口_部署运行.sh)：终端执行后回车确认；关闭该终端将结束本实例服务和两集群。
- [快捷停止备份](./【Linux】快捷入口_停止备份.sh)：确认后暂停服务、生成加密备份并停止两集群，保留全部数据。
- [部署运行源码](./部署运行.sh/部署运行.sh)与[停止备份源码](./停止备份.sh/停止备份.sh)独立自包含；各自可以完成 `check / plan / status / run / backup / stop`。
- [代码地图](./代码地图/README.md)、[测试与验证边界](../../测试/部署/README.md)、[后端契约](../../代码/后端/Go/接口契约/README.md)。

```sh
./部署运行.sh/部署运行.sh check
./部署运行.sh/部署运行.sh run
./停止备份.sh/停止备份.sh stop
```

`check / plan / status` 不创建实例目录、不生成密钥、不安装工具、不启停资源。非交互变更必须显式传 `--yes`；`--state /绝对/私密目录` 为本次实例选择，路径包含空格可用，目录不得属于其他项目、是陌生非空目录或符号链接。`stop --no-backup` 只在明确不需新增备份时使用，仍保留现有备份和数据。

## 环境与外置资源

准备 Bash4+、既有NATS2.x、nc、Go1.27、PostgreSQL18 完整 bin、Node22.12+／npm、支持 CMS AES256-GCM 的 OpenSSL、curl、lsof、GNU coreutils/tar。必须以普通专用用户运行，拒绝 root。工具从 PATH、`JOBS_IM_GO`、`JOBS_IM_PG_BIN`、`JOBS_IM_OPENSSL` 取得；PG还可发现 `/usr/lib/postgresql/18/bin`。不自动 apt／dnf／sudo 安装，不关 Go 校验。Web 依赖须事先准备。

默认 state 是 `${XDG_STATE_HOME:-$HOME/.local/state}/JobsIM/Instances/项目路径哈希`。`Application/` 存部署身份、应用秘密和媒体；`MainDatabase/`、`JournalDatabase/` 为两个物理数据库目录；`TLS/` 为实例证书密钥；`Secrets/` 为数据库密码；`Processes/`、`owner`、`receipt.txt` 为归属记录。数据不进源码，私密 umask077／目录700。未配置 systemd／容器／反向代理，不宣称系统服务安装或开机自启。

| 可配置变量 | 默认／含义 |
| --- | --- |
| `JOBS_IM_DEPLOY_STATE` | 外置实例目录，命令行 `--state` 优先 |
| `JOBS_IM_HTTPS_PORT` | 9443，网关只绑定127.0.0.1 |
| `JOBS_IM_PG_PORT` / `JOBS_IM_JOURNAL_PORT` | 16543 / 16544，数据库只绑定127.0.0.1 |
| `JOBS_IM_INTERNAL_PORT` | 18101，按Go源码动态发现角色，当前连续九个内部端口 |
| `JOBS_IM_PUBLIC_HOST` | 127.0.0.1，公开 URL 与新增证书 SAN；自定义域名须由操作者配置有效解析／受控反向入口，此脚本仍只监听回环 |
| `JOBS_IM_BACKUP_DIR` | state 同级 Backups/实例名，必须位于 state 之外 |
| `JOBS_IM_RECOVERY_DIR` | state 同级 RecoveryKeys/实例名，必须与数据、加密归档分开保管 |

端口冲突会拒绝运行，不杀占用者。同项目重开前先加密备份，再在相同数据上启动，保留部署身份；不会重置密码、覆盖已有 CA 或静默接管未知数据库。源码路径从脚本自身解析，无启动工作目录要求。移动项目会改变默认实例定位；需要继续旧实例时显式指定原 `--state`，核对归属后受控迁移，不能手改归属收据冒充接管。

两个集群及所有管理／应用／备份客户端均使用 `127.0.0.1` TCP，禁用 Unix socket，长 `--state` 不再依赖 socket 路径上限。既有数据库、密码和 `Sockets/` 目录保留。

## 首次连接与生命周期

启动成功输出用户 URL、deploymentId、CA SHA256、公开 `connection.json`。其中不含口令或令牌。管理写入口尚未实现，不生成虚假管理后台 URL。已安装 `qrencode` 时可输出终端与 PNG URL 二维码，否则明确回退可复制 URL；本轮无二维码验收。

由可信部署终端取得并核对 CA／指纹，原生端只在对应连接档案内信任该 CA，仍校验 SAN、主机名与有效期。Web 必须按目标系统／浏览器受控信任本地 CA，或改用公信 HTTPS；未信任即阻断，不能绕过证书警告。脚本不修改系统全局信任。自签 CA 私钥属于本实例维护者，CA 公钥可交给客户端核对；反部署后需由维护者按受控记录手动清理曾安装的浏览器／系统信任，不自动删除机器全局证书。

TLS 在实例内私有 `.TLS-pending.*` 暂存，CA 与服务端证书包含正确用途及 SKI／AKI，严格验证通过后才提交；失败保留 pending 与日志，残缺／未知的旧材料不覆盖。有效旧材料原样复用；旧链不合格时先备份、核对 CA 指纹与扩展，CA 合格才可按原 CA 受控重签服务端证书，CA 本身缺陷需另行安排信任迁移。该 TLS 函数在 Mac 宿主 Bash 隔离验证通过，不代表 Linux 原生服务验收。

运行终端保持前台；Ctrl+C／TERM／退出会回收本轮匹配的服务与两集群。停止时核对 PID、启动时间、命令路径、项目收据和物理集群标识，拒绝强杀陌生进程。并发事务锁不会自动删除；遇到锁应先核对持锁进程，不能据此删除数据。事务失败时保留日志、密钥和数据供诊断。

## 加密备份与恢复边界

`backup` 与默认 `stop` 都会暂停本实例，完成后保持停止。停止后按当前源码发现的八个物理域库逐个 `pg_dump --format=custom`，检查每份 `pg_restore --list`，与部署身份、应用／媒体、TLS 和数据库秘密一起经 OpenSSL CMS AES256-GCM 加密。解密私钥保存在 state 之外的 RecoveryKeys，归档不包含这把解密私钥；清单与 SHA256 校验结果随归档保存。备份后执行解密目录验真，删除的只是本次确切临时文件。

备份包含独立 journal 的导出以供取证／重建，但**不允许将当前 journal 随业务库回滚**。目前没有自动恢复／升级迁移／回滚向导；加密归档可解密、dump 可列目录，仅证明归档结构有效，不等于数据库恢复、媒体完整性、恢复后旧 epoch／会话拒绝或跨机恢复已通过。恢复应在隔离环境按后端受控恢复合同继续实现并验证；不要直接覆盖当前业务目录或独立 journal。此入口运行账号兼具其域数据库建表／迁移权限，尚未宣称生产级迁移账号与运行账号分离。

反部署默认保留业务库、独立 journal、媒体、部署身份、CA／私钥、备份和恢复钥。当前入口不提供删除数据的选项，不把停止当成清零。需迁移／删除时先核对归属、备份与密钥可用性、外部信任安装记录，再走另行明确的数据处置流程。

当前新增governance占用第八个内部端口（base+7），本入口将其独占schema映射到identity数据库，纳入identity.dump，不新增数据库或迁移既有数据布局。Go安全恢复工具的单业务库演练证据不覆盖本入口的多业务库聚合恢复；当前仍未交付跨库自动恢复。

## 自托管 NATS JetStream

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

候选八域CMS协调恢复使用 `jobsimops --mode deployment-restore-plan|deployment-restore --deployment-spec <私有spec>`；路径相对spec解析，实际写入须 `--offline`。目标原入口只读 `restore-layout.conf`取端口，`restore.pending`时拒启动。最新current ledger交接、旧broker隔离重建及当前未完成实测边界见[恢复候选](../../项目相关报告/实现进度/20261116-剩余能力闭环/服务端恢复.md)，不能据此将`safe_restore`标为已验。
