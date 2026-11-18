# <span id="前言">JobsIM iOS 工程脚本</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、入口与影响范围](#entries)
- [二、使用方式](#examples)
- [三、同步目录与签名](#project-model)
- [四、实际验证与边界](#validation)

</details>
<!-- jobs-toc:end -->

---

本目录提供 [**Ruby**](https://www.ruby-lang.org) 原生工程准备、配置维护与模拟器构建入口。脚本从自身位置解析工程根目录，固定路径保持不变；客户端运行说明见[上级 README](../README.md)。

## 一、<span id="entries">入口与影响范围</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

> 三个入口分别处理依赖、工程配置和模拟器构建；按实际需要选择，不将依赖准备作为每次签名排障的前置动作。

| 源码 | 行为 | 写入与失败边界 |
| --- | --- | --- |
| [PrepareDependencies.rb](./PrepareDependencies.rb) | 按 `Dependencies/` 清单从已安装模板复用锁定第三方源码和 podspec；缺 [**GRDB**](https://github.com/groue/GRDB.swift) 时下载固定官方版本并核验归档 SHA；工程缺失时调用生成器，最后执行 `pod install` | 会写入 `Vendor/`、`Pods/`、锁文件和工作区；缺来源、摘要不符或命令失败即退出，不修改模板源码或全局代理 |
| [GenerateProject.rb](./GenerateProject.rb) | 首次建立工程及共享 Scheme；已有工程时原位维护主 App 同步目录、测试引用与签名配置 | 不运行 `pod install`，不重装依赖；保留现有 target UUID、Pods 集成、测试资源、基础 xcconfig 和已有共享 Scheme |
| [Build.rb](./Build.rb) | 对指定模拟器执行 build 或 test，可用重复 `--only=` 定向测试 | 写入工程 `build/DerivedData`、日志及唯一命名的测试结果；返回真实构建退出码，不清理历史结果或其它工程 |

## 二、<span id="examples">使用方式</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

以下示例从 `代码/前端/iOS/` 执行，需要现有 [**Xcode**](https://developer.apple.com/xcode/)、Ruby／[**xcodeproj**](https://github.com/CocoaPods/Xcodeproj) 和 [**CocoaPods**](https://cocoapods.org/) 环境；脚本不会安装全局工具。

```sh
# 仅在明确需要准备依赖时执行；会运行 pod install。
JOBS_SWIFT_TEMPLATE="<已安装的 Swift 基础工程目录>" ruby Scripts/PrepareDependencies.rb

# 新建或维护工程配置；不会运行 pod install。
ruby Scripts/GenerateProject.rb

# 明确选择已有模拟器。
ruby Scripts/Build.rb <模拟器UUID>
ruby Scripts/Build.rb --test <模拟器UUID>
ruby Scripts/Build.rb --test <模拟器UUID> --only=JobsIMTests/IMSessionTests
```

依赖准备入口的模板默认位置来自当前脚本相对布局；仓库迁移或布局不同，应通过 `JOBS_SWIFT_TEMPLATE` 明确指定已安装模板。仅工程目录缺失时，依赖准备入口才调用生成器；已有工程不会因执行依赖准备而自动重新生成。

构建不传 UUID 时，build 查找名为 `JobsIM Native` 的可用模拟器，test 查找 `JobsIM Instance Trust`；找不到即退出，不创建或重置用户模拟器。日志位于 `build/build.log` 或 `build/test.log`，测试结果位于 `build/Results/`。本入口固定使用 `platform=iOS Simulator`，不接受真机 UDID 作为真机构建捷径。

## 三、<span id="project-model">同步目录与签名</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

生成器使用 `PBXFileSystemSynchronizedRootGroup` 对应物理 `JobsIM/`，由 [**Swift**](https://www.swift.org/) 源码、Assets 和十语 `.lproj` 自动形成 App 成员。`App`、`Resource`、`Services`、`UI` 及下级目录随磁盘变化；`Resource/Info.plist`、`Resource/Simulator.entitlements` 保持可见，但排除自动资源成员资格，继续由构建设置引用。现有工程迁移会移除这些物理文件的旧显式引用，避免重复编译或复制；测试源码和夹具仍归原测试 target。

App 和测试 target 使用现有团队 `K5692KLW75`、Automatic 签名及原 Bundle ID。真机允许正常代码签名；只有模拟器使用 `-` 临时签名及模拟器 entitlements。真机从 `JobsIM.xcworkspace` 的 JobsIM Scheme 选择已连接设备运行，需匹配的有效开发身份、描述文件及手机开发者信任，不修改 Bundle ID 或创建新账号来绕过签名问题。

## 四、<span id="validation">实际验证与边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[2026-11-18 证据](../../../../项目相关报告/测试/前端/iOS/20261118-目录与真机签名/README.md)记录已有工程原位维护与隔离新工程各自连续两次生成幂等，target／Pods／测试配置保留；同一源码随后完成真机编译、签名验证、安装与信任后的启动。本轮没有执行依赖准备或重新安装 Pods，也没有重跑 [**XCTest**](https://developer.apple.com/documentation/xctest)。目录界面目视及完整真机业务流程仍未验。

本 README 在原有 118 项输入封存后补齐说明，不追加进旧输入记录，也不作为新一轮构建或测试证据。

<a id="🔚" href="#前言">返回前言</a>
