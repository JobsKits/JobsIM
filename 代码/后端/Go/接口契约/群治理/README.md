# 群治理、邀请与原操作结果

![JobsIM](../../../../前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、原权限快照](#jobs-toc-heading-1)
- [二、群治理](#jobs-toc-heading-2)
- [三、邀请与审批](#jobs-toc-heading-3)
- [四、分页与公开资料](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、原权限快照</span><!-- jobs-toc:heading-end -->

`GET /api/conversations/{id}/governance`返回`{version,membershipGeneration,roleGeneration,role,state,readOnly,policy:{version,muteAll,slowSeconds,maxMembers},mutedUntil,canRecover}`。所有版本／代际都是精确十进制字符串。默认全员禁言关闭、普通成员发言间隔0秒、容量200；管理员／群主免全员禁言和慢速，单人禁言独立检查。

用户确认之前取得所见快照。新写携原`version/membershipGeneration/roleGeneration`；目标写另携原`targetMembershipGeneration/targetRoleGeneration`。服务端在同一事务锁群和actor行后逐项比较，退重入、角色ABA或群变化返回`409 GROUP_PERMISSION_CHANGED`／`GROUP_TARGET_CHANGED`，不能自动换新资格重发。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、群治理</span><!-- jobs-toc:heading-end -->

`POST /api/conversations/{id}/governance`基础body为`{requestId,version,membershipGeneration,roleGeneration,action}`：

| action | 其它字段／授权 |
| --- | --- |
| `settings` | `muteAll:boolean,slowSeconds:0..3600,maxMembers:2..10000`；仅群主，容量不得低于当前成员数 |
| `mute` | `targetId,targetMembershipGeneration,targetRoleGeneration,mutedUntil`；空字符串解除，未来绝对截止最多一年；管理员不能操作群主或同级管理员 |
| `role` | 同一目标三元组＋`role:admin\|member`；仅群主 |
| `transfer` | 同一目标三元组；仅群主，当前有效继任者。旧PATCH ownerId入口已关闭 |
| `dissolve` | 仅群主；解散不可由普通恢复动作重开，并撤销原群邀请 |
| `recover` | 原群version、`targetId,targetMembershipGeneration`；当前admin与独立operator `recover_group` grant，群须owner_unavailable；另需下述RAM step-up |

成功响应`{status:"applied",requestId,action,version}`，新version是字符串。`recover`先POST`/api/auth/step-up`，body`{action:"group.recover",targetId:群ID,requestId:原nonce,otp,payload:原governance body}`；governance首写只在RAM附`stepUpId`。OTP／授权不进入业务原body或摘要。过期或授权冲突停止旧授权，不自动换nonce；非成员admin不因本能力获得群发现权限。

`GET /api/conversations/{id}/governance/operations/{requestId}`只读当前actor的既成原结果，404为`GROUP_OPERATION_NOT_FOUND`，不创建。旧nonce先核原digest并回原事实，再验证任何尚未创建的新写；不得因后继角色变化重跑已成原单。首次未见只由用户明确沿原body再试。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、邀请与审批</span><!-- jobs-toc:heading-end -->

| 入口 | 请求与原责任 |
| --- | --- |
| `POST .../invites` | `{requestId,version,membershipGeneration,roleGeneration,expiresAt,maxUses,approvalRequired}`；不超过30天的未来UTC时刻，次数1～10000，审批布尔显式填写；当前manager。GET`.../invites/operations/{requestId}`按原creator只查 |
| `DELETE .../invites/{inviteId}` | JSON body`{requestId,version,membershipGeneration,roleGeneration}`；原actor结果写group_operations，GET governance/operations只查。响应`{status:"applied",requestId,action:"invite_revoke",targetId:inviteId,state:"revoked"}` |
| `POST .../join-requests` | `{requestId,inviteId,token}`；同用户原nonce绑定原invite；GET`.../join-requests/operations/{requestId}`只查原申请者，不需要token |
| `POST .../join-requests/{applicationId}` | `{requestId,version,membershipGeneration,roleGeneration,approve:boolean}`；当前manager，审批时还核原邀请仍有效及creator资格；原actor结果写group_operations。响应`{status:"applied",requestId,action:"join_decide",targetId:applicationId,state,membershipGeneration:string\|null}` |

创建邀请、撤销和审批成功后推进群version。原请求必须保留原nonce/body，不能静默按当前群版本重基。`requestId`允许8～128字符；Web新建统一UUID。撤销／审批只查原单和invites创建／join申请各有独立路由，不用另一个actor的当前列表状态冒充本人的原操作结果。

邀请创建由服务端冻结原creator membership／role代际及identity securityVersion／restoreEpoch，客户端不能传这些字段。新join和pending approval均复核creator仍是原活跃manager，四项精确相同。旧0／未知列失效保留；退重入、降权再升、改密、封禁、注销申请再取消和恢复都不能复活旧邀请。原已成join结果仍可按本人原nonce只查，不能借历史回放重新入群。

分享格式统一为`JobsIM-Invite-1:<标准base64的UTF-8 JSON>`，JSON仅`{deploymentId,conversationId,inviteId,token}`。token只RAM／用户明确复制；客户端只持久入群nonce和inviteId，不保存token或其无钥hash。冷启动若需要首次重试，必须重新输入同一code并核实例／群／invite一致，原nonce不变。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、分页与公开资料</span><!-- jobs-toc:heading-end -->

`GET .../members`、`.../invites`、`.../join-requests`支持`limit=1..100&after=<原UUID>`，返回`{items,nextCursor}`；空游标表示结束。成员返回当前role和精确成员／角色代际。邀请列表中的token只供当前manager显式分享，不能持久化或当成已批准本地存储资格。

申请列表每页先关数据库rows，再向identity owner请求本页公开profiles；项目仅返回`username/displayName/accountState`，保留申请自己的`state`和`createdAt`。不会下放identity密码、凭据或未知额外字段；已抹除／不可用账号显示占位。UI主显姓名／账号，申请态和时间次级，不用opaque UUID作为批准对象的主文案。

Web实现与限定验证见[群Web闭环报告](../../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Web群治理.md)。独立恢复后的operator grants须重新审阅，历史备份不携带当前治理授权。
