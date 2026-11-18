# <span id="前言">聊天交互</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、会话与对方设置](#jobs-toc-heading-1)
- [二、消息操作与草稿](#jobs-toc-heading-2)
- [三、举报与验证](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、会话与对方设置 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a></span><!-- jobs-toc:heading-end -->

聊天push隐藏底Tab；一对一标题使用当前获权peer／member的显示名或用户名，UID不作为正常标题。图片/视频选择器请求系统 `.compatible` 表示，当前格式兼容性待设备验证。输入框外的根视图点击结束编辑，手势不取消按钮或消息触摸。右上进入对方设置，提供资料、当前账号本地星标、拉黑/解除、删除联系人和举报；拉黑保留联系人，由服务端禁止双方消息，不再提供允许再次申请开关。长按消息头像打开真实用户资料页。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、消息操作与草稿 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a></span><!-- jobs-toc:heading-end -->

消息长按可收藏、置顶、举报和选择聊天记录；本人当前可撤回的媒体保留撤回入口，编辑另限本人非空文本。编辑使用 `recallForEditing`：先选择覆盖或保存当前草稿，撤回原消息已确认后才把原文填入输入框；失败不提前替换输入。保存草稿在聊天设置中恢复，当前输入非空时再次确认，恢复不自动删除历史。正式请求沿原消息 ID/版本执行，不用 PATCH 冒充撤回。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、举报与验证 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a></span><!-- jobs-toc:heading-end -->

举报打开[独立材料页](../IMReportComposerVC/README.md)，由用户明确选择消息和图片/视频。当前冻结build-31通过118单元及UI20同候选4方法，真实Mac普通认证、小林昵称聊天／真实消息、隐藏Tab与单浮标退出已验；材料方法另覆盖实际预览渲染、关闭／移除与本地演示提交。build-22的UI16十方法保留原键盘收起、头像资料、关系设置、撤回保存／恢复草稿等范围，不与当前四项累加。完整媒体撤回／正式官方举报网络链与iPhone全部触控仍待专项，详见[中央证据](../../../../../../项目相关报告/测试/前端/iOS/20261118-UI整改/README.md)，保留失败原件。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
