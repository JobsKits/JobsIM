# <span id="前言">【Linux】部署JobsIM.sh</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、运行](#jobs-toc-heading-1)
- [二、安全与记录](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

```bash
/bin/bash './【Linux】部署JobsIM.sh'
/bin/bash './【Linux】部署JobsIM.sh' check
```

默认进入确认＋fzf部署模式菜单；显式run可配--mode local或server。路径、工具与全部参数见[本平台说明](../../README.md)。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、安全与记录 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

生命周期、资源原点、UID／设备／inode与进程身份直接内嵌；程序、双PG、NATS、TLS和媒体只限本实例。受保护journal／秘密／实例身份文件、共享工具、已有外置归档与恢复钥保留；未知／并发变化不删，部分完成不说全量恢复。执行前阅读[验证与恢复边界](../../README.md#前言)。
<a id="🔚" href="#前言">返回前言</a>
