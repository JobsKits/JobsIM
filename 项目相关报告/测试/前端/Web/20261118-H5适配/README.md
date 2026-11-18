# <span id="前言">JobsIM 桌面与手机 H5 验证</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、结果与输入](#jobs-toc-heading-1)
- [二、当前行为与生命周期](#jobs-toc-heading-2)
- [三、反例与目视检查](#jobs-toc-heading-3)
- [四、复核入口](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、结果与输入 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

Web 使用同一份 TypeScript／React 前端和 API，同时覆盖桌面浏览器与手机 H5。当前合同见[前端架构 1.7](../../../../../需求/IM前端架构表.md/IM前端架构表.md#mobile-web-h5)；H5 仍计入 Web 端，不另造跨平台 App。当前源码、测试 SHA、日志与构建位置以[最终摘要](./summary.json)为准。

| 验证 | 结果 | 原件与边界 |
| --- | --- | --- |
| Chrome 移动与触控 | 52 通过／0 失败／0 跳过 | [逐项记录](./最终阅读行验证/summary.json)，API 显式断开并进入本地演示 |
| self-state | 22 通过／0 失败 | [日志](./最终阅读行构建/self-state.log)，既有状态与读取资格回归，不是真实跨端 API |
| 桌面 browser | 8 通过／0 失败 | [日志](./最终阅读行构建/browser.log)，桌面分栏与既有交互 |
| TypeScript／生产构建 | 退出码均为 0 | [命令与结果](./summary.json)，独立 Dist，不替换用户运行实例 |

各套件独立计数，不相加成需求完成数。iOS／Android 真机、物理软键盘、刘海安全区、物理麦克风、真实 HTTPS 与最终用户文件保存尚未由本报告验收。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、当前行为与生命周期 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[可见视口钩子](../../../../../代码/前端/Web/Src/useVisibleViewport.ts)监听 visualViewport、窗口尺寸与输入焦点，按实际可见高度／偏移布局；捏合缩放保留原文档布局。卸载时取消动画帧、移除监听和临时 CSS 属性。没有新增用户持久偏好，语言、主题、字号和身份／消息协议沿用原存储。

[App](../../../../../代码/前端/Web/Src/App.tsx)按实际可见侧栏决定快捷键行为，已读观察取消息容器与可见视口交集，缩放或偏移挡住的消息不作为完整可见。[样式](../../../../../代码/前端/Web/Src/styles.css)覆盖触控横屏、安全区、模态滚动、短视口紧凑输入、附件及录音预览。极短230px夹具保留至少32px可滚动消息容器及发送／本地演示提示，紧凑视口内置顶条随消息滚动。实际滚到消息后验证完整一行文字位于容器及可见视口内，并以elementFromPoint核对没有被置顶条遮挡；仍不表示真实手机输入体验已验收。

浏览器用例覆盖 320／390／430 竖屏与 844 横屏、触控会话导航／发送／返回、认证弹窗、附件文件选择、合成麦克风 MediaRecorder 预览与取消、十语言×三字号、三主题，以及布局和可见视口同步缩到 230px、视口偏移和捏合时的读取裁剪。全程监听 pageerror，未处理异常会使套件失败。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、反例与目视检查 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[修前有效结果](./修前有效/summary.json)为 19 通过／5 失败。首轮几何检查通过后，目视发现横屏输入区挤占消息空间，遂收紧阅读区门禁；[阅读区反例](./阅读区反例/summary.json)保留 23 通过／2 失败。后续目视仍发现230px容器被sticky置顶条盖住；[置顶遮挡反例](./置顶遮挡反例/summary.json)为51通过／1失败，证明容器高度本身不能保证阅读。仅在紧凑视口让置顶条随历史滚动后，最终52项全过。原失败与中间轮原件保留，不改写为成功。

已目视[横屏](./最终阅读行验证/chat-844x390.png)、[230px完整消息行](./最终阅读行验证/keyboard-390x230-message-line.png)及[小屏大字号设置](./最终阅读行验证/settings-320-large.png)。截图验证的是 Chrome 模拟中的布局；触控、合成音轨和 visualViewport 注入分别在测试中注明。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、复核入口 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[独立测试及运行说明](../../../../../测试/前端/Web/h5-browser.cjs/README.md)从自身位置解析工程，用随机本地端口启动测试服务；报告目录显式指定，退出关闭自有浏览器与 Vite。当前 Web 包亦由[MacOS 部署验证](../../../部署/20261118-成对部署反部署/README.md)按输入 SHA 和实际 HTTP 返回字节另行核对；这不代替手机扫码与真机交互。

<a id="🔚" href="#前言">回到首页</a>
