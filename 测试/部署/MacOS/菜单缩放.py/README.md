# <span id="前言">MacOS 菜单缩放隔离回归</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、范围与运行](#jobs-toc-heading-1)
- [二、断言与边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、范围与运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

在本目录运行 `python3 ./菜单缩放.py --yes`。仅使用已有Python标准库、zsh和fzf，不安装依赖、不联网，不调用真实部署、数据库、浏览器、隧道或用户实例。脚本从自身位置定位两份正式MacOS源码，去掉唯一main调用后载入真实选择／后台菜单／控制器函数；业务清理仅替换为本轮临时假服务。

每次创建新的源码外中文空格夹具和[中央报告子目录](../../../../项目相关报告/测试/部署/MacOS/20261118-终端菜单缩放/README.md)，记录源SHA、fzf版本、逐组结果和原始PTY字节。失败现场保留，不覆盖历史结果。传入TTY的是真实方向键、Esc、回车及Ctrl+C字节；调整内核TTY尺寸并发送SIGWINCH，覆盖连续放大／缩小和窄窗口。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、断言与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

两份自包含实现分别验证旧40%高度对照、全屏选择、Esc收起后回车重开、真实控制器Ctrl+C（有匹配／无匹配搜索两组）、前台选择与取消，共14组。后台选择和Esc不终止临时假服务，Ctrl+C沿生产控制器中断与清理路径结束假服务；用户部署不作为夹具。全屏组核对备用屏进入／退出成对，退出在清理输出之前归还原屏，默认fzf选项不能覆盖入口设置。

PTY协议与行为通过不等于Terminal图形界面拖拽目视验收。本机界面连接超时，实际Terminal拖拽仍待核；测试不更改终端全局设置、不重启既有服务。已启动控制器不热载入新源码，源码修正从下次正常启动生效。

<a id="🔚" href="#前言">回到首页</a>
