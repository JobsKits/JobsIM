# JobsIM · Xcode 依赖快捷入口

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、入口与配置](#jobs-toc-heading-2)
- [二、目标、确认与作用范围](#jobs-toc-heading-3)
- [三、只读检查、日志与验证](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本目录提供 [**Xcode**](https://developer.apple.com/xcode/) Behaviors 使用的两个自包含脚本包，复用本人 Swift UIKit 模板的实现。脚本随 iOS 工程携带，不依赖工程外脚本库；Behaviors 是用户级设置，换电脑仍需登记。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、入口与配置 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| Behavior | 包内说明 | Run 脚本路径（相对 iOS 工作区目录） |
| --- | --- | --- |
| `🫘当前工程 · Pod Install` | [安装依赖](<./【MacOS@Xcode】🫘当前工程Pod Install.command/README.md>) | `ScriptsByPods/【MacOS@Xcode】🫘当前工程Pod Install.command/【MacOS@Xcode】🫘当前工程Pod Install.command` |
| `🫘当前工程 · Pod Update` | [更新依赖](<./【MacOS@Xcode】🫘当前工程Pod Update.command/README.md>) | `ScriptsByPods/【MacOS@Xcode】🫘当前工程Pod Update.command/【MacOS@Xcode】🫘当前工程Pod Update.command` |

在 `Xcode → Behaviors → Edit Behaviors…` 启用相应行为的 Run 并保持上述相对路径；本机已有这两项设置。激活 `../JobsIM.xcworkspace` 后从菜单调用。不要将入口登记成模板工程的绝对路径；移动工程时保留本目录及其执行权限。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、目标、确认与作用范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

脚本优先使用显式 `--xcode-path`，否则读取 Xcode 本次传入的 `XcodeWorkspacePath`／`XcodeProjectPath`，只在当前工程祖先目录寻找最近的 Podfile，遇仓库边界停止。缺少工程上下文、路径不存在、生成的 Pods／Swift Package 工程或明确的非 iOS 声明都会拒绝；不猜测历史工程或工作目录。

入口打开 Terminal 并显示精确目标，**空回车执行基础模式，大写 `YES` 执行完整模式，其它输入、EOF 或 Ctrl+C 取消**。基础模式只调用所选 `pod install`／`pod update`，并为该子进程设置四项 `JOBS_POD_*` 开关以供支持它们的 Podfile 跳过附加任务；完整模式先调用 `pod --version`，不注入这四项开关。JobsIM 当前 Podfile 没有读取这些开关，两种模式均执行其现有内联 `post_install`，维护 Pods 工程 Ruby 文件引用及构建设置；没有额外外援脚本或自动 Build Phase 挂载。

两个入口均复用现有 CocoaPods，不安装或升级 Ruby、Homebrew、CocoaPods；缺失时停止。实际依赖操作可下载依赖，修改 `Pods/`、`Podfile.lock` 及工作区。Install 优先复用锁定依赖；Update 按 Podfile 版本范围重新解析全部依赖。Podfile 对本人模板与本地 Vendor 的依赖保持其原有要求，首次准备按[工程说明](../README.md#native-run)执行。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、只读检查、日志与验证 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

在本目录运行，`--check` 不调用 pod、不执行 Podfile、不创建日志或打开终端：

```shell
/bin/zsh './【MacOS@Xcode】🫘当前工程Pod Install.command/【MacOS@Xcode】🫘当前工程Pod Install.command' \
  --check --xcode-path "$(cd .. && pwd)/JobsIM.xcworkspace"
/bin/zsh './【MacOS@Xcode】🫘当前工程Pod Update.command/【MacOS@Xcode】🫘当前工程Pod Update.command' \
  --check --xcode-path "$(cd .. && pwd)/JobsIM.xcodeproj"
```

确认执行后，每次在系统临时目录建立权限受限的独立 `jobs-xcode-pod-install.log.*`／`jobs-xcode-pod-update.log.*`，终端显示完整路径。安装、更新及日志失败保留真实非零退出码；打开 Terminal 仅表示交接成功，最终结果以终端和日志为准。取消不启动依赖操作，开始后的 CocoaPods 修改不自动回滚。

两份脚本与本人 Swift UIKit 模板逐字节一致；语法、执行权限、工作区／工程定位和迁移路径检查通过。真实 CocoaPods 安装、更新与 App 构建未由本入口验证，不据路径检查宣称依赖可构建。

支持完整配色保存的终端会临时使用黑底白字，正常结束或可捕获中断后恢复；非 TTY、`NO_COLOR` 或无法安全保护时保留纯文本。强制终止后的配色恢复按运行时显示的恢复入口处理。

[返回 iOS 工程说明](../README.md#native-project-support)

<a id="🔚" href="#前言">返回前言</a>
