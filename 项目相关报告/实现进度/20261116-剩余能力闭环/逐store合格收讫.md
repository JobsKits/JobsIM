# 逐store合格收讫与保留策略

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前边界](#jobs-toc-heading-1)
- [二、生产源码与入口](#jobs-toc-heading-2)
- [三、已运行证据](#jobs-toc-heading-3)
- [四、验收限制](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、当前边界</span><!-- jobs-toc:heading-end -->

当前源码提供[qualified-receipts-v1契约](../../../代码/后端/Go/接口契约/合格收讫/README.md)，原bootstrap的`after_receipt:false`与旧sync context／storage顶层false保持兼容。新协商按当前本人、设备、storeGeneration、epoch与已批准的精确构建／耐久报告判断正文和原件资格。未批准库继续普通传输同步与默认cloud_history；未给生产a4或Canonical写入任何profile批准项，未操作这两实例。

当前已验后端独立PG／JetStream正反及文件锁3方法，相关真实PG59方法全部通过；CLI最新完整本地135通过／0失败／2显式环境跳过。完整输入及候选分别封存，初轮失败保留。服务器真实正向中的profile是测试明确批准，不证明任何真实客户端原生硬盘；五端新协商及生产批准仍由各端／部署分别验收。当前没有给a4或Canonical写profile，没有启停或测试这两实例。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、生产源码与入口</span><!-- jobs-toc:heading-end -->

| 能力 | 当前实现 | 默认、持久化与生命周期 |
| --- | --- | --- |
| 逐store协商 | `receipt_capabilities.go`的receipt-context | 新协议独立读取，严格核当前登记原库；qualifiedAck是声明仍匹配批准，不是消息ACK，未批准false，历史QID查询不恢复新资格 |
| 实时运行保护 | `retention_readiness.go`、`journal_receipt_readiness.go` | 真PG system identifier不同、message与journal耐久设置、当前epoch、实际JetStream发布／消费连接齐备才ready，同PG两库拒绝 |
| 精确批准发现 | receipt-profiles | 当前认证、store和本地实测build SHA；只列已配置项，不登记、不宣称本地探针通过 |
| 声明原单 | storage-results | 完整原六字段＋原nonce HMAC匹配后只读；未知404、异体409，批准撤回仍给历史result但current false |
| 原目标观察 | receipt-targets | 当前原人类成员代际，仅本人原device／store／qualification的原DG／CV／manifest；显式原QID只查同本人原库历史声明，messageCurrent独立范围自然到期，global先拒；不泄露他人设备 |
| 策略写 | retention_policy.go、send_admission.go | 新after-body／after-object分别核当前actor原store和实时保护；原已成nonce先读，CAS／明确n／仅新消息不变 |
| 原件代际 | sync_object_receipts.go | ro1从已成父正文结果复制原DG／manifest，提交再次核父tuple，结果沿原pair；旧无manifest允许空对 |
| CLI新协商 | receipt_negotiation.go、receipt.go、receipt_objects.go | 新context真值不改旧同步流；body／object proof及结果严格核原pair，声明未知先纯查，不换原nonce／报告 |
| CLI本库探针 | receipt_negotiation.go | 实际当前SQLite FULL提交随机摘要后读回；file合同另随机私有文件完整读回、file／dir fsync并精确移除；不生成消息ACK或部署批准 |
| CLI保留策略 | receipt_policy.go | 原nonce／CAS／policy先FULL提交receipt_policy_operations，未知保pending，显式原单一次PUT，完成只显示原结果，不补最新CAS |
| CLI迁移原raw | receipt_bodies、receipt_migration_source.go | 原envelope与content意图同FULL事务；历史qualifiedAtCommit先纯查原结果，再按DG／原成员／messageCurrent或legacy完整permission验证；当前批准撤回不抹历史fact，不造目标，global同tx删除，注销drop内容表 |
| QID旋转围栏 | send_admission.go、forward_media.go | PUT storage的gateway与owner均EX，等旧正文／原件交付SH排空后旋转资格，GET／纯查仍SH |

共享入口仅server.go注册新handler、control.go新只读协商feature、CLI main.go调用独立header helper和help。迁移源码、上传新协议、其他聊天的注销三份生产文件均保留。CLI普通迁移由主代理实现，未混入本收讫结论。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、已运行证据</span><!-- jobs-toc:heading-end -->

首个49输入CLI候选仅编译，通过后继续新增迟到context、原raw来源与历史QID守卫；它保留在证据目录，不追认为当前。最终当前CLI与后端均先`go test -c -p 1 -overlay <本轮overlay> -o <本轮候选.test>`，保存二进制，然后用`go tool test2json -p <包名> -t <同一候选.test> -test.v`执行，不重复编译。均使用Go1.27.4、GOMAXPROCS=2、GOPROXY=off与GOTOOLCHAIN=local。

| 本轮与输入 | 实际命令范围／结果 | 原始证据 |
| --- | --- | --- |
| CLI首完整候选，52输入 | 完整133 PASS／1 FAIL／2 SKIP；global夹具错误保留 | [首完整结果](./逐store收讫证据/CLI-完整候选-20261117-130619/完整本地结果.json) |
| CLI修夹具检查点，52输入 | 先窄global 1 PASS，再同一binary完整134 PASS／0 FAIL／2 SKIP；两轮跑前后输入无漂移 | [最终结果](./逐store收讫证据/CLI-修夹具最终-20261117-131146/完整本地结果.json) |
| CLI升级门禁最终，52输入 | 升级窄项1 PASS，再同一binary完整135 PASS／0 FAIL／2 SKIP；12收讫＋10迁移在内，52输入无漂移；独立darwin/arm64产品构建及--help退出0 | [最新完整结果](./逐store收讫证据/CLI-升级门禁最终-20261117-142600/完整本地结果.json)、[可执行构建](./逐store收讫证据/CLI-升级门禁最终-20261117-142600/可执行构建结果.json) |
| 后端首轮，184输入 | EX锁1 PASS，独立正向／声明负向2 FAIL，未隐藏 | [首轮原始JSONL](./逐store收讫证据/后端-真实就绪-20261117-130930/真实PG与JetStream.jsonl) |
| 后端第二轮，184输入 | 2 PASS／1 FAIL；已修旧声明兼容，原件事务内二次读取仍失败 | [第二轮原始JSONL](./逐store收讫证据/后端-第二轮-20261117-131839/真实PG与JetStream.jsonl) |
| 后端第三轮，184输入 | 3 PASS／0 FAIL／0 SKIP，包4.853秒；独立PG＋真实JetStream正向3.89秒、同PG／资格负向0.43秒、实际EX锁0.09秒 | [正反最终结果](./逐store收讫证据/后端-第三轮-20261117-132216/真实PG与JetStream结果.json) |
| 同后端候选相关PG | `^Test(Receipt|ObjectReceipt|Retention|MediaRetention|SyncMigrationPG)`，59顶层PASS／0 FAIL／0 SKIP，含子项65个PASS事件，包23.717秒；184输入无漂移 | [相关最终结果](./逐store收讫证据/后端-第三轮-20261117-132216/相关真实PG结果.json) |

CLI最新测试候选SHA256为`7e10c4f003c2d2007d59b13377f527a70dc3d1d1a42dedc5bbeb792c3f406ba6`，后端第三轮为`67e8d35b0a4cac5fdd690cc815f4ef500192e7f45b321cdb8a9cdf72e750f391`，二进制、全输入副本、跑前SHA、overlay、编译日志和实际结果分别保留。CLI两项SKIP为`TestAuthCoordinatorActualHTTPSAndUnknownClosure`（未提供其专用真实HTTPS环境）与`TestAuthNativeSystemVaultSeparateDiagnostic`（未启用目标原生vault诊断）；不借本地注入HTTP声称它们通过。135已包含新增12个收讫方法、10个普通迁移方法及既有全本地集，不跨轮加算。

实际可执行候选为同最新输入构建的`jobsim-darwin-arm64`，Mach-O 64-bit arm64，SHA256 `e17f5340cfabe6b5b3e4ced7ab3100107f594a8c155bc9131066af1c5e7e2c25`；生产Go源／go.mod／go.sum逐项清单及聚合SHA `e3f70ca8aca35763dbee116e7b1154a08d718aa14f4a11603b13f7300193e114`在上述可执行构建结果。它与.test不同，未作为安装签名／vault或生产receipt profile批准，--help没有调用API。升级增量仅解除只读协商处误用旧build的死门槛：新声明仍核本进程SHA／批准／原库探针，新ACK与新after策略的实际写阶段仍严格核当前build与耐久；原已成事实纯查不借旧声明阻断。早期33eef测试候选与134通过检查点保留，不能替代此次135结果。

CLI初轮global失败来自单次SQLite Exec拼两条带位置参数INSERT，第二条夹具引用不对应真实messageId；拆成两次Exec并读回实际ID后，原生产global删除逻辑和零残留断言不变。后端首两轮抓出两项生产兼容缺口：旧成功storage声明没有qualified布尔，历史查单必须保留完整已批准旧tuple；新增ro1原DG／manifest后，事务内FOR SHARE父记录二次读取漏这两列，导致原件均409。最终补齐二次读取，校验不放松。新声明显式保存历史qualified/objectQualified快照；旧缺bit只认可成功声明表中完整原store／epoch／profile／build／report／合同，不把缺资格的新库批准。

[CLI12方法](../../../测试/CLI/receipt_negotiation_test.go)实跑覆盖新true与旧false兼容、错库／反常readiness、大代际和坏原结果保pending、ro1父代际、声明unknown纯查／撤回、真实SQLite／文件／目录探针、policy原nonce／CAS与完成不再POST、未资格／缺n不入队、raw同FULL失败回滚／global清除／fresh cloud_only迁移及legacy完整permission；新程序不能因旧build声明仍批准而无法纯查或声明新批准build，旧build也不能借此新建ACK或after-policy。HTTP均为明确注入，仅本机存储操作真实。

[后端2PG＋1文件锁方法](../../../测试/后端/接口/receipt_capabilities_pg_test.go)正向使用本人的62381随机Main库、62382独立物理PG随机journal库、动态loopback私有JetStream、实际StartSyncWorkers、九生产handler与严格HTTPS测试gateway。实测真就绪、旧兼容、原件父tuple、策略发布、新消息冻结／累计ACK／独立journal保护／n=0 cloud_only清理、历史QID与原成员、已到期pending global拒旧来源、总线停止拒新策略。负向实测无批准、错store、同PG拒独立、body-only不扩原件、原声明查询不写／批准撤回不复活、未知或另一device历史QID拒绝；实际本机文件锁证明资格PUT先排空原receipt SH，gateway和owner的EX一致。相关59还覆盖既有正文／对象、保留／媒体和普通迁移的正反／恢复反例。

资源仅限既有Integration UUID/Safety，维护连接由0600且归本人pgpass及明确62381／62382 guard约束；各用例创建／清除自己的随机数据库。NATS token／配置／原日志只在工程外私有临时目录。测试清理对确切子进程TERM＋Wait并等sync连接退出；外层trap停止本人启动的两PG。两次成功窗后62381／62382无listener，原集群和其他测试源数据保留，见[资源收尾](./逐store收讫证据/后端-第三轮-20261117-132216/资源收尾.json)及同目录生命周期日志。a4／Canonical始终未操作。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、验收限制</span><!-- jobs-toc:heading-end -->

服务器不能从客户端布尔值或部署列表远程证明硬盘落盘。每端需要当前build真实耐久验证、部署批准与当前store声明；未覆盖目标OS、真机或断电时应分别标未验。五端新协商接线由各端实现者独立验证；后端完成不能替代客户端同事务正文／回执、完整原件fsync和原target的实际ACK。当前不宣称五端完整端到端合格收讫已完成。
