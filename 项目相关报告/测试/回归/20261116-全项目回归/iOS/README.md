# JobsIM · iOS 当前源码回归

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 🔥 <span id="前言">前言</span>

执行日期：2026-11-16。使用当前工作树、Xcode 27.2 和已有 iOS 27.2 运行时，独立创建 `JobsIM Regression 20261116` 模拟器。原有工程文件改动保留，不重新生成工程、不执行 pod install、不修改模板或第三方源码，不复用真实账户或部署。

## 一、原始回归与证据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 检查 | 本轮实际结果 | 原件 |
| --- | --- | --- |
| 当前源码完整编译与本地 XCTest | 64 通过、0 失败、0 跳过；7 类本地测试 | [结构化摘要](./本地回归摘要.json)、[编译及测试日志](./xcodebuild.log)、`本地回归.xcresult` |
| 演示原生 UI | 1 通过、0 失败、0 跳过；唯一文本发送可见、重启仍在、设置切黑夜和英文 | [结构化摘要](./演示UI摘要.json)、[原始日志](./演示UI.log)、[截图索引](./UI截图/manifest.json) |
| 国际化资源 | 10 语各 209 键，无缺键、额外键、空值或直接引用的缺失键 | [结构与国际化](./结构与国际化.json)；只验证资源完整性，不代替十语实际排版与翻译质量 |
| 工程引用 | App 62、数据测试 17、UI 测试 3 份源码挂载；无缺文件，无遗漏本地测试文件 | 只读 xcodeproj API 核查，未保存工程 |
| Pod 快捷入口 | 两包与本人 Swift UIKit 模板脚本 SHA 一致，执行位及语法正常；8 条工作区／工程／环境变量路径成功，无上下文 2 条拒绝 | [迁移检查](./Pod入口迁移.json)另覆盖中文、空格、emoji、复制改名及工程外 cwd，两项通过 |

64 项由 `IMAuthCredentialTests`（3）、`IMAuthCoordinatorTests`（9）、`IMAuthoritativeSyncTests`（21）、`IMPersonalStateTests`（10）、`IMDataExportTests`（10）、`IMStorageTests`（9）和 `IMSessionTests`（2）构成。它们使用真实原生 GRDB／文件／Keychain 与显式注入请求，覆盖身份隔离、原意图、迟到响应、同步原子提交、草稿采纳、ZIP 目录完整性；不冒充服务端 HTTPS 联调。

执行以 `xcodebuild -workspace JobsIM.xcworkspace -scheme JobsIM -destination platform=iOS\ Simulator,id=<本轮UUID> -derivedDataPath <工程外持久构建目录> -resultBundlePath <本轮唯一结果包> -jobs 1 -parallel-testing-enabled NO` 加上述 `-only-testing:` 筛选及 `CODE_SIGN_IDENTITY=- test` 完成。演示 UI 首轮使用同一产品的 `test-without-building`。准确 argv、SDK 与环境见各原始日志；UUID见[本轮模拟器记录](./模拟器UUID.txt)。

## 二、发现与修正 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

Xcode 两个 Behavior 的全局相对路径正确，JobsIM 缺少对应 `ScriptsByPods` 包；已补齐随工程携带的两个自包含入口及说明。未改共享 Behavior、Podfile、锁文件或用户原有工程配置；实际依赖安装／更新没有执行。CUA 观察时当前 Xcode 仅打开其它工程，本轮没有从 JobsIM 菜单完成真实 Terminal 交接，因此只读定位与真实菜单分别记录。

原生 UI 测试虽通过，截图仍暴露黑夜模式固定深绿色文字／图标偏暗。`IMUI.green` 原来固定 `#287C66`，对深色页面 `#101512` 为 3.66:1、对深色面板 `#1B241F` 为 3.16:1，低于工程 4.5:1 基线。现将文字／图标强调色按有效主题解析，黑夜使用 `#65C8AB`；实心按钮、未读徽标与本人消息气泡单独使用 `greenFill=#287C66`，保持白字 5.04:1。黑夜强调色对页面／面板分别为 9.14:1／7.89:1，见[数值对照](./主题对比度对照.json)。

新增原生测试按 light／dark 的实际 UITraitCollection 解析颜色，检查主字、次字、强调字在页面和面板上的对比，以及实心底上的白字。所有颜色均为不透明实际解析值；不靠更改系统主题或只查色值名称判定通过。修正后的编译、全部本地测试和演示主题 UI 单独留证，不用原 64 项包追认新源码。

修正后的最终结果为 **65 项本地原生＋1 项演示 UI 同轮全部通过，0 失败／跳过**，见[结构化摘要](./主题修正摘要.json)、[原始日志](./主题修正回归.log)及[修正后截图索引](./主题修正截图/manifest.json)。实际查看了黑夜英文截图，状态、返回入口与选中 tab 已使用浅色强调字；白字徽标／气泡底保留深绿。102 项输入在最终运行前后 SHA 一致，当前 App `38992d97aa88`／测试 `dad4799ee227`／UI 测试 `3b6cff4f9893` 的完整摘要见[产物清单](./最终产物SHA256.json)。专用模拟器结束为 Shutdown。没有改动工程配置或新增存储字段。

## 三、当前缺口与验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

设置页当前有设备、恢复码、正式导出与普通本地 JSON 导出；没有本人主动改密、账号注销申请／受限取消和普通端迁移入口。恢复码改密不替代登录态主动改密；普通 JSON 分享没有迁移导入／原子交接，不满足完整备份恢复合同。完整收讫目标、累计 ACK 与保留清理仍未完成，不启用 `after_receipt`。

本轮未连接真实 API、未测试当前实例 CA／HTTPS 注册、群治理、正式 ZIP 系统保存或跨进程本人状态 UI；没有真机、后台推送、断电／满盘、十语主题全部组合、完整群禁言／解散／邀请和迁移证据。原历史 HTTPS／文件选择器结果按其原输入保留，不移作本轮证据。第三方过时 API 告警、缺 AccentColor 及未使用 capture 告警保留在日志，未修改第三方。

Release 配置曾作诊断探测：Xcode 27.2 不支持 `-dry-run`，该命令退出 64，见 `Release计划.log`。直接将 `-Owholemodule` 传给 swiftc 的报错不代表工程配置错误：本机 Xcode `Swift.xcspec` 明确将其展开为 `-O -whole-module-optimization`；因此没有改动用户原有 Release 设置。本轮没有执行 Release 构建，不将 Debug 通过外推为发布签名通过。

原始输入见[首次输入 SHA](./输入SHA256.json)，主题修正输入见[修正输入 SHA](./主题修正输入SHA256.json)；结果包与截图保持原件。模拟器仅用于本轮测试，结束核对为 Shutdown，不删除其数据。

[返回全项目回归](../README.md) · [返回 iOS 工程](../../../../../代码/前端/iOS/README.md)

<a id="🔚" href="#前言">返回前言</a>
