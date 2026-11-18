# JobsIM Web 群治理与邀请闭环

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、交付入口与行为](#jobs-toc-heading-1)
- [二、原责任与秘密生命周期](#jobs-toc-heading-2)
- [三、实际验证与源码冻结](#jobs-toc-heading-3)
- [四、复现与边界](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、交付入口与行为</span><!-- jobs-toc:heading-end -->

群成员页已接入真实治理面板：全群禁言、慢速、人数上限、成员禁言／解禁、管理员任免、转让、解散及具备独立授权时的群恢复。新会话弹窗提供邀请码加入入口。成员、邀请、申请和本人的历史原单均有完整空态、重载和分页。申请以公开姓名为主文案，申请状态与账号状态分别展示。

| 位置 | 当前实现 |
| --- | --- |
| [GroupGovernancePanel.tsx](../../../代码/前端/Web/Src/GroupGovernancePanel.tsx)／[GroupControls.tsx](../../../代码/前端/Web/Src/GroupControls.tsx) | 服务端治理快照、动作确认、冻结目标、分页成员／邀请／申请、显式复制及原单只查／明确重试 |
| [GroupJoinPanel.tsx](../../../代码/前端/Web/Src/GroupJoinPanel.tsx)／[App.tsx](../../../代码/前端/Web/Src/App.tsx) | 同实例分享码入群、同码冷启动续原单、真实服务请求与既有演示回退；真实转让走governance，标题仍走原PATCH |
| [GroupGovernanceProtocol.ts](../../../代码/前端/Web/Src/GroupGovernanceProtocol.ts) | 字符串代际、DTO白名单、分享码标准base64 UTF8、原actor／target及step-up用途绑定 |
| [GroupGovernanceStore.ts](../../../代码/前端/Web/Src/GroupGovernanceStore.ts)／[GroupGovernanceActions.ts](../../../代码/前端/Web/Src/GroupGovernanceActions.ts) | 严格IDB原责任、WebLocks、只查原结果、原family／设备写门禁、15秒有界请求和非秘密结果白名单 |
| [GroupGovernanceLabels.ts](../../../代码/前端/Web/Src/GroupGovernanceLabels.ts)／styles | 简中、繁中、藏、英、日、法、西、德、俄、韩完整文案；继承白天／黑夜／跟随系统与字号，窄屏控件可用 |
| [group_invites.go](../../../代码/后端/Go/group_invites.go)／[契约](../../../代码/后端/Go/接口契约/群治理/README.md) | 管理原nonce／快照、邀请issuer四代际、申请人公开资料白名单及游标分页 |

策略原默认值由服务器返回，Web不以新默认静默覆盖。确认时固定所见group version、actor membership／role及目标membership／role；目标选择后即使名册更新也不替换代际。旧快照失败后必须重新观察并重新确认，不沿原nonce重基。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、原责任与秘密生命周期</span><!-- jobs-toc:heading-end -->

每个写操作先将原身份、nonce、类型、非秘密body及阶段严格提交IDB，再请求原actor结果。查询404 `GROUP_OPERATION_NOT_FOUND`只表示未查到，零创建POST；用户明确重试才沿原body与nonce发送。已成原单可在后继登录中只查，后继family／设备不能首次发布旧原意图。响应白名单不保存额外服务端字段。

`group-operations|origin|deployment|userId`仅保存非秘密责任；密码、OTP、stepUpId、邀请token及其hash均不持久化。邀请使用`JobsIM-Invite-1:<标准base64 UTF8 JSON>`，JSON为`deploymentId/conversationId/inviteId/token`。只有用户明确复制才写剪贴板，复制前重新检查邀请仍有效。入群原单仅保存nonce与inviteId，冷启动要重新输入同码才能续原请求。

恢复必须由服务端`canRecover`授权；OTP通过`/auth/step-up`绑定原requestId、群ID与原body，仅RAM内短期authorization可附到同一governance请求。身份改变或授权过期禁止发送旧操作，不自动换nonce或刷新授权。

服务端create、revoke、decide均先读自己的旧nonce既成结果，再检新操作权限。邀请由服务器冻结creator membership／role／security／epoch，join和批准重新核对当前active管理资格与全部原值；退群重入、角色ABA、改密、封禁、注销后取消和恢复均不能复活旧码。旧0代际记录保留但拒绝使用。分页申请仅向identity请求本页公开资料，保留申请state，另用accountState；未知额外凭据不回传。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、实际验证与源码冻结</span><!-- jobs-toc:heading-end -->

| 当前验证 | 结果与证据 |
| --- | --- |
| 群浏览器 | [最终26组通过](./Web群治理/04-最终浏览器/结果.json)：实际Chrome／React／IndexedDB／WebLocks，含原body、双未知、404零POST、actor／target ABA、旧family、同码续单、秘密拒存、恢复、分页、毒字段、十语及实际表单；[首轮22组](./Web群治理/01-首轮/结果.json)保留 |
| 注销回归 | [23组通过](./Web群治理/06-注销回归/结果.json)，使用本轮群代码后的相同App源码 |
| 认证／主App | [认证16组及App8类别通过](./Web群治理/07-认证与App回归.log)；[App独立结果](../../测试/前端/Web/执行记录/1794874323084-23791/browser-result.json)，无JS异常 |
| TypeScript／构建 | [tsc --noEmit](./Web群治理/03-类型.json)、[Vite生产构建](./Web群治理/05-构建执行.json)均exit0 |
| Go编译 | [go test ./... -run '^$'](./Web群治理/02-Go编译.json)exit0，编译不冒充执行PG反例 |
| 管理原单真实PG | 前5顶层方法实际通过，涵盖角色ABA、actor／nonce、批准大代际、issuer五种变化及批准重核；源码与记录见下文分轮边界 |
| 申请人页真实PG | 第6顶层方法修正夹具后实际通过0.42秒：真实internal/member路由替换错误direct占位，毒字段／申请state／limit与after断言未降低 |

PG前5通过记录保留于[第二次恢复专项](./恢复迁移证据/第二次恢复专项-命名空间导入失败.jsonl)。随后新增第6方法首次403记录保留于[第三次恢复与申请页失败](./恢复迁移证据/第三次恢复-17.60秒通过与group申请页失败.jsonl)，没有将失败轮标绿。第6修夹具后的通过记录为[最终冻结专项](./恢复迁移证据/最终冻结-八域与原入口真实通过.jsonl)，其中另两项为其他代理的恢复验证；本片不把3个顶层计为群3项，也不声称群6在该轮全部重跑。

最终[64份输入索引](./Web群治理/08-最终输入/索引.json)包含54份浏览器候选源码及补充配置／后端／说明；54份与最终26组执行时哈希逐字节一致。构建位于工程外`Build/Web/GroupGovernance/1794874112975-22930/Production`：JS `index-AKBKHTXc.js` SHA256 `dc3065edc84bb6fe30c11713a0a97daebf00ee996f4a9b7d7c994c38ac5b3bcd`，CSS `index-BsxsEHpf.css` SHA256 `d3730e836387ec79548f82a5ebe7d7994dc66696f531fe9da637826e501ff908`。精确路径、全部产物摘要见索引。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、复现与边界</span><!-- jobs-toc:heading-end -->

```shell
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_GROUP_REPORT=/本轮未使用的新目录 \
node 测试/前端/Web/group-governance.cjs/group-governance.cjs
node 代码/前端/Web/node_modules/typescript/bin/tsc --noEmit -p 代码/前端/Web/tsconfig.json
# 在代码/后端/Go内，仅验证编译：
go test ./... -run '^$'
```

使用既有Node24.22.1、Playwright1.65.1与Chrome。随机回环Vite不读代理，实际`/api`网络阻断；HTTP、身份、剪贴板为明确夹具，浏览器结果不证明真实HTTPS、系统剪贴板、MFA消费或实际群写入。Go专项使用独立随机PG，身份与journal传输为受控夹具；不使用用户账号与运行实例。三主题按既有语义色继承，藏文黑夜及390宽截图已实际检查。

本轮外层每秒检查4GiB门禁，每子进程180秒，超限只结束自己进程组；最终浏览器／回归最低余量约5.3GiB。没有清理历史证据、修改第三方、提交或推送。普通迁移、实际部署后多端联调由后续切片单独验证，不能借本轮冻结推断覆盖。
