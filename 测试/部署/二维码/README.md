# JobsIM 离线二维码验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、执行](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

构建[二维码组件](../../../部署/公共组件/地址二维码/README.md)，实际生成HTML／SVG／PNG；Mac使用系统CoreImage解码PNG逐字核对URL，另核拒绝带凭据／token的网址。测试不公开服务、建立隧道或打开真实浏览器。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、执行 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a></span><!-- jobs-toc:heading-end -->

设置`JOBS_IM_GO`指向健康Go，依赖已按go.sum准备后运行[验证入口](./验证二维码.mjs/验证二维码.mjs)。测试编译使用`GOPROXY=off`，输出在本轮临时目录。平台实际菜单、地址变化、退出失效和失败保服务由各平台测试分别取证。

[终端二维码定向入口](./终端二维码.py/README.md)使用独立PTY验证足尺寸背景方格、窄窗／高度／NO_COLOR原生PNG、非TTY和扫码留屏后回车菜单，并把实际ANSI输出经AppKit成像后由CoreImage解码；不借仅生成PNG的证据代替终端显示。
<a id="🔚" href="#前言">返回前言</a>
