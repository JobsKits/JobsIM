# CLI 搜索与群协作限定证据

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、结果与产物](#result)
- [二、实现与故障验证](#implementation)
- [三、真实 HTTPS 范围](#https)
- [四、边界](#limits)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本轮只修改 CLI 与其外置测试，补服务端搜索／收藏分页、精确源、结构化提及和群公告。此前[普通转发 17 组](../20261115-普通转发/README.md)及[治理基线](../20261115-CLI治理/README.md)保留原证据和二进制；跨轮项数不相加，不代表完整 I／243 项验收。

## 一、<span id="result">结果与产物</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

21 组真实 SQLite／受控 HTTP 故障测试通过，0 失败；最终独立 binary 的真实 HTTPS 协作链通过。构建使用现有 Go 1.27.4 与依赖锁，没有新增依赖、全局信任／代理变更或后端重启。

| 证据 | 位置 |
| --- | --- |
| 被测输入与二进制摘要 | [验证摘要](./验证摘要.json) |
| SQLite／故障 21 组 | [结构化结果](./SQLite结果.json)、[原始日志](./SQLite测试.log) |
| 真实 HTTPS | [结构化结果](./CollaborationHTTPS结果.json)、[原始日志](./CollaborationHTTPS测试.log) |
| 离线入口 | [运行说明](../../../../测试/CLI/运行测试.mjs/README.md) |
| 真实入口 | [协作 HTTPS 说明](../../../../测试/CLI/collaboration-https.cjs/README.md) |

本轮输出为用户缓存 `CLI/collaboration/jobsim`，SHA256=`bd973f53f167f1a18be99593ea20c921e2e46bd36891a0e52814e2076e5f6b53`。原 `CLI/jobsim-forward` 仍为 `ecd708106af3db4e2a1a4243632d9ea87857c70a72bb25fc196313b8830e52c5`，已逐字节指纹复核未覆盖。实际绝对路径与大小见摘要，不把本机路径写成产品运行依赖。

## 二、<span id="implementation">实现与故障验证</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- `search/favorites/more` 保留原查询／会话／实例／本人／恢复代际，游标原样回传；默认 50，UTF-8 查询 1～256 字节。`open-result` 只取本次结果 ID，重新 GET 会话和源消息，不依赖目录前 100 条。源 ACL 拒绝或账号／epoch 变化清本次查询状态；不持久化列表正文或游标，普通展示递归去临时 proof。
- `mention` 的当前成员数组／@所有人、原 sendAdmission 与原角色快照先入 SQLite，再 HTTP；重试保持原名单与资格。新写核 mentions／send_admission，@所有人还核 role_admission 和当前群主；缺能力或旧缺角色快照只读原结果。明确资格／成员拒绝冻结，429 和暂时策略失败不误作成功或永久失效。
- `pending.payload` 支持类型化数组／布尔，空字段保持省略；旧 map[string]string 的精确 JSON 摘要兼容有独立断言。新私有角色快照也纳入本地完整性摘要，不给旧记录补新值。原治理冻结的纯历史 GET 核对、原转发 proof／ID、无正文缓存围栏继续通过既有组。
- `announce/clear-announcement` 冻结原 requestId、CAS、成员／角色代际、源 contentVersion 与到期引用，只持久化引用。未知先 GET 原 operation，明确恢复才沿原 body 写；已成原事实与当前 GET 分开显示，旧 current=false 不回滚新公告。
- 公告原事实的持久化和展示都使用类型化元数据，拒绝把意外响应中的源正文写入或显示为历史公告事实。原请求含正文等不支持字段时，事务前直接拒绝。SQL 故障无部分操作，旧未知不能被新 ID 覆盖；资格／版本／到期等明确失效冻结，恢复代际变化冻结旧操作。
- 本地 schema3 增量添加公告引用操作表；schema1→3 保留原 Outbox 字节、摘要和独立冻结原因。进程关闭重开继续取原操作，不改角色／CAS后重放。

## 三、<span id="https">真实 HTTPS 范围</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

使用匹配 CA、主机名及独立部署的 28443 服务，两位随机测试用户，不用个人账号。实际创建 51 条消息与收藏，搜索／收藏均 50＋1 翻页；查询后编辑源，精确打开显示新正文；临时 source proof 不出现在普通命令输出。

成员提及和群主 @所有人经真实后端返回，收件人 GET 验证 mentionedMe。公告完成不设期限／一天期限发布、个人收藏变化不破坏正文版本、原操作 current=false 不回滚新版本、源编辑变 unavailable、明确清除与进程重开。转让群主后新 @所有人／新公告在入队前被拒；源成员退出重入后，旧列表原文 GET 被拒并清分页会话。

## 四、<span id="limits">边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

丢响应、旧排队角色变化、缺能力／缺角色记录、到期拒绝及 SQL 回滚是独立 HTTP 故障服务与真实 SQLite 的证据，不冒充真实后端停机／丢包。真实 HTTPS 本轮验证角色失权后的新写门禁，旧队列角色回转拒绝属于受控故障断言。目录前 100 条之外的精确定位由测试服务仅提供 exact GET 模拟，真实测试没有创建 101 会话。

CLI 为终端 JSON 交互，没有 GUI、真机或完整本地演示回退。已输出终端的正文及用户明确下载的文件不承诺远程擦除；转发本地缓存仍仅占位。完整 I／243、生产灾备及下一块 auth-refresh 均不在本轮通过范围。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
