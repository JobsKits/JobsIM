# <span id="前言">登录门样式与独立设置整改</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、当前源码](#changes)
- [二、静态证据](#evidence)
- [三、待验与保护边界](#pending)

</details>
<!-- jobs-toc:end -->

---

本目录单独保存后续登录门整改的输入和静态证据，不覆盖前轮AuthRoot、真机签名或旧UI测试结果。当前为源码与静态检查层，尚未把本轮源码编译、安装或做实际截图；后续统一原生验证由主流程记录。

## 一、<span id="changes">当前源码</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

认证卡片使用18／14／12pt字号、居中标题、带底色和描边的输入框、两密码眼睛、左记住密码与右找回入口；中间薄玻璃、两侧实心64pt切换栏。背景使用真实打包山湖图，通过ImageIO直接缩采样至1600像素；源码可配置本地静音视频并在离页／后台／减少动态效果时暂停，当前默认图片。

原地址、连接、CA与验真块迁入新独立设置页。连接异常保留左取消／右前往设置的系统对话框。记住密码仅接主流程提供的当前profile服务，成功认证和verified pair均匹配后才保存；切实例清旧输入与请求签名。演示开关使用专用服务，候选明文预填但不自动登录、不记住密码；删除原Auth本地演示直入按钮。

24新键集中补齐十语，其中12为本轮Auth／服务状态、12为独立资料／演示浮标协作；仅简繁原avatar文字去“静态”。源说明见[登录门](../../../../../../../代码/前端/iOS/JobsIM/UI/IMAuthVC/README.md)及[登录设置](../../../../../../../代码/前端/iOS/JobsIM/UI/IMAuthSettingsVC/README.md)。

## 二、<span id="evidence">静态证据</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

> 结果汇总见 [summary.json](./summary.json)，冻结输入见 [inputs.json](./inputs.json)。

| 检查 | 原件与范围 |
| --- | --- |
| 修改前基线 | [baseline.json](./baseline.json)及`before/`保留原AuthVC与十语，不覆盖已有未提交工作 |
| Swift语法 | [swift-parse.log](./swift-parse.log)，本轮11个自有Swift文件，仅frontend parse，不是typecheck或build |
| 构造与DSL | [construction-audit-all.log](./construction-audit-all.log)为全自有只读扫描；他域命中已报主流程，不能把局部零问题说成全仓通过 |
| 十语 | [localization-validation.json](./localization-validation.json)，每语437键、24新增、键集一致、无重复／空值，`plutil`均通过 |
| 主题静态下界 | [contrast.json](./contrast.json)按半透明底板最不利底图合成；仍待真实稳定态截图复核 |

## 三、<span id="pending">待验与保护边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

未运行构建／模拟器／设备操作。布局、键盘、三主题、十语、眼睛与记住密码、设置push／返回、服务器异常弹窗、CA导入及专用Demo主机仍待实际验收；视频路径当前只有源码，不借图片默认态称视频实测通过。

没有改App／Scene、IMSession、第三方Pods、测试target或其它代理拥有的聊天／资料源。Remember／Demo服务及主机配置由主流程维护，当前只是调用其接口；素材来源／许可也由主流程登记，不在本目录复制或暴露演示凭据。

<a id="🔚" href="#前言">返回前言</a>
