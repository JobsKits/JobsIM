# <span id="前言">JobsIM CodeGraph 项目索引</span>

![JobsIM](../../../../代码/前端/Web/Public/Logo.svg)

[toc]

---

## 一、实际产物与范围 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

项目根 `.codegraph/` 是 CodeGraph 的实际 SQLite 符号／关系索引，供 AI 使用 `codegraph explore` 查询当前源码与调用关系；各工程的人工 `代码地图/README.md` 继续作为阅读说明，两者职责不同。[原生配置](../../../../codegraph.json)使用相对路径，排除规则随源码保留；索引可重建并由根 `.gitignore` 排除，不在工程内分发工具二进制。

复用本机已装官方 `@colbymchenry/codegraph` 1.6.3 及其自包含运行时，没有安装、升级或修改第三方工具。已有 `codemap` 和 Finder“更新代码地图（.codegraph）”也是现成入口，但可能优先发现另一份已装 1.6.2；本报告按实际执行的 1.6.3 取证，不混用版本结果。

语义索引支持本人 Go（后端、CLI、部署公共组件）、TypeScript／TSX、JavaScript（含 cjs／mjs）、Swift、Kotlin／KTS、ArkTS、Ruby 和 Python。XML／properties 只记文件级信息；Bash／zsh 的 sh／command／zsh、PowerShell ps1、CMD／bat 以及 CSS／HTML／SQL 不在本版本的完整语义建图范围。部署 Shell／PowerShell 的人工说明与源码仍保留，不能把公共 Go 组件的索引写成全部部署脚本已建调用图。

## 二、排除与测试边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

配置明确排除整个 `项目相关报告/`，避免历史冻结副本、原输入与查询日志混入当前图；同时排除 `.git`、Pods、Vendor／vendor、ManualBy、node_modules、build／Build、Dist／dist、DerivedData、SDK工具链、缓存，以及运行数据、秘密、TLS、备份和恢复钥目录。原有 Git 忽略规则仍生效，不重新纳入第三方或生成物。初建的 Vendor 由 iOS 原有忽略规则排除，现已另加显式 `**/Vendor/`，使配置独立表达边界。

本人测试保留。最终 720 个文件中有 249 份位于 `测试/`；其中只有 144 份被工具的 `isTest` 规则自动识别。其余 105 份仍可按文件路径／普通调用关系检索，不能因工具输出“no tests found”而认定工程没有测试。最终文件、语言与测试数量以[摘要](./summary.json)指向的当轮原件为准；静态关系不证明运行时行为或全部动态派发。

## 三、更新与查询 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

在项目根运行现有 CLI；以下环境只限定当前进程及子进程，不修改系统或 Shell 配置。完整重建用 `index`，日常增量可用 `sync`。单 worker 限制用于与其他构建并行，不是性能承诺；开始重型工作前仍须保留 4GiB 可用空间。

```sh
CODEGRAPH_TELEMETRY=0 DO_NOT_TRACK=1 CODEGRAPH_NO_DOWNLOAD=1 \
CODEGRAPH_NO_WATCH=1 CODEGRAPH_NO_DAEMON=1 \
CODEGRAPH_PARSE_WORKERS=1 CODEGRAPH_RESOLVE_WORKERS=1 \
CODEGRAPH_NO_STORE_WORKER=1 CODEGRAPH_QUERY_POOL_SIZE=1 \
codegraph index .
```

```sh
CODEGRAPH_TELEMETRY=0 CODEGRAPH_NO_DOWNLOAD=1 \
CODEGRAPH_NO_WATCH=1 CODEGRAPH_NO_DAEMON=1 \
CODEGRAPH_QUERY_POOL_SIZE=1 codegraph status --json --strict .

CODEGRAPH_TELEMETRY=0 CODEGRAPH_NO_DOWNLOAD=1 \
CODEGRAPH_NO_WATCH=1 CODEGRAPH_NO_DAEMON=1 \
CODEGRAPH_QUERY_POOL_SIZE=1 codegraph explore --path . --max-files 1 \
'代码/前端/Web/Src/useVisibleViewport.ts isRectInVisibleViewport'
```

首次 `init --yes` 在禁用 watcher 时会自动添加 Git 同步钩子。本轮已精确撤回仅由初建新建的三个 marker-only 钩子，恢复其原不存在状态，其他钩子保留；[撤回审计](./自动Git钩子撤回.json)记录原件、身份与字节核对。当前原生项目 schema 没有 disableHooks 选项，后续仅用 `index`／`sync`，不重复 `init`。本任务未改全局或项目 AGENTS、Agent 连接配置及 OS@Extension。

## 四、验证依据 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[摘要](./summary.json)区分初建与最终源码冻结后的重建。最终重建为 720 文件、17,464 节点、62,550 边，约 11.6 秒、最大 RSS 约 730MiB、数据库约 88MiB，只代表本轮宿主与输入。775 份源码／配置输入在重建及查询窗口逐 SHA／mode 核对零漂移；最后八份 Linux Shell QR 修正的通知晚于重建取样；其最终字节已在775份清单内，与二维码专项最终SHA逐项吻合。补做 sync 再核零漂移，未借旧 Shell 字节。严格状态为 complete、全部 pending／待重建／解析错误为零，SQLite integrity_check 为 ok，八项真实路径查询返回源码／符号。初建 717 文件原件另保留，不追认并发窗口。

验收同时核对项目／索引路径、CLI 完整状态与待处理项、实际索引文件的禁入边界、源 SHA，以及真实 `explore` 返回的符号／路径和调用关系，不以目录存在或进程退出码独立判过。原始日志及初建查询保留；最终输入稳定后重新取证，历史原件不改写。

<a id="🔚" href="#前言">回到首页</a>
