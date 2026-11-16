# Go 后端第三方依赖与许可来源

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [前言](#jobs-toc-heading-1)
- [用途与保留](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">前言</span><!-- jobs-toc:heading-end -->

核验日期：2026-11-15。按 [go.mod](../go.mod) 的直接依赖块计数，后端为 **5 个直接依赖**，另列7个传递依赖；不把工具链、测试模块及CLI工程依赖混入后端直接依赖数。锁文件 [go.sum](../go.sum) 已在工作树，当前任务未创建提交。

本清单读取经Go模块校验的指定版本LICENSE原文，随工程按字节保留版权与许可文本；不修改第三方源码。版本表示本次采用，不代表以后自动最新或已通过漏洞数据库审计。

| 模块 | 锁定版本 | 来源 | 层级 | 许可证 | 依据 |
| --- | --- | --- | --- | --- | --- |
| `github.com/jackc/pgx/v5` | `v5.13.0` | 公有 | 直接 | MIT | [上游原文](https://github.com/jackc/pgx/blob/v5.13.0/LICENSE)／[随工程保留原文](./许可证/pgx.txt) |
| `golang.org/x/crypto` | `v0.59.0` | 公有 | 直接 | BSD-3-Clause | [上游原文](https://github.com/golang/crypto/blob/v0.59.0/LICENSE)／[随工程保留原文](./许可证/x-crypto.txt) |
| `golang.org/x/sys` | `v0.50.0` | 公有 | 直接 | BSD-3-Clause | [上游原文](https://github.com/golang/sys/blob/v0.50.0/LICENSE)／[随工程保留原文](./许可证/x-sys.txt) |
| `golang.org/x/text` | `v0.44.0` | 公有 | 直接 | BSD-3-Clause | [上游原文](https://github.com/golang/text/blob/v0.44.0/LICENSE)／[随工程保留原文](./许可证/x-text.txt) |
| `github.com/jackc/pgpassfile` | `v1.0.0` | 公有 | 传递 | MIT | [上游原文](https://github.com/jackc/pgpassfile/blob/v1.0.0/LICENSE)／[随工程保留原文](./许可证/pgpassfile.txt) |
| `github.com/jackc/pgservicefile` | `v0.0.0-20240606120523-5a60cdf6a761` | 公有 | 传递 | MIT | [上游原文](https://github.com/jackc/pgservicefile/blob/5a60cdf6a761/LICENSE)／[随工程保留原文](./许可证/pgservicefile.txt) |
| `github.com/jackc/puddle/v2` | `v2.2.3` | 公有 | 传递 | MIT | [上游原文](https://github.com/jackc/puddle/blob/v2.2.3/LICENSE)／[随工程保留原文](./许可证/puddle.txt) |
| `golang.org/x/sync` | `v0.25.0` | 公有 | 传递 | BSD-3-Clause | [上游原文](https://github.com/golang/sync/blob/v0.25.0/LICENSE)／[随工程保留原文](./许可证/x-sync.txt) |
| `github.com/nats-io/nats.go` | `v1.55.0` | 公有 | 直接 | Apache-2.0 | [上游原文](https://github.com/nats-io/nats.go/blob/v1.55.0/LICENSE)／[随工程保留原文](./许可证/nats-go.txt) |
| `github.com/nats-io/nkeys` | `v0.4.16` | 公有 | 传递 | Apache-2.0 | [上游原文](https://github.com/nats-io/nkeys/blob/v0.4.16/LICENSE)／[随工程保留原文](./许可证/nkeys.txt) |
| `github.com/nats-io/nuid` | `v1.0.1` | 公有 | 传递 | Apache-2.0 | [上游原文](https://github.com/nats-io/nuid/blob/v1.0.1/LICENSE)／[随工程保留原文](./许可证/nuid.txt) |
| `github.com/klauspost/compress` | `v1.20.0` | 公有 | 传递 | BSD-3-Clause | [上游原文](https://github.com/klauspost/compress/blob/v1.20.0/LICENSE)／[随工程保留原文](./许可证/compress.txt) |

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">用途与保留</span><!-- jobs-toc:heading-end -->

pgx及其传递组件用于真实PostgreSQL；x/crypto用于Argon2id，x/text用于密码NFC，x/sys直接用于Windows导出文件锁，x/sync是传递同步组件。全部仍有实际依赖；没有私有模块地址。

[**NATS**](https://nats.io/)客户端用于自托管JetStream指针事件发布／消费；nkeys／nuid及WebSocket传递压缩实现随模块编入，不表示已启用外部渠道、浏览器直连总线或SDK产品化。指定模块的压缩包来自Go官方代理，解包后的模块与go.mod两种h1分别和官方校验数据库比对，记录见[模块校验](./NATS模块校验.json)；没有更改全局代理或Go配置。nats-server是独立部署工具，不计业务直接模块数；采用[官方v2.15.3发行包](https://github.com/nats-io/nats-server/releases/tag/v2.15.3)及同发行SHA256SUMS，工具保留在工程外，服务端许可随其发行包保留。

逐文件摘要及来源记录见 [许可证来源清单](./许可证来源.json)。Go标准库／语言运行时的许可随 [Go发行](https://go.dev/LICENSE) 核对；PostgreSQL是独立部署软件，按其[官方许可](https://www.postgresql.org/about/licence/)及原发行包保留文件，不把数据库工具链计为业务API集成。CLI的pure-Go SQLite等依赖由CLI工程单独维护。
