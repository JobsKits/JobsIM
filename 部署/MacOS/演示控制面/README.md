# JobsIM 原生演示宿主

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、使用](#usage)
- [二、隔离与信任](#isolation)
- [三、固定控制接口](#protocol)

</details>
<!-- jobs-toc:end -->

## 🔥 <span id="前言">前言</span>

当前 Mac 为 iPhone 提供一个独立的原生演示实例。入口为[启动演示控制面](./【MacOS】启动演示控制面.command/【MacOS】启动演示控制面.command)，实现源码为 [Go 控制器](../../../代码/后端/Go/cmd/jobsimdemo/main.go)。完整项目目录可移动，入口从自身位置解析依赖；生成的控制器产品保存在本目录 `Products/`，独立运行数据从当前用户系统目录解析，不固化开发机路径。

## 一、<span id="usage">使用</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

1、让 Mac 和 iPhone 接入同一可信局域网，双击入口并阅读自述后回车确认。
2、入口复用已安装的 [Go](https://go.dev/)、[Node.js](https://nodejs.org/)、[PostgreSQL](https://www.postgresql.org/) 与 [NATS](https://nats.io/) 工具，离线构建当前后端和 Web 静态资源，保持有限 HTTPS 控制面在前台。
3、将生成的公开配对描述打包入本机构建的 iOS App；在登录右上设置启用演示，控制面确保专用服务启动并核验预埋非管理员账号。App 明文预填候选密码，由用户点击登录。
4、App 关闭演示或点浮标仅断开前端并回登录。Mac 入口收到 Ctrl+C 后精确停止自身启动的 demo 子进程，保留数据库、对象、CA 与公开配对信息；不提供远程停止、删除或重置入口。

## 二、<span id="isolation">隔离与信任</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

固定实例位于用户应用支持目录 `JobsIM/Demo/NativeDemo-v1/`，控制面19544、业务 HTTPS19543只绑定明确的本机私有 LAN IPv4；业务 PG19545、物理独立 journal PG19546、NATS19547及角色19560～19568使用各自归属。原9443实例不参与此流程。NATS按既有最低资源合同配置512MiB磁盘与32MiB内存，重型准备保留4GiB宿主空间。

控制面和业务使用同一专属 CA，证书含实际 IP SAN，客户端核对 CA 指纹、证书链、主机名和 bootstrap 部署身份。公开描述生成至 `代码/前端/iOS/JobsIM/Resource/DemoHost/demo-host.json`，仅包含 controllerURL、CA公钥证书、SHA256及demo标记；没有密码、私钥或数据路径。该文件和 `Products/` 是本机生成物，不作为通用静态配置提交。换机须在目标 Mac 准备并重新配对，地址／CA改变时拒绝静默覆盖原信任。

## 三、<span id="protocol">固定控制接口</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

`POST /api/demo/v1/ensure-start`只接受空正文或空JSON对象，启动任务立即返回202／starting；`GET /api/demo/v1/status`查询状态且不带账号密码。就绪后再次POST原固定动作取得当前普通演示候选。任何客户端命令、路径、端口、URL及额外字段均拒绝。ready后每次ensure都重验普通RAM认证proof的当前UID／用户名，失效仅按每账号最多一次／分钟尝试原候选正常登录，失败503并保留服务及资料；GET status不触发登录。用户名或密码后来失效时明确失败，不远程重置密码或数据库。

实际 Mac启动、产品摘要、专属HTTPS／GIF／资料证据见[本专项报告](../../../项目相关报告/测试/前端/iOS/20261118-资料与原生演示/Backend/README.md)。iPhone与UI验证由中央报告另记。

<a id="🔚" href="#前言">返回前言</a>
