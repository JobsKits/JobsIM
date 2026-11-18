# JobsIM · iOS 原生客户端

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

## 🔥 <span id="前言">前言</span>

Swift／UIKit 独立工程。会话与消息是主要界面，正式账户和明确进入的演示空间隔离；当前实现切片不代表基础全部需求通过。产品需求在仓库的 `需求/`，本目录记录真实运行方式。

## <span id="native-run">一、运行</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

需要 Xcode 27.2（当前验证工具）、iOS 16+ SDK目标、Ruby／CocoaPods，以及同级 `JobsBaseConfig/JobsBaseConfig@JobsSwiftBaseConfigDemo` 中已安装的本人模板依赖。模板位置可通过 `JOBS_SWIFT_TEMPLATE` 显式指定；运行产物不依赖开发机绝对路径。

在本目录运行：

```sh
ruby Scripts/PrepareDependencies.rb
ruby Scripts/Build.rb <模拟器UUID>
```

打开 `JobsIM.xcworkspace`，选择 JobsIM Scheme。当前独立验证模拟器名为 JobsIM Native；不复用其他项目的模拟器。运行时在“服务器连接”填写 HTTPS 根地址并先验证实例身份，再注册／登录。Debug仅允许本机loopback HTTP联调，Release不会关闭TLS验证。普通HTTPS使用系统认可的证书。固定实例可显式导入PEM／DER CA并输入经独立可信渠道核实的SHA256指纹，确认后只保存到该URL的应用内档案；SecTrust仍校验主机名、证书有效期及证书链，重定向限定同实例API路径，不写设备系统信任。移除信任会退出当前凭据；换证书／实例建立新的本地隔离档案。扫码描述入口尚未交付。

无需服务器可在登录页主动进入“演示空间”，常驻身份标记及返回入口。也可以向调试启动传 `--demo`。正式网络失败显示离线并保留真实缓存、草稿和原ID待发，不将输入框换成样例。

## <span id="native-actions">二、当前操作</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- 登录／注册／账号恢复、固定实例验证、Keychain凭据、退出、独立原生设备页、当前设备标记与撤销本机后退出、账户恢复码生成和单次用途说明。新access／refresh接入范围与实测状态见第五节。正式请求确认撤权401后回认证入口，保留旧命名空间草稿／历史；旧令牌的迟到401不能退出新账户。
- 会话与联系人、用户名搜索／好友申请、收件箱接受／拒绝、双方删好友、拉黑／解黑及本人显式重开入站申请门禁。申请与reopen持久化原requestId，解黑不恢复好友；单聊、建群、成员邀请／移除、重命名、转让群主和退出均使用服务器结果，成员移除绑定看到的membershipGeneration。
- 收藏使用本人独立分页接口；消息搜索要求明确关键词，翻页沿服务端原cursor。选择结果后重新鉴权读取单个会话及精确消息，可定位会话列表前100条之外的来源，已撤回／失权内容不回退旧搜索正文。
- 文本、图片／动图、视频、文件、录音消息；系统选择器、鉴权上传／下载、图片正文预览及QuickLook。已鉴权普通媒体副本按账户／实例／epoch／消息隔离，只有传输失败可以回读；401／403／404和已知删除／不可取正文不回退，已见删除清理对应消息的媒体副本。编辑、回复、反应、收藏和置顶调用同一真实API。
- 普通转发仅在实例明确公开`forwarding=true`时提供。先GET重验所见内容版本和本人源proof，变化后须重新明确选择；目标再GET当前会话资格。Outbox只存原引用及两个原授权快照，不复制正文／附件／提及。未知结果先只读查原事实；仅用户明确“重试原转发”才可在404后沿原ID／原body提交，失效原proof不补新值。转发不可编辑，本人仍可撤回。
- 静态JPEG／PNG头像≤2MiB，持久化原发布操作，丢响应查询原事实并GET me重取当前资料，旧current=false结果不覆盖新头像；消息／用户／会话举报及本人记录，不自动附聊天正文。
- GRDB＋系统SQLite保存账户／实例／恢复代次／演示隔离的历史和草稿；消息增量正文及cursor同事务，旧版本不覆盖新版本／撤回。草稿只在Outbox提交成功后清空；Outbox先提交后联网，创建时冻结本人Conversation的原sendAdmission，未知结果沿原clientMsgId／restoreEpoch／sendAdmission重试或核对，不从刷新后的资格补写原载荷；代次变化归档旧空间并重新认证，不生成已读或合格删除ACK。
- 群聊可从当前成员中选择结构化提及；全体提及只向当前群主开放，并在发送前核验`role_admission`。提及名单在上传前及入队时冻结，文字中的`@`不自行变为提醒；提醒标记仅取服务端`mentionedMe`。群公告引用原消息`contentVersion`，群主发布／清除前冻结成员及角色代际和原requestId；丢响应只查询原操作，重启不自动重发。
- 简繁藏英日法西德俄韩共十语、白天／黑夜／跟随系统和小／标准／大字号。主要会话／消息／联系人列表有空态及重新加载；演示数据不进入真实队列。

前台持续分页拉取与本地未读落盘；进入会话只清本机未读，不发送已读或云清理资格ACK。系统本地通知须用户授权，默认隐藏正文，转发通知始终不含投影正文；通知与点击按当前账户／实例档案校验；后台停止主动轮询，不冒充APNs远程推送。只读会话禁止新入队并保留草稿；恢复后contentUnavailable显示独立占位，作者仍可依法向服务端请求撤回。转发缓存每次展示前GET当前投影，未知／失权时清正文、入口及媒体副本；附件仅接受同源固定转发路由，逐次验权且没有离线副本回退。本地JSON导出排除凭据／恢复码／Outbox和临时许可；转发只导出占位元数据，不携带投影正文或附件入口。不含媒体文件字节，不支持导入，不冒充服务端备份或受控迁移。共享面板结束后移除临时导出文件。

API使用统一 `/api` 契约；交互请求超时4秒，媒体120秒。当前会话列表5秒、消息3秒轮询，尚未实现完整长连接同步；消息cursor保留`restoreEpoch:sequence`原串并按十进制位数／字典序比较序号。业务页统一经JobsIMTransport／JobsNetworking公开请求接口；普通HTTPS用JobsDefaultAgent，实例CA用自有JobsAgent严格信任适配器；关闭网络日志，秘密只进平台Keychain，不进SQLite／UserDefaults／URL。头像统一先显示打包Logo，再异步鉴权加载当前授权URL；省略资料／下载失败／复用／切换账户立即回退Logo，旧图不会留在新行。SF Symbols来源及iconfont检查见 [DESIGN.md](./DESIGN.md)。

## <span id="native-project-support">三、项目配置支持</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 入口 | 时机与行为 | 产物及失败边界 |
| --- | --- | --- |
| [ScriptsByPods](./ScriptsByPods/README.md) · Xcode 当前工程 Pod Install／Update | 手动 Behavior，按当前工作区定位 Podfile并打开 Terminal；空回车基础安装／更新，大写 YES 完整流程；其它输入、EOF、Ctrl+C取消。保留现有全局相对路径设置，不自动挂载到构建 | 两个自包含包随工程携带；确认后可更新 Pods、锁文件与工作区，独立日志在系统临时目录；不安装全局工具。JobsIM 当前内联 post_install 两种模式均执行；语法与只读路径核对通过，真实 pod／构建未验 |
| `Scripts/PrepareDependencies.rb` | 手动首次准备；从已安装模板只读复用锁定第三方原始源码／podspec，校验podspec摘要；GRDB采用官方v7.11.1归档及固定SHA256；随后执行pod install | 依赖放在忽略的Vendor／Pods；缺原始来源、摘要不符或安装失败直接停止，不改模板及系统代理 |
| `Scripts/GenerateProject.rb` | 手动生成新项目文件引用和共享Scheme；只在准备时项目缺失才调用 | 独立JobsIM.xcodeproj；主动再次执行会重建生成配置，需随后pod install，不在每次构建自动覆盖 |
| `Podfile`／`Podfile.deps` | 逐行依赖与安装hook分离；post_install维护Pods根组中两份Ruby文件的唯一引用，并核对PBXProject根UUID | 只维护必要展示及构建设置，不把Ruby文件放进Build Phase；真实Xcode红钻显示仍须界面核验 |
| `Scripts/Build.rb` | 手动build，`--test`运行原生数据／UI测试，支持重复`--only=<测试标识>`定向筛选；build默认JobsIM Native，test默认无系统root的JobsIM Instance Trust，或显式UUID | `build/DerivedData`及build/test.log；test使用`build/Results`唯一显式结果路径避免Xcode默认轮换，失败返回真实退出码，不清理其他项目或删除历史产物 |

只读复用的JobsByPods保持本人模板目录结构。新建领域／存储／传输模块分别是JobsIMCore、JobsIMStorage和JobsIMTransport；上层不import GRDB。第三方许可保持原文，22项版本／来源／原许可与业务直接使用或模板依赖分类在[许可清单](./Dependencies/ThirdPartyLicenses.json)，来源／摘要记录在Dependencies目录的JSON中；Vendor源码与Pods是可重建依赖，不作为自有源码修改。

## <span id="native-validation">四、验证边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

```sh
ruby Scripts/Build.rb --test <模拟器UUID>
```

原生数据测试位于仓库 `测试/前端/iOS`，覆盖命名空间及恢复代次隔离、稳定Outbox身份、同ID异参拒绝、确认事务、游标原子提交与版本防倒退、重开持久化与大ID。媒体测试使用实际AVAudioRecorder编码M4A，经HTTPS上传、消息绑定、原ID对账、下载哈希及AVAudioPlayer播放验证；具体结果见[测试说明](../../../测试/前端/iOS/README.md)。

前轮完整18项及随后4项定向／20个不同测试的外置证据保留。上一轮iOS26.5专用模拟器完整30/30通过（24项数据／API＋6条UI，0跳过），最终4项定向复核及1项独立结果路径验证也通过。原始旧xcresult曾被Xcode默认轮换，现在改为build/Results唯一结果路径；历史外置截图、摘要和SHA未改。实际分轮结果与截图见[外置测试记录](../../../测试/前端/iOS/README.md)，不写成单次20项运行。

发送准入增量：正式POST前须重新确认`bootstrap.features.send_admission=true`；全体提及另核验`role_admission=true`。GRDB的`send-admission-freeze`迁移增加冻结原因列，保留旧Outbox原始字节，缺快照记录永久排除自动待发；服务器`SEND_ADMISSION_CHANGED`及确定的角色／提及资格拒绝同样冻结。暂停气泡只通过`message-results`纯查询核对原结果，不向发送接口探测未知ID；未建事实返回404也不创建消息。原快照及ID不变，已提交事实经当前read ACL核对后收口。新建会话缓存本人DTO，附件在上传前捕获原准入。搜索收藏5/5、群互动5/5和对端确认1/1通过；随后普通转发／实例围栏8/8＋对端确认1/1通过，见外置测试最新章节，不以旧30项结果冒充新合同全量回归。当前十语各147键（新增认证文案已静态核对，尚未运行），9个具体页面控制器保持原结构。

认证后的请求还核对已绑定的`deploymentId`与恢复epoch；实例身份失配时退出本地认证、保留旧namespace和Outbox原字节，须重新验真实例，不能把原proof送入新实例。普通媒体离线副本、头像、设备撤销、群退重入及direct资格循环5个受影响旧方法已分轮定向复核，不写成一次全量运行。

转发投影另有本地持久代际：临时503或200不可用投影清理后，晚到的发送确认、原结果与历史不能恢复正文／sidecar；只有之后发起且代际匹配、version不倒退的精确GET可恢复。200 `forwardUnavailable`允许后续重新验权，不将它永久锁存。该增量另3/3定向通过，旧8＋1证据不覆盖。

仍待继续落实：账号注销闭环、完整同步协议／收讫资格／global删除与恢复日志联动、媒体断点续传与耐久回执、完整群角色／禁言等治理、受控端侧迁移及服务端备份恢复、扫码描述、真机与十语全部设备／主题／可访问性组合。联系人、群成员、单聊会话和消息发送人头像已统一鉴权异步渲染；服务端没有群独立头像接口时显示本地Logo。头像与CA原生文件选择器完整链路已在iOS26.5专用模拟器通过；iOS27.2远程Files层输入无效的失败证据保留，不据旧运行时通过推断27.2已修复。未知响应的全部UI组合仍须继续。当前收件箱展示后端当前授权的可选申请人昵称／用户名；服务端省略时以ID回退，不自行取陌生头像；通知只覆盖本机已同步消息。当前不发云清理资格ACK，不启用RTC／支付／地图／E2EE／SDK专项；243主项保持原台账未验收。

## 五、<span id="native-auth-refresh">Access／Refresh接入与恢复边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本轮恢复后的Xcode 27.2／SDK 27.2完整`build-for-testing`通过。新建无系统CA的iOS27.2专用模拟器后，本机12/12和新服务真实HTTPS4/4分别通过，0失败／跳过；没有重跑旧30项或把旧转发证据算作本轮。`IMAuthCoordinator`已进入App target，外置新增测试也进入原生test target；仅`build-for-testing`不表示测试执行。

- 实例明确公开`auth_refresh=true`后，初始认证读取15分钟AT、30天滑动／首次90天绝对上限的RT家族字段；服务端裁决期限。旧七天bearer不能当RT。曾接入新协议的可信档案持久记录能力，恢复epoch变化也不能静默退回旧认证协议。
- 整对AT／RT、family、generation、期限及实例／账户／设备绑定序列化进同一个`WhenUnlockedThisDeviceOnly` Keychain条目。只有完整响应校验、安全库成功、本地绑定成功后才发布给业务；普通SQLite和UserDefaults不保存密码、OTP、RT或其离线猜测摘要。
- 同设备并发401只对`ACCESS_EXPIRED`合并一次refresh，刷新接口不带AT。其他撤权错误不自动刷新；媒体上传／下载也使用同一协调入口。成功后保持原family／device及namespace，结果返回前重验原上下文。
- RT未知响应或安全库失败先留非秘密family／request状态，停止业务和旧RT重试。本人重新输入密码与管理员当前OTP，通过`reauthenticate`关联旧family；服务器撤旧family后建立新设备。旧Outbox原字节保留并冻结，不自动搬迁或发送。
- 初始register／login／reauth在HTTP前先保存非秘密原意图。未知结果继续原requestId及原body，密码／OTP由用户重新输入；不因本地计时或输入变化自动换认证ID。明确重新验证使用新ID，并在实时`auth_retirement_chain=true`时携`previousRequestIds`旧→新完整未收口链（最多64、去重、不含本次ID）及末尾`previousRequestId`；不要求先等交付过期。旧请求不补vector，旧OTP不可重建时使用用户当前本人证明建立关联后继。未知注册没有账号已成证据时仍只能续原注册。已安全存妥但SQLite交接未完成的本地检查点可在重启续作，不能先flush业务。
- 登录页提供管理员可选验证器代码与明确“重新验证身份”确认。旧认证的晚到响应不能覆盖后来选择的新认证；新设备空间包含profile／deployment／user／sourceContext／epoch／device，刷新不会切空间。

用户清理整个`~/Library/Caches`后，旧集成域的运行数据、CA、私密env、工具链与备份已丢失；原报告只保留历史验证含义。幸存iOS源码／Pods与仓库内结果不等于旧服务恢复。新服务使用`~/Library/Application Support/JobsIM`，备份使用`~/Documents/JobsIMBackups`；客户端须按新CA和新实例重新建立档案，不能套用旧namespace／proof。当前只用幸存模拟器作为编译目标，不重建旧缓存或停止旧残留服务。

上一认证轮编译命令直接指定持久输出目录；下面的`OTHER_SWIFT_FLAGS`会被Xcode后置的`-j`覆盖时，不能据此认定实际driver并发为2。原轮结果与原始报告保留，不追认未核实的并发上限：

```sh
xcodebuild -workspace JobsIM.xcworkspace -scheme JobsIM \
  -destination 'platform=iOS Simulator,id=<可用模拟器UUID>' \
  -derivedDataPath "$HOME/Library/Application Support/JobsIM/Build/iOS/DerivedData" \
  -jobs 1 ONLY_ACTIVE_ARCH=YES ARCHS=arm64 \
  OTHER_SWIFT_FLAGS='$(inherited) -j2' CODE_SIGN_IDENTITY=- build-for-testing
```

本轮以独立36443实例、独立账户和应用内专用CA完成真实refresh／本人重新验证／完整未知后继链，以及实际文件上传、消息绑定和鉴权下载。业务`ACCESS_EXPIRED`由客户端闭包受控注入；不声称在原生端实际等待15分钟、30天或90天，也未做管理员OTP UI、真机或全语言主题矩阵。安全库失败是本机测试注入；SQLite重开是新本地会话／连接，不冒充进程断电演练。首张启动过渡空白帧另存且不作验收，后续已实际查看的认证入口截图只有空字段，不含凭据。

[本轮独立测试摘要](../../../项目相关报告/测试/前端/iOS/2026-11-15-认证刷新与家族隔离/测试摘要.json)、[SHA256清单](../../../项目相关报告/测试/前端/iOS/2026-11-15-认证刷新与家族隔离/SHA256SUMS)及[实际认证入口](../../../项目相关报告/测试/前端/iOS/2026-11-15-认证刷新与家族隔离/认证入口-原生UI.png)。旧证据未覆盖。

## <span id="native-authoritative-sync">六、权威快照与增量（验证检查点）</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

正式消息同步改用[权威协议](../../后端/Go/接口契约/README.md#authoritative-sync)。`IMAuthoritativeSync`先核实时能力及实例身份，再以GRDB内已提交的登记nonce／物理`storeGeneration`注册当前服务端设备。每页校验原`referenceEnvelope`／`bodyEnvelope`字节SHA、引用和正文结构、恢复代际与版本；完整manifest／每页阶段元数据保留，阶段表不写正文。最终validate与首次当前权限changes都成功后，正文、origin／redacted元数据、连续游标、global终态及Outbox只读证明在同一GRDB事务安装。迟到账号／权限／本地guard不能越过CAS。未知续原请求及snapshot；明确过期或失效才废弃该快照意图。

后台不再向消息创建路由自动重发。原队列只沿原会话路由及完整原payload调用`message-results`；仅200事实加事务内原体相等才能收口。同步碰到同clientMsgId、redacted区间、404／409或网络未知都不能确认原待发。能力消失收紧云视图，不退回旧历史轮询，也不切演示。`qualifiedACK`／`after_receipt`仍关闭；服务端已读、云草稿／偏好和正式迁移仍未交付。

消息页提供“本地历史（只读）”。只有当前已核身份、同profile／实例／本人／personal／恢复代际／设备下，曾完整安装并具有匹配正文hash的普通副本才可见；旧未核缓存、替换数据库、其他来源／代际和待核备份继续隔离。此入口禁发消息、群写和云附件获取，不把保留副本当云权限。global永久清理跨已登记旧设备及retained／收藏／未读等持有者，清媒体sidecar／通知／可见预览；forward正文及临时proof不写持久历史，展示须精确GET当前投影。

本切片当前arm64应用和测试包已真实编译，18条注入XCTest在自有iOS Simulator／GRDB全部通过（0失败／跳过）；仍未连接44443作新同步HTTPS或完整UI验收。公告只存引用，既知global不复活，原Outbox须原body纯查及事务再核。构造审计69份Swift为0问题；冷构建中断、静态save限定错误及首18项6过／12失败的原始证据保留。夹具现保存同一页原refs字节，另验同结构异字节不能越过原摘要；未削弱生产校验。原认证App／67源码和旧报告继续封存。当前精确范围与源SHA见[权威同步报告](../../../项目相关报告/测试/前端/iOS/2026-11-16-权威快照与增量/README.md)。


## 七、<span id="personal-state-ui">本人状态与云草稿原生入口（在建）</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本次接续现有S2消费，入口是聊天页“我的同步状态”、会话列表“会话显示”及设置页同名状态入口。默认列表不展示归档／隐藏；恢复筛选分别查看，置顶按服务端原rank排序。手动未读与可见消息阅读观察分开，静音只控制本人通知，不改变云消息资格。字号／真实可见ID观察沿用原实现。

云草稿每次打开／重载均精确GET。保存／清除先冻结原CAS／nonce／epoch／family／da1；回复另精确GET核原contentVersion和dr1，不存quote。prepared仍待确认、retired保独立文字而不冒充保存；未知仅查完整原body结果，明确重试才允许404后沿原体收尾。current=false只保原事实，重新GET当前状态。空草稿没有清空编辑框的采纳入口；载入／恢复必须明确确认，且编辑器文字／回复／@快照仍相等，不能覆盖期间的新输入，也不发送或改正式Outbox。

| 位置 | 默认与持久化 | 生命周期／差异 | 当前证据 |
| --- | --- | --- | --- |
| [本人状态服务](./JobsIM/Services/IMPersonalStateService/IMPersonalStateService.swift) | 同GRDB self-state-operations保存原意图；临时proof只随原待核body，结果仅元数据 | 实例／本人／epoch／device／family绑定；恢复捕当前本地guard，原body不补新proof；旧结果不写回当前投影 | 历史30保留；当前31本人状态／同步本地与10导出同轮通过 |
| [原生状态页](./JobsIM/UI/IMPersonalStateVC/IMPersonalStateVC.swift) | view动作、草稿保存／清除／明确采纳、原操作核对／重试／明确归档拒绝记录 | 页面离开取消任务；回执前核原上下文；原始ID／许可不作为主要用户文案 | 2真实业务已过，完整UI未过，不套用旧21＋12 |
| [聊天接线](./JobsIM/UI/IMChatVC/IMChatVC.swift)／[列表](./JobsIM/UI/IMListVC/IMListVC.swift)／[设置](./JobsIM/UI/IMSettingsVC/IMSettingsVC.swift) | 十语言、现有三主题；本机草稿仍独立；默认筛选All | 受实时S2／cloud_drafts门禁，失效不退演示；恢复列表不授回历史资格 | 本轮独立[报告](../../../项目相关报告/测试/前端/iOS/20261116-S2原生界面/README.md) |

S2界面的冻结包已实际30项本地和2项真实业务通过，完整原生UI仍未通过。最新DEBUG夹具改为Documents原子公开档案交接，凭据继续原Keychain／GRDB；该修补已随正式导出完整编译，新增后的31项本地与10导出共41同轮实际通过，不追认旧包。真实字号选择与完整UI待空间门禁恢复后复验，六个本轮测试家族已逐项关闭；分轮原失败、公开journal与撤销补证见上述报告。

本轮编译／原生注入／真实HTTPS／界面／真机分别记录；旧S1与原S2产品、输入SHA和失败保持历史范围，完整I仍未完成。

## 八、<span id="personal-data-export">正式本人数据导出（源码检查点）</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

设置新增“本人数据导出”，原JSON备份保持独立。原生列表只列本机原请求，进入／重载均查实际状态；创建、明确同体重试、取消分别保存原nonce／UTF8 body／SHA，GRDB FULL事务与原record CAS不覆盖并发取消。能力撤回仍可核对、取消旧任务，新建和下载需实时chat_export；断网显示未确认与重载，不伪造制品。

下载前重新查ready／published／epoch和短票，票及bearer不进普通库。严格实例信任、固定同源路由、不跟随重定向，ZIP流写自有临时文件；完整EOCD／ZIP64／中央目录与安全路径校验后交原生Files复制保存，结束清临时文件。外部已存ZIP不声称可远程抹除。三主题与十语209键静态核齐；源码捕原namespace／family及页面生命周期，迟到结果不交付其他身份。本轮App89e02736／test94a1cbb8完整build-for-testing通过；31原S2＋10导出共41已原生本地通过、0失败／跳过，既有自有sim已正常关闭；未连接API／执行真实系统保存／真机。123输入与完整包逐字节核齐，首轮Swift6隔离错误与旧Products保留，见[独立编译报告](../../../项目相关报告/测试/前端/iOS/20261116-正式本人导出/README.md)。

## 九、<span id="current-regression">当前源码回归与主题可读性</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[跨端回归中的 iOS 证据](../../../项目相关报告/测试/回归/20261116-全项目回归/iOS/README.md)以当前工作树在独立 iOS 27.2 模拟器完整编译，最终 65 项原生本地测试和 1 项演示 UI 同轮通过，0 失败／跳过；消息发送可见、重启保留、真实设置入口切黑夜／英文均已验证，十语各 209 键核齐。黑夜文字／图标强调色与承载白字的实心底色分离，页面／面板上的文字及白字实心底分别通过实际 trait 解析的 4.5:1 对比度检查；未新增持久化字段，随现有主题和控件生命周期更新。两份 Pod Behavior 自包含入口的只读定位／迁移检查通过，真实 pod 未执行。

上述证据没有连接真实 API，不替代当前候选的正式导出系统保存、跨进程本人状态 UI、完整群治理、真机或全部十语主题矩阵。主动改密、注销和普通端迁移入口仍未交付，完整收讫与清云资格继续保持关闭；当前版本不作完整 I 封版。

<a id="🔚" href="#前言">返回前言</a>

当前新增[本人主动改密](../../../项目相关报告/实现进度/20261116-剩余能力闭环/iOS改密.md)：设置新改密／认证页恢复原单，GRDB只存非秘密责任、专用无Cookie严格HTTPS、原family门禁及迟到隔离。71原生测试零失败／零跳过，包含原规范部署入口实际HTTPS改密；新增表单UI、管理员TOTP与真机仍待验。后续群治理和注销的候选另行记录，不追认此轮摘要为最终全项目。

本轮新增[本人注销与精确本机清理](../../../项目相关报告/实现进度/20261116-剩余能力闭环/iOS注销.md)、[群治理与历史原单](../../../项目相关报告/实现进度/20261116-剩余能力闭环/iOS群治理.md)。最新应用／测试包真实编译及78项模拟器方法通过；源码、网络与UI边界按报告分列，十语各277键，主题沿用现有三选项。

## 十、<span id="personal-migration">普通聊天迁移</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

设置新增普通迁移：目标配对、源已确认内容选择／原件模式、加密文件队列、原请求只查／明确原体重试及独立本机历史。源／目标限定同本人同实例personal、当前epoch下不同device及真实registered store。当前密码／管理员OTP、mp1证明、key与installToken仅RAM，后台／离开取消并清输入；非秘密原nonce、family／device／store／选择与普通内容摘要先GRDB FULL CAS，不给旧family／store补新资格。

目标按原manifest与message JSON片段SHA、AES-GCM及所有原件长度／SHA整批核验，密文暂存由同步guard／global专用事务保护。独立HTTPS重新验密取钥，5秒target-bound验证后不再await，立即本地FULL事务发布。global优先、保留较新本地CV，不改cursor／Outbox／qualified ACK，不自动清源。installed_reported只读原事实，不重复取key／capsule；服务端报告不证明本机实际durability。转发历史读取实时GET核当前来源，失败调用独立archive正文清除并显示占位。

十语每种保留原277键并新增41键，沿现有三主题和字号；选择、原单、档案及内容列表提供空态／重载／更多入口。最终真实原生122项全部通过，0失败／跳过，包含迁移协议／Store12、Service10、global迟到竞争、Go fraction expiry及两条qualified cloud_only来源反例；前两轮111／11、112／10失败原件保留。HTTP夹具与真实SQLite／CryptoKit边界、UI／系统文件和真实双端HTTPS待核项分别见[独立记录](../../../项目相关报告/实现进度/20261116-剩余能力闭环/iOS普通迁移.md)。

本地基础Pod从Podfile实际位置逐级发现 `JobsBaseConfig/JobsBaseConfig@JobsSwiftBaseConfigDemo`，多候选须指定 `JOBS_SWIFT_TEMPLATE`。整树迁至JobsProds后已实际 `pod install --no-repo-update` 并确认46依赖版本／校验和不变，更新相对路径后完整编译通过；两个Xcode Behaviors已实际菜单启动至本工程脚本并在确认前取消，[搬迁证据](../../../项目相关报告/实现进度/20261116-剩余能力闭环/工作树搬迁/README.md)。

## 十一、<span id="receipt-durability">逐物理库收讫</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

设置→本机收讫资格提供实际App代码集合测量、真实当前SQLite／原件probe、精确部署批准匹配及用户明确声明；server ready、旧SyncContext顶层false、本端耐久与每条已成事实分别显示，默认cloud_history。原正文bytes＋原意图同FULL，完整原件SHA与文件／目录fsync后才可ACK；原nonce未知先纯查，旧family不能写原单。密码、OTP与认证凭据不进入收讫记录，原proof随同体回执意图保存。后台清RAM验证；退出／换库／epoch不继承验证。

默认关闭的自动正文协调器仅在用户明确启用后的本进程原family运行，有界补查已有本库冻结target，未知原单只查不自动重POST，完整原件仍需明确下载。正常sync推进不使同build／物理库／epoch的真实报告失效，每次阶段事务仍比较最新完整guard；global与账号清理使用可恢复的精确文件清理责任。十语当前每种360键，原前缀保留，沿现有三主题、空态／重载／分页。

最终真实原生122项通过，0失败／0跳过，47秒，126输入及应用／测试产物零SHA变化。收讫Storage8与Service14实际执行，包括代码集合摘要、声明旧7／新9兼容、原责任／自动协调及异permission零stage／ACK。首轮111／11与合同修正112／10原件保留；前者修生产guard计数校验，后者补夹具checkpoint，不放宽Store身份门禁。真实HTTPS新收讫、系统UI／后台、真机与断电能力分别待核，不宣称已具备完整云清理验收。源码、结果和边界见[独立收讫记录](../../../项目相关报告/实现进度/20261116-剩余能力闭环/iOS逐存储收讫.md)。

同一最终应用另有1项实际演示UI通过：发送→重启持久化→白天／黑夜→中英切换，2张截图已核；不是收讫设置／真实账号UI验证。当前可运行的是专用模拟器App，110文件、126源码与codesign verify exit0的成品映射见[最终iOS源码与可运行成品](../../../项目相关报告/实现进度/20261116-剩余能力闭环/iOS收讫证据/最终iOS源码与可运行成品.json)，不代表物理iPhone签名安装版。
