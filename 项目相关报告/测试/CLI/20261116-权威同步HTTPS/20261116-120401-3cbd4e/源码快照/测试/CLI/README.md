# <span id="前言">JobsIM CLI 测试</span>

![JobsIM](../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、SQLite 与故障边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

```sh
node 运行测试.mjs/运行测试.mjs
```

使用 `GO_BINARY` 可指定用户级 Go。入口从自身位置解析工程，用临时 Go overlay 引入[外置测试](./outbox_test.go)，同时引入 outbox_test.go、governance_test.go 与 collaboration_test.go，不复制测试到业务工程，不依赖当前工作目录。

真实 SQLite／故障测试覆盖发送前持久事务、丢响应沿原ID、同ID异参拒绝、进程重开、账户／恢复代次隔离、旧队列冻结、正文与游标原子提交、大字符串ID、版本／游标防倒退，以及原JSONL显式整批迁入和源文件保护。搜索协作新增旧 map[string]string 摘要兼容、原 @所有人资格跨进程保留／缺能力只读／失效冻结、签名游标原样传回与查询／身份围栏、精确源重新鉴权、公告未知原操作／旧事实不回滚／拒绝正文持久化及 SQL 回滚。新增原发信资格跨进程重启不变、资格失效持久冻结、旧缺值不补齐及无能力声明服务不POST。新增治理原操作落盘后丢响应／重开沿原 ID、删除好友只冻对应 direct、原 payload 字节／hash 不变、退群前历史／游标撤回、注入 SQL 错误整事务回滚、原代际参数和 PATCH 未知结果不重放、schema1→2 保留旧行。治理本地冻结后显式对账只 GET，不因远端撤权请求未达而新建旧消息；既成退群结果确认不回填历史。网络故障由独立HTTP测试服务产生，不用返回本地成功代替。

## 二、真实 HTTPS 联调 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

```sh
JOBSIM_CLI_BINARY=/your/jobsim JOBSIM_CA_FILE=/private/ca.pem node https-integration.cjs/https-integration.cjs
```

[联调入口](./https-integration.cjs/README.md)默认连接 `https://127.0.0.1:8443`，可用 `JOBSIM_SERVER_URL` 指定。真实创建两测试账户，验证私有CA和主机名、好友接受、发送、附件上传／消息／鉴权下载、已有文件不覆盖、进程重启后历史和草稿，以及同机另一账户缓存隔离。追加原操作跨进程保存、本人关系、删友／拉黑／解黑／reopen／新接受、原资格冻结、群改名／加踢／转让／离群，以及重入后旧离群代际不清新缓存或冻结新待发。测试使用不存在的媒体对象制造真实持久失败队列，不伪造本地已发送；SQL 故障与丢响应由前一节独立故障服务覆盖。退出撤销各自测试会话。

私有SQLite状态在临时目录，不保存密码／Token／恢复码；报告只保存断言、执行时间与二进制摘要。结构化结果及日志位于 `报告/`，本轮源码／二进制摘要和限定证据见[治理报告](../../项目相关报告/测试/CLI/20261115-CLI治理/README.md)，局部通过不代表完整I或生产灾备验收。



[普通转发专项](../../项目相关报告/测试/CLI/20261115-普通转发/README.md)：17组SQLite／故障及真实HTTPS转发通过。[转发运行器](./forward-https.cjs/forward-https.cjs)要求已有相应能力的真实后端和受管CLI；不启动假成功API。

## 三、<span id="collaboration">搜索协作专项</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[搜索协作报告](../../项目相关报告/测试/CLI/20261115-搜索协作/README.md)分别保存当前 21 组 SQLite／故障和实际 HTTPS 51 条搜索／收藏分页、精确源、结构化提及、公告与角色门禁结果。[真实入口说明](./collaboration-https.cjs/README.md)要求匹配能力的隔离服务和本轮独立 binary；默认 28443，不覆盖此前普通转发基线产物。

`JOBSIM_CLI_REPORT_DIR` 可选择运行结果目录；全部路径从入口文件或调用者显式配置解析。跨轮数量不相加，完整 I／真机／生产灾备仍未验收。

## 四、<span id="auth-refresh">访问／刷新家族与系统凭据</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[刷新报告](../../项目相关报告/测试/CLI/20261115-刷新家族/README.md)绑定本轮源码与产物。当前外置运行器新增[auth_test.go](./auth_test.go)及[auth_https_test.go](./auth_https_test.go)：31个顶层本机PASS，2个需显式环境的HTTPS／原生库默认跳过；不要用历史21加算当前数量。真实HTTPS协调器另通过原始CA、本人family SQL访问时钟、六并发单刷新及完整多层未知退休；系统库在该协调器测试中是有界内存替身，不能充当OS证据。

[原生终端入口](./auth-https.py/README.md)实测当前Mac系统库不可写时的明确TTY内存模式、实际刷新／唯一消息／对端读取与退出撤家族；非交互默认在认证HTTP前失败。Mac独立签名诊断如实记录DPK写不可用；Windows／Linux只完成amd64跨编译，目标系统安全服务仍未验。所有新增报告在独立目录，原治理／搜索／普通转发证据保留。

## 五、<span id="authoritative-sync">权威同步本地回归</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[sync_test.go](./sync_test.go)的 14 个顶层方法已加入运行器 overlay：独立 physical UUID 与原 Auth 32hex 分离、原登记／快照重开续跑、原 bytes digest／缺页／精确计数、末次分类／权限／迟到 guard、完整原路由／body 只读 proof 与 hash CAS、低版本 global 优先、普通副本资格 hash、能力消失不回退、事务回滚与跨 device 清理意图恢复、已注册库缺 UUID 拒绝。现有方法内另含错路由 global、缺 contentVersion／deleted、可用 forward 查单与旧服务无责任反例。

此前[低空间源码检查点](../../项目相关报告/测试/CLI/20261116-权威同步/README.md)保持原样。资源恢复后，Go1.27.4／GOMAXPROCS=2／`-p=1` 的 MacOS arm64 构建及运行器[完整本地回归](../../项目相关报告/测试/CLI/20261116-权威同步编译回归/20261116-113743-192d48/README.md)通过 45 个顶层方法（旧 31＋新 14），0 失败；真实 HTTPS 和 OS 安全库 2 项因没有显式环境而跳过。首轮 7 个旧夹具失败与 Node24 SIGKILL 原日志保留，最终用既有 Node18.20.1 运行正式入口通过。旧固定 binary／旧报告不更新。

当前运行器包括 auth_test.go、auth_https_test.go 与 sync_test.go；默认 Go 内部超时 120 秒，可用 `JOBSIM_CLI_TEST_TIMEOUT=180s` 等显式时长覆盖。真实 CA／服务 schema1 已由下述独立专项补充，OS 安全库、跨系统原生与生产部署仍须单独验证，不将本地 SQLite／注入 HTTP 视为真实服务验收。

[sync-https.py](./sync-https.py/README.md)随后在固定57／44443完成[10 项真实 HTTPS／PTY 检查](../../项目相关报告/测试/CLI/20261116-权威同步HTTPS/20261116-120401-3cbd4e/README.md)，与 45 项本地集分别记数。只新账号、严格CA、process-only、新 profile／DataRoot，验证原体只读对账、peer 真同 CID 不 ACK 本人、global／撤权／旧 device 库清理；坏媒体实际 403 OBJECT_NOT_OWNED，退群精确云读实际 403 NOT_A_MEMBER。不把拒绝写为未知、不声称 CLI HTTP 零创建计数，OS 安全库正向仍未验；原首轮与旧证据均保留。

<a id="🔚" href="#前言">回到首页</a>
