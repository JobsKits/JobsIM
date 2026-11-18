# Web 注销与受限数据权利验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 一、执行

在项目根目录运行：

```shell
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_DELETION_REPORT=/本轮未使用的报告目录 \
node 测试/前端/Web/account-deletion.cjs/account-deletion.cjs
```

入口从自身位置定位工程，使用已有Node／Vite／Chrome，不安装依赖。随机回环Vite使用`configFile:false`，不加载开发代理；全部实际`/api`网络阻断。已有报告目录拒绝覆盖；默认输出到中央执行记录的时间／PID唯一目录。启动前有4GiB磁盘门禁，外层运行器每秒检查余量并限制本次子进程组为180秒。

## 二、范围

23组实际Chrome检查覆盖：严格IDB责任先于HTTP、两类未知、404零写与原family明确同体重试、后继family、无普通会话续尾／受限查询、query与cancel用途和有效期、恢复码消费、原取消nonce先落盘、取消两类丢响应、服务器当前版本恢复、换身份／坏DTO／超大响应、未知额外字段拒落秘、旧版本拒回滚、能力撤回、普通API与冷context门禁、prepared后依赖403保unknown、十语、真实管理员表单确认／OTP／清空、完整空态与窄屏。

身份、密码、恢复码、证明和HTTP响应均为显式RAM夹具；测试使用真实React、IndexedDB和WebLocks。它不证明真实HTTPS、当前密码哈希校验、真实TOTP消费、跨集群journal、实际账号数据抹除、OS密码管理器或移动真机。结果和输入源码摘要／快照见[实现报告](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Web注销.md)。

完成注销后不能用旧密码新签证明的限制来自服务端合同；原本浏览器的完整创建上下文仍可只查原责任，短期query证明的有效期也不因本测试延长。
