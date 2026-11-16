# HarmonyOS 提及、公告与分页切片

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [已验证](#jobs-toc-heading-1)
- [本轮范围](#jobs-toc-heading-2)
- [未验边界](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">已验证</span><!-- jobs-toc:heading-end -->

2026-11-15真实完成21项主机纯规则测试，21通过、0失败；当前应用HAP及包含22项Hypium用例的外置测试HAP均编译成功，均未签名。[结果与摘要](./结果.json)记录两份HAP、实际Ark字节码、构建日志／源码快照与8份当前ArkTS源码的SHA256；[主机输出](./主机规则结果.txt)保留实际执行结果。

- [应用构建日志](../20261115-115419-32625/运行日志.txt)及[源码快照清单](../20261115-115419-32625/source-manifest.json)。
- [外置测试HAP日志](../20261115-115449-33701/运行日志.txt)及[源码快照清单](../20261115-115449-33701/source-manifest.json)。

工具链复用已存在的libsdl镜像CLT6.1.1.280、SDK6.1.1.125／API24与Hvigor6.24.2。没有新下载SDK、安装全局CA或修改系统代理；旧13用例HAP、旧主机输出与发送准入证据保持原样。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">本轮范围</span><!-- jobs-toc:heading-end -->

当前成员原生选择／群主全体、草稿与原Outbox提及字段、按消息ID去重的本机提及回执与已读清除、已有未读分批权威重投影；公告仅引用源ID／contentVersion，用户选择无期／1小时／1天／7天，确认快照与原请求先持久，再查原operation，完成后刷新当前。current=false不回滚，未知结果不放弃，来源失效／过期不显示旧正文；待办不复制聊天正文。冻结原消息通过只读message-results核对，不从404降级到创建型发送。根代理先前新增的独立收藏／搜索分页、精确来源定位也已进入本次两份HAP。

21项主机测试执行当前Models／Strings纯规则，包括新结构化字段、代际／角色限制、未读去重、内容版本、无正文原公告请求、清除与期限；22项Hypium只完成源码原生编译，不是22项设备执行通过。十语当前144键，三主题沿用原生实现。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">未验边界</span><!-- jobs-toc:heading-end -->

HDC只读枚举返回`[Empty]`，没有授权目标；未安装未签名包，没有ArkUI截图、鸿蒙真实HTTPS联调或设备Hypium结果。系统RDB事务／重启及断电耐久、后台／通知、AssetStore、媒体预览、三主题／全部语言与字号设备组合仍待原生运行验证。前台分批提及投影不是跨端已读，未发送qualified ACK，未开启清云、厂商推送、RTC或E2EE；不变更243项验收状态，不宣称全I完成。
