# CLI 本人注销与受限查询取消

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、入口与责任](#jobs-toc-heading-2)
- [二、本机清理](#jobs-toc-heading-3)
- [三、实际验证与未验增量](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

CLI新增本人注销、匿名原结果、当前密码／单次恢复码受限查询与取消、最终本机清理。最后实测候选全runner 115通过／0失败／2环境跳过；其中注销11方法通过，包含“未知取消一次恢复码不得连续签query和cancel”，已构建darwin/arm64二进制。此后main.go仅将未保存默认URL改为原入口9443，尚未重编；已有／显式URL不变，不追认115轮二进制包含此增量。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、入口与责任 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

登录后`account-delete`明确确认；无口令命令参数，当前密码／OTP只经既有TTY无回显读取。原当前family／device／physical store／epoch及nonce先在authentication.sqlite的own_deletion_intents以WAL／FULL保存。首次只在精确原family创建，未知先暂停普通API／refresh，冷启动在读安全凭据前复核原责任；服务端撤全部family后仅清匹配原family的本机安全条目，迟到旧结果不清后继。

无普通会话也可用`account-deletions`列表、`deletion-reconcile REQUEST_ID`匿名原密码查结果、`deletion-retry REQUEST_ID`明确同原nonce续跑。每次先调用deletion-results；纯查零创建请求，只有原404才能由原family首次POST，旧epoch／能力撤回只允许查原单。prepared后的依赖403维持unknown；只有fresh首写明确白名单拒绝才回active。

`deletion-query REQUEST_ID`以当前密码或一枚当前恢复码换RAM query proof；`deletion-cancel REQUEST_ID`冻结所见version并确认，原cancel nonce／cancelVersion先落盘。未知取消只能先查，`deletion-cancel-retry REQUEST_ID`明确续原nonce，查询新版不重基。proof的purpose／deletionId／版本／epoch／期限均检查，不写磁盘或日志，不带普通AT／Cookie；请求禁止重定向。完成不自动登录、取消不复活旧会话。

已通过第11方法的恢复码细节：未知取消先查仍requested且同原version后，将该原cancel责任记prepared，再要求用户下一次明确输入不同当前证明；同一单次恢复码不会连续消费为query与cancel。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">二、本机清理 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

completed先落公开回执，再在原profile根耐久写最小AccountErasure用户floor；仅枚举项目管理的namespace散列目录，读取数据库identity逐项核原origin／profile／deployment／user／personal。全部旧epoch／device自有client.sqlite删所有正文、草稿、Outbox、sync／资格／group／导出／路径引用表，以secure_delete／VACUUM／WAL截断完成清理，保最小身份与终态拒写事实。关闭当前旧连接，入口openStore额外拒终态floor，迟到旧连接不能重建正文表。

ReceiptObjects中只删除原epoch目录下UUID.bin托管原件并同步父目录；未知路径／符号链接中止并保清理责任，不遍历其他profile／user。CLI头像上传读取用户所选原文件，导出／下载明确写用户所选位置，没有把这些外部文件冒称应用私有缓存；此处不删除外部原件或已导出的ZIP。清理失败保completed+localCleared=false，原账号元数据重开或查单可续；不把服务器completed等同所有设备／历史备份物理清空。

源码为[own_deletion.go](../../../代码/前端/CLI/cmd/jobsim/own_deletion.go)、[own_deletion_command.go](../../../代码/前端/CLI/cmd/jobsim/own_deletion_command.go)、[own_deletion_cleanup.go](../../../代码/前端/CLI/cmd/jobsim/own_deletion_cleanup.go)。auth／main／store仅加最小门禁与命令hooks，群文件仅按主代理授权gofmt；不替换其新family和typed结果改动。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">三、实际验证与未验增量 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[最后实跑候选](./CLI注销证据/恢复码原单修复/SQLite结果.json)115顶层通过、0失败、2跳过（环境未提供真实HTTPS／原生安全库诊断）；[跑前后源码](./CLI注销证据/恢复码原单修复/跑后一致性.json)一致，完整输入／raw日志及[产物SHA](./CLI注销证据/恢复码原单修复/产物SHA256.txt)同目录保留。11项注销含真实SQLite持久／重开、原责任无秘密、只查不创建、能力／后继隔离、prepared403、proof目的版本、未知取消、CAS、精确多store清理和迟到写。网络为本机HTTP受控夹具，安全库为测试适配；没有连接a4、真实HTTPS、真实Keychain或终端无回显交互验收。

首轮113过／1失败／2跳过，失败是元数据解码容忍未知password字段；本模块改DisallowUnknownFields后114通过。首次失败和修后候选均保留。新根为用户迁仓后的JobsProds/JobsIM，运行路径继续由项目／配置位置解析，不固化旧仓路径；旧报告绝对运行路径不改写。

115轮之后默认9443增量与两端最新迁移／注销清理新增源码受4GiB门禁暂停测试／原生编译；两端取消核心已另跑Harmony16项host，不外推最新Android／Harmony包。未提交／推送，未清历史数据或缓存。

<a id="🔚" href="#前言">返回前言</a>
