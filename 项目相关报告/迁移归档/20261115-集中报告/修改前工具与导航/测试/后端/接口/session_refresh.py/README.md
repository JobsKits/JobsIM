# JobsIM 登录刷新家族测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 一、环境与入口

执行`python3 测试/后端/接口/session_refresh.py/session_refresh.py --prepare-from-environment <显式隔离环境JSON> --go <Go可执行文件>`：从显式源取得PG工具／CA／独立业务及journal集群连接，在外置新目录创建随机测试DB，以另存的旧`jobsimd-forward-fixed`先产生真实7天会话，再构建当前刷新服务验证迁移。默认32443及内部33101～33108，端口占用停止。旧fixed文件缺失不能伪造“实际旧服务迁移”通过。无SDK下载、代理或全局信任操作。

复用已准备的本测试实例使用`--environment-file <refreshTestRealm环境JSON>`。该JSON及LegacyFixture含秘密，只保存在仓库外600权限目录，不能提交。测试从脚本位置导入邻近共享工具，显式环境决定实际数据位置，不依赖本机Cache路径。服务重启仅本测试记录且可执行路径／端口匹配的PID，保留原CA／身份／epoch；测试不接管其他联调域。

全新持久namespace用`--skip-legacy-fixture --report <新的报告路径>`显式跳过已丢失的旧binary夹具；报告记`realOldSchemaMigration=false`，不能借SQL形状夹具冒称重跑真实旧服务。环境`allowOwnedRestartForTests=false`时跳过BFF重启，不打断并行端侧联调。带空格的用户数据目录由完整启动前缀匹配自身PID，不能用未转义ps输出拆词后误判。PG工具通过显式pgBin／pgPassFile连接，密码不写入命令行。

## 二、已执行范围

[真实实测报告](../../../../项目相关报告/测试/后端/登录刷新家族真实实测.json)记录86次HTTP／43语义断言：旧7天token明确撤销而不变RT、访问过期／单代旋转、旧digest重用撤本family而另一独立设备有效、并发两次只有一代成功、真实TLS socket不读取成功响应后经本人验证受控reauth、初始认证unknown且密文清后由previousRequestId定位撤旧、迟到已退休原认证不建孤立family、退休前序不能分叉，以及不存在账号不能借reauth造号。

Web使用真实CookieJar核上游AT／RT不入JSON、Cookie不是RT、服务端密封BFF、access过期后的CSRF／Origin刷新、同family旧generation拒借新密文、同CookieJar模拟两标签Alice／Bob的读写／logout围栏及BFF进程重启。它不是两个真实浏览器页面；真实双页验证由Web端另附。

闲置／绝对截止、source／security／epoch和大于JS安全整数的代际使用明确的专用SQL夹具；不声称等待了90天。真实账号恢复另经现有恢复码HTTP完成。脚本只读取限流计数并等待当前分钟窗口，不重置计数、不降低生产默认限流。原生安全库落盘、系统锁屏、多标签真实浏览器及完整I验收不由本测试代替。

## 三、独立补证与状态

[Go并发测试](../session_refresh_test.go)在私密`JOBS_IM_REFRESH_TEST_ENV`下连接真实PG与TLS，测试中刻意直接调用当前refresh事务，不走外层OS文件锁，证明user／session事务锁仍保证一消费、一reuse，独立设备继续授权；普通运行器无显式环境时跳过联网。[结构化证据](../../../../项目相关报告/测试/后端/刷新数据库并发单消费实测.json)及[原始日志](../../../../项目相关报告/测试/后端/刷新数据库并发原始日志.txt)不含凭据。

[真实隔离恢复](../session_refresh_restore.py/README.md)证明旧AT／RT／Cookie拒绝、BFF密文清理及备份后退休事实从独立journal重放。各报告绑定实际执行时binary／脚本摘要，不能外推至后来修改。

2026-11-15后续收尾期间，检测到外置`~/Library/Caches/JobsIM`及工具链／PG数据／测试CA均已不在原路径，原服务进程残留，原因由根任务核对。已归档报告和仓库源码保留；此处不宣称当前服务仍可用或最新源码已全部回归。恢复环境须使用新的持久数据位置／namespace，不重建旧空目录伪装原实例；最后未验证差异由根任务明确续跑。

根任务后确认用户清理脚本清空Caches。新工具／运行状态改在用户Application Support/JobsIM，备份在Documents/JobsIMBackups，恢复key分离；新CA／身份／两套物理PG18.7集群均独立创建，没有恢复原账号。官方Postgres.app工具经发行SHA256、磁盘镜像、签名与Gatekeeper核验，Go官方工具链由根任务核验。

新[持久环境与完整退休链报告](../../../../项目相关报告/测试/后端/登录刷新持久环境与完整退休链实测.json)记录102 HTTP／48语义：含原300秒认证未知、R0实际提交却不读响应→R1从未发送→R2完整闭包回收、迟到R1拒建、已有R0→R1边保留、缺后继拒分叉、foreign owner不影响原family。只读auth_retirement_chain区别旧仅auth_refresh服务；vector冻结在新显式意图中，不补旧body。另[新Go日志](../../../../项目相关报告/测试/后端/刷新持久环境Go原始日志.txt)包含backend／CLI测试、无外层OS锁的真实PG并发及独立空库中的legacy SQL行撤旧；后者只是形状夹具，不是真实旧binary迁移。[新恢复证据](../../../../项目相关报告/测试/后端/登录刷新持久环境恢复拒旧实测.json)另记22 HTTP／9语义。历史86／43及旧报告原样保留。
