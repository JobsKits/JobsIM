# <span id="前言">JobsIM 代码</span>

![JobsIM](./前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、独立实现](#jobs-toc-heading-1)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、独立实现 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

- [Go后端](./后端/Go/README.md)：真实PostgreSQL、按域HTTP角色、统一[接口契约](./后端/Go/接口契约/README.md)。
- [Web](./前端/Web/README.md)：React/TypeScript、同源会话、IndexedDB。
- [iOS](./前端/iOS/README.md)：Swift/UIKit、JobsByPods与原生存储。
- [Android](./前端/Android/README.md)：Kotlin/Compose、Room。
- [HarmonyOS](./前端/HarmonyOS/README.md)：ArkTS/ArkUI、RDB。
- [CLI](./前端/CLI/README.md)：Go命令行与稳定待发身份。

各端实现各自原生界面，共用服务协议。AI查询使用工程根的实际`.codegraph/`机器索引与`codegraph.json`配置，重建及查询见[索引说明](../项目相关报告/测试/工具/20261118-CodeGraph/README.md)；各端README中的“代码地图”仅是人工职责导航，不代替CodeGraph。需求以[需求目录](../需求/README.md)为唯一产品目标，代码状态见[实现状态](../项目相关报告/实现进度/README.md)。

<a id="🔚" href="#前言">回到首页</a>
