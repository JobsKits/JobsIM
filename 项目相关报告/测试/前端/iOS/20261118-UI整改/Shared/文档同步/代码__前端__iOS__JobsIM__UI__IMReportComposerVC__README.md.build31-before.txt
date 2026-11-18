# <span id="前言">举报材料编排</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 一、用户选择与提交 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

举报包含目标类型/ID、原因、最多 50 条用户选择的消息 ID，以及最多 10 个图片或视频材料。列表显示文件名、MIME类型和大小，按字体采用最小88pt行高；支持嵌入IMBaseVC导航的原生QuickLook预览与明确完成按钮、滑动删除；提交前独立确认材料数量。不自动上传整个会话，不借普通聊天媒体接口传举报证据。图片上限 20 MiB，视频 64 MiB；选择器显式请求系统 `.compatible` 表示，不自行转码或压缩证据，名称和 MIME 按系统返回内容展示。系统对 HEIC 原片及视频返回的实际格式与后端兼容性仍待当前设备验证；服务端仍重新校验。

## 二、文件归属与原单恢复 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

选择器回调在原账号 namespace 和演示状态内验证，并检查账号未注销。界面副本写入 ApplicationSupport/JobsIM/ReportDrafts/<namespace.key>/<UUID>，目录登记所属 namespace，拒绝符号链接；副本普通文件使用私有权限及系统数据保护。共享 submitReport 在首次网络等待前复制自己的 ReportOutbox 耐久原单；界面退出、成功及无效迟到回调只清理所属界面副本。我的举报页可恢复未完成原提交，不能用新内容改写旧请求身份。

## 三、验证边界 <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

当前build-22／device-build-7已构建验签，113单元和UI16同候选10方法通过；材料方法实际覆盖PHPicker选择本任务公开图片夹具、回调私有复制、文件名／MIME／大小、原生QuickLook关闭、滑删／重选及本地演示提交。真实官方后台上传与未知原单恢复、HEIC／视频格式全链、iPhone完整材料操作不由本地模拟成功追认；当前证据和失败历史见[中央报告](../../../../../../项目相关报告/测试/前端/iOS/20261118-UI整改/README.md)。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
