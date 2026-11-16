# <span id="前言">统计项目.mjs</span>

![JobsIM](../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、用途与运行](#jobs-toc-heading-1)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、用途与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

从脚本位置解析工程根目录，统计自有业务代码、外置测试及配置／脚本的物理行数（含空白和注释）。执行 `node 统计项目.mjs`，只读源码并更新工程根 `项目相关报告/项目统计/报告.json`；不安装依赖，不修改源码或系统设置。需要现有 Node.js。

范围为 `代码/`、`测试/`、`部署/`、`工具/`；跳过第三方、符号链接、构建产物、资源、锁文件和整个 `项目相关报告/`、测试报告。名单、逐文件分类及统计时间写入报告。新增语言、生成目录或改动分类时应先更新脚本，再更新根README表格及饼图；这是源码规模统计，不代表功能验收通过率。

Python入口按脚本计入（测试目录归测试，其余归配置／脚本），不漏掉真实接口驱动；`InstrumentationAssets`内按原件复制的Room schema等测试资源不计代码行。

<a id="🔚" href="#前言">回到首页</a>
