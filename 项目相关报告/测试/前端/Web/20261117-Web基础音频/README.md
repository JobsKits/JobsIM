# <span id="前言">Web 基础录音与播放续点候选</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前实现](#jobs-toc-heading-1)
- [二、实际验证与原始失败](#jobs-toc-heading-2)
- [三、真实合成音轨与视觉](#jobs-toc-heading-3)
- [四、输入、资源与产物边界](#jobs-toc-heading-4)
- [五、官方API依据](#jobs-toc-heading-5)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、当前实现 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

本轮依据现行FE-028／FE-053／FE-069，在已有聊天输入区、上传与Outbox上接原生录音，保留现有绿色工作区、语义色、三主题与十语；没有新增外部SDK。播放续点使用原JobsIM v1／spaces严格事务和现有global／auth门禁，不使用localStorage旁路。

| 入口 | 默认／行为 | 持久化与生命周期 |
| --- | --- | --- |
| 输入区“录制语音” | 显式点击才申请麦克风；实际探测Opus WebM／MP4；1秒至5分钟／16 MiB上限 | 活动track在取消、设备中断、隐藏／离页、会话／身份变化或销毁时停止；迟到许可stream同样关闭 |
| 停止→试听→明确发送 | 停止不自动发消息；可试听、重录、取消 | 未入Outbox时保留RAM预览；原身份、会话、namespace与恢复代际未变才继续现upload；服务端须确认audio类型 |
| 原消息音频／视频 | 原生控件、0.75／1／1.25／1.5／2倍速；“已播放”为本机标记 | 显式播放先精确GET原消息重核权限／媒体路径；不授予服务端已读／收讫资格 |
| 本机续点 | 绑定本人完整namespace、消息／会话、contentVersion和实际媒体字节SHA；恢复位置不自动播放 | 最多256条metadata，无URL／正文／文件名／凭据；forward及演示Blob不持久化；非有限时长提示不能可靠保存 |
| 失权／撤回／global／换身份 | 停旧播放，撤Blob URL；晚到旧版本／不同摘要不能删或写新位置 | 续点读写同严格事务核global与auth-active／auth-frozen；global同原事务清未挂载namespace与archive对应项 |

源码与实际入口见[Web说明](../../../../../代码/前端/Web/README.md#web-audio-messages)和[代码地图](../../../../../代码/前端/Web/代码地图/README.md)。原生平台本轮未运行；iOS旧录音仅作为显式权限入口参考，没有照搬停止即发送或临时文件清理行为。

**当前发送前半段仍未耐久闭环。** 点击发送到`/media`返回前只有RAM录音；进程退出会丢本次发送意图。本候选验证现有upload／Outbox接线与本机恢复，不代表稳定上传nonce、原Blob先严格落盘、上传丢响应原结果查询或重启明确同体重试已实现。后续按后端最终合同接入，不能把这些下一步写成已通过。FE-028／FE-053／FE-069及基础阶段整体均保持相应未验范围。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、实际验证与原始失败 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 范围 | 本轮结果 | 原件 |
| --- | --- | --- |
| 生产录音状态机／续点规则／十语 | 22组纯Node通过；无浏览器／编译 | [规则结果](./纯规则-1794881192280/结果.json) |
| 完整Web严格类型 | `tsc --noEmit`通过 | [最终类型回执](./02-类型检查-1794882044332-87393/进程.json) |
| Chrome原生录音与严格IDB | 最终19组通过，包含2个实际App演示输入区案例 | [最终浏览器结果](./03-原生浏览器-1794882303104-90890/浏览器/结果.json) |
| 原global事务 | 6组通过 | [结果](./04-global-facts-1794882127499-90230/浏览器/结果.json) |
| 原Outbox完整body证明 | 10组通过 | [结果](./04-outbox-proof-1794882128257-90230/浏览器/原请求队列结果.json) |
| 原登录／续期协议 | 16组通过 | [结果](./04-auth-session-1794882129209-90230/浏览器/浏览器协议结果.json) |
| 私有生产构建 | 成功；不覆盖现有运行包 | [回执](./05-私有构建-1794882222357-90614/进程.json)、[产物摘要](./最新私有构建.json) |

首轮Chrome在“403撤权清除精确续点”失败，[红原件](./03-原生浏览器-1794881938424-86367/浏览器/失败.json)及当时输入保留。UI收到403后立即卸载播放器，异步forget复用挂载signal，被cleanup取消，因此RAM已撤而IDB位置未清。修复将删除权限绑定到仍有效的原认证／消息／CV／摘要，允许该精确清理在卸载后完成；较新CV或不同摘要仍由CAS保全。修后先17组通过，追加2个实际App演示接线和截图后再取新脚本输入19组通过，未覆盖红原件。

浏览器还实际验证原生play→暂停→精确鉴权→重放的事件顺序；旧排队pause仅在元素当前确为paused时清播放授权。持续播放换auth不等待App重绘，timeupdate立即停播／撤URL。真实IDB验证了未挂载当前namespace／archive同scope清理、其它scope保留、global后迟到save零复活、新CV首次save和旧CV／不同摘要的保存／删除拒旧。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、真实合成音轨与视觉 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

已有[**Chrome**](https://www.google.com/chrome/)的原生[**MediaRecorder**](https://developer.mozilla.org/en-US/docs/Web/API/MediaRecorder)实际编码[**AudioContext**](https://developer.mozilla.org/en-US/docs/Web/API/AudioContext)合成220Hz音轨；没有访问真实麦克风，浏览器输出静音。导出的[真实合成WebM](./03-原生浏览器-1794882303104-90890/浏览器/原生合成音轨.webm)与[容器摘要](./03-原生浏览器-1794882303104-90890/浏览器/合成容器SHA.json)可供后端独立轨道分类测试。它不是手造EBML，也不是用户录音或真实服务上传证据。

17组轮的合成容器曾先行提供给后端owner：19497字节、MIME`audio/webm`、SHA`beecd91256e6d0b0f266fad307af79072abd097905aa0ec63c7f221581ea3191`，原件在`03-原生浏览器-1794882057111-87548/浏览器/`；最终19组另有独立容器与摘要，不互换回执。后端新WebM解析及其真实容器兼容结果由后端报告记录，不能在本页推断已接实际服务。

已查看本轮[英文白天试听](./03-原生浏览器-1794882303104-90890/浏览器/录音试听-英文-白天.png)、[藏文黑夜390px入口](./03-原生浏览器-1794882303104-90890/浏览器/录音-藏文-黑夜.png)与[实际App中文黑夜输入区](./03-原生浏览器-1794882303104-90890/浏览器/App录音入口-中文-黑夜.png)。沿用原语义色、焦点和现有本地图标；长文案换行，未另造跨平台外壳。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、输入、资源与产物边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[外层运行器](./运行器.cjs)在每阶段重新枚举全部`Src/`及本地资源／入口／锁文件／配置和关联脚本，包含apiBoundedBytes、迁移新模块与global清理hook，先复制固定输入，再核前后SHA与完整文件集合。4GiB磁盘监控、180秒子进程组上限及单worker串行均未触发停止；没有下载依赖、VM、SDK或调用真实API／麦克风。阶段分别记录原始日志，不能将不同方法组相加为产品功能数。

生产包为`index-DFmR11q7.js`，SHA`1b816e189b08fad52c14330de7223fe51d45f1bc000ecb50d41d1495c089e7b7`；CSS为`index-DgDohfYo.css`，SHA`b9f5c87fa9594ee3a1085439f3a38a250409c45927178c83922686570e763f9b`。绝对私有输出位置与全部产物摘要见[构建记录](./最新私有构建.json)。最后新增2个App案例只改变测试脚本，生产源与该包输入一致；实际运行实例未由本轮替换。

浏览器HTTP、身份、上传回调和剪贴板均为明确夹具；实际App只验本地演示上传接线，不证明当前真实账户的上传／OutboxHTTP。系统电话／耳机切换、真实麦克风、断电／配额、跨浏览器编码、真实HTTPS与稳定上传原责任仍待独立目标环境验证。

## <span id="jobs-toc-heading-5" data-jobs-toc="v2">五、官方API依据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

本轮实际查阅[麦克风许可与安全上下文](https://developer.mozilla.org/en-US/docs/Web/API/MediaDevices/getUserMedia)、[录音容器支持探测](https://developer.mozilla.org/en-US/docs/Web/API/MediaRecorder/isTypeSupported_static)、[停止录音及最终数据事件](https://developer.mozilla.org/en-US/docs/Web/API/MediaRecorder/stop)、[显式停止所有track](https://developer.mozilla.org/en-US/docs/Web/API/MediaStreamTrack/stop)、[当前播放位置](https://developer.mozilla.org/en-US/docs/Web/API/HTMLMediaElement/currentTime)和[暂停播放](https://developer.mozilla.org/en-US/docs/Web/API/HTMLMediaElement/pause)。支持探测不保证设备资源一直可用，许可Promise也可能长期未决，因此实现额外处理迟到stream、设备中断与原owner失效。

<a id="🔚" href="#前言">回到首页</a>
