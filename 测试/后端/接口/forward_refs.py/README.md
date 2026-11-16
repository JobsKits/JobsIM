# JobsIM 普通转发真实测试

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [一、运行](#jobs-toc-heading-1)
- [二、证据与边界](#jobs-toc-heading-2)

</details>
<!-- jobs-toc:end -->

---

## <span id="jobs-toc-heading-1" data-jobs-toc="v2">一、运行</span><!-- jobs-toc:heading-end -->

从仓库根目录执行`python3 测试/后端/接口/forward_refs.py/forward_refs.py --environment-file <独立转发测试域私密environment.json>`。只连接显式HTTPS实例，CA校验保持开启；创建本轮随机账户，结束撤本轮会话。数据库连接、密码、原proof和token不进报告。脚本读取邻近共享HTTP测试工具，不需要安装测试框架。

服务须有forwarding及相关父能力；真实AAC-M4A／MP4／GIF使用已安装ffmpeg生成合成样本，不录用户麦克风，不下载媒体。上传PNG、GIF、音频、视频和文件后以目标消息鉴权代理核字节／SHA／Range；原对象仍不向目标授权。60MiB慢流测试会短暂阻塞实例内容写，只可在独立受控域运行，不能占用其他端在途窗口。

## <span id="jobs-toc-heading-2" data-jobs-toc="v2">二、证据与边界</span><!-- jobs-toc:heading-end -->

[真实闭环报告](../../../../项目相关报告/测试/后端/普通转发真实闭环.json)记录185次HTTP／104语义断言：纯引用行无正文／attachment、当前reader proof、同原ID幂等／真实未读成功响应、链深8／拒9、独立目标ACL、源edit／recall／离群重入／账号恢复撤权、普通logout保留资格、当前搜索收藏及五类真实媒体。HTTP轮询次数可随worker调度变化，不能当产品验收项数。

独立补证见[JSON与持久续跑](../forward_fences.py/README.md)、[真实恢复代际](../forward_restore.py/README.md)、[最终候选既有回归与导出明确拒绝](../forward_compatibility.py/README.md)。每份报告绑定执行时源码／binary快照；后续仅改实验导出错误可解释性时，不能把旧主报告冒称为新binary全量重跑。

[固定构建摘要](../../../../项目相关报告/测试/后端/普通转发固定构建快照.json)汇总最终binary及所有证据校验和；本轮独立28443域只供测试，主8443等域是否已受控升级由根任务另行确认。

当前forwardUnavailable是本次投影不可用，可包含临时未知；客户端精确GET可在同version恢复，不能永久锁存boolean。永久ref失效由服务端状态／版本保障。实验导出含forward仍明确拒绝，尚无链级导出holder；不证明跨主机／强保护、全媒体物理GC、端侧耐久ACK或完整I及243项验收。

默认报告现直接写入工程根 `项目相关报告/测试/后端/执行记录/本轮唯一目录/`，Node 使用时间与 PID，Python 使用 UUID，避免覆盖迁入的历史固定文件。原有显式报告参数／环境变量优先；请选择新目录。本轮只核对独立执行目录表达式和脚本语法，没有重新运行业务测试或真实接口。
