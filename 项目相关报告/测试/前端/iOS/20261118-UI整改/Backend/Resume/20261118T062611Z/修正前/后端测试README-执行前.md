# JobsIM 后端测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 前言

测试代码独立放在本目录，生产 Go 工程不复制测试文件。当前结果覆盖已经运行的基础切片；单一现行需求中的完整 I、跨故障域强保护及全量功能验收仍须继续完成，不以这里的通过数替代产品验收。

完整原件收讫使用[独立专项入口](./接口/object_receipts.mjs/README.md)，区分纯规则与真实数据库夹具，不访问TLS服务；验证报告与尚在补齐的范围见[中央检查点](../../项目相关报告/测试/后端/20261116-完整对象收讫/README.md)。

本人主动改密使用[专项入口](./接口/password_change.mjs/README.md)，当前4纯规则／12真实PG及62d3生产构建通过；[中央检查点](../../项目相关报告/测试/后端/20261116-本人主动改密/README.md)保留原Cookie失败与输入冻结。生产auth／journal同随机PG库、message／governance显式进程夹具，不证明HTTPS、独立双集群恢复或五端入口。

## 一、离线单元测试与编译

从仓库根目录运行 `python3 测试/后端/运行测试.py --go <Go可执行文件>`。运行器用临时 Go overlay 挂载 [安全测试](./security_test.go)与 `接口/` 内全部 `*_test.go`，当前共49份，覆盖安全、头像、发送围栏、刷新、同步、本人状态／草稿、收讫、正式导出、改密及注销普通准入门禁。重复文件名会拒绝覆盖；没有显式隔离环境时，数据库／真实角色用例仍按各自门禁跳过。默认随后兼容运行 CLI 的 `outbox_test.go` 冒烟测试，CLI完整15份外置用例应使用[独立入口](../CLI/运行测试.mjs/运行测试.mjs)；`--backend-only` 可仅运行后端，避免跨端任务重复执行。不会将测试写入工程源目录。

显式设置`JOBS_IM_MIGRATION_TEST_ENV=<私密环境文件>`才运行[空库迁移测试](./接口/fresh_migrations_test.go)，创建随机专用业务／journal数据库、验证全部角色从空库迁移及幂等重跑并只清理本测试数据库。显式设置`JOBS_IM_EXPORT_TEST_ENV=<独立协议域环境文件>`才运行[导出协议／文件栅栏测试](./接口/export_fence_test.go)，用真实PG、TLS测试角色、1.68MB附件和文件锁验证取消后无晚写复活；该私有harness不与运行中的实验worker共用业务数据库。

[发送围栏外置测试](./接口/send_admission_test.go)由同一运行器挂载。常规执行验证实际子进程持锁、等待取消、kill后内核释放及不同StateDir证明拒绝；设置`JOBS_IM_ADMISSION_TEST_ENV=<私密测试环境JSON>`后，还核PG未决／提交／回滚原事实、真实带标记进程崩溃后恢复、异库标记保留及备份恢复门禁，并创建8个独立角色数据库验证退重入旧快照拒绝和新意图成功，结束关闭并删除本轮独占数据库。[结构化证据](../../项目相关报告/测试/后端/发送围栏与未知提交实测.json)仅证明MacOS本地文件系统与真实PG，不代替Linux／Windows原生或跨主机验收。

[转发网关外置测试](./接口/forward_gateway_test.go)同样通过overlay挂载。常规执行核公开路径持锁模式；显式`JOBS_IM_FORWARD_JSON_FIXTURE=<独立转发域私密夹具>`才连真实角色HTTP／PG验证history／search／favorites／announcement／message-results整个JSON写出期间仍持围栏、客户端取消排空、伪造内部证明不能跳锁，以及外层SH之后出现marker不触发内部嵌套提升自锁。另一项仅进程内注入principal RPC503，验证精确GET／history／结果200空占位、搜索收藏503及同version精确GET恢复，不冒充部署服务真实故障；对应[临时投影证据](../../项目相关报告/测试/后端/转发临时资格投影恢复实测.json)。私密夹具生成与真实慢TLS客户端步骤见[持锁专项](./接口/forward_fences.py/README.md)。

后端工程独立执行 `go build ./...` 与 `go vet ./...`；依赖由工程 `go.mod`／`go.sum` 锁定并保留校验，不要求全局修改 Go 环境。

## 二、真实 HTTP、数据库与恢复

以下测试会创建带随机后缀的专用账户与数据，必须指向受控测试实例。HTTPS 信任显式测试 CA，禁止关闭证书验证。

| 入口 | 执行与范围 | 已记录证据 |
| --- | --- | --- |
| [核心接口回归](./接口/integration.py) | `python3 测试/后端/接口/integration.py --server https://127.0.0.1:<端口> --ca-file <显式CA.pem>`；检查注册、联系人、稳定消息ID、异参冲突、编辑回复、反应／置顶／收藏、拉黑保联系人／双向direct阻断、解黑新代际准入、删除后新轮申请不需reopen、Cookie／CSRF及恢复码，finally撤本轮会话。 | [核心 HTTP 报告](../../项目相关报告/测试/后端/核心HTTP回归.json) |
| [本人关系只读接口](./接口/contact_relationship.py) | `python3 测试/后端/接口/contact_relationship.py --environment-file <私密环境JSON>`；两个真实账号验证owner隔离、旧reopen兼容原单、拒绝／删除后新申请无需reopen、拉黑保好友且头像仍拒绝、解除后当前好友恢复、伪造owner查询无效、GET不创建记录及大代际保持字符串。 | [本人关系报告](../../项目相关报告/测试/后端/本人关系只读接口实测.json) |
| [登录刷新家族](./接口/session_refresh.py/README.md) | 独立新域，真实旧7天schema迁移、AT／RT单代旋转／并发／重用／多设备、实际丢成功响应、初始未知关联撤旧与持久退休、BFF密封／CSRF／原family header；期限和代际有明确SQL夹具，非90天实等。 | [86 HTTP／43语义](../../项目相关报告/测试/后端/登录刷新家族真实实测.json)、[无外层OS锁的PG并发](../../项目相关报告/测试/后端/刷新数据库并发单消费实测.json) |
| [刷新持久环境续验](./接口/session_refresh.py/README.md) | 新CA／身份／两套物理PG，补完整多层未知退休闭包与迟到拒建；旧binary已丢失，本轮不称真实旧schema迁移，BFF进程重启也未重复。 | [102 HTTP／48语义](../../项目相关报告/测试/后端/登录刷新持久环境与完整退休链实测.json)、[新PG并发](../../项目相关报告/测试/后端/刷新数据库并发持久环境实测.json)、[Go原始日志](../../项目相关报告/测试/后端/刷新持久环境Go原始日志.txt) |
| [刷新恢复拒旧](./接口/session_refresh_restore.py/README.md) | 新建独占源／目标及独立journal；真实加密backup、offline restore、原恢复操作续跑，旧AT／RT／Cookie拒、密文清，备份后退休事实重放不复活旧认证。 | [22 HTTP／9语义](../../项目相关报告/测试/后端/登录刷新恢复拒旧实测.json) |
| [刷新持久环境恢复](./接口/session_refresh_restore.py/README.md) | 独立持久namespace重跑，Documents备份与运行目录key分离，保留历史报告。 | [新22 HTTP／9语义](../../项目相关报告/测试/后端/登录刷新持久环境恢复拒旧实测.json) |
| [权威快照与增量](./接口/sync_snapshot.py/README.md) | 新隔离域；511消息跨owner500页界，完整manifest与refs／body／event原字节envelope，final validate、新append、权限ABA、合法历史global墓碑、真实NATS断开后HTTP补偿及同存储重连去重。510行SQL只用于分页夹具，不当发送API证据。 | [100 HTTP／1282语义／32页](../../项目相关报告/测试/后端/权威同步底座/真实HTTPS.json)、[7组＋9子项Go](../../项目相关报告/测试/后端/权威同步底座/Go定向.json) |
| [同步真实恢复](./接口/sync_restore.py/README.md) | 同固定binary另建源／目标及独立journal，真实backup／offline restore E1→E2，旧凭据／边界／页拒旧、原创建tuple保留、global和缺新编辑正文占位、新快照／delta；原E1 wire网络重投不入target、不增加E2位置。 | [22 HTTP／56语义／2页成功续验](../../项目相关报告/测试/后端/权威同步底座/真实恢复.json)，[raw保留包装FAIL＋RESUME](../../项目相关报告/测试/后端/权威同步底座/真实恢复.log) |
| [原发送准入与提交围栏](./接口/send_admission.py) | `python3 测试/后端/接口/send_admission.py --environment-file <私密环境JSON>`；真实HTTPS检查旧摘要回放、缺快照不新建、peer token不能借用、direct删除新接受／拉黑保好友双向拒绝／解黑新准入／群退重入、当前read ACL及真实PG阻塞提交时撤权等待。 | [52项HTTP报告](../../项目相关报告/测试/后端/发送准入代际实测.json) |
| [搜索与本人收藏分页](./接口/search_favorites.py) | `python3 测试/后端/接口/search_favorites.py --environment-file <私密环境JSON>`；较新110条他人命中不挤本人结果、本人会话超公共前100、签名游标隔离、收藏源撤回／失权／退重入及原会话／消息定位ACL；[游标测试](./接口/search_test.go)核原owner／查询／恢复代际。 | [搜索收藏报告](../../项目相关报告/测试/后端/搜索与本人收藏分页实测.json) |
| [结构化提及与群公告](./接口/mentions_announcements.py/README.md) | `python3 测试/后端/接口/mentions_announcements.py/mentions_announcements.py --environment-file <私密环境JSON>`；真实成员代际／block／独立限频、内容版本与互动版本分离、引用源变化／到期／重入占位、原操作immutable及只读原消息查结果绝不新建；父群锁与源编辑真实竞态。 | [提及公告报告](../../项目相关报告/测试/后端/提及与公告实测.json) |
| [群角色准入代际](./接口/role_qualification.py/README.md) | 先在真实旧无role服务准备已提交原公告，原实例受控升级后验证owner转出／转回、旧@all／普通待发／公告PUT和DELETE拒未建、已成旧摘要兼容、成员与joinedAt不变、无关成员与no-op不抬授权；大数role字段仅SQL精度夹具。 | [角色代际报告](../../项目相关报告/测试/后端/角色准入代际实测.json) |
| [普通引用式转发](./接口/forward_refs.py/README.md) | 纯引用入库、原proof／目标token／原ID、真实成功响应未读对账、链深8、编辑／撤回／源退重入／改密、当前搜索收藏，以及真实PNG／GIF／AAC-M4A／MP4／文件代理、Range／SHA与慢媒体排空。 | [185 HTTP／104语义报告](../../项目相关报告/测试/后端/普通转发真实闭环.json) |
| [转发公共JSON与持久续跑](./接口/forward_fences.py/README.md) | 显式独立域；真实慢TLS公共JSON阻塞EX、断开释放、200后代分批拒假完成及worker恢复；另用明确SQL夹具模拟owner pending已提交但RPC未到，再经真实原退群接口收尾。 | [218 HTTP／12语义报告](../../项目相关报告/测试/后端/转发公共JSON与持久续跑.json)、[5类JSON围栏](../../项目相关报告/测试/后端/转发公共JSON持锁与私有证明.json) |
| [转发恢复代际](./接口/forward_restore.py/README.md) | 另建源／目标业务DB及独立journal DB，仅停止自己记录PID；真实加密backup／offline restore，source／membership／security不变且旧ref仍active时只因epoch拒旧投影，新显式proof可新发，旧ref不复活。 | [33 HTTP／16语义报告](../../项目相关报告/测试/后端/普通转发恢复代际实测.json) |
| [转发构建既有回归](./接口/forward_compatibility.py/README.md) | 当前候选构建重新跑核心55、分页279、当前role／提及／公告77；历史报告不覆盖。`--export-only`另核含forward实验包明确失败／failureCode及制品清理。 | [既有回归](../../项目相关报告/测试/后端/转发构建既有接口回归.json)、[导出拒绝11 HTTP／3语义](../../项目相关报告/测试/后端/转发导出明确拒绝实测.json) |
| [备份、隔离恢复与音频](./接口/backup_restore.py) | `python3 测试/后端/接口/backup_restore.py --environment-file <私密环境JSON>`；只接受显式 `Safety` 独立测试域，停止其记录的 PID，真实 pg_dump、加密制品、独立数据库恢复、旧会话／口令／游标／Outbox拒旧，生成真实 AAC/M4A并鉴权下载核验。 | [备份恢复与音频报告](../../项目相关报告/测试/后端/备份恢复与音频实测.json) |
| [管理员 MFA 与状态](./接口/admin_mfa.py) | `python3 测试/后端/接口/admin_mfa.py --environment-file <私密环境JSON>`；只接受显式 `Safety` 域，受控初始化首位管理员，检查 TOTP 必须与防重放、目标／载荷／版本绑定、原成功操作对账及封禁／墓碑／恢复。每 TOTP 步只能用一次，测试会等待有效窗口，不能与其他管理员测试并发消耗同一因子。 | [管理员 MFA 报告](../../项目相关报告/测试/后端/管理员MFA与状态实测.json) |
| [管理员重置凭证](./接口/reset_grant.py) | 专用Safety域；检查密码用途／600秒到期／单次消费、发放即撤旧登录资格、原凭证不回显、原消费结果对账且不重置MFA。 | [重置凭证报告](../../项目相关报告/测试/后端/管理员重置凭证实测.json) |
| [头像／最小举报](./接口/avatar_reports.py) | `--environment-file <私密环境JSON>`，可选`--admin-fixture <私密管理员文件>`；真实512/128 JPEG、四并发原ID、ACL／block、坏图保旧、原结果current=false、源授权及MFA治理。 | [头像／举报报告](../../项目相关报告/测试/后端/静态头像与最小举报实测.json) |
| [关闭会话受限续跑](./接口/session_closure.py) | 只接显式Assets实验域；真实持有文件EX锁使logout排空超时，确认普通请求401、仅原close／CSRF可续，释放锁后沿原logout完成。 | [关闭续跑报告](../../项目相关报告/测试/后端/设备关闭受限续跑实测.json) |

历史导出另有[公开实验生命周期](../../项目相关报告/测试/后端/实验导出公开生命周期实测.json)与[私有协议／写入栅栏](../../项目相关报告/测试/后端/受控导出协议与写入栅栏实测.json)证据，当时正式能力关闭。后续[正式本人导出2b58](../../项目相关报告/测试/后端/20261116-正式本人导出/README.md)已有独立真实全媒体／源排空／恢复验证，当前`chat_export=true`；五端接线与真实交付分别登记，旧实验包不替代正式协议。

私密环境 JSON 位于仓库外、权限600，含运行器需要的数据库连接、CA、状态目录与记录的服务 PID。测试只输出脱敏断言与汇总，凭据、Cookie、TOTP种子、备份密钥不得写入报告或仓库。恢复测试操作专用源／目标数据库，不默认选择用户联调服务；有效备份失败时不得被覆盖或删除。

本轮转发切片的[固定构建快照](../../项目相关报告/测试/后端/普通转发固定构建快照.json)记录最终binary、各报告／日志及48个后端／测试源码摘要、实际能力和59条正式普通HTTP统计口径。主转发／慢TLS／恢复证据保留各自早一候选快照；最终仅导出失败原因可解释性窄改另有当前binary回归，不把历史报告改写成全部重跑。

## 三、结果的边界

报告记录实际执行时的范围，不代表后续新增代码自动通过；不同报告有重叠，不能相加成产品通过项。备份恢复实测包括导入已提交响应丢失后的同操作续跑／epoch不重复分配、完整journal snapshot anchor/tail、旧编辑正文不复活和互动版本推进。同步底座与后续S2的本人read／云草稿／偏好分别依[实际coverage](../../代码/后端/Go/接口契约/README.md#authoritative-sync)及逐轮报告；未覆盖peer资料流、完整合格端存储／目标ACK仍须补齐。跨域最小权限、全账户数据域同步、全部拒绝门禁及资格的精确恢复证明、全量安全恢复覆盖、媒体多引用生命周期与跨端完整I仍需专项证据；`after_receipt`、`e2ee`、`strong_protection`、`safe_restore`不能由测试绕过开关宣称已交付，正式导出后端限定通过也不等于客户端完整交付。

工程说明见 [后端 README](../../代码/后端/Go/README.md)，实际接口字段见 [接口契约](../../代码/后端/Go/接口契约/README.md)。

新 Python 接口执行默认输出直接进入工程根 `项目相关报告/测试/后端/执行记录/<UUID>/`，保留旧固定 JSON 证据；显式 `--report` 仍按用户指定位置。Go 外置用例只在显式报告环境变量存在时写出，调用方应指定中央域的新子项。


完整注销新增外置 `接口/account_deletion_pg_test.go`，复用既有显式隔离PG门禁与随机专属数据库；`TestAccountDeletionPG*`覆盖全部family撤销、匿名原体不创建、用途证明／单次恢复码、独立封禁不被取消覆盖、取消journal结果未知续跑、0天最终清理以及恢复时取消intent不降闸。RPC明确为内存传输，journal在测试PG独立schema，不称真实HTTPS／双集群恢复或原生端通过。默认runner会自动发现；实际结果与失败修复保留中央实现证据，不能用未开启PG门禁的skip当执行成功。

联系人／改名／举报材料的当前增量测试为[ui_contract_test.go](./接口/ui_contract_test.go)，由后端运行器动态纳入外置overlay。核心HTTP／本人关系／发送准入脚本已同步当前关系合同；其断言源码更新与AST可解析不等于真实HTTP通过。当前源码、格式检查及低磁盘暂缓的编译／PG／HTTPS状态见[本轮后端清单](../../项目相关报告/测试/前端/iOS/20261118-UI整改/Backend/summary.json)，旧报告原件仅保留其原始验证范围。
