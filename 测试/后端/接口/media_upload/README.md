# JobsIM 稳定媒体上传合同

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、能力与请求](#request)
- [二、结果与明确重试](#result)
- [三、实现与验证状态](#verification)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

`stable-upload-v1` 是技术协议标记，属于现行一套 JobsIM 需求。录音先保全本地原件和原身份元数据，再上传；响应丢失或进程重启后先纯查原单，不换 nonce、不借新 family 续传。不得持久化 Cookie、Token、CSRF 或其它凭据。

## 一、<span id="request">能力与请求</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

`GET /api/media/capabilities` 使用当前普通身份，返回 `protocol="stable-upload-v1"`、`supported=true`、文件 `maxBytes="67108864"`、请求体 `maxRequestBytes="68157440"` 与 `resultPath="/api/media/upload-results"`。能力未匹配时，新持久上传队列不自动回退到非幂等路径。

`POST /api/media` 保留原 `multipart/form-data` 的 `file` 字段；稳定路径只允许一个文件和下列八个短字段，各出现一次。`POST /api/media/upload-results` 使用同名八字段的短 JSON，只读取原责任，不创建对象、发布文件、补写状态或自动重试。

| 字段 | 合同 |
| --- | --- |
| `requestId` | 8～128 字符的稳定原 nonce，推荐随机 UUID。 |
| `userId`／`deviceId`／`familyId`／`restoreEpoch` | 生成原责任时捕获的身份；必须与本次实际认证及独立 epoch 一致。刷新可延续同 family，重新登录或恢复后的新 family／epoch 不能承接旧单。 |
| `name` | 原文件的规范 basename，不含路径或控制字符，最多 255 字节；须与文件 part 的规范名称一致。 |
| `size` | 原文件实际字节数的规范十进制字符串，0～64 MiB。1 MiB 请求余量仅供 multipart 元数据，不扩大文件上限。 |
| `sha256` | 原文件实际字节的 SHA-256，小写 64 位十六进制；服务器重新计算并核对。 |

浏览器使用原 family 的普通 BFF、Origin 与 CSRF；原生使用原普通访问凭据。纯查也遵守相同身份门禁，不新增“后继 family 查询历史原单”的授权。旧无 nonce 调用继续兼容，但不具备稳定原结果保证。

## 二、<span id="result">结果与明确重试</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

| 结果 | 客户端行为 |
| --- | --- |
| `404 UPLOAD_NOT_FOUND` | 本次纯查未见原单，不创建任何对象；保留本地原件，等待用户明确上传。 |
| `200 uploadState="prepared"` | 原责任已有固定 `id`，但尚无可用 URL；只在用户明确选择同原体重试时重新 POST 文件。 |
| `200 uploadState="committed"` | 包含同原 `id/name/contentType/size/sha256/url` 及 `requestId/protocol`；沿原稳定消息 ID 接续消息 Outbox，语音仍须 `contentType` 为 `audio/*`。响应 `size` 为数字。 |
| `409` 上下文、元数据或原体冲突 | 冻结原单，不覆盖文件，不换 ID 伪装成功；用户另开新责任须明确表达。 |
| `410 UPLOAD_RETIRED` | 原对象已被清理或原责任已退休，终止旧责任，不重新发布原 UUID。 |
| 未知、超时、独立 journal 未确认 | 保留本地责任；先纯查，不将网络失败当成未创建，也不后台重放字节。 |

本地恢复可以枚举旧原单；发现 user／device／family／epoch 已变化时仅冻结展示，不能携新凭据替旧单查询或续传。相同 nonce 的同原体明确重试复用原对象，不能覆盖已经绑定、已清理或受 owner／retention floor 约束的 UUID。

## 三、<span id="verification">实现与验证状态</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

独立 `media_upload*.go` 与共享入口窄接线已实现。[稳定上传限定回归](../../../../项目相关报告/测试/后端/20261117-稳定媒体上传/README.md)实际通过 1 个上传纯规则、8 个新上传数据库组、受影响注销 19 组及两个 cmd 编译；最新真实浏览器合成音轨已用于实际分类和进程内生产上传处理器。真实 HTTPS、两端持久队列重启／上传联调、麦克风与播放仍分别验收，不能把后端组通过外推为端到端完成。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
