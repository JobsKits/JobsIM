# <span id="前言">MacOS 成对原生集成</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、运行与授权范围](#jobs-toc-heading-1)
- [二、证据](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、运行与授权范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

已有Python仅作为测试驱动，正式部署不依赖它。执行同目录 `成对原生集成.py --yes`，自动创建全新的源码外私密fixture、独立端口及合成CA/服务器证书，使用真实PTY和fzf操作正式部署与反部署入口，不接管已有state、账户或全局信任。`--report`只能指定新的中央报告子目录。`--local-only`实际验证默认仅本机、自签实例TLS、无隧道启动和仅停保留；它不执行清理。

实际覆盖服务器模式的工具/版本检查、原生双PG/NATS、外置SAN证书、Go/Web构建、严格TLS双账户聊天及默认备份反部署。保留journal、实例最小身份及匹配钥、外置归档和恢复钥，核业务资源清理与端口释放，结果为partial。公网Quick、真实终端GUI关闭、跨机灾备和生产保护由独立实测界定，不由本入口的本地通过推断。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、证据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

原始PTY日志和结果进入[MacOS专项](../../../../项目相关报告/测试/部署/20261118-成对部署反部署/MacOS/)，私有凭据及数据库留fixture而不进报告。保留失败原件及后续审计；原日志发现备份缺项时不能只凭退出0和summary计为通过。

<a id="🔚" href="#前言">回到首页</a>
