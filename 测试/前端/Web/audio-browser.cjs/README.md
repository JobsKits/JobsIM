# <span id="前言">Web 原生音频浏览器验证</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行](#jobs-toc-heading-1)
- [二、真实与夹具边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

复用已有[**Chrome**](https://www.google.com/chrome/)、[**Playwright**](https://playwright.dev)和Vite，不安装依赖。入口用随机回环地址、`configFile:false`和私有缓存，不读开发代理，实际API网络全部阻断。

```sh
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_AUDIO_BROWSER_REPORT=/本轮未使用目录 \
node 测试/前端/Web/audio-browser.cjs/audio-browser.cjs
```

入口启动前检查4GiB；持续磁盘检查、180秒上限和单worker由本轮[外层运行器](../../../../项目相关报告/测试/前端/Web/20261117-Web基础音频/运行器.cjs)提供，只关闭本次浏览器／Vite与子进程组。报告目录拒绝覆盖，输入摘要前后核齐；不会重启或替换既有服务。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、真实与夹具边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

使用真实React、IndexedDB严格事务、原生MediaRecorder和HTMLMediaElement事件。AudioContext的合成音轨替代`getUserMedia`返回值，不访问真实麦克风；原始合成WebM导出到本轮证据，记录容器MIME／大小并在报告封存SHA。HTTP、身份、上传回调和权限拒绝为显式隔离夹具，App仅实际验证演示空间的composer→现upload接线。

19组覆盖显式开始／停止预览／明确发送、失败保留与同音频重试、迟到stream关闭、许可失败／隐藏中断、原生play-pause鉴权重放顺序、倍速、严格IDB续点／重挂载、403清理、持续播放换auth撤URL、未挂载namespace与archive的global事务清理／其它scope隔离、global迟到save、旧CV／摘要CAS和藏文窄屏。原始403失败及修复后新输入见[报告](../../../../项目相关报告/测试/前端/Web/20261117-Web基础音频/README.md)。

不证明真实麦克风、系统电话／耳机路由、真实HTTPS、媒体上传幂等、Blob点击发送前耐久或断电恢复；这些须独立合同与目标环境验收。本机heard不是服务端已读／收讫资格。

<a id="🔚" href="#前言">回到首页</a>
