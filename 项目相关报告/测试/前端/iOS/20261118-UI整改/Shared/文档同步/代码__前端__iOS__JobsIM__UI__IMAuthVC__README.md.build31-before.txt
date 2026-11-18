# <span id="前言">登录门与认证卡片</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

[IMAuthVC](./IMAuthVC.swift)负责登录／注册／找回密码和原认证未知结果入口；[登录设置](../IMAuthSettingsVC/README.md)通过右上角齿轮独立 push。界面使用 [**UIKit**](https://developer.apple.com/documentation/uikit) 与 [**SnapKit**](https://github.com/SnapKit/SnapKit)，保留同一表单和64pt实心侧栏左右换位的 AppDoor Style1 动画。

## 一、<span id="appearance">界面与背景</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

主标题18pt、输入14pt、辅助12pt，实色切换侧轨文字18pt，并沿用应用字号调整；标题在不含侧栏的表单内容区居中。登录／注册副说明统一“Jobs荣誉出品”。卡片不允许用户上下拖动、惯性滚动或回弹；键盘焦点引发的程序化定位／避让保留，控件通过重排保持可达。密码及确认密码末端的眼睛分别控制明暗，眼睛／记住圆形图标视觉18～20pt、触控最小44×44pt；操作文案随下一次行为变化；账号、输入文字、背景和描边使用成对的明暗语义色。

[IMAuthBackgroundView](./IMAuthBackgroundView.swift)默认读取打包的[山湖图片](../../Resource/AuthMedia/AuthBackground.jpg)，兼容 `AuthMedia/` 子目录及构建资源平铺；[ImageIO工厂](../Factories/UIImage/IMAuthThumbnail+Make.swift)直接从URL缩采样至最长边1600，避免先解码完整4226×2847图。素材出处与许可由项目资产文档统一维护。

背景也可在源码配置为 `.image(named:)` 资产图或 `.video(name:fileExtension:posterNamed:)` 打包视频。视频静音循环，离开登录页、进入后台或开启减少动态效果时暂停；保留静态海报，不下载外部视频。中间表单使用系统薄毛玻璃及受控半透明底色；开启减少透明度时使用实底。左右切换侧栏始终保持品牌实色。

## 二、<span id="credentials">凭据、连接与演示边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

密码下方左侧为默认关闭的空／实圆“记住密码”，右侧为找回密码；两者与密码框左右边缘对应。候选只从当前profile的`IMRememberedLoginStore`恢复，记录绑定UID与用户名，不自动登录。显式取消立即忘记；忘记失败会恢复勾选并显示错误。只有登录成功、profile／deployment／URL与提交时一致且当前 verified pair匹配时才保存，注册、恢复、失败、未知结果和演示凭据均不保存。completed注销按UID精确清除本人所有历史用户名记录；旧无UID记录只按原注销单核验用户名兼容清理，保留其它账户。

`IMDemoEnvironmentService` 的同profile候选优先于记住密码，并以明文显示；用户仍需点击登录请求专用服务。这里没有旧的`enterDemo()`直接跳转按钮。卡片不展示客户端OTP／双重验证自选，也不展示技术恢复按钮或帮助展开；OTP字段只在服务端明确挑战后显示并沿原认证责任处理。连接未配置或服务器不可用时提供持续显示的系统对话框，左取消、右前往设置；不会自动消失或绕过认证。

切换实例后清输入、OTP和本页请求签名，再读取新profile候选。提交捕获 mode／requestID及连接上下文，进行中禁用表单和切换入口；原`pendingAuthentication`的原证明确认和固定请求责任保留；认证／改密／注销仅在原证明可用时内部核对原结果，RAM证明丢失时经设置中的重新验证自然提示补充（改密仍需原旧／新密码对），原绑定／nonce保持且404不自动新建，服务端失败不伪装为成功；显式“重新验证身份”放在独立登录设置，正常找回密码仍保留。

## 三、<span id="checks">验证与标识</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

已验冻结build-22／device-build-7对应113单元及UI16十方法，UI18四指定已另通过并目视玻璃／浅色辅助说明；这些证据分别按其冻结源解释。上述18pt侧轨、禁手势、统一副说明、图标／触控、服务端挑战和入口收口已在当前生产源码接入并冻结，尚待对应构建和实测，不借旧包追认。此前认证标题／输入边界、双眼睛、记住按钮、设置push／返回、演示浮标退出均在模拟器实际覆盖。静态原件保留在[原冻结切片](../../../../../../项目相关报告/测试/前端/iOS/20261118-UI整改/AuthRoot/ResumeAuth/README.md)，当前成品与系统结果见[中央证据](../../../../../../项目相关报告/测试/前端/iOS/20261118-UI整改/README.md)；视频实播与完整iPhone交互不由默认图片态推定通过。

主要可访问标识：`auth.settings`、`auth.cardTitle`、`auth.modeSwitch`、`auth.username`、`auth.password`、`auth.confirmPassword`、`auth.password.visibility`、`auth.confirmPassword.visibility`、`auth.rememberPassword`、`auth.recover`、`auth.primary`。所有新增文字集中维护十种 `Localizable.strings`，不修改第三方 Pods。

<a id="🔚" href="#前言">返回前言</a>
