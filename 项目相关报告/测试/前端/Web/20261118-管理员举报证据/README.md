# <span id="前言">Web 管理员举报证据切片</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前入口与行为](#behavior)
- [二、证据媒体与权限边界](#media)
- [三、当前检查](#checks)
- [四、待验](#pending)

</details>
<!-- jobs-toc:end -->

---

本目录记录 2026-11-18 的管理员举报详情与过滤源码，不扩展普通聊天界面。初轮静态证据保留在本目录；恢复空间后的[独立浏览器续验](./Resume/README.md)已完成42通过／0失败，全部 API 使用明确本地夹具，未连接真实实例。

## 一、<span id="behavior">当前入口与行为</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

管理员举报列表可按 `reporterId`／`targetUserId` 两项可选 UUID 过滤，并可清除筛选。点击一条举报先读取 `/admin/reports/{id}`，显示举报原因、举报人／目标用户、主动提交材料及主动选择的消息；必须收到 `includesWholeConversation:false`，不展示整段会话。

详情中的“管理”继续进入原有 MFA 处置表单，保留原 requestId、step-up 证明、expectedVersion、未知结果与重试流程；只读查看不扩大写权限。新增 [AdminReportDetail](../../../../../代码/前端/Web/Src/AdminReportDetail.tsx)、[协议校验](../../../../../代码/前端/Web/Src/AdminReportProtocol.ts)、[十语标签](../../../../../代码/前端/Web/Src/AdminReportLabels.ts)和局部样式；[AdminPanel](../../../../../代码/前端/Web/Src/AdminPanel.tsx)只接入这些入口，基础词库仅接新增标签回退。

## 二、<span id="media">证据媒体与权限边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

媒体只在显式点击时通过原有 `apiBoundedBytes` 认证链读取，使用当前会话的 cookie／家族头，不将私有 URL 直接放入图片或视频标签。固定路由分别为当前举报关联的 `/api/reports/evidence/{objectId}?reportId={reportId}` 和 `/api/admin/reports/{reportId}/messages/{messageId}/attachment`；拒绝外源、其它举报、重复／多余查询和路径变化。

请求使用 `cache:no-store`、`redirect:error`，不写持久媒体缓存。图片最多20MiB、视频最多64MiB，按声明类型选择实际流上限；证据校验声明尺寸及 SHA256，预览再核文件签名与 MIME。当前只有一个活动预览；切换、关闭、离开页面、账号变更、后台隐藏或已知失权时中止请求、撤销 blob URL，视频另暂停并释放 src。`available:false` 的正文／附件地址在校验层即清除；读取失败不回旧 blob。

浏览器续验已在实际组件中核对显式加载、单预览、视频暂停、blob 撤销、迟到响应和夹具失权清空；服务器即时撤权仍需实际联调。后台每次媒体 GET 的授权、举报关联、源内容版本和删除 floor 校验由后端负责，本目录不代其声明已验。

## 三、<span id="checks">当前检查</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

> [summary.json](./summary.json)及逐项原件区分检查层次。

| 检查 | 结果 |
| --- | --- |
| [TypeScript](https://www.typescriptlang.org/) `tsc --noEmit` | 退出码0；[原日志](./tsc.log)，没有生成 Web bundle |
| 生产协议及标签 | 16通过／0失败；[逐项结果](./protocol-tests.json)，测试入口见[说明](../../../../../测试/前端/Web/admin-report-evidence.cjs/README.md) |
| 原 MFA apply | 测试核对与 [before/AdminPanel.tsx](./before/AdminPanel.tsx) 字节一致，不是实际 MFA 网络验收 |
| 十语标签 | 十种语言、每种11个新键，键集一致且非空 |
| 独立浏览器续验 | 42通过／0失败；[当前逐项结果](./Resume/browser-2/summary.json)，首轮40通过／1选择器失败[原件保留](./Resume/browser-1/summary.json) |

初轮源和测试 SHA 见 [inputs.json](./inputs.json)，续验前后12项输入见[当前绑定](./Resume/browser-2/inputs-after.json)；8项原静态源码输入均未变化。普通 Web 聊天、根 API、会话协议及原第三方源码没有在本切片修改；既有测试结果不覆盖新切片。

## 四、<span id="pending">待验</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

仍待真实举报及认证媒体 GET、后端权限／版本／删除 floor 拒绝、实际 MFA 处置、完整20／64MiB响应与真实手机验证。320px十语明暗布局属于桌面 Chrome 视口；跟随系统的应用级切换不在本次夹具范围。未启动默认实例、创建生产举报或下载用户真实证据。

<a id="🔚" href="#前言">返回前言</a>
