# iOS 权威快照与增量 · 验证检查点

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [前言](#%E5%89%8D%E8%A8%80)

</details>
<!-- jobs-toc:end -->

## <span id="前言">前言</span>

当前arm64应用／测试包已编译通过，18个权威同步注入方法在自有iOS Simulator全部通过（0失败／跳过），使用真实GRDB、模拟协议响应及隔离真实账户恢复的DEBUG host（实际协议请求由fixture闭包注入）。真实同步HTTPS、真机与完整I尚未验收。旧认证12＋4、转发等历史结果保持独立，不累加。

| 范围 | 当前事实 |
| --- | --- |
| 前置保护 | 上一67份Jobs自有Swift和前报告28个SHA输入核对一致；原App另存，原Products及报告未删除 |
| 源码 | Swift原envelope SHA／结构与精确计数校验、原注册／快照意图、全页stage、finalvalidate＋首次权限changes、GRDB同事务投影／origin／redacted／连续游标／只读原Outbox证明 |
| 本地持有者 | scope索引和global终态；已登记同来源旧device／retained／收藏／未读持有者清理；forward正文与临时proof不持久缓存 |
| 只读历史 | 仅同当前已核身份／来源／epoch／设备，且完整安装资格、版本及本地正文hash匹配的普通副本；不开放云附件或写操作 |
| 注入测试 | 18/18通过；完整页／原字节／缺页、分类／权限／guard、origin冲突、原体proof／同CID不确认、global／retained、opaque32hex、能力消失及公告仅存引用；均为注入协议而非真实HTTPS |
| 静态检查 | 当前69份应用／本地Pod构造审计0问题（23工厂允许命中）；70份Swift输入SHA与编译轮相符 |
| 构建与失败 | 冷依赖240秒中断及早先低盘保留；首次类型错误为实例事务未限定静态save，已改Self.save。首18项6过／12失败，源于夹具重复编码原refs导致摘要不稳定；现保存同页原字节并补同结构异字节反例，产品摘要校验未削弱 |
| 实际网络 | 本iOS轮未连接44443／未作真实API或SQL；测试仅安装启动自有927610A5模拟器，未操作其他实例 |

`DEBUG`测试进程显式设置`JOBS_IM_SYNC_INJECTED_TESTS=1`时，共享IMSession使用内存store并跳过原账户／Keychain／消息库恢复，verifyRestoreEpoch拒绝真实请求；这不是通用IMAPIClient的全局HTTP拦截。实际只选18个注入同步方法及后述12个本地认证方法，排除会读取fixture发真实请求的集成类。此标记默认关闭、不写持久偏好，真实HTTPS仍需单独的新身份及严格CA配置。

永久运行根为`~/Library/Application Support/JobsIM`。本切片外置构建在其`Build/iOS/AuthoritativeSync`，原保护点在`Build/iOS/Baselines/Auth-before-authoritative-sync-20261116`。构建日志及门禁JSON保留；不创建或清理旧Caches，不操作其他模拟器／服务。

未覆盖：不可归属旧namespace的受控归属与迁移、完整旧库global清理验收、进程／断电恢复、原生媒体或UI链、完整大快照容量、真机／iPad。旧未知缓存和来源不明备份继续隔离不显示。服务端已读、云草稿／偏好、qualifiedACK及after_receipt仍未交付／关闭。

辅助Outbox只读查单的503／超时仅产生零proof，不能阻挡已完整校验的sync／global事务；身份／epoch／namespace改变仍整体拒绝。未能与global消息事实可靠关联的原Outbox继续保留原ID／原body，不凭clientMsgId当发送确认；这部分原意图正文尚无完整global清理验收，不宣称“全旧库所有持有者已清”。

初次69份应用／本地Pod源码及17项测试的静态记录见[源码检查点](./源码检查点.json)及[源码SHA256](./源码SHA256.json)，保留当时字节含义。之后发现公告同步对象内含`message`，已在统一持久化入口剔除嵌套原文，只留消息引用和版本；新增第18项测试源码，69份应用／本地Pod Swift加测试文件共70份重新parser通过，见[公告引用补正](./公告引用补正.json)。这些原始source-only记录保留当时结论。随后修复静态save限定与夹具字节稳定性，完成当前编译及18项运行；[当前汇总及108份App文件SHA](./原生编译与18项注入验证.json)、[最后构建](./执行记录/20261116-121027-24344/构建结果.json)、[18项原始结果](./执行记录/20261116-121027-24344/测试结果.json)、[原失败输入精确补存来源](./执行记录/首轮输入补存/来源核对.json)分别列示，旧失败未改写。

[客户端说明](../../../../../代码/前端/iOS/README.md#native-authoritative-sync)、[代码地图](../../../../../代码/前端/iOS/代码地图.md)、[注入测试源码](../../../../../测试/前端/iOS/IMAuthoritativeSyncTests.swift)、[冻结后端协议](../../../../../代码/后端/Go/接口契约/README.md#authoritative-sync)。

当前同一候选App另外完成原认证本地12项回归（凭据3＋协调器9，0失败／跳过），只用随机测试Keychain项／临时库和注入请求；108份App文件SHA前后不变。详见[当前原生回归汇总与隔离范围](./当前原生回归汇总.json)和[12项原始结果](./执行记录/20261116-122209-31755/认证本地回归结果.json)。18与12为不同合同测试，不把旧4项HTTPS追认到当前候选，也不计为30项产品验收通过。

<a id="🔚" href="#前言">返回前言</a>
