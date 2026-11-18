# <span id="前言">JobsIM 机器索引、二维码与 iPhone 工程交付</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、用户问题与实现范围](#jobs-toc-heading-1)
- [二、实际结果](#jobs-toc-heading-2)
- [三、需求、资产及历史证据](#jobs-toc-heading-3)
- [四、运行与数据边界](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、用户问题与实现范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

本轮处理用户提供的四张截图和真机安装错误：代码地图应是项目根实际`.codegraph/`机器索引，终端应直接显示二维码，Xcode主工程应完整对应磁盘目录，iPhone成品须有有效签名。人工README导航、PNG文件生成、模拟器编译和无签名App都不能替代对应交付。

| 问题 | 根因与实现 | 验证入口 |
| --- | --- | --- |
| AI代码地图 | 原来只有人工职责说明；复用Finder“更新代码地图”所用官方CodeGraph，以根codegraph.json控制当前源码索引，排除依赖／构建／历史副本和秘密，实际.codegraph由本机重建 | [CodeGraph](../../工具/20261118-CodeGraph/README.md) |
| 终端二维码 | 公共工具原先只有显式开启终端绘制才画码，Mac/Linux漏传，Windows宽度固定0；修正当前终端尺寸／渲染与窄窗展示，保留URL、离线页和仅本机标签 | [二维码](../../部署/20261118-终端二维码/README.md) |
| Xcode文件平铺 | 项目与生成器使用平铺的虚拟group；改为同步磁盘文件夹，保留App／Resource／Services／UI层级，核对target、资源例外、外置测试及Pods集成 | [目录与签名](../../前端/iOS/20261118-目录与真机签名/README.md) |
| iPhone无签名 | 当前工程与生成器明确关闭iphoneos签名；改为真机有效签名，模拟器设置按SDK隔离，优先复用已有团队／证书／匹配设备的描述文件 | 同上，编译／签名／安装／启动分别记录 |

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、实际结果 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 检查 | 实际结果与范围 |
| --- | --- |
| iOS项目与生成器 | 同步根对应App／Resource／Services／UI；既有target UUID、Pods构建phase／xcconfig、外置测试资源保留，existing／fresh生成两轮幂等 |
| iPhone签名构建 | build退出0，用时118.42秒；78物理Swift及1个Xcode生成文件实际编译，10语言／Assets在包，Info正常处理、模拟器entitlements未复制；严格codesign验证通过 |
| 实际安装与启动 | 复用既有团队／有效证书与匹配设备的描述文件，正常安装成功；首次手机尚未信任的拒绝原件保留，用户确认信任后只重试一次启动成功；PID85059及执行路径与原已安装成品一致，无重新构建／重装／卸载清数据 |
| 终端二维码 | 18个Mac真实PTY场景通过；可靠背景格、深浅Menlo成像解码逐字等于原URL，窄列／矮行／NO_COLOR走本轮PNG，实际fzf前留码和非TTY不等待；跨平台调用／语法见独立摘要 |
| 当前机器索引 | 720文件／17,464节点／62,550边，775份源码／配置输入无漂移；严格状态完整、待处理／解析错误为0，SQLite完整性ok，8次真实源码／路径查询通过，禁入路径0；249本人测试中144自动识别为测试、105仍能按路径检索。Shell／PowerShell不支持语义索引，静态图不证明完整动态调用；不靠目录存在判成功 |

[最终输入与证据核对](./现行输入与证据核对.json)逐项核了775份CodeGraph输入、20份二维码输入和118份iOS输入，未见漂移；这些集合有交集，不相加为独立功能或验收项。签名成品二进制与本轮构建摘要仍匹配；本仓256份受控文档的目录往返／幂等检查通过，未暂存、提交或推送。

Xcode导航器无可访问图形窗口，本轮验证的是项目模型、物理树、真实编译清单和产物，不称GUI目视通过；未重跑完整XCTest、全部真机业务／其它设备与团队。二维码可见和解码不代表手机能够访问回环地址或公网已通。前轮Linux96、Windows38、Mac50仍只证明原冻结输入；本轮二维码修改以独立定向结果和新SHA记录，不借旧绿色计数追认新源。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、需求、资产及历史证据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

现行[需求总纲](../../../../需求/IM需求明细表.md/IM需求明细表.md#guided-installation)、[前端工程合同](../../../../需求/IM前端架构表.md/IM前端架构表.md#ios-pods)、[T-48／ARCH-20](../../../../需求/IM功能验收表.md/IM功能验收表.md)直接维护最终要求，未新增平行版本或主验收编号。人工职责文档继续保留，并明确机器索引入口；[资产盘点](../../../资产盘点/README.md)登记入口、默认行为、生命周期与验证边界。

[前一轮成对部署](../../部署/20261118-成对部署反部署/README.md)和[原全项目回归](../20261118-全项目回归/README.md)的原始日志、SHA及失败保留，分别只证明当时冻结输入。本轮不借用前轮“二维码PNG能生成”作为终端已显示，也不借用模拟器构建作为iPhone签名安装成功。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、运行与数据边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

源码、项目配置和生成器保持同源；不改第三方Pods或供应商源码，不为界面目录清理用户数据、替换Bundle ID或创建新Apple身份。代码索引使用已有健康工具，不安装全局工具或留下额外Git自动钩子。二维码专项只使用独立PTY／编码器夹具，不启停用户部署实例或擅自公开地址；真机安装按用户要求正常升级，不卸载App清数据。

<a id="🔚" href="#前言">回到首页</a>
