# JobsIM CLI 测试报告

![Jobs出品，必属精品](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、归档清单](#jobs-toc-heading-2)
- [二、校验边界](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本页只提供归档导航。当前功能与实际验证范围统一见[资产盘点](../../资产盘点/README.md)；历史失败、中间轮与跳过均保留，不跨构建合并计数。原执行入口保留在 `测试/`，新输出默认直接写这里。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、归档清单 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 名称 | 类型 |
| --- | --- |
| [S2本人状态](./20261116-S2本人状态/README.md) | 历史15a4候选55本地／13真实TTY与HTTPS；当前收讫新源另候选，不追认 |
| [正文收讫](./20261116-正文收讫/README.md) | 新12acd7候选67本地／0失败／2范围跳过；正文与receipt_outbox同SQLite事务，未真实API／资格批准或累计ACK |
| [同步编译／45本地](./20261116-权威同步编译回归/20261116-113743-192d48/README.md) | 候选Mac构建、45 PASS／2显式SKIP；首失败与修补保留 |
| [同步真实PTY／HTTPS](./20261116-权威同步HTTPS/20261116-120401-3cbd4e/README.md) | 固定57严格CA，10项通过；process-only，非OS库正向 |
| [同步Linux／Windows交叉编译](./20261116-权威同步交叉编译/20261116-123709-093a23/README.md) | amd64静态产物；未目标系统运行／vault或网络 |
| [20261116-权威同步](./20261116-权威同步/README.md) | 初始source-only原件；后续编译／45本地及真实专项另列，不追认原SHA |
| [20261115-CLI治理](./20261115-CLI治理/README.md) | 分轮目录 |
| [20261115-刷新家族](./20261115-刷新家族/README.md) | 分轮目录 |
| [20261115-搜索协作](./20261115-搜索协作/README.md) | 分轮目录 |
| [20261115-普通转发](./20261115-普通转发/README.md) | 分轮目录 |
| [ForwardHTTPS结果.json](./ForwardHTTPS结果.json) | 原始证据 |
| [HTTPS结果.json](./HTTPS结果.json) | 原始证据 |
| [SQLite测试.log](./SQLite测试.log) | 原始证据 |
| [SQLite结果.json](./SQLite结果.json) | 原始证据 |

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、校验边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

原 JSON／PNG／log／旧 SHA 字节保留。迁移可能仅改 README 的相对链接；原导航与原 SHA 已另存[迁移归档](../../迁移归档/20261115-集中报告/README.md)。检查历史全文清单时必须使用原导航副本，新导航由迁移新清单独立核验。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
