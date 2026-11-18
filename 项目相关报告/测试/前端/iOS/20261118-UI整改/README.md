# <span id="前言">JobsIM iOS界面整改与当前验证</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、十二条对应](#twelve-items)
- [二、账户、草稿与材料](#identity-materials)
- [三、当前证据](#actual-checks)
- [四、当前待核](#remaining-validation)

</details>
<!-- jobs-toc:end -->

---

当前覆盖2026-11-18用户十二条整改，以及随后确认的登录、独立设置、真实Mac演示、单浮标、主题、资料与统一空态要求。正式[需求合同](../../../../../需求/IM需求明细表.md/IM需求明细表.md#ios-native-ui-contract)直接定义最终行为；本页记录实现位置、冻结输入和实际验证。原目录／签名轮的App不包含本轮改动，不能据其安装成功认定新UI已运行。

登录页以用户随后确认的八项要求为准：卡片禁止手势上下滚动，键盘仅程序避让；侧轨使用18pt文字；副标题统一“Jobs荣誉出品”；记住密码和眼睛使用19pt图标，保留44pt触控区域；验证码由服务端挑战决定；重新验证身份归登录设置；卡片不显示内部操作核对入口。当前冻结候选为build31，118项隔离单元已通过；最终UI、成品与手机证据分列如下，不以旧包成功代替当前源码验证。

| 登录要求 | 当前行为与证据边界 |
| --- | --- |
| 卡片不随手势上下移动 | 禁用用户滚动与回弹；保留聚焦字段的程序避让；Next逐项输入，最后Return仅收键盘、不提交账户请求 |
| 侧轨与品牌 | “新用户注册／返回登录”18pt半粗体，登录／注册副标题统一“Jobs荣誉出品” |
| 圆圈与眼睛 | 19pt符号配置；圆圈按钮至少44pt高，密码及确认密码眼睛显式44×44pt；图形和点击范围分开设置 |
| 双重验证 | 卡片无启用开关；服务端明确拒绝并要求验证码后展示输入，未知原请求保留原凭证与requestId，不用新OTP替换 |
| 身份验证与技术恢复 | 齿轮设置内唯一身份验证入口；原改密／注销待核时只查原申请，按需提供原口令证明；404不创建新操作。卡片无技术核对菜单，真实待核申请的完整UI网络链仍待专项验证 |

## 一、<span id="twelve-items">十二条对应</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

下表其余九条本地演示流程采用build22／UI16的冻结证据；最新登录整改及材料／真实Mac登录采用build31／UI20。两组范围分开记录，不把旧结果算作新候选全量回归。

| 项目 | 当前源码行为 | 验证与待核 |
| --- | --- | --- |
| [App图标](../../../../../代码/前端/iOS/JobsIM/Resource/Assets.xcassets/AppIcon.appiconset/Contents.json) | 1024不透明原Logo衍生图；Debug／Release与生成器绑定AppIcon | 尺寸、Alpha与签名成品AppIcon绑定已核；手机主屏目视仍待核 |
| [AppDoor登录门](../../../../../代码/前端/iOS/JobsIM/UI/IMAuthVC/IMAuthVC.swift) | Style1卡片及双侧切换；图片／视频背景、毛玻璃中卡、描边输入、居中标题、密码眼睛、记住密码；齿轮推独立登录设置 | 当前系统UI已过认证控件布局、显隐切换与登录／注册换边；视频实播和完整真机布局仍待核 |
| [品牌与演示标记](../../../../../代码/前端/iOS/JobsIM/UI/IMDemoEnvironmentOverlay/IMDemoEnvironmentOverlay.swift) | 根页隐藏重复标题；演示只保留可拖拽、点击退出到登录的单浮标 | 当前系统UI已过拖拽、首屏可见及真实演示退出；不自动切回正式账户 |
| [三Tab与个人中心](../../../../../代码/前端/iOS/JobsIM/UI/IMRootVC/IMRootVC.swift) | 联系人／消息／当前用户名头像；收藏设置及账户切换收进个人中心 | 当前UI已过三Tab顺序、个人中心与收藏设置入口；真实多账户切换完整矩阵仍待核 |
| [联系人关系与举报](../../../../../代码/前端/iOS/JobsIM/UI/IMChatSettingsVC/IMChatSettingsVC.swift) | 点联系人直聊；拉黑保关系；确认／侧滑删除；选择图片视频与多条聊天举报 | 当前UI已过本地演示拉黑保好友、删除取消及系统图片选择→私有副本→预览关闭→移除→再选→提交；完整真实材料上传至后台另验 |
| [设置深色cell](../../../../../代码/前端/iOS/JobsIM/UI/IMUI/IMUI.swift) | 导航、cell容器、主副文字及选中态统一语义色；移除模板强制浅色的本地绑定 | 当前UI已过白天／黑夜／跟随系统与返回个人中心；截图目视与系统UI证据分列 |
| [消息页入口](../../../../../代码/前端/iOS/JobsIM/UI/IMConversationsVC/IMConversationsVC.swift) | 移除编辑／新建对话按钮 | 源码已核，当前消息Tab导航流程通过 |
| [收键盘](../../../../../代码/前端/iOS/JobsIM/UI/IMChatVC/IMChatVC.swift) | 点击输入框外及交互滚动收起；不吞cell动作 | 当前系统UI已过聊天输入与点击非输入区域收键盘 |
| [聊天独占](../../../../../代码/前端/iOS/JobsIM/UI/IMChatVC/IMChatVC.swift) | 聊天push隐藏Tab，返回恢复；单聊使用已获权对方昵称，群聊保群名 | 当前本地演示及真实Mac聊天导航通过；两项标题隔离单元通过 |
| [头像与资料](../../../../../代码/前端/iOS/JobsIM/UI/IMUserProfileVC/IMUserProfileVC.swift) | 长按头像进资料；他人只读；本人点头像设置静态／GIF，昵称／签名可改，UID／注册时间和联系状态只读，保存置底 | 当前UI已过他人只读、本人头像入口、资料字段与完整保存按钮滚动边界；真实资料／GIF后端另有证据，真机GIF上传播放仍待核 |
| [消息动作与草稿](../../../../../代码/前端/iOS/JobsIM/Services/IMMessageEditingService/IMMessageEditingService.swift) | 回复、收藏、置顶、撤回、举报、先撤回再编辑；覆盖／保存及可恢复草稿 | 当前UI已过保存现有草稿再载原文并恢复旧草稿；真实撤回冲突／丢响应矩阵仍待核 |
| [通讯录组织](../../../../../代码/前端/iOS/JobsIM/UI/IMContactsVC/IMContactsVC.swift) | 滚动搜索、新朋友／群聊／星标、普通好友及🌟A～Z#索引；独立添加／搜索页 | 当前真实Mac两好友与单聊可见；同步通知合并刷新、命名空间与任务代次等6项单元通过，长列表索引边界仍待核 |

## 二、<span id="identity-materials">账户、草稿与材料</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

账户切换先悬置旧选择，停止同步并隔离迟到结果；保留原Keychain与Outbox，经目标当前实例／家族／恢复代际和GET me验证后才发布。用户名修改有稳定原requestId和expectedUsername，服务器确认当前值后更新身份绑定，不修改旧未知认证请求。演示资料和关系只落演示命名空间。

编辑只对本人可撤回文本开放，成功撤回后回填原文；左覆盖、右保存，保存的是原输入草稿并提供恢复，不备份被撤回正文。举报仅包含显式所选材料；私有副本在ReportDrafts／ReportOutbox下按命名空间隔离，未知上传查原8字段责任，注销按登记空间精确清理并拒绝迟到写入。后台明细按当前源权限取选中消息，不复制整段会话。

## 三、<span id="actual-checks">当前证据</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- [认证／导航静态切片](./AuthRoot/README.md)：Swift语法、十语键集、静态颜色、图标出处与源摘要。
- [联系人／聊天](./ContactsChat/README.md)、[后端隔离回归](./Backend/Resume/20261118T062611Z/summary.json)、[Web后台证据](../../Web/20261118-管理员举报证据/README.md)分别封存。后端此前27项隔离回归及vet通过；Web此前42组浏览器、16条协议及类型检查通过，不能代替iOS新候选的集成验证。
- 最新冻结[185项输入与八项模拟器产品摘要](./Shared/current-candidate-build31-inputs.json)，[build-for-testing-31](./Shared/build-for-testing-31.log)与[device-build-9](./Shared/device-build-9.log)均通过；再次核对输入零漂移。签名成品[JobsIM.app](./Shared/成品/device-build-9/JobsIM.app)含105文件／56,367,357字节，[严格验签](./Shared/device-build-9-codesign-verify.log)通过，[产品清单](./Shared/device-build-9-product.json)逐文件封存SHA。Bundle ID与开发团队保持。
- [最新隔离单元](./Shared/unit-tests-5.log)118／118通过、0失败，11个本地白名单类在host启动前注入，不使用用户默认9443实例。新增5项MFA拒绝、未知原请求及原证明保留反例已实际运行；此前6项联系人／快照恢复与2项单聊标题包含在118内，不与旧105／111／113累加。
- [同候选系统UI20](./Shared/ui-tests-20.log)4／4通过、0失败，107.88秒；[附件清单](./Shared/ui-tests-20-attachments/manifest.json)保留截图、系统预览画布与材料元数据。四条覆盖登录控件布局／44pt触控、禁手势上下滚动、眼睛显隐／记住状态、设置唯一身份验证入口、注册Next与末尾Return收键盘、登录注册换边；另经系统选图→私有副本→实际画布渲染→关闭移除再选→提交，以及专用Mac开关预填后普通API登录、昵称单聊、唯一浮标退出。六张实际截图已目视复核，静态截图不代替交互断言。
- 较早[build22／UI16](./Shared/ui-tests-16.log)10／10通过及其[截图](./Shared/ui-tests-16-attachments/manifest.json)仅证明该冻结输入的九条本地演示与一条真实Mac流程；其关系、草稿、主题、资料等未全部在最新UI20重复。最新四项不与旧十项相加。
- 最新手机安装[第一次尝试](./Shared/device-build-9-install.log)返回CoreDevice 4016；[设备信息](./Shared/device-details-current-6.log)显示unavailable，已请用户重连并保持解锁。device9尚未安装；手机此前[device7普通启动](./Shared/device-build-7-launch.json)及[60.284秒PID观察](./Shared/device-build-7-stability.json)不包含最新八项，不用于证明新包启动。当前手机后续结果以本页及summary更新为准。
- Mac独立原生演示保持ready，专用HTTPS控制面／网关、双物理PG、NATS、九角色和Web以普通API预埋非管理员账号、好友、单聊、群聊和消息。公开Bundle描述不含密码或私钥。[后端资料与原生演示证据](../20261118-资料与原生演示/Backend/README.md)分列30业务Go／4控制面方法／25 HTTPS等冻结切片；后续最小快照SQL另有真实PG修前红→修后绿和9次真实schema2请求，17投影均可安装，未把原30方法冒认全量重跑。[末次只读核验](./Shared/native-demo-final-readonly-review.json)确认236生产输入与原PID无漂移，严格CA状态／bootstrap为200。
- 原失败与旧构建保留：UI19暴露眼睛实际宽37.333pt后，显式固定rightView的44×44pt并指定小符号配置，UI20通过；build27～29的Swift 6隔离／显式self错误均由build30、31实际编译纠正。UI11～15原失败分别用于修复浮标层级、空态副标题可访问性、中文设置按钮、材料行裁切与预览关闭、同步自取消／不可安装快照重试，以及真实单聊UUID标题；严格快照校验未放宽，未清模拟器数据规避错误。旧失败、旧device5／7与UI8等只证明各自输入。
- [最终CodeGraph增量同步](./Shared/codegraph-sync-auth-final.log)完成3项变化／97节点，[当前密码框源码查询](./Shared/codegraph-query-auth-final.log)可用；37未解析引用仅为末次增量范围，不作为全库覆盖量。需求243主项编号、范围及未验收状态保持；十语均449键且集合一致。正式文档直接定义最终行为，资产盘点分列源码已实现／实际已验／待核，检查见[文档同步记录](./Shared/文档同步/summary.json)。

## 四、<span id="remaining-validation">当前待核</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

仍待最新签名包的手机安装启动、完整真机UI与后台生命周期、真实管理员MFA挑战UI、原待核改密／注销的纯查询UI网络链、真机GIF上传播放、背景视频实播、真实多媒体举报交付至后台、长联系人索引边界及多账户实际切换矩阵。动态图片服务端处理／HTTP和客户端系统选图各有证据，不能相互替代。全部243项与五端互通未据本轮验收；用户默认9443实例未作为测试环境。

<a id="🔚" href="#前言">返回前言</a>
