# <span id="前言">iOS 认证、根导航与个人中心整改</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、源码范围](#scope)
- [二、图标与十语](#assets)
- [三、已完成检查](#checks)
- [四、统一构建与待验](#pending)

</details>
<!-- jobs-toc:end -->

---

本目录记录 2026-11-18 的源码切片与低磁盘静态检查。当前 [summary.json](./summary.json) 明确区分源码、语法、静态颜色和实际运行；本轮没有构建、启动模拟器或安装设备。

## 一、<span id="scope">源码范围</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

认证页参考用户指定的 OC 新工程 `JobsByPods/JobsAppDoor@Pods/Core/JobsAppDoorStyle1`，已核对 Jobs 文件头，只读借鉴其共用卡片与左右换边栏。目标仍为 [**Swift**](https://www.swift.org/) 原生页面，不迁入 OC 的账号、短信或示例业务。

- 认证门保留原登录／注册／恢复、请求身份与未知响应确认、服务器／CA 信任及原操作核对；64pt 切换栏左右换边，共用账号／密码输入。卡片切换采用参考的 0.7 秒、延迟 0.1 秒、阻尼 1／速度 0.1；首次入场 `0.01 → 1.1 → 1`，减少动态效果时直接显示稳定状态。
- 新增 `IMRootVC`、`IMPersonalCenterVC`、`IMUserProfileVC`。三个 Tab 顺序为联系人、消息、个人中心；末项使用当前用户名和头像，收藏及设置收进个人中心。
- 根页隐藏重复标题并释放原标题栏高度；演示根页显示“当前演示”。演示保留显式入口，调试 `--demo` 限于 DEBUG。
- 本人资料编辑及账户切换调用主代理维护的 Session 合同。界面不清 Keychain／Outbox，不实现本地假改名或自动切回；资料结果另核命名空间与凭据家族，失权时清展示。
- 设置 cell 使用统一页面／卡片／选中背景和主副文字颜色，补演示退出恢复正式账号时的误报判断。

联系人／会话／聊天／消息气泡由 B 代理维护；共享 Session／Transport／后端由其它代理维护，未纳入本目录自己的已验证范围。

## 二、<span id="assets">图标与十语</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

App 图标复用原 JobsIM `Logo.svg` 的绿色与白色消息图形，仅改为不透明方形底并交系统裁切圆角。1024×1024 PNG 已查看，来源与 SHA 见 [icon-provenance.json](./icon-provenance.json)；未以新占位图代替品牌，也未下载素材。图标是否正确出现在设备主屏仍待统一构建安装。

简繁藏英日法西德俄韩共十份词库由本代理集中维护，当前各 413 键，新增各 53 键，保留原值且无重复、缺键或语法错误。新增项包括 B 代理及共享服务需要的文案，逐语内容见 [localization-additions.json](./localization-additions.json)；本机语法检查不等于全部译文经母语审校或长文案实际布局通过。

## 三、<span id="checks">已完成检查</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

> 本次空间低于 4GiB，遵守重型门禁，仅执行小范围静态检查。

| 检查 | 当前证据与范围 |
| --- | --- |
| 九份 Swift 语法 | `swiftc -frontend -parse` 退出码 0；[日志](./swift-parse.log)，未做类型检查或链接 |
| 十语文件 | `plutil -lint`、键集、重复及原值核对，结果见 [summary.json](./summary.json) |
| 语义颜色 | 明暗主题的主字／副字／强调字对页面／卡片／选中底色，静态对比最低 4.596:1；白字主按钮为 5.042:1，未替代界面截图验色 |
| 图标 | 1024×1024、无 Alpha，本地图像已查看；设备图标未验 |
| 输入对应 | [baseline.json](./baseline.json) 为修改前；[inputs.json](./inputs.json) 为当前自有切片，旧回归或签名产物不覆盖新 UI 源码 |

## 四、<span id="pending">统一构建与待验</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

共享 API 类型对接、[**Xcode**](https://developer.apple.com/xcode/) 编译链接、认证三状态、键盘／小屏／横屏、三态主题截图、长翻译、真实账户切换与资料更新、签名安装后的图标和 Tab 都待主代理统一验证。之前真机签名安装启动成功属于上一轮冻结源，不作为本轮 UI 已运行的证据。

<a id="🔚" href="#前言">返回前言</a>
