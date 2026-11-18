# <span id="前言">Android 原生录音闭环</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、范围与入口 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本片补[FE-028、FE-069](../../../需求/IM功能验收表.md/IM功能验收表.md)的 Android 前台录音入口，不将通用附件上传当作录音实现。聊天编辑区麦克风按钮打开原生录音面板，沿用当前 Compose 主题、字号及十语。采用现有暖白／墨黑／松绿与 Material 3 控件；录音任务需要保持麦克风状态和发送确认，使用可取消的原生对话框，不新增独立导航或视觉素材。

**当前交付状态：直接录音初始候选有绑定源码的33项JVM及双APK；后续耐久队列与审查修复已落源码，最新状态尚未编译／运行通过。** 新源码、工程入口和最终可运行产物必须一致；旧APK不代表最新队列已交付，不据此声称本片或整个基础版完成。

明确开始后请求麦克风权限；权限返回不自动开录，用户再次选择开始。录音时显示时长，可取消或停止；停止先验证音轨并进入本机试听，只有另行明确“发送语音”才上传。达到本片上限5分钟／8 MiB也只进入预览，不自动发送。短于1秒、无效容器、不支持设备、拒权及录音失败都有可操作反馈，文字聊天不依赖麦克风授权。

## 二、状态、文件与原身份 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 边界 | 本片源码行为 |
| --- | --- |
| 原录音归属 | RAM lease 固定本机 namespace、实例／恢复代际／设备／family、会话、成员 generation 及发送准入；不能换身份继续使用旧文件 |
| 生命周期 | 离开页面、系统返回、ON_PAUSE／ON_STOP、音频焦点丢失、耳机路由中断、API29及以上被系统静音，停止录制／试听并撤销原lease。后台不继续录音，不建前台服务，不自动恢复录制 |
| 暂存 | 仅应用私有 cache/VoiceRecordings 下随机命名 M4A；原件不进普通设置或 Room。取消、离开及入队后尝试删本轮暂存；新引擎／新录音仅清本模块无活跃持有者的旧文件，拒跟随符号链接。清缓存导致原件缺失时明确失败，不制造成功；删除失败不等于物理擦除通过 |
| 录制与试听 | 系统 MediaRecorder 输出单声道 AAC／MPEG-4；停止后读取最终音轨再提供 MediaPlayer 试听。试听仅控制本次未发录音，停止后再次播放从头开始，不冒充已交付消息的续点／倍速／未听标记 |
| 上传与入队 | 专用读入口只接受自有路径及活跃lease，显式 audio/mp4；用户选文件仍走原 readAsset MIME 规则，服务端仍独立识别。每次挂起后及 Room 同一 writer 事务前后复核原归属，成功后沿既有稳定 clientMsgId／原 body Outbox 与 submit 处理 |
| 失败与取消 | 入队前失败仅当前原lease可显式重试，不后台自动上传；离开即撤资格。入队后临时文件不再是业务唯一副本，原待发责任不因关面板而删除；“已入待发”不等于服务端接受／设备收讫／已读 |

聊天消息只有正式Room原意图耐久后才进入原发送流程。初始候选的旧媒体上传没有原对象对账，新队列改用下述stable-upload-v1；该新合同的真实HTTPS联调另验，不以接口设计宣称精确一次对象上传已实际通过。

初始131份输入候选曾有明确缺口：用户点“发送语音”后，POST /media与恢复代际核对完成之前只有cache和RAM lease，进程死亡会丢本次责任；媒体响应丢失也没有原对象对账。该候选33项JVM／双APK只归其原输入，不代表以下新队列已验证。

当前新增源码针对[FE-021保存附件队列](../../../需求/IM需求明细表.md/IM需求明细表.md)和FE-028上传失败恢复补独立 noBackup/VoiceUploads，不改Room schema：每份预览第一次明确发送即固定upload requestId／clientMsgId、字节SHA、当刻回复目标和稳定本人／实例／device／family／epoch。RAM generation仅保护当前执行，不入清单。归属header先耐久，随后原件file／dir fsync、AtomicFile最终manifest与目录发布同步成功后才允许HTTP；partial header不当完整任务、不准上传，保留字节供原任务恢复或明确清理。原件保存只显示“已保存待处理，尚未发出”。

后端适配按stable-upload-v1能力、同原八字段纯结果查询及可选nonce上传合同：重启／响应未知先查；prepared只有预留id，不是可用原件，404也不创建；只有用户明确恢复／同原体重试才再POST。committed核原name／size／SHA／audio/mp4／固定URL，objectId先耐久，再查原message结果，之后沿同clientMsgId／原body进入Room Outbox。消息入队未知不造新ID。正式路径不退旧无nonce上传；旧服务无能力时保留本地任务。原有HTTPS BaseURL子路径继续沿Isolation.endpoint原规则，清单namespace精确核同一owner组件，拒分隔符注入。

编辑区旁新增本机语音任务入口，只有当前本人／profile／实例的任务可见；有完整空态、重载、纯查询、明确恢复和未上传任务删除确认。旧family／epoch／owner任务只读冻结；恢复写入还须选中原会话并保持原准入，不静默改回复目标或发到另一会话。32个活跃任务、64 MiB总原件、16 MiB空间余量与128条本机任务记录是本轮工程默认；配额／空间／fsync失败拒HTTP，不删唯一原件。仅已确认完成或明确取消且无原件的本机终态记录可在记录上限时回收，不回收未知责任。

源码已实际接生命周期：OwnDeletionCleanup.erase在Room索引删除前精确清本profile/deployment/user全部epoch队列，失败保本地清理责任；SyncMediaCleanup.drain在completeCleanup前按已验证global的serverId及Room原sender/clientMsgId映射清对应源并fsync，peer同clientMsgId不误删。clearIdentity取消在途并持久冻结旧本人任务，connect／publishSession／logout沿原调用链进入；离开会话、后台只取消在途且保耐久原件。原工程没有独立clearProfile／clearLocalData产品函数，connect新建profile不会删旧数据；未将不存在的操作写成已接。

审查后追加持久拒写边界：只有账号真正completed的清理会先在同一queue FileLock下耐久`.account-erased-<归属SHA>`最小拒写marker及AtomicFile floor.json，然后清任务。floor只含schema／profile／deployment／user，不含录音、密码或凭据，不随正常退出／取消任务清除。prepare、状态更新、读上传原件、每个HTTP准入与Room入队前后均复核；floor与普通logout冻结不同，旧协程／旧进程不能在擦除完成后重新prepare或推广旧响应。marker先耐久，即使随后floor metadata失败仍保守拒写；本地擦除保持待完成并可续尾。

global清理匹配同时核serverId、会话、sender／clientMsgId及规范个人namespace归属；同scope旧设备／epoch／retained holder仍可清，不跨会话。尚未清掉的attachmentId必须等原objectId，已清字段不拿空text作归属证明。terminal状态耐久后若在unlink／目录同步前中断，后续列表加载只续本地清理，不发HTTP；失败显示pending，重载重试，不能因状态已终结就永久留下原件。

已验证global还会先耐久独立的最小消息拒旧floor（profile／deployment／user／conversation／clientMsgId及serverId，零正文／凭据）；即使普通终态任务清单后来被有界GC，旧进程也不能拿原seed重新prepare已清字节。该floor不由普通取消／完成生成，HTTP准入、原结果推广、入Room和普通audio Outbox再发送均复核；它与账号拒写floor一同保留，不当普通缓存回收。

## 三、源码与验证 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 位置 | 责任 |
| --- | --- |
| [VoiceRecordingComposer.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceRecordingComposer.kt) | 原生权限、录音／预览／明确发送面板及页面生命周期 |
| [VoiceRecordingEngine.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceRecordingEngine.kt) | MediaRecorder、焦点／系统静音中断、最终容器验证、试听与资源释放 |
| [VoiceRecordingRules.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceRecordingRules.kt) | 原身份、可撤销lease、状态及有界录音规则 |
| [VoiceRecordingFiles.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceRecordingFiles.kt) | 私有随机文件、自有孤儿回收与专用有界读取 |
| [VoiceRecordingText.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceRecordingText.kt) | 十语各22键，继承当前语言选择 |
| [MainActivity.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/MainActivity.kt)／[ChatModel.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/ChatModel.kt) | 编辑区一行入口；原身份快照与复用上传／Outbox的窄接线 |
| [AndroidManifest.xml](../../../代码/前端/Android/app/src/main/AndroidManifest.xml) | RECORD_AUDIO及可选麦克风设备能力 |
| [VoiceRecordingRulesTest.kt](../../../测试/前端/Android/Unit/VoiceRecordingRulesTest.kt) | 五个 JVM 源码用例：可发状态、身份／准入变化、已撤lease拒旧、边界及十语 |
| [VoiceRecordingNativeTest.kt](../../../测试/前端/Android/Instrumentation/VoiceRecordingNativeTest.kt) | 三个真实 MediaRecorder 仪器方法：AAC停止后PREVIEW不自动上传、取消只清自有原件、切family拒旧且不复活；初始候选仅已编译，未执行 |
| [VoiceUploadProtocol.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceUploadProtocol.kt)／[Queue](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceUploadQueue.kt) | 稳定归属、精确namespace、原body、私有文件／AtomicFile、互斥、容量及清理 |
| [VoiceUploadApi.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceUploadApi.kt)／[Coordinator](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceUploadCoordinator.kt) | typed stable-upload-v1、先纯查、明确原单重试、原message结果及Outbox交接 |
| [VoiceUploadScreen.kt](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceUploadScreen.kt)／[Text](../../../代码/前端/Android/app/src/main/java/com/jobs/im/VoiceUploadText.kt) | 本机本人任务列表、原会话显示、确认／冻结／空态／重载与十语各24键 |
| [VoiceUploadRulesTest.kt](../../../测试/前端/Android/Unit/VoiceUploadRulesTest.kt) | 七个新增纯规则方法，含namespace注入及精确global匹配反例；最新状态未执行 |
| [Queue仪器](../../../测试/前端/Android/Instrumentation/VoiceUploadQueueNativeTest.kt)／[Global floor仪器](../../../测试/前端/Android/Instrumentation/VoiceUploadGlobalFloorNativeTest.kt)／[API仪器](../../../测试/前端/Android/Instrumentation/VoiceUploadApiNativeTest.kt) | 12个真实文件与注入中断边界、4个typed HTTP夹具方法；最新状态未编译／执行，不冒充物理断电或真实HTTPS |

初始直接录音候选已有真实编译／JVM证据：[131份输入快照](./初始录音候选源码快照/输入SHA256.json)明确排除后续未接的 VoiceUpload 前缀模块，工程外副本执行前后SHA一致；本轮全 JVM **33通过、0失败、0跳过**（含5个新录音规则方法及原28个受影响基础规则），debug app与androidTest APK均实际编译，耗时66.465秒。见[真实结果](./初始候选编译-20261117-103046-c74bc250/结果.json)、[原始Gradle日志](./初始候选编译-20261117-103046-c74bc250/Gradle.log)及同目录JUnit XML。app SHA为`0401c3b2f1e333ad6b530e1c7e4b12008aaf386ecd12dbc1d50bc01856818ba4`，test SHA为`7cd8e53ce5c3df7140c27c2b3f523a2c75f6975a6a102083362e17c9e590a3ce`；未安装或替换用户应用。

本轮使用暖JDK／SDK／Gradle，offline、单worker、无常驻daemon；900秒超时和每秒4GiB空间监控，66次采样最低10,556,985,344字节。首次运行器预检误按android-37目录定位，而暖SDK实际为android-37.0，尚未启动Gradle便退出；[预检失败原件](./初始候选编译-20261117-102952-9a5234a9/结果.json)保留，修运行器路径判断后使用既有SDK，未下载或更改环境。

十语各22个非空标签、Manifest XML解析、受影响共享文件的 git diff --check及本报告相对链接静态检查通过。3个新真实录音仪器方法仅编译，**未执行、未以默认SKIP计通过**。设备权限弹窗、真实麦克风／AAC编解码、上限回调、电话／路由中断、后台／切本人／切实例／切会话、清缓存及真实 HTTPS 上传与重发均待本轮设备验证；未查看新面板截图。后续独立 VoiceUpload 持久队列／适配与其新增用例不在上述131份输入或APK中，须另封当前输入、编译和验证，不能追认这轮33项。

新队列首轮[142输入候选](./耐久队列候选-20261117-112821-bc3d7406/结果.json)真实编译26.117秒后因Android公开SDK无O_DIRECTORY常量失败，0测试、0APK，原件保留；主树改复用项目既有PersonalExportFiles.syncDirectory。随后[另封142输入复验](./耐久队列复验-20261117-113114-8d3d816e/结果.json)运行67.725秒，于KSP阶段因空间跌破4GiB由自身watchdog停止：exit143、68次采样、最低4,251,209,728字节、0测试／0APK，输入未漂移。该事实不回改初始候选已经完成的验证，也不能算作新队列编译通过。后加account floor／global精确归属／terminal清理续跑修复均不在这两轮冻结输入中；当前低盘，只执行廉价源码、标签和diff核查，等待新的完整验证窗口。

真实录音仪器用例默认跳过；只有运行器明确传 voiceCapture=true、可抛弃的目标安装已获麦克风权限、目标应用已在前台且设备有麦克风时才执行。测试本身不启动主页面、不修改用户账号／数据库、不自动授予权限或开启模拟器；缺前提的 SKIP 不能计作真实录音通过。运行器应先确认专用测试目标，再沿已有 Android instrumentation 入口限定 VoiceRecordingNativeTest。该专项验证原生引擎，不代替权限面板、完整 Compose 点击、声音质量及端到端发送验收。

FE-053与FE-028中的已交付音频续点／倍速／未听标记仍未由本片实现。AppLock、视频拍摄、RTC、外部推送和其他端录音不在本片范围。

官方依据：[MediaRecorder](https://developer.android.com/media/platform/mediarecorder)、[运行时权限](https://developer.android.com/training/permissions/requesting)、[音频输入共享与系统静音](https://developer.android.com/media/platform/sharing-audio-input)。本片按官方前台采集／释放要求处理，文档核查不替代设备实测。

应用的 Skills：jobs-frontend-experience 补齐开始／拒权／取消／预览／明确发送闭环；frontend-design 保留现有视觉层次；Impeccable Android／Operate 采用 Material 3 语义色、可滚动长文案与原生控件。因当前任务已有明确范围和设计基线，按子任务原范围推演，没有新增产品采访或更改设计系统。

<a id="🔚" href="#前言">回到前言</a>
