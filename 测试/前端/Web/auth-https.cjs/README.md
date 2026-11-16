# <span id="前言">Web 会话续期真实 HTTPS 回归</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行与影响](#jobs-toc-heading-1)
- [二、验证范围](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行与影响 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

先构建 Web Dist，并由隔离网关同源提供。设置 `JOBSIM_AUTH_TEST_SEED` 为明确标记 `refreshTestRealm`／`persistentData` 的私密 seed，运行同目录 `auth-https.cjs`。Playwright／Chrome／报告目录变量同[浏览器协议回归](../auth-session.cjs/README.md)。

此测试会创建两个隔离账号、好友关系、群与消息，仅用 SQL 调整自己新建 Web family 的访问期限；不改他人会话、不修改限流、不重启服务。测试密钥及数据库连接由工程外 seed 读取，不写入报告。Node 按 CA 验证 TLS／主机名，浏览器只信任本轮公开证书 SPKI；不导入系统信任，不允许通配忽略所有证书错误。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、验证范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

当前 8 组以生产 Dist、真实 Cookie、真实 PG 与真实 UI 覆盖双标签页一次续期、提交后丢响应与重载、本人证明关联重新认证、仅退休原 family、旧 Outbox 原 body／原准入保留冻结、新会话正常发送、仅剩不透明 Cookie 时的公开上下文学习、关闭排队表单后禁止提交凭据，以及原登录响应丢失后的证明保护与显式关联重新验证。

脚本只证明这些定向链路，不代替完整 IM、全部浏览器、真机或完整同步验收。模拟 BFF 的 16 组测试独立记录，不相加为产品覆盖率。最近一次结果见[认证输入保护报告](../../../../项目相关报告/测试/前端/Web/20261115-认证输入保护/README.md)。


默认输出根据脚本位置写入工程根 `项目相关报告/测试/前端/Web/执行记录/本脚本-时间-PID/`；`JOBSIM_AUTH_REPORT` 继续优先，用它为当前范围选择新中央子项。报告迁移只改默认路径，未重跑浏览器或 HTTPS。

<a id="🔚" href="#前言">回到首页</a>
