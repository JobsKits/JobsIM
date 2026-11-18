# JobsIM [**MacOS**](https://www.apple.com/macos/) 双PG长路径限定验证

![Jobs出品，必属精品](../../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 🔥 <span id="前言">前言</span>

2026-11-16 在本机对 [**PostgreSQL**](https://www.postgresql.org/) 18.7（Postgres.app）执行真实隔离专项。被测源码为[部署运行](../../../../../部署/MacOS/部署运行.command/部署运行.command)与[停止备份](../../../../../部署/MacOS/停止备份.command/停止备份.command)，摘要见[源码SHA256](./源码SHA256.txt)。既有用户实例只读检查日志，没有启停、迁移或清理。

## 一、<span id="results">范围与结果</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

从两份正式入口加载 `cleanup()` 之前的真实函数定义，不执行正式 `main()`；在用户 Application Support 的 `JobsIM/DeploymentTests/PG-path.*` 唯一新目录中创建合成测试数据，动态选择两个空闲回环端口。私密测试数据保留在报告结果记录的位置，数据库进程均已停止；报告不包含口令或密钥。

| 检查项 | 实际结果 |
| --- | --- |
| 中文、空格、长实例目录 | 旧式 journal socket 路径为189字节，双物理集群均启动成功 |
| 连接配置 | 两集群 SQL 读回 `listen_addresses=127.0.0.1`、`unix_socket_directories` 为空；连接地址为回环 TCP |
| 集群隔离 | 两个 `pg_controldata` 读回并登记收据的物理系统标识不同 |
| 独立副本重启 | 部署运行副本停止后，通过停止备份副本重启；原集群标识、操作员密码摘要和两库测试行保持 |
| 收尾 | 两集群 `pg_ctl status` 均已停止，两个测试端口无监听；原数据保留 |
| 既有 unit | MacOS／Linux语法、check／plan无state写入、陌生非空state拒绝接管通过 |

执行输出见[结果](./结果.txt)、[生命周期](./生命周期.log)、[业务集群日志](./pg-Main.log)与[journal集群日志](./pg-Journal.log)。本轮只验证集群函数与已有只读单元检查，没有执行应用构建、NATS、聊天、加密备份或恢复演练；不代表用户既有实例完整部署已验收。

## 二、<span id="diagnosis">原失败依据与当前行为</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

原实例 `pg-Journal.log` 明确报告 socket 路径过长（104字节，上限103字节），随后 `FATAL: could not create any Unix-domain sockets`；Main 路径为101字节，先启动后由失败清理正常停止。

两份 MacOS 实现通过 `pg_ctl -o` 传入 `-k ''`，禁用 Unix socket；所有管理、应用与备份客户端继续显式连接 `127.0.0.1`。数据库路径、集群收据与密钥保持原布局。Linux 实现仍有同类 socket 路径模式，本轮只做源码检查和语法验证，没有 Linux 实机长路径证据；Windows 实现未使用此 Unix socket 参数，两平台未修改。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
