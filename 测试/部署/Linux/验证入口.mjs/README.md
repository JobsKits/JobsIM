# <span id="前言">验证Linux成对入口</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行与边界](#jobs-toc-heading-1)
- [二、证据](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

使用已有Node测试工具执行同目录验证入口.mjs；测试工具的Node依赖不属于部署生命周期。可用JOBSIM_LINUX_REPORT指定独立证据目录，JOBSIM_TEST_OPENSSL指定已有OpenSSL3，不安装工具。实际断言覆盖新建／复用、旧无清单、受保护journal与实例秘密、备份失败／变更、软硬链接、清零／取消、重复及中断重试。只读版本解析、真实Linux生命周期、Cloudflare公网、TTY／桌面和服务器证书仍分别验收。

```bash
node './验证入口.mjs'
```

备份失败用真实上层 `if dispatch_action`／`if undeploy_instance` 条件调用，注入停服、PG准备、broker清单、dump／list、tar、CMS加密／解密和解密dump复验失败，断言不继续宣称备份或删除。八域dump使用明确替身，CMS由已有OpenSSL3真实完成；不把合成dump当真实PG有效导出。中断重试夹具显式执行真实退出清理的锁释放。

本地默认、Quick失败／晚退回环恢复与继续菜单由生产函数配明确进程／网络替身核验；未启停真实服务或公网。字节预算边界按本轮宿主实际剩余空间计算，不分配巨型文件。`JOBSIM_LINUX_LIBRARY`与`JOBSIM_LINUX_CASE_FILTER`可选择只读旧输入及定向矩阵，输出独立目录，不覆盖原件。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、证据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[报告目录](../../../../项目相关报告/测试/部署/20261118-成对部署反部署/Linux/summary.json)保留源SHA、逐项结果、失败原件及外置临时目录位置；不将不同层的测试数量累加为产品验收数。
<a id="🔚" href="#前言">返回前言</a>
