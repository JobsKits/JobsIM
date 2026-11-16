# CLI schema2本人状态

当前独立切片的本地验证已完成。首轮`01-local`通过53顶层本地组；最终`02-final-local`55组通过（旧45＋新10），0失败，真实HTTPS与OS vault两项按范围跳过。`03-build`完成MacOS arm64候选构建，SHA256为`665c470690468da04bb93030b468a4d49408d818868f19900e4296a2c5a36c94`；只新外置Products，不覆盖旧binary。未据此前53组追认最终输入。

新增10组执行了真实SQLite／注入HTTP：自动S1→S2和保原身份／Outbox；归档事务故障回滚；owner引用revision与对象version分离及本人边界；默认空草稿与临时许可剔除；prepared原body续查和current=false不改本地输入；自动查单404零创建与原体摘要；本地草稿采纳CAS；原观察集合不能用最大sequence冒充；原method／route／requestId围栏；隐藏视图与all恢复入口。不是实际JobsIM S2 HTTPS／PG验证。

当前只Jobs CLI自有源码／外置测试／本端文档，未连接S2服务、修改PG或启停realm。旧S1 binary、原报告和跨平台产物保留。Go1.27.4，GOMAXPROCS=2、-p=1、GOPROXY=off；复用本任务幸存的AppSupport私有Go Cache，Products／Temp／报告独立。每进程180秒及空闲4GiB门禁。

入口／行为／默认／持久化／生命周期见[CLI README](../../../../代码/前端/CLI/README.md#schema2本人状态与云草稿)。重要区别：协议S2不改变本地schema4、物理UUID或认证32hex绑定；只允许专用事务封存旧流后重新快照，普通更新不得更换nonce。新增metadata的owner引用revision不等于对象自身version。视图影响会话默认排序／隐藏，`conversations all`保留恢复入口；字号是本人云偏好，不伪称改终端字体。

所有本人写意图先耐久保存原method／route／body／hash；未知先只读原结果，明确动作才可按原请求续跑一次，prepared非成功、retired冻结。普通local draft不被云投影或旧成功自动覆盖，明确采纳按当前本地原文CAS。history／sync没有自动read观察，正式消息也不会因相同clientMsgId或云草稿变更销队列。

首轮本地不包含S2真实HTTPS，后续独立真实轮见下文；Mac原生安全库正向与Linux／Windows新源码运行仍未验。旧fixed57只证明S1，process-only认证不替代OS securestore；qualifiedACK／after_receipt继续未实现。

## 后续真实轮与当前窄修

`HTTPS-01`通过前5项后发现read的独立server operation UUID被误当requestId；`HTTPS-02`通过前6项后发现draft同类兼容。原失败保留。首轮活跃device检查误用`/api/sessions`（正确为`/api/auth/sessions`），其false不等于撤权失败；TTY均实际写signed_out，控制family均logout200＋原AT401。第二轮起两类撤权证据均完整。

`HTTPS-03`在587de912候选通过13／13真实TTY检查、四个family清理全部实际通过：2TTY signed_out及原device不在活跃列表，2本人API控制family logout200及原AT401。独立证明自动S2、字号本人隔离、视图隐藏／all恢复、history不自动已读、真实观察洞、手动未读独立、文本与回复草稿、远端不覆盖本地、采纳CAS、旧完成事实不回滚、源global仅失效reply与显式清空。没有注入丢响应或统计CLI HTTP次数。

追加窄修使已完成操作显式resume也纯查当前标记，防旧缓存current:true误示；旧587de13项不追认该后续输入。最终`13-currentProof-local`55／0／2范围跳过，`14-currentProof-build`及`HTTPS-04`同输入候选SHA256为`15a4c053c7d82a60c57adfea5910aa3f32e5810b3b45e0813b6e0985e6dfe1cf`，13／13真实检查通过，特别断言旧完成操作current=false及当前文本未回滚。新candidate位于工程外AppSupport独立Products，原各binary不覆盖。

`HTTPS-04`实际创建本轮两个新用户、两个PTY限程family及两个本人API控制family。退出后两PTY均有成功HTTP后才写的signed_out与原device不在活跃会话；两个control均实际logout200及原AT401。未使用PG／SQL／服务启停／配置变更，未触旧57／7ef用户，原DataRoot保留供只读核查。没有将TTY退出推断成撤权，没有记录密码／AT／RT／原TTY，没有宣称设备OS安全库正向、真实丢响应注入或CLI HTTP计数。
