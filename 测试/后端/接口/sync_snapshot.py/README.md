# 权威同步底座真实 HTTPS 测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、执行与副作用](#run)
- [二、用例与边界](#coverage)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

这里验证有明确覆盖范围的完整快照、签名增量及自托管[**NATS JetStream**](https://docs.nats.io/nats-concepts/jetstream)指针回放。测试只接受显式新建的私有Safety环境，不碰共享刷新基线、旧报告或系统信任，不声明客户端已原子安装、已阅或qualified ACK。

## 一、<span id="run">执行与副作用</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

- 入口为[测试脚本](./sync_snapshot.py)，依赖标准[**Python**](https://www.python.org/)与已起好的独立后端／[**PostgreSQL**](https://www.postgresql.org/)／NATS。先完成后端构建及独立CA验真，seed只从私密文件读取，不复制凭据到报告。
- 必须提供`freshSyncRealm=true`、`allowOwnedRestartForTests=true`且runDir末级为Safety的专用seed。脚本创建三个独立测试账号、好友／群及正常API消息，另明确用本域SQL造510行分页夹具；SQL批量不作为发送API验收证据。
- 总线专项只按seed的完整binary／config命令核对自有NATS PID，真实停止和重启该进程；不停止后端或其他域，不删除JetStream存储，不降低或清零认证限流。期间只使用该测试域，勿并发客户端验收。
- `--report`必须显式指定中央项目报告目录的新文件；不覆盖旧切片证据。私密原始会话夹具留在工程外Safety／Secrets，公开报告只含计数、摘要和能力边界。

```shell
python3 ./sync_snapshot.py \
  --environment-file /明确选择的新私有Safety/environment.json \
  --report ../../../../项目相关报告/测试/后端/权威同步底座/真实HTTPS.json
```

无seed、错误归属、非HTTPS、PID不符、缺页或断言失败均非零退出，不自动修补原业务或新建替代身份继续假过。标准输出仅脱敏报告；详细进程日志仍在指定Safety目录。

## 二、<span id="coverage">用例与边界</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 范围 | 断言 |
| --- | --- |
| 本地库绑定 | 原注册重试、替换库拒旧device、transport_only不发ACK |
| 完整快照 | 超100与owner500边界、乱序读页、完整manifest、refs原字节摘要、body原字节摘要及显示对象一致 |
| 原子安装前门禁 | 缺页／错序／他人页拒绝、final validate、原ID不续期、源revision变化不可安装、无关新append走原边界delta |
| 增量 | 固定origin摘要、签名游标、重复回放不占新position、明确redacted区间、精确十进制位置 |
| 权限与分类 | 退群再入的ABA拒旧快照／旧cursor、旧云正文仍拒；历史合法成员仅得global墓碑，陌生猜ID与缺失unknown |
| JetStream | 真正PubAck水位、真实停总线时Outbox保留及HTTP补偿、同存储重连后去重不新增用户游标 |
| 无正文持久 | origins／events无正文，另Go套件解密页检查其仅为refs；没有把E2EE密文当可存总线正文 |

[外置Go测试](../sync_snapshot_test.go)通过临时overlay挂载，包含真正独立auth owner数据库＋其他owner RPC的双采集变动、真实NATS ingest去重／篡改拒绝、owner事务并发／回滚／重复迁移、中段缺R2禁止推进及超过浮点精度的本地游标测试。它们各自说明SQL／协议／本地范围，不把Go构造消息冒充真实总线网络发布。

尚未覆盖客户端实际本地事务／存储资格、收讫ACK／after_receipt、read／云草稿／偏好流、普通迁移、正式导出与注销、跨主机与强保护。NATS当前基本配置是单机文件单副本；同存储重连不等于总线数据全丢恢复或RPO0。现有单业务DB备份＋独立日志也不等于多业务DB整套灾备工具已完成。

契约字段、失败保留规则与安装顺序见[后端HTTP契约](../../../../代码/后端/Go/接口契约/README.md#authoritative-sync)。证据统一见[中央同步报告目录](../../../../项目相关报告/测试/后端/权威同步底座/)。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
