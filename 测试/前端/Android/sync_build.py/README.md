# <span id="前言">Android 权威同步有界构建</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、功能与入口](#jobs-toc-heading-1)
- [二、输出与验证边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、功能与入口</span><!-- jobs-toc:heading-end -->

复用现有 JBR21、SDK37.0 与 Gradle9.5缓存，离线单worker依次编译应用 APK、执行当前自有 Unit 方法（现为 SyncWire5＋Isolation6）、编译androidTest APK；不安装、不启动模拟器、不请求API／SQL，不替换 `Dist` 或e295旧基线。无参数只显示自述；已经协调资源后从仓库任意位置执行：

```sh
python3 测试/前端/Android/sync_build.py/sync_build.py --execute
```

`--max-seconds` 允许30至600秒，默认600；Gradle堆1536m、Kotlin同进程、单worker，JUnit单fork256m。每秒检查本项目卷至少4GiB可用；低空间、超时或取消只停止本次创建的no-daemon进程组，不执行全局stop、clean、SDK下载或其它进程停止。工具缺失直接失败，`JOBSIM_JAVA_HOME`／`ANDROID_SDK_ROOT` 可指定已经存在的工具，不自动安装。

`--report-root 项目相关报告/测试/前端/Android/20261116-S2本人状态` 可将新候选放独立中央切片；相对目录以仓库根解析。它只改变报告输出，不更改产物／数据库或默认测试选择。S2 新增的原生 `SelfStateTest` 在这里仅编译，执行须另行协调仪器窗口。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、输出与验证边界</span><!-- jobs-toc:heading-end -->

每轮新建中央 `项目相关报告/测试/前端/Android/20261116-权威同步/Build-<timestamp>-<pid>-<random>`，保存原输入／SHA、命令与资源门禁、Gradle原始日志、结果。成功时封存本轮两APK／SHA、当前全部JVM原XML（现11项）和Room9 schema；每类方法逐名与XML核对；Gradle UP-TO-DATE复用的XML如实标明，不声称新执行。任何失败保留原日志，不拿旧app/build产物冒充本轮成功。中间限额配置置于当前用户Application Support／JobsIM，不重建旧Caches路径。

工具在前后逐字节核对两份e295认证包；结果只证明编译与JVM，不等于仪器、真实Room执行、界面、HTTPS、断电或完整 I 验收。后续安装／仪器须另行协调专用窗口；本入口不会触发它。Ctrl+C取消构建，保留当前报告和编译缓存。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
