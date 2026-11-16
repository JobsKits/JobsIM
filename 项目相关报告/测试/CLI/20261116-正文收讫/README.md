# CLI独立正文收讫

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、实现与入口](#implementation)
- [二、执行证据](#evidence)
- [三、新增12组与待验](#cases)
- [四、后续对象及目标接线](#extension)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮仅CLI自有源码／外置测试及本端文档。新增12组实际SQLite＋注入HTTP反例，当前完整67顶层PASS／0FAIL／2项HTTPS／OS安全库按范围跳过；MacOS arm64新候选构建通过。没有连接E2／真实收讫服务、设置真实部署批准profile、修改Go后端或启停PG／NATS／App。旧15a4候选和55＋13真实历史不覆盖，新源不能借旧HTTPS通过。

## 一、<span id="implementation">实现与入口</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[工程README局部资产表](../../../../代码/前端/CLI/README.md#content-receipts)列入口／行为／默认／持久化／生命周期；[receipt.go](../../../../代码/前端/CLI/cmd/jobsim/receipt.go)实现能力、存储批准原声明、纯文本challenge及原单续跑。沿本地schema4与原physical UUID／current device／epoch，新增`storage_declarations`和`receipt_outbox`，不迁移认证32hex绑定或正式Outbox。

`receive-text`核原bodyEnvelope／SHA、完整DTO及rc1原tuple后，同SQLite事务装正文和收据原nonce／body／hash，再请求原结果和首写。rc1在客户端只解码核一致性，不自称验证服务端HMAC。receipt_outbox仅含正文事实tuple与原收据proof，不存第二份聊天正文；global清正文后仍可只读查历史已成收据。纯查404、503及坏200均不当作确认；显式retry只沿原body，不换qualification／proof／epoch。已完成404不新建，过期未成冻结但仍可查询原事实。

部署存储批准、CLI本地可执行文件SHA和WAL／FULL检查均不是运行时认证、设备／文件系统断电证明或OS凭据库正向。当前普通文本独立收据，转发与附件拒绝，不确认冻结目标范围；`objectComplete`／`targetRangeComplete`／`deletionEligible`都须false，`after_receipt`关闭。正文收据不推进累计ACK、不等于已读、不确认消息Outbox或允许删除云端唯一副本。

## 二、<span id="evidence">执行证据</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- `01-local`首轮67／0／2，随后补原method／route结构核验、运行本地构建SHA匹配、live WAL／FULL和声明current一致性，首日志保留，不追认后续输入。
- `02-final-local`最终67／0／2，含新增12顶层组；[原日志](./02-final-local/SQLite测试.log)和[逐输入SHA](./02-final-local/SQLite结果.json)绑定最终产品Go输入。
- `03-build`最终同Go输入MacOS arm64构建，候选SHA256为`12acd7dd54f4f7c3b17852f5be9c58b7f81eefd6ed6e8ecac9d6924ee556416b`；产物在AppSupport独立Products，路径与资源记录见[构建记录](./03-build/Process.json)。未执行该候选联网认证或收讫。

Go1.27.4，GOMAXPROCS2／`-p=1`／CGO关闭／GOPROXY=off，复用已有AppSupport私有Go Cache；每进程180秒与4GiB空闲硬门禁均未触发。没有下载安装依赖／修改全局环境。测试的暂存库和故障HTTP只在本轮进程内；没有将注入rc1当作实际服务签名证明。

## 三、<span id="cases">新增12组与待验</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 组 | 实际验证 |
| --- | --- |
| 正文／意图先于HTTP | 提交前从另一SQL读点核正文与原意图，正式发送Outbox仍pending |
| SQL故障 | receipt_outbox触发器失败，正文和原意图一起回滚，0个创建receipt POST |
| 未知与重开 | 原SQLite重开，自动404纯查不写，显式重投原body／nonce，完成再查询不重复写 |
| 原挑战完整性 | 错device／epoch／qualification／digest／期限／签名形状、缺内容版本、global、forward／附件拒绝 |
| 能力／批准变化 | flag关闭、不同本地binary或qualification不得补绑旧意图 |
| global／guard | 终态正文不可复活，旧guard不能迟到装正文与收据；旧意图不写新receipt |
| 过期 | 实际跨原challenge截止，先纯查再冻结，原body不改、不取新挑战 |
| 身份／原路由 | 错device、错误原route、新hash也拒、原payload_hash损坏不联系receipt服务 |
| 坏成功／历史事实 | deletionEligible=true坏200不完成，策略撤回后原已成事实可只读记录，完成404不重建 |
| 声明未知 | GET当前qualified不等于原PUT成功，后续显式重投相同原body并持久确认 |
| 存储门禁 | 未批准不取challenge，NORMAL不得声明或落新收据，当前可执行文件SHA不能借旧批准 |
| 恢复／换库 | 旧epoch与更换physical UUID不能继承原意图或发receipt，原字节保留 |

未验：实际批准策略／严格CA收讫HTTPS／后端签名与真实PITR、真实响应丢失／进程崩溃、设备断电或满盘硬件故障、原生安全库正向、Linux／Windows新源构建及目标系统运行。SQL触发器是明确故障注入，不冒充物理断电。完整对象／冻结targets／累计范围／删除worker尚未接通，不将本轮称为完整qualified ACK或完整I。

## 四、<span id="extension">后续对象及目标接线</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

后续可复用`receiptControl`的当前physical／身份／guard门禁、`stageReceipt`的正文＋原意图同事务、`loadReceipt`／`finishReceipt`的原摘要CAS以及`performReceipt`先纯查与显式原体续跑。当前`receiptCheckIntent`／`receiptValidateChallenge`／`receiptValidateResult`只接受`message_content`普通文本，不能直接把附件描述或forward投影塞入同kind。

对象接线需要后端独立kind／对象完整digest及版本／文件耐久点与授权生命周期；文件先达到耐久点，再同库元数据＋独立对象收据意图，发送／重试前再次核实文件。冻结目标／累计范围需要独立manifest／generation／连续位置及完整策略，不能复用现HTTP history或sync cursor当作ACK。接口定稿后再增专门校验器与测试，不放宽当前false门禁。

<a id="🔚" href="#前言">回到首页</a>
