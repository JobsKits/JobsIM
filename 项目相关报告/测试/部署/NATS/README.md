# 自托管 NATS 部署限定验证

![Jobs出品，必属精品](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、交付范围](#jobs-toc-heading-2)
- [二、实际结果](#jobs-toc-heading-3)
- [三、历史与限制](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

核验日期：2026-11-16。本轮补齐三平台部署／停止入口的本机JetStream依赖；**MacOS真实独立broker通过，Linux仅Bash解析，Windows仅PowerShell解析及C#桥编译**。没有重新执行完整应用／双PG／八库备份或聚合恢复，不把旧七库报告外推为新NATS验收。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、交付范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

三平台各自两份独立自包含实施脚本，同平台仅默认动作不同；当前从Go入口发现9内部角色＋gateway，端口按源顺序动态计算。治理schema沿用identity库，sync新增独立域库，journal仍在独立PG集群。既有TLS、WebDist、数据库归属／口令、暂停备份、数据保留、二维码可选回退及平台差异均保留。

新增loopback NATS、私有token／配置、PID＋启动时间＋工具／配置SHA256归属、认证PING及JetStream就绪、先应用后broker停止、冷态broker备份范围。数据在外置实例`Application/JetStream`，配置／token在Secrets，秘密不作argv／日志／环境值。缺既有工具明确报查找位置，不自动包管理器安装、下载或降级关闭同步；check／plan／status不初始化、不连接或启停broker。

Windows用独立控制台进程组及精确CTRL_BREAK；旧进程无组收据时拒强停，应用未证实退出则保留broker／外部PG。依据[微软控制台组信号](https://learn.microsoft.com/en-us/windows/console/generateconsolectrlevent)与[NATS2.15.3 Windows信号实现](https://github.com/nats-io/nats-server/blob/v2.15.3/server/signal_windows.go)，没有把`--signal stop`误作普通Windows进程PID控制。该平台行为尚未原生执行。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、实际结果 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 范围 | 实际结果 | 证据 |
| --- | --- | --- |
| Mac／Linux入口语法 | 8个实施／快捷脚本解析通过 | [静态／只读／同源检查](./20261116-部署检查/静态与只读结果.json) |
| Mac原有unit | 通过；未知非空state哨兵保留 | [原unit输出](./20261116-部署检查/Mac-Unit.txt) |
| check／plan／status | 三者退出0，state仍不存在 | [check](./20261116-部署检查/check.txt)、[plan](./20261116-部署检查/plan.txt)、[status](./20261116-部署检查/status.txt) |
| 缺NATS工具 | 非零拒绝，显示实际查找位置，无state写入 | [缺工具输出](./20261116-部署检查/缺NATS工具.txt) |
| 可迁移路径 | 中文／空格复制位置，从`/tmp`调用check通过，无state写入 | [路径输出](./20261116-部署检查/复制改名与外部CWD.txt) |
| 配对副本 | 三平台部署／停止主体逐字相同，仅默认动作不同 | [检查JSON](./20261116-部署检查/静态与只读结果.json) |
| Windows宿主检查 | 两份PowerShell Parser与C#桥编译通过；未调用Win32 | [原输出](./20261116-部署检查/PowerShell解析与桥编译.txt) |
| 真实NATS协议 | 缺／错token拒绝；正确认证及JetStream file发布通过 | [最终首次协议](./20261116-005403-59263/首次协议.txt) |
| 重启持久化 | 原token不变，原目录重启后读回seq1原内容 | [最终重启协议](./20261116-005403-59263/重启协议.txt) |
| 归属与秘密 | broker argv／日志不含真实token；配置漂移拒绝；陌生自建sleep PID保留 | [保护输出](./20261116-005403-59263/归属保护.txt) |
| 停止及归档范围 | 自有broker已退出，JetStream和私有材料保留，生成明确冷态范围清单 | [结果](./20261116-005403-59263/结果.txt)、[broker清单](./20261116-005403-59263/broker-manifest.txt)、[起止一致源码SHA](./20261116-005403-59263/源码SHA256.txt) |

工具为已按官方release校验SHA256的现成NATS2.15.3；官方校验记录位于工程外受管工具目录。协议测试只用Node内置TCP，未装新依赖。测试使用显式健康Node24.22.1和进程级PATH；不持久更改环境／代理／CA。既有36443／44443应用及45222 broker未被本任务启停或写入。

本轮独立测试数据和token留在用户Application Support的DeploymentTests私有目录，报告只记录非秘密位置／端口与结果，不复制token、配置正文或JetStream文件到报告。broker数据归档是完整备份范围的一部分；本轮只验证范围清单，**未声称执行了新增八库＋NATS完整加密备份／恢复**。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、历史与限制 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

早期宿主Node26因缺libsimdutf中断于broker启动之前；改用现成健康Node，没有修Homebrew或下载。一个早期测试入口把EXIT钩子定义在函数内，初始化即退出0，已在[原目录说明](./20261116-001237-31263/README.md)明确不算通过；最终入口改脚本级钩子，只有完整执行标志及源码起止SHA一致才通过。[先前轮源码绑定限制](./20261116-002335-42629/说明.md)同样保留，最终证据以上表为准。

Windows／Linux目标机、ACL／控制台组实际信号、真实关窗／父进程异常回收、服务托管、8库＋broker一致备份／隔离恢复、容量与故障域耐久仍未验。默认资源是broker磁盘512MiB／内存32MiB，并非整机容量保证；不宣称强保护、RPO0或完整I验收。Windows后端私有文件验证还需与实际ACL联合验收，不能用Unix0600位冒充。

入口与参数：[MacOS](../../../../部署/MacOS/README.md)、[Linux](../../../../部署/Linux/README.md)、[WindowsOS](../../../../部署/WindowsOS/README.md)、[隔离测试](../../../../测试/部署/MacOS/NATS专项.command/README.md)。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
