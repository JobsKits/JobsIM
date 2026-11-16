# <span id="前言">Web global 终态持久化测试</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、作用与范围](#jobs-toc-heading-1)
- [二、运行](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、作用与范围</span><!-- jobs-toc:heading-end -->

这是6组真实Chrome IndexedDB模块测试，[本轮6组通过](../../../../项目相关报告/测试/前端/Web/执行记录/global-facts-1794770422806-9398/结果.json)。测试注入本地投影与认证上下文，全部`/api/**`请求阻断，不代表真实HTTPS、服务端global或原生平台验收。覆盖跨device／epoch／archive／retained清理、其他scope／未知旧hash不动、原Outbox／草稿不改、global不创建sync责任、旧保存不复活、原始pending气泡、只读完整ledger、持久guard与已验证global投影同cursor事务。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、运行</span><!-- jobs-toc:heading-end -->

先协调宿主磁盘和浏览器资源，复用已有 Vite 服务，不启动后端或构建新生产包。仓库根示例：

```sh
"$HOME/.cache/codex-runtimes/codex-primary-runtime/dependencies/node/bin/node" 测试/前端/Web/global-facts.cjs/global-facts.cjs
```

需要 Node22.12+、已安装的 Playwright 和 Chrome；优先使用当前独立 Node，避免本机失效的 Brew 链。`JOBSIM_WEB_ORIGIN` 默认 `http://127.0.0.1:5189`，仅接受本机地址；`PLAYWRIGHT_MODULE`／`JOBSIM_CHROME` 可指定既有工具路径，不自动下载安装。`JOBSIM_GLOBAL_REPORT` 可指定新的空报告目录，默认使用 `项目相关报告/测试/前端/Web/执行记录/global-facts-<time>-<pid>`，拒绝覆盖已有报告。

运行至多60秒，错误退出非零；关闭测试自己创建的 Chrome，不操作其它窗口。每轮封存7个模块与本脚本原件／SHA和逐组结果；输入不含凭据、服务器seed或消息秘密。Node语法检查、源码编译与6组浏览器执行分别记证据，不套用旧8／12等结果。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
