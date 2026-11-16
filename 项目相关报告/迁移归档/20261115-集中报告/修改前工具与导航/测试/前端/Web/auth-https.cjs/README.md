# <span id="前言">Web 会话续期真实 HTTPS 回归</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、执行与影响 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

先构建 Web Dist，并由隔离网关同源提供。设置 `JOBSIM_AUTH_TEST_SEED` 为明确标记 `refreshTestRealm`／`persistentData` 的私密 seed，运行同目录 `auth-https.cjs`。Playwright／Chrome／报告目录变量同[浏览器协议回归](../auth-session.cjs/README.md)。

此测试会创建两个隔离账号、好友关系、群与消息，仅用 SQL 调整自己新建 Web family 的访问期限；不改他人会话、不修改限流、不重启服务。测试密钥及数据库连接由工程外 seed 读取，不写入报告。Node 按 CA 验证 TLS／主机名，浏览器只信任本轮公开证书 SPKI；不导入系统信任，不允许通配忽略所有证书错误。

## 二、验证范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

生产 Dist、真实 Cookie、真实 PG 与真实 UI 共同覆盖双标签页一次续期、提交后丢响应与重载、本人证明关联重新认证、仅退休原family、旧 Outbox 原body／原准入保留冻结、新会话正常发送及仅剩不透明 Cookie 时的公开上下文学习。

脚本只证明这些定向链路，不代替完整 IM、全部浏览器、真机或完整同步验收。模拟BFF的11组测试独立记录，不相加为产品覆盖率。

<a id="🔚" href="#前言">回到首页</a>
