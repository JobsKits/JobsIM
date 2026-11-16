# JobsIM CLI 系统凭据助手

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、功能与边界](#capabilities)
- [二、构建与随包位置](#packaging)
- [三、管道协议](#protocol)
- [四、验证与运行前提](#validation)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本目录提供 Jobs 自有 [**Swift**](https://www.swift.org/) 命令行助手，以系统 [**Keychain**](https://developer.apple.com/documentation/security/keychain-services) 保管 CLI 的完整认证状态。Go 主程序继续采用 `CGO_ENABLED=0`；助手独立编译、签名和随包分发，不依赖 UI 或第三方凭据库。合同见[前端生命周期与凭据保护](../../../../../需求/IM前端架构表.md/IM前端架构表.md#platform-lifecycle)。

## 一、<span id="capabilities">功能与边界</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

- `read`、`write`、`delete` 只操作固定 `JobsIM.CLI` service 与指定的 64 位小写十六进制 account；account 是来源与身份上下文的非秘密摘要，不接受自定义 service 或访问组。
- 一项完整 data 是替换单位，最大 8 KiB；更新失败保留原项，不先删除再新增。新项采用 `kSecAttrAccessibleWhenUnlockedThisDeviceOnly`、`kSecAttrSynchronizable=false` 与 Data Protection Keychain，系统拒绝或锁定时失败。
- 仅匿名标准输入／标准输出传递 JSON，单次请求和响应分别不超过 12 KiB；不接受命令行秘密、环境变量秘密、临时凭据文件或明文回退。
- 只输出 `ok`、`not_found`、`unavailable` 与成功读取的 base64 data；系统错误细节、输入和凭据均不输出到日志。
- Go 调用端限制单次助手调用为 15 秒，摘要及签名核验分别有大小／时间限制。删除不存在的条目成功；安全库读取不存在条目由调用端转换为 `errVaultNotFound`。

## 二、<span id="packaging">构建与随包位置</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

使用受控的 macOS [**Xcode**](https://developer.apple.com/xcode)／Swift 工具链编译本目录 `JobsIMKeychain.swift`，编译参数须包含 `-parse-as-library`，链接系统 `Foundation` 与 `Security`，部署目标至少 macOS 13。分别生成 arm64／x86_64 助手或经验证的 universal 产物；Go 主程序和助手架构必须匹配。源码不携带本机绝对路径、签名证书、私钥或已编译二进制。

发行者须为助手设置稳定的签名标识，使用本人获授权的签名身份及适合 Data Protection Keychain 的应用标识／访问组授权，签名后再计算 SHA-256。需要启用沙盒的发行方式另行验证其授权，不把缺少 entitlement 的运行失败当作钥匙不存在。采用相同签名身份更新，避免后续版本失去原条目的访问资格；正式发行还须按发行渠道完成公证与信任核验。

固定安装布局如下；路径均从实际 Go 可执行文件位置解析，不接受环境 override：

```text
发行目录/
├── jobsim
└── Helpers/
    ├── JobsIMKeychain
    └── SHA256SUMS
```

`SHA256SUMS` 只保留签名后助手的一条记录，格式为 `64位SHA256十六进制摘要`、空白、`JobsIMKeychain`。Go 每次调用前都核验该摘要与 `/usr/bin/codesign --verify --strict`；当前 vault 实例固定首次读取的摘要。`Helpers`、助手与摘要文件不得向组或其他用户开放写权限，助手必须是可执行普通文件，文件本身不得为软链接。发布包来源和目录权限仍须受信任；同一 OS 用户能替换整个发行目录时，sidecar 摘要不构成对该用户的绝对隔离。

## 三、<span id="protocol">管道协议</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

每个助手进程只接收一个 UTF-8 JSON 对象，调用方写完后关闭输入管道。请求形态为 `{"operation":"read|write|delete","account":"64位小写hex","data":"base64"}`；仅 `write` 携带非空 data。响应形态为 `{"status":"ok|not_found|unavailable","data":"base64"}`；仅成功 `read` 携带 data。协议示意中的字符串不是可用凭据，不应把真实请求复制到终端或诊断报告。

CLI 的 Linux 实现固定使用 `/usr/bin/secret-tool` 的 `store`／`lookup`／`clear`，按[GNOME官方实现](https://github.com/GNOME/libsecret/blob/main/tool/secret-tool.c)通过标准输入存储 base64，系统服务不可达时受控失败；Windows 实现通过系统 [**Credential Manager**](https://learn.microsoft.com/en-us/windows/win32/api/wincred/nf-wincred-credwritew) 的 Generic 条目与 `CRED_PERSIST_LOCAL_MACHINE` 保存原始 data，实际最大 2560 字节，超过即失败。上述系统库均不以普通文件或环境变量替代。

## 四、<span id="validation">验证与运行前提</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

2026-11-15已用系统Swift6.4.2、明确macOS27.2 SDK／macosx13目标及-j1编译助手，使用现有本人开发身份严格验签。一个旧身份实际报CSSMERR_TP_CERT_REVOKED，已排除；另一个严格验签通过，但未虚构应用授权时独立非秘密条目read=not_found、write/delete=unavailable，未通过系统库正向回环。[独立状态码](../../../../../项目相关报告/测试/CLI/20261115-刷新家族/Mac系统库独立状态码.json)确认写／删=-34018，等于errSecMissingEntitlement；准确结果见[本轮诊断](../../../../../项目相关报告/测试/CLI/20261115-刷新家族/Mac系统库诊断.json)。启动因此实际写／删探测，不把“读不到条目”误当“可写”；不可用时仅明确TTY临时模式，默认无人值守在认证HTTP前失败，见[终端证据](../../../../../项目相关报告/测试/CLI/20261115-刷新家族/README.md)。

正式系统库验证仍需覆盖具备实际应用授权的读写替换／删除、锁定、错误摘要／缺失助手、8 KiB上界、破损JSON和管道超时；Windows／Linux已作amd64类型及链接构建，尚未目标系统运行。Windows另验2560字节上界，Linux验Secret Service存在、解锁、不可达和权限；不据跨编译宣称这些已通过。

本机诊断可把 adhoc 签名与明确的 `com.apple.application-identifier`、仅含一个同值条目的 `keychain-access-groups` entitlement 作为候选验证路线：使用单独的本地诊断标识、不伪造发行 Team ID，先核对嵌入的 entitlement 与严格签名验证，再仅用独立新 account 和非秘密测试数据做完整读写／删除，最后清理该诊断条目。是否被当前系统接受必须以真实结果判断；缺少授权、系统拒绝或 `unavailable` 都保留为失败证据，不改用 classic keychain。adhoc 没有证书链／CMS 发行者证明，即便本机操作成功也不作为正式签名、升级兼容或发行信任验收。

Go 的跨进程认证锁在私有认证目录内固定使用 `.auth.lock`，macOS／Linux 使用非阻塞 `flock`，Windows 使用 `LockFileEx`。持锁期间不删除或替换该锁文件；进程退出由 OS 释放锁，调用方正常结束须执行返回的释放函数。Unix 认证目录和锁文件只允许当前用户访问；Windows 目录需由认证状态管理层置于当前用户的私有数据根并核验其访问权限。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
