# S2 本地类型编译与纯规则验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、实际结论](#results)
- [二、测试覆盖与未执行范围](#scope)
- [三、运行约束与复现输入](#controls)
- [四、输入与归档绑定](#binding)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本目录记录 2026-11-16 对 S2 同一工作树输入执行的本机 [**Go**](https://go.dev/) 类型编译和选定纯规则测试。目标为 `darwin/arm64`，工具为 Go 1.27.4。两步首轮通过，没有失败、超时或源码修正。

[原 source-only 检查点](../2026-11-16-S2本人状态-源码检查点/README.md)及其 80 项源输入保持原样；本目录追加后续限定验证，不把其当时的未验声明重写为历史通过。固定 `57a86767d12f005da08c728ff544a2b66076b2db03a203e3221d3490e355f290`、原环境和历史报告没有变动。

## 一、<span id="results">实际结论</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 检查 | 实际结果 | 边界 |
| --- | --- | --- |
| 生产包与命令类型编译 | `jobsim/backend`、`cmd/jobsimd`、`cmd/jobsimops` 通过；墙钟 2.991 秒 | `go test -run '^$' ./...` 编译本机目标；没有启动服务或生成可部署候选 |
| 外置测试编译 | 隔离 overlay 挂载 9 份 S2 测试源并通过编译 | 仅挂载到测试视图，生产目录未复制测试文件 |
| 选定本地规则 | **16 组＋11 子例通过，0 失败／0 跳过**；测试包执行 0.478 秒，含编译墙钟 4.440 秒 | 仅选定范围；另 19 个外部状态测试未选择、未执行，不将其写成跳过或通过 |
| 源输入核对 | 69 份生产源＋2 个模块锁＋9 份外置测试，共 **80 项 SHA256** 均与 source-only 输入一致 | 本轮源码及测试源码零修改 |

原始输出、确切命令／环境、测试选择和逐项状态分别见[生产编译日志](./01-生产类型编译-原始.log)、[生产编译命令](./01-生产类型编译.json)、[测试原始 JSONL](./02-本地纯规则-原始.jsonl)、[测试命令与选择](./02-本地纯规则.json)及[结构化摘要](./验证摘要.json)。没有重造原始输出，也没有不存在的失败日志。

## 二、<span id="scope">测试覆盖与未执行范围</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

- schema1／schema2 规范值、游标身份及物理 store 字段绑定、新 private kind 的协商过滤、私有指针形状。
- 偏好／视图原请求严格解析、精确大整数、显式 null 与 false、未知字段及尾随请求拒绝、个人状态路径的共享围栏分类。
- 读点的真实创建 tuple 解析、历史起点规范化、乱序洞合并及 legacy 不伪造顺序、原 body 摘要保留观察顺序。
- 未读汇总请求的精确水位与重复 ID 拒绝；没有执行 SQL 计数，也没有据此证明精确未读的真实数据库结果。
- 已有草稿基础的独立 reply proof 目的域、内容与交互版本区分、成员／安全／恢复绑定、本人密文 AAD 与逻辑重基线分离、同 tuple 拒绝强度及同 digest 保正文的纯计算规则。

未连接 [**PostgreSQL**](https://www.postgresql.org/)，未执行 SQL、迁移、真实 owner RPC、API、NATS、服务启停、备份恢复或客户端／设备测试；没有执行 WindowsOS／Linux 交叉或原生构建。草稿迁移／公开入口／holder 回调／journal 重放仍按原检查点保持未挂、未完成。纯函数通过不表示该业务闭环已实现，S2 尚无交付能力声明。

## 三、<span id="controls">运行约束与复现输入</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

两步均先检查 4GiB 可用空间，限制 `GOMAXPROCS=2`、包并行 `-p 1`、外层 180 秒和测试内部 170 秒。使用已有工具及模块缓存，进程内设置 `GOTOOLCHAIN=local`、`GOPROXY=off`、`GOSUMDB=off`；没有下载模块或修改全局环境。所有 `JOBS_IM_*` 环境变量从测试子进程移除，选定测试也没有数据库／网络入口。

[工具版本原始输出](./00-Go版本-原始.log)与[本机目标及缓存位置原始输出](./00-Go环境-原始.log)保留实际读取结果。确切选择表达式与 overlay 位置在命令元数据中；[overlay](./本地测试Overlay.json)的绝对路径只记录当次解析结果，不作为分发产物的运行依赖。重新定位仓库时按当前根目录重建映射。

本轮没有运行通用后端测试器，避免连带 CLI 或外部状态用例。后续真实验证须另建受管隔离域，本目录不提供上线或新域就绪结论。

## 四、<span id="binding">输入与归档绑定</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

[输入核验](./输入核验.json)绑定原 source-only manifest 的 SHA；[本轮源 SHA](./源SHA.json)逐项保存 80 个当前摘要及与原输入一致的结果。[验证摘要](./验证摘要.json)列出 16 个实际执行组、19 个未选择组、原始日志 SHA 和两步退出状态；[归档校验](./SHA256SUMS)覆盖本目录其余文件。

秘密 seed、数据库、实例 CA 私钥、AT／RT／Cookie、NATS token 与工程外备份不进入本目录。全部结论仅属于上述源 SHA 与本机限定运行，不外推全 I、243 主项验收或固定57之外的真实服务行为。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
