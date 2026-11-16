# <span id="前言">Android 原生测试</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、入口与模式](#jobs-toc-heading-1)
- [二、测试来源](#jobs-toc-heading-2)
- [三、当前证据](#jobs-toc-heading-3)
- [四、本地 HTTPS 与截图](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、入口与模式 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

从仓库根运行：

```sh
测试/前端/Android/运行测试.command/运行测试.command unit
测试/前端/Android/运行测试.command/运行测试.command emulator
测试/前端/Android/运行测试.command/运行测试.command device
```

`check` 检查 Java/Gradle 与 SDK；`unit`／`build` 编译 APK 并运行 JVM 测试。`emulator`／`device` 只使用已经启动／连接且 adb 已授权的对应目标，安装并运行仪器测试，不下载镜像或改其它设备。多个目标使用 fzf 方向键／回车选择，Esc取消；无交互可用 `ANDROID_SERIAL` 指定已枚举的目标。真机须开发者模式和 USB 调试，并在设备确认授权；模拟器由 Android Studio Device Manager 启动。Ctrl+C 终止本次测试，不卸载用户其他应用。

Gradle connected 测试可能安装后清理本测试包；执行只使用独立测试实例，不在个人正式账户上测试。日志与截图生成在本目录 `报告/`，失败保留非零退出码；APK另导出到工程 `Dist/`。报告不包含账号密码、令牌或消息秘密。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、测试来源 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

- [Unit/IsolationTest.kt](./Unit/IsolationTest.kt)：不同可信连接档案隔离、演示禁止正式发送、拒绝不安全／带秘密URL、十语词条完整，以及恢复代际变化不重发旧Outbox，以及证书输入在整文件分配前受大小限制。
- [Instrumentation/DatabaseIsolationTest.kt](./Instrumentation/DatabaseIsolationTest.kt)：在实际 Android RDB环境验证 Room 正式与演示分库、pending保持，并检查 SQLite synchronous=FULL。它不等于断电、清云或灾备资格验证。

Gradle通过 `sourceSets.*.kotlin.directories` 引用以上唯一外置源，不在业务目录复制。

可选 `live-emulator`／`live-device` 另执行真实HTTPS双账户业务，必须显式提供 `JOBS_IM_TEST_URL`、`JOBS_IM_DEPLOYMENT_ID`、`JOBS_IM_CA_FILE`，仅用于隔离测试服务器；会创建带android前缀的测试账户和会话。默认设备模式的LiveApiTest没有服务参数时跳过，不能记为联调通过。

- [LiveApiTest.kt](./Instrumentation/LiveApiTest.kt)：已核对CA／主机／部署的原生HTTPS、默认不信测试CA应拒绝、注册／好友／私聊幂等／编辑／反应／置顶／收藏／上传／撤回与退出。

- [StorageLifecycleTest.kt](./Instrumentation/StorageLifecycleTest.kt)：真实 schema2→8 升级保留原 payload／ID／epoch，消息／游标事务失败回滚、超大数字字符串、待发对账、新草稿保护、未读去重及隔离数据库关闭重开；匿名重新连接也冻结原 profile／deployment／旧 epoch 的所有账号待发。
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

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、当前证据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

访问／刷新家族及新CA定向结果见 [访问刷新限定报告](./报告/20261115-访问刷新/README.md)；本轮同包6 JVM＋10仪器通过，原始失败与中间轮保留，不与旧轮相加。引用式转发与当前受影响回归见 [引用转发限定证据](./报告/20261115-引用转发/README.md)。此前搜索协作和角色资格的定向证据见 [搜索协作限定证据](./报告/20261115-搜索协作/README.md)。原生功能的 6 JVM／8 仪器基线与限定截图保存在 [20261115 基础闭环](./报告/20261115-基础闭环/README.md)；此前发送准入围栏源码／APK 的实际结果见 [发送准入限定证据](./报告/20261115-发送准入围栏/README.md)。旧报告保持当时范围，不把跳过的 live 测试计作通过。未真机验收，尚未证明全部页面、十语／主题／字体组合、断电／灾备或完整 I 业务通过。

[被测工程](../../../代码/前端/Android/README.md) · [本端代码地图](../../../代码/前端/Android/代码地图/README.md)。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、本地 HTTPS 与截图 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

Android 模拟器中的 127.0.0.1 属于设备本身。若隔离服务只监听宿主回环地址，须对已确认的专用目标显式设置 `adb -s <serial> reverse tcp:8443 tcp:8443`，再使用证书 SAN 覆盖的地址；测试不改系统信任或全局代理。该路径不证明手机局域网／公网可达。

手动 Gradle 运行并需保留本测试包／截图时，可加 `-Pandroid.injected.androidTest.leaveApksInstalledAfterRun=true`；不要在个人正式账号设备上执行。NativeUiTest 图片位于本测试应用专属外部文件目录 `UiEvidence/`；搜索协作原生截图为 `DiscoveryUiEvidence/`，转发为 `ForwardUiEvidence/`，与对应测试 XML／APK 指纹一起保存。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
