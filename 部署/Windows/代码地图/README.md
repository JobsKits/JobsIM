# <span id="前言">Windows 部署代码地图</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、文件与职责](#jobs-toc-heading-1)
- [二、函数与资源](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、文件与职责 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 位置 | 职责 |
| --- | --- |
| [部署快捷入口](../【Windows】双击部署（快捷入口）.cmd)／[反部署快捷入口](../【Windows】双击反部署（快捷入口）.cmd) | 唯一实施包发现、参数与退出码透传，不绕过策略 |
| [部署实现](../Deployment/【Windows】部署.ps1/【Windows】部署.ps1)／[反部署实现](../Undeployment/【Windows】反部署.ps1/【Windows】反部署.ps1) | 相同自包含内核，仅默认动作不同 |
| [平台说明](../README.md) | 模式、数据库、工具、TLS、资源、备份和实际边界 |
| [唯一外置测试](../../../测试/部署/Windows/生命周期.ps1/生命周期.ps1) | 解析及host隔离函数，不启动用户实例 |

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、函数与资源 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

| 入口 | 核心职责／持久资料 |
| --- | --- |
| `Main`／`Select-RunMode`／`Select-DatabaseRoute` | 介绍确认、fzf模式、实例命名空间、自动双PG／高级外部路线 |
| `Find-Tools`／`Resolve-LatestTool` | [公共resolver](../../公共组件/依赖版本/README.md)、当前官方目标与实际健康／原生架构检查，机器版本缓存 |
| `Initialize-ManagedPg`／`Start-ManagedPg`／`Stop-ManagedPg` | 本实例物理双PG、非超级角色、随机秘密及精确PID／路径／SHA；SQL密码仅stdin |
| `Prepare-TLS`／`Prepare-ServerTLS` | 本机实例CA或明确服务器证书，严格SAN／用途／有效期／公私钥检查，原件只读 |
| `Start-Nats`／`Stop-Nats` | `Secrets/nats.*`、`Application/JetStream`、认证就绪及精确停止 |
| `Start-BackendProcess`／`Start-QuickTunnel`／`Wait-InstanceSession` | 独立组／Job，严格origin CA，公网URL后绑定BFF同源，存活监控与网页菜单 |
| `New-AccessQR`／`Expire-AccessQR` | [离线QR组件](../../公共组件/地址二维码/README.md)，本次非秘密地址，退出失效 |
| `Initialize-Origin`／`Seal-Origin`／`Get-ResourceSnapshot` | 部署前原点、逐文件SHA＋Windows FileId／owner SID／链接数、项目／状态路径及保留资源 |
| `Confirm-Undeploy`／`Undeploy-Instance` | 默认备份、默认取消＋独立空行回车复核、备份证明、真实逆操作、外部审计 |
| `Remove-VerifiedSnapshot`／`Get-RetainedRecoveryRows` | 全剩余树CAS、逐对象删除／中断记录、独立journal与恢复凭据保留 |
| `Backup-Instance` | 八域dump＋冷态文件／NATS、CMS加密解密SHA验真，外置备份与恢复钥 |

<a id="🔚" href="#前言">返回前言</a>
