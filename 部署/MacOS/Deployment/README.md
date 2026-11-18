# <span id="前言">JobsIM MacOS 部署入口</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、部署](#jobs-toc-heading-1)
- [二、检查与配对入口](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、部署 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

日常双击[部署快捷入口](../【MacOS】双击部署（快捷入口）.command)，它原样转交本目录的[原生实现](./【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command)，不依赖启动工作目录。先确认自述，再用fzf选择本地部署或服务器部署；取消模式或公开确认不会停止既有业务。

本地模式默认仅本机，不要求cloudflared；明确选择临时穿透后使用实例CA严格校验origin，取得实际Quick网址后绑定后端PublicURL，通过公网健康再显示访问菜单。服务器模式不建穿透，必须明确HTTPS origin、配套SAN证书、私钥及信任链；解析、反向代理及防火墙由部署方配置。穿透失败及晚退撤销旧地址/二维码并严格恢复本机Origin，本机服务保持，菜单可重试、诊断或停止。双模式都复用JobsIM原生独立双PG、NATS、身份及API，未验证生产强保护。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、检查与配对入口 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

`--check`、`--plan --mode local`只读，不联网、不探测工具或创建状态。正常运行执行官方版本及功能检查，来源/完整性不明时停止；不修改全局工具、系统信任或源码锁文件。资源原点及退出控制直接内嵌，实现和参数见[平台说明](../README.md)。

停止保留、备份和真实清理由[配对反部署](../Undeployment/README.md)处理，不能用停止代替反部署；独立journal和匹配实例钥默认保留。

<a id="🔚" href="#前言">回到首页</a>
