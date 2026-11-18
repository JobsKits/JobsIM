# JobsIM 部署版本解析验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、执行](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

只测试[公共版本组件](../../../部署/公共组件/依赖版本/README.md)，不安装工具、不连接业务库或启动部署。官方元数据的正常、缺件、预发布、重复／坏摘要、错误架构、TLS／限流与取消使用显式transport夹具；真实查询另留当前报告。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">一、执行 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a></span><!-- jobs-toc:heading-end -->

设置`JOBS_IM_GO`为已存在的健康Go绝对路径，再运行[同名入口](./运行测试.mjs/运行测试.mjs)。测试通过临时overlay挂载本目录普通`*_test.go`，不改生产模块或源码锁；运行需要Node.js，仅为测试环境，不增加部署解释器。

```shell
node ./运行测试.mjs/运行测试.mjs
```

覆盖原版本／模块报告合同及JobsIM新增的Node.js LTS资产、NATS精确资产、PostgreSQL不伪造原生包规则。
<a id="🔚" href="#前言">返回前言</a>
