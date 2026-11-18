# <span id="前言">Debian · 部署</span>

![JobsIM](../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、唯一实施入口](#jobs-toc-heading-1)
- [二、范围与验收](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、唯一实施入口 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[【Linux】部署JobsIM.sh](./【Linux】部署JobsIM.sh/【Linux】部署JobsIM.sh)为本平台完整自包含Bash实现，入口按自身位置解析项目，不依赖调用目录、旧生命周期库或其他平台脚本。正常动作展示自述、确认及fzf双模式选择；check／plan／status只读。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、范围与验收 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

[平台完整说明](../README.md)定义工具、预算、真实HTTPS／CA、实例归属、独立journal及秘密保留、检查点与失败边界。源码存在或隔离测试通过不代表Linux目标机原生验收。
<a id="🔚" href="#前言">返回前言</a>
