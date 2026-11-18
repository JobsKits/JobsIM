# JobsIM iOS 群治理

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [前言](#%E5%89%8D%E8%A8%80)
- [一、行为与持久化](#jobs-toc-heading-2)
- [二、验证](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

## <span id="前言">前言</span>

2026-11-17，[群管理页](../../../代码/前端/iOS/JobsIM/UI/IMGroupGovernanceVC/IMGroupGovernanceVC.swift)与[原操作服务](../../../代码/前端/iOS/JobsIM/Services/IMGroupGovernanceService/IMGroupGovernanceService.swift)已挂应用target，群成员页进入高级群管理，设置页凭邀请码加入。原成员列表保留，新增服务端分页，不将前200名显示当全量。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、行为与持久化 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

- 全员禁言、普通成员慢速、容量、个人禁言、管理员升降、群主转让、解散；成员／邀请／申请分别分页、空态与重载。申请显示服务端公开姓名或用户名，不主显示UUID。
- 创建与撤销邀请、批准／拒绝申请都固定所见群version及本人MG／RG，目标成员另固定其MG／RG；原nonce先落SQLite，再查原结果，仅显式重试发写操作。重新登录／换设备可查本人历史责任，但原设备／family／epoch不符只能查，旧schema缺原资格不猜补。
- 同码冷续保持原申请编号；邀请码只在RAM或用户明确复制的剪贴板中，不存SQLite。群主不可用时仅后端返回canRecover才提供恢复入口，现有活跃成员可作为候选；本机独立授权和新TOTP均由服务端验。OTP和step-up只在内存，过期重试重新取证明，不替换原观察版本。
- 原结果校验requestId；治理另核action、applied、正版本；撤销／审批核typed action、原target及终态，错目标保持pending。账号注销completed清理本人的协议集合，别账号／别profile保留。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、验证 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[最新78项结果／103输入及二进制](./iOS改密/注销群控78结果及冻结.json)在专用模拟器执行通过，0失败、0跳过。包含群控4个方法：原计数器大于JavaScript安全整数仍精确、未知同body重试／纯查零POST；邀请秘密不落盘和冷恢复同nonce；跨部署与秘密metadata拒绝；换设备family后仅历史查询、typed错目标不能完成原单。与注销和原同步／导出／认证合跑，没有累加旧轮充数。十语各277键一致，主题沿用白天／黑夜／跟随系统。

原始[编译](./iOS改密/注销群控78编译.log)、[测试日志](./iOS改密/注销群控78.log)与[结果包](./iOS改密/注销群控78.xcresult)保留。群治理真实HTTPS、完整原生表单目视、管理员TOTP与本机独立grant、真机仍待验；该轮真实网络方法只覆盖已注明的本人改密，不外推群控。

<a id="🔚" href="#前言">返回前言</a>
