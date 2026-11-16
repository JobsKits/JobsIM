# 鸿蒙普通转发限定验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、已验证](#jobs-toc-heading-2)
- [二、尚未验证](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮按当前单一需求实现原生引用式转发，不代表完整第一阶段或243项通过。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、已验证</span><!-- jobs-toc:heading-end -->

25项纯规则主机测试实际通过；应用HAP、含25项Hypium的测试HAP构建通过，包CRC与[源码／产物摘要](./结果.json)已核对。真实日志为[应用构建](../20261115-131832-70850/运行日志.txt)、[测试包构建](../20261115-131844-70783/运行日志.txt)、[主机规则](./主机规则结果.txt)。旧角色／提及公告证据保留。

转发选择先GET当前源版本，变化后要求重新选择；目标GET后将原源proof、源内容版本、目标sendAdmission、clientMsgId与epoch入RDB，正文、附件ID、回复及提及不复制。未知仅明确重试，先纯查原message-results，404后才同ID／原body再投；明确失效及源HISTORY_NOT_ALLOWED／NOT_A_MEMBER拒绝冻结，不补资格；临时503不误冻。

前台历史及收藏／搜索中的转发独立重查投影，依赖未知或源不可用清空正文／附件URL／proof；缓存进入页面先显示占位。媒体只接固定当前消息附件代理路径，每次重新鉴权下载；已记录的本机临时预览文件在失效／离线时清理。本人转发隐藏编辑，保留撤回。新增文案完整十语言。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、尚未验证</span><!-- jobs-toc:heading-end -->

HDC无目标，未安装未签名包，25项Hypium只是编译，并未执行。未验证鸿蒙真实HTTPS、ArkUI、RDB耐久／强杀、系统文件预览生命周期和撤权时的外部预览行为；不把Node规则或其他端结果外推为鸿蒙运行通过。主题／语言和设备组合仍需实测。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
