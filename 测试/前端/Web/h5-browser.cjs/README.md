# <span id="前言">H5 浏览器专项</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、运行与范围](#jobs-toc-heading-1)
- [二、证据边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、运行与范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

运行 `node h5-browser.cjs`，从脚本位置解析工程。需要已准备的 Web 依赖、Playwright 与 Chrome；`JOBSIM_PLAYWRIGHT_MODULE`、`BROWSER_EXECUTABLE` 可指定已有工具，`JOBSIM_H5_OUTPUT` 指定本轮新报告目录。入口启动私有随机端口 Vite，拦截 API 到独立本地演示，不接实际部署实例。

覆盖 320／390／430 竖屏和 844 横屏、真实触控点击、发送／返回／认证／设置、三档字号与十语言、三主题、可见视口 300 与布局和可见视口同时缩到 230、偏移／捏合、附件选择、合成麦克风录音预览与取消、短视口消息阅读区、滚到完整且无遮挡的消息文字行和可见已读裁剪。监听全部 pageerror，异常使套件失败。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、证据边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[2026-11-18 原件与最终摘要](../../../../项目相关报告/测试/前端/Web/20261118-H5适配/summary.json)保留阅读区修前 23 过／2 失败与修后 52 过；另有 self-state、桌面 browser、类型检查和私有构建日志。截图已实际查看。套件计数分别记录，不合并为需求验收总数。

Chrome 移动与触控模拟、显式 visualViewport 夹具不等于 iOS／Android 真机键盘或刘海安全区；合成麦克风使用真实 MediaRecorder，但不是物理麦克风或公网 HTTPS 验收。

<a id="🔚" href="#前言">回到首页</a>
