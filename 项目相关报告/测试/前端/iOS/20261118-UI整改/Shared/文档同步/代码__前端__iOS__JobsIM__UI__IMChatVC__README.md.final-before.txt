# <span id="前言">聊天交互</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 一、会话与对方设置 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

聊天 push 隐藏底 Tab。图片/视频选择器请求系统 `.compatible` 表示，当前格式兼容性待设备验证。输入框外的根视图点击结束编辑，手势不取消按钮或消息触摸。右上进入对方设置，提供资料、当前账号本地星标、拉黑/解除、删除联系人和举报；拉黑保留联系人，由服务端禁止双方消息，不再提供允许再次申请开关。长按消息头像打开真实用户资料页。

## 二、消息操作与草稿 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

消息长按可收藏、置顶、举报和选择聊天记录；本人当前可撤回的媒体保留撤回入口，编辑另限本人非空文本。编辑使用 `recallForEditing`：先选择覆盖或保存当前草稿，撤回原消息已确认后才把原文填入输入框；失败不提前替换输入。保存草稿在聊天设置中恢复，当前输入非空时再次确认，恢复不自动删除历史。正式请求沿原消息 ID/版本执行，不用 PATCH 冒充撤回。

## 三、举报与验证 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

举报打开[独立材料页](../IMReportComposerVC/README.md)，由用户明确选择消息和图片/视频。当前仅完成源码、语法解析和构造静态检查；触摸、键盘、长按菜单、图片/视频撤回、草稿冲突和正式举报网络链路待构建后的实际运行验收，未借前轮真机结果判本轮通过。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
