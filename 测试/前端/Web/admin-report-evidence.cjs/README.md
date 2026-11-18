# <span id="前言">管理员举报证据测试</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行](#run)
- [二、范围与边界](#cases)

</details>
<!-- jobs-toc:end -->

---

[协议执行器](./admin-report-evidence.cjs)直接加载生产 `AdminReportProtocol.ts` 和十语标签，使用 [**Node.js**](https://nodejs.org) 22.13+ 自带的类型剥离。[浏览器执行器](./browser.cjs)通过隔离的 [**Vite**](https://vite.dev/) 入口加载实际管理组件，使用现有 [**Playwright**](https://playwright.dev/) 和 [**Google Chrome**](https://www.google.com/chrome/)；全部 API 都由明确夹具应答，不访问真实服务、不下载工具。

## 一、<span id="run">执行</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

从项目根运行，标准输出为逐项 JSON；失败返回非零退出码：

```sh
node 测试/前端/Web/admin-report-evidence.cjs/admin-report-evidence.cjs
```

MFA 原链比对还读取[本轮修改前快照](../../../../项目相关报告/测试/前端/Web/20261118-管理员举报证据/before/AdminPanel.tsx)。它是当前切片的比对基线，不代表旧版本的功能验收。

浏览器执行需要已有 Web 依赖、可加载的 Playwright 模块及 Chrome。下面的路径由调用者明确指定，输出目录必须为空；默认浏览器为系统应用目录中的 Chrome，可用 `BROWSER_EXECUTABLE` 覆盖：

```sh
JOBSIM_PLAYWRIGHT_MODULE='/path/to/existing/playwright' \
JOBSIM_ADMIN_EVIDENCE_OUTPUT='/path/to/new/evidence-directory' \
node 测试/前端/Web/admin-report-evidence.cjs/browser.cjs
```

执行前确认宿主可用空间至少4GiB。执行器随机监听 `127.0.0.1` 端口，禁用项目 Vite 配置及代理；所有非 GET API 和外源请求都被拒绝并记录。只启动、关闭自身服务与浏览器，不连接9443、不提交 MFA、不修改已有实例或浏览器资料。

## 二、<span id="cases">范围与边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

当前 16 项覆盖双 UUID 过滤、跨举报／重复查询／外源／越界媒体路径、整段会话声明拒绝、失权正文和 URL 清除、尺寸／摘要／版本反例、媒体类型签名、图片20MiB与视频64MiB上限、十语键集，以及现有 MFA 提交和未知结果重试函数的逐字保留。

浏览器42项覆盖两项单独及组合过滤、显式材料加载、PNG／MP4解码、单预览切换、视频暂停及 blob 撤销、迟到响应、403、失权元数据、认证边界事件、卸载、只读权限和十语320px明暗布局。隐藏页测试明确注入 `document.hidden` 与可见性事件；不是实际手机切后台。图片复用项目品牌图标，短视频复用已有原生测试夹具。

浏览器记录随机服务地址、工具版本、请求与认证头存在性、fetch选项、截图及执行前后源码 SHA。只验证本地夹具下实际组件、认证客户端和媒体生命周期；真实登录／MFA、后端即时授权和真机浏览器仍须另验。当前结果和首轮失败原件见[浏览器续验](../../../../项目相关报告/测试/前端/Web/20261118-管理员举报证据/Resume/README.md)。

<a id="🔚" href="#前言">返回前言</a>
