# CLI 原生凭据随包构建

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

暂无章节。

</details>
<!-- jobs-toc:end -->

---

从脚本实际位置定位CLI，Go主程序固定`CGO_ENABLED=0`及锁定依赖。显式`--output`保存产物，不初始化数据库、不启动服务、不修改系统信任。macOS另用系统Swift编译[自有助手](../../原生助手/Keychain/README.md)，签名后生成单项SHA256SUMS；Win／Linux只构建当前平台接口，跨平台编译不等于目标系统凭据验证。

```text
python3 Build.py --go <已核验Go> --output <外置发行目录> --sign-identity <已有授权身份> --application-id <实际获权应用标识>
```

本机独立诊断可明确用`--local-adhoc --application-id <独立诊断标识>`替代发行签名。它不伪造TeamID，不代表正式发行或Keychain运行通过；是否可使用DP Keychain仍以实际读写结果为准，拒绝时不降级classic。命令不接收AT、RT、密码或OTP。

BuildReceipt只记录架构／签名性质和资产摘要。脚本不下载工具链，缺Go／Swift／签名授权时直接失败；数据与凭据运行验证由[CLI外置测试](../../../../../测试/CLI/README.md)另行执行。
