# 本地单元测试配置生成器

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、运行与输出](#usage)
- [二、隔离与验收边界](#boundary)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

由已审核的旧`.xctestrun`只提取结构，用显式传入的当前模拟器产品生成单元测试白名单配置；不编译、不运行测试、不接业务API、不安装依赖。

## 一、<span id="usage">运行与输出</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

依赖系统现有[**Python**](https://www.python.org/) 3标准库。先完成当前源码的`build-for-testing`，再调用：

```shell
python3 生成隔离单元配置.py --template 已审核模板.xctestrun --products 当前DerivedData/Build/Products --output 新报告/本地单元.xctestrun
```

所有路径均由参数显式指定；模板中的`__TESTROOT__`替换为当前产品根，系统占位仍由[**Xcode**](https://developer.apple.com/xcode/)解析。现有输出拒绝覆盖，同目录另写JSON摘要记录实际host与测试二进制SHA。生成失败返回非零，不执行旧产品。

## 二、<span id="boundary">隔离与验收边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

只保留`JobsIMTests`项及FormatVersion=1元数据，移除UI runner依赖和命令行参数；十一类本地方法由配置内`OnlyTestIdentifiers`限定，打印相同的CLI选择器供双重限制。两组host环境都在启动前注入`JOBS_IM_SYNC_INJECTED_TESTS=1`，使共享会话进入内存库且不恢复原Keychain／连接。测试自身的记住密码用例只读写随机UUID档案专属Keychain并在退出清除，不查询用户真实密码。

生成器检查当前二进制含白名单类及模拟器产品身份。配置生成、测试实际执行、源码与产物摘要核对分别取证；此脚本不能将旧包或未运行的方法认定为通过。全套集成测试含固定服务端口和真实HTTP入口，不能去掉白名单后直接运行。

<a id="🔚" href="#前言">返回前言</a>
