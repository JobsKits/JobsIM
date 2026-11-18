# <span id="前言">JobsIM MacOS 部署代码地图</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、入口与主要方法](#jobs-toc-heading-1)
- [二、服务、信任与退出](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、入口与主要方法 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 入口 | 定位与职责 |
| --- | --- |
| [部署快捷入口](../【MacOS】双击部署（快捷入口）.command) | 从自身位置转交Deployment，保留参数和退出码 |
| [反部署快捷入口](../【MacOS】双击反部署（快捷入口）.command) | 从自身位置转交Undeployment，默认undeploy |
| [部署实现](../Deployment/【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command) | `initialize`、`parse_arguments`、`dispatch_action`确定真实路径、模式及只读边界；`select_im_mode`、`prepare_im_tools`处理键盘选择与官方目标/能力核验 |
| [反部署实现](../Undeployment/【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command) | 独立内嵌相同生命周期，`undeploy_instance`编排仅停/清理、备份/清零及最终确认 |

`origin_begin/origin_record/origin_load/origin_verify_tree`记录并验证新建/复用资源；`resource_protected`及Application最小身份保护保留独立恢复事实和匹配钥。`undeploy_snapshot/undeploy_verify_plan/undeploy_delete_plan`以冷态原计划、设备/inode、SHA及目录全集控制逐项清理；`remove_instance_reference`只处理当前实例匹配引用。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、服务、信任与退出 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

`prepare_clusters/prepare_databases/prepare_tls/start_nats/launch_im_backend`保留真实双PG、按域库、独立journal、NATS及TLS。`process_live/process_matches/nats_process_matches/check_cluster`区分实际活跃进程、已退出僵尸态及未知身份；`backup_instance`逐步传递错误并核解密目录，`require_host_space`按真实字节保留4GiB，不把估算当最低机器验证。

`select_im_mode`在本地模式内默认仅本机，明确公开才准备穿透；`attempt_public_access/recover_private_after_tunnel`处理失败/晚退的严格回本机与菜单重试。`start_im_tunnel`先核私有origin CA，再取得Quick网址并重启本轮Go绑定实际BFF Origin；`write_connection`区分公网系统信任和本地origin CA。`prepare_server_tls/local_api_json`验证服务器SAN/私钥及真实主机名，不能以127.0.0.1证书绕过正式主机验证。`prepare_im_qr`生成离线访问页，`monitor_im_session/start_im_browser_menu`持续监测服务及菜单；`start_session_guardian/cleanup`按本轮归属处理入口/终端异常退出，不触用户其他进程。

实现边界见[平台说明](../README.md)，实际证据与未验项见[测试入口](../../../测试/部署/MacOS/README.md)。

<a id="🔚" href="#前言">回到首页</a>
