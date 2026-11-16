# JobsIM 鸿蒙原生测试

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [当前结果](#jobs-toc-heading-1)
- [使用](#jobs-toc-heading-2)
- [用例与未验边界](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">当前结果</span><!-- jobs-toc:heading-end -->

2026-11-15已通过**真实应用HAP与外置测试HAP构建**，均为未签名包；未执行模拟器／真机安装或Hypium用例。工具来自用户指定libsdl镜像：CLT6.1.1.280、SDK6.1.1.125／API24、Hvigor6.24.2、Node18.20.1。当前25项Hypium用例已编译，不写成25项设备测试通过；另有25项纯规则主机测试实际通过。

[本轮提及公告与分页证据](./报告/20261115-提及公告与分页/README.md)提供成功日志、源码快照清单及HAP／Ark字节码摘要。更早缺SDK、官网取包和构建失败日志保留供核查，当前SDK阻塞已由镜像解除，不再要求登录华为下载门户。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">使用</span><!-- jobs-toc:heading-end -->

在仓库根执行[同名自包含入口](./运行测试.command/运行测试.command)：

```sh
测试/前端/HarmonyOS/运行测试.command/运行测试.command check
测试/前端/HarmonyOS/运行测试.command/运行测试.command build
测试/前端/HarmonyOS/运行测试.command/运行测试.command test-build
测试/前端/HarmonyOS/运行测试.command/运行测试.command emulator
测试/前端/HarmonyOS/运行测试.command/运行测试.command device
测试/前端/HarmonyOS/【MacOS】运行规则测试.command/【MacOS】运行规则测试.command
```

先运行[工具链准备入口](../../../代码/前端/HarmonyOS/工具链/README.md)。`JOBS_IM_HARMONY_CLT_HOME`／`JOBS_IM_HARMONY_CACHE`可指定源码外缓存；缺件仅从libsdl镜像取得，旧健康缓存保留实际来源，不修改全局环境、代理或系统CA。入口直接使用CLT内置插件和Node，`--offline`避免Hvigor包装器安装；本地Hypium依赖来自同镜像SDK，意外网络查询也不回退其他registry。临时Python下载取证脚本不是产品依赖。

官方构建器拒绝中文工程路径，入口在ASCII用户缓存生成本轮源码快照、标准ohosTest布局和必要的本地依赖元数据。原测试物理源仍在本目录OhosTest；只在临时副本调整相对import及依赖位置。`source-manifest.json`记录原始／暂存文件SHA256，HAP导出工程`Dist/`。日志位于`报告/时间目录/`。构建失败保留真实退出码，缺工具／无设备／签名缺失返回非零，不生成假的成功标记。

emulator／device需要已启动并授权的目标及适用开发签名；当前未为截图额外下载模拟器。多个设备使用fzf方向键／回车选择、Esc取消，非交互可用`HDC_TARGET`指定已枚举设备。`HDC_COMMAND`可指定已有HDC。模式与设备类型不匹配会拒绝，不把模拟器记为真机。入口只安装已签名测试HAP，运行`aa test`并要求可信完成计数；未签名包不会强行安装。签名材料和目标设备信任仍是独立前置条件，不因下载源改为镜像而取消。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">用例与未验边界</span><!-- jobs-toc:heading-end -->

外置Hypium当前25项覆盖：可信连接、演示／正式、账户设备与恢复代次隔离，pending本人入站筛选，只读群、原群资格代际，大整数游标、恢复会话账户／设备／epoch核对，严格认证用途JSON、原发送准入、独立收藏／搜索签名路径、结构化选员与owner全体、提及去重／已读／block投影、独立内容版本、公告无正文原请求／清除／期限／明确拒绝及十语。这里只证明测试源码原生编译并进入HAP；未执行设备Hypium。

[主机规则入口](./【MacOS】运行规则测试.command/README.md)使用同镜像SDK已有Node与TypeScript转译当前自有纯规则，Node内置test实际25/25通过，记录见本轮证据；它不执行原生UI、RDB、通知、文件预览或AssetStore。

本轮新增已实现／已编译的联系人、群管理、历史分页／前台同步、草稿／本机未读、原生凭据／设备会话、头像原文件原操作、举报及鉴权附件打开，详见[工程当前能力](../../../代码/前端/HarmonyOS/README.md)。其真实鸿蒙网络联调、原结果丢响应、权限／恢复竞态和UI仍未设备验证；不再把这些已落盘组写成尚无源码，也不把它们写成验收完成。

剩余I和设备门禁包括：完整本人导出／受控导入迁移、收讫资格ACK与清云、容量／无障碍、所有十语／三主题／字号与API12设备组合、系统跟随／权限、RDB耐久、AssetStore锁屏及重启／卸载、通知和媒体预览／播放。签名安装、模拟器与真机均未验；前台轮询不是后台厂商推送，本机未读不是跨端已读。完整243项不据测试数量或HAP构建标为通过。

[原发信资格补齐报告](报告/20261115-原发信资格/README.md)：应用／13用例测试HAP构建与12项主机规则通过；设备及实际鸿蒙HTTP仍未执行。


最新③切片见[提及公告与分页报告](./报告/20261115-提及公告与分页/README.md)。22项Hypium仍是已编译未设备执行；HDC只读枚举为`[Empty]`，未签名包未安装，未进行ArkUI截图或鸿蒙真实HTTPS联调。旧13用例HAP及旧报告原样保留，不把新的22用例源码覆盖成旧证据的实际执行结果。

[原角色资格专项](报告/20261115-原角色资格/README.md)：23项主机规则执行、应用及23用例测试HAP编译通过；设备与真实鸿蒙HTTP未验。

[普通转发限定证据](报告/20261115-普通转发/README.md)：25项主机规则及应用／25用例测试HAP构建通过，设备与真实鸿蒙HTTPS仍未执行。
