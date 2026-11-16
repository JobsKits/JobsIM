# <span id="前言">browser.cjs</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、用途与运行](#jobs-toc-heading-1)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、用途与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

浏览器演示、十语言、主题、响应布局与持久队列。从本目录运行 `node browser.cjs`；工程与报告位置按脚本位置解析，无须固定工作目录。详细参数与前置见[测试说明](../README.md)。

依赖 Node.js；浏览器测试另需已安装的 Playwright 和 Chrome，可通过 `JOBSIM_PLAYWRIGHT_MODULE`、`BROWSER_EXECUTABLE` 指定。CLI 单元入口用 `GO_BINARY` 指定 Go；联调入口还需已运行的隔离测试服务器、匹配的CA和测试配置。脚本不安装依赖、不修改系统代理或全局证书。

输出保存在工程根 `项目相关报告/测试/前端/Web/`，错误打印到当前终端。联调会创建独立测试账号及业务数据并撤销测试会话；只在自有测试环境运行，不连接生产数据。管理员测试需专用私密fixture，不输出凭据。

默认输出按脚本自身位置解析到工程根 `项目相关报告/测试/前端/Web/`，不依赖旧报告软链接。


默认报告现直接写入工程根 `项目相关报告/测试/前端/Web/执行记录/本轮唯一目录/`，Node 使用时间与 PID，Python 使用 UUID，避免覆盖迁入的历史固定文件。原有显式报告参数／环境变量优先；请选择新目录。本轮只核对独立执行目录表达式和脚本语法，没有重新运行业务测试或真实接口。

<a id="🔚" href="#前言">回到首页</a>
