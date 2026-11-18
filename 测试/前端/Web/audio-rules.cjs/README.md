# <span id="前言">Web 音频纯规则验证</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、执行](#jobs-toc-heading-1)
- [二、验证范围](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、执行 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

使用已有[**Node.js**](https://nodejs.org)24的原生TypeScript类型擦除，不启动浏览器、Vite或编译，不下载依赖。

```sh
JOBSIM_AUDIO_RULE_REPORT=/本轮未使用目录 \
node 测试/前端/Web/audio-rules.cjs/audio-rules.cjs
```

入口从自身位置解析工程；未给报告路径时在中央音频报告下生成唯一时间目录，已有目录拒绝覆盖。源码与脚本SHA在执行前后核对，成功保存输入副本，失败保留原始堆栈。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、验证范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a></span><!-- jobs-toc:heading-end -->

22组直接执行生产录音控制器、播放白名单／版本转换和十语。录音端口为显式fake，覆盖未点击零许可、重复开始、迟到stream、取消／身份变化、许可拒绝、短录／设备中断／意外停止、最终dataavailable、时长／字节上限；续点覆盖精确身份绑定、global白名单、首次新版本save、迟到旧版本／不同摘要、heard及时间不回退、完成归零和256条界限。

这是实际规则执行，不证明MediaRecorder、麦克风、IndexedDB、UI、真实API或生产构建；原生浏览器证据见[音频报告](../../../../项目相关报告/测试/前端/Web/20261117-Web基础音频/README.md)。

<a id="🔚" href="#前言">回到首页</a>
