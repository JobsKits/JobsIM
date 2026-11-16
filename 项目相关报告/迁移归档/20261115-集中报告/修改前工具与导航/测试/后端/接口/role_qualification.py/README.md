# 群角色准入代际真实验证

[toc]

在受控测试实例执行，使用已有Python标准库和相邻[提及公告测试](../mentions_announcements.py/README.md)的HTTPS客户端；不安装依赖、不自动升级或停止服务。私密环境JSON、测试会话fixture均位于仓库外，fixture创建权限600，报告不含凭据或正文。

本专项分两阶段，保证兼容性证据确实来自旧接口完成事实：

```shell
python3 ./role_qualification.py --environment-file /选择的私密环境.json --prepare
# 由实例owner在安全窗口受控升级；保留CA、deploymentId、epoch及数据。
python3 ./role_qualification.py --environment-file /同一私密环境.json --fixture-file /上一步打印的私密fixture.json
```

prepare只接受没有role_admission的新角色保护前服务，创建随机账号／群／真实消息和一条无role字段已成功公告；不会在已升级服务伪造旧提交。验证完撤销测试会话，因此旧fixture不是可反复登录的凭据包。当前已升级实例不能直接重跑prepare；复测须由owner准备可控旧服务切片，不由脚本回滚或更换产品服务。

验证owner→member→owner期间客户端未观察角色变化：旧未建@all、普通旧意图及公告发布／清除不能借返回资格首次提交；当前读取ACL内旧已成消息和旧无role公告仍可核对，不因补字段改变摘要。成员代际／joinedAt不变、无关成员token不变、同owner／改名不抬角色授权；超过JS精度的roleGeneration使用独立测试成员SQL夹具验证字符串，不冒充真实角色转让次数。

默认报告为`测试/后端/报告/角色准入代际实测.json`及SHA256，另记录旧接口准备14项HTTP与新版46项HTTP／27项语义断言。旧[181／42提及公告报告](../../../../项目相关报告/测试/后端/提及与公告实测.json)按当时源码保留，不将其当作新角色代码的自动回归结果。本专项不证明客户端UI、设备、跨主机围栏、完整恢复或全I验收。
