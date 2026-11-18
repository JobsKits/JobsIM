# <span id="前言">管理员举报证据浏览器续验</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、轮次与输入](#rounds)
- [二、浏览器实际结果](#coverage)
- [三、验证边界](#boundary)

</details>
<!-- jobs-toc:end -->

---

本轮独立本机服务加载实际 `AdminPanel`、详情、认证客户端与媒体组件；API 全部为明确夹具。最终42通过／0失败，没有生产源码修改；首轮失败和截图保持原件。当前汇总见 [summary.json](./summary.json)。

## 一、<span id="rounds">轮次与输入</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

> 两轮独立保存，没有用后轮覆盖失败。

| 轮次 | 结果 | 解释与原件 |
| --- | --- | --- |
| browser-1 | 40通过／1失败 | 过滤后的行正确显示原因及目标ID，测试却对包含二者的 span 要求纯原因精确匹配；[失败截图](./browser-1/failure-1.png)、[逐项结果](./browser-1/summary.json)保留 |
| browser-2 | 42通过／0失败 | 改为核对实际举报行并补“只按目标用户过滤”；清除夹具残留通知后截图，[逐项结果](./browser-2/summary.json)、[请求](./browser-2/requests.json)、[fetch选项](./browser-2/fetch-options.json) |

使用既有 [**Playwright**](https://playwright.dev/) 1.65.1、[**Google Chrome**](https://www.google.com/chrome/) 157.0.8103.79、[**Node.js**](https://nodejs.org) 26.13.1，未下载浏览器或工具。随机监听 loopback，项目代理配置关闭；[环境原件](./browser-2/environment.json)记录实际端口。执行前可用空间约16GiB，高于4GiB门禁。

每轮[前](./browser-2/inputs-before.json)／[后](./browser-2/inputs-after.json)12项输入一致，包括9个生产文件、执行器、品牌PNG与已有短MP4夹具。8项初轮静态源码输入与本轮仍逐字节一致，原类型检查及16项协议测试保留各自原件；本轮也观察到协议16通过／0失败。最终文档与输入对账另见 [verification.json](./verification.json)。

## 二、<span id="coverage">浏览器实际结果</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

两项可选 UUID 支持单独及组合过滤，非法值不发列表请求，清除后恢复全部夹具举报。详情只显示明确选入材料，不自动下载媒体；只读管理员可看，管理仍进入原 MFA 表单但没有提交。

PNG实际解码、短MP4实际加载播放；切换前同步退休旧预览，最多一个活动 blob。视频被暂停并释放 src；切换、关闭、卸载、迟到响应、认证边界及已知403均核到撤销或不产生迟到 blob。`available:false` 不保旧正文和媒体URL。二进制请求走现有客户端、带夹具 cookie及家族头，并使用 `no-store`、`redirect:error`、`credentials:include`；没有外源或非GET请求，没有未捕获页面错误。

320×568桌面视口中，十种语言分别核到明暗主题无横向溢出，过滤输入与详情按钮可滚动到达；标题取当前语言。已查看[英文亮色](./browser-2/admin-320-light-en.png)、[藏文暗色](./browser-2/admin-320-dark-bo.png)及[桌面暗色预览](./browser-2/admin-desktop-dark-evidence.png)截图；其余英语／藏文明暗截图保存在同轮目录。

## 三、<span id="boundary">验证边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

认证状态和所有 HTTP 响应均为测试夹具；403及认证边界证明客户端清空路径，不证明真实管理员权限已撤销。隐藏页测试显式注入 `document.hidden` 与可见性事件；窄屏是桌面 Chrome 视口，不能外推真机、系统键盘或切后台行为。应用级“跟随系统”选择不在此独立入口内。

未连接9443、未启停已有实例、未创建真实举报或执行 MFA／管理写操作。真实后端的选入关联、版本、删除 floor、每次读取授权和完整20／64MiB流响应仍待联调。执行器只关闭本轮服务和浏览器；源码、共享工具、用户缓存和数据均保留。

复现入口及环境变量见[测试说明](../../../../../../测试/前端/Web/admin-report-evidence.cjs/README.md)。

<a id="🔚" href="#前言">返回前言</a>
