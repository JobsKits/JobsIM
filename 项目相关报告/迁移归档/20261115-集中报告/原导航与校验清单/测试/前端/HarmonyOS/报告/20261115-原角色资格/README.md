# <span id="前言">鸿蒙原角色资格保护</span>

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、修改与边界](#jobs-toc-heading-1)
- [二、证据](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、修改与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

群公告新意图冻结创建时的`roleGeneration`与成员代际、原requestId/CAS/截止，不能把恢复群主后的角色资格补进旧body。`@全体`和公告写入前核对当前实例`role_admission:true`；旧服务只允许查原已成结果，不借旧服务首次创建。明确资格拒绝后冻结消息或终结归档原公告；频率限制仍保留原意图等待明确重试。

2026-11-15：23项主机纯规则实际通过，应用HAP及23用例Hypium测试HAP编译通过，均未签名。主机用例含旧body不补角色、新body原角色冻结和能力门禁；不是鸿蒙设备RDB／UI／HTTP执行。HDC目标为空、设备Hypium／真机仍未验。前一[提及公告与分页证据](../20261115-提及公告与分页/README.md)保持，不拿旧HAP算本轮通过。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、证据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

- [应用构建日志](../20261115-120902-65997/运行日志.txt)与[源码清单](../20261115-120902-65997/source-manifest.json)，9.038秒。
- [测试HAP构建日志](../20261115-121031-97776/运行日志.txt)与[源码清单](../20261115-121031-97776/source-manifest.json)，13.566秒。
- [包CRC／SHA与当前ArkTS源码摘要](结果.json)。Dist后续构建可能更新，以本报告摘要锁定对应产物。

仍复用libsdl镜像工具链和离线缓存，没有新SDK下载、系统CA／代理变更或第三方源码改动。

<a id="🔚" href="#前言">回到首页</a>
