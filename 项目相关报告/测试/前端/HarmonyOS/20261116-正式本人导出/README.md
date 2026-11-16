# <span id="前言">JobsIM HarmonyOS 正式本人导出</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、前言](#jobs-toc-heading-1)
- [二、实现与存储](#jobs-toc-heading-2)
- [三、逐层证据与未验边界](#jobs-toc-heading-3)
- [四、接续与导航](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、前言 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

当前原生ArkTS／ArkUI导出源码已落盘；首轮15项主机注入通过、纯协议类型检查零错误。其后新增文件／系统选择器适配反例与页面生命周期修补，18项目标被4GiB硬门禁拦在执行前，不能以原15项追认当前全部输入。新5项Hypium仅有源码，当前共42方法；本切片应用／测试HAP尚未构建，无HDC安装、鸿蒙HTTPS或真实系统保存通过声明。

未访问58443、E2或任何其它API，未创建用户／家族、启停服务、下载工具或修改全局CA。旧35项S2主机、41项旧规则和37方法HAP分别保留，不与本轮15项累加为验收。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、实现与存储 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

设置→独立本人导出页，沿现有ArkUI暖白／墨黑／松绿、三主题、字号和十语41标签；提供本设备任务列表、完整Logo空态／重新加载、原请求核对／明确重试、取消确认与清理状态、下载／停止。演示和未登录只提示连接后可用，不构造私人ZIP，不冒充服务端全设备任务列表。

独立`export_intents`保存原create字符串／SHA／nonce／epoch、独立原cancel字符串／SHA／nonce及任务ID。复用正式RDB的写串行与FULL设置，以当前namespace＋稳定family隔离，核已有物理UUID，不创建假同步登记；原单变化、CAS失败或原物理库不匹配拒绝HTTP。新记录无AT／RT、下载票、制品key、聊天正文或ZIP。重载只读原完整body；仅明确`EXPORT_OPERATION_NOT_FOUND`且用户主动重试才原body创建，已成任务404不会重新创建。能力撤回仍可查／取消，新建和下载停止。

RCP继续使用既有实例CA／主机名校验、关闭重定向和认证协调；ticket只在RAM中的header，不落URL、RDB或日志。下载流写本模块私有临时文件，1GiB总量／32MiB目录上限，校验安全服务端文件名、ZIP MIME、长度／完整中央目录、文件SHA，文件和父目录fsync后进入`DocumentViewPicker.save`。保存前重新查当前ready／权限／期限与新短票并重校本地文件，再分块写目标并fsync；取消选择不显示已保存，失败尽力删本次新文档并提示可能残留不完整文件。身份／page／请求生命周期变化拒迟到结果，停止下载和退出清私有暂存；外部已经保存的ZIP不声称远程可抹除。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、逐层证据与未验边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 层次 | 已实际执行 | 明确没有证明 |
| --- | --- | --- |
| 正式六API | 首轮真实ArkTS Service对注入HTTP执行原create／原results／status／cancel／ticket路径和载荷断言 | 无正式服务HTTPS；源2b58矩阵不能替代本端API |
| 原单／物理空间 | 实际ExportStore经系统Python私有SQLite适配，重开原体、CAS、跨family空集与物理UUID变化拒绝 | 非Harmony RDB NAPI；无设备断电／跨进程资格声明 |
| 鉴权下载 | 实际Api转译结果对注入RCP执行CA、不重定向、RAM票header、流式sink及超限／身份失效无finish | 非原生RCP网络、TLS握手或取消实现；首轮未执行新ACCESS_EXPIRED文件反例 |
| ZIP／安全文件名 | 原协议规则有界读取、截断HTTP200／尾随／坏中央目录、文件上限及路径／控制字符拒绝 | 不等于文件已落盘，也没有解压／CRC验证资格宣称 |
| 文件／目标选择 | 新2项Node文件适配源码：file→rename→parent fsync先于picker、目标精确字节、取消≠保存及迟到身份清新目标 | 尚未执行这2项；也没有Harmony文件NAPI、DocumentViewPicker或用户目标提供方证明 |
| 界面／生命周期 | 已接page／family／namespace／physical围栏和旧操作责任，纯层曾覆盖迟到成功不得发布 | 当前后补页面未HAP编译；未执行真实导航、前后台、停止下载、十语三主题渲染或系统保存 |

| 独立原件 | 结果与输入 |
| --- | --- |
| [首轮15项](./Host-ulwapl/结果.json)／[原始输出](./Host-ulwapl/原始结果.log)／[输入SHA](./Host-ulwapl/源SHA.json) | 15／15，零fail／skip，纯类型0错；Python SQLite与RCP为明确适配器 |
| [后补source-only](./SourceOnly-追加输入.json) | 当前12个本轮变更输入SHA；不是全应用依赖闭包或新通过记录 |
| [4GiB门禁](./Gate-4GiB.json) | 第二轮启动即拒绝，18项目标均未执行，未创建第二工作目录 |

当前18目标＝原15＋新增1项ACCESS_EXPIRED流重置＋2项文件适配；即使以后主机通过，也不能代替原生服务／系统选择器。Hypium新增5项只覆盖十语／文件名／原epoch-store／截断／上限纯规则，不冒充运行原生文件或UI。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、接续与导航 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

空间恢复且取得资源窗口后，先复跑18项主机与本轮静态界面检测，再由现有离线镜像入口构建应用／测试HAP；设备和真实HTTPS必须另行验证原始意图、选择器取消、写失败、RCP断开与当前身份保存。当前没有HDC目标，不为此下载SDK／模拟器或放宽系统信任。所有243主验收状态保持原样。

[实现入口](../../../../../代码/前端/HarmonyOS/README.md#harmony-personal-export) · [外置测试](../../../../../测试/前端/HarmonyOS/README.md) · [后端合同十一节](../../../../../代码/后端/Go/接口契约/README.md#personal-data-export) · [主机入口](../../../../../测试/前端/HarmonyOS/Unit/RunExport.mjs)

<a id="🔚" href="#前言">回到首页</a>
