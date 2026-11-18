# <span id="前言">Web 附件耐久原单纯规则</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行](#jobs-toc-heading-1)
- [二、检查边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

使用已有Node.js 24原生TypeScript类型擦除直接导入生产规则，无依赖安装、浏览器、编译输出或网络。

```sh
node 测试/前端/Web/attachment-rules.cjs/attachment-rules.cjs
```

输出到[本轮报告目录](../../../../项目相关报告/测试/前端/Web/20261117-附件耐久发送/README.md)下新的`纯规则-时间戳/`，记录实际断言、运行时间及规则／脚本输入前后SHA；不覆盖原件。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、检查边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

直接检查原身份／8字段manifest／Blob／回复CV白名单、8 MiB语音与32项／64 MiB同scope限额、prepared不可当收据、原对象ID不可更换、完整user／conversation／client映射及坏holder拒绝。独立最小拒旧floor覆盖任务GC、精确去重、冲突保原值、损坏／跨scope／多余秘密字段、structuredClone后的boxed String UUID拒绝；十语文案键完整。

这些是有意义的纯规则反例，**不证明真实IndexedDB事务、HTTP、WebLocks、类型检查、录音设备或生产包**。真实浏览器边界见[附件浏览器入口](../attachment-browser.cjs/README.md)。

<a id="🔚" href="#前言">回到首页</a>
