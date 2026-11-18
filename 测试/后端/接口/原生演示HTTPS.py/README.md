# 原生演示 HTTPS 验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、范围与调用](#checks)

</details>
<!-- jobs-toc:end -->

## 🔥 <span id="前言">前言</span>

[验证脚本](./原生演示HTTPS.py)使用Python标准库，对显式给定的公开demo配对执行严格CA及主机验证。需要已经就绪的Mac独立demo；只接受该控制面19544／业务19543，不使用默认实例。密码和访问令牌只在RAM，输出仅记录检查、公开ID和摘要。

## 一、<span id="checks">范围与调用</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

参数为 `--descriptor <公开demo-host.json> --gif-source <专属合成GIF夹具> --report <新报告.json>`。脚本新建三个有明确前缀的普通测试账户，只在它们之间检查资料／签名、GIF上传／读取／预览、原单重放、陌生／好友／拉黑权限、联系方式hidden与固定控制接口。结束退出本轮会话，保留专属测试账户／材料供审计，不删除数据、不改seed账户密码。

实际多帧延时、loop和disposal语义另用[原生Go用例](../profile_gif_test.go)核验；HTTPS下载字节再由报告中的Go标准库工具独立DecodeAll，不能将MIME或静态预览当动态支持。实际结果见[专项报告](../../../../项目相关报告/测试/前端/iOS/20261118-资料与原生演示/Backend/README.md)。

<a id="🔚" href="#前言">返回前言</a>
