# <span id="前言">JobsIM Web 正式本人导出验证</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、前言](#jobs-toc-heading-1)
- [二、运行方法](#jobs-toc-heading-2)
- [三、用例范围](#jobs-toc-heading-3)
- [四、真实联调边界](#jobs-toc-heading-4)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、前言 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

本脚本只启动私有Vite和实际Chrome，使用真实React、IndexedDB严格事务与Web Locks，导出HTTP服务、文件接收端和浏览器保存动作由测试注入。私有Vite不读取项目代理，所有浏览器`/api`外网请求阻断，不连接58443／E2或其它后端。

当前脚本含19组Chrome注入用例，覆盖缺少所选Intl语言时的明确数字日期回退；最新执行、类型检查与独立构建见[当前回归](../../../../项目相关报告/测试/回归/20261116-全项目回归/Web-CLI.md)。原18组、首轮类型失败及三张原截图保留在[中央独立入口](../../../../项目相关报告/测试/前端/Web/20261116-正式本人导出/README.md)，不改旧记录、不追认其它包或后端测试。磁盘低于4GiB时脚本在启动Vite／Chrome前退出。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、运行方法 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

运行前准备现有锁定依赖、Node24、Chrome及Playwright，通过`JOBSIM_PLAYWRIGHT_MODULE`指定现成Playwright模块，通过`JOBSIM_EXPORT_WORK`指定工程外持久临时目录。`JOBSIM_EXPORT_REPORT`必须是未使用的新目录。不要为运行脚本下载新SDK、安装全局CA或改低磁盘门禁。

```sh
node 测试/前端/Web/data-export.cjs/data-export.cjs
```

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、用例范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

脚本覆盖：创建成功丢响应的纯查单、原请求未达后明确同体重试、取消原单持久化、能力撤回收尾、原namespace冻结／摘要损坏／排队及迟到身份围栏、真实`apiFile`的`credentials:include`／family/header传递、安全文件名／ZIP完整性／大小／MIME反例、任务UI确认与重载、下载跨账号拒绝、十语三主题和窄屏空态。日期反例明确注入不完整Intl环境，要求实际任务行呈现本地时区的`YYYY-MM-DD HH:mm`，不静默借用浏览器宿主语言。注入保存端不会证明浏览器已将字节持久写入用户磁盘。

## <span id="jobs-toc-heading-4" data-jobs-toc="v2">四、真实联调边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

真实HTTPS需独立新窗口及本人测试fixture，保留正式2b58域身份／CA／DB，且只有受管静态包切换；本脚本不启停任何后端进程。最终计数以中央实际结果为准，未运行项不写PASS。

<a id="🔚" href="#前言">回到首页</a>
