# <span id="前言">Web 会话续期真实联调</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、结果与边界](#jobs-toc-heading-1)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、结果与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

6组通过，见[真实联调结果](真实联调结果.json)和[执行器](../../auth-https.cjs/README.md)。新建持久隔离部署，真实生产 Dist、Chrome、HTTPS、PG、HttpOnly Secure Host Cookie；未复用已丢失的旧测试库／CA。

双标签页只消费一次刷新；新代际使用原值加一核对。真实刷新已提交但响应丢失后，重载也不再次消费。本人证明重新认证只撤原Web family，独立native family保持有效；旧device空间的原消息编号、正文和发送准入保持且冻结，新device不接管旧队列。新会话发送已核200响应；删除浏览器登录元数据后，仅凭有效不透明Cookie也能安全学习公开上下文并续期一次。截图已查看；报告不含口令、Token或密钥。

另有11组模拟BFF浏览器协议与既有离线UI回归，证据分别记录。本报告不宣称全I、完整同步、合格收讫或全部浏览器通过。

<a id="🔚" href="#前言">回到首页</a>
