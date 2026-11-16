# 注销状态的普通准入门禁

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、范围](#jobs-toc-heading-1)
- [二、验证](#jobs-toc-heading-2)
- [三、接续](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、范围</span><!-- jobs-toc:heading-end -->

本切片只补现有`auth.users.deletion_state`的普通认证门禁；**没有公开注销API、受限data_rights证明、冷静期取消或到期跨owner清理worker，不代表注销流程完成**。现行I合同不变，默认7天、0～30天及取消／processing同请求行CAS仍须完整实现。

仅`none`可正常登录／重认证及交付旧初始凭据、普通AT访问、BFF解析、RT旋转、恢复码／reset grant兑换、管理员首次签发重置凭证、正式导出principal授权和新主动改密。identity后台把非none账号旧family加入撤销范围。密码变更prepared若被注销状态取代则退休并清密文，已完成历史结果不再宣称current；不能借恢复密码重新开放普通聊天。

未修改账号active／banned／tombstoned与recovery_state；没有拿墓碑冒充申请，不在等待期清正文或解除处罚。未来受限查询／取消入口须独立purpose授权，不能放开这些普通路径。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、验证</span><!-- jobs-toc:heading-end -->

新[外置反例](../../../../测试/后端/接口/account_deletion_admission_test.go)用真实随机PG、完整identity迁移、生产auth／journal处理器、有效session family／BFF和有效恢复材料，分别检查requested／processing／completed的拒绝与不消费、不新发；另验prepared改密退休、历史改密结果current=false。message／governance仍为进程内确认夹具。

[首轮结果](./执行记录/2026-11-16T13-08-42-850Z-923656/结果.json)4纯／18PG通过事件保留。独立只读复审发现prepared管理员重置原单尚缺注销状态复核，已补保护／冻结前的精确原责任检查及最终用户行锁内再次复核，并增加两时点反例；还补了已成login／register／reauth原ID交付的正向对照与注销后拒发凭据反例。

[最终输入与结果](./执行记录/2026-11-16T13-17-18-670Z-b518fc/结果.json)：4纯规则、25个PG通过事件，零失败／跳过；PG为原12改密＋5个新测试组及8个表驱动子项，不能当25个互不重复业务场景。95项输入前后SHA不变，[两个cmd构建与输入封存](./最终构建与输入.json)通过；[复核后固定源码](./复核后固定输入.zip)与首轮归档分别保留。无HTTPS、独立双集群恢复或原生测试；未替换已有运行域，未新开API会话。

`修改前/`保存七份实际修改前Jobs源码；[执行入口](../../../../测试/后端/接口/account_deletion_admission.mjs/README.md)不改生产模块，不下载依赖。先前62d3改密、9f3b对象及2b58导出的包与报告保持原义，不追认为已经包含本次七文件门禁。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、接续</span><!-- jobs-toc:heading-end -->

继续治理请求行、原申请／结果、短期受限本人证明、原取消nonce／截止竞争、逐owner清理与保留manifest、最终不可恢复floor及journal恢复重放。用户名最小占用、他人合法历史、最终群owner_unavailable及备份物理残留窗均按原需求处理；不因已加普通门禁而开启或宣布注销能力完成。
