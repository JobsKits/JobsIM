# CLI 权威同步真实终端联调

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

显式执行 `python3 sync-https.py --environment-file <固定57私密seed> --binary <候选CLI> --data-root <不存在的本轮私有目录> --report-dir <不存在的新中央报告目录>`。仅 Python 标准库，不安装依赖；检查 freshSyncRealm／persistentData、固定回环44443及57 binary摘要、严格CA／主机名，拒绝旧数据或旧报告目录。每个自身TTY限180秒、阶段30秒、4GiB磁盘门禁；失败只正常退出或停止本入口自己的进程组，不控制服务器／PG／NATS。

实际新建本轮两账号，通过真实PTY明确选择 process-only 会话，缺钥匙服务时不写明文凭据；不冒充OS安全库正向。密码不进argv／环境／普通SQLite或报告，不保存原终端缓冲。新 profile／DataRoot 绑定原实例CA；仅观察自己本地SQLite，不用SQL改服务端。

用例覆盖好友／群、双向消息、真实schema1快照／增量、缺媒体对象明确拒绝产生的未提交原意图、本人只读查单、另一新peer同clientMsgId实际成功不确认本人队列、真实recall/global、退出后新device／UUID与旧库清理，以及被移出群后合法普通本地历史与后续global。缺媒体拒绝不是未知提交。辅助peer同ID由另一个本轮本人控制family真实API产生；该family仅存在进程内并在finally logout。没有拦截CLI HTTP计数，不宣称实测零创建POST；不扩大503／响应丢失故障注入。

所有finally只退出本轮自己的TTY／撤控制family，保留私有数据作继续核查，不删除既有账号或SQL清群。报告只保存断言、公有ID、源码／binary摘要、脱敏状态；原payload／sendAdmission仅保留在原私有DataRoot，不复制进报告。失败轮保留，重跑使用新目录。完整I／生产恢复／跨系统与OS安全库仍需单独验证。
