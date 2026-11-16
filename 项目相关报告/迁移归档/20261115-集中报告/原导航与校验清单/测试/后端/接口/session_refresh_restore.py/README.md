# JobsIM 刷新凭据隔离恢复测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、运行](#jobs-toc-heading-1)
- [二、结果与边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、运行</span><!-- jobs-toc:heading-end -->

执行`python3 测试/后端/接口/session_refresh_restore.py/session_refresh_restore.py --environment-file <refreshTestRealm私密环境JSON> --go <Go可执行文件>`。另建随机源／目标业务DB、独立journal DB及外置State，默认34443／35101～35108；复制当前已验证候选binary，单独构建jobsimops。只停止自身精确PID，不影响源环境、现有端侧测试或其他实例。

`--report <新的报告路径>`保留历史报告。环境pgPassFile只由进程环境传入，备份写环境backupRoot的独立随机子目录；恢复key仍在运行目录的分离位置，不嵌入备份。

真实加密backup与独立恢复key分目录保管，备份时尚未退休原登录R1；备份后通过当前用户密码完成R1退休，再offline恢复到旧快照。相同restore operation重复执行不得多分配epoch。完成后停止本测试服务，私密环境、DB及加密制品保留审计，不删除外部工程。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、结果与边界</span><!-- jobs-toc:heading-end -->

[22次HTTP／9语义报告](../../报告/登录刷新恢复拒旧实测.json)证明旧access／refresh／Cookie全部拒绝、BFF上游密文清空、旧active RT不恢复授权；备份后退休R1通过独立journal重放，迟到旧初始认证仍401 AUTHENTICATION_RETIRED。新的本人验证可在当前epoch关联未知原请求，Web必须重新登录并确认新BFF家族。

这是同机真实PG／独立journal的隔离恢复证据，不是跨故障域RPO0、PITR链、硬件安全库或完整DBC-05／I验收。报告含实际binary与脚本摘要，后续源码修改和外部缓存清理不改变这份历史执行事实，也不代表旧运行实例仍存在。持久数据与恢复制品不得依赖可被系统／用户整体清理的Cache作为唯一副本。

[新持久环境报告](../../报告/登录刷新持久环境恢复拒旧实测.json)在独立Application Support运行目录及Documents备份重新执行22 HTTP／9语义，绑定7ef53be2候选；未复用旧CA／账号／业务DB，未停止36443端侧共享服务。
