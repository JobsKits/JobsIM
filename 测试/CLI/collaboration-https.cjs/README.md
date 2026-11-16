# CLI 搜索与群协作真实 HTTPS 验证

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、运行](#run)
- [二、结果与边界](#scope)

</details>
<!-- jobs-toc:end -->

---

## 🔥 <span id="前言">前言</span>

本入口仅用于已有相应能力的隔离 HTTPS 服务。使用现有 Node.js 和受管 CLI，不下载依赖、不启动模拟成功 API，不改系统信任或代理。

## 一、<span id="run">运行</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

```sh
JOBSIM_CLI_BINARY=/your/jobsim JOBSIM_CA_FILE=/trusted/ca.pem \
JOBSIM_SERVER_URL=https://127.0.0.1:28443 \
node collaboration-https.cjs/collaboration-https.cjs
```

从任意工作目录均可使用本脚本的实际路径执行。要求已有 `send_admission/role_admission/mentions/announcements` 能力；匹配实例 CA 与主机名。测试创建两个随机账号、51 条源消息／收藏及少量提及／公告，不使用个人账号；密码和令牌只留进程内存。结束撤销各自会话，私有 SQLite 临时目录与结构化报告分开。

## 二、<span id="scope">结果与边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

断言涵盖搜索／收藏 50＋1 游标分页、精确源变更、结构化提及、公告原结果与当前引用分离、角色新写门禁、源成员退出重入失权及进程重开。没有注入网络错误；原操作丢响应、能力缺失与 SQL 回滚由外置 Go 故障测试独立覆盖。

默认结果写入上级 `报告/20261115-搜索协作/`，可用 `JOBSIM_CLI_REPORT_DIR` 选择实际输出目录。报告只保留断言、时间及二进制摘要，不回显密码、令牌、临时 proof 或私有数据库路径。失败返回非零；局部通过不代表完整 I 或生产灾备验收。


默认报告现直接写入工程根 `项目相关报告/测试/CLI/执行记录/本轮唯一目录/`，Node 使用时间与 PID，Python 使用 UUID，避免覆盖迁入的历史固定文件。原有显式报告参数／环境变量优先；请选择新目录。本轮只核对独立执行目录表达式和脚本语法，没有重新运行业务测试或真实接口。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
