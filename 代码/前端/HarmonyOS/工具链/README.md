# 鸿蒙工具链

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [实际来源与配套](#jobs-toc-heading-1)
- [构建与间接依赖](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">实际来源与配套</span><!-- jobs-toc:heading-end -->

华为相关新工具／SDK制品统一从用户指定镜像 [libsdl.org/harmonyos-devtools](https://libsdl.org/harmonyos-devtools/) 取得。本工程使用镜像 `commandline-tools-mac-arm64-6.1.1.280.zip` 中的实际配套：CLT6.1.1.280、Hvigor6.24.2、SDK6.1.1.125／API24、Node18.20.1、OHPM6.1.2.268；`modelVersion=6.1.1`，compile／target=`6.1.1(24)`，compatible=`5.0.0(12)`。不把镜像API24写成官方26.0.0，不声称它是最新SDK。当前工程不混用此前官方26.0.0构建器的插件。原工具缓存已在本机清理后丢失，以下部分获取记录属于此前构建的历史事实。

[MacOS arm64准备入口](./【MacOS】准备工具链.command/【MacOS】准备工具链.command)默认check。默认工具根为 `~/Library/Application Support/JobsIM/Toolchains/HarmonyMirror`，避免用户清理Caches时删除已安装SDK。prepare先复用匹配缓存，缺缓存时仅从上述镜像下载完整约1.9GiB的ZIP，逐项CRC验证并记录本地SHA256，再解压到用户持久工具目录。可通过`JOBS_IM_HARMONY_CACHE`或`JOBS_IM_HARMONY_CLT_HOME`指定源码外位置。不修改全局SDK、系统CA、npm配置、Shell启动文件或系统代理。显式`JOBS_IM_DOWNLOAD_PROXY`只传给当前curl进程；取消退出后失效，不做全局路由切换。

旧缓存清空后，2026-11-15 在上述持久工具目录重新按同一 ZIP 中央目录取得 ArkTS 构建所需的 43,497 个条目，随后补齐 29 个组件元数据／许可文件及资源编译器依赖的 5 个原始动态库。资源编译器及其 10 个递归库已核对依赖闭包，实际应用 HAP 构建通过；没有完整获取 2GB 归档。`selected-extraction.json`、`extra-component-metadata.json` 及 `extra-resource-dylibs.json` 记录真实 URL、固定 ETag、每文件 CRC32 和本地 SHA256；旧缓存的 `extra-runtime-libraries.json` 属于历史记录，不是本次恢复清单。

CRC 与本地摘要用于传输完整性核对，**不是发布方签名或完整原包摘要**。部分 ZIP 不当成完整安装包，未下载的 native C++／模拟器／预览器功能不宣称具备。check 会核对两类 SDK 的十个组件元数据以及本工程已知资源编译库，实际编译仍单独验收；先前缺组件元数据、缺动态库的失败原日志保留在[认证专项报告](../../../../项目相关报告/测试/前端/HarmonyOS/20261115-访问刷新/README.md)。构建入口只依赖标准平台工具与已提取 CLT，不依赖本次临时下载取证 Python 脚本。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">构建与间接依赖</span><!-- jobs-toc:heading-end -->

运行[外置测试入口](../../../../测试/前端/HarmonyOS/README.md)的check、build或test-build。入口直接调用镜像包内的Node和Hvigor JS入口，使用其本地插件并传`--offline`，避免hvigorw自动从其它registry安装包装器。Hypium使用同一镜像SDK内置的1.0.0版本：临时构建目录只复制未改动的JS运行库及TypeScript声明，排除SDK编译器用的`.ets`声明，新增独立包元数据；OHPM仅安装此本地路径依赖，意外远程查询仍固定镜像地址并失败，不回退官网或registry.npmjs。

首轮沿旧入口时曾自动获取官方Hypium1.0.21并尝试npm包装依赖，这是已纠正的间接下载路径，失败日志保留。当前成功应用／测试构建使用镜像SDK内置本地依赖，不把旧下载改写成镜像来源。无需华为网页登录来取得当前镜像SDK；开发签名和目标设备信任仍单独验收。

华为Hvigor拒绝中文工程路径。入口把本工程自有源码和外置测试复制到用户缓存的ASCII工作目录，记录`source-manifest.json`。仅在该临时副本调整测试相对import和本地依赖定位；不移动原仓库、不关闭路径／类型检查。开启normalizedOHMUrl和大小写检查，正确解析本地测试库。应用及测试HAP导出`Dist/`，源码／SDK不随安装包整体分发。

版本／接口核对可使用官方文档：[命令行构建](https://developer.huawei.com/consumer/en/doc/doccenter-deveco-studio/ide-command-line-building-app/)、[工程路径约束00306003](https://developer.huawei.com/consumer/en/doc/doccenter-deveco-studio/ide-hvigor-errorcode-00306/)、[RCP原生PATCH及安全配置](https://developer.huawei.com/consumer/en/doc/harmonyos-references/remote-communication-rcp/)。实际制品来源始终是用户指定镜像。核查日期2026-11-15。
