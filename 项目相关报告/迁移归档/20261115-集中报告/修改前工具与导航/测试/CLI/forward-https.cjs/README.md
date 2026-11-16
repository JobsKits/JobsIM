# <span id="前言">CLI 转发真实HTTPS入口</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

[toc]

## 用途

在自有隔离后端启用forwarding后运行`node forward-https.cjs`，设置`JOBSIM_CLI_BINARY`、`JOBSIM_CA_FILE`、`JOBSIM_SERVER_URL`。只使用已有受管CLI与匹配CA，验证主机名，不安装工具或改变全局证书。测试创建随机账号、真实好友／会话／转发，退出撤销会话；凭据仅进程内与标准输入，不写报告。

输出`../报告/ForwardHTTPS结果.json`，覆盖版本重选、纯引用文本／文件、草稿保留、固定消息附件代理精确字节与不覆盖文件、源编辑失效／本人撤回及重开占位缓存。SQLite故障／双类未知另由[事务测试](../README.md)验证；结果不代表完整I。

<a id="🔚" href="#前言">回到首页</a>
