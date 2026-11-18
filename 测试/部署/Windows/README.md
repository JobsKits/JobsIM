# <span id="前言">Windows 部署隔离测试</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行与边界](#jobs-toc-heading-1)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[唯一测试源码](./生命周期.ps1/生命周期.ps1)从真实部署脚本AST导入函数，不执行Main。SID／ACL和服务存活探针使用明确host夹具；原点、快照、逐对象清理、锁、中断恢复及备份判断使用生产函数。OpenSSL加密解密／服务器证书SAN与匹配钥执行真实工具，PG大小元数据／dump／list在相应用例为明确注入，不使用真实数据库。

```powershell
pwsh -NoLogo -NoProfile -File ./生命周期.ps1/生命周期.ps1 -ReportDirectory <外置结果目录>
```

夹具使用独立随机私有运行目录，不部署／停止／清理用户实例。原始失败与最终输入SHA分开保留，输出[中央证据](../../../项目相关报告/测试/部署/20261118-成对部署反部署/WindowsOS/summary.json)。当前在Mac宿主PowerShell执行，负向原生退出码使用POSIX false，不是Windows原生测试入口。Windows SID、ACL、PE工具执行、双PG、SQL、Job／控制台／关窗、快捷入口双击、完整跨库恢复和公网／手机均未被host通过替代。

对象身份反例以实际宿主 inode／uid／链接数替代 Windows FILE_ID_INFO／SID：覆盖同字节文件替换、Application及state目录换壳、owner／链接数变化、真实硬链接及缺身份旧清单；Windows原生对象API只编译，未执行。最终确认覆盖空行、非空、空白字符与EOF，`-Yes` 不旁路。

<a id="🔚" href="#前言">返回前言</a>
