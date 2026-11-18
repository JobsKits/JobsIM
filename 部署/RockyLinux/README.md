# <span id="前言">JobsIM · RockyLinux 原生部署与反部署</span>

![JobsIM](../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、入口与运行边界](#jobs-toc-heading-1)
- [二、模式、机器与工具](#jobs-toc-heading-2)
- [三、参数、路径与归属](#jobs-toc-heading-3)
- [四、访问、信任与生命周期](#jobs-toc-heading-4)
- [五、反部署、备份与验证](#jobs-toc-heading-5)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、入口与运行边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[**RockyLinux**](https://rockylinux.org/)使用本平台自包含Bash4+，不转调其他系统脚本，不经过Docker或虚拟机。本机仅有MacOS宿主隔离验证；本发行版安装、真实PG／NATS运行、公网与桌面双击仍需目标机验收，不能由语法检查代替。

| 入口 | 当前行为 |
| --- | --- |
| [双击部署](./双击部署（快捷入口）.desktop) | 图形终端打开后回车授权，fzf选择本地或服务器，再选本地访问范围 |
| [双击反部署](./双击反部署（快捷入口）.desktop) | fzf先选实例模式，再选仅停保留或资源清理 |
| [Deployment](./Deployment/README.md) | `menu / run / check / plan / status / stop / backup` |
| [Undeployment](./Undeployment/README.md) | 真正的原点清理、备份默认／清零、检查点与受保护保留项 |
| [代码地图](./代码地图/README.md) | 资源归属、工具、进程、TLS、二维码与恢复定位 |

桌面文件只从自己的本地目录解析唯一同名脚本包；缺包、多包、相对路径或URI输入会停止。需要图形桌面、终端模拟器，以及用户按文件管理器规则允许启动；脚本不替用户修改桌面信任。无桌面直接用终端或SSH执行：

```bash
/bin/bash './Deployment/【Linux】部署JobsIM.sh/【Linux】部署JobsIM.sh'
/bin/bash './Undeployment/【Linux】反部署JobsIM.sh/【Linux】反部署JobsIM.sh'
```

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、模式、机器与工具 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

本地模式默认仅本机，严格HTTPS核验双PG／JetStream／Go，不查询或安装cloudflared。只有明确选择公开穿透，才准备Cloudflare Quick Tunnel并用本次空配置连接本机Origin CA；取得真实地址后只重启本轮Go以绑定同源PublicURL，再核验公网HTTPS。穿透失败或运行中退出时撤回旧公开描述和二维码，停匹配隧道，只重启自己的Go回本机PublicURL；本地严格健康通过后继续提供重试／诊断／停止菜单，不重建PG、NATS或身份。不会关闭TLS校验、安装全局根证书或修改本人cloudflared配置。公开入口单独回车确认，取得地址者可到达登录页，只使用允许公开的联调资料。

服务器模式必须提供真实HTTPS origin及匹配的证书／私钥／CA，校验SAN与密钥对；需要操作者已准备域名与可信反向入口，应用仍仅监听回环。公网严格健康失败不报告部署完成。两模式使用不同实例路径与本机引用，不能拿本地标签替代服务器验证。

CPU与总／可用内存只报告，尚未测定最低支持档位。新的重型下载／构建保留至少4GiB可用磁盘；下载按官方响应的实际字节数、解包按压缩流实际展开字节数另加4GiB预留，无法确认大小时停止该安装。备份按当前文件逻辑字节与实测dump／源快照估算归档及验真空间，另保留4GiB；不清缓存腾空间。未经最低资源／并发压测，不承诺原生方案具体节省多少内存。

基础环境为Bash4+、curl、GNU coreutils／tar、lsof、nc、OpenSSL3及发行版原生[**PostgreSQL**](https://www.postgresql.org/)18完整工具。PG获取以[本发行版官方安装说明](https://www.postgresql.org/download/linux/redhat/)为准，入口不sudo、不自动安装或升级用户数据库主版本。

正常部署按[官方版本解析器](../公共组件/依赖版本/README.md)核对Go、fzf、Node LTS、NATS的当前稳定目标；仅明确公开时另核cloudflared与必要能力；显式指定的旧／坏工具停止，不覆盖原件。健康且版本足够的工具复用；缺失的可验证资产按官方SHA下载至机器级版本目录，元数据暂不可用会退避重试、Ctrl+C取消；无可信摘要不安装。PG官方目标单独报告，仍核验业务需要的18工具，不臆造Linux PG官方二进制包。业务Go／Web依赖只报告更新，保持源码依赖锁。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、参数、路径与归属 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

```bash
/bin/bash './Deployment/【Linux】部署JobsIM.sh/【Linux】部署JobsIM.sh' check
/bin/bash './Deployment/【Linux】部署JobsIM.sh/【Linux】部署JobsIM.sh' run --mode local --access local
/bin/bash './Undeployment/【Linux】反部署JobsIM.sh/【Linux】反部署JobsIM.sh' undeploy --mode local
```

`check / plan / status`只读，不创建实例、不查询网络最新版本、不安装工具或启停服务。`--access local|quick`选择本地访问范围，默认仅本机；公开仍需独立回车。`--state`选择独立私有绝对路径，拒绝源码目录、符号链接及链接祖先；`--yes`只用于已明确授权的常规自动化，不跳过清理确认。`stop --no-backup`是仅停运维子动作，保留资料，不能称为完整反部署。

| 配置 | 位置／约束 |
| --- | --- |
| `JOBS_IM_DEPLOY_STATE`／`--state` | 独立实例；默认按项目哈希、发行版、local／server分开 |
| `JOBS_IM_TOOLS_ROOT` | 默认XDG机器数据目录下JobsTools／JobsIM，版本缓存与共享工具保留 |
| `JOBS_IM_BACKUP_DIR`／`JOBS_IM_RECOVERY_DIR` | 实例外且彼此独立的加密归档与恢复私钥 |
| `JOBS_IM_GO`／`JOBS_IM_FZF_BIN`／`JOBS_IM_NODE`／`JOBS_IM_NATS_BIN`／`JOBS_IM_CLOUDFLARED` | 分别使用完整`JOBS_IM_`前缀显式指定健康工具，不覆盖本机原件 |
| `JOBS_IM_PG_BIN / JOBS_IM_OPENSSL` | 已有PG18与OpenSSL可执行路径 |
| `JOBS_IM_PUBLIC_URL` | 服务器正式HTTPS origin，不含路径、凭据、查询或片段 |
| `JOBS_IM_SERVER_CERT`／`JOBS_IM_SERVER_KEY`／`JOBS_IM_SERVER_CA` | 分别使用完整`JOBS_IM_`前缀；服务器配套材料，私钥须本人0600文件 |
| `JOBS_IM_HTTPS_PORT`／`JOBS_IM_PG_PORT`／`JOBS_IM_JOURNAL_PORT`／`JOBS_IM_INTERNAL_PORT`／`JOBS_IM_NATS_PORT` | 完整`JOBS_IM_`前缀；默认9443／16543／16544／18101起／4222，均回环 |

首次确认后记录`.Deployment/resources.v1`与`resources.tsv`：uid、根目录dev／inode、首次原点、实例部署ID、资源created／reused及设备／inode。已存在资源不会追认为新建；旧无清单实例不会被猜测清理。双PG按物理标识核验，按当前Go入口发现九角色与八物理域数据库；governance schema沿identity库，journal独立。TLS、Secrets、Application／媒体、JetStream、WebDist与会话产物分工保存，源码和用户共享工具不属于删除范围。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、访问、信任与生命周期 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

成功后显示用户端完整URL及部署ID，不生成尚未交付的管理后台假链接。实际地址由[离线二维码工具](../公共组件/地址二维码/README.md)生成终端与HTML／SVG／PNG；读取TTY真实宽高，足尺寸时显示不依赖块字形的黑白背景方格，NO_COLOR不输出ANSI。尺寸不足或主题极性无法保证时，仅在有图形桌面且存在xdg-open时打开本轮本地PNG；无图形预览明确报告未显示可扫码图，保留PNG／URL，不修改全局配色。回环码明确仅本机、手机不可用。菜单可打开用户端或本地扫码页，本地模式还提供开启／重试公开穿透及诊断；无桌面／二维码构建失败保留可复制URL。旧二维码在退出时标记失效，另存图片无法主动撤销；临时地址结束后不可复用。

Quick公开描述使用真实公网URL与`tlsTrust=system`；`originCaSha256`只用于后端直连核对，不能冒充Cloudflare公网证书根。原直连描述保留`caSha256`语义。新system-trust描述的原生端导入仍需专门验收；本任务没有改端侧解析或放宽安全校验。

**关闭此运行终端，服务将结束。** 正常退出、INT／TERM／HUP沿同一清理，另有父进程身份监测兜底；只核对本次PID、启动时间、可执行路径／配置摘要后结束进程，不按名称或端口强杀。新启动仅在确认模式后停原匹配实例，再启新实例；后续失败保留数据并说明旧实例已停，不承诺自动恢复旧进程。未知锁／PID、端口冲突或活动反部署保留并停止。真实关窗、断电、父进程异常与公网行为仍须Linux目标机验收。

终端保护只在可可靠读回与恢复当前TTY配色时生效；未知终端／无TTY／NO_COLOR使用原状态或纯文本，不修改系统终端Profile。操作日志在本次Sessions运行目录，工具版本目标与业务库报告各自留文件；秘密不进普通日志。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">五、反部署、备份与验证 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

反部署首先选择仅停止保留或进入清理。进入清理后fzf默认“数据库数据备份”，也可明确选清零；最终准确范围需独立回车，任何非空、EOF、空选或Esc取消，`--yes`不旁路。备份在全部匹配服务冷停后覆盖两个PG冷态目录、业务／附件、TLS及匹配秘密，CMS AES256-GCM归档外置，校验恢复钥匹配并实际解密逐文件比对；失败或确认后的源内容变化保留源与原备份，不静默重备份后继续删。broker清单必须生成并在解密副本逐字核对；任何步骤失败均沿调用条件链返回。运维逻辑备份另验全部八域dump的pg_restore清单，并从CMS实际解密出broker清单和八dump逐项比对；与反部署完整冷态目录备份分别记录。冷备份有效不等于受控恢复演练；journal不可随业务回滚。

只对created清单资源以原身份／内容快照逐文件unlink及逐目录rmdir；遇软链接、硬链接、挂载边界、未知对象、身份／权限改变或并发写入拒绝清理。检查点记录原计划、数据策略和已尝试项，中断只能沿原计划核对重试。**JournalDatabase、Secrets、TLS、owner及Application／instance.json默认保留**，与已有备份／恢复钥、共享SDK、源码同样不由普通清零销毁；有保留项时返回未完全还原状态，禁止把原state直接初始化成新身份。销毁受保护资料须另行明确授权。

[独立测试入口](../../测试/部署/Linux/README.md)与[源码绑定证据](../../项目相关报告/测试/部署/20261118-成对部署反部署/Linux/summary.json)记录Bash语法、帮助／非Linux拒绝及临时文件夹的归属、加密备份／验真、清零、取消、并发与中断反例。MacOS上适配stat参数的隔离结果不等于四发行版原生安装、数据库服务、Cloudflare公网、桌面双击或真机完成。
<a id="🔚" href="#前言">返回前言</a>
