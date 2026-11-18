# 媒体收讫与原件清理测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 一、范围

[外置 Go 用例](../media_retention_pg_test.go)由[统一运行器](../../运行测试.py)自动挂载，源码不复制进生产目录。2 个纯规则方法核对独立附件期限、显式零延迟、数据库微秒往返及流式到期停止；11 个真实 PostgreSQL 方法核对原设备续传、完整原件描述、不可延期、失败保字节、归档取消、对象收讫、重放、共享围栏、符号链接拒绝、持久时钟闭锁、显式旧对象白名单、既成 global 的旧件抹除及备份／离线重清选择。

数据库用例复用受保护的 `newRetentionPG` 随机数据库与私有临时文件；身份、原冻结计划和治理 RPC 是明确夹具。它们不证明真实 HTTPS、独立 journal 集群、真实原生端存储批准、第三方媒体库、WindowsOS 或 Linux 原生文件系统。

## 二、执行

在项目根目录运行：

```shell
python3 测试/后端/运行测试.py --backend-only --go /选择的Go可执行文件
```

无显式隔离环境时，纯规则运行，数据库方法跳过。真实 PG 需预先准备工程外 `Integration/<UUID>/Safety/Secrets/pgpass`，权限 `0600`，以及三项明确环境变量：`JOBS_IM_SYNC_SCHEMAS_TEST=1`、`JOBS_IM_SYNC_SCHEMAS_REALM_URL`、`JOBS_IM_SYNC_SCHEMAS_ADMIN_URL`。受保护辅助函数核对已有隔离端口、owner、随机数据库前缀和 passfile 所属；不得改成普通联调实例、真实账号或默认本机数据库。

定向执行使用同一外置 overlay：

```shell
go test -json -overlay /本轮私有overlay.json ./... \
  -run '^Test(MediaRetention|Retention|ObjectReceiptPGMediaChecksRealOriginalFile)' -count=1
```

运行时保持磁盘余量每秒检查，低于 4 GiB 或本轮超时只终止自己的进程组。保留原始日志、输入摘要与首次失败；不能只替换最后汇总。结果和当前限制集中到[实现报告](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/媒体收讫清理.md)。

## 三、关键反例

| 反例 | 必须观察到的结果 |
| --- | --- |
| 正文已清，但原对象期限未到 | 只有原冻结且正文描述已 ACK 的当前合格物理设备可续传；Range 返回原字节 |
| 同用户不同 device/store/epoch 或已失权 | 不返回原件描述及字节 |
| 只有正文 ACK、缺原件完成或延迟未填 | 不误判完整原件收讫；未填延迟不得变为零 |
| 原件收讫延迟显式 `0` | 按原件完成时刻清理，且不晚于独立绝对截止 |
| 编辑或恢复旧快照 | 已保护的对象截止不得推迟，terminal floor 不复活 |
| journal 或精确归档取消失败 | 原字节保留；已经退休的对象拒新读取，待后台沿原责任重试 |
| 活跃共享投影、替换成符号链接 | 排他清理等待共享响应排空；拒绝跟随符号链接 |
| 旧对象无收讫 assignment | 只接受 message owner 明确 `legacy=true,cleanupEligible=false` 的白名单；不据 `409/503` 猜旧对象，不造清理期限 |
| 旧白名单对象后来完成 global | 只接受 owner 明确的 `erasureOnly`＋原 job/event；独立最小删除事实保护及重放，不伪造收讫目标／manifest |
