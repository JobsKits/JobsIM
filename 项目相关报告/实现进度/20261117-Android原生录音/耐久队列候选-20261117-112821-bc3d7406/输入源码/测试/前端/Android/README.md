# <span id="前言">Android 原生测试</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

## 一、入口与模式 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[当前全项目回归](../../../项目相关报告/测试/回归/20261116-全项目回归/Android-HarmonyOS.md)在原生导出十语文案调整后，离线执行16 JVM（0失败／错误／跳过）及应用／测试APK构建。新应用3e72d039、测试76826edf及XML只归档到本轮回归目录，旧e295／5774与原报告保持各自范围；没有ADB目标，本轮没有仪器、系统保存或真实HTTPS执行。

从仓库根运行：

```sh
测试/前端/Android/运行测试.command/运行测试.command unit
测试/前端/Android/运行测试.command/运行测试.command emulator
测试/前端/Android/运行测试.command/运行测试.command device
```

`check` 检查 Java/Gradle 与 SDK；`unit`／`build` 编译 APK 并运行 JVM 测试。`emulator`／`device` 只使用已经启动／连接且 adb 已授权的对应目标，安装并运行仪器测试，不下载镜像或改其它设备。多个目标使用 fzf 方向键／回车选择，Esc取消；无交互可用 `ANDROID_SERIAL` 指定已枚举的目标。真机须开发者模式和 USB 调试，并在设备确认授权；模拟器由 Android Studio Device Manager 启动。Ctrl+C 终止本次测试，不卸载用户其他应用。

Gradle connected 测试可能安装后清理本测试包；执行只使用独立测试实例，不在个人正式账户上测试。日志与截图生成在工程根 `项目相关报告/测试/前端/Android/`，失败保留非零退出码；APK另导出到工程 `Dist/`。报告不包含账号密码、令牌或消息秘密。

## 二、测试来源 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- [OwnPasswordRulesTest](./Unit/OwnPasswordRulesTest.kt)：4项JVM覆盖NFC／码点／边界、拒绝复用、未知／changed家族门禁、秘密诊断脱敏及十语键；[OwnPasswordLifecycleTest](./Instrumentation/OwnPasswordLifecycleTest.kt)另有4项真实Room适配源码，覆盖重建、原责任无秘密、只读查单、错误证明、原家族／新家族隔离与迟到回包。仪器方法本轮仅编译，不写成设备已执行；[独立报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS改密.md)保留实际命令与当前包。

- [Unit/IsolationTest.kt](./Unit/IsolationTest.kt)：不同可信连接档案隔离、演示禁止正式发送、拒绝不安全／带秘密URL、十语词条完整，以及恢复代际变化不重发旧Outbox，以及证书输入在整文件分配前受大小限制。
- [Instrumentation/DatabaseIsolationTest.kt](./Instrumentation/DatabaseIsolationTest.kt)：在实际 Android RDB环境验证 Room 正式与演示分库、pending保持，并检查 SQLite synchronous=FULL。它不等于断电、清云或灾备资格验证。

Gradle通过 `sourceSets.*.kotlin.directories` 引用以上唯一外置源，不在业务目录复制。

可选 `live-emulator`／`live-device` 另执行真实HTTPS双账户业务，必须显式提供 `JOBS_IM_TEST_URL`、`JOBS_IM_DEPLOYMENT_ID`、`JOBS_IM_CA_FILE`，仅用于隔离测试服务器；会创建带android前缀的测试账户和会话。默认设备模式的LiveApiTest没有服务参数时跳过，不能记为联调通过。

- [LiveApiTest.kt](./Instrumentation/LiveApiTest.kt)：已核对CA／主机／部署的原生HTTPS、默认不信测试CA应拒绝、注册／好友／私聊幂等／编辑／反应／置顶／收藏／上传／撤回与退出。

- [StorageLifecycleTest.kt](./Instrumentation/StorageLifecycleTest.kt)：当前目标schema2→9升级保留原payload／ID／epoch（本轮未执行此类，历史2→8证据独立），消息／游标事务失败回滚、超大数字字符串、待发对账、新草稿保护、未读去重及隔离数据库关闭重开；匿名重新连接也冻结原 profile／deployment／旧 epoch 的所有账号待发。
- [ModelClosureTest.kt](./Instrumentation/ModelClosureTest.kt)：真实 HTTPS 的原生 ViewModel，收件箱／本人关系、群成员资格代际、只读门禁、消息／头像／举报丢响应后原操作重试、current=false 当前资料、附件鉴权下载、设备撤销和 ViewModel 重建后的 Keystore 恢复；并发轮询／打开会话去重及发送期间新草稿保留。
- [NotificationPrivacyTest.kt](./Instrumentation/NotificationPrivacyTest.kt)：实际系统通知的命名空间隔离与正文隐藏，测试完恢复自己的通知设置。
- [NativeUiTest.kt](./Instrumentation/NativeUiTest.kt)：真实 Activity 的冷启动、聊天／群成员／设置、原生明暗／跟随系统截图；仅测试自己的隔离账号。

- [AdmissionFenceTest.kt](./Instrumentation/AdmissionFenceTest.kt)：原生持久待发在错过退出／重入并冷恢复后，仍用原 sendAdmission／clientMsgId 对账并永久冻结；旧行不补新资格，旧服务缺强制能力时不 POST，对当前新意图和同 ID 改 token 冲突作真实验证。
- [DiscoveryLifecycleTest.kt](./Instrumentation/DiscoveryLifecycleTest.kt)：真实 51 条搜索／收藏的 50＋1 分页，不透明游标、精确源编辑读回、目录遗漏会话、退出重入旧原文拒绝及账号切换晚回包隔离；目录遗漏由测试传输层过滤，不冒充真实创建 101 会话。
- [CollaborationLifecycleTest.kt](./Instrumentation/CollaborationLifecycleTest.kt)：提及草稿重建、原消息丢响应同 payload 重试、成员提及与 @所有人权限、contentVersion 公告源、原公告 current=false 不回滚及编辑失效。
- [RoleAdmissionLifecycleTest.kt](./Instrumentation/RoleAdmissionLifecycleTest.kt)：真实 owner→member→owner 原角色资格永久失效，旧 @所有人及公告不补新资格；无能力／旧缺角色记录只读原结果，不 PUT／DELETE。
- [DiscoveryCollaborationUiTest.kt](./Instrumentation/DiscoveryCollaborationUiTest.kt)：原生收藏与搜索、精确原文／提及显示、点击成员选择发送、公告发布／清除及持久主题设置；以服务端事实断言操作结果，截图范围见对应报告。
- [ForwardLifecycleTest.kt](./Instrumentation/ForwardLifecycleTest.kt)：真实引用转发原 body 先落盘、HTTP 前离线／真实提交后丢响应、只读核对与明确原发送重试、来源编辑重选、源成员重入旧资格永久冻结、缺能力禁止创建、暂时同版本未知投影去正文及文件来源撤回后禁止下载。
- [ForwardUiTest.kt](./Instrumentation/ForwardUiTest.kt)／[NativeForwardActions.kt](./Instrumentation/NativeForwardActions.kt)：实际原生目标选择、未知结果核对与显式原 ID 重试，服务端事实断言及深色截图；操作器只按可见原生节点点击，不调用业务方法代替点击。
- [ApiFixtures.kt](./Instrumentation/ApiFixtures.kt)：只为隔离测试的新意图读取当前 Conversation 快照，不供业务代码调用。

- [NativeSessionLifecycleTest.kt](./Instrumentation/NativeSessionLifecycleTest.kt)：真实 Keystore／AtomicFile及Room关闭重开，12协程单航班、保存失败与unknown不重用旧RT、已存安全回执续交接、旧设备Outbox及晚入队冻结、原认证链不可回填、旧401不能借新family；网络部分明确为受控传输夹具。
- [NativeSessionHttpsTest.kt](./Instrumentation/NativeSessionHttpsTest.kt)：独立新CA真实401期限夹具刷新、真实提交后丢响应与本人重新验证、R0已成／R1未POST／R2完整闭包退休、原生明暗按钮与安全库冷恢复。只能通过[原生刷新定向入口](./native_refresh.py/README.md)或等效受控host协调器运行；普通live参数不会自动启用它。


- [Migration8To9Test.kt](./Instrumentation/Migration8To9Test.kt)：使用 [V8原件](./InstrumentationAssets/JobsIM-schema8.json)及[三方来源](./InstrumentationAssets/Schema8来源.json)，只在独立UUID库按原createSql／索引／setupQueries建V8；真实BundledSQLite走既有8→9。四表原列及所有TEXT逐字节比较，原历史／草稿／pending forward原体／frozen／原操作／native_scope保留；received forward按既定策略遮蔽；新sync表空、无原资格继承，显式新登记仍未ready。schema8只在testAPK，不进入产品资源。

## 三、当前证据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

正式本人导出新增[5个JVM文件／ZIP反例](./Unit/PersonalExportFileTest.kt)已过；当前全部16 JVM／0失败／0跳过（6旧Isolation＋5旧SyncWire＋5新导出），主包1588c450与test76826edf构建通过。[5个Room／模型原生注入方法](./Instrumentation/PersonalExportLifecycleTest.kt)仅编译，未安装／执行，独立输入及首轮编译FAIL见[本轮报告](../../../项目相关报告/测试/前端/Android/20261116-正式本人导出/README.md)。不借旧5774的11／23／3追认；源码含CreateDocument入口，实际选择／取消、URI保存、文件／目录fsync及HTTPS均须另约原生窗口，本轮不启动AVD或自动连服务。

S2 本人状态／云草稿已完成[限定同源验证](../../../项目相关报告/测试/前端/Android/20261116-S2本人状态/README.md)：JVM11、同包原生23（原10＋SelfState13）及新E2真实HTTPS／UI3分别通过、0失败／跳过，不跨轮累加。产品5774／测试7067安装读回一致；真实6family均明确关闭。5张截图已实看，其中3张过渡帧不作稳定终态视觉证明；旧失败均保留。下述旧10项报告仍指978原候选，不追认本轮新包。

当前权威同步应用／测试APK已构建，JVM10项（新增4＋原6）与仪器10项（8同步＋1旧Api身份＋1V8迁移）分别通过、无失败／跳过。新增 [SyncWireTest.kt](./Unit/SyncWireTest.kt) 的4个JVM反例和 [AuthoritativeSyncTest.kt](./Instrumentation/AuthoritativeSyncTest.kt) 的8个原生注入方法，另在 NativeSessionLifecycleTest 增加旧 Api 对象不能借新 family 的1个反例；另有 [Migration8To9Test.kt](./Instrumentation/Migration8To9Test.kt) 的1项真实迁移。原9与新增1在最终同包整组10项重跑，不相加旧轮计数。覆盖原始 bytes SHA／UTF8、opaque client ID、缺页／原 nonce 重启、原 body＋route proof CAS、持久 restriction、跨device／retained global终态、forward占位，以及辅助查单503／网络未知／不完整200不阻塞已验同步，以及commit后中断、Room重开、空增量续清与自有媒体删除失败重试，以及精确GET／业务回执的deleted在sync事件前保护所有holder。所有 API 都是进程内注入，不声称实际 HTTPS／NATS 通过。

StorageLifecycleTest 当前源码已改为 history 同 clientID 不自动清 Outbox，迁移目标为 schema9；下列历史报告的 schema2→8、6＋10等原结果仍只证明其原输入／原APK。[权威同步检查点](../../../项目相关报告/测试/前端/Android/20261116-权威同步/README.md)保存旧认证输入、失败原raw与最终同包证据；新增有界[构建入口](./sync_build.py/README.md)及[定向仪器入口](./sync_instrument.py/README.md)，执行前须协调磁盘与既有原生工具窗口；不要覆盖 canonical e295 包来冒充本轮结果。

访问／刷新家族及新CA定向结果见 [访问刷新限定报告](../../../项目相关报告/测试/前端/Android/20261115-访问刷新/README.md)；该认证历史包6 JVM＋10仪器通过，原始失败与中间轮保留，不与旧轮相加。引用式转发与当前受影响回归见 [引用转发限定证据](../../../项目相关报告/测试/前端/Android/20261115-引用转发/README.md)。此前搜索协作和角色资格的定向证据见 [搜索协作限定证据](../../../项目相关报告/测试/前端/Android/20261115-搜索协作/README.md)。原生功能的 6 JVM／8 仪器基线与限定截图保存在 [20261115 基础闭环](../../../项目相关报告/测试/前端/Android/20261115-基础闭环/README.md)；此前发送准入围栏源码／APK 的实际结果见 [发送准入限定证据](../../../项目相关报告/测试/前端/Android/20261115-发送准入围栏/README.md)。旧报告保持当时范围，不把跳过的 live 测试计作通过。未真机验收，尚未证明全部页面、十语／主题／字体组合、断电／灾备或完整 I 业务通过。

[被测工程](../../../代码/前端/Android/README.md) · [本端代码地图](../../../代码/前端/Android/代码地图/README.md)。

## 四、本地 HTTPS 与截图 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

Android 模拟器中的 127.0.0.1 属于设备本身。若隔离服务只监听宿主回环地址，须对已确认的专用目标显式设置 `adb -s <serial> reverse tcp:8443 tcp:8443`，再使用证书 SAN 覆盖的地址；测试不改系统信任或全局代理。该路径不证明手机局域网／公网可达。

手动 Gradle 运行并需保留本测试包／截图时，可加 `-Pandroid.injected.androidTest.leaveApksInstalledAfterRun=true`；不要在个人正式账号设备上执行。NativeUiTest 图片位于本测试应用专属外部文件目录 `UiEvidence/`；搜索协作原生截图为 `DiscoveryUiEvidence/`，转发为 `ForwardUiEvidence/`，与对应测试 XML／APK 指纹一起保存。



本轮追加能力撤回与恢复代际反例：旧原单允许匿名纯查询；全部续跑必须重新通过能力门禁，404 未建立原单须由原家族 AT 创建。更新验证为鸿蒙15项主机通过、两端应用与测试包重新编译；Android新增第5项 Room 反例仅已编译，未执行设备测试。


当前新增`Unit/OwnDeletionRulesTest.kt`4项和改密第5项规则反例，全部JVM合计25项通过。`Instrumentation/OwnDeletionLifecycleTest.kt`新增5项实际Room／缓存清理反例，改密instrumentation增至6项并改正`JobsIM-test-`专用DB名；11个新增方法均在test APK DEX核到，尚未设备执行。原始日志和包见[两端注销报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS注销.md)。


## 群治理当前候选

[GroupRulesTest](./Unit/GroupRulesTest.kt)新增3项JVM（精确正整数／邀请诊断脱敏／十语完整），当前全量28项通过、无失败／错误／跳过。[GroupLifecycleTest](./Instrumentation/GroupLifecycleTest.kt)7项真实Room适配源码覆盖转让确认后不重基、未知邀请纯查／token不落盘、冷启动同码同nonce、迟到身份、角色撤回清RAM；这7项仅编译，未运行设备。应用／test APK和原始日志、源码映射归档到[群控限定报告](../../../项目相关报告/实现进度/20261116-剩余能力闭环/Android-HarmonyOS群控.md)；旧Dist不改，不能由编译推断UI、RDB生命周期或真实HTTPS通过。


2026-11-17后增取消门禁源码：unknown取消先纯查询，同原cancelVersion才允许用新证明明确续同nonce；旧缺版本记录与查询新版都保持只读，不静默补资格。十语新增先查／冻结说明。新增Harmony4主机反例（目标16）、Android4 Room方法（目标9）及Hypium3方法（目标57）未运行／未编译，受低于4GiB磁盘门禁限制；旧APK/HAP及先前12主机结果不追认该增量。见`项目相关报告/实现进度/20261116-剩余能力闭环/注销证据/取消原版本新增源码待验.json`。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
