# <span id="前言">登录设置</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

[IMAuthSettingsVC](./IMAuthSettingsVC.swift)由登录门右上齿轮推入，沿用未认证根的导航栈和返回按钮。连接地址、连接状态、CA导入／移除、实例验真及演示开关收口在此，不占认证卡片。

## 一、<span id="connection">连接与信任</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

连接仍调用 `IMSession.connect`，显示已验真的实例ID、地址和信任指纹供确认后才调用 `useInstance`；不把网页可达当作实例验真。移除当前CA时保留原退出及客户端重建行为，完成后通知登录门重新核对上下文。

CA文件从 `UIDocumentPickerViewController.makeImporting` 以 `asCopy:true` 导入，不扩大到外部目录扫描；保留大于0且小于65536字节门禁。仍要求手动输入已核对指纹，并在再次显示地址与SHA256后明确确认保存，不关闭TLS验证。

## 二、<span id="demo">专用演示服务</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

开关调用注入的 `IMDemoEnvironmentService.setEnabled`，由服务启动／校验专用主机并提供同profile候选；本页不创建假账号或用本地 `enterDemo()` 代替服务。启动中持续显示“正在启动”，禁用冲突控件但保留导航返回。失败恢复原开关状态并持续显示系统错误对话框；没有演示主机配置时如实阻断。

设置成功通过 `onContextChanged` 通知认证页重置请求签名和旧输入，读取同profile候选；不自动点击登录、不将演示密码存入记住密码。主机控制面、种子数据、部署说明和源配置由对应服务与部署模块维护。

## 三、<span id="checks">验证边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

当前仅源码和静态检查。新设置页的实际push／返回、文件提供者、严格TLS和专用主机启用仍需统一原生／部署验证；不能由旧登录页或旧本地演示的结果推定通过。[证据入口](../../../../../../项目相关报告/测试/前端/iOS/20261118-UI整改/AuthRoot/ResumeAuth/README.md)。

<a id="🔚" href="#前言">返回前言</a>
