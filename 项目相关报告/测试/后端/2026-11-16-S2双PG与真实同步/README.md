# S2 双物理 PG 与真实同步验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、逐组结果与原始失败](#results)
- [二、真实事件链路与权限边界](#proof)
- [三、运行身份、隔离和资源](#realm)
- [四、源与撤收](#provenance)
- [五、未验证与交付边界](#remaining)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本目录记录 2026-11-16 的 S2 Phase B 后端定向验证。使用本任务新私有域中的两个真实物理 [**PostgreSQL**](https://www.postgresql.org/) 18.7 cluster、自托管 [**NATS JetStream**](https://docs.nats.io/nats-concepts/jetstream) 2.15.3 和真实 CA 验证的 HTTPS 候选服务。**8 组通过，125 项 JSON HTTP 检查／334 项语义断言／17 个校验页。** 另有3条真实 socket 提交后零读取丢响应写、1条 CAS 预期冲突、10条健康检查，按原单位分列，不累加为产品验收数。

候选 SHA256 为 `25de7285297c3773ee09cb90bf00b3e92f8c9319153befd7878cc274ea954001`，沿[Phase A 的实际构建](../2026-11-16-S2真PG定向/README.md)复制为独立路径；本轮未重新编译、未修正69份生产源。固定57／7ef及其他旧域没有升级或启停；没有把新域当作旧 Caches 数据恢复。全部本轮进程已按精确身份关闭，DB／State／JetStream store和私密日志保留。

## 一、<span id="results">逐组结果与原始失败</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 组 | 通过范围 | JSON HTTP 检查 | 语义 | 校验页 | 实际结果 |
| --- | --- | ---: | ---: | ---: | --- |
| A | 严格HTTPS bootstrap、两个物理PG身份、真实认证及第二设备、网络消费 | 8 | 5 | 0 | [结果](./A-真实结果.json)／[原始输出](./A-原始.log) |
| B | S1／S2注册保持物理store、不同stream、空库拒绝、原coverage过滤及虚拟默认页 | 23 | 69 | 5 | [修测试路径后结果](./B-重跑1-真实结果.json)／[原始输出](./B-重跑1-原始.log) |
| C | 真消息创建tuple／ownerRPC、乱序读洞及压缩、并发观察合并、精确当前未读、陌生资格拒绝 | 22 | 7 | 0 | [结果](./C-真实结果.json)／[原始输出](./C-原始.log) |
| D | view CAS、manualUnread独立意图、偏好原body证明和旧成功事实不回滚 | 12 | 6 | 0 | [结果](./D-真实结果.json)／[原始输出](./D-原始.log) |
| E | 真trigger→Outbox→PubAck→网络origin、稀疏观察removed事件、无源循环、S1过滤／S2增量 | 4 | 87 | 0 | [结果](./E-真实结果.json)／[原始输出](./E-原始.log) |
| F | 多页envelope、read投影不造cut外计数、final validate、原boundary接续、偏好版本废旧快照 | 18 | 69 | 7 | [结果](./F-真实结果.json)／[原始输出](./F-原始.log) |
| G | 真实退群重入ABA、保留metadata不授云权、原代际意图拒绝、历史合法global墓碑 | 18 | 32 | 2 | [结果](./G-真实结果.json)／[原始输出](./G-原始.log) |
| H | read／view／prefs三条实际丢成功响应后仅原body查单；真实NATS停止补偿和同store重启去重 | 20 | 59 | 3 | [结果](./H-真实结果.json)／[原始输出](./H-原始.log) |

[首次执行记录](./首次执行.json)保留 A 和 B 原尝试。B 原测试误请求不存在的 `/api/sync/status`，得到真实404；[失败结果](./B-真实结果.json)、[失败原始输出](./B-原始.log)及[原失败输入字节镜像](./失败输入-测试脚本.py)保留。仅把两处测试路径改为已有 `/api/sync/context`，记录[测试修补前后SHA](./测试修补-01.json)；[后续执行](./后续执行-01.json)再从 B 验至 H。原B尝试的11项HTTP／7项语义不并入最终通过行，也不改写为原输入通过。

资源归档宿主脚本随后有一次 `timezone` 未导入的 NameError，发生在任何撤收信号之前；[归档失败说明](./资源归档失败.json)保留该事实。修宿主import后实际资源读取及撤收完成。两类失败都没有产品源码修正，不能把宿主错误或错误路由用例说成产品失败，也不能隐去原记录。

## 二、<span id="proof">真实事件链路与权限边界</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

C／D的本人状态HTTP变更之后，测试先通过只读SQL等待各owner的 `sync_clock==sync_publication`，并检查所有已提交Outbox的origin已在sync.origins中。这个等待期间不调用sync/context、snapshot或changes；所以不能由HTTP回放补偿掩盖JetStream发布／消费缺失。实际read_state／read_observation／conversation_view及directory.preferences触发器均有提交指针，已完成prefix压缩还产生read_observation removed事件。导入和分配后的源clock稳定，没有origins／events回灌源表。

随后才从原S2边界读取changes，校验原event envelope与动态body envelope；S1同服务兼容流继续过滤所有新增private kind。H另精确停止本域NATS，真实偏好写提交后publication落后；受鉴权owner HTTP回放仍给出变化。同store重启broker并实际PubAck／消费追平后，同一origin不增加用户position。这里没有断开旧域broker，没有删除JS存储，没有宣称broker存储全丢恢复或强保护RPO0。

read资格与创建tuple都走本轮实际认证及conversation/message owner RPC，不沿Phase A的合成边界。离群后公共read仍403，S2只返回保留metadata的writable=false；重入后的未成旧read意图409 QUALIFICATION_CHANGED，旧manifest权限代际不可复活。G未专门保留一个跨退重入仍存在的稀疏观察来验证currentlyApplicable字段，不能把该具体分支写成本轮已验。

H在三个真实TLS socket完整发送原请求，并用本域SQL确认操作提交，再在**没有读取任何响应字节**时关闭连接；随后只发对应POST结果查询原body，未发第二次创建型写。未知查单404不创建记录。该三次原始写与D的预期CAS冲突未计入125项JSON成功检查，单独列示。

## 三、<span id="realm">运行身份、隔离和资源</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

- 实际URL为 `https://127.0.0.1:50443`；部署ID `9418b928-df8e-4e9f-9ad5-4cdc39ce98b3`、epoch `1` 从[真实bootstrap](./实际Bootstrap.json)取得，未从seed猜身份。
- 主PG62381复用本任务新建且已停的Phase A空cluster，只创建全新业务DB；独立journal PG62382使用全新initdb／身份／数据库。两边system_identifier和durable实测见[启动步骤](./启动步骤.json)及[最终摘要](./验证摘要.json)，确认不同物理cluster，不用同cluster第二DB替代。
- PhaseA environment／plan先做0600独立快照并核SHA，原文件逐字节未改。新State、对象、marker、NATS store／token及journal数据均在工程外持久Application Support本轮PhaseB子目录。[来源索引](./来源与私密日志索引.json)绑定快照、候选和私密原始日志SHA；私密文件本体不进入仓库。
- 新CA沿本域Phase A生成的CA，使用 `ssl.create_default_context(cafile=...)`，继续校主机名／有效期和SAN；CA SHA见摘要。没有安装系统信任、关闭TLS或复用fixed57／7ef的密钥。内部HTTP角色只回环，持同一规范化StateDir的认证与围栏证明；不据此宣称跨主机mTLS。
- App的运行与实际SQL子进程清除继承PG／JobsIM数据库导流项，显式绑定两端127.0.0.1、62381／62382、新owner及随机数据库白名单。秘密只读0600文件，AT／RT／密码／NATS token／CA key不放argv或报告。NATS配置／token也均为本轮私密文件，启动argv只有配置路径。

[实际资源快照](./实际资源.json)为结束前实读，**不是峰值**：

| 项目 | 配置／理论上限 | 结束前实际快照 |
| --- | --- | --- |
| App | `--service=all`，一个OS进程、9owner＋gateway共10listeners | 一进程RSS 42352KiB；10个角色health分别通过，内部角色为回环HTTP |
| 主PG | max_connections80；8个App池×MaxConns8＝理论64；shared_buffers16MiB | 13个App连接＋1维护；PG进程树RSS求和296768KiB |
| journal PG | max_connections16；1池理论8；shared_buffers16MiB | 1个App连接＋1维护；PG进程树RSS求和79792KiB |
| NATS | loopback51222、mem8MiB／file512MiB；产品stream单副本7天／256MiB | RSS22576KiB，实际停止／同存储重启 |

PG树RSS求和包含共享页重复，不能当作独占物理内存。配置的80／16也不是实际常驻连接数。fsync／full_page_writes／synchronous_commit两边均on；单组外层180秒，SQL30秒、HTTP25秒，只有明确CAS用例内两writer；每组前及关键启动检查4GiB空闲。未新增工具下载或全局环境修改，也没有新增前端构建。

## 四、<span id="provenance">源与撤收</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

[源SHA](./源SHA.json)绑定69份生产Go、2个模块锁、4份实际Python测试／借用helper，共75项；[固定源码zip](./固定源码.zip)保存相同75个输入。生产／锁与Phase A构建输入相等，候选二进制在本轮为独立路径硬链接，未悄悄重编或覆盖旧候选。失败输入镜像的哈希精确等于首轮测试SHA，与最终通过输入分开。

按[撤收摘要](./撤收摘要.json)，先核精确PID及完整命令停止App，再核NATS当前重启后的PID／binary／config停止broker；随后核各postmaster的PID／datadir／port，按journal→主PG顺序fast stop。没有按名称pkill、没有强杀、没有清库或删数据；[journal停止原始日志](./撤收-journalPG.log)和[主PG停止原始日志](./撤收-mainPG.log)保留。运行DB／State／JS store及私密原始日志均保留，未备份或操作其他域。

## 五、<span id="remaining">未验证与交付边界</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

本轮不是旧二进制schema迁移或真实S2备份恢复／journal重放。S1兼容指本候选按原schema1协议工作，不追认旧服务已重新运行本矩阵。没有大counter SQL夹具，没有端侧S2接线／快照原子commit、真实设备存储资格或qualified ACK验证。

云草稿迁移／公开入口／holder仍未挂；正式导出、普通迁移、注销、完整peer资料／公开已读开关、完整偏好矩阵、跨主机原子性和强保护继续未完成。字号／view后端metadata通过不表示各端通知、排序、静音或字号已应用。`after_receipt`仍false，不改243主验收状态，不宣称完整I交付。

[契约9.5](../../../../代码/后端/Go/接口契约/README.md#sync-self-state)与[测试入口说明](../../../../测试/后端/接口/self_state_sync.py/README.md)提供字段和复现边界。[SHA256SUMS](./SHA256SUMS)覆盖本目录其余文件；原始FAIL、修补、结果、资源和撤收记录均保留。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
