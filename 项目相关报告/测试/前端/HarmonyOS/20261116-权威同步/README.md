# HarmonyOS schema1权威同步

本轮完成原生ArkTS消费者源码及离线编译，未完成鸿蒙设备验收。没有连接任何后端realm、修改SQL或启停既有服务；旧认证／转发HAP与证据独立保留。

追加兼容窄修：后端owner的sync引用revision与Message业务version独立；删除客户端错误等式，并增加不同revision／大业务version反例，主机25／25通过。原24／24及App-03／Test-01输入和结果不改写；这两份HAP尚未包含该两行修复，待资源／clean门禁解除后重编，不把当前源码追认为旧编译输入。见`OwnerRevision补充/`。

| 验证 | 结果 | 范围 |
| --- | --- | --- |
| 初轮主机 | 18通过／2失败 | 原始日志保留；发现损坏辅助原体阻挡global，并纠正SQL触发器注入位置 |
| 最终同步主机 | 24通过／0失败／0跳过 | SDK TypeScript纯模块严格检查、实际ArkTS转译；私有Python SQLite WAL／FULL适配执行Store事务，非Harmony RDB NAPI |
| 旧规则回归 | 41通过／0失败／0跳过 | 原认证、API与业务规则主机注入；不追认为设备认证 |
| 应用HAP | App-01、App-02失败；App-03通过 | 保原ArkTS错误；修禁delete、Record上下文及SyncStore接口实现，未改SDK／第三方 |
| 测试HAP | Test-01通过 | 旧29＋新增3项源码，32项仅编译，未执行Hypium |
| 设备／HTTPS | 未验 | 没有可用已授权HDC target；unsigned HAP不是安装或原生运行证明 |

新源码为[SyncProtocol](../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/SyncProtocol.ets)、[SyncEngine](../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/SyncEngine.ets)、[SyncStore](../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/SyncStore.ets)、[SyncText](../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/SyncText.ets)，并接[前台Index](../../../../../代码/前端/HarmonyOS/entry/src/main/ets/pages/Index.ets)、LocalStore、NativeServices和EntryAbility。详细入口／默认／持久化／生命周期见[工程README](../../../../../代码/前端/HarmonyOS/README.md#权威同步schema1)。

主机反例覆盖：完整页与首次changes之前零正文发布；原字节哈希／外层对象矛盾；缺页、权限ABA、分类水位、精确大计数、连续位置；原nonce复用；peer同CID不确认；辅助503／坏200／损坏原体零proof而global仍落地；原route／sender／body hash与guard CAS；事务失败回滚；低版本global胜过新正文；同来源旧device／epoch持有者清理且不跨用户；forward许可和正文／公告来源不持久；清理失败续跑；能力消失不回退；已注册物理UUID缺失拒绝重建。

原先epoch:seq消息历史轮询改为schema1权威安装；原旧缓存及未决队列不静默迁移或自动发。合法普通历史只在当前身份、来源、epoch已核且具有资格hash时显式只读访问；未知旧结构保持隔离。暂存表只含引用，快照正文在RAM并限制64MiB；终态清理包含已核holder、提及／本地未读、旧lookup隔离副本、私有媒体缓存和通用通知意图。已有用户显式导出或外部预览交出的文件不宣称可回收。

消息同步不是ACK。只有原消息路由只读解析的完整200，且本人sender／CID／conversation、原body hash及同事务guard匹配才确认；未知和坏200不销队列，自动消费者不创建消息。schema1不包含服务端已读、云草稿、偏好；schema2、qualifiedACK、after_receipt、正式迁移及全I容量仍未交付。

每次Hvigor限非并行／memory策略，180秒有界，Data空闲4GiB硬门禁；本轮未触发门禁。`App-*`／`Test-*`保构建日志、退出码和ASCII快照输入SHA；`主机/`保各轮日志和输入。上一认证Dist原字节在`上一认证Dist/`并有独立SHA，新Dist及当前输入另封，不覆盖旧基线。
