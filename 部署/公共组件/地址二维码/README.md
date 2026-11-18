# 部署地址二维码

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、输出与使用](#usage)
- [二、来源与固定版本](#dependency)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

由部署入口调用的独立 [**Go**](https://go.dev/) 工具，使用本地已准备的 Go 构建，不依赖 Python、Node.js 或在线二维码网站，不修改业务后端模块。

## 一、<span id="usage">输出与使用</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

```shell
go build -mod=readonly -trimpath -o ./deployqr .
./deployqr --output-dir ./本轮二维码 --title '部署入口' \
  --entry '用户端|https://example.com/' \
  --terminal-width 100 --terminal-height 50 --ansi
```

- 完整 URL 原样编码；可重复传入多个有明确用途的入口，第一项为主入口。输出目录必须为本次运行独占且非符号链接的目录，不编造不存在的管理网页。
- 每个入口生成保留四模块白色静区的黑白 SVG 和 PNG；`index.html` 内嵌 SVG，无外部资源，浏览器直接打开即可扫码、选择复制文本或点击访问。
- 调用方传入真实 `--terminal-width 列数 --terminal-height 行数 --ansi` 时，用黑白背景色及双空格绘制完整模块，避免依赖方块字形的行间距。N模块二维码需要至少 `2N+2` 列、`N+6` 行；尺寸不够明确报告所需大小，由平台打开本轮本地PNG。第一项主入口最后绘制，多个入口滚屏后仍能看到当前主码；HTML／PNG顺序保持原样。
- 未传 `--ansi`、`TERM=dumb` 或已设置 `NO_COLOR`（包括空值）时不输出ANSI；指定了足够终端尺寸仍绘制无色字符码。其黑白极性由终端主题决定，平台同时打开本轮本地黑白PNG保障扫码，不把深色终端负片或只有文字当完成。`--terminal-width 0` 保持仅文件及URL输出，不读取输入、不改变终端配置。高度0表示调用方未限制高度。
- `--expire --output-dir ./本轮二维码` 将页面标记为失效，不再显示可扫码入口。已另存的图片无法撤销；图片只记录地址，服务和临时隧道结束后不再代表可用入口，重新部署必须使用新码。
- 使用最终对外地址。`0.0.0.0`、`[::]` 是监听地址，拒绝生成；回环地址明确标记“仅本机，手机不可用”。公网扫码只使用本轮实际隧道地址或明确配置并另验可达性的正式HTTPS地址，不推测域名或自动公开实例。
- 不在入口中放账号、密码、令牌或签名。本工具拒绝 URL userinfo 以及常见认证查询参数；普通查询参数、URL 转义与片段保持原文。该检查不替代调用方确认地址不含秘密。
- 二维码构建或生成失败仅告警；部署仍保留可复制 URL。辅助二进制和本轮二维码均放本项目 Runtime 的本次独占会话目录，归原部署与反部署清单统一管理。

## 二、<span id="dependency">来源与固定版本</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

二维码编码依赖 [**go-qrcode**](https://github.com/skip2/go-qrcode)，固定版本 `v0.0.0-20200617195104-da1b6568686e`，完整校验值在 `go.sum`，MIT 许可原文保留在 `LICENSE-go-qrcode.txt`。未复制或修改第三方源码；依赖准备沿用部署当前进程的 Go 镜像、官方回退及缓存配置，模块下载后依照 Go 校验机制核对内容。地址仅在本机内存参与编码，HTML/SVG/PNG 生成过程不联网。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
