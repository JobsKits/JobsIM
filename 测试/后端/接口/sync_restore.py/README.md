# 权威同步真实备份恢复代际测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、执行与私密边界](#execution)
- [二、实际断言与范围](#coverage)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

[测试脚本](./sync_restore.py)使用固定本地候选二进制，创建独立的数据库、CA／SKI／AKI、State 与带认证的[**NATS JetStream**](https://docs.nats.io/nats-concepts/jetstream)。只借用原同步 seed 中既有工具和两个[**PostgreSQL**](https://www.postgresql.org/)物理集群，不连接原44443 HTTP服务、不修改原seed、不停止原服务、不编译或下载。

## 一、<span id="execution">执行与私密边界</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

- 依赖既有[**Python**](https://www.python.org/)、OpenSSL、匹配的PostgreSQL工具、固定jobsimd与同源jobsimops、nats-server，以及原seed指定的当前用户私有PGPASSFILE；缺失即退出，不安装依赖。源码只复用[同步HTTPS测试类](../sync_snapshot.py/sync_snapshot.py)，不执行其510行批量套件。
- 首屏展示内置自述，交互运行按回车继续；自动化仅通过明确的`--execute-authorized`表达已有授权。公开端口47443、内部48101–48109和NATS48222必须全部空闲，任一占用即停止。
- 新run位于运行态Integration目录的`sync-restore-随机/Safety`。固定二进制用硬链接复用且不修改；私密seed、auth／password、token、PGPASSFILE与完整进程日志留在新Safety，文件权限0600、目录0700。连接信息只通过环境传递，SQL走stdin，token不进入argv或公开日志。
- 加密备份另置当前用户`Documents/JobsIMBackups/sync-restore-随机/`，恢复密钥另置新run的`Keys/`。源应用只按本轮创建的完整argv与PID核对后停止，PostgreSQL与持久broker跨恢复持续运行。结束停止本轮应用，保留该独立broker及数据库／备份／私密诊断供核查；不会删除证据或批量清理端口。
- `--report`和`--raw-log`必须是中央报告目录下尚不存在的新文件；不会覆盖既有100HTTP或其它证据。失败非零退出，公开日志只记步骤与计数，具体异常留私密`Secrets/failure.json`；源码问题只报告，不自动改业务源码或换身份继续。
- 若真实恢复已完成、仅后续测试探针失败，可显式追加`--resume-owned-environment /该新run/Safety/environment.json`续验。它要求原失败报告与raw SHA完全匹配、新应用已停止、原broker PID／完整argv仍相同；只重验该目标E2，不重新备份／恢复或分配epoch。原失败报告先存私密历史文件，raw保留原字节并追加`RESUME`，最终报告注明重试原因与计数范围；不接受已通过报告来覆盖证据。

```shell
python3 ./sync_restore.py \
  --environment-file /明确选择的原同步域/Safety/environment.json \
  --report ../../../../项目相关报告/测试/后端/权威同步底座/真实恢复.json \
  --raw-log ../../../../项目相关报告/测试/后端/权威同步底座/真实恢复.log \
  --execute-authorized
```

## 二、<span id="coverage">实际断言与范围</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

- 两个新账号通过真实HTTPS建立好友、群与两条消息，注册同步store、读取完整manifest并校验原boundary。真实加密备份后再撤回第一条、编辑第二条；独立journal保留这些后续保护事实。
- 停止新域源应用，创建同业务物理集群的全新空目标DB与空RestoredState，真实`restore --offline`并用同operation-id重查，确认只分配一次E2。全部角色再用同固定binary、RestoredState、目标数据库及同一持久broker启动。
- 旧AT401；原密码新登录得到新device／E2。旧boundary409 `RESTORE_EPOCH_CHANGED`，旧snapshot页404；新snapshot引用全部属于E2，老消息保留E1原creation tuple。撤回分类global且正文为空；缺新编辑正文时`contentUnavailable`且不回退旧文。完整新manifest通过最终validate，E2新消息有新创建代际并出现在原新boundary后的真实delta。
- 在任何同步HTTP补偿前用SQL观察E2 origin到达，证明真实JetStream网络续接。旧epoch负例选备份后真实Outbox origin，保存broker原始wire字节和MsgId；等待既有去重窗口自然到期，确认PubAck非duplicate、consumer真实ack floor推进，目标仍无该E1 origin且E2用户位置不增加。等待不超过150秒，不修改stream／consumer配置、不清去重、不制造替代origin。

这是同一业务物理集群中单业务数据库恢复到新数据库，加上另一个物理PostgreSQL集群上的独立journal，以及同持久单机JetStream跨应用恢复验证。不是空JetStream重建、多owner独立数据库聚合恢复、跨主机灾备、强保护、客户端原子安装或qualified ACK验收。

本轮最终证据包含一次测试探针的NATS `CONNECT`未声明`headers`能力导致的失败，以及修正包装后同一已恢复域的续验；业务源码与恢复操作未改。成功续验计数为22次HTTP检查、56条语义断言，初始建域／真实备份／离线恢复保留在同一raw前段，不把未单独计数的前段请求凑入结果。

证据归[中央权威同步报告目录](../../../../项目相关报告/测试/后端/权威同步底座/)。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
