# <span id="前言">Web 业务终态与媒体围栏测试</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、范围与执行](#jobs-toc-heading-1)
- [二、证据边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、范围与执行</span><!-- jobs-toc:heading-end -->

[business-global.cjs](./business-global.cjs) 在真实Chrome／IndexedDB中加载业务API模块，响应明确由测试函数模拟，全部真实`/api/**`网络阻断。本轮9组通过，见[原始结果](../../../../项目相关报告/测试/前端/Web/执行记录/business-global-1794771094464-11141/业务终态结果.json)，不作为真实服务或App界面完整验收。

覆盖原消息／会话／过滤条件及原写入client ID绑定、已知global不能被更高版本复活、原队列不改、迟到forward及附件拒绝、共享ledger重读、Set副本、重复事实不反复失效。注入一次IDB写失败，确认先通知当前持有者失效、失败不冒充已持久化，恢复后才完成原事实重试。

先启动稳定的本机Vite，设置`JOBSIM_WEB_URL`，使用已有Node22.12+运行本目录脚本。可设`JOBSIM_PLAYWRIGHT_MODULE`及`BROWSER_EXECUTABLE`；不会安装工具。修改源码后先重启开发服务，避免HMR查询参数导致同一状态模块出现多个实例。脚本仅关闭自己创建的Chrome，60秒有界；唯一输出及14份输入原件／SHA位于中央执行记录。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、证据边界</span><!-- jobs-toc:heading-end -->

共享ledger用另一写入入口模拟同源兄弟标签页，媒体完成时实际重读IDB；不声称完成真实双设备、OS下载、断电恢复或全局所有持有者验收。App的云视图限制与组件卸载还应结合真实UI验收。首次同源模拟5组、后续8组及一次失败记录保留原义，不累加为需求完成数。

<a id="🔚" href="#前言">回到首页</a>
