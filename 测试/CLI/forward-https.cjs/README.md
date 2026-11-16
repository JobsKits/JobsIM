# <span id="前言">CLI 转发真实HTTPS入口</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [用途](#jobs-toc-heading-1)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">用途</span><!-- jobs-toc:heading-end -->

在自有隔离后端启用forwarding后运行`node forward-https.cjs`，设置`JOBSIM_CLI_BINARY`、`JOBSIM_CA_FILE`、`JOBSIM_SERVER_URL`。只使用已有受管CLI与匹配CA，验证主机名，不安装工具或改变全局证书。测试创建随机账号、真实好友／会话／转发，退出撤销会话；凭据仅进程内与标准输入，不写报告。

输出`../../../项目相关报告/测试/CLI/ForwardHTTPS结果.json`，覆盖版本重选、纯引用文本／文件、草稿保留、固定消息附件代理精确字节与不覆盖文件、源编辑失效／本人撤回及重开占位缓存。SQLite故障／双类未知另由[事务测试](../README.md)验证；结果不代表完整I。


默认报告现直接写入工程根 `项目相关报告/测试/CLI/执行记录/本轮唯一目录/`，Node 使用时间与 PID，Python 使用 UUID，避免覆盖迁入的历史固定文件。原有显式报告参数／环境变量优先；请选择新目录。本轮只核对独立执行目录表达式和脚本语法，没有重新运行业务测试或真实接口。

<a id="🔚" href="#前言">回到首页</a>
