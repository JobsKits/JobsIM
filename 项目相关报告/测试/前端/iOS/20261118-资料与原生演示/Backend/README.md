# 用户资料、GIF头像与Mac原生演示证据

![Jobs出品，必属精品](https://picsum.photos/1500/400)

<!-- jobs-toc:start -->
<!-- jobs-toc:source {"v":2,"token":"[toc]"} -->
<details>
<summary>📚 目录（点击展开 / 收起）</summary>

- [🔥 前言](#%E5%89%8D%E8%A8%80)
- [一、资料与动态图](#profile)
- [二、实际验证](#tests)
- [三、当前运行的原生演示](#native-demo)
- [四、验收边界](#limits)

</details>
<!-- jobs-toc:end -->

## 🔥 <span id="前言">前言</span>

本报告只覆盖当前后端／Mac宿主的实际证据，[汇总](./summary.json)与[生产输入](./SyncAnnouncement/final-inputs.json)对应236项生产输入。业务专项使用51份外置overlay，当前同步修复增为52份；控制器4项为独立包测试。冻结后源摘要零漂移。[当前运行产品](./SyncAnnouncement/final-runtime.json)分别记录控制器和JobsIMd的SHA、端口及精确PID，不借先前27条旧源结果追认。iOS实际页面、真机与完整243项验收由中央报告分别登记。

## 一、<span id="profile">资料与动态图</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

用户资料投影真实 `auth.users.created_at`，UID沿用既有ID；签名允许空值、上限200个Unicode码点，可单独PATCH并回读。联系方式从加密权威绑定记录读取，状态为bound／unbound／hidden／unavailable；他人不获知绑定是否存在。此源码没有实现邮箱／短信验证或新增绑定流程，不能把显示字段视为OTP交付。读取沿本人／当前好友／共同有效会话与拉黑权限，删除本人账户时清理新增绑定权威行。

头像继续沿原上传责任、sourceDigest、epoch／generation、稳定原操作、发布CAS与当前访问ACL；GIF在DecodeAll前检查画布、帧数及累计解码像素，输入2MiB、单画布16MP、120帧／总帧32MP与总时长10分钟。合成disposal后生成512多帧主件并保留延时／loop，128JPEG仅作独立预览；不把首帧保存冒充动态图支持。

## 二、<span id="tests">实际验证</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 范围 | 当前结果与原件 |
| --- | --- |
| Go当前30条唯一方法 | [30条核对](./ProfileGIF/current30-merged.json)：30 PASS／0 FAIL／0未解决SKIP。含3条新增多帧语义／预算／真实PG资料隐私，以及27条受影响既有关系／举报／媒体／原单等方法；重复执行不累加。首次整轮28过／2浏览器夹具缺失跳过原件保留，核同一原始Blob SHA后两条实际重跑通过 |
| 控制候选重验 | [4条race方法](./controller-credential-tests.result.json)全部通过：正常proof重复ensure不消耗密码登录、用户名改变拒缓存候选、改密导致401及每账号1/min重试边界、过期proof按同一普通身份重新验证；GET status不进行认证。另[17条真实HTTPS](./credential-ready-https.json)实际修改demo_xia用户名→ensure503／不返回accounts／不自动重置→操作者明确改回→连续8次ensure就绪；未在此真实场景更改seed密码 |
| direct／self与群公告快照 | [本次专项](./SyncAnnouncement/summary.json)：正常direct＋self＋group混合baseline在旧SQL真实PG修前失败，过滤为仅group捕公告后通过；保group公告且旧形状受限公告ref仍不能安装，页门禁和iOS严格校验未放宽。专属真实schema2 device／context／snapshot／page／validate／changes共9请求通过，17投影只有1个群公告、全部installable且两联系人peerId仍在；原[失败实链](./readonly-sync-v2-wire.json)与[修后原件](./SyncAnnouncement/https-v2-final.json)分别保留。既有30方法不是此单SQL后的全量重跑 |
| 原生处理语义 | 实际三帧、延时、loop、DisposalPrevious合成、512GIF／128JPEG、畸形／大画布／过帧拒绝；资料PG核只读实际注册时间、200码点边界、本人真实加密绑定、他人hidden及陌生／拉黑拒读 |
| 真实专属HTTPS | [25请求](./ProfileGIF/https-final.json)：严格CA及同host／部署、Web首页、固定控制动作／状态无凭据、3个新非管理员测试账户、签名回读、实际GIF上传／原单重放／好友和拉黑ACL。没有改seed密码或默认实例 |
| 实际下载帧 | [Go DecodeAll](./ProfileGIF/https-native-decode.json)重新读取真实HTTPS下载字节，验证2帧、9／21百分之一秒延时、loop=0、512尺寸及帧颜色确有区别；不止MIME检查 |
| 静态与原生构建 | Go四包实际编译、生产及51份测试overlay的go vet退出0；Shell语法／帮助、Python语法通过。当前Go与Web成品由真实Mac helper离线构建，复用本机既有工具 |
| 测试PG收尾 | [精确生命周期](./ProfileGIF/pg-lifecycle.json)：本任务只启动已有专属62381 PG域，每方法生成独立测试库并清理；测试结束精确停止、pg_ctl状态3，数据保留。此组in-process RPC不冒充真实多物理PG或NATS；真实demo另有下节证据 |

## 三、<span id="native-demo">当前运行的原生演示</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

入口源码为[Mac控制面](../../../../../../部署/MacOS/演示控制面/README.md)，当前本机实际控制器PID64239，前台保持运行供iPhone连接。业务HTTPS19543、控制19544绑定明确LAN IPv4；双物理PG19545／19546、NATS19547及内部19560～19568独立运行。CA、IP SAN、服务健康、Web资源和三份普通种子账号登录通过后才ready。随后每次ready ensure重验RAM普通认证proof的当前UID／用户名；失效仅按每账号最多一次／分钟尝试原候选正常登录，失败503并保业务及资料，不重置密码。GET status不触发登录；控制面关闭尝试退出自身proof会话。实际部署ID `2cbd9bd2-1205-44fb-870c-9cad48135c48`；本次精确停止／重启保留原部署身份、CA及数据，见[状态](./status-final.json)和[运行摘要](./SyncAnnouncement/final-runtime.json)。

预埋用户名为demo_jobs、demo_lin、demo_xia；显示名Jobs／小林／小夏，群为周末小组，只使用虚构资料。环境标识由前端单浮标负责，不把“演示”前缀附到每个人名。真实资料对齐检查见[名称核验](./seed-display-alignment.json)。公开Bundle描述仅URL、公开CA／指纹及demo标记；候选密码只在固定ensure响应中交付，GET status、报告和源码不携带密码。

第一次prepare因NATS128MiB／16MiB低于现有门槛拒绝，日志保留；修回512MiB／32MiB后真实构建通过，保护门槛未降低。固定启动只使用已构建本机产品，无远程命令／路径／任意端口或删库动作；App关开关／浮标只断前端，Mac关闭helper才精确停止自有demo子进程并保数据。原9443实例不参与。

[最终只读复核](./SyncAnnouncement/final-readonly-integrity.json)再次核对236项生产输入零漂移、两个实际产品SHA完全匹配冻结记录；控制器PID64239与业务PID64663均为原精确进程，严格CA状态200／ready、原部署与CA保持、status不带账户。SQL实链仍按9请求、17投影／两好友及仅群公告的实际范围记录；本次复核未重启、改业务或追加全量Go运行。

## 四、<span id="limits">验收边界</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

本报告不证明iPhone动画播放、真实登录设置开关触控或最终App安装；这些由主流程新签名包与系统测试留证。真实浏览器交互／MFA、断电保护和完整基础标段仍需各自证据。HTTPS每轮3个测试账户及其材料留在专属demo中用于审计（两轮各3，不累计计为50项），不擅自清库；会话已退出。没有创建提交或推送。

<a id="🔚" href="#前言">返回前言</a>
