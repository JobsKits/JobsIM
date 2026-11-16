# <span id="前言">CLI schema1 真实 HTTPS 与终端专项</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、结论与固定范围](#jobs-toc-heading-1)
- [二、实际检查](#jobs-toc-heading-2)
- [三、产物、秘密与收尾](#jobs-toc-heading-3)
- [四、原轮与未验边界](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、结论与固定范围</span><!-- jobs-toc:heading-end -->

2026-11-16，固定57／`https://127.0.0.1:44443` 的最终 **10 项限定检查全部通过**，driver 退出0，1.01秒，最低观测空闲12.34GiB。使用新建账号、新私有DataRoot／profile、真实PTY与严格实例CA／主机名；不改原账号，不用PG／SQL，不启停或升级服务、不改配置。

[原始断言](./Assertions/result.json)、[driver结果](./Driver.json)、[脱敏日志](./Driver.log)、[输入SHA及源码快照](./源码SHA256.json)分别保留。此前[45个本地测试](../../20261116-权威同步编译回归/20261116-113743-192d48/README.md)与本专项分别记数；不是完整I或跨平台验收。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、实际检查</span><!-- jobs-toc:heading-end -->

| 检查 | 实际观察 |
| --- | --- |
| 严格CA与固定实例 | seed freshSyncRealm／persistentData为true，指定binary文件SHA符合固定57；严格CA bootstrap后绑定部署／epoch，末次相同 |
| 两账户真实终端 | 两独立CLI process-only会话，显式输入yes；真实好友接受及群创建 |
| 双向权威同步 | 两端真实发送，schema1 snapshot／changes安装后两条消息均在各自SQLite；连续位置本轮均为12 |
| 原体与同CID归属 | 坏媒体实际HTTP403 OBJECT_NOT_OWNED；原行未提交，本人原body／hash／state保存。另一新peer控制family用同CID真正提交成功，Alice sync后原行不ACK、不被改写 |
| 独立global | Bob真实recall另一条消息，Alice sync后terminal存在、正文移除，原未提交行原字节／hash／state不变 |
| 原终端退出 | Alice第一个真实TTY正常quit、退出0 |
| 新device与旧库清理 | 同DataRoot本人重新login，新device／physical UUID不同，原installation32hex不变；旧队列冻结且原body／hash保留，不迁入新库。后续global进入可信旧库terminal并清旧正文 |
| 退群云读与本地资格 | 移除Bob后其精确GET实际403 NOT_A_MEMBER；local-history明确cloudPermission=false，先前合法普通正文仍可读 |
| 历史holder global | 原作者真实recall后，已离群Bob sync照样得到global；retained正文移除、terminal存在、本地入口不再返回该消息 |
| 末次身份 | 部署5ff51e9e-a6fa-4ff1-83ce-23ee15125682、epoch1始终一致 |

坏媒体是明确拒绝，不是“提交成功但响应未知”。peer同CID是另一真实作者的成功消息，不是伪响应；辅助控制family仅在进程内持凭据，最终明确logout成功。没有拦截／统计CLI的HTTP创建次数，**不宣称实测零创建POST**。未知503、丢响应、SQL故障和迟到guard等仍以此前本地注入集为限定证据，不外推本次真实故障注入。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、产物、秘密与收尾</span><!-- jobs-toc:heading-end -->

候选CLI SHA256：`18738c34228b3318a7a68c8eb34854cb9c3a84fc0351d405a69d16d365f12929`；固定服务seed所指binary文件SHA256：`57a86767d12f005da08c728ff544a2b66076b2db03a203e3221d3490e355f290`。CLI Go源码与其编译轮输入SHA一致，没有另build或换服务。

[运行入口](../../../../../测试/CLI/sync-https.py/README.md)只需Python标准库。私有DataRoot在本轮Application Support/JobsIM/Build/CLI下，路径见Driver.json；原Outbox proof／body只保留该私有目录，不复制进报告。密码只经PTY内存输入；AT／RT只由CLI或辅助控制family内存持有，原终端缓冲不落盘、不写环境或argv。不输出私密seed正文，不复制数据库／CA私钥。

最终三个自身TTY均正常退出0；独立peer控制family的logout返回成功。CLI正常quit会执行自身logout，但本轮没有再查询服务端会话清单独立证明每个TTY家族的撤销状态，不能将进程退出当成该独立证据。只关闭本入口自己的进程；新账号／群与私有状态保留可核，不SQL清库、不删除其他数据。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、原轮与未验边界</span><!-- jobs-toc:heading-end -->

[首轮](../20261116-115919-5dc9cd/README.md)原结果／原源SHA保留；它没有具体拒绝码和云端精确GET断言，最终轮加入并全新重跑，同一10项不累加。没有业务代码变更，不以加强后的脚本追认原轮。

本次实际路径是process-only，不证明OS安全库正向或Mac DPK entitlement已解决；Windows／Linux原生、完整迁移／灾备、server read／云draft与偏好、qualifiedACK／after_receipt均不由此宣布完成。原合法普通本地历史与global只在本实例／本人／同epoch的受控资格范围内验证；用户显式下载外部文件不远程删除。无提交推送。

<a id="🔚" href="#前言">返回首页</a>
