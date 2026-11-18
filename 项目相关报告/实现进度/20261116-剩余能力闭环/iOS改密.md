# iOS 本人主动改密

[当前闭环进度](./README.md) · [原始证据](./iOS改密/)

2026-11-17，本候选已接设置页新改密及认证页原单恢复。沿用白天／黑夜／跟随系统与十语，密码、确认和验证码只存本次界面内存，退场清空；耐久记录只保存原nonce、原账号／设备／family／实例／epoch与状态，SQLite事务拒绝混入秘密字段。未知／处理中／已成功的原family均停止普通业务及刷新；终态和迟到结果只影响原family，保留后来登录的独立family。

原单结果通过专用短超时HTTPS传输查询，禁用Cookie、共享凭据、缓存和重定向；首次新写只带已经复核的原family AT，已准备的原单显式续跑可匿名证明，纯查不创建操作。撤回能力或恢复epoch变化后保留匿名纯查，拒绝继续写入。新密码按NFC验证15–128个Unicode标量、最多512 UTF8字节，保留空格。

当前验证：[完整71项结果](./iOS改密/71项结果摘要.json)零失败／零跳过，包括15认证协调器、3整对、21权威同步、10正式导出、10本人状态、3会话／对比度、9物理存储方法。其内一项使用**原MacOS部署入口构建并启动的真实HTTPS、独立双PG、JetStream和生产角色**，通过指纹核对的实例CA严格握手，真实注册→本人改密→后台结果→原AT及密码拒绝→新密码登录→旧原单查询保留新family→真实退出；端存储是临时物理SQLite，不是内存数据库。单独首轮真实HTTPS也通过，旧47／49项候选保持各自原件，不累加为新增通过。

主要源码：[服务](../../../代码/前端/iOS/JobsIM/Services/IMPasswordChangeService/IMPasswordChangeService.swift)、[界面](../../../代码/前端/iOS/JobsIM/UI/IMPasswordChangeVC/IMPasswordChangeVC.swift)、[受限传输](../../../代码/前端/iOS/JobsByPods/JobsIMTransport/Core/IMInstanceAgent/IMInstanceAgent.swift)、[本地CAS](../../../代码/前端/iOS/JobsByPods/JobsIMStorage/Core/IMLocalStore/IMLocalStore.swift)、[认证门禁](../../../代码/前端/iOS/JobsIM/Services/IMAuthCredentials/IMAuthCoordinator.swift)、[外置测试](../../../测试/前端/iOS/IMAuthCoordinatorTests.swift)。工程挂载保留原有用户改动，新增两份App源，无需改第三方或重新安装Pods；十语各221键。输入和App／测试／实际服务器二进制摘要见[冻结清单](./iOS改密/71项最终输入及二进制.json)，后续群治理和注销改动另验，不能把本候选摘要称为最终全项目摘要。

本轮是真实模拟器原生生产传输和物理SQLite验证；尚未用XCUITest操作新增改密表单，未验管理员真实TOTP、真机系统生命周期、独立恢复后端到端或批准该构建具有合格收讫资格。测试账户仅在外置隔离实例，不使用用户原失败实例；没有修改系统CA或全局信任。
