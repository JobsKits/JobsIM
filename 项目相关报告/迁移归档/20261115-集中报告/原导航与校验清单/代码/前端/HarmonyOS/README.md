# JobsIM 鸿蒙原生工程

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [当前实现](#jobs-toc-heading-1)
- [构建与运行](#jobs-toc-heading-2)
- [信任与数据边界](#jobs-toc-heading-3)
- [尚未验收与剩余I边界](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">当前实现</span><!-- jobs-toc:heading-end -->

ArkTS／ArkUI Stage＋系统RDB原生客户端，保持暖白、墨黑和松绿；设置包含十语、白天／黑夜／跟随系统与持久字号。新增流程已分组实际构建应用和外置测试HAP，不等于完整I验收。

- 联系人：本人pending入站申请与当前requester资料；缺资料显示不可用，不沿用旧响应。接受／拒绝、搜索／建聊、删除、拉黑／解黑、本人显式reopen；原未决请求ID持久保留，解黑不复活旧申请。屏蔽列表明确只是本机操作记录，服务端仍最终判定双向门禁。
- 群：成员与资格代际、改名、添加／移除、转让、退出；可从联系人／成员选择，保留ID入口。移除重试只使用原membershipGeneration；只读群不新发送或提交普通治理写，危险动作核对目标后确认。
- 消息：文字和25MiB内媒体文件上传，回复／编辑／撤回、反应、置顶／收藏；内容不可用占位。epoch:seq游标原样持久，按ID合并服务端当前版本；可继续加载其余历史，前台每5秒轮询增量。草稿含回复及编辑目标，未读是本机读点，不能作为跨端已读或收讫资格证明。
- 提及：从当前群成员原生列表选择，群主可选全体；仅文字`@昵称`不取得提及资格。草稿与原Outbox持久mentionedUserIds／mentionAll，发送重试不换ID／原资格。RDB提及回执按原消息ID去重，与正文和history cursor同事务；升级前缓存先标为已观察，不因旧编辑重新提醒。打开会话清除本机提及未读；前台每轮按20条分批GET原消息重核已有未读，服务端mentionedMe=false／失权即清，解黑不复活已观察提醒。不是跨端已读或qualified ACK。
- 群公告：本群消息可作为来源，当前群主选择无期／1小时／1天／7天，确认前捕获原contentVersion／公告CAS／本人成员代际。完整原请求先持久，再GET原operation，缺失才按原PUT／DELETE body续跑；完成后重新GET当前公告，current=false不回滚。待办仅存消息ID／版本／到期引用，不存来源正文或附件；失效／过期不显示旧正文。明确拒绝的原操作可确认放弃，超时／503等未知结果保持待核。新入口需当前实例mentions／announcements明确支持。
- 搜索／收藏：独立分页接口及原样签名游标，收藏不发空搜索词；打开来源重取当前会话与消息，支持公共会话前100条以外的单项定位。冻结队列可调用只读message-results核对既成事实；404保留原记录且绝不转向创建型POST messages。
- 账号：严格区分登录OTP与账号恢复码；AssetStore只保存绑定可信连接、账户、设备和restoreEpoch的令牌，普通偏好只存公开指针。冷启动先校验已核对CA和bootstrap，再校验当前auth/context；设备会话列表／撤销及原ID恢复码生成已接入。冷启动无法在线核验时不恢复正式身份，不把演示替代正式缓存。
- 头像：本人原JPEG／PNG文件先写私有文件并fsync，持久SHA256、requestId及profileVersion；未知结果先查原操作，不重选文件／新ID；current=false以当前me为准，不恢复旧头像。服务端负责静态格式、2MiB／16MP与转换限制。
- 举报：消息／用户／会话入口、目标理由、原操作持久重试与本人列表。当前后端列表最多100项，没有虚构分页或聊天正文证据接口。
- 附件：每次打开前重新通过当前会话鉴权下载，私有缓存文件使用本地生成名称。显式调用系统外部预览，格式不支持时显示失败，不声称应用内安全播放器或已交出内容可回收。
- 本地通知：用户主动授权／开启，只发JobsIM与十语通用“新消息”，无发送人或正文。仅依赖前台同步时发现的新消息；退后台停止轮询，不冒充远程厂商推送。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">构建与运行</span><!-- jobs-toc:heading-end -->

使用用户指定[libsdl镜像工具链](./工具链/README.md)：CLT6.1.1.280、SDK6.1.1.125／API24、Hvigor6.24.2、Node18.20.1；最低兼容API12。源码只保留配置与锁，SDK、Node、缓存均在工程外，不修改系统全局环境、CA或代理。健康已有缓存复用，不切回官网／其它registry间接补工具。

```sh
../../../测试/前端/HarmonyOS/运行测试.command/运行测试.command check
../../../测试/前端/HarmonyOS/运行测试.command/运行测试.command build
../../../测试/前端/HarmonyOS/运行测试.command/运行测试.command test-build
../../../测试/前端/HarmonyOS/【MacOS】运行规则测试.command/【MacOS】运行规则测试.command
```

官方构建器拒绝中文路径，入口在用户缓存ASCII快照构建，保留源码／暂存SHA256清单；外置测试仍唯一位于`测试/前端/HarmonyOS/OhosTest`，只在快照调整相对引用和镜像内置测试依赖元数据。应用／测试HAP导出`Dist/`，当前均unsigned。签名材料、目标授权及设备执行单独准备，不因更换下载源而豁免。

[测试入口与最新证据](../../../测试/前端/HarmonyOS/README.md)区分实际主机规则、HAP构建和未执行的设备用例。没有目标设备结果时，不宣称ArkUI、RDB、AssetStore或通知已实机通过。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">信任与数据边界</span><!-- jobs-toc:heading-end -->

输入的待核服务地址与当前可信地址分离，编辑表单不会携带旧bearer请求新地址。新连接先核对可信部署终端给出的deploymentId及CA指纹；只在当前实例RCP会话使用系统信任或核对后的私有CA，仍校验主机名／有效期，显式禁止自动重定向，不安装全局根证书。冷启动重新核对已存CA字节与指纹后才读取令牌。

正式与演示为独立RDB；消息、草稿和游标按可信连接档案／实例／账户／设备／epoch隔离。待发持久原clientMsgId和创建时epoch，恢复变化冻结原队列，不换ID自动重发。目录资格消失或history明确403／404时删除已收正文缓存并冻结原待发；未知结果仍需按原ID核对，不把本地状态当作送达。账号级头像／举报／关系操作另按同可信档案、本人账户和epoch持久，重新认证后可核对原操作，不复用旧会话秘密。

RDB设置synchronous=FULL只是配置，设备断电和完整收讫／清云资格仍待验证。原生AssetStore使用DEVICE_UNLOCKED与SYNC_TYPE.NEVER，不设置卸载后保留；只有明确未命中返回空，服务错误不静默覆盖。令牌不进RDB、普通偏好或日志。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">尚未验收与剩余I边界</span><!-- jobs-toc:heading-end -->

本轮没有签名、模拟器／真机执行或真实鸿蒙后端联调；22项Hypium只完成测试包编译，21项实际主机规则不覆盖原生平台。仍需设备核对生命周期、CA拒绝路径、所有语言／字号／三主题、通知、媒体打开／播放、AssetStore、RDB耐久和原结果丢响应／恢复竞态。完整本人数据导出／受控导入迁移、资格ACK与清云、全部I容量及无障碍等仍未完成，不能把更多源码和HAP算成全I或243项验收。

厂商远程推送、RTC、地图、支付、产品SDK和E2EE按后续标段接入。单一现行需求仍是完整产品目标。

[后端契约](../../后端/Go/接口契约/README.md) · [代码地图](./代码地图/README.md) · [最新限定证据](../../../测试/前端/HarmonyOS/报告/20261115-提及公告与分页/README.md) · [产品记录](./PRODUCT.md) · [设计记录](./DESIGN.md)。

消息原发信资格绑定及旧记录保护见[专项证据](../../../测试/前端/HarmonyOS/报告/20261115-原发信资格/README.md)。原资格失效冻结原记录；媒体上传不改用后来会话资格，旧服务不接收POST。设备上的RDB／HTTP组合仍待验证。

[原角色资格专项](../../../测试/前端/HarmonyOS/报告/20261115-原角色资格/README.md)：23项主机规则执行、应用及23用例测试HAP编译通过；设备与真实鸿蒙HTTP未验。

普通转发已接原生源版本重选、原始引用入队、未知结果纯查／明确重投、转发投影清理与固定消息附件代理；[本轮25规则／HAP证据](../../../测试/前端/HarmonyOS/报告/20261115-普通转发/README.md)不代表鸿蒙设备运行。
