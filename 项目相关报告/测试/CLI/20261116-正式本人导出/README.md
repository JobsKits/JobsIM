# CLI正式本人数据导出

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、入口与持久化](#implementation)
- [二、原始验证](#evidence)
- [三、新增八组与边界](#cases)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮只CLI自有源码／外置测试与本端文档，独立接正式本人导出六API，不从本地历史拼包。最终完整88顶层本地PASS／0FAIL／2项环境跳过及MacOS arm64新候选构建通过，SHA为`13cd243875f318fc8f53811866f24e912a7466291b2fb0a5caf259f15e8fe714`。网络为本地HTTP替身，未连接E2／PG、启停服务或修改后端／其它前端。前置80对象候选eb42、79／395、旧67及15a4历史保各自原件。

## 一、<span id="implementation">入口与持久化</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[CLI局部资产表](../../../../代码/前端/CLI/README.md#personal-data-export)列七命令与行为，`data_export.go`在schema4增量`data_exports`保存原创建／独立取消body与摘要、原scope／本人／personal／连接profile／physical UUID／device／epoch／session family、job关联及已筛选状态。没有票据／正文／ZIP bytes入DB；unknown先完整原单只读查，fresh或此次显式同体重试才沿原nonce创建；已成404不重建。cap撤回仍允许原查单／状态／取消，不能下载或新建。

`data_export_file.go`申请RAM短票，鉴权header下载并拒redirect；完整count／SHA、ZIP结束记录／安全路径／CRC、file及目录fsync后，重新核当前ready／artifact／build／cap和本地身份，用no-replace hardlink发布。目标已存在或竞争创建不覆盖；晚family／source撤权或坏ZIP不发布。原auth.go只新增可附短票的既有认证下载helper，旧调用保持无票；仅确定ACCESS_EXPIRED沿既有single-flight轮换。

源当前性由服务端源SH和流后精确GET负责；GET与客户端文件系统没有分布式原子提交，不保证拦住最后核查之后才发生的独立源动作。客户端完整SHA是本地摘要，服务端没有返回明文ZIP预期SHA，不冒称远端SHA比对。流及解压CRC校验合计各限1GiB，超限明确失败，不发布截断包。

普通失败只清本次已核inode临时名并fsync目录；强杀可能遗留用户选择目录内0600私有part，不自动发布／广扫。用户已获外部ZIP不承诺远程删除；这与托管附件收讫的global清理是不同责任。

## 二、<span id="evidence">原始验证</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- `01-local`保首类型失败：新增test少接一个createDataExport返回值；未执行任何测试，不算通过。
- `02-local`修test后完整88 PASS／0 FAIL／2 SKIP，[原日志](./02-local/SQLite测试.log)及逐源码SHA保留；本地真实SQLite／文件与HTTP替身，零真实API／PG。
- `03-final-local`为帮助命令接线后的最终同输入88／0／2，[原日志](./03-final-local/SQLite测试.log)和[逐输入SHA](./03-final-local/SQLite结果.json)保留。
- `04-build`为同产品源MacOS arm64、CGO=0新候选，[产物路径和SHA](./04-build/Binary.json)及[资源记录](./04-build/Process.json)独立；没有执行该候选实际联网认证。

使用既有Go1.27.4与私有warmCache，GOMAXPROCS2／`-p=1`／GOPROXY=off，独立Temp／Products；每进程180秒／4GiB资源门禁。未下载安装工具、清理缓存或修改全局环境。

## 三、<span id="cases">新增八组与边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 组 | 本地实际验证 |
| --- | --- |
| 原单先于HTTP | SQLite WAL／FULL、完整创建body与family原摘要可在创建handler检查 |
| unknown／重开 | 原库重开、404纯查不创建、明确同ID／body重试、已成404不重建 |
| cap撤回／取消 | 原单与状态仍可查，独立cancel body先耐久、未知不换，终态清完无重复取消写 |
| 身份／原体 | 换family／physical UUID／epoch／原hash／坏取消体不触达结果owner |
| ZIP成功 | 完整文件、count／SHA、fsync及无覆盖发布，短票仅header且不进DB，临时名清除 |
| 坏流／竞态 | 截断／错MIME、流时换family、源终态、目标竞争均不发布或覆盖旧文件 |
| 精确协议 | 大代际字符串、RFC3339 offset、错epoch／非规范代际和完整ZIP检查 |
| SQL失败 | 原意图插入故障时无创建／查单HTTP |

待验：严格CA实际正式导出TTY／服务源流及取消未知、真实进程强杀／断电／满盘、跨文件系统、Windows／Linux新候选和目标OS、系统安全凭据正向。当前测试不是后端加密／源holder／真实下载签名证明；未覆盖完整I或243项，不借后端2b58和Web真实矩阵代替CLI验证。

<a id="🔚" href="#前言">回到首页</a>
