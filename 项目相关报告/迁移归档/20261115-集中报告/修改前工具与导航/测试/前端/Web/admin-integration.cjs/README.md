# <span id="前言">admin-integration.cjs</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、用途与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

真实HTTPS管理员MFA、功能开关和原操作对账。从本目录运行 `node admin-integration.cjs`；工程与报告位置按脚本位置解析，无须固定工作目录。详细参数与前置见[测试说明](../README.md)。

依赖 Node.js；浏览器测试另需已安装的 Playwright 和 Chrome，可通过 `JOBSIM_PLAYWRIGHT_MODULE`、`BROWSER_EXECUTABLE` 指定。CLI 单元入口用 `GO_BINARY` 指定 Go；联调入口还需已运行的隔离测试服务器、匹配的CA和测试配置。脚本不安装依赖、不修改系统代理或全局证书。

输出保存在上一级 `报告/`，错误打印到当前终端。联调会创建独立测试账号及业务数据并撤销测试会话；只在自有测试环境运行，不连接生产数据。管理员测试需专用私密fixture，不输出凭据。

<a id="🔚" href="#前言">回到首页</a>
