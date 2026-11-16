# <span id="前言">iOS 本人状态与云草稿原生界面</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、增量资产与边界](#jobs-toc-heading-1)
- [二、验证记录](#jobs-toc-heading-2)
- [三、已发现并保留的失败](#jobs-toc-heading-3)
- [四、当前未封存检查点](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、增量资产与边界</span><!-- jobs-toc:heading-end -->

本轮接续[原S2的21同步＋12认证本地结果](../20261116-S2本人状态/README.md)，旧结果不追认新源。原修改前6项输入逐字节保存在[源改动前](./源改动前/输入SHA.json)，旧产品目录未覆盖。

| 入口 | 行为／默认值 | 存储与生命周期 | 源码与差异 | 验证 |
| --- | --- | --- | --- | --- |
| 聊天“我的同步状态” | 手动未读、pin／mute／archive／hidden；独立阅读与本机缓存读数 | fresh本人GET＋CAS，原nonce／body先GRDB，晚family／guard拒绝 | IMPersonalStateService／VC、IMSession、IMLocalStore | 已有冻结包30本地与2真实业务；DEBUG交接随新导出包编译；31原S2与10导出同轮本地通过，完整UI待复验 |
| 会话列表“会话显示” | 默认不显示归档／隐藏；独立恢复筛选；按原pinnedRank排序 | 仅应用本人投影，展示过滤不删除消息或改授权 | IMListVC | 映射反例包含在30本地；完整UI仍未通过 |
| 云草稿保存／清除 | 新da1与原CAS；回复用源contentVersion／独立dr1；无quote | 原完整body先落盘；未知只读查单，明确同原体重试；prepared待核、retired保文字 | 原self-state-operations；结果不含旧正文 | 真PUT舍弃回执后只读原单／真实retired保文字已通过 |
| 载入／恢复 | 非空本人文字、明确确认；默认不带旧引用，可单独选择重新核对的回复 | 本地text／reply／@快照CAS；不自动覆盖、不发送、不改正式Outbox | IMChatVC＋状态页；无编辑器时分享独立文字，不导出许可 | 早轮实际UI到采纳，整链未通过；不追认失败轮 |
| 设置状态入口 | 查看待核／拒绝记录与本人保留草稿 | 原family绑定；恢复捕当前guard，原body不改；页面离开取消任务 | 原设置／字号／可见ID观察沿用 | 十语184键一致；字号真实UI最终复验待完成，全主题未实看 |

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、验证记录</span><!-- jobs-toc:heading-end -->

所有构建／测试输出使用唯一子目录，4GiB硬门禁、单worker，原始失败不覆盖。原生与真实HTTPS只按下文实际分轮范围记录；未跑真机。工具入口与隔离范围见[本地包装说明](../../../../../测试/前端/iOS/personal_state.py/README.md)。

原envelope摘要、完整manifest／finalvalidate／首次changes、S2事务cursor和原Outbox只读proof不作降级。qualified ACK／after_receipt、正式迁移、注销等仍依原I台账。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、已发现并保留的失败</span><!-- jobs-toc:heading-end -->

新产物目录首次冷依赖构建428.6秒通过，后续增量分别独立保存。新增9反例与原21同步共30在原生模拟器通过；实际HTTP业务两项也已多轮通过，各自保留包和输入范围，不累加。

实际界面第一轮完成view五项与草稿保存／明确采纳，末尾少一次返回而找不到Settings；第二轮实际点到Large却服务端仍standard，定位`choose`先重建根控制器、后排队weak-self任务。产品窄修为S2字号写成功再由原偏好应用路径刷新；本机字号／主题／语言仍走原本机设置。再轮遇系统AX旧行的无限frame，夹具改为重取恢复列表行并核有限可点击区域，不点不可见控件。

一次跨XCTest阶段UserDefaults journal仅读回UI两条，未重建或虚称六条；之后测试改原子Documents公开ID文件。各失败轮已执行关闭，能取得的六条完整事实另存；首次四条业务关闭由通过方法中的真实logout／原AT401证明，不补造丢失的逐ID记录。

真实草稿丢回执场景是原体先入GRDB后真PUT并舍弃已收到的响应，再用新连接／新session只读查单；原连接仍在，不能称整进程断电或真实网络丢包。retired原体由夹具冻结后真实编辑来源，当前文本与显式恢复另经真GET核对。稳定最终结果待本轮后续封存，不把中间2真过或过渡截图扩成完整I。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、当前未封存检查点</span><!-- jobs-toc:heading-end -->

`Build-20261116-182322-d0b761`为本次公开档案交接修补前的冻结包；`Test-20261116-182952-5122e6`实际30／30本地通过。新独立模拟器不冒充被外部清理的旧设备／Keychain恢复。

`Real-20261116-184537-fe3e3b`真实业务2／2通过；随后准备UI成功，但`Ui-20261116-184627-7af057`在首个会话入口失败。只读观察到GRDB、Keychain、Documents保留而最新UserDefaults UI档案键未进入后续进程；不将这一现象推断为具体系统清理机制。最新源码改用限于DEBUG、固定`s2-UUID`文件名与公开字段白名单的原子plist交接，并新增本地反例。该增量后续已随[正式导出Build-205705](../20261116-正式本人导出/Build-20261116-205705-4c8f38/结果.json)完整编译；31原S2与10导出共41在冻结89e／94a1包已原生本地通过，0失败／跳过；仅使用本端既有sim并已关闭，未跑跨进程完整UI，旧30项不证明新输入。

`Cleanup-20261116-185804`仅沿原会话收尾，无新认证；UI业务5项值断言失败保留，不能写为整链通过。退出发生于原AT到期前，六条原会话均闭合；旁附[后端精确只读撤销补证](./Cleanup-20261116-185804/后端精确只读撤销补证.json)核最后一个家族revoked、active family／device均为0。当前自有模拟器已正常关闭，低于4GiB时不再构建或新建测试账号。
