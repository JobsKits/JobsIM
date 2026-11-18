# <span id="前言">iOS 目录与真机签名验收</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前结果](#results)
- [二、源与产物对应](#source-product)
- [三、目录模型与界面边界](#hierarchy)
- [四、签名、安装与信任记录](#signing-trust)
- [五、未验范围](#limits)

</details>
<!-- jobs-toc:end -->

---

本目录记录 2026-11-18 的工程同步目录、真机编译、签名、安装和启动结果。当前结论以已封存的 [summary.json](./summary.json) 及各层原始证据为准；本说明后补，不改写原输入、失败记录或产物。

## 一、<span id="results">当前结果</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

> [**Xcode**](https://developer.apple.com/xcode/) 27.2 与 [**iPhone**](https://www.apple.com/iphone/) 13／[**iOS**](https://www.apple.com/ios/) 27.2.1 的本轮限定验证结果如下。

| 层次 | 结果与边界 | 原件 |
| --- | --- | --- |
| 工程模型 | 主 `JobsIM/` 为文件系统同步组；已有／隔离新工程重复生成均幂等，原 target UUID、Pods 阶段、测试源码资源及基础 xcconfig 保留 | [结构核对](./project-structure.json)、[新工程隔离核对](./generator-isolated.json) |
| 实际编译与资源 | Debug 真机编译成功；78 份物理 [**Swift**](https://www.swift.org/) 全部进入实际文件表，另含 1 份 Xcode 生成的 Assets 符号；十语词库和 Assets 入包，Info.plist 正常处理，模拟器 entitlements 未复制为资源 | [构建结果](./device-build.json)、[完整日志](./device-build.log)、[产物核对](./product-audit.json) |
| 代码签名 | 复用现有开发身份及匹配团队／Bundle ID／设备的描述文件，严格签名验证退出码 0 | [签名验证](./codesign-final.log)、[签名详情](./codesign-detail.log)、[entitlements](./entitlements.plist) |
| 实际安装 | `com.jobs.im` 正常安装成功，未执行卸载或数据清理 | [安装回执](./install.json)、[安装日志](./install.log)、[设备应用查询](./apps-after.json) |
| 实际启动 | 用户信任现有开发者并保持解锁后，对同一已安装成品仅重试一次成功；PID 85059 在后续查询仍运行，路径与安装回执一致 | [启动回执](./launch-after-trust.json)、[启动日志](./launch-after-trust.log)、[最终进程](./processes-final.json) |

构建使用现有 DerivedData，耗时 118.42 秒；监测到的最低可用空间为 8,859,475,968 字节，高于本轮 4GiB 保留线。该数据只描述本次执行，不作为最低磁盘或性能承诺。

## 二、<span id="source-product">源与产物对应</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

修改范围为[工程文件](../../../../../代码/前端/iOS/JobsIM.xcodeproj/project.pbxproj)、[生成器](../../../../../代码/前端/iOS/Scripts/GenerateProject.rb)及[客户端说明](../../../../../代码/前端/iOS/README.md)。脚本职责见[Scripts README](../../../../../代码/前端/iOS/Scripts/README.md)；第三方源码、Pods 生成文件和业务 Swift 源码均未作为此次修复对象。

[构建输入](./build-inputs.json)保存 111 项自有源码及工程 SHA；[最终输入](./input-after.json)共 118 项，含配置和当时文档，核对无漂移。改前配置保存在 [before/](./before/)，改前 SHA 见 [input-before.json](./input-before.json)。本 README 及 Scripts README 均在封存后新增，不属于旧 118 项验证输入。

真机成品路径记录于 `summary.json` 的 `product.appPath`，位于本机既有 DerivedData 的 `Debug-iphoneos/JobsIM.app`，不是模拟器产物。安装及启动使用同一份成品；首次信任失败到成功重试之间没有重建、重装或换证书。

| 产物 | SHA256 |
| --- | --- |
| `JobsIM` | `11d0966feace9380d3852ae16b198b2281ce533f04fac9f5a963276965782a0c` |
| `JobsIM.debug.dylib` | `ac8e6694348ab76de95d96a145bfe92d014c09864fafc617f5f831999d390d30` |

## 三、<span id="hierarchy">目录模型与界面边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

主组采用 `PBXFileSystemSynchronizedRootGroup`，直接绑定磁盘 `JobsIM/` 下的 `App`、`Resource`、`Services`、`UI`，物理层级见 [physical-tree.json](./physical-tree.json)。`Resource/Info.plist` 与 `Resource/Simulator.entitlements` 排除自动 target 成员资格，继续由构建设置使用；测试目录及夹具保持原 target 归属。

目录结论同时依据工程模型、真实物理树、实际编译文件表及成品资源，不仅是同步组对象存在。当前 Xcode 脚本模型可定位工作区，但辅助功能接口返回零个窗口，窗口枚举也没有可见 Xcode 窗口；没有完成导航栏图标及展开层级的界面目视验证，不以模型核对替代该项。

## 四、<span id="signing-trust">签名、安装与信任记录</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

工程及生成源已移除真机专属 `CODE_SIGNING_ALLOWED=NO`，保留模拟器专属临时签名。现有团队为 `K5692KLW75`，Bundle ID 为 `com.jobs.im`；本轮复用身份 SHA1 为 `624B5DAF975AACD47C00DCB0EEC845CCC35FD400`，匹配描述文件包含该设备，记录有效期至 2026-11-25 02:54:33 UTC。它们是本轮环境快照，后续仍须检查有效期和设备资格；未创建账号或证书，未导出私钥。

改前缓存产物确实未签名，失败见 [codesign-before.log](./codesign-before.log)。修复后的严格签名验证和实际安装均成功，但首次启动仍被设备 `SBMainWorkspace` 以 Security 拒绝，错误包含签名／entitlements／用户未显式信任等可能原因；原始 [launch.json](./launch.json) 与 [launch.log](./launch.log)完整保留。用户确认信任已有开发者后，同一已安装成品启动成功，最终进程路径与安装路径一致。本地签名成功、设备安装成功和用户信任后启动成功分别记账，不将第一轮失败抹成一次全绿。

## 五、<span id="limits">未验范围</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本轮未重跑 [**XCTest**](https://developer.apple.com/documentation/xctest)，未验完整真机业务、其它设备／系统／签名团队，也未取得 Xcode 导航界面的目视证据。此前模拟器或业务回归保持各自输入和结果，不能并入本次签名验收。

<a id="🔚" href="#前言">返回前言</a>
