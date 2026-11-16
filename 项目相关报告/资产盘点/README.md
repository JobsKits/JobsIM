# JobsIM 五端资产盘点

![Jobs出品，必属精品](../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、五端当前基线](#jobs-toc-heading-2)
- [二、默认值、存储与生命周期](#jobs-toc-heading-3)
- [三、功能入口与跨端行为盘点](#jobs-toc-heading-4)
- [四、第一阶段最短补齐切片](#phase-one-slices)
- [五、平台验证缺口与维护规则](#jobs-toc-heading-6)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

盘点日期：2026-11-16。功能补齐先依据本表，每次修改 App 在同一任务更新入口、行为、默认值、持久化、生命周期、源码位置、差异及验证。这里记录当前实现和证据边界，不替代[现行需求](../../需求/README.md)及 243 项[功能验收](../../需求/IM功能验收表.md/IM功能验收表.md)，不将第一阶段未交付事项改排后期。

“已验”只覆盖对应报告列明的构建、环境和用例；历史构建、主机替身、模拟器、真机与目标系统验证分别列出。各轮测试不累加为需求完成数。报告集中于[项目报告入口](../README.md)，源码说明、接口契约和工具入口保留原位。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、五端当前基线 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 端 | 原生入口与实现 | 当前证据 | 当前差异／未验 |
| --- | --- | --- | --- |
| [Android](../../代码/前端/Android/README.md) | Kotlin／Compose；Room9／BundledSQLite／Keystore | [S2本人状态](../测试/前端/Android/20261116-S2本人状态/README.md)：最终5774产品与测试包读回一致，11 JVM、23/23原生及3/3真实HTTPS／UI通过；6个自有family退出已验。[正式本人导出](../测试/前端/Android/20261116-正式本人导出/README.md)：App1588c450／test76826edf构建、16 JVM（含5新导出）通过，原FAIL独立保留 | 新5 Room／模型仪器方法仅编译，未安装／启动AVD／API；系统保存选择、文件／目录fsync、真实HTTPS待验，不借旧5774通过。原截图过渡帧、物理真机、完整I及qualified ACK边界保持 |
| [iOS](../../代码/前端/iOS/README.md) | Swift／UIKit、Jobs自有Pod；GRDB／Keychain | [S2原生界面](../测试/前端/iOS/20261116-S2原生界面/README.md)：App／测试包构建及21原同步＋9新状态／UIKit反例共30/30原生通过；此前S2基础21＋认证本地12是各自固定输入，旧S1与首次失败保留 | clean删除原专用sim后，新sim真实接口已跑，完整UI因跨XCTest档案交接失败及磁盘门禁未通过；六个自有family已实际退出并有[精确撤销补证](../测试/前端/iOS/20261116-S2原生界面/Cleanup-20261116-185804/后端精确只读撤销补证.json)。新增DEBUG公开档案原子交接已随导出同包编译；31原S2目标＋10导出共41本地实际通过，0失败／跳过；仅启动本端既有sim并已正常关闭；[旧末轮限定补证](../测试/前端/iOS/20261116-S2原生界面/Real-20261116-174109-419860/后端限定只读补证-账户零.json)仅说明精确时窗匹配账户0，不是logout证明，原两次authenticate失败保留。旧4条认证HTTPS不追认新产品；真机／完整I／qualified ACK未验；[正式本人导出](../测试/前端/iOS/20261116-正式本人导出/README.md)新App89e02736／test94a1cbb8完整build-for-testing通过，123输入／包归档同SHA；首轮Swift6隔离失败保留，41本地通过（含原生GRDB／文件与注入请求），无该切片完整UI／真实HTTPS／系统保存证明 |
| [CLI](../../代码/前端/CLI/README.md) | Go终端；pure-Go SQLite；系统秘密库助手 | [附件收讫](../测试/CLI/20261116-附件收讫/README.md)eb42候选：80本地／0失败／2环境跳过及MacOS构建，含持久preparation／global和晚到文件／SQL围栏；[正式本人导出](../测试/CLI/20261116-正式本人导出/README.md)13cd候选：88本地／0失败／2环境跳过及MacOS构建；该轮35项typed输入已独立封存；后续改密候选另验，不再声称旧输入与当前全部源一致 | 两轮均真实SQLite／文件fsync＋注入HTTP，无真实收讫批准／导出HTTPS／PG。旧15a4的55本地／13真TTY、12ac正文67及395对象79各留原件不累计；process-only不是DPK正向，Windows／Linux目标未验，targets／累计ACK／删除及after_receipt仍未闭环 |
| [Web](../../代码/前端/Web/README.md) | 标准Web／React；同源BFF；IndexedDB | [旧D5 S2](../测试/前端/Web/20261116-S2本人状态/README.md)真11／浏览器26／模拟认证16分别封存；[独立草稿恢复](../测试/前端/Web/20261116-独立草稿恢复/README.md)10注入／构建；[正式本人导出](../测试/前端/Web/20261116-正式本人导出/README.md)全工程TypeScript7检查、18组实际Chrome注入及独立生产构建通过，JS a53afe46（index-D15iqubR） | 正式导出HTTP／文件／保存端为显式注入，真实HTTPS及最终用户保存未验；30语言／主题组合与390px截图属本轮限定证据。独立恢复未真实Settings联调；旧D5、旧DXdl或后端矩阵不追认新候选，native fsync／断电／完整I未验 |
| [HarmonyOS](../../代码/前端/HarmonyOS/README.md) | ArkTS／ArkUI，`Index`；RDB；Asset Store | [S2本人状态](../测试/前端/HarmonyOS/20261116-S2本人状态/README.md)：35/35当前同步与本人状态host、41/41旧规则分别通过；最终App02／Test02双HAP构建通过，51输入与36 ETS一致，37 Hypium方法仅编译；旧包与原始失败保留 | HDC无目标，未鸿蒙设备安装／RDB NAPI／真实HTTPS，host适配SQLite不冒充native RDB；S1 owner revision修复首次进入本S2包，旧S1 HAP不追认；完整I及qualified ACK未验 正式导出源已接；首15host限定通过，追加18目标及本轮HAP／设备未验 |

Android 旧认证固定包 SHA256：`e295688485e0caa73932670e48da227eae804108416625b62fba5b0e01cee8b3`。旧转发基线及其报告单独保留，不能合并计数。运行库、实例 CA、密钥和私密测试种子位于工程外持久目录；旧 Cache 清空后重建的新实例采用新身份，旧报告不证明新环境恢复旧数据。

当前已实现模块在开发与回归交替推进，不代表代码全部写完。正式导出Web已18注入／类型／构建，CLI已88本地／Mac构建；iOS已App／测试包编译及41本地通过，Android已16 JVM及主／测试包构建但5原生方法仅编译，Harmony导出源已接，首15host通过、追加18目标未验，HAP／设备未验，五端真实交付仍需继续。iOS新候选与鸿蒙真实同步／UI、S2 coverage外数据域与系统生命周期、完整合格ACK、主动改密客户端、账号注销和普通迁移仍属I，不以旧证据或本地通过替代。

本次续写以五端表为准，各轮不累计。D5云草稿采纳清提及与编辑／回复，视图／草稿CAS核所见版本，阅读排遮挡／绑定原成员代际等原证据保留。后端[云草稿](../测试/后端/20261116-云草稿/README.md)含独立真实恢复；[正式本人导出2b58](../测试/后端/20261116-正式本人导出/README.md)最终全媒体独立轮27 HTTP／13语义，当前客户端证据见上表。[完整对象9f3b](../测试/后端/20261116-完整对象收讫/README.md)7纯／9真PG（6新对象＋3旧正文）、全cmd类型／构建及media SH排空有独立证据，owner依赖显式进程夹具，无HTTPS／端落盘。CLI对象eb42的80及导出13cd的88仅本地；targets／累计ACK／保留worker尚缺，after_receipt=false。本人主动改密另有[最终4纯＋12真PG](../测试/后端/20261116-本人主动改密/执行记录/2026-11-16T12-41-43-795Z-ff2d05/结果.json)：auth／journal生产处理器同随机隔离PG库，message／governance为显式进程内确认夹具，无HTTPS／独立恢复／原生客户端，[62d3构建](../测试/后端/20261116-本人主动改密/生产构建.json)与[94项输入归档](../测试/后端/20261116-本人主动改密/最终输入SHA256.json)已过。

注销当前仅[普通准入门禁](../测试/后端/20261116-注销普通准入门禁/README.md)先落盘：deletion_state非none拒普通登录／AT／BFF／RT／恢复／重置／正式导出principal／新改密，后台撤旧family，prepared改密退休、历史成功不再current。4纯及25PG通过事件含原12改密与5新组／8子项，两个cmd构建通过；尚无公开注销、受限证明、取消与最终清理，不启用或宣称完整能力。

CLI当前新[主动改密7150](../测试/CLI/20261116-本人主动改密/README.md)产品源注册99顶层方法，原raw唯一97PASS／2SKIP与源码名单相同，Go退出0、Mac构建通过；37项typed输入逐SHA核齐，旧计数器提示粘连少报的95原JSON保留，独立复核不改旧96／88轮。原责任仅非秘密nonce／身份／状态，密码和OTP只RAM，原family显式retry与后继family隔离已本地验证，真实HTTPS／TTY／系统库仍未验。Harmony[正式导出](../测试/前端/HarmonyOS/20261116-正式本人导出/README.md)已新增设置页／原单RDB／RCP流／私有ZIP／原生保存源及十语41标签：原15host限定通过；最新12变更输入与18目标被4GiB门禁拦下、5 Hypium仅源／42方法，本轮HAP／RDB NAPI／选择器／设备未验，不能追认旧15为最新全通过。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、默认值、存储与生命周期 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 端 | 默认值与设置入口 | 持久化及秘密边界 | 生命周期及隔离 | 源码位置 |
| --- | --- | --- | --- | --- |
| Android | 设置：简中 `zh-Hans`、跟随系统、字号 1；通知默认关闭，正文默认隐藏；十语言及白天／黑夜／系统可切换 | Room9／BundledSQLite以同协程IMMEDIATE writer保存历史、原 Outbox、草稿、游标和非秘密认证意图；Keystore 加密 AtomicFile 原子保存完整 AT/RT 对，秘密不入 Room；正式导出原create／cancel body与SHA复用同Room物理UUID，短票仅RAM | 前台约 5 秒轮询；按 profile／来源／部署／用户／personal／epoch／device 分空间；同 family 刷新不换空间，reauth 新 device 冻结旧队列；旧响应有代际围栏；导出私有ZIP先完整验证再进系统保存，取消选择不报已保存（本轮原生待验） | [入口](../../代码/前端/Android/app/src/main/java/com/jobs/im/MainActivity.kt)、[存储](../../代码/前端/Android/app/src/main/java/com/jobs/im/Store.kt)、[认证协调](../../代码/前端/Android/app/src/main/java/com/jobs/im/SessionCoordinator.kt)、[传输／安全库](../../代码/前端/Android/app/src/main/java/com/jobs/im/Transport.kt)、[导出](../../代码/前端/Android/app/src/main/java/com/jobs/im/PersonalExportModel.kt) |
| iOS | 设置：简中 `zh-Hans`、跟随系统、字号偏移 0；通知及预览默认关闭；十语言、三主题、三档字号 | GRDB 历史／原意图；UserDefaults 非秘密设置；Keychain 整对 `WhenUnlockedThisDeviceOnly`；正式导出原create／cancel body、nonce与SHA在同GRDB独立records，短票仅RAM | 前台会话约 5 秒／消息约 3 秒，后台停止轮询；原 family/device 刷新稳定空间，新认证不搬旧 Outbox；迟到响应按原身份检查；导出页离开取消自有任务，ZIP utility校验后交Files；10导出逻辑已原生本地通过，Files与真HTTPS未验 | [会话](../../代码/前端/iOS/JobsIM/Services/IMSession/IMSession.swift)、[设置](../../代码/前端/iOS/JobsIM/UI/IMSettingsVC/IMSettingsVC.swift)、[字体／主题](../../代码/前端/iOS/JobsIM/UI/IMUI/IMUI.swift)、[传输](../../代码/前端/iOS/JobsByPods/JobsIMTransport/Core/IMAPIClient/IMAPIClient.swift)、[导出服务](../../代码/前端/iOS/JobsIM/Services/IMDataExportService/IMDataExportService.swift)、[导出页](../../代码/前端/iOS/JobsIM/UI/IMDataExportVC/IMDataExportVC.swift) |
| CLI | 终端命令／TTY；sync、sync-status、local-history；无GUI主题／语言。安全库失败默认拒无人值守，显式yes才限程 | 旧Auth整对系统库／nonce链不变；新SQLite schema4同库存快照stage、refs／origin、guard／cursor、global及cleanup intent；每SQLite另有UUID，原32hex安装身份与文件名不改，正文／对象receipt原意图、preparation与精确文件清理、data_exports创建／取消原body和family增量同库；票据仅RAM，AT／RT不入库 | 启动及显式命令串行同步；无责任先核cap才登记，责任存在后能力消失不退旧历史。迟到身份／guard拒提交；startup／每轮排空已验证清理intent；跨SQLite不宣称原子事务；只读本地历史须资格／正文hash，旧未知隔离。15a4真实专项属历史；eb42对象80与13cd导出88为本地／Mac构建，无新切片真实API，Windows／Linux运行未验 | [认证](../../代码/前端/CLI/cmd/jobsim/auth.go)、[协议](../../代码/前端/CLI/cmd/jobsim/sync_protocol.go)、[存储](../../代码/前端/CLI/cmd/jobsim/sync_store.go)、[编排](../../代码/前端/CLI/cmd/jobsim/sync_engine.go)、[检查点](../测试/CLI/20261116-权威同步/README.md)、[对象收讫](../../代码/前端/CLI/cmd/jobsim/receipt_objects.go)、[正式导出](../../代码/前端/CLI/cmd/jobsim/data_export.go) |
| Web | 设置：简中 `zh-CN`、跟随系统、正常字号、通知关闭；首屏独立演示数据有可见标识；十语言／三主题 | IndexedDB事务保存原队列／历史及正式导出原创建／取消body，短票仅RAM；localStorage仅非秘密设置和原nonce链；密码／OTP proof仅RAM；AT/RT只留BFF服务端密封存储，浏览器仅持HttpOnly不透明会话Cookie | 外层 auth lifecycle WebLock→内层 refresh 锁；同部署现有认证责任禁止降旧协议；切表单／关闭取消旧排队意图；旧 namespace 即使无 key 也持久 fence | [App](../../代码/前端/Web/Src/App.tsx)、[认证](../../代码/前端/Web/Src/AuthSession.ts)、[传输](../../代码/前端/Web/Src/api.ts)、[存储](../../代码/前端/Web/Src/storage.ts)、[导出动作](../../代码/前端/Web/Src/DataExportActions.ts) |
| HarmonyOS | 设置：简中 `zh-Hans`、跟随系统、字号 1、通知关闭；十语言／三主题 | RDB串行事务、单Asset小于1KiB设备解锁且不同步；S2归档／物理UUID／原Outbox／global责任及本人状态原意图已接源码 | 页面可见约5秒轮询，隐藏停止；restore晚回context围栏有41旧规则，S2另35host通过；最终双HAP与37方法test-HAP仅编译，RDB／AssetStore／家族隔离未设备验证 | [页面](../../代码/前端/HarmonyOS/entry/src/main/ets/pages/Index.ets)、[协调器](../../代码/前端/HarmonyOS/entry/src/main/ets/Data/AuthCoordinator.ets)、[存储](../../代码/前端/HarmonyOS/entry/src/main/ets/Data/Store.ets)、[原生服务](../../代码/前端/HarmonyOS/entry/src/main/ets/Data/NativeServices.ets) |

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、功能入口与跨端行为盘点 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

下表的“已接”表示相应源入口存在；实际通过范围仍以第一节报告为准。所有普通发送原意图先入本地库再 HTTP，保持原 ID／原 body／原 sendAdmission；明确资格失效永久冻结，网络未知不换 ID。界面演示数据与生产意图隔离，不借演示回退绕过鉴权或制造发送成功。

| 能力 | Android／iOS | Web／HarmonyOS | CLI | 默认、持久化及生命周期／差异 |
| --- | --- | --- | --- | --- |
| 登录、注册、OTP、刷新／重认证 | 原生认证表单、恢复入口；整对与未知链已接 | Web表单原证明受锁保护；Harmony协调器已接，41旧host与S2最终双HAP／37方法编译分列、设备待验 | 登录命令／TTY，原始 nonce 与完整退休链 | 仅 `ACCESS_EXPIRED` 刷新；实时 `auth_refresh`／`auth_retirement_chain` 门禁；旧 RT 未知绝不复用，改密码／OTP 走显式新证明关联退休旧链 |
| 本人主动改密 | 客户端待接，不以登录／恢复入口代替 | Web／Harmony入口待接 | [CLI主动改密](../测试/CLI/20261116-本人主动改密/README.md)7150：97本地／0失败／2环境跳过及Mac构建；匿名原单核对、原family显式重试、发前cap／身份复核已接，无真实API | 后端[password_change.go](../../代码/后端/Go/password_change.go)／[worker](../../代码/后端/Go/password_change_worker.go)，当前源码只读cap `own_password_change=true`；当前本人密码验证、管理员新OTP、原nonce结果纯查，成功撤销全families并退休旧恢复组／reset grants，不新建登录。4纯＋12真PG限定范围见最终原件；客户端及真实HTTPS／独立恢复待验 |
| 会话／历史／草稿／未读 | 会话／聊天页；本地草稿与未读，以及S2本人状态／云草稿入口 | Web本人状态与Harmony Index／SelfState已源码接线；Web新独立恢复仅10注入／构建 | conversations／messages／sync及本人状态命令 | 权威快照／增量与覆盖清单已实现，S2加入read／view／字号／文本draft；只覆盖声明数据域，本人读点不是公开已读或可靠收讫。未覆盖流与完整I继续，不能再笼统标为无同步底座 |
| 快照／增量消费（尚未全端验收） | Android5774：11 JVM／23原生／3真实HTTPS与UI；iOS：固定30原生，真接口已跑／UI未过；新31原S2＋10导出共41本地通过；完整UI／真HTTPS仍未复验 | 旧Web D5：11真／26本地；新D_uZ6muW恢复组件10注入／构建未真。Harmony35host／41旧，双HAP／37方法仅编译 | 历史15a4：55本地／13真TTY与HTTPS；eb42对象80和13cd导出88为各自本地／Mac构建，不增加同步真实服务范围 | 同DB存物理UUID、原请求、完整refs、guard、正文及连续cursor；完整页＋finalvalidate＋首次权限changes后发布。原Outbox须完整body／route只读200且事务再核；unknown／retained与global分别处理；逐端真实范围以第一节为准，qualified ACK／after_receipt未闭环 |
| 业务global／迟到媒体围栏 | Android Room9同协程IMMEDIATE writer及原生global／retained用例；iOS GRDB同类规则纳30原生，新HTTP另验 | Web D5有RAM／耐久清理／共享ledger媒体围栏；Harmony源码已接retained／跨holder global／cleanup，35host含对应反例，未设备 | 15a4真验覆盖global、peer同CID不误确认、退群保留普通历史及可信旧device清理；新收讫源不借此追认 | 原体未获查单证明继续保留，不猜clientID删正文；物理断电和各端未覆盖媒体／UI场景仍未验，外部显式下载不远程擦除。Web [GlobalMessages](../../代码/前端/Web/Src/GlobalMessages.ts)、[MessageFacts](../../代码/前端/Web/Src/MessageFacts.ts)、[PrivateMedia](../../代码/前端/Web/Src/PrivateMedia.tsx)各轮证据独立 |
| 正文／对象可靠收讫 | 当前不因同步／已读自动成为合格ACK；原件file／dir fsync尚无本端对象闭环证据 | Web保持浏览器传输存储边界；Harmony未设备验证，不签发删除资格 | eb42候选80本地／Mac构建已过：FULL准备记录、原件count／SHA／file＋dir fsync、描述＋原收据同事务、global精确清理和晚到拒绝；无真实批准API | 后端9f3b的7纯／9真PG及构建有独立限定证据。旧SQLite-only资格不升级file-fsync资格，独立objectComplete可为true但不等于冻结targets／累计ACK；targetRangeComplete／deletionEligible及after_receipt仍false |
| 好友申请与关系治理 | 联系人／管理页；仅本人待处理入站列表 | 联系人视图／管理入口 | request／accept／reject／relationship／delete／block／unblock／reopen | 原 request/reopen 操作 ID 持久；删好友不等于关门禁；reopen 仅本人显式新代际开放，不自动成为好友；治理前冻结对应旧 direct 队列 |
| 群管理 | 成员页／管理页：加人、移除、改名、转让、退出 | 群管理面板／Index 动作 | members／add-member／remove-member／rename／transfer／leave | remove/leave用原membershipGeneration，owner另以roleGeneration围栏；离群撤云权限并冻结旧队列，已接同步端按retained／global分类处理合法本地普通历史；具体未验端不视为通过，完整禁言／解散仍缺 |
| 文本与附件 | 原生输入、系统选择／保存／打开 | 浏览器文件、Harmony 文件与系统预览 | 文件路径上传／下载 | text/image/audio/video/file 已接；鉴权媒体 URL 同源固定路径；Android 单附件上限 64MiB，Harmony 当前 UI 25MiB 与后端 64MiB 有差异；不把系统预览写成内置音视频录制／播放器 |
| 回复、编辑、撤回、反应、置顶／收藏 | 消息动作入口 | 消息动作／菜单 | 相应命令 | 原消息 ID 和内容版本操作；contentUnavailable 显示占位，不冒充用户删除；作者擦除入口仍需保留；撤权清缓存正文和 sidecar |
| 搜索／收藏分页与原文定位 | 独立查找／收藏页，可重载和空态 | Web 查找页、Harmony 查找入口 | search／favorites／精确 message 读取 | 独立签名游标绑定本人／epoch／条件，原样回传；选择结果重新精确 GET 和会话鉴权；不把临时 forward proof 展示／普通导出 |
| 结构化 @ 与群公告 | 编辑器成员选择、owner @all；公告入口 | 对应编辑器／公告面板 | 结构化 mentions／announcement 命令 | 原名单／all 和原准入先存，不回填；公告原 ID、CAS、member＋role＋contentVersion 无正文意图持久；未知先原操作 GET，旧结果不覆盖当前；缺 role_admission 不写 |
| 正式引用转发 | 原文重鉴权→选目标→确认；原 body 固定 | 同样引用式入口与投影围栏 | forward 原意图与只读对账 | forwarding 门禁；源 proof／内容版本／目标准入原样保存；临时占位后旧同 version history/ack 不解锁，新精确 GET 可恢复；永久源失效由 server 状态／version保证；普通导出只占位 |
| 静态头像／举报 | 设置头像与原文件操作；消息举报及本人列表 | 对应设置／举报入口 | avatar／report 命令 | PNG/JPEG 原文件＋原 op ID；未知重试原操作，current=false 后新 GET me；举报不等于已交付申诉闭环 |
| 本地通知 | 系统本地通知，默认无正文 | 浏览器／Harmony 本地通知入口 | 无 GUI 系统通知入口 | 默认关闭，仅客户端活跃同步后提醒；后台 APNs／FCM／厂商推送未交付，第三方推送后续排期 |
| 设备撤销／恢复码 | 设置页 | 设置／安全视图 | devices／revoke／recovery 命令 | family/device 撤销围栏已接；不代表完整 MFA 开通、轮转、全部账号安全流程已验收 |
| 备份／正式导出／注销／迁移 | iOS导出App89e02736／test94a1cbb8完整编译通过，123输入核齐；10导出与31原S2共41本地通过，0失败／跳过，无真实导出／保存；Android导出App1588／test7682构建及16 JVM已过，5原生方法仅编译／未API，不借旧S2包通过 | Web导出18 Chrome注入＋TS类型＋a53生产包通过，真实HTTPS／最终保存未验；Harmony导出源已接，首15host通过、追加18目标未验，HAP／设备未验 | data-export七命令、原创建／取消body耐久与纯查、RAM票据／ZIP流及不覆盖发布已接；13cd完整88本地／Mac构建，无真实API | 后端2b58正式当前资料／状态／举报／消息及全媒体已限定完成，forward占位、无秘密或原发送恢复库。普通本地JSON及旧实验协议不替代正式入口；账号注销／同实例同账号普通迁移仍I |

功能源码导航：[Android 模块](../../代码/前端/Android/README.md)、[iOS 代码地图](../../代码/前端/iOS/代码地图.md)、[CLI 代码地图](../../代码/前端/CLI/代码地图/README.md)、[Web 代码地图](../../代码/前端/Web/代码地图/README.md)、[HarmonyOS 模块](../../代码/前端/HarmonyOS/README.md)、[后端契约](../../代码/后端/Go/接口契约/README.md)。表中未列出的录音录像、支付、地图、RTC、产品 SDK、E2EE 等不得由一个通用附件入口推断已实现。

## 四、<span id="phase-one-slices">第一阶段最短补齐切片</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 优先序 | 可执行合同切片 | 需要实际交付／验证 |
| --- | --- | --- |
| 1 | S2逐端真实与未覆盖同步域 | 后端快照／增量、本人read／view／字号／文本draft及真实恢复已接；Android5774真3、旧Web D5真11、旧CLI15a4真13分别成立。iOS新sim真接口已跑／完整UI未过，新档案交接修订的31本地通过、跨进程完整UI未过，Harmony已35host／双HAP编译但设备未验；Web新恢复候选仅10注入／构建。继续逐端系统／网络场景及coverage外数据域，保留固定57／7ef旧证据 |
| 2 | 合格ACK／对象／目标与保留策略 | CLI对象eb42的80本地／Mac构建已过，含preparation／global／晚到围栏；13cd导出88不计入收讫真实验证。后端9f3b有7纯／9真PG／构建，无收讫HTTPS／端文件耐久实链。补真实批准／原件交付、冻结targets／累计ACK、撤权／恢复及保留worker，达到原保护条件后才可启after_receipt |
| 3 | 正式导出五端接线与真实交付 | 后端2b58已限定完成；Web18注入＋TS／a53构建、CLI88本地／13cd Mac构建成立，无本轮真实HTTPS；iOS App／测试包编译与31＋10共41本地通过，真实导出／保存未验，Android已16 JVM／主与测试包构建、5原生仅编译，Harmony导出源已接，首15host通过、追加18目标未验，HAP／设备未验。继续原nonce查单／取消／失败／短票／鉴权ZIP的逐端真实交付，forward仅占位 |
| 4 | 主动改密客户端与真实闭环 | 后端4纯＋12真PG及62d3构建／94输入已封存；接当前本人验证、管理员OTP、公开nonce／context持久且密码对仅RAM、原结果纯查与未知续跑、成功全family撤销／新登录；补HTTPS／独立恢复／五端验证，不用恢复码或恢复入口冒充 |
| 5 | 账号注销 | 本人验证、等待期／撤销、受限查询、worker／journal、设备与凭据撤销、对象清理及恢复拒旧真实闭环 |
| 6 | 普通端迁移 | 同实例／同账号／同来源迁移认证、可恢复传输及端本地事务；原 ID 和空间交接、ACK／撤权；与后期 E2EE 密钥迁移分开验收 |

这些是交付排序，不是减少需求范围。完整群治理、陌生人／隐私、举报申诉、MFA 管理等既有 I 项仍须逐项对照[实现进度](../实现进度/README.md)和验收表继续补齐。

## <span id="jobs-toc-heading-6" data-jobs-toc="v2">五、平台验证缺口与维护规则 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

- 平台验证单独排期：Android／iOS物理真机、后台与系统生命周期；Harmony双HAP已构建，但签名／HDC安装／Asset Store／RDB／真实HTTPS未设备验证；CLI正式macOS entitlement及Windows／Linux本机安全库；Web旧D5真11与新恢复候选10注入各自封存，新候选真实联调及手机浏览器矩阵待验。它们不能遮盖第四节的产品合同缺口。
- 后续 App 修改必须以本表为起点，在同一任务修改相应行与实际证据链接；尚在编译／测试的行保留“检查点／未验／待修”，通过后再填准确构建与范围。
- 原始 JSON／PNG／log／包和旧 SHA 清单是历史证据；报告迁移只重写导航，并归档原导航字节。新导航另封清单，不修改 JSON 内原 sourceSHA／旧路径的历史含义。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
