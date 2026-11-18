# <span id="前言">终端二维码定向验证</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、入口与范围](#jobs-toc-heading-1)
- [二、可视与解码](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、入口与范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[测试入口](./终端二维码.py)使用已有Python／Go及MacOS系统AppKit／CoreImage，直接加载两份正式MacOS二维码函数，在新建目录与真实PTY中验证尺寸、NO_COLOR空值、TERM=dumb、非TTY和扫码留屏后回车打开真实fzf菜单。只用本轮短sleep模拟菜单的服务存活，不运行Go业务、PG、NATS或隧道。

```shell
python3 ./终端二维码.py --report 相对或绝对的新报告目录
```

`JOBS_IM_GO`须指定已准备的健康Go。测试以`GOPROXY=off`构建小二维码工具，不安装依赖；输出目录必须不存在。`--native-preview`只请求预览本轮生成的一张本地PNG，不打开业务网页或公开实例。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、可视与解码 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[AppKit渲染器](./渲染终端码.js)读取PTY实际输出的ANSI／字符，以Menlo与真实单元背景成像，再由CoreImage解码逐字核对原URL。足尺寸80列×50行、长地址及菜单停留分别核深／浅背景；32列、80列×30行、12行窗口验证完整黑白PNG路径。NO_COLOR不出现ANSI，字符极性不被冒充为通用可扫码保证，独立核原生PNG。

原件包括逐场PTY日志、纯文本运行日志、终端成像PNG、生成PNG与前后SHA。公开示例地址是离线合成文字，不代表该域名存在或可达；回环标签保持“仅本机，手机不可用”。Linux／Windows的调用合同和解析与目标系统原生显示效果分别记录。

<a id="🔚" href="#前言">回到首页</a>
