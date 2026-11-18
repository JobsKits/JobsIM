# <span id="前言">RockyLinux 部署代码地图</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、文件定位](#jobs-toc-heading-1)
- [二、函数关系](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、文件定位 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 文件 | 职责 |
| --- | --- |
| [部署脚本](../Deployment/【Linux】部署JobsIM.sh/【Linux】部署JobsIM.sh) | 双模式及本地默认／公开菜单、官方工具择新／健康、双PG与NATS、TLS、实际PublicURL、二维码与前台生命周期 |
| [反部署脚本](../Undeployment/【Linux】反部署JobsIM.sh/【Linux】反部署JobsIM.sh) | 独立内嵌同源函数，原点清单、私有锁、冷备份验真、默认journal保护及逐对象撤回 |
| [平台说明](../README.md) | 配置、路径、身份、能力与未验证边界 |
| [官方版本](../../公共组件/依赖版本/README.md) | 仅版本／摘要解析与只读业务库报告，不接管生命周期 |
| [离线二维码](../../公共组件/地址二维码/README.md) | 只在本机生成实际入口的HTML／SVG／PNG |

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、函数关系 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

`main → parse_arguments → dispatch_action`；部署经`select_deploy_mode / select_local_access / configure_mode_state → capture_resource_origin → acquire_lock → initialize_resource_ledger → prepare_current_tools → prepare_clusters / prepare_databases / prepare_tls → build_source → start_backend_for_url → [明确公开时 attempt_quick_access] → write_connection → monitor_instance / handle_tunnel_exit / recover_local_access`。

反部署经`undeploy_select → undeploy_confirm → acquire_lock → stop_* → undeploy_snapshot → undeploy_verify_backup → undeploy_delete_plan`。任何清理只接受首次created清单及当前精确快照；`undeploy_protected`排除独立journal与匹配秘密，`cleanup`负责本次进程／锁退出，不承担数据删除。各发行版与双入口仅平台常量和默认动作不同，交付脚本本身无生成器依赖。
<a id="🔚" href="#前言">返回前言</a>
