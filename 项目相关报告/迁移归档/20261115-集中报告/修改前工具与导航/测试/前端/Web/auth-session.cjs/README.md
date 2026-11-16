# <span id="前言">Web 登录续期浏览器协议回归</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、执行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

在 Web 工程运行 Vite 后，设置 `JOBSIM_WEB_URL` 为其地址，运行同目录 `auth-session.cjs`。可用 `JOBSIM_PLAYWRIGHT_MODULE` 指定已安装的 Playwright，`BROWSER_EXECUTABLE` 指定 Chrome，`JOBSIM_AUTH_REPORT` 指定报告目录；不会安装依赖。默认访问本机5188端口。应在源码稳定后运行，避免 HMR 在用例中途替换模块。

## 二、范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

真实 Chrome 执行源码模块、Web Locks、localStorage／sessionStorage 与 fetch，BFF 响应由测试路由模拟。覆盖并发续期、跨标签页单次消费、未知响应重载后停止重试、初始／后继原nonce、迟到跨账号响应、私有媒体家族头、存储失败先于刷新POST、冷启动公开上下文学习及部署隔离。

本回归不证明服务端事务、真实 Cookie 保密、TLS、真实媒体权限或全产品登录 UI；这些必须另跑 HTTPS 与原生交互测试。报告不包含真实账号秘密。

<a id="🔚" href="#前言">回到首页</a>
