# <span id="前言">JobsIM Web 测试</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、测试入口 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 入口 | 验证内容 | 依赖 |
| --- | --- | --- |
| [browser.cjs](./browser.cjs/README.md) | 演示发送、主题、十语言、字号、桌面／移动溢出、页面异常 | 已运行的 Web、Playwright、Chrome |
| [integration.cjs](./integration.cjs/README.md) | 真实HTTPS注册、Cookie、好友、双向消息、原ID重试、头像持久操作／历史头像不回滚、举报／群治理、原发送资格与本人关系治理 | 已运行的Go网关、两套PostgreSQL、CA／公开服务器证书 |
| [admin-integration.cjs](./admin-integration.cjs/README.md) | 管理员MFA、目标／版本绑定、写入丢响应原授权对账、开关恢复 | 独立测试服务、CA、私有管理员fixture；不要连接生产 |
| [auth-session.cjs](./auth-session.cjs/README.md) | 认证续期、跨标签页、原证明与退休链门禁；明确模拟 BFF | Vite、Chrome、Playwright |
| [auth-https.cjs](./auth-https.cjs/README.md) | 真实 BFF Cookie／原家族单次刷新／本人证明关联退休 | 生产 Dist、独立刷新环境及私密 seed |
| [sync-protocol.cjs](./sync-protocol.cjs/README.md) | 原始 envelope 摘要、完整页、版本与精确连续位置反例 | 既有 TypeScript CLI、Node WebCrypto；不请求 API |
| [sync-store.cjs](./sync-store.cjs/README.md) | 真实 IndexedDB／WebLocks 的暂存、事务、生命周期／guard、原请求恢复 | Vite、Chrome、Playwright；明确模拟 API |
| [sync-view.cjs](./sync-view.cjs/README.md) | 普通／global 投影、未读、原队列保护和只读 proof 的事务再核 | 真实浏览器模块与 IndexedDB；不请求真实 API |
| [outbox-proof.cjs](./outbox-proof.cjs/README.md) | 原完整请求的队列CAS、opaque client ID及完整DTO反例 | 真实IndexedDB，proof明确注入；全部API阻断 |
| [global-facts.cjs](./global-facts.cjs/README.md) | 跨设备／代际／隔离持有者的终态ledger、迟到保存围栏 | 真实IndexedDB，投影及上下文明示模拟 |
| [business-global.cjs](./business-global.cjs/README.md) | 业务DTO原路由绑定、迟到媒体、存储失败后的终态重试 | 真实浏览器模块／IDB，业务API明示模拟 |
| [sync-https.cjs](./sync-https.cjs/README.md) | 生产UI多页同步、未知发送查单、辅助503不阻global、空存储／撤权／历史global清理 | 独立同步环境／NATS、生产Dist、私密seed；辅助503明确注入 |
| [self-state.cjs](./self-state.cjs/README.md) | 本人原动作、版本／成员代际、草稿恢复与采纳CAS，实际组件和移动布局 | 真实Chrome／IDB／WebLocks，HTTP明确注入 |
| [self-state-https.cjs](./self-state-https.cjs/README.md) | 生产BFF／S2、可见／遮挡阅读、视图恢复、云草稿原单冷启、两浏览器及旧编辑清理 | 显式受管E2／生产Dist／指定CA，本轮新账号与退出核验 |
| [owned-web-runtime.cjs](./owned-web-runtime.cjs/README.md) | 固定后端同实例切换已构建Web目录，精确PID短启与失败恢复 | 显式允许自有App重启及全部客户端已关闭；不启停PG／NATS |

```sh
JOBSIM_WEB_URL=http://127.0.0.1:5178 node browser.cjs/browser.cjs
JOBSIM_CA_FILE=/private/ca.pem JOBSIM_TEST_CERT=/private/server.pem node integration.cjs/integration.cjs
```

测试从自身位置定位工程，默认按时间／进程号写入[中央执行记录](../../../项目相关报告/测试/前端/Web/执行记录/)，不把启动工作目录当路径基准。可用 `JOBSIM_PLAYWRIGHT_MODULE` 指定用户级 Playwright 模块位置，`BROWSER_EXECUTABLE` 指定 Chrome；真实接口入口按各自 README 提供独立服务／证书或私密 seed。不会安装依赖或修改系统工具。

## 二、信任与报告 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

真实联调的原生 HTTPS 对端校验指定 CA 和 SAN；独立浏览器仅信任测试提供的准确公开证书 SPKI，不安装 Mac 全局证书、不关闭应用的证书校验。生产使用可信证书或运维配置的私有 CA。

报告含截图及结构化结论，不保存密码、Token 或私钥。`browser-result.json` 只说明对应浏览器断言；`integration-result.json` 只说明真实接口链路。完整产品仍按[需求验收表](../../../需求/IM功能验收表.md/IM功能验收表.md)逐项验收。

既有真实回归覆盖收藏／搜索50＋1分页及原消息撤回重鉴权、结构化成员选择原body丢响应重试、未读提及编辑去重／block后重新投影、公告原请求刷新核对／个人互动不失效／内容编辑失效和清除。历史截图见[查找](../../../项目相关报告/测试/前端/Web/Web-Lookup-HTTPS.png)和[公告](../../../项目相关报告/测试/前端/Web/Web-Announcement-HTTPS.png)，对应[当时结果](../../../项目相关报告/测试/前端/Web/integration-result.json)不追认新构建；通知测试只证明浏览器协议，不宣称 OS 实际送达。

[普通转发真实联调](./forward.cjs/README.md)已验证两类未知结果的原ID／proof、源资格撤回后的无正文缓存和本人撤回入口；纯规则浏览器还核对错误附件原点／路径不回退源媒体ID。最终服务修订的复跑另留结果，不外推完整I。

各组数只描述对应源码、包和环境的定向断言；Node、模拟 API、真实 HTTPS、原生模拟器和真机分别记录，不相加为需求完成数。

[独立草稿恢复专项](./draft-recovery.cjs/README.md)新增10组真实Chrome／React／IndexedDB／Web Locks注入检查，并完成TypeScript及私有生产构建；覆盖退群或未列出会话及只有retired原操作的候选、仅精确GET显示、复制／清空所见版本、原单404零重投、其它输入保持、能力撤回和旧family迟到／排队取消。全部`/api`路由阻断、Vite无API代理、剪贴板为注入接收端；无E2／真实后端／OS剪贴板证明。新候选及独立原输入见[本轮报告](../../../项目相关报告/测试/前端/Web/20261116-独立草稿恢复/README.md)，旧D5的11真／26本地不追认该增量。

<a id="🔚" href="#前言">回到首页</a>
