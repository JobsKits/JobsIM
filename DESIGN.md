# JobsIM 界面设计

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、用途与来源](#jobs-toc-heading-1)
- [二、设计变量](#jobs-toc-heading-2)
- [三、状态与验证](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、用途与来源</span><!-- jobs-toc:heading-end -->

Operate：日常沟通应用。产品和当前阶段依据现有需求及用户明确的原生优先授权。视觉参数为Codex可逆工程默认，不记为用户逐项审批。

实际采用jobs-frontend-experience组织任务闭环；ui-ux-pro-max提取原生触点、可见焦点、主题与状态；frontend-design明确色彩／字体／布局；impeccable context及craft-floor用于克制的操作界面和实际截图；web-design-guidelines用于语义、焦点、错误及输入检查。检索工具的营销Hero建议不适用于聊天工作区。方向seed为seed:a0240c8d2089656b，第5候选采用安静的对话工作台：熟悉导航、信息节奏和清晰消息层次，视觉服务日常操作。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、设计变量</span><!-- jobs-toc:heading-end -->

- 主色#287C66，浅色底#F5F7F4、白面#FFFFFF、正文#263A34；深色底#18201C、面#202A26、正文#E2ECE5、主色#91CFB7。
- Web自托管Source Sans 3，中文及藏文走系统字体回退；原生各自采用平台文字语义和缩放。
- Desktop为76px导航／320px会话／弹性消息；小屏按会话列表→消息重组，保留原生平台返回行为。
- 图标先查询iconfont，页面未能取得可核授权资源；Web复用官方Lucide(ISC)本地构建，各原生端用系统／Material图标。Logo为自绘几何对话标志，姓名字母头像为语义回退。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、状态与验证</span><!-- jobs-toc:heading-end -->

演示空间持续显示身份提示，真实账号离线保留缓存、草稿和稳定Outbox。按钮有可见焦点、输入法Enter不误发；Dialog支持Escape、Tab循环和焦点返回。三主题／十语和移动溢出有浏览器验证。实际截图在测试/前端/Web/报告；各原生截图与结果归各自测试目录。缺目标SDK的端不宣称视觉或真机验收。
