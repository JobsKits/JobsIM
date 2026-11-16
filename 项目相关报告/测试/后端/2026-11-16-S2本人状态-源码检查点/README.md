# S2 本人状态同步源码检查点

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、源码状态](#status)
- [二、已接源码的本人状态入口](#registered-source-routes)
- [三、未挂接与未完成](#unwired)
- [四、静态路由与角色口径](#static-inventory)
- [五、验证与源清单](#verification)
- [六、继续点](#continue)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本目录记录 2026-11-16 工作树中本人读点、会话私有状态、字体偏好及草稿基础的 **source-only** 检查点。S2 指同步协议 `schemaVersion:"2"`，不代表第二套产品需求或产品版本。当前源码尚未编译、迁移或运行，新能力未验收。

固定后端 `57a86767d12f005da08c728ff544a2b66076b2db03a203e3221d3490e355f290`、44443 固定环境及[原同步底座证据](../权威同步底座/README.md)保持不变。原报告只证明其固定源码和限定场景，不覆盖本目录中的工作树改动。243 主验收状态没有改变。

## 一、<span id="status">源码状态</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 范围 | 当前接线与行为 | 状态 |
| --- | --- | --- |
| schema 协商与设备登记 | `schemaVersion` 选择独立同步流；同一可信连接、聊天 namespace、认证 device 与物理 store 不因协议升级更换。`device_stores` 独立于 schema，旧登记回填冲突拒绝，禁止以 schema2 绕过空库 409 | 已接源码；迁移／兼容未验 |
| schema1 覆盖 | 保持原覆盖和原游标序列化；不向其分配偏好、view、read 或 draft 新 kind | 已接源码；回归未验 |
| 字体偏好 | 个人来源的 `small/standard/large`，按 epoch/version CAS；稳定原请求结果和完整原 body 的只读核对；旧 epoch 普通值重基线后有效版本为零 | 已接源码；端侧映射及运行未验 |
| 会话私有视图 | 个人置顶顺序、静音截止、归档、隐藏；`manualUnreadGeneration` 独立于真实读点。可选未读锚点由消息 owner 校验真实创建 tuple | 已接源码；并发／端侧行为未验 |
| 本人读点 | 原观察 ID 及 member generation 绑定；只按真实 creation epoch/sequence 合并连续前缀，乱序洞与 legacy 逐 ID 保留；已被连续前缀覆盖的观察在同事务压缩 | 已接源码；真实 owner／恢复未验 |
| 实时未读汇总 | 本域只读快照固定观察集合，消息 owner 按当前成员历史起点／删除／发送人重验；批次及结尾必须保持同一消息水位，超界或变化拒绝发布近似数 | 已接源码；精确计数未实测 |
| schema2 快照／增量 | 本人 read、观察、view 与偏好引用接入 capture/hydrate；离群保留纯个人元数据，旧成员观察不作为当前资格；snapshot 的 read 对象仍为 `countStatus:not_computed`，实时计数另由 GET read 返回 | 已接源码；manifest／原子安装未验 |
| 自有同步来源 | `sync` 同时作为状态来源和消费 owner；[**NATS JetStream**](https://docs.nats.io/nats-concepts/jetstream) 发布与消费分开连接，仅状态源表触发 Outbox，导入表不触发回流 | 已接源码；总线／补偿／恢复未验 |
| 草稿正文与 floor | 本人 text 独立 AEAD，原 cipher epoch/version 与逻辑重基线分开；floor 可在 view 不存在时保留拒绝；引用退休保留本人当前及未发布独立密文，显式 clear／正文替换负责擦除 | 仅基础 helper；迁移未挂、未验 |
| 草稿 reply 许可 | 独立目的域 `replyAdmission` 绑定读者、源内容版本、成员／安全／恢复代际；同步消息投影及实验导出剔除该临时许可，不复制 quote 正文 | proof/helper 已写；完整草稿流程未挂、未验 |
| 草稿 reply holder | message owner 的 prepared/active/revoking/cleared 元数据、原准备摘要及完成校验 | 仅定义；迁移／路由／失效交接未挂、未验 |

字体服务端目前仅保存枚举，不能据此声称四个图形端已执行跨端字号映射；CLI 继续尊重终端字号。公开已读、在线态、附件字节耐久、qualified ACK、正式导出、注销及普通迁移没有借此完成。`after_receipt` 继续关闭。

## 二、<span id="registered-source-routes">已接源码的本人状态入口</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

> 以下是工作树注册路径，不是部署或接口实测结果。详细定位与对应源 SHA 见[源清单及静态路由证据](./源SHA.json)。

| 方法与路径 | 用途 | 冻结边界 |
| --- | --- | --- |
| `GET /api/me/preferences` | 本人来源字体偏好 | 不含未实施的隐私开关 |
| `PATCH /api/me/preferences` | 偏好 CAS | 原 requestId、revisionEpoch、expectedVersion、fontPreset |
| `GET /api/me/preferences/operations/{requestId}` | 原操作元数据 | 不能代替原 body 摘要核对 |
| `POST /api/me/preferences-results` | 完整原偏好请求只读核对 | 未成返回 404，绝不创建 |
| `GET /api/conversations/{id}/view` | 本人会话视图 | 个人状态不改变群／云历史权限 |
| `PATCH /api/conversations/{id}/view` | 视图 CAS | 原 nonce、epoch/version；手动未读另核独立 generation |
| `POST /api/conversations/{id}/view-results` | 完整原视图请求只读核对 | 未成返回 404，绝不创建 |
| `GET /api/conversations/{id}/read` | 当前本人读点及实时未读 | 精确字符串 `unreadCount` 与 `countSourceWatermark`；依赖变化拒绝计数 |
| `PUT /api/conversations/{id}/read` | 合并本人观察 | 原 nonce、epoch、membershipGeneration、observedMessageIds |
| `POST /api/conversations/{id}/read-results` | 完整原读请求只读核对 | 原已成结果与当前状态分开；不重做观察写入 |

实时未读汇总不嵌入原 manifest cut：同步 read 投影的原字节 envelope 仅表达该 ref 的本人读状态。客户端后续接入仍须完成捕获 namespace/family/epoch/permission、完整页 staging、原字节摘要验证、本地原子安装和第一次 changes 重验；本检查点不提供客户端存储合格结论。

## 三、<span id="unwired">未挂接与未完成</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

- `migrateSelfDraftFloor`、`migrateDraftReplyHolders` 及 `draftReplyHolderRoutes` 尚未进入应用迁移／Handler 调用链。`self_state.go` 识别 `draft` 路径，只承担网关分类，不能算已注册草稿接口。
- 草稿 GET／PUT／DELETE／原 body 只读查单、正向正文提交与原操作元数据结果尚未实现。`draft_pending` 的 prepared/retired 基础不代表已完成业务提交。
- journal 白名单、typed floor 记录、独立日志重放、恢复时旧 pending／reply 资格退休尚未接入。
- 源编辑／global／成员退出／本人安全撤权的 prepared 与 active holder 清单、持久 callback denial、排空及后台续办尚未接入。内部 RPC 须校同一规范化 StateDir，不能嵌套宿主围栏；双方须先提交本域事务再互调。
- 引用清理只移除预览／发送资格，保留本人独立输入 text；新源状态不能给旧意图补 proof，旧 Outbox body 不自动删 reply 后另发。保留在 retired pending 中的 ownedText 尚无公开取回／明确采用流程。
- 云草稿计划只覆盖 ownedText 与纯 reply 引用；附件队列、本机上传字节、结构化 mention 选择上下文仍为本端耐久范围。相应 `localOnlyFields` 及冲突交互尚未交付，不能称为完整云草稿。
- 尚无新候选二进制、运行域、迁移／恢复演练、客户端适配或 `sync_schema2`／草稿交付能力声明。

## 四、<span id="static-inventory">静态路由与角色口径</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

> 统计日期 2026-11-16。单位为 `Handler` 可达调用链中的去重公开 HTTP method/path；仅展开源码注册，不实例化 Handler，不证明其可构建或可服务。逐条证据保存在[源SHA.json](./源SHA.json)。

| 分类 | 工作树静态数量 | 口径 |
| --- | ---: | --- |
| 普通接口 | 79 | 排除管理员权限入口、实验导出及 health |
| 管理入口 | 10 | `/api/admin/` 的 8 条，加实际强制 Admin 的 capabilities PUT 与 auth step-up |
| 实验导出 | 5 | 仅 `ExperimentalExports` 分支注册；不纳入普通接口 |
| health | 1 | 公开 method/path 去重，不按每个内部角色重复累计 |
| 合计 | 95 | 实验开；实验关为 90，均含 health |

GET 隐式匹配的 HEAD、框架生成的 404／405、网关转发前缀、静态 Web 文件、内部 `/internal/` RPC、未挂函数不另计。schema1／schema2 使用相同的 7 条 `/api/sync/` method/path，以 body/query/manifest 协商协议流；schema2 不增加 HTTP 子路由。本人状态入口另增上述 10 条普通注册，不能将分支数量再加到路由数。

生产 [**Go**](https://go.dev/) 源码为 **69 份**：递归统计 `代码/后端/Go` 中自有 `*.go`，含两个 `cmd` 入口，排除 `*_test.go`、供应商及构建目录。固定 57 的源清单是 56 份；其普通接口 **69 条**仍为历史实测口径，不能与当前源码文件数混用。

角色为 `identity→auth`、`directory`、`conversation`、`message`、`media`、`control`、`journal`、`governance`、`sync` 共 **9 个 owner**，加 `gateway` 为 **10 个逻辑 HTTP 单元**。`--role all` 在同一 `jobsimd` 进程启动这些 listener，不能据此写成 10 个独立进程；逐角色部署的进程数另看实际部署。同步事件来源为 schema1 的 4 个 owner，以及 schema2 加上 `sync` 的 5 个，均不改变 9 owner 角色总数。

## 五、<span id="verification">验证与源清单</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 检查 | 当前结论 |
| --- | --- |
| 修改源的 gofmt 解析／格式 | 已完成；只证明语法解析／格式，不是类型检查 |
| Go 构建／vet／跨平台编译 | 未运行 |
| 新测试源码 | 已保存 9 份；全部未执行 |
| [**PostgreSQL**](https://www.postgresql.org/) 迁移／并发／恢复 | 未运行 |
| HTTP／NATS／丢响应／撤权竞争 | 未运行 |
| 客户端、模拟器、真机及完整 I | 本检查点不提供结论 |

[源SHA.json](./源SHA.json)包含全部 69 份生产源、两个模块锁文件、本切片 9 份测试源的 SHA256、相对路径及与固定 57 源清单的字节差异分类，另含路由注册和 schema 分支定位。哈希只固定当前文本，不替代编译、SQL 或运行证据。工程外的 seed、数据库、实例 CA 私钥、AT／RT／Cookie、NATS token 及备份不进入本目录。

## 六、<span id="continue">继续点</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

- 先完成草稿原意图、holder、floor、journal 与回调／续办的闭环，再挂迁移和公开入口；明确 ownedText 与源引用的清理粒度。
- 资源门禁解除后，在全新持久隔离域做类型检查、迁移、并发、真实 owner RPC／总线与恢复验证，保留默认限流和既有固定域。
- 再封独立候选与真实契约，逐端接入 schema2；根据同一候选证据更新资产状态。固定 57 的原归档、原 SHA 及原报告继续保留。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
