# Android HTTP200 转发占位恢复定向证据

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、结果](#result)
- [二、验证的语义与边界](#proof)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮只给既有 `ForwardLifecycleTest` 补充受控分支并运行这一条仪器，不改 Android 产品源码、安装包或此前[6 JVM＋10 定向仪器报告](../20261115-引用转发/README.md)。不把这条额外验证与旧轮次相加。

## 一、<span id="result">结果</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

API37／Android17 ARM64 专用模拟器的单条生命周期测试通过，0 失败／错误／跳过，Gradle 退出 0。[验证摘要](./验证摘要.json)记录测试源和测试 APK 指纹；[原始日志](./单条仪器日志.txt)及[XML](<./TEST-JobsIMReview_20261114_1424(AVD) - 17-_app-.xml>)保留实际结果。

产品构建输入与旧报告逐份校验一致，目标 APK 字节仍为 SHA256 `624785dc9619898162917aad32c443ab19a992129895f87d25c1d5e3523078e1`；测试 APK 更新不代表产品包更新。初编遭遇既有超长协程测试方法的 JVM 大小上限，已仅将新增分支提成同文件 private suspend helper；[编译诊断](./首次编译方法大小诊断.txt)保留，不计作业务通过。

## 二、<span id="proof">验证的语义与边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

真实 HTTPS 精确消息 GET 的合法原体，经测试传输层转换为 HTTP200、`forwardUnavailable=true`、正文／附件 URL／临时 proof 为空，version 保持不变。随后分别通过真实 Room 的历史页和回执写入路径放回同版本旧投影，触发正常首绘，确认旧正文／URL不能解除占位；再关闭注入，新的精确 GET 于相同 version 恢复正文，确认不存在不可逆本地 latch。

这里的 HTTP200 临时依赖未知为受控响应注入，不冒充真实后端依赖故障；旧历史／回执由真实 Room 重插模拟，不冒充实际网络乱序。好友、群、转发与最终精确 GET 原体仍来自真实 HTTPS 28443 独立实例。未重跑全套、未真机／系统强杀／全 I 验收。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
