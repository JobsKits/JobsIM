# 云草稿实现与真实恢复记录

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、按实际单位分列](#results)
- [二、持久与恢复事实](#safety)
- [三、运行与来源](#runtime)
- [四、剩余边界](#limits)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮新增公开草稿GET／PUT／DELETE／完整原body结果查询、独立CAS／nonce／epoch、加密prepared intent、typed无正文journal floor、prepared／active reply holder、源／成员／principal／forward祖先失效回调及续跑worker，并将draft指针纳入schema2快照／增量／JetStream。没有复制quote、没有把草稿自动发送、没有修改固定57或7ef域。

## 一、<span id="results">按实际单位分列</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 验证 | 实际结果 | 证据 |
| --- | --- | --- |
| 当前生产构建 | 75 Go源，3.409秒；编译前后SHA相同 | [命令与状态](./执行.json)、[原始构建输出](./01-候选构建.log)、[初始生产SHA](./生产输入.json) |
| 本地纯规则 | 6个顶层通过，0失败／跳过；原reply／AAD／floor规则3个，新body／scope／byte界限／journal规则3个 | [原始Go JSON](./02-纯规则.log)、[13份测试overlay](./测试Overlay.json) |
| 第一条本人文本云草稿 | 9 HTTP检查／3语义，真实GET v0→PUT→精确GET正文 | [首次HTTP](./03-首次草稿HTTP.json) |
| 六组功能 | 89 HTTP检查／114语义／6页；另1条预期CAS冲突单列 | [逐组原始命令／SHA](./首次功能执行.json)及各组原始log／结果 |
| 持久prepared中断 | 原PUT已耐久prepared，受控SIGTERM＋同域重启，worker沿原加密intent收尾；没有第二PUT，原事实唯一 | [保留宿主失败](./prepared-中断续跑.json)、[仅只读成功续验](./prepared-成功续验.json) |
| 新加密backup／offline restore | 51 HTTP／130语义／9页，295566字节加密备份，新target DB与State、独立journal重放 | [恢复结果与工具步骤](./恢复-首次结果.json)、[维护工具构建](./04-维护工具构建.json) |
| final能力标记 | 只control.go变动；新独立binary、精确target重启、bootstrap两flag为true，4 HTTP／4语义窄验 | [final构建](./final-窄构建.json)、[实际bootstrap](./final-实际Bootstrap.json)、[窄HTTP](./final-窄HTTP.json) |

六功能组分别为[basic](./basic-结果.json)、[source](./source-结果.json)、[membership](./membership-结果.json)、[forwarding](./forwarding-结果.json)、[principal](./principal-结果.json)、[sync](./sync-结果.json)。它们覆盖默认无写、原nonce异body／未知查单不新建、并发CAS、清空与旧成功不回正文、互动不废content引用、源edit／global只撤reply保ownedText、未发失效reply的显式recovery、退重入旧admission、祖先forward失效、账号安全恢复及真实S2指针投影／NATS。

原prepared宿主脚本在重启成功后调用错误的private_json模块路径，抛AttributeError；首FAIL保留。随后只按唯一完整命令核新PID，再原body只读查单和精确GET，确认后台已完成原intent、操作仅1条。没有重发PUT，没有改产品源码，也没有把宿主错误删成原输入通过。首次4GiB门禁曾在828MiB时于创建新域／编译之前拦住；该次没有运行Go或PG，空间恢复后才执行本目录。

## 二、<span id="safety">持久与恢复事实</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

原意图在本域先加密落盘，source holder prepared／promote及独立journal不跨本地PG事务持锁；正提交前核floor==pending、当前epoch与原摘要。worker不补新proof。来源内容或成员／principal操作持EX，先持久revoking和selector责任，sync确认floor后才cleared；旧release不能抢revoking。当前回复预览未知不误擦独立text，永久失效也只去引用。未发布retired的本人text经GET.recovery显式提供，不自动覆盖输入或改正式Outbox。

恢复验证是真实新备份／新target DB导入及独立journal tail：备份后clear阻旧text复活；备份后新owned digest没有对应备份cipher时明确unavailable，不退回旧A；源recall保留匹配的本人cipher但旧reply holder全部退休。部署身份保留、真实恢复epoch推进到2，mutable草稿effective版本从0起；旧未成E1 body409、旧AT401。随后新认证／新device／store注册、S2完整页与final validate、新E2文本提交及网络Outbox追平都通过。没有SQL直接改epoch冒充该恢复。

S2事件与journal均不存草稿明文；draft body只由当前本人hydration返回，不存quote／临时许可到指针。网络证明沿已有SQL观察publication／origins再取HTTP，未拿HTTP补偿替代NATS。云附件字节／上传队列仍属本地，不伪造云制品。

## 三、<span id="runtime">运行与来源</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

新域54443实际部署ID`24fb0a3c-99a0-4ada-9d01-07467724e829`，当前恢复epoch2。两物理PG沿本任务自有62381／62382，只建新草稿业务／journal／恢复target库；新CA、State、NATS55222、binary与Secrets独立，未重启旧固定域或改PhaseB数据／报告。实际身份／durable与启动步骤见[准备记录](./启动步骤.json)、[原bootstrap](./实际Bootstrap.json)。严格CA、SAN／主机名／有效期校验持续启用，没有装系统信任。持久运行目录在Application Support，备份在Documents/JobsIMBackups，recovery key另存私密Secrets，不进入备份或仓库。

初始完整矩阵binary为`e69e51d2f0620752005df9ee8656fc6ce77511ae1939d994707615b63f2e7214`；维护工具`69fb4a4a80807e5bae4cadb2cc06e02a3812f5b8fd589ee97714db4dc2af2d63`与其同源。final为`dea37a66395b0b2c503ba934bafef327d3d87b407c73a10595e770f46948a2cd`，只增加cloud_drafts配置标记，未宣称完整矩阵在final重新运行。两个[初始源码zip](./e69-固定源码.zip)／[final源码zip](./final-固定源码.zip)与[96项源SHA](./源SHA.json)逐字节绑定输入（75Go＋2锁＋13外置Go测试＋6个Python测试／helper）。

每步4GiB空闲门禁、Go GOMAXPROCS2／-p1、外层180秒、SQL30秒／HTTP25秒；仅明确CAS用例两writer。没有下载SDK或大工具。当前恢复target按根代理安排保留给原生客户端独立fixture联调；原source seed已标退休，不能错用它给E2客户端。实际运行私密seed／PGpass／ATRT／CA key／NATS token不入仓，报告只记录非秘密身份／统计及SHA。

## 四、<span id="limits">剩余边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本轮完成后端基础文本云草稿闭环，不等于五端UI／本地冲突和存储资格已经验收；各端仍须用自己的账号／device/store测试。after_receipt关闭，正式本人导出、普通迁移、注销、完整隐私矩阵和强保护继续单列。没有修改243主项状态，不把这些重叠测试计数相加成基础版完成数。

原始日志／步骤、fail与续验不覆盖；[验证摘要](./验证摘要.json)和[SHA256SUMS](./SHA256SUMS)索引本目录。接口见[契约9.6](../../../../../代码/后端/Go/接口契约/README.md#cloud-text-drafts)，测试代码见[入口说明](../../../../../测试/后端/接口/cloud_drafts.py/README.md)。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
