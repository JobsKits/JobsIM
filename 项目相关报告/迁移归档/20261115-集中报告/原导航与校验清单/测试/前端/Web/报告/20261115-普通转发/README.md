# <span id="前言">Web 普通转发验证</span>

![JobsIM](../../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、限定结果](#jobs-toc-heading-1)
- [二、边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、限定结果</span><!-- jobs-toc:heading-end -->

最终公网JSON／媒体锁版上，普通转发真实HTTPS／PostgreSQL复跑通过：[结果](./forward-result.json)、[原始日志](./真实联调.log)、[当前源码摘要](./验证摘要.json)。[截图](./Web-Forward-Source-Unavailable.png)已实际查看，失效源只显示占位，本人撤回入口保留。

覆盖当前源内容版本／目标资格、纯引用原始Outbox、已提交丢响应刷新后仅查既成结果、未到达时先查404再明确同ID／原body／proof重投一次、本人隐藏编辑保留撤回、源退群清界面及持久缓存正文／附件地址／proof。另覆盖慢POST不回滚已同步的新失效版本、503或200临时占位后旧同version历史不能恢复正文，后续新的精确GET可同version恢复。

构建、十语言／主题与真实IndexedDB原始请求保留／失效占位、错误媒体原点／路径拒绝另有浏览器基线通过。全部既有Web HTTPS主链已复跑通过，[原始日志](./既有链回归.log)单独保留；消息／公告故障测试等待真实提交或拦截完成后再继续，且暂存历史页避免背景同步抢先消掉待发，避免把本地pending出现当成网络完成。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、边界</span><!-- jobs-toc:heading-end -->

该隔离域没有WebDir，测试仅为静态请求提供实际构建Dist文件；所有API、会话和TLS均真实。该域静态部署、全部媒体交互、真机浏览器及全I未据此验收；细节见[运行说明](../../forward.cjs/README.md)。结果只对应上述当前源码与限定范围。

<a id="🔚" href="#前言">回到首页</a>
