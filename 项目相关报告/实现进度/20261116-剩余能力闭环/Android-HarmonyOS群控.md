# Android／HarmonyOS 群治理与邀请闭环

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、入口与实现](#jobs-toc-heading-2)
- [二、实际验证](#jobs-toc-heading-3)
- [三、限制](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

2026-11-17，两端原生群控接入当前服务端治理、邀请与审批合同。当前完成源码、主机协议／SQLite适配回归和APK／HAP编译；设备操作、原生RDB生命周期、系统剪贴板、主题实际渲染和各端真实HTTPS未执行，不以其它端联调代替。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、入口与实现 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 能力 | 入口、默认与责任 | 生命周期与权限 |
| --- | --- | --- |
| 群设置 | 群成员页→群治理；读取服务端muteAll／slowSeconds／maxMembers，默认新群为false／0／200；界面范围0～3600秒、2～10000人 | 群主明确保存；普通成员全员禁言／发言间隔豁免群主与管理员，十语有说明 |
| 成员治理 | 原成员页50项UUID游标；管理员角色、1小时个人禁言／解除、群主转让、解散 | 确认前冻结群version与actor MG／RG、target MG／RG；不再向旧ownerId PATCH转让，不自动刷新重基 |
| 独立恢复 | 仅GET governance明确canRecover时显示；选择所见target后确认 | 当前OTP换RAM step-up，绑定group.recover／原nonce／原body；不持久OTP／stepUpId，不放大非成员admin读取范围 |
| 邀请 | 管理者列表50项分页；默认1次、需审批、有效1天，可明确修改次数／审批开关；创建／撤销／复制 | 标准JobsIM-Invite-1分享码绑定deploymentId，token仅RAM／用户明确复制；不放数据库或诊断 |
| 入群／审批 | 设置→通过分享码入群；管理员审批列表50项分页、批准／拒绝 | 耐久仅nonce＋inviteId；重启重输同码复用原单，待审批明确区别已批准；后端独立核原issuer四代际 |
| 原操作 | 本人操作列表，20项展开，按当前群筛选；空态与重载 | GET原nonce先查；只有原404且明确重试才POST／DELETE原body；原namespace（含device）与原family必须精确匹配；旧行缺family不补资格，仅查原结果。旧epoch只查，未知不换nonce，不把403当未创建 |

Android实现为[GroupTools](../../../代码/前端/Android/app/src/main/java/com/jobs/im/GroupTools.kt)、[GroupScreen](../../../代码/前端/Android/app/src/main/java/com/jobs/im/GroupScreen.kt)、[GroupText](../../../代码/前端/Android/app/src/main/java/com/jobs/im/GroupText.kt)，通过ChatModel与现有设置／成员页接线。原责任保存同一Room的sync_records/group-operation，不加实体版本。

HarmonyOS实现为[GroupTools](../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/GroupTools.ets)、[GroupStore](../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/GroupStore.ets)、[GroupText](../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/GroupText.ets)及Index原生页；公开原责任存group_action_intents，复用LocalStore串行SQL/CAS。注销completed将该表按精确本人前缀清除，其他账户及独立注销责任保留。

两端沿既有白天／黑夜／跟随系统与十语37键，列表有完整Logo空态／重载。页面离开／后台清输入与RAM邀请；重新加载发现失去管理权限也清邀请。晚到身份／页面／family不发布结果；Harmony剪贴板回调同样检查生命周期。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、实际验证 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

- [Harmony群控主机](./群控证据/HarmonyHost-2jjNoL/结果.json)：20／20、0失败／跳过；严格分享码、原actor／target代际、未知纯查／原体重试、同码冷启动续单、prepared后403、旧epoch、分页、恢复step-up仅RAM、迟到身份、角色撤回、同epoch后继family／device仅只查、旧缺family只读、revoke／decide强类型原结果和当前GroupStore实际SQLite/CAS／重开／账号隔离。HTTP／RDB由主机适配，不是设备验证。
- [注销清理主机](./注销证据/HarmonyHost-x8e2Rn/结果.json)：12／12，新增group_action_intents本人／他人隔离清理断言。
- Android当前JVM全量28／28，新增GroupRules3项；应用／test APK已编译。新增GroupLifecycle7项以及改密6项、注销5项Room适配源码均进入测试包，仅编译未执行。
- Harmony应用与54方法test HAP已编译，unsigned，未安装／执行Hypium。新增4项覆盖十语、秘密不落原责任、精确原route及只读权限。

失败不覆盖：首轮Harmony原生编译3处非空Record字面量不符ArkTS约束，见[首次原始日志](./群控证据/Harmony首次/构建.log)，改为显式赋键后应用／测试编译通过。首轮群控主机15过／1失败为夹具next属性只有setter，测试比较了undefined；修夹具为独立cursor后16项通过，再增角色撤回为17项。旧失败、旧候选与Dist均保留。

最终输入／包摘要见[最终冻结结果](./群控证据/最终冻结.json)；只读交叉审确认原family缺口后已补原namespace＋family门禁，强类型revoke／decide结果及申请者实名；20项主机已过，该改动应用／测试APK与应用／54方法test HAP的窄编译全通过，124份当前编译源码／资源与快照逐字节一致，包CRC有效，7个Android群控方法／54个Hypium方法名进入DEX／ABC。当前[最终冻结](./群控证据/最终冻结.json)保全SHA、实际日志与非编译文档后改说明；原family补围栏前候选单独保留，不追认新源。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、限制 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

未启动AVD／HDC设备、未连接用户实际账号，未改CA或清缓存／历史数据；不把后端issuer四代际PG结果算作端侧真实网络执行。独立恢复还依赖操作者已在当前部署明确授予的独立本地grant；没有授权时客户端只显示普通权限。普通端迁移由独立协议继续实现，不混同群邀请或复制旧同步资格。

未提交／推送。

<a id="🔚" href="#前言">返回前言</a>

2026-11-17真实Android补证：Android当前7个GroupLifecycle原生方法全部实际通过，API／step-up为受控注入，真实群治理HTTPS、管理员grant及全主题页面另验；Harmony20host按原冻结范围成立。同一当前9660aa38／7b3f14b4双包的[105逐方法记录](./普通迁移两端证据/Android独立AVD-20261117-153544-d52437/最终105逐方法实际核对.json)列出86本地通过／16明确跳过及3独立S2 HTTPS；当前源未借旧包累加计数，私有5580已停并保数据。
