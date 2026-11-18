# <span id="前言">登录设置</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、连接与信任](#connection)
- [二、专用演示服务](#demo)
- [三、验证边界](#checks)

</details>
<!-- jobs-toc:end -->

---

[IMAuthSettingsVC](./IMAuthSettingsVC.swift)由登录门右上齿轮推入，沿用未认证根的导航栈和返回按钮。连接地址、连接状态、CA导入／移除、实例验真、演示开关及“重新验证身份”收口在此，不占认证卡片。认证卡片只保留正常注册／登录／找回等用户流程，不再放技术恢复或帮助展开；OTP输入由服务端策略／明确挑战决定，不能由客户端自选是否执行，认证／改密／注销的原证明可用时内部核对，证明缺失时经设置中的重新验证自然提示补充；保留原绑定／nonce，404不自动新建。

## 一、<span id="connection">连接与信任</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

连接仍调用 `IMSession.connect`，显示已验真的实例ID、地址和信任指纹供确认后才调用 `useInstance`；不把网页可达当作实例验真。移除当前CA时保留原退出及客户端重建行为，完成后通知登录门重新核对上下文。

设置仅保留一个“重新验证身份”入口，按当前用户名对应的最新未决操作类型进入既有[改密](../IMPasswordChangeVC/IMPasswordChangeVC.swift)／[注销](../IMAccountDeletionVC/IMAccountDeletionVC.swift)页面的`verificationOnly`模式；该参数默认`false`，正常账户操作入口保持原职责，无未决操作时才进入明确新认证确认。验证模式用“验证身份／继续”等自然提示补充原证明，只核对原绑定的结果，不出现技术恢复菜单或无关OTP。改密／注销原结果查询仍需要原密码证明（改密为原旧／新密码对）；RAM丢失后不能只靠nonce后台查询或换新请求绕过。界面自然提示补充所需证明，继续核原绑定／nonce，404不自动新建；不额外展开技术恢复菜单。管理员强制MFA仍由服务端执行，客户端仅消费明确挑战，不宣称已经交付通用MFA策略开关。

CA文件从 `UIDocumentPickerViewController.makeImporting` 以 `asCopy:true` 导入，不扩大到外部目录扫描；保留大于0且小于65536字节门禁。仍要求手动输入已核对指纹，并在再次显示地址与SHA256后明确确认保存，不关闭TLS验证。

## 二、<span id="demo">专用演示服务</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

开关调用注入的 `IMDemoEnvironmentService.setEnabled`，由服务启动／校验专用主机并提供同profile候选；本页不创建假账号或用本地 `enterDemo()` 代替服务。启动中持续显示“正在启动”，禁用冲突控件但保留导航返回。失败恢复原开关状态并持续显示系统错误对话框；没有演示主机配置时如实阻断。

设置成功通过 `onContextChanged` 通知认证页重置请求签名和旧输入，读取同profile候选；不自动点击登录、不将演示密码存入记住密码。主机控制面、种子数据、部署说明和源配置由对应服务与部署模块维护。

## 三、<span id="checks">验证边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

当前入口收口／服务端挑战及verificationOnly分流已接入冻结build-31，118隔离单元与UI20同候选4方法通过；当前Auth方法覆盖独立设置push／返回及唯一重新验证入口，真实Mac方法覆盖开关／候选预填／普通认证／小林聊天和浮标退出。5项新增隔离MFA契约反例通过，不等于真实管理员挑战UI或真实未决改密／注销验证链已验；这些系统链路仍待专项，不能以按钮存在替代。Mac控制面严格专属CA、当前签名包与结果见[中央报告](../../../../../../项目相关报告/测试/前端/iOS/20261118-UI整改/README.md)。旧UI16与[静态切片](../../../../../../项目相关报告/测试/前端/iOS/20261118-UI整改/AuthRoot/ResumeAuth/README.md)保留原输入；CA文件提供者与所有证书异常不以真实demo成功替代。

<a id="🔚" href="#前言">返回前言</a>
