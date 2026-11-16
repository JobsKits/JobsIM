# JobsIM

![JobsIM](./代码/前端/Web/Public/Logo.svg)

[toc]

---

## 🔥 <span id="前言">前言</span>

JobsIM 是面向个人和小团队的自托管即时通讯软件，目前在研，由 Jobs 维护。第一阶段先打通基础聊天和部署闭环。iOS、Android、HarmonyOS分别采用原生工程，Web为标准React/TypeScript应用，CLI与后端使用Go。已有需求统一归入[需求目录](./需求/README.md)，不复制第二套版本。

## 一、<span id="engineering">工程入口</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 目录 | 内容 |
| --- | --- |
| [代码](./代码/README.md) | 五端与后端独立工程、接口契约和各单元代码地图 |
| [测试](./测试/README.md) | 按端组织的外置测试、执行入口与实际证据 |
| [需求](./需求/README.md) | 编码前的全部需求、架构、数据库、验收、原始xlsx和配图 |
| [部署](./部署/README.md) | 按操作系统组织的运行、停止、备份与资源归属说明 |
| [当前实现状态](./项目相关报告/实现进度/README.md) | 已实现范围、未完成项与后续接续位置 |

## 二、<span id="run">运行与验证</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

优先阅读对应工程README。开发依赖放用户级缓存，工程只维护声明、锁和受控链接；业务数据保存在项目外。正式前端使用同源HttpOnly会话，原生使用系统秘密存储，独立演示不会回放给真实用户。

Web已提供可运行界面；后端采用真实PostgreSQL及按域HTTP角色，开发入口可组合启动。完整平台构建、集成、恢复和第一阶段验收仍按实际测试结果更新，不把源码存在视为全部完成。支付／钱包、SDK产品化、地图、RTC与E2EE专项按需求留在后续标段。

## 三、<span id="ownership">维护边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

保留全部既有需求内容、图片和原始表格；未改本人模板或第三方源码。当前不提交或推送。Apache-2.0为自有源码发行基线，第三方许可与资源出处由各工程说明维护。

## 四、<span id="toolchains">工具与运行条件</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

下表为2026-11-15本次源码声明／本机验证基线，不代表各平台已完成发布验收。锁文件均在工作树中，本轮未创建Git提交。

| 单元 | 语言／构建工具 | 最低支持与锁定依据 |
| --- | --- | --- |
| 后端／CLI | Go 1.27.4，PostgreSQL 18.7 | `go.mod`声明Go 1.27，`go.sum`锁模块；CLI真实SQLite实现见自身依赖清单 |
| Web | Node.js ≥22.12、React19.3.1、TypeScript7.0.2、Vite8.3.8 | `package-lock.json`；现代浏览器须支持IndexedDB、AbortSignal.any／timeout；旧浏览器未验 |
| iOS | Xcode27.2、Swift6、CocoaPods | iOS16+，`Podfile.lock`；真实模拟器测试与签名边界见iOS测试报告 |
| Android | Kotlin2.4.20、AGP9.3.1、Gradle9.5.0、Java17目标 | minSdk26、compile／target37；Gradle wrapper固定版本，依赖在构建声明中显式指定 |
| HarmonyOS | 镜像CLT6.1.1.280、Hvigor6.24.2、ArkTS／ArkUI | API24配套构建、兼容目标以当前build-profile为准；[libsdl镜像](https://libsdl.org/harmonyos-devtools/)实际取源与构建记录见鸿蒙README |

开发、集成测试、部署实例使用各自独立数据目录与配置；测试夹具不随源码分发。正式部署采用HTTPS、独立秘密文件及按域数据库权限。自建CA需显式配置，不能关闭校验。网页离线预览执行Web环境准备及 `npm run dev`，无需先部署服务器；接口失败会保留演示，恢复后的真实响应自动覆盖真实空间。

启动问题按端排查：Node缺失先检查准备脚本；HTTPS失败先核CA／SAN／端口；iOS模板缺失显式设置 `JOBS_SWIFT_TEMPLATE`；鸿蒙工具缺件按固定镜像补齐；数据库端口占用由部署入口停止并报告，不接管外部进程。

## 五、<span id="statistics">项目统计</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

统计日期统一为2026-11-15，含当前未提交代码；与历史Git提交分开。源码规模由[统计入口](./工具/统计项目.mjs/README.md)读取实际文件，逐文件依据见[报告](./工具/统计项目.mjs/报告.json)。

### 5.1、历史与页面

| 指标 | 本次读取结果与口径 |
| --- | --- |
| 首次建库 | 远程平台建库时间未核实，不用首提交时间代替 |
| 首次提交 | `git log --reverse`：2026-10-08 22:59:18 +08:00，`cdf750d` |
| 最近提交 | `git log -1`：2026-11-04 17:43:29 +08:00，`18ceb74` |
| 提交／贡献者 | 当前HEAD历史5次提交；`git shortlog -sn --all`计1名，Jobs |

| 端 | 页面数 | 子页面数 | 路由数 | 源码口径 |
| --- | --- | --- | --- | --- |
| iOS | 9 | 已含在9中 | 不使用URL路由 | 具体UIViewController类；排除抽象IMBaseVC、弹层、系统选择器；`JobsIM/UI` |
| Android | 1 | 0 | 0 | Activity计数；Home／聊天／设置为单Activity状态，弹层不计；`MainActivity.kt` |
| HarmonyOS | 1 | 0 | 1 | `@Entry`页面及main_pages声明；Index内状态与弹层不另计 |
| Web | 1 | 0 | 1个文档入口 | `main.tsx→App`根页面组件；导航状态、对话框不计独立路由，无移动Web容器 |

页面采用各端真实工程单元口径，不能跨端直接比较功能覆盖率；小量页面承载较多IM状态，因此与接口数的比例不按普通表单业务判断。

### 5.2、后端与运行规模

| 指标 | 数量／边界 | 统计依据 |
| --- | --- | --- |
| 对外HTTP | 62 | 各`*Routes`中method＋pattern计一条，展开pin／favorite循环；排除health、internal及管理语义接口；[HTTP契约](./代码/后端/Go/接口契约/README.md) |
| 实验接口 | 另5条HTTP，默认关闭 | 仅 `--experimental-exports`，不计正式聊天导出交付 |
| 对外RPC／MQ | 0／0 | 内部按域HTTP不计外部RPC |
| 域与部署角色 | 8个持久owner域＋gateway，共9个角色 | identity／directory／conversation／message／media／control／journal／governance；`all`为开发编排，不是独立生产拓扑证明 |
| 核心业务／安全主表 | 11；实验导出另2 | users、sessions、recovery_code_batches、mfa_factors、reset_grants、profiles、contact_requests、conversations、messages、objects、reports；排除日志、关系、中间、资格历史明细与配置表 |
| 状态流转对象 | 15类，见下文 | 按业务生命周期计，不按物理表或状态个数计 |
| 外部业务系统集成 | 0 | 邮件、支付、地图、推送、RTC均未接入 |
| 周期后台组件 | 2 | governance导出清理／实验构建循环；message转发失效恢复循环。备份／恢复为手动命令 |
| 中间件／存储 | PostgreSQL业务与独立journal集群、受控本地文件卷 | 无Redis、MQ或Elasticsearch运行依赖；跨库部署备份与单业务库恢复证据分别报告 |
| 峰值QPS／日增／容量 | 未测 | 当前接口与隔离恢复实测不作为容量达标证据 |

状态对象为账户、设备会话、恢复码批次／单码、账号恢复操作、MFA强验证资格、reset-grant、好友申请、入站请求门禁、群、成员资格代际、消息、头像处理／发布、举报、群公告引用／原操作、转发引用及失效清理。运维恢复操作和实验导出生命周期另列，不混入正式业务交付数。

### 5.3、自有代码行数

以下为物理行（含空白和注释），范围为代码／测试／部署／工具；排除第三方、符号链接、资源、锁文件、生成物和测试报告。业务与测试分别列示。

| 类别 | 文件数 | 行数 | 行占比 |
| --- | --- | --- | --- |
| 业务代码 | 169 | 39366 | 67.23% |
| 测试代码 | 61 | 14869 | 25.39% |
| 配置与脚本 | 45 | 4321 | 7.38% |
| 第三方依赖 | 排除 | 排除 | 不进入分母 |

```mermaid
pie showData
    title 自有工程物理行占比（2026-11-15）
    "业务代码" : 39366
    "测试代码" : 14869
    "配置与脚本" : 4321
```

## 六、<span id="flows">模块与核心流程</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

工程未初始化CodeGraph；下图依据各单元现有代码地图与入口源码。

```mermaid
flowchart TD
    A["Web main.tsx / App.tsx"] -->|同源Cookie＋CSRF| G["网关 cmd/jobsimd/main.go"]
    B["iOS AppDelegate / SceneDelegate"] -->|HTTPS Bearer| G
    C["Android MainActivity.kt"] -->|HTTPS Bearer| G
    D["HarmonyOS EntryAbility.ets / Index.ets"] -->|HTTPS Bearer| G
    E["CLI cmd/jobsim/main.go"] -->|HTTPS Bearer| G
    G -->|域间HTTP＋服务鉴权| I["identity / directory / conversation"]
    G -->|域间HTTP＋服务鉴权| M["message / media / control / governance"]
    I --> P[(业务PostgreSQL)]
    M --> P
    I --> J["journal / operations.go"]
    M --> J
    J --> Q[(独立journal PostgreSQL)]
    M --> F[(受控媒体／导出文件卷)]
    X["外部支付／推送／地图等"] -.->|后续标段，未接入| G
```

```mermaid
flowchart TD
    A["前端发送入口 App.tsx / IMSession / ChatModel / local.go"] --> B[固定clientMsgId与restoreEpoch]
    B --> C["本地事务：Outbox＋草稿"]
    C -->|提交成功后HTTPS| D["message.go：鉴权、资格代际、幂等检查"]
    D --> E[业务消息事务与独立journal交接]
    E --> F[返回权威消息与游标]
    F --> G["本地消息／游标原子更新，移除已确认队列"]
    E -->|响应丢失| H[保留原ID和原载荷]
    H --> D
    A -->|服务器恢复代次变化| I[归档旧缓存、冻结旧队列、重新认证]
    I --> J[重新拉取权威历史；不可用正文保留占位]
```

转发引用另有以下生命周期；临时依赖未知只隐藏投影并重查，不冒充源已永久删除。

```mermaid
stateDiagram-v2
    [*] --> active: 原源版本与资格验证后入库
    active --> invalidating: 源内容变化、撤权或恢复代际失效
    invalidating --> unavailable: 排空派生持有者并提交占位／版本事件
    unavailable --> [*]: 不由重新入群或旧凭据恢复
```

## 七、<span id="feature-boundaries">功能与工程边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 类别 | 当前实现方式与验收边界 |
| --- | --- |
| 网络与安全 | HTTPS；Web HttpOnly会话／CSRF，原生Bearer；iOS Keychain／Android Keystore；管理员MFA。实例CA与绑定按各端实测。可切换加密协议／E2EE仍按后续专项实施 |
| 数据 | Web IndexedDB、iOS GRDB、Android Room／BundledSQLite、鸿蒙RDB、CLI modernc SQLite；空间、账号和恢复代次隔离；完整同步／收讫资格未全量验收 |
| 消息与治理 | 基础文本／附件、编辑撤回、回复、原引用转发、反应、置顶收藏、好友与群、结构化提及及群公告；头像、举报、账户封禁墓碑服务已实测；各端UI覆盖程度见实现进度 |
| 端能力 | 文件／图片选择及iOS原生录音已有实测；通知为本地能力，第三方推送未接入；蓝牙、定位、扫码、NFC、生物识别、支付、钱包暂未接入 |
| 工程 | 十语言、白天／黑夜／系统主题、字号、演示回退；没有热更新／外部崩溃收集／生产CI/CD交付声明；诊断和测试日志在各自报告目录 |
| 后端 | 原操作幂等、版本比较、资格围栏、限流、事务交接与能力开关；未启用强保护／after_receipt／完整聊天导出，未宣称全局分布式锁或完整链路追踪 |
| 隐私与权限 | 真实秘密在项目外；报告不存Token；麦克风／文件／通知由用户动作申请。跨平台隐私清单及商店发布还需对应平台验收 |

## 八、<span id="dependencies">依赖与许可</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

2026-11-15按直接运行依赖列示；测试／编译工具和传递依赖不混入直接运行总数。无新增私有商业SDK；Jobs本人基础模板属于独立自有复用，按iOS来源清单定位。

| 直接依赖 | 版本 | 来源 | 许可 | 用途／仍使用 |
| --- | --- | --- | --- | --- |
| pgx/v5 | 5.13.0 | 公库Go模块 | MIT | 后端PostgreSQL／是 |
| x/crypto、x/text、x/sys | 0.59.0／0.44.0／0.50.0 | Go官方公库 | BSD-3-Clause | 后端密码、文本、文件锁／是 |
| x/term | 0.48.0 | Go官方公库 | BSD-3-Clause | CLI秘密输入／是 |
| modernc.org/sqlite | 1.62.0 | 公库Go模块 | BSD-3-Clause及所含SQLite公共领域等，见原文 | CLI持久化／是；[含传递许可清单](./代码/前端/CLI/第三方许可/清单.json) |
| React、ReactDOM | 均19.3.1 | npm官方公库 | MIT | Web／是 |
| lucide-react | 1.73.0 | npm官方公库 | ISC／部分Feather MIT | Web图标／是 |
| Activity Compose／Material3／Material Icons Core／Lifecycle Compose | 1.12.4／后两Material按BOM2026.09.00／2.10.0 | AndroidX公库 | Apache-2.0 | Android UI／是 |
| Room Runtime／Room KTX／SQLite Bundled | 2.8.5／2.8.5／2.7.1 | AndroidX公库 | Apache-2.0 | Android持久化／是 |
| kotlinx-coroutines-android | 1.10.2 | Kotlin公库 | Apache-2.0 | Android异步／是 |
| iOS直接第三方Pod | 锁定版本逐项见Podfile.lock及Dependencies来源清单 | 公开上游原文只读复用 | 以各原始podspec／LICENSE为准，不用自有Apache覆盖 | GRDB、Alamofire、SnapKit等；模板附带依赖与业务直接调用区分 |

完整来源／摘要与许可入口见[Web资源出处](./代码/前端/Web/Public/资源出处.md)、[iOS逐项许可清单](./代码/前端/iOS/Dependencies/ThirdPartyLicenses.json)、[Android构建声明](./代码/前端/Android/app/build.gradle.kts)、[Go许可清单](./代码/后端/Go/第三方依赖/README.md)。鸿蒙工具链及Hypium为构建／测试依赖，实际下载固定用户指定镜像；工具许可不随自有代码重新授权。

本次未进行完整漏洞数据库扫描，不能据此声称无已知漏洞。版本显式锁定不等同永久安全；模板附带旧依赖的维护状态与最终发行最小依赖集仍需单独复核。传递许可保留原文，发现GPL／AGPL等约束时须单列处理，不能直接套用项目LICENSE。

<a id="🔚" href="#前言">返回前言</a>
