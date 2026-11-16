# <span id="前言">Web 同步耐久存储测试</span>

![Jobs出品，必属精品](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、范围与运行](#jobs-toc-heading-1)
- [二、验证边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、范围与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[sync-store.cjs](./sync-store.cjs) 使用已有 Node、Playwright 和 Chrome，动态导入已启动 Vite 的真实源码，在真实 IndexedDB／WebLocks 上测试事务、暂存、冷重载和旧响应。所有 `/api/` 网络请求明确阻断，业务 API 由测试函数注入；不访问真实账号、不安装依赖、不改系统证书／代理，不把本轮记为后端 HTTPS 联调通过。

设置 `JOBSIM_WEB_URL` 为本地 Vite 地址；`JOBSIM_PLAYWRIGHT_MODULE` 指向已有模块，`BROWSER_EXECUTABLE` 可指定现有浏览器。运行 `node ./sync-store.cjs`。源码须稳定，执行中 SHA 变化会失败。

默认报告按脚本位置解析到工程根 `项目相关报告/测试/前端/Web/执行记录/sync-store-时间-PID/`；`JOBSIM_SYNC_REPORT` 可指定新目录，已存在即拒绝覆盖。失败保留部分结果及非零退出码，仅关闭本脚本创建的浏览器。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

12 组覆盖 single flight、完整持久页、晚 context、新旧 view 围栏、unknown 保游标／隔离历史、global 清重复 holder、原 snapshot 请求未知经冷重载保持 ID、部分暂存恢复重取全页、能力门禁、真实 IDB 回滚、晚撤权 CAS 及双 tab WebLocks。实际结果由各轮原始 JSON 记录；未执行不记通过。

没有真实后端／NATS、物理断电、qualified ACK、原生设备或全 I 验收。只保存无秘密夹具和源码摘要，不保存认证凭据。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
