# JobsIM 部署公共组件

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、模块](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

各系统原生部署／反部署保留自身生命周期，独立辅助工具只负责版本解析、只读依赖报告及离线二维码，不接管实例锁、进程或数据清理。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、模块 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a></span><!-- jobs-toc:heading-end -->

| 组件 | 职责 |
| --- | --- |
| [依赖版本](./依赖版本/README.md) | 查询官方稳定目标、检查精确资产／摘要、比较版本、只读模块报告 |
| [地址二维码](./地址二维码/README.md) | 本地生成终端及HTML／SVG／PNG二维码，不外发部署地址 |

源码和工具引导锁来源于李林哲项目的Jobs自有部署辅助模块，按JobsIM适配；二维码依赖保留原MIT许可及go.sum。编译产物、下载及模块缓存放在机器管理位置，运行卡片归本实例会话，不把第三方源码或SDK混入生命周期实现。原生入口按[部署导航](../README.md)调用。
<a id="🔚" href="#前言">返回前言</a>
