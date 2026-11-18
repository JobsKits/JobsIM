# <span id="前言">JobsIM 终端二维码验证</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前显示与入口](#jobs-toc-heading-1)
- [二、实际验证](#jobs-toc-heading-2)
- [三、根因、失败与收尾](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、当前显示与入口 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

足尺寸可交互终端直接显示黑白二维码，Mac扫码停留后回车进入网页菜单，避免fzf立即盖住码。公共编码器采用背景色40／47与双空格绘制整模块，不依赖块字符填满字体单元。按当前TTY列／行实测空间，四模块静区保留；宽度需2N+2列、高度需N+6行，N为含静区的模块边长。

宽窄或颜色条件无法可靠绘码时，Mac自动打开本轮本地PNG；Linux在图形会话中使用xdg-open，Windows仅在本地交互桌面请求图片。NO_COLOR即使为空也不输出ANSI，同时本地黑白PNG提供可靠展示；无ANSI字符图本身不冒称通用手机扫码通过。无法打开图片时明确未完成并保留URL、PNG和HTML，不假报已显示。

仅本机URL仍标明“手机不可用”，扫码不会改变地址的网络可达性，也不自动建立公网穿透。非TTY不等待输入、不在Mac弹原生窗口。既有运行终端不会热替换已加载脚本，新逻辑在重新运行[部署快捷入口](../../../../部署/MacOS/【MacOS】双击部署（快捷入口）.command)时生效，不为展示二维码擅自重启用户实例。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、实际验证 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 检查 | 结果与原件 |
| --- | --- |
| Mac真实PTY | [18通过／0失败](./最终背景方格PTY/summary.json)，含80×50、80×30、32列、12行、NO_COLOR空值、TERM=dumb、双地址、实际fzf回车停留和非TTY |
| 终端成像 | 12张使用真实Menlo的深浅背景成像经CoreImage逐字解码；[深色样本](./最终背景方格PTY/0-80列50行/终端字体-dark.png)已查看。此为PTY字节与原生字体／ANSI单元成像，不是用户现有Terminal窗口截图 |
| Linux调用与语法 | [8个QR函数在Mac Bash隔离实际调用、10份Mac／Linux语法](./跨平台调用与语法.json)通过；未运行Linux原生终端／桌面 |
| Windows调用与解析 | [2个正式函数在Mac PowerShell隔离实调用、2份解析](./Windows二维码合同.json)通过；不等于Windows原生显示 |
| 基础离线码 | [2个PNG精确解码、2类凭据URL拒绝](./基本二维码.json)；未改地址保护合同 |

当前源码与证据SHA见[摘要](./summary.json)及[输入](./当前源码SHA256.json)。实际测试入口和复跑说明在[独立二维码测试](../../../../测试/部署/二维码/终端二维码.py/README.md)。物理手机相机扫码、真实公网连通、Linux／Windows原生窗口显示尚未验收。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、根因、失败与收尾 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

原Mac／Linux调用没有开启终端绘制，Windows固定宽度0；COLUMNS也不足以代表当前TTY尺寸。首次块字符方案在真实字体成像中出现finder裂缝和长URL解码失败，原红／中间绿色原件均保留；最终改用可靠背景格，不降低解码阈值。扫码停留路径另触发原zsh超时`.1`错误，已修为`0.1`并实测回车菜单。

[收尾](./收尾.json)与[原生预览收尾](./原生预览收尾.json)记录本轮PTY／fzf／sleep退出，只精确关闭本轮PNG文档，不退出共享Preview。未启动／停止用户9443、PG、NATS或隧道，未修改全局终端Profile、缓存配置或用户实例数据。前轮Linux96／Windows38／Mac50和旧SHA保持原冻结输入，不追认本轮二维码增量。

<a id="🔚" href="#前言">回到首页</a>
