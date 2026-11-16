# JobsIM 五端资产盘点

![Jobs出品，必属精品](../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 🔥 <span id="前言">前言</span>

盘点日期：2026-11-15。功能补齐先依据本表，每次修改 App 在同一任务更新入口、行为、默认值、持久化、生命周期、源码位置、差异及验证。这里记录当前实现和证据边界，不替代[现行需求](../../需求/README.md)及 243 项[功能验收](../../需求/IM功能验收表.md/IM功能验收表.md)，不将第一阶段未交付事项改排后期。

“已验”只覆盖对应报告列明的构建、环境和用例；历史构建、主机替身、模拟器、真机与目标系统验证分别列出。各轮测试不累加为需求完成数。报告集中于[项目报告入口](../README.md)，源码说明、接口契约和工具入口保留原位。

## 一、五端当前基线 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 端 | 原生入口与实现 | 当前证据 | 当前差异／未验 |
| --- | --- | --- | --- |
| [Android](../../代码/前端/Android/README.md) | Kotlin／Compose，`MainActivity`；Room／BundledSQLite；Keystore | [访问刷新报告](../测试/前端/Android/20261115-访问刷新/README.md)：最终同包 10 条仪器＋6 条 JVM 全通过；安装读回与 APK 同 SHA | 模拟器验证，未物理真机；AT 过期为自有 family 的服务端 SQL 时钟夹具，未自然等待 15 分钟；完整 I 未验收 |
| [iOS](../../代码/前端/iOS/README.md) | Swift／UIKit，Jobs 自建 UI 与传输 Pod；GRDB；Keychain | [访问刷新摘要](../测试/前端/iOS/2026-11-15-认证刷新与家族隔离/测试摘要.json)：12 条本机＋4 条真实 HTTPS 通过 | 过期入口为客户端注入 `ACCESS_EXPIRED` 后真实刷新，未自然等到期限；模拟器通过不等于真机或发布验收 |
| [CLI](../../代码/前端/CLI/README.md) | Go 终端交互，pure-Go SQLite；系统安全库助手 | [刷新家族报告](../测试/CLI/20261115-刷新家族/README.md)：31 个本地顶层 PASS、2 个显式 SKIP；真实协调器和真实 TTY 各 1 组 | macOS DPK 正向保存缺 entitlement，实际 TTY 经明确选择使用限程会话；Windows／Linux amd64 仅交叉编译；目标系统安全库、锁屏与发行待验 |
| [Web](../../代码/前端/Web/README.md) | 标准 Web／React；同源 BFF；IndexedDB | 最新源码已 build（`index-CnD80aSu`）；16 个模拟 host＋[新 8 组真实 GUI](../测试/前端/Web/20261115-认证输入保护)通过 | 新 8 组原始输出已在中央目录，最终 SnapshotSHA 待封存；旧 6 组真实 HTTPS 历史 scope 保留，不叠加计数 |
| [HarmonyOS](../../代码/前端/HarmonyOS/README.md) | ArkTS／ArkUI，`Index`；RDB；Asset Store | 新认证源码语法转译 0 错误，10 个纯注入反例通过 | **新 auth 未完整 HAP 编译、未原生安装／HTTPS**；第 11 个反例发现 restore 旧 context 晚到可能误清新 device，已捕获原 stamp/token 最小修复，待反例复跑／编译；无 HDC 目标。旧 HAP／主机规则结果仅证明原构建 |

Android 当前验证包 SHA256：`e295688485e0caa73932670e48da227eae804108416625b62fba5b0e01cee8b3`。旧转发基线及其报告单独保留，不能合并计数。运行库、实例 CA、密钥和私密测试种子位于工程外持久目录；旧 Cache 清空后重建的新实例采用新身份，旧报告不证明新环境恢复旧数据。

## 二、默认值、存储与生命周期 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 端 | 默认值与设置入口 | 持久化及秘密边界 | 生命周期及隔离 | 源码位置 |
| --- | --- | --- | --- | --- |
| Android | 设置：简中 `zh-Hans`、跟随系统、字号 1；通知默认关闭，正文默认隐藏；十语言及白天／黑夜／系统可切换 | Room 保存历史、原 Outbox、草稿、游标和非秘密认证意图；Keystore 加密 AtomicFile 原子保存完整 AT/RT 对，秘密不入 Room | 前台约 5 秒轮询；按 profile／来源／部署／用户／personal／epoch／device 分空间；同 family 刷新不换空间，reauth 新 device 冻结旧队列；旧响应有代际围栏 | [入口](../../代码/前端/Android/app/src/main/java/com/jobs/im/MainActivity.kt)、[存储](../../代码/前端/Android/app/src/main/java/com/jobs/im/Store.kt)、[认证协调](../../代码/前端/Android/app/src/main/java/com/jobs/im/SessionCoordinator.kt)、[传输／安全库](../../代码/前端/Android/app/src/main/java/com/jobs/im/Transport.kt) |
| iOS | 设置：简中 `zh-Hans`、跟随系统、字号偏移 0；通知及预览默认关闭；十语言、三主题、三档字号 | GRDB 历史／原意图；UserDefaults 非秘密设置；Keychain 整对 `WhenUnlockedThisDeviceOnly` | 前台会话约 5 秒／消息约 3 秒，后台停止轮询；原 family/device 刷新稳定空间，新认证不搬旧 Outbox；迟到响应按原身份检查 | [会话](../../代码/前端/iOS/JobsIM/Services/IMSession/IMSession.swift)、[设置](../../代码/前端/iOS/JobsIM/UI/IMSettingsVC/IMSettingsVC.swift)、[字体／主题](../../代码/前端/iOS/JobsIM/UI/IMUI/IMUI.swift)、[传输](../../代码/前端/iOS/JobsByPods/JobsIMTransport/Core/IMAPIClient/IMAPIClient.swift) |
| CLI | 终端命令及 TTY；无 GUI 主题／语言设置；安全库不可用默认拒无人值守认证，用户明确 yes 才启限程会话 | SQLite WAL／FULL 保存原 payload/ID/准入、非秘密 nonce 链；系统库整对保存；AT/RT 不入普通库／日志 | 单 flight；刷新未知或安全库失败 sticky unknown，不重用旧 RT；device＋store_generation 隔离，丢本地消息库不借旧库凭据自动续发；quit 尝试撤本 family | [入口](../../代码/前端/CLI/cmd/jobsim/main.go)、[认证](../../代码/前端/CLI/cmd/jobsim/auth.go)、[库](../../代码/前端/CLI/cmd/jobsim/store.go)、[凭据](../../代码/前端/CLI/cmd/jobsim/credentials.go) |
| Web | 设置：简中 `zh-CN`、跟随系统、正常字号、通知关闭；首屏独立演示数据有可见标识；十语言／三主题 | IndexedDB 事务保存原队列／历史；localStorage 仅非秘密设置和原 nonce 链；密码／OTP proof 仅 RAM；AT/RT 留 BFF HttpOnly Cookie | 外层 auth lifecycle WebLock→内层 refresh 锁；同部署现有认证责任禁止降旧协议；切表单／关闭取消旧排队意图；旧 namespace 即使无 key 也持久 fence | [App](../../代码/前端/Web/Src/App.tsx)、[认证](../../代码/前端/Web/Src/AuthSession.ts)、[传输](../../代码/前端/Web/Src/api.ts)、[存储](../../代码/前端/Web/Src/storage.ts) |
| HarmonyOS | 设置：简中 `zh-Hans`、跟随系统、字号 1、通知关闭；十语言／三主题 | RDB 串行事务、单 Asset 小于 1KiB 设备解锁且不同步；新完整凭据与交接代码尚未原生验证 | 页面可见约 5 秒轮询，隐藏停止；新协调器持久 unknown／原 nonce／空间交接；**restore 晚回旧 context 围栏源码已修，待反例复跑／编译，不能宣称家族隔离已验** | [页面](../../代码/前端/HarmonyOS/entry/src/main/ets/pages/Index.ets)、[协调器](../../代码/前端/HarmonyOS/entry/src/main/ets/Data/AuthCoordinator.ets)、[存储](../../代码/前端/HarmonyOS/entry/src/main/ets/Data/Store.ets)、[原生服务](../../代码/前端/HarmonyOS/entry/src/main/ets/Data/NativeServices.ets) |

## 三、功能入口与跨端行为盘点 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

下表的“已接”表示相应源入口存在；实际通过范围仍以第一节报告为准。所有普通发送原意图先入本地库再 HTTP，保持原 ID／原 body／原 sendAdmission；明确资格失效永久冻结，网络未知不换 ID。界面演示数据与生产意图隔离，不借演示回退绕过鉴权或制造发送成功。

| 能力 | Android／iOS | Web／HarmonyOS | CLI | 默认、持久化及生命周期／差异 |
| --- | --- | --- | --- | --- |
| 登录、注册、OTP、刷新／重认证 | 原生认证表单、恢复入口；整对与未知链已接 | Web 表单原证明受锁保护；Harmony 新协调器 P1 源码已修，待复跑及编译 | 登录命令／TTY，原始 nonce 与完整退休链 | 仅 `ACCESS_EXPIRED` 刷新；实时 `auth_refresh`／`auth_retirement_chain` 门禁；旧 RT 未知绝不复用，改密码／OTP 走显式新证明关联退休旧链 |
| 会话／历史／草稿／未读 | 会话页和聊天页；历史分页、本地草稿与未读 | Web 会话视图、Harmony Index；本地缓存、分页与重载 | conversations／messages／sync 等命令 | 当前前台轮询／增量为已交付子集；本地读点不等于服务端跨端读同步；**完整快照／完整增量／跳过原因仍缺** |
| 好友申请与关系治理 | 联系人／管理页；仅本人待处理入站列表 | 联系人视图／管理入口 | request／accept／reject／relationship／delete／block／unblock／reopen | 原 request/reopen 操作 ID 持久；删好友不等于关门禁；reopen 仅本人显式新代际开放，不自动成为好友；治理前冻结对应旧 direct 队列 |
| 群管理 | 成员页／管理页：加人、移除、改名、转让、退出 | 群管理面板／Index 动作 | members／add-member／remove-member／rename／transfer／leave | remove/leave 使用原 membershipGeneration；owner 资格另以 roleGeneration 围栏；离群清本机该群历史／游标并冻结旧队列；完整禁言／解散仍缺 |
| 文本与附件 | 原生输入、系统选择／保存／打开 | 浏览器文件、Harmony 文件与系统预览 | 文件路径上传／下载 | text/image/audio/video/file 已接；鉴权媒体 URL 同源固定路径；Android 单附件上限 64MiB，Harmony 当前 UI 25MiB 与后端 64MiB 有差异；不把系统预览写成内置音视频录制／播放器 |
| 回复、编辑、撤回、反应、置顶／收藏 | 消息动作入口 | 消息动作／菜单 | 相应命令 | 原消息 ID 和内容版本操作；contentUnavailable 显示占位，不冒充用户删除；作者擦除入口仍需保留；撤权清缓存正文和 sidecar |
| 搜索／收藏分页与原文定位 | 独立查找／收藏页，可重载和空态 | Web 查找页、Harmony 查找入口 | search／favorites／精确 message 读取 | 独立签名游标绑定本人／epoch／条件，原样回传；选择结果重新精确 GET 和会话鉴权；不把临时 forward proof 展示／普通导出 |
| 结构化 @ 与群公告 | 编辑器成员选择、owner @all；公告入口 | 对应编辑器／公告面板 | 结构化 mentions／announcement 命令 | 原名单／all 和原准入先存，不回填；公告原 ID、CAS、member＋role＋contentVersion 无正文意图持久；未知先原操作 GET，旧结果不覆盖当前；缺 role_admission 不写 |
| 正式引用转发 | 原文重鉴权→选目标→确认；原 body 固定 | 同样引用式入口与投影围栏 | forward 原意图与只读对账 | forwarding 门禁；源 proof／内容版本／目标准入原样保存；临时占位后旧同 version history/ack 不解锁，新精确 GET 可恢复；永久源失效由 server 状态／version保证；普通导出只占位 |
| 静态头像／举报 | 设置头像与原文件操作；消息举报及本人列表 | 对应设置／举报入口 | avatar／report 命令 | PNG/JPEG 原文件＋原 op ID；未知重试原操作，current=false 后新 GET me；举报不等于已交付申诉闭环 |
| 本地通知 | 系统本地通知，默认无正文 | 浏览器／Harmony 本地通知入口 | 无 GUI 系统通知入口 | 默认关闭，仅客户端活跃同步后提醒；后台 APNs／FCM／厂商推送未交付，第三方推送后续排期 |
| 设备撤销／恢复码 | 设置页 | 设置／安全视图 | devices／revoke／recovery 命令 | family/device 撤销围栏已接；不代表完整 MFA 开通、轮转、全部账号安全流程已验收 |
| 备份／导出／注销／迁移 | 部分本地 JSON 备份入口；无正式完整协议 | Web 无普通离线正文导出；Harmony 无正式完整迁移 | 本地受限导出命令与占位 | 当前普通导出不含秘密、原发送恢复库或引用投影正文；**正式导出、账号注销、同实例同账号普通端迁移属于 I 缺口**，不能用本地 JSON 代替 |

功能源码导航：[Android 模块](../../代码/前端/Android/README.md)、[iOS 代码地图](../../代码/前端/iOS/代码地图.md)、[CLI 代码地图](../../代码/前端/CLI/代码地图/README.md)、[Web 代码地图](../../代码/前端/Web/代码地图/README.md)、[HarmonyOS 模块](../../代码/前端/HarmonyOS/README.md)、[后端契约](../../代码/后端/Go/接口契约/README.md)。表中未列出的录音录像、支付、地图、RTC、产品 SDK、E2EE 等不得由一个通用附件入口推断已实现。

## 四、第一阶段最短补齐切片 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 优先序 | 可执行合同切片 | 需要实际交付／验证 |
| --- | --- | --- |
| 1 | 完整快照＋增量 | 明确 snapshot/cursor/hasMore、epoch、全分页与删除／撤权／跳过原因；各端事务落库后才推进游标，断点与账号／端资格切换不串数据 |
| 2 | 合格 ACK／保留策略 | store 注册、receipt_outbox、正文与对象合格 ACK、重传去重／撤权；仅具备完整证据才允许 after_receipt，不能拿本地已读替代 |
| 3 | 正式导出 | 完整当前数据／附件权限及引用链持有者；任务状态、取消／失败／下载生命周期、导出脱密与限权；实验导出不冒充默认正式能力 |
| 4 | 账号注销 | 本人验证、等待期／撤销、受限查询、worker／journal、设备与凭据撤销、对象清理及恢复拒旧真实闭环 |
| 5 | 普通端迁移 | 同实例／同账号／同来源迁移认证、可恢复传输及端本地事务；原 ID 和空间交接、ACK／撤权；与后期 E2EE 密钥迁移分开验收 |

这些是交付排序，不是减少需求范围。完整群治理、陌生人／隐私、举报申诉、MFA 管理等既有 I 项仍须逐项对照[实现进度](../实现进度/README.md)和验收表继续补齐。

## 五、平台验证缺口与维护规则 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- 平台验证单独排期：Android／iOS 物理真机、后台与系统生命周期；Harmony HAP／签名／HDC 安装／Asset Store／RDB／真实 HTTPS；CLI 正式 macOS entitlement 及 Windows／Linux 本机安全库；Web 新 8 组 SnapshotSHA 最终封存。它们不能遮盖第四节的产品合同缺口。
- 后续 App 修改必须以本表为起点，在同一任务修改相应行与实际证据链接；尚在编译／测试的行保留“检查点／未验／待修”，通过后再填准确构建与范围。
- 原始 JSON／PNG／log／包和旧 SHA 清单是历史证据；报告迁移只重写导航，并归档原导航字节。新导航另封清单，不修改 JSON 内原 sourceSHA／旧路径的历史含义。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
