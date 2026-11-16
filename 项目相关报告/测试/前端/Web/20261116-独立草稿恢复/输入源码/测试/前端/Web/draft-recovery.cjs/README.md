# Web独立草稿恢复浏览器专项

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

真实Chrome、React、IndexedDB及Web Locks，运行当前Jobs自有SelfDraftRecovery／SelfStateActions／SyncStore。业务HTTP函数和剪贴板接收端为显式注入，不是实际后端或OS剪贴板验收；不读取seed、Cookie、账号或CA。全部浏览器`/api`请求在发出前阻断，本脚本自行启动无proxy、随机回环端口的私有Vite；不加载项目开发代理，不连接E2或其它服务。

## 一、<span id="run">运行</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

```sh
JOBSIM_PLAYWRIGHT_MODULE=/已有/playwright模块 \
JOBSIM_RECOVERY_REPORT=/本轮新空报告目录 \
JOBSIM_RECOVERY_WORK=/本任务外置工作目录 \
/已有/node24 测试/前端/Web/draft-recovery.cjs/draft-recovery.cjs
```

工程路径从脚本位置定位；复用当前Web依赖和已有Chrome，不下载／安装依赖。`BROWSER_EXECUTABLE`可指定已有Chrome；未指定使用macOS标准应用位置。`JOBSIM_RECOVERY_WORK`缺省为系统临时目录，`JOBSIM_RECOVERY_REPORT`缺省为中央执行记录唯一目录；已有报告目录拒绝覆盖。Vite／Browser均在finally关闭，只操作本入口创建实例。

## 二、<span id="cases">当前10组</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- 候选核本人／epoch／kind／channel／UUID，非当前或只读draft，以及无S2投影的retired原操作均可发现。
- 列表不呈现metadata／原body／proof；点选只精确GET获准独立文字，不伪造Conversation或读源消息。
- 原单404固定原body的只读draft-results，实际IndexedDB读回原nonce／route／method／hash未变，零PUT／DELETE。
- 明确复制当前GET.ownedText或GET.recovery，复制前复核所见version／text／recovery operation；测试剪贴板是注入sink。
- 远端变化不偷偷替换用户所选复制内容。
- 清空确认冻结精确大版本，旧CAS拒绝；正确clear保持另会话完整本地Space／输入字节不变。
- GET未知及flag撤回不从metadata或旧body填回当前文字，保留错误与重载。
- 旧family／namespace迟到GET不能进入新mount的完整空态；空态可重载。
- 实际Web Locks排队clear在原family失效后取消，等待锁尾完成再确认没有创建写。
- 十语言和三种主题解析均无移动宽度横向溢出，选中提示有主题色／边框；保中英藏截图。

源码SHA、截图与阶段日志见[本轮中央报告](../../../../项目相关报告/测试/前端/Web/20261116-独立草稿恢复/README.md)。资源watchdog由本轮执行协调器独立提供180秒／4GiB门禁，不宣称脚本自身常驻监测；真实服务、认证／CA、离群授权、系统剪贴板／存储耐久和完整I另验。

<a id="🔚" href="#前言">回到首页</a>
