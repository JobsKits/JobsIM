# <span id="前言">JobsIM Web 测试</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、测试入口](#jobs-toc-heading-1)
- [二、信任与报告](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、测试入口 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 入口 | 验证内容 | 依赖 |
| --- | --- | --- |
| [browser.cjs](browser.cjs/README.md) | 演示发送、主题、十语言、字号、桌面／移动溢出、页面异常 | 已运行的 Web、Playwright、Chrome |
| [integration.cjs](integration.cjs/README.md) | 真实HTTPS注册、Cookie、好友、双向消息、原ID重试、头像持久操作／历史头像不回滚、举报／群治理、原发送资格与本人关系治理 | 已运行的Go网关、两套PostgreSQL、CA／公开服务器证书 |
| [admin-integration.cjs](admin-integration.cjs/README.md) | 管理员MFA、目标／版本绑定、写入丢响应原授权对账、开关恢复 | 独立测试服务、CA、私有管理员fixture；不要连接生产 |

```sh
JOBSIM_WEB_URL=http://127.0.0.1:5178 node browser.cjs/browser.cjs
JOBSIM_CA_FILE=/private/ca.pem JOBSIM_TEST_CERT=/private/server.pem node integration.cjs/integration.cjs
```

测试从自身位置生成 `报告/`，不依赖工程内测试文件。可用 `JOBSIM_PLAYWRIGHT_MODULE` 指定用户级 Playwright 模块位置，`BROWSER_EXECUTABLE` 指定 Chrome，`JOBSIM_SERVER_URL` 指定真实 HTTPS 服务。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、信任与报告 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

真实联调的原生 HTTPS 对端校验指定 CA 和 SAN；独立浏览器仅信任测试提供的准确公开证书 SPKI，不安装 Mac 全局证书、不关闭应用的证书校验。生产使用可信证书或运维配置的私有 CA。

报告含截图及结构化结论，不保存密码、Token 或私钥。`browser-result.json` 只说明对应浏览器断言；`integration-result.json` 只说明真实接口链路。完整产品仍按[需求验收表](../../../需求/IM功能验收表.md/IM功能验收表.md)逐项验收。

新增真实回归覆盖收藏／搜索50＋1分页及原消息撤回重鉴权、结构化成员选择原body丢响应重试、未读提及编辑去重／block后重新投影、公告原请求刷新核对／个人互动不失效／内容编辑失效和清除。截图见`报告/Web-Lookup-HTTPS.png`和`报告/Web-Announcement-HTTPS.png`，结果详见`integration-result.json`；通知测试只证明浏览器协议，不宣称OS实际送达。

<a id="🔚" href="#前言">回到首页</a>

[普通转发真实联调](forward.cjs/README.md)已验证两类未知结果的原ID／proof、源资格撤回后的无正文缓存和本人撤回入口；纯规则浏览器还核对错误附件原点／路径不回退源媒体ID。最终服务修订的复跑另留结果，不外推完整I。
