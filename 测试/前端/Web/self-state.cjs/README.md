# Web 本人状态浏览器专项

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

运行 `node self-state.cjs`，使用 `JOBSIM_WEB_URL` 指向已启动的本机 Vite；`JOBSIM_PLAYWRIGHT_MODULE` 可指定已有 Playwright 模块，默认使用项目可解析的 `playwright`。不安装依赖、不下载浏览器，复用本机 Chrome；模块路径缺失时直接失败。

真实执行 IndexedDB、Web Locks 和 React 本人状态组件，全部 `/api` 请求阻断，业务 HTTP 为独立显式夹具。覆盖 S1→S2 原责任归档、owner clock 与对象版本、原请求恢复、能力撤回、队列身份、可见阅读空洞、云草稿 CAS／恢复及明确采纳。真实接口与 native fsync／合格 ACK 不在本脚本范围。

输出默认位于项目根 `项目相关报告/测试/前端/Web/执行记录/self-state-*`，可通过 `JOBSIM_SYNC_REPORT` 指定新空目录；保存输入 SHA、原始失败与当场截图。开发服务器发生热更新时，模块类可能形成不同 URL 的实例；固定源码后重新启动本轮专用 Vite，再创建全新浏览器 context，保留原失败，不放宽产品校验。

结果范围和已查看截图见[当前实施报告](../../../../项目相关报告/测试/前端/Web/20261116-S2本人状态/README.md)。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
