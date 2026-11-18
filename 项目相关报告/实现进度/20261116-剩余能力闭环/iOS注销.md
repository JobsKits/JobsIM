# JobsIM iOS 本人注销与本机清理

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [前言](#%E5%89%8D%E8%A8%80)
- [一、入口与责任](#jobs-toc-heading-2)
- [二、本机清理与门禁](#jobs-toc-heading-3)
- [三、实际验证与边界](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

## <span id="前言">前言</span>

2026-11-17，本轮新增[注销服务](../../../代码/前端/iOS/JobsIM/Services/IMAccountDeletionService/IMAccountDeletionService.swift)、[文件清理](../../../代码/前端/iOS/JobsIM/Services/IMAccountDeletionService/IMAccountDeletionFiles.swift)及[原生表单](../../../代码/前端/iOS/JobsIM/UI/IMAccountDeletionVC/IMAccountDeletionVC.swift)，与现有设置和认证页接线。本报告分清源码、编译、模拟器本地执行与真实网络，不把局部通过称完整第一阶段验收。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、入口与责任 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

- 设置→注销本人账号，登录页先填原用户名→核对注销。申请前确认，密码／恢复码二选一和管理员OTP只在内存；原申请核对使用原密码，受限查询／取消重新证明本人，恢复码消费由服务端负责。后台或页面离开取消任务并清空输入。
- 原请求、设备、family、恢复代际先持久于独立协议集合。未知申请先只查原结果，显式重试保留原体；后继family不能首次创建旧单。无普通登录仍能核对原申请、按目的重新证明查询／取消；取消未知不换编号，重新核对所见版本后才允许同nonce重试。
- 公开收据采用 `JobsIM-Deletion-1:`，仅用户名、实例与原单公开上下文，不含密码、proof、令牌、本地路径或family。导入只登记原责任，不能冒充登录或把查不到解释为创建授权。
- 原单列表分页、完整无记录／重载；十语、三主题。截止时间与状态以服务端原结果为准，不在客户端宣布冷静期已完成。成功撤销后也需重新登录。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、本机清理与门禁 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[自有存储](../../../代码/前端/iOS/JobsByPods/JobsIMStorage/Core/IMLocalStore/IMLocalStore.swift)新增明确账号归属登记、不可逆本地拒绝事实与私有导出暂存归属。服务端completed先耐久，再对精确profile／deployment／user所有已登记epoch和设备命名空间清理；消息、Outbox、同步页／原引用／资格、头像原单、私有媒体缓存和登记的导出暂存一并处理。外部Files已保存文件、别账号、别profile和Demo不动。

清理失败保留completed原责任与本地拒绝事实，启动／连接／页面重载继续，不以失败清理恢复业务；SQLite安全删除与WAL截断失败不标本机完成。秘密存储清理只匹配该账号；迟到认证在写Keychain前再次检查拒绝事实，已清理命名空间拒读拒写。早期无部署字段的合法旧namespace还按精确已登记key拒绝；此最后一项增强已进入[78项新候选](./iOS改密/注销群控78结果及冻结.json)，0失败／跳过，输入逐字节一致；77原件保留，不追认旧结果。

专用HTTPS账户传输使用临时无Cookie／无凭据缓存会话、禁止重定向、固定路由／报文上限、可取消请求和本实例CA；错误不能通过迟到Set-Cookie清掉新登录。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、实际验证与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[77项结果及输入冻结](./iOS改密/注销77结果及冻结.json)对应当前注销首候选：应用与测试包真实编译，7类77方法在专用模拟器执行，77通过、0失败、0跳过，103份自有输入跑前／后相同。包含原71基线、3群控和3新增注销反例：依赖403仍保unknown／不借后继family／证明不落盘；取消目的错误零请求、丢响应原nonce与版本续行；所有epoch本地拒绝、迟到写／认证绑定拒绝、其他账号／profile保全、WAL无已删原体。原71中的改密真实HTTPS方法仍在本轮执行；这不代表注销已真实HTTPS或表单点击通过。

原始[编译](./iOS改密/注销第二轮编译.log)、[测试日志](./iOS改密/注销77.log)、[结果包](./iOS改密/注销77.xcresult)保留。首轮仅测试源码async断言编译错误，修为先await再断言后通过，失败日志未改。模拟器结束已为Shutdown。管理员TOTP、恢复码真实消费、注销真实HTTPS、系统后台／文件选择器／真机、完整表单目视、普通迁移另验。

<a id="🔚" href="#前言">返回前言</a>
