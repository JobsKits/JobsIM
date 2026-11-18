# <span id="前言">联系人与通讯录</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、入口与分区](#jobs-toc-heading-1)
- [二、索引、星标与删除](#jobs-toc-heading-2)
- [三、状态与验证](#jobs-toc-heading-3)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、入口与分区 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a></span><!-- jobs-toc:heading-end -->

联系人为首个根 Tab，根页不重复绘制页面标题。列表内包含滚动搜索、新的朋友、群聊、星标朋友及普通联系人；搜索框滚出后，右上加好友左侧显示独立搜索页入口。点联系人使用 `session.directConversation(user:)` 打开一对一会话，群聊进入现有群会话。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、索引、星标与删除 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a></span><!-- jobs-toc:heading-end -->

索引为 🌟、A–Z、#；中文按拉丁转写，首字符是空格、表情或无法归入 A–Z 的名称归 #。星标保存在当前 namespace 的 `contact-stars` 集合，正式与演示隔离。标准尾部侧滑提供删除；最终弹窗左侧红色删除、右侧手滑，删除联系人关系保留合法聊天历史。好友申请、删除、拉黑和解除使用既有耐久原单服务，不绕过请求身份。

## <span id="jobs-toc-heading-3" data-jobs-toc="v2">三、状态与验证 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a></span><!-- jobs-toc:heading-end -->

列表提供空态与重新加载、下拉刷新；已有账号上下文变化后拒绝旧页写操作。主题采用共享语义色，多语言由统一字符串表提供。当前冻结build-31通过118单元及UI20同候选4方法；真实Mac方法覆盖普通认证后的联系人、小林昵称聊天／隐藏Tab及单浮标退出。build-22的UI16十方法保留原三Tab、关系、档案、草稿和主题等范围，不合并为当前十四方法。通知合并刷新不取消自身同步，严格拒绝不可安装快照并正常以新请求重试。全部索引滚动／跨账户及完整iPhone触控仍按专项验收；原[静态报告](../../../../../../项目相关报告/测试/前端/iOS/20261118-UI整改/ContactsChat/README.md)保留，当前成品与证据见[中央报告](../../../../../../项目相关报告/测试/前端/iOS/20261118-UI整改/README.md)。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
