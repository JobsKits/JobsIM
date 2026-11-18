# <span id="前言">JobsIM MacOS 反部署入口</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、选择与确认](#jobs-toc-heading-1)
- [二、归属与保留](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、选择与确认 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

双击[反部署快捷入口](../【MacOS】双击反部署（快捷入口）.command)，由[独立自包含实现](./【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command)执行。首层默认仅停止并保留，可选择进入清理；清理层默认数据库数据备份，也可显式选择数据清零。最终显示确切范围并读取回车，非空或EOF取消；`--yes`不会授权这一步。

备份必须在state及源码之外加密保存，逐域dump检查、解密目录验真及来源摘要均通过才进入删除。任一步失败立即传播，源资料保留；不能把部分归档记为完整备份。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、归属与保留 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

只清原点明确为本实例新建、设备/inode及文件摘要仍匹配的资源。链接、未知顶层、错用户、对象替换、目录新增或确认后内容变化阻断；逐项unlink/rmdir，不整树递归删除。中断按原断点续跑，不新建业务库或扩大原计划。

`JournalDatabase`、`Application/instance.json`及其父目录、`Secrets`、`TLS`、原点审计、外置备份及恢复钥默认保留，销毁独立恢复事实另行授权；清理结果因此明确为partial。复用/共享工具、源码、无归属依据的旧部署和系统信任不处理。参数、资源和恢复限制见[平台完整说明](../README.md)。

<a id="🔚" href="#前言">回到首页</a>
