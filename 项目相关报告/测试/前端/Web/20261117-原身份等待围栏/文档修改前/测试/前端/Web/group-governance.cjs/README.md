# Web 群治理与邀请原单验证

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、执行

```shell
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_GROUP_REPORT=/本轮未使用的新目录 \
node 测试/前端/Web/group-governance.cjs/group-governance.cjs
```

脚本从自身位置定位工程，使用既有Node／Vite／Chrome，随机回环Vite以`configFile:false`运行，不读开发代理，全部实际`/api`网络阻断。已有报告目录拒绝覆盖；默认写中央执行记录的时间／PID唯一目录。不安装依赖、证书，不接真实账号。

## 二、范围

26组实际React／IndexedDB／WebLocks反例覆盖：原body先耐久、actor与target所见代际、角色ABA、大代际、两种未知、404零POST、旧family／设备只读、邀请token不入库、标准base64 UTF8跨实例拒绝、同码冷启动续单、revoke／decide原actor查单、独立恢复RAM OTP／step-up、身份变化、三列表分页／重复游标、真实事务abort、能力撤回、坏结果／未知额外字段、十语、申请人姓名、真实策略确认、目标props更新不重基、复制前撤销、实际恢复表单即时清空、实际入群码清空及藏文窄屏空态。

HTTP、身份、服务端操作与剪贴板为明确RAM夹具，不证明真实HTTPS、真实MFA消费、实际群成员写入或系统剪贴板交付。后端新增manager original与issuer frozen反例在[Go用例](../../../后端/接口/group_manager_original_test.go)分轮执行真实随机PG，数据与计数不能和浏览器组相加为产品项。

入口启动前检查4GiB；本轮外层watchdog每秒检查余量、每子进程180秒，只停自己的进程组。原输入与成功源码快照、构建与历史失败边界集中到[群Web闭环报告](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Web群治理.md)。
