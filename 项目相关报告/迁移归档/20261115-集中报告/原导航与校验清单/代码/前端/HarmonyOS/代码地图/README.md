# HarmonyOS 代码地图

| 入口 | 职责 |
| --- | --- |
| [EntryAbility](../entry/src/main/ets/entryability/EntryAbility.ets) | Stage Ability 与窗口入口 |
| [Index](../entry/src/main/ets/pages/Index.ets) | ArkUI 原生页面、身份与连接、CA核对、聊天／分页来源定位、提及选员、群公告原操作及设置 |
| [Models](../entry/src/main/ets/Data/Models.ets) | HTTP对象、可信连接档案、签名查询路径、原提及载荷／未读投影及无正文公告意图规则 |
| [Store](../entry/src/main/ets/Data/Store.ets) | 系统RDB、正式／演示分库、消息／草稿／游标／未读／原操作及资格失效收尾；提及回执与正文／游标同事务、旧缓存观察迁移 |
| [Api](../entry/src/main/ets/Data/Api.ets) | 严格RCP HTTPS、JSON、原头像multipart与当前鉴权下载 |
| [NativeServices](../entry/src/main/ets/Data/NativeServices.ets) | AssetStore绑定凭据、无正文通知与用户显式系统外部预览 |
| [Strings](../entry/src/main/ets/Data/Strings.ets) | 十语控件词条 |
| [项目配置](../build-profile.json5) | 产品目标、模块与构建模式 |
| [模块配置](../entry/src/main/module.json5) | Ability、设备范围、INTERNET 权限和页面资源 |
| [外置测试](../../../../测试/前端/HarmonyOS/README.md) | 22项Hypium源码与应用／测试HAP、21项实际主机规则、设备模式及未验边界 |

只定位本原生工程；当前已取得镜像SDK应用／测试HAP编译证据，设备未验，详见[工程 README](../README.md)。

[工具链](../工具链/README.md)管理镜像实际配套与用户级缓存准备；构建和测试入口支持ASCII源码快照，当前成功构建与保留的历史失败均见测试记录。


提及与公告没有新增Web容器：仍为同一ArkUI入口的原生子视图，消息来源使用contentVersion，原操作只保存引用。最新[限定证据](../../../../测试/前端/HarmonyOS/报告/20261115-提及公告与分页/README.md)分别记录主机规则、两类HAP及设备／HTTPS未验，不将本机提及读点当作跨端ACK。
