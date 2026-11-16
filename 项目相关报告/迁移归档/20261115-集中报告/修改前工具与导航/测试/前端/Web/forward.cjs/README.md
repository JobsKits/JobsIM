# <span id="前言">Web 普通转发真实联调</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、范围

`forward.cjs`使用已构建的真实Web Dist、Chrome与匹配CA／证书，调用隔离测试域真实HTTPS／PostgreSQL。运行前设置`JOBSIM_SERVER_URL`、`JOBSIM_CA_FILE`和`JOBSIM_TEST_CERT`，可用`JOBSIM_PLAYWRIGHT_MODULE`／`BROWSER_EXECUTABLE`指定已有浏览器依赖。不安装依赖、不改系统信任／代理。

该隔离域未配置WebDir，Playwright只为非API静态请求提供工程实际Dist文件；API均到真实服务器，仍核对TLS／主机名及明确证书公钥。静态部署在原HTTPS主域既有全链另验，不把本用例说成隔离域部署通过。

已验证：当前源内容版本与目标资格，纯引用原意图持久化，已提交丢响应后刷新只查原结果而不重复POST，未到达时先查404且明确沿同ID／body／proof重投一次；慢POST响应不覆盖已同步的新失效版本，503／200临时占位后旧同version历史不能恢复正文，只有新的精确GET可恢复；本人转发隐藏编辑保留撤回；源群退出后即使无新消息事件也清界面和持久缓存正文／附件／proof。[结果](../../../../项目相关报告/测试/前端/Web/forward-result.json)与[实际截图](../../../../项目相关报告/测试/前端/Web/Web-Forward-Source-Unavailable.png)可核对。

故障只用于测试：一次真实提交后的丢响应、一次阻断POST未到达，以及暂时保留目标历史页以便核对独立Outbox，以及客户端投影读层503／200占位和真实旧历史回放。不把这些故障注入当作生产宕机。当前未覆盖真机浏览器、全部媒体交互、强保护或全I；不相加为243项通过。

<a id="🔚" href="#前言">回到首页</a>
