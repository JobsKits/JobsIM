# S2 本人状态真 PostgreSQL 定向验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、结果与证据](#results)
- [二、真实覆盖和迁移范围](#coverage)
- [三、隔离、资源与运行边界](#isolation)
- [四、仍未验证和未交付项](#remaining)
- [五、输入与归档](#hashes)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本目录追加 2026-11-16 的 Phase A 限定验证：使用全新私有目录、全新身份和单个物理 [**PostgreSQL**](https://www.postgresql.org/) 18.7 cluster，验证已接入的本人 read／view／preferences、schema1 兼容和 physical-store 围栏。**20 个真实 PG 测试组＋2 个纯本地连接 guard 通过，0 测试失败／0 跳过。** 测试目标编译及候选二进制构建也通过。

[原 source-only 检查点](../2026-11-16-S2本人状态-源码检查点/README.md)和[先前本地编译报告](../2026-11-16-S2本地编译与纯规则/README.md)保持原义。本轮没有修正生产源码，没有改固定 `57a86767d12f005da08c728ff544a2b66076b2db03a203e3221d3490e355f290`、7ef 或任何既有运行域、seed 与报告。没有把新域当作已删除旧数据的恢复。

## 一、<span id="results">结果与证据</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 实际检查 | 结果 | 原始／结构化证据 |
| --- | --- | --- |
| 候选生产二进制 | `darwin/arm64`、Go 1.27.4，SHA256 `25de7285297c3773ee09cb90bf00b3e92f8c9319153befd7878cc274ea954001`；未启动 | [构建原始日志](./01-候选编译.log)、[准备摘要](./准备摘要.json) |
| 12 份外置测试目标编译 | 通过；`-run '^$'`，未执行 PG 测试 | [编译原始 JSONL](./02-测试目标编译.jsonl)、[确切命令与时间](./编译及guard摘要.json) |
| 两项纯本地连接 guard | 2／2 通过；拒绝外国端点、fallback、owner、库前缀、非私密或 symlink 凭据文件 | [原始 JSONL](./03-连接围栏纯本地.jsonl) |
| 真实 PG 定向 | 20／20 顶层组通过，0 子例、0 失败、0 跳过；每组独立建库并清理 | [逐组命令、时间和日志 SHA](./PG首次执行摘要.json)、[最终摘要](./验证摘要.json) |
| 清理与停止 | 20 个随机测试库已清理；仅余 `postgres/template0/template1`；精确核对本轮 PID、端口和数据目录后停止自有 postmaster | [SQL 核验原始输出](./24-清理与durable核验.log)、[停止原始日志](./25-仅停止自有PG.log)、[收尾摘要](./收尾摘要.json) |

初次归档准备脚本曾把生产源按顶层 `glob` 计成 67，触发输入数量断言；改为包含两个 `cmd` 源文件的递归采集后继续。该失败发生在任何 Go／SQL 测试之前，记录于[执行准备失败](./执行准备失败.json)。没有抹掉该记录，也不将其算作数据库或产品失败。

## 二、<span id="coverage">真实覆盖和迁移范围</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 组别 | PG 顶层组 | 本轮实际证明 |
| --- | ---: | --- |
| schema 迁移 | 4 | 固化57逻辑 DDL 夹具上的 `migrateSyncSchemas`；重复迁移约束身份、冲突回填事务回滚、既有 physical-store 冲突和 scope 不匹配拒绝 |
| physical-store 注册 | 2 | 直接调用生产 `syncRegister`，使用真实 PG 注册事务；S1／S2保持同一 device/store、分开 stream；竞争两种 schema 与不同 store 时仅一个提交，无孤儿 stream／registration |
| 私有偏好 | 4 | `migrations["directory"]` 与最小 instance；默认／未知查单无写入、并发版本 CAS、相同 nonce 重验、原成功事实及 epoch 重基线 |
| 消息读边界／未读汇总 | 4 | 精简消息元数据表与预置 membership 上的真实 SQL；真实 creation tuple 排序、输入顺序／可见性拒绝、大 counter、缺损元数据拒绝、精确 eligible ID 计数和洞保留 |
| 本人 read 持久化 | 3 | 最小 instance 上的 `migrateSelfRead`；乱序洞、legacy、原事实、并发观察并集 CAS 重试、epoch 和资格代际 |
| 本人 view | 3 | `migrateSelfRead`＋`migrateSelfViews`＋`migrateSyncOwner(sync)`；版本 CAS、原 nonce 查单、manualUnread 独立代际、E1/v20→E2/v0→v1，提交后最小本人 view Outbox 元数据 |

测试入口源见[连接 guard](../../../../测试/后端/接口/s2_pg_guard_test.go)、[schema 测试](../../../../测试/后端/接口/sync_schemas_test.go)、[physical-store 注册测试](../../../../测试/后端/接口/sync_register_pg_test.go)、[偏好测试](../../../../测试/后端/接口/source_preferences_test.go)、[消息边界测试](../../../../测试/后端/接口/message_read_points_test.go)、[未读汇总测试](../../../../测试/后端/接口/message_read_summary_test.go)、[read 测试](../../../../测试/后端/接口/self_read_test.go)及[view PG 测试](../../../../测试/后端/接口/self_views_pg_test.go)。这些相对路径以当前目录为基准，源内容由本目录 SHA 绑定。

**没有执行九 owner 完整启动迁移或旧二进制物理库升级。** schema 兼容基于新库里的固定57 DDL 夹具，E1→E2基于自有 SQL 更新 instance；不代表真实备份恢复或 journal 重放。read helper 未安装 sync owner 触发器，本轮没有通过写 read 表证明 `read_state/read_observation` Outbox；view Outbox 的通过不能外推到 read。

## 三、<span id="isolation">隔离、资源与运行边界</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

运行数据位于用户 Application Support 的全新 UUID namespace，唯一新 PG 监听 `127.0.0.1:62381`，TCP SCRAM 鉴权、没有全局 socket。所有子进程先移除继承的 `PG*`、`JOBS_IM_*`、`DATABASE_URL`／`JOURNAL_DATABASE_URL` 及其他 `*_DATABASE_URL`，再显式设置四项测试门禁和本轮 DSN。每个管理、子库、连接池及额外锁连接均核对本轮 endpoint、owner、数据库白名单；密码只由0600常规私密文件读取，不放 argv、日志或本目录。

`fsync`、`full_page_writes`、`synchronous_commit` 均保留 `on`；PG 最大连接12，`shared_buffers=16MB`。每步检查至少4GiB空闲，Go `GOMAXPROCS=2`、`-p 1`、顶层 `-parallel 1`，外层180秒／测试内部170秒。仅用已装工具和模块缓存，进程级 `GOTOOLCHAIN=local`、`GOPROXY=off`、`GOSUMDB=off`；没有下载或修改全局环境。[运行环境](./运行环境.json)和[overlay](./测试Overlay.json)保留当次边界与解析路径。

没有启动 jobsimd、HTTP listener 或 NATS。`https://127.0.0.1:50443` 仅为四个测试 helper 的隔离门禁标记，不是已部署或已验证地址。新 CA／SAN 只完成离线链验证，没有安装设备／系统信任，没有发送真实 HTTPS 请求。

注册测试用明确的内存 RoundTripper 返回账号夹具；selfread 的 message boundary 使用合成回调；消息 SQL 组的 membership 为预置元数据。它们不证明真实鉴权、跨 owner 权限／资格 RPC、gateway 响应围栏或公网链路。测试结束仅停止已核对的本轮 postmaster，保留新 PG 数据、私密 seed、CA 和完整初始化／DDL日志；没有备份或恢复既有域。

## 四、<span id="remaining">仍未验证和未交付项</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

双物理 PG、完整九 owner／网关启动、真实 HTTPS与资格 RPC、read Outbox、schema2 快照／变化流、NATS JetStream 提交／补偿／恢复、真实恢复后的 S2 连续性、端侧安装／持久 ACK 仍需后续独立 Phase B。草稿基础继续保持未挂迁移／入口／holder 回调，不在本轮选择或激活；没有以这些 PG 结果宣布草稿、普通迁移、完整导出或注销完成。

本轮未运行客户端、真机、Linux／Windows 原生验证；不改243主验收状态，不表示全 I 已完成，也不改变固定57的现有 schema1覆盖。

## 五、<span id="hashes">输入与归档</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

[测试前输入](./源SHA-测试前.json)及[最终源 SHA](./源SHA.json)绑定69份生产 Go、2个模块锁、12份外置测试，共83项；整个运行期间摘要未变。相较先前80项本地输入，4份原测试仅加连接 guard，新增guard／view PG／register PG三份测试；生产与模块锁全部一致。旧80项 manifest保持原样。

[SHA256SUMS](./SHA256SUMS)覆盖本目录其余归档文件。私密 seed、PG凭据／数据、CA私钥不进入仓库；当次解析的工程外绝对路径只作执行元数据，不是分发运行依赖。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
