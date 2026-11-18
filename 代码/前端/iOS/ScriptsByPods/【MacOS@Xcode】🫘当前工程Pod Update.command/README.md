# <span id="前言">Xcode · 当前工程 Pod Update</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、用途与操作](#jobs-toc-heading-1)
- [二、只读检查与日志](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、用途与操作 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[主脚本](<./【MacOS@Xcode】🫘当前工程Pod Update.command>)由 `Xcode → Behaviors → 🫘当前工程 · Pod Update` 手动启动，识别当前 iOS 工作区及最近的 Podfile，再交给 Terminal 确认。空回车执行不带 Pod 名的 `pod update`，精确输入大写 `YES` 先运行 `pod --version` 后进入完整更新流程；其它输入、EOF 或 Ctrl+C 取消。无 Xcode 上下文的双击会停止，终端可用 `--xcode-path` 明确传入现存工程的绝对路径。

更新会按 Podfile 允许范围重新解析全部依赖，可能下载依赖并修改 Pods、锁文件和工作区。日常复用锁定版本优先选择[Pod Install](<../【MacOS@Xcode】🫘当前工程Pod Install.command/README.md>)。本包自包含且不调用 Install 脚本，复用已有 [**CocoaPods**](https://cocoapods.org/)，不安装或升级全局工具；JobsIM 当前 Podfile 的内联配置 hook 保持执行。开关、目标校验、模板依赖、配色恢复与迁移要求见[目录说明](../README.md)。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、只读检查与日志 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

在本脚本包目录运行：

```shell
/bin/zsh './【MacOS@Xcode】🫘当前工程Pod Update.command' \
  --check --xcode-path "$(cd ../.. && pwd)/JobsIM.xcodeproj"
```

`--check` 只读定位，不调用 pod、不写日志、不打开 Terminal；`--help` 显示参数说明，`--terminal` 仅供内部交接使用。确认更新后才创建系统临时目录内的独立 `jobs-xcode-pod-update.log.*`，实际路径在终端显示。更新或日志写入失败保留非零退出码；依赖操作开始后的改动不会自动回滚。

脚本字节、执行位、语法及只读路径定位已核对，真实更新／构建未验。

[返回入口说明](../README.md) · [返回工程配置](../../README.md#native-project-support)

<a id="🔚" href="#前言">返回前言</a>
