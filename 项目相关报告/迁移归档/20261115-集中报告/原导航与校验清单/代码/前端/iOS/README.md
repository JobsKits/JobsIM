# JobsIM · iOS 原生客户端

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、运行](#native-run)
- [二、当前操作](#native-actions)
- [三、项目配置支持](#native-project-support)
- [四、验证边界](#native-validation)
- [五、Access／Refresh接入与恢复边界](#native-auth-refresh)

</details>
<!-- jobs-toc:end -->

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

发送准入增量：正式POST前须重新确认`bootstrap.features.send_admission=true`；全体提及另核验`role_admission=true`。GRDB的`send-admission-freeze`迁移增加冻结原因列，保留旧Outbox原始字节，缺快照记录永久排除自动待发；服务器`SEND_ADMISSION_CHANGED`及确定的角色／提及资格拒绝同样冻结。暂停气泡只通过`message-results`纯查询核对原结果，不向发送接口探测未知ID；未建事实返回404也不创建消息。原快照及ID不变，已提交事实经当前read ACL核对后收口。新建会话缓存本人DTO，附件在上传前捕获原准入。搜索收藏5/5、群互动5/5和对端确认1/1通过；随后普通转发／实例围栏8/8＋对端确认1/1通过，见外置测试最新章节，不以旧30项结果冒充新合同全量回归。当前十语各146键（新增认证文案已静态核对，尚未运行），9个具体页面控制器保持原结构。

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

本轮编译直接指定持久输出目录，并把Swift driver并发限制为2；从本目录执行：

```sh
xcodebuild -workspace JobsIM.xcworkspace -scheme JobsIM \
  -destination 'platform=iOS Simulator,id=<可用模拟器UUID>' \
  -derivedDataPath "$HOME/Library/Application Support/JobsIM/Build/iOS/DerivedData" \
  -jobs 1 ONLY_ACTIVE_ARCH=YES ARCHS=arm64 \
  OTHER_SWIFT_FLAGS='$(inherited) -j2' CODE_SIGN_IDENTITY=- build-for-testing
```

本轮以独立36443实例、独立账户和应用内专用CA完成真实refresh／本人重新验证／完整未知后继链，以及实际文件上传、消息绑定和鉴权下载。业务`ACCESS_EXPIRED`由客户端闭包受控注入；不声称在原生端实际等待15分钟、30天或90天，也未做管理员OTP UI、真机或全语言主题矩阵。安全库失败是本机测试注入；SQLite重开是新本地会话／连接，不冒充进程断电演练。首张启动过渡空白帧另存且不作验收，后续已实际查看的认证入口截图只有空字段，不含凭据。

[本轮独立测试摘要](../../../测试/前端/iOS/报告/2026-11-15-认证刷新与家族隔离/测试摘要.json)、[SHA256清单](../../../测试/前端/iOS/报告/2026-11-15-认证刷新与家族隔离/SHA256SUMS)及[实际认证入口](../../../测试/前端/iOS/报告/2026-11-15-认证刷新与家族隔离/认证入口-原生UI.png)。旧证据未覆盖。

<a id="🔚" href="#前言">返回前言</a>
