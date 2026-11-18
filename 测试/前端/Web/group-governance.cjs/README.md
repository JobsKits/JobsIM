# <span id="前言">Web 群治理与邀请原单验证</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、执行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

```shell
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_GROUP_REPORT=/本轮未使用的新目录 \
node 测试/前端/Web/group-governance.cjs/group-governance.cjs
```

脚本从自身位置定位工程，使用既有Node／Vite／Chrome，随机回环Vite以`configFile:false`运行，不读开发代理，全部实际`/api`网络阻断。已有报告目录拒绝覆盖；默认写中央执行记录的时间／PID唯一目录。不安装依赖、证书，不接真实账号。

## 二、范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

29组实际React／IndexedDB／WebLocks反例覆盖：原body先耐久、actor与target所见代际、角色ABA、大代际、两种未知、404零POST、旧family／设备只读、邀请token不入库、标准base64 UTF8跨实例拒绝、同码冷启动续单、revoke／decide原actor查单、独立恢复RAM OTP／step-up、身份变化、三列表分页／重复游标、真实事务abort、能力撤回、坏结果／未知额外字段、十语、申请人姓名、真实策略确认、目标props更新不重基、复制前撤销、实际恢复表单即时清空、实际入群码清空及藏文窄屏空态。

共享transport另包含三组真实模块反例：空闲刷新等待时切换family必须零HTTP；相同family更新refreshGeneration可继续原请求；access过期完成刷新后的切换不能以新family重放原body。两组跨family反例检查原`AUTH_REQUEST_ABORTED`与后继身份保留，不只依赖组件收到响应后的检查。

HTTP、身份、服务端操作与剪贴板为明确RAM夹具，不证明真实HTTPS、真实MFA消费、实际群成员写入或系统剪贴板交付。后端新增manager original与issuer frozen反例在[Go用例](../../../后端/接口/group_manager_original_test.go)分轮执行真实随机PG，数据与计数不能和浏览器组相加为产品项。

入口启动前检查4GiB；持续watchdog及180秒上限由外层运行器提供，脚本本身仅在finally关闭本次Vite／浏览器。本轮有效修前／修后直接入口执行与后续有watchdog的关联回归边界分别记录在[原身份围栏报告](../../../../项目相关报告/测试/前端/Web/20261117-原身份等待围栏/README.md)，不把其它轮的监控口径外推。本轮输入、成功源码及截图独立保留；前序原输入与构建见[群Web闭环报告](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Web群治理.md)。

<a id="🔚" href="#前言">回到首页</a>
