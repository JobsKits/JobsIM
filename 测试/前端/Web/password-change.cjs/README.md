# <span id="前言">Web 本人主动改密验证</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、运行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

运行当前工程的真实Chrome／React、IndexedDB严格事务与Web Locks；复用已有Node、Vite与Playwright，不安装依赖。密码、响应及BFF行为均为显式隔离夹具；全部实际API请求阻断，只有冷context与Cookie安全反例由精确浏览器路由返回夹具，不连接真实后端或现有账户。

```sh
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_PASSWORD_REPORT=/本轮新的报告目录 \
node 测试/前端/Web/password-change.cjs/password-change.cjs
```

入口从自身位置定位工程。未指定报告目录时写入中央Web执行记录的时间／PID唯一目录，已有目录拒绝覆盖。`BROWSER_EXECUTABLE`可指定现有Chrome；私有Vite配置不读取开发代理，使用随机回环端口。本脚本在启动前检查4GiB磁盘下限；持续磁盘检查和180秒上限由外层协调器提供，未内置无限重试或下载。finally仅关闭本次Vite与浏览器。

## 二、验证边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

覆盖非秘密原责任先于HTTP、提交与投递两类丢响应、原ID纯查／显式重试、匿名已准备续尾、后继family及Cookie隔离、实时能力／CSRF期间身份改变、错误DTO／原证明／首拒、冷启动恢复门禁、真实IDB中止、WebLocks排队失效、NFC和显著空格、未知额外字段拒落秘、真实表单确认／显示密码／立即清空，以及十语三主题窄屏。密码与OTP只在本次浏览器RAM夹具中；报告不保存实际账户证明或HTTP原body。

结果、输入源码SHA和成功轮源码快照写入本轮目录。各组为独立安全／行为断言，不等于全部需求验收，也不证明真实服务的密码校验、TOTP消费、journal跨域撤销、系统密码管理器或用户真实账户改密。

当前执行及初次失败原件见[闭环报告](../../../../项目相关报告/实现进度/20261116-剩余能力闭环/Web改密.md)。

<a id="🔚" href="#前言">回到首页</a>
