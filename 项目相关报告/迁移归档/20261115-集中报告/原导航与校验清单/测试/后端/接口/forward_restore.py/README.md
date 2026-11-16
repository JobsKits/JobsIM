# JobsIM 转发恢复代际测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、运行范围](#jobs-toc-heading-1)
- [二、已验证](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、运行范围</span><!-- jobs-toc:heading-end -->

执行`python3 测试/后端/接口/forward_restore.py/forward_restore.py --environment-file <显式独立转发域环境> --go <Go可执行文件>`。原环境只提供现有binary、工具、CA和两独立PG集群连接；测试另建随机独占业务源／恢复目标DB和journal DB、外置State，默认30443／31101～31108，端口占用即拒绝。可显式改端口，但拒用8443／18443／20443／28443。没有下载或全局信任变更。

测试复制固定服务器binary，并把当前jobsimops编译到本测试外置目录。备份加密包与单独恢复key分目录保存；只停止本测试记录且argv精确匹配的PID，不停止原联调域。真实pg_dump、offline pg_restore、同原恢复操作重试后重新登录，结束停本测试服务，私密环境／数据库／制品留作审计，不自动清理其他任务资产。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、已验证</span><!-- jobs-toc:heading-end -->

[报告](../../报告/普通转发恢复代际实测.json)为33次HTTP／16语义。特意不在备份后改变源正文、成员代际或securityVersion；恢复后DB仍有active:epoch1引用，源仍可读，目标仅因新restoreEpoch拒旧正文／附件。原已成body只读核对仍返回占位，原未建epoch1拒创建，新显式选择与proof在当前epoch成功，但不复活原ref。

测试为同机隔离恢复，不是跨故障域RPO0、PITR、全资产恢复或safe_restore验收。报告含所复制服务器binary摘要，不能把后续不同binary外推为本次执行。
