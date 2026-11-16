# 权威同步底座验证范围

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、证据与实际结论](#evidence)
- [二、范围、保存与限制](#boundaries)
- [三、执行入口](#reproduce)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本目录只记录固定后端候选`57a86767d12f005da08c728ff544a2b66076b2db03a203e3221d3490e355f290`的同步底座证据。当前覆盖有明确范围的快照／增量和自托管[**NATS JetStream**](https://docs.nats.io/nats-concepts/jetstream)指针回放，不等于全账户数据域、客户端耐久资格或243主项验收。

## 一、<span id="evidence">证据与实际结论</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 证据 | 通过范围 | 不能外推 |
| --- | --- | --- |
| [真实HTTPS](./真实HTTPS.json) | 100项HTTP／1282语义／32页；511消息跨公共history100和owner500界限，原字节refs／body／event摘要，全manifest／validate，新append衔接、权限ABA与历史合法global墓碑；真实停NATS、HTTP补偿、同存储重连后不多占用户游标 | 510行显式SQL批量只用于完整分页，不作为发送API接受证据；不是客户端存储资格或真实已阅 |
| [Go原始输出](./Go定向.log)／[Go范围摘要](./Go定向.json) | 7组＋9子项，0失败／0跳过。独立database的owner共同cut变动拒绝、真[**PostgreSQL**](https://www.postgresql.org/)事务并发／回滚／重复迁移、补偿中段缺行拒绝、metadata／refs密文与AAD、精确大整数、MacOS私有文件权限 | 业务owner的不同database位于同一业务集群；不声称不同物理业务集群／跨主机。NATSStore函数的真PG去重不算网络发布证据 |
| [真实恢复](./真实恢复.json)／[原始执行与续验](./真实恢复.log) | 独立新域的实际备份／同原操作双次restore确认E1→E2；成功续验22次HTTP／56语义／2页，旧AT／边界／页拒旧、原创建tuple保留、新refs重基线、postbackup global与缺新编辑正文占位、新delta；原E1 wire真实网络PubAck非duplicate，消费后不入target、不加E2位置 | 原日志明确保留包装探针FAIL＋RESUME，成功续验计数不含此前步骤、不伪称单次无中断。一个业务DB→同集群新DB＋另物理集群journal，同持久broker；不是空JS重建或多业务DB聚合恢复 |
| [构建](./构建.json) | [**Go**](https://go.dev/)1.27.4、darwin/arm64构建与vet，WindowsOS／Linux amd64交叉编译 | 不等于目标系统原生服务、WindowsOS真实DACL或Linux文件权限已执行 |
| [源码摘要](./源码摘要.json)／[固定源码归档](./固定源码.zip) | 56个自有Go源文件＋归档内go.mod／go.sum，绑定当前固定实现，后续工作树变化不能追认本报告 | 不含运行密钥、DB、CA私钥、Cookie／AT／RT、私密seed或第三方源码 |

真实HTTPS报告由脚本当次写入，是断言摘要，不冒充完整网络抓包。Go日志是当次stdout／stderr原始重定向，不由摘要重造。重复运行及不同证据有重叠，计数不能相加为产品验收项。固定源归档不代表创建Git提交或推送。

## 二、<span id="boundaries">范围、保存与限制</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

服务端只覆盖本人账号／设备元数据／本人profile、本人联系人／私人关系／申请、当前会话与成员ID、当前可读消息／互动／公告、附件引用及历史合法global墓碑。read、云草稿、偏好、完整peer资料流、附件字节耐久确认、普通迁移、正式导出与注销继续I待办。没有qualified ACK，after_receipt始终false。

事件／Outbox／总线和快照页只持引用；正文动态当前鉴权，refs／event固定digest与bodyDigest分开。页校验或源版本失败保留旧库／staging、不推进边界；final validate仅允许端侧尝试本地原子安装，不是本地提交证明。返回到客户端commit仍有权限变化窗口，客户端须按捕获上下文及第一次changes重验，不能凭当前候选后端通过便宣称任一前端已合格。

基本部署限定同主机规范化StateDir，角色9个owner加gateway；NATS文件单副本不提供强保护RPO0。总线可由持久owner回放补偿；本轮真实重连及应用恢复使用原JetStream存储，不声称总线存储全失恢复。现有备份运维支持一个业务database内的owner schema加另一个物理集群journal，多业务database聚合备份仍未实现。同步的实际恢复由本目录独立报告证明，不借历史刷新／转发报告追认。

运行数据／State／密钥在工程外Application Support的JobsIM持久目录，备份在Documents的JobsIMBackups；它们不随公开报告打包，旧Caches已清空的数据没有被重建或冒称恢复。密钥与备份另置，独立journal归档只做TOC验证时明确记录未实际恢复；不能把一个通过的基本恢复步骤写成全资产灾备。

## 三、<span id="reproduce">执行入口</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

接口字段／安装顺序见[同步契约](../../../../代码/后端/Go/接口契约/README.md#authoritative-sync)，运行及自托管依赖见[后端说明](../../../../代码/后端/Go/README.md)，脚本与副作用见[HTTPS测试说明](../../../../测试/后端/接口/sync_snapshot.py/README.md)。Go摘要保留实际overlay与命令；私密seed须由执行者显式选择，不把本机路径固化为分发运行依赖。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
