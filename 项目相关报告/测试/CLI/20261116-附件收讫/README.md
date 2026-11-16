# CLI完整附件收讫

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、入口与耐久边界](#implementation)
- [二、执行输入与结果](#evidence)
- [三、新增12组范围](#cases)
- [四、待验及后续接线](#limits)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮CLI自有源码／外置测试及本端文档增量：79顶层本地PASS／0FAIL，真实HTTPS和原生vault两项按范围跳过；MacOS arm64新候选构建通过。新增12对象组采用真实[**SQLite**](https://sqlite.org/)和文件fsync、HTTP替身及受控进程退出，不是实际服务签名、部署批准或物理断电证明。没有连接真实API／PG／E2、修改Go后端或启停服务；旧67报告、冻结源码及12acd7候选原件保持。

preparation修订现已独立验证完整80 PASS／0FAIL／2环境跳过及MacOS arm64候选eb42a9e1：原context持久准备、SQLite文件创建围栏、最终SQL交接、global与晚到写拒绝。06／07为新运行记录，08冻结同输入；05-source-only仍保存先前低空间门禁。原79结果fea47和395候选保持历史，不追认其未跟踪孤儿边界。

## 一、<span id="implementation">入口与耐久边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[工程README资产增量](../../../../代码/前端/CLI/README.md#object-receipts)列命令／默认／持久化／生命周期，[代码地图](../../../../代码/前端/CLI/代码地图/README.md#object-receipts)定位本轮源文件。保本地schema4／原physical UUID／原认证32hex命名空间；不新增依赖。`receive-message`接普通媒体描述，原`receive-text`仍拒附件；独立`receive-object`鉴权下载原件，`receive-object-file`校验复制本地原件，转发仍拒。

服务端优先：实时能力＋批准的`native_sqlite_full_file_fsync_v1`／objectQualified是门禁，旧`native_sqlite_full_v1`及缺字段不默认升级。父message_content必须同device／store／epoch／qualification已recorded，ro1核tuple、期限、对象ID及精确字符串size／SHA；客户端只核一致性，不自称验证后端签名。

39506f前置候选为全文件count／SHA及file／目录fsync后，同WAL／FULL事务落描述和原receipt_outbox；原nonce／body／ro1／资格不换，无正文／文件bytes复制到outbox。其文件→SQL之间未跟踪的孤儿边界已在静审确认不足，因此不能作为当前完整生命周期交付。新source-only修订先FULL保存preparation原身份／对象／内容元数据，创建文件与取消共享SQLite写围栏；完整文件后最终事务落描述＋ACK意图并删preparation，错误／崩溃按原路径排清理，不猜ACK、不开广扫。原用户导入文件不移动／删除。

纯查object-receipt-results先行；404仅fresh或用户明确retry、仍pending、原文件／资格／期限／当前身份有效才原体write一次。文件缺失／改写／替换／symlink不重发收据，仍可查历史已成事实；历史recorded不保证后来文件当前可用，`receipt-file-status`分别显示。未知及普通retained不删；global同事务留清理意图／冻结pending，启动和同步精确drain私有holder、目录fsync，失败保责任，跨可信旧库复用原scope核验。

`objectComplete:true`限独立对象已成事实；targetRangeComplete／deletionEligible／after_receipt均false，不确认正式消息Outbox、累计位置、已阅或云端唯一副本删除。用户外部显式下载不承诺远程擦除。

## 二、<span id="evidence">执行输入与结果</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- `01-targeted`：23顶层PASS／0FAIL，筛选TestObjectReceipt和TestReceipt前缀（旧Storage前缀组随后全量覆盖）。这是新增retained／真实过期子例前的独立输入，不追认后续源。
- `02-full`：78 PASS／1 FAIL／2 SKIP，保[首轮原日志](./02-full/SQLite测试.log)及其输入SHA。失败为新夹具在原text同ID／同version下安装file描述，却保留旧text的eligibility摘要；普通本地历史正确拒绝该不匹配内容。修夹具经authoritative projection安装同一file DTO，产品门禁未放宽。
- `03-final-full`：79 PASS／0 FAIL／2 SKIP；[完整原日志](./03-final-full/SQLite测试.log)、[运行记录](./03-final-full/Process.json)和逐源码SHA独立。
- `04-build`：MacOS arm64、CGO=0候选；[路径／SHA／资源记录](./04-build/Process.json)。本轮没有执行该候选实际网络认证。

Go1.27.4，GOMAXPROCS2／`-p=1`／GOPROXY=off／GOTOOLCHAIN=local，复用已存在私有warm cache，独立Temp／Products／日志；每步180秒和空闲4GiB硬门禁，未触发。任务初读磁盘曾低于4GiB，当时仅小源，恢复后才测试。未清理全局缓存、下载安装依赖或扩全局环境。

## 三、<span id="cases">新增12组范围</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 组 | 验证点 |
| --- | --- |
| 全文件先于HTTP | count／SHA／file及目录fsync、descriptor和原receipt可从SQL检查；正式消息Outbox不ACK |
| 未知与重开 | 503保pending，纯查404无write，显式沿原body／nonce／路径重试；completed不重建 |
| count／SHA与原件 | 同长错误、短缺、额外字节拒绝；正确导入保原文件字节 |
| SQL与崩溃点 | SQL触发器令描述／意图一起回滚，不HTTP；文件耐久到SQL前孤儿不升ACK；实际子进程exit91后原SQLite／文件可重开核完整原意图 |
| 文件生命周期 | 同长替换、symlink、目录替换、缺失保原请求且不重发 |
| 合同／能力 | 旧正文合同、缺新字段、伪objectQualified、缺flag、不同本地构建拒新挑战／下载 |
| 父正文事实 | pending／坏result／换qualification／epoch／physical拒对象收据 |
| ro1上下文 | 错对象／size／digest／parent／epoch／内容版本、过期、错误proof域、target越界拒绝 |
| 过期与历史事实 | 原pending实际跨deadline后冻结原body；缺文件／批准撤回仍能纯查历史recorded，completed404不新建 |
| retained／global | unknown保普通合法本地副本与文件；global后pending冻结、SQL清理意图重开续删，原用户文件保留 |
| 元数据／坏200 | descriptor／payload hash变化、缺完整结果和503不完成或write |
| forward／不完整媒体 | forward投影和无原objectId的媒体描述不计正文收讫 |

## 四、<span id="limits">待验及后续接线</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

待验：严格CA真实对象服务／批准策略与后端签名、真实下载取消／响应丢失、跨设备托管对象清理实链、MacOS安全库正向、Linux／Windows新源编译与目标系统文件耐久、设备断电／满盘硬件故障。39506f前置输入的fsync为实际系统调用成功，不能替代文件系统／硬件验证；其未跟踪孤儿边界是当前修订原因，新源尚未实跑。受控退出与SQL注入不是物理断电。

冻结目标和完整累计ACK／retention worker后续复用原parent和对象receipt元数据、完整本地描述与结果验证；不能把对象flag或既有sync cursor升级为targetRangeComplete／deletionEligible。需要后端新冻结范围合同与独立验收；默认after_receipt关闭。



<a id="🔚" href="#前言">回到首页</a>
