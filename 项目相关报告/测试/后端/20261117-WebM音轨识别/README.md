# JobsIM [**WebM**](https://www.webmproject.org/) 纯音频识别与结构边界

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、分类入口与兼容范围](#classification)
- [二、有界读取与流式结构](#bounds)
- [三、已执行的 6 个测试组](#verification)
- [四、输入核验、原件与复现](#evidence)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

浏览器录制的纯音频 WebM 现在依据实际已打开文件的轨道结构分类为 `audio/webm`；有视频轨道的容器仍为 `video/webm`。文件名和客户端声明 MIME 不决定音频资格，`TrackType=audio` 配视频 CodecID 的伪装会被拒绝。现有 MP4 音频、视频和混合轨道分类保持原行为。

这项能力已由 2026-11-17 10:05:40～10:05:42（UTC+8）的原 [**Go**](https://go.dev/) 回归覆盖：**6 个 WebM 顶层测试、44 个子项全部通过，0 失败、0 跳过**。复用时已逐项核对该轮 172 项输入一致，未重复启动 Go；对应原件与保全输入如下。该批使用手造容器元数据和受控 ReaderAt。后续[稳定媒体上传专项](../20261117-稳定媒体上传/README.md)已新增真实 [MediaRecorder](https://www.w3.org/TR/mediastream-recording/) 合成音轨 Blob 的实际分类和进程内上传验证；真实网络上传、麦克风、音频解码与播放仍另验。

## 一、<span id="classification">分类入口与兼容范围</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 入口 | 行为 |
| --- | --- |
| [文件类型入口](../../../../代码/后端/Go/media_format.go) | 打开实际文件并取其大小；遇 EBML 签名后用同一文件描述符解析轨道，不沿用通用嗅探器对所有 WebM 返回的视频类型。解析失败返回 `400 INVALID_MEDIA_CONTAINER`。 |
| [WebM 轨道解析器](../../../../代码/后端/Go/media_webm.go) | 要求 `DocType=webm`、单一 Segment、首个 Cluster 前唯一的有效 Tracks；TrackEntry 必须有唯一且非零的 TrackNumber／TrackUID、明确 TrackType 和 CodecID。 |
| [外置反例与回归](../../../../测试/后端/接口/media_webm_test.go) | 覆盖实际文件入口、ReaderAt 结构解析、伪装、截断、资源边界、流式未知长度，以及原 BMFF 分类。 |

纯音频轨道只接受 `TrackType=2` 且 CodecID 为 `A_OPUS` 或 `A_VORBIS`。`TrackType=1` 必须配非空 `V_` 视频 CodecID；任一视频轨道存在即返回 `video/webm`。这只决定容器媒体类型，不证明具体编码包可解码；未知音频编码、其它轨道类型、多 Segment、重复／过晚 Tracks 等不在当前支持范围，按无效容器拒绝，不能宣称完整 WebM／Matroska 兼容。

前端录音可先通过 `MediaRecorder.isTypeSupported` 选择 `audio/webm;codecs=opus`，再探测 `audio/webm`；既有 MP4 音频支持保持独立。上传后的规范结果仍须是 `audio/*` 才能作为语音使用。官方说明区分 WebM 的纯音频 MIME，并列出 Opus／Vorbis 音频编码；轨道枚举与必需字段以官方 schema 为依据。[**WebM 容器说明**](https://www.webmproject.org/docs/container/)、[**IETF Matroska schema**](https://raw.githubusercontent.com/ietf-wg-cellar/matroska-specification/master/ebml_matroska.xml)。

## 二、<span id="bounds">有界读取与流式结构</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 边界 | 当前限制 |
| --- | --- |
| ID／长度 VINT | ID 最多 4 字节，长度最多 8 字节；拒零前导标记、保留 ID、不合法的非最短 ID、截断及超出父元素的长度；长度计算先比较剩余范围，避免加法溢出。 |
| 元素／深度／轨道 | 最多 20,000 个解析元素、8 层已识别元数据、64 个轨道。 |
| 关键字段 | EBML Header 最多 4 KiB；Tracks 最多 4 MiB；单 TrackEntry 最多 1 MiB；DocType 最多 16 字节；CodecID 最多 64 字节；无符号字段为 1～8 字节。 |
| ReaderAt | 单次字段读取最多 64 字节，累计最多 1 MiB；额外探测文件末字节验证声明大小可读。媒体包不整体加载、解码或另存。 |
| 元数据／媒体包 | 普通有限长度字段最多 16 MiB，已识别元数据 master 最多 4 MiB。SimpleBlock／Block 及其 BlockGroup 不套该元数据长度上限，按已打开文件的实际范围跳过，不分配媒体包大小的内存。 |
| 未知长度 | 只允许 Segment／Cluster。未知长度 Cluster 逐元素跳过有限子元素，在合法顶层元素边界结束；不在二进制 payload 中搜索 Tracks 或 CodecID 字节。其它未知长度元素拒绝。 |

流式 Segment 可使用浏览器常见的 8 字节未知长度。解析仍检查后续元素，拒绝把第二个 Tracks 或新 Segment 拼在音频轨道后面；CodecPrivate 内貌似视频轨道的字节不会被当成真实轨道。轨道类型和编码必须来自 TrackEntry 的直接字段，不能通过二进制私有数据中的 `A_OPUS` 文本获得音频资格。

VINT 与未知长度规则参考 [**RFC 8794**](https://www.rfc-editor.org/rfc/rfc8794.html)。[**RFC 9559**](https://www.rfc-editor.org/rfc/rfc9559.html)已修正单字节 ID `0x80` 的有效性，解析器允许该 ID，并有 ChapterDisplay 正例；仍拒绝多字节零 ID、非最短 ID 和 `0xFF`。这项修正不等于实现完整章节语义。

## 三、<span id="verification">已执行的 6 个测试组</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 顶层测试 | 参数化子项 | 验证内容 |
| --- | --- | --- |
| `TestWebMClassifiesActualTracksInsteadOfSniffOrName` | 7 | Opus、Vorbis、音视频混合、纯视频、CodecPrivate 假轨道、未知长度 Segment／多个 Cluster／Cues，以及合法 `0x80`；并复现标准嗅探器对纯音频给出 `video/webm`。 |
| `TestWebMRejectsSpoofedAndMalformedTrackStructures` | 29 | 类型与编码不符、缺字段／二进制诱饵、空／重复／晚 Tracks、重复轨道身份、异常字符串、未知长度非法元素、截断／非法 VINT／超界长度、第二 Segment 与错误 DocType。 |
| `TestWebMBoundsElementsDepthTracksAndReaderAt` | 3 | 轨道数、元数据深度、元素数上限；同时检查小块 ReaderAt 和短物理文件拒绝。 |
| `TestWebMRejectsMalformedContainerThroughStoredMediaEntry` | 0 | 实际文件入口不能被声明的 `audio/webm` 绕过。 |
| `TestWebMSkipsLargeEncodedPacketsButBoundsMetadata` | 2 | 虚拟大于 16 MiB 的 SimpleBlock 可有界跳过，等长普通字段被拒绝；无真实媒体包解码。 |
| `TestWebMPreservesBMFFTrackClassification` | 3 | MP4 音频仍为 `audio/mp4`，视频和混合轨道仍为 `video/mp4`。 |

合计 **6 个顶层 PASS＋44 个子项 PASS＝50 个 PASS 事件**，不能记成 50 个独立顶层测试。此处复用的完整回归原汇总为 71 个顶层通过、115 个顶层数据库条件跳过、0 失败；其余业务不计入 WebM 专项结论。两个 cmd 包在原 JSONL 中为无测试文件的包级 skip，不能写成两个 cmd 的独立构建或运行已经验收。

## 四、<span id="evidence">输入核验、原件与复现</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 原件 | 用途 |
| --- | --- |
| [原跑前输入 SHA256](../../../实现进度/20261116-剩余能力闭环/工作树搬迁/重启前Go纯回归/跑前输入.json) | 完整后端源码、模块文件与外置测试输入，共 172 项。 |
| [原始 JSONL](../../../实现进度/20261116-剩余能力闭环/工作树搬迁/重启前Go纯回归/原始.jsonl)、[stderr](../../../实现进度/20261116-剩余能力闭环/工作树搬迁/重启前Go纯回归/stderr.txt)与[原汇总](../../../实现进度/20261116-剩余能力闭环/工作树搬迁/重启前Go纯回归/结果.json) | 六个 WebM 顶层测试及其所有子项的实际通过事件；原运行 `changedInputs=[]`。 |
| [复用核验](./复用核验.json) | 记录原件摘要、172 项当前 SHA 全部一致、六组事件计数和本专项未另启 Go 的事实。 |
| [已验输入保全](./已验输入/) | 执行后按原跑前 SHA256 逐字节核齐再复制的输入；不是运行前已存在该封存目录的追认。三个关键文件可分别核对[类型入口](./已验输入/代码/后端/Go/media_format.go)、[解析器](./已验输入/代码/后端/Go/media_webm.go)和[测试](./已验输入/测试/后端/接口/media_webm_test.go)。 |
| [备用验证入口](./验证.mjs) | 仅 `node --check` 通过，未执行；为后续源码变化后的独立回归保留，不能用它证明本轮测试已运行。 |

备用入口会重新冻结完整后端与递归外置测试，拒绝同名冲突，在副本执行 WebM／BMFF 和已有头像／纯媒体规则；成功后可编译两个 cmd。限定现有本地 Go 工具链和暖缓存，`GOMAXPROCS=2`、`-p=1`、`GOPROXY=off`、`GOTOOLCHAIN=local`，每阶段有超时及 4 GiB 磁盘监控。不会启动 [**PostgreSQL**](https://www.postgresql.org/)、真实服务或浏览器，也不下载依赖、清缓存或删除原证据。仅在源码变化、失败或新的覆盖缺口需要复验时使用，不因已有脚本而重复已过的相同输入。

```shell
node ./验证.mjs
```

真实浏览器验证须后续在受控窗口由 MediaRecorder 生成音频 Blob，覆盖浏览器实际容器、未知长度结构、上传后规范 MIME 和语音入口；合成音轨也须明确标注无真实麦克风。当前测试的手造 Opus／Vorbis 元数据不能替代该证据，不外推真实录音时长、硬件采集、端侧落盘或音频播放已经通过。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
