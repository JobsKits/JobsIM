# 本人主动改密实现检查点

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、实现与默认值](#jobs-toc-heading-1)
- [二、实际验证](#jobs-toc-heading-2)
- [三、接续](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、实现与默认值</span><!-- jobs-toc:heading-end -->

[主动改密](../../../../代码/后端/Go/password_change.go)提供创建／显式原单重试和只读原结果两条入口，只读能力`own_password_change=true`。首次必须当前本人／原设备／epoch及旧密码，管理员另需当前TOTP。密码不持久化、不返回新凭据，完成后重新登录；不重置MFA或解除封禁。协议见[HTTP合同2.3](../../../../代码/后端/Go/接口契约/README.md#own-password-change)。

auth.password_changes保存原nonce、身份、带服务端钥的意图摘要及prepared替换哈希密文。密文AAD绑定实例／本人／设备／epoch／操作，原密码对与OTP不入表／日志；journal只有本人／凭据代际／安全代际。新操作先关闭旧准入，再保护intent及跨owner清理，本域提交替换哈希并撤全family、RT、BFF、初始交付、旧恢复码与重置凭证。提交即清密文，result与导出清理完成后标completed。

[恢复worker](../../../../代码/后端/Go/password_change_worker.go)复用identity的实际外层EX，按有界批次及last_attempt_at续原责任，不需要原密码。旧epoch或失去原责任则退休，不解除处罚／恢复状态、不降floor。结果接口不创建／重投／追加安全事实，仅写每IP限流计数；回放旧成功不清后来登录的Cookie。current=true须额外核独立日志epoch，恢复门禁不齐则503。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、实际验证</span><!-- jobs-toc:heading-end -->

本切片已有4组纯规则及真实PG专项；最终执行目录、构建与输入冻结见本目录JSON及执行记录。每轮均追加保留，不把旧输入的通过追认新源。

- 首轮[4纯／7PG](./执行记录/2026-11-16T12-32-36-977Z-34a950/结果.json)通过。
- 增加管理员正向和迟到浏览器Cookie反例后，[4纯／7PG通过、1PG失败](./执行记录/2026-11-16T12-34-48-981Z-edb12e/结果.json)保留：旧成功回放会错误清Cookie。随后修为仅首次当前已认证浏览器提交可清其Cookie，原单回放不清Cookie；[修后4纯／8PG](./执行记录/2026-11-16T12-35-40-885Z-3c3c51/结果.json)通过。
- 后续增加匿名原证明查单每IP20次限流、独立日志门禁及并发原单反例，最终[4纯／12真实PG](./执行记录/2026-11-16T12-41-43-795Z-ff2d05/结果.json)全部通过，零失败／跳过，前后94项输入SHA不变；两种实际session family均被撤销，原未过期AT真实authorize返回401。journal单独推进与两次读取间两侧同时推进的反例均拒将旧结果称为当前有效，不累计旧轮数量。

[生产构建](./生产构建.json)通过，jobsimd SHA256为`62d30eb3cfe365e9b6d86d759eeb6d5563da6fa0236d5f1f4d609c3637022f58`；[94项输入](./最终输入SHA256.json)与[冻结源码](./源码-62d30eb3.zip)包含86份生产Go、两个模块锁、五份外置测试及专项入口。该构建没有替换既有运行域或加载对象批准，不外推旧对象9f3b／导出2b58专项已覆盖本次改密。

PG专项在独占随机数据库重复完整identity迁移；生产auth／journal处理器和真实撤销触发器验证两种session family。message／governance是显式进程内确认夹具，journal也在该随机库；**没有真实HTTPS、独立双集群恢复、跨owner实际下载排空或客户端UI验收**。新迁移／接口不改旧9f3b对象或2b58导出的冻结包及报告。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、接续</span><!-- jobs-toc:heading-end -->

五端主动改密产品入口仍待接；客户端只保存非秘密原nonce与上下文，原证明只留内存，重启需本人重新输入，不把密码或无钥摘要写入本地库。响应unknown沿原单只查／显式重试，不能改nonce自动另建。后台恢复和旧原单成功不能代替用户重新登录，客户端不能清除后来激活的另一family。

[专项入口](../../../../测试/后端/接口/password_change.mjs/README.md)可复现本地与隔离PG验证。账号注销、普通迁移、全目标收讫及保留worker等第一阶段缺口继续，不因本切片完成后端改密而标完整I交付。
