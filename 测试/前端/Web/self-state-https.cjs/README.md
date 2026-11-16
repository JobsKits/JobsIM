# Web 本人状态真实 HTTPS 专项

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

入口 `node self-state-https.cjs`。必须显式提供 `JOBSIM_SELF_TEST_ENV`，指向已受管、客户端窗口已释放的独立恢复测试实例 manifest；`JOBSIM_PLAYWRIGHT_MODULE` 指向已有 Playwright 模块。脚本不下载依赖、不重启服务、不修改共享系统信任或数据库时钟。

实例必须是当前指定的 `54443` 隔离域与固定 DEA37 后端，manifest 包含已构建 Web 目录。Node 使用指定 CA 正常校验证书；Chrome 仅放行本轮指定服务器公钥 SPKI，不使用全局 TLS 忽略。先核实例、epoch、功能和磁盘空间，再为本轮新建账号／会话；私密夹具只存工程外 `Integration/.../Secrets`，报告不保存密码、AT／RT、Cookie 或 da1／dr1。

覆盖真实 BFF、S2 入库、视口内观察／未见空洞、独立手动未读、归档隐藏恢复、云／本地草稿分离、严格 HTTPS 受控丢响应后原单恢复、两个真实浏览器 context、明确采纳、云清空及字体。受控丢响应通过使用原完整请求的严格 CA Node HTTPS 代理提交，再中断浏览器响应；不把 200 单独当保存成功。

各轮结果、原始失败和截图写到 `项目相关报告/测试/前端/Web/20261116-S2本人状态/HTTPS-*`。结束时分别确认浏览器原设备已退出、控制会话 logout 与原 AT 401。失败须先按该轮私密夹具补齐清理证据，禁止修改原失败 JSON、复用旧 actor 或静默重试成新成功。

[实施与证据入口](../../../../项目相关报告/测试/前端/Web/20261116-S2本人状态/README.md)。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
