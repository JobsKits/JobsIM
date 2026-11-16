# <span id="前言">Web 登录续期浏览器协议回归</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行](#jobs-toc-heading-1)
- [二、范围](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

在 Web 工程运行 Vite 后，设置 `JOBSIM_WEB_URL` 为其地址，运行同目录 `auth-session.cjs`。可用 `JOBSIM_PLAYWRIGHT_MODULE` 指定已安装的 Playwright，`BROWSER_EXECUTABLE` 指定 Chrome，`JOBSIM_AUTH_REPORT` 指定报告目录；不会安装依赖。默认访问本机5188端口。应在源码稳定后运行，避免 HMR 在用例中途替换模块。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

真实 Chrome 执行源码模块、Web Locks、IndexedDB、localStorage／sessionStorage 与 fetch，BFF 响应由测试路由模拟。当前 16 组覆盖并发续期、跨标签页单次消费、未知响应重载后停止重试、初始／后继原 nonce、迟到跨账号响应、私有媒体家族头、存储失败先于刷新 POST、冷启动公开上下文学习、部署隔离、旧设备队列冻结、原证明保护、完整认证生命周期锁及能力关闭后的请求拒绝。

本回归不证明服务端事务、真实 Cookie 保密、TLS、真实媒体权限或全产品登录 UI；这些必须另跑 HTTPS 与原生交互测试。报告不包含真实账号秘密。

最近一次专项结果见[认证输入保护报告](../../../../项目相关报告/测试/前端/Web/20261115-认证输入保护/README.md)。组数按脚本的 `checks` 数组计算，不代表功能覆盖率。


默认输出根据脚本位置写入工程根 `项目相关报告/测试/前端/Web/执行记录/本脚本-时间-PID/`；`JOBSIM_AUTH_REPORT` 继续优先，用它为当前范围选择新中央子项。报告迁移只改默认路径，未重跑浏览器或 HTTPS。

<a id="🔚" href="#前言">回到首页</a>
