# JobsProds · 产品工程总览

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

`JobsProds` 集中存放 Jobs 的产品工程，当前共有 **5 个工程**。盘点日期为 **2026-11-17**，依据为本目录实际存在的一级产品工程目录；同一产品内部的多个客户端、后端、测试和独立子仓统一归入所属工程，不重复计数，规约、报告、隐藏文件和本说明不计入。

## 一、<span id="projects">工程一览</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

> 点击工程名称进入对应项目说明，查看运行方式、功能范围与当前验收状态。

| 工程 | 定位 | 主要用途 |
| --- | --- | --- |
| [JobsDeviceInfo](./JobsDeviceInfo/README.md) | 设备信息采集与展示网站 | 手机扫码确认采集，电脑同步查看设备标识或浏览器基础信息。 |
| [JobsIM](./JobsIM/README.md) | 自托管即时通讯软件 | 基础／标准／高级三类实施标段，提供五端客户端及配套服务端。 |
| [啄木鸟维修平台](./啄木鸟维修平台/README.md) | 维修与配件业务协同平台 | 协同客户、调度、工程师、仓库、质检和财务，管理维修工单及履约。 |
| [学术服务平台](./学术服务平台/README.md) | 自营学术课程与服务商城 | 展示并销售课程与服务，管理下单、付款、交付、确认和评价。 |
| [语言学习（单机版）](./语言学习（单机版）/README.md) | 多语种离线词典与拼读工具集合 | 学习字母和拼读、查询英语词汇、点读日语汉字，并使用本机语音与辅助翻译。 |

## 二、<span id="layout">目录结构</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

```text
JobsProds/
├── README.md                  # 工程总览与文档入口
├── AGENTS.md                  # 本目录的维护规约
├── JobsDeviceInfo/            # 设备信息采集站
├── JobsIM/                    # 自托管即时通讯
├── 啄木鸟维修平台/            # 维修与配件业务协同
├── 学术服务平台/              # 自营课程与服务商城
└── 语言学习（单机版）/        # 多语种离线学习
```

## 三、<span id="details">各工程说明</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

### 3.1、<span id="device-info">JobsDeviceInfo · 设备信息采集站</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

面向本人或已明确同意的设备使用者。电脑生成二维码，手机扫码确认并提交信息，电脑同步展示结果；支持字段复制、缺失信息补录与会话清除。

苹果设备通过系统描述文件回传可用标识；[**Android**](https://developer.android.com/) 与[**鸿蒙**](https://developer.huawei.com/consumer/cn/harmonyos/)通过浏览器回传公开的基础信息。可读取字段受系统权限限制，手动补录会标明来源。工程包含网页、[**Cloudflare Workers**](https://developers.cloudflare.com/workers/) 后端及部署、预览入口。

详细入口：[项目说明](./JobsDeviceInfo/README.md)。

### 3.2、<span id="jobsim">JobsIM · 自托管即时通讯</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

面向个人与小团队，建设可自行部署和维护的即时通讯产品。当前以基础聊天与部署闭环为主线，包含账户、会话、消息、附件及同步等业务；功能完成度按资产盘点和实现进度登记。部署使用原生Go／PostgreSQL／NATS与六系统成对快捷入口、Deployment／Undeployment，按李林哲现行交付方式组织；Web同时覆盖桌面与手机H5，低总体资源／配置／维护成本与真实保护边界见[部署需求对照](./JobsIM/项目相关报告/测试/部署/20261118-成对部署反部署/README.md)及[H5验证](./JobsIM/项目相关报告/测试/前端/Web/20261118-H5适配/README.md)。[原源码回归](./JobsIM/项目相关报告/测试/回归/20261118-全项目回归/README.md)覆盖其冻结输入的后端、五端及独立运维切片，原部署需求符合性未通过，已修复迁移容量探针、Web异常、鸿蒙翻译和CLI漏测；构建、本地／模拟器、真实HTTPS与环境跳过分别列证，完整基础阶段仍未验收。

附件链路提供固定上传任务和原结果查询，服务端已有隔离数据库与构建验证。Web原件保存／明确恢复／删除后拒旧有41项冻结浏览器证据；其后两文件收讫／注销查询清理增量通过相关专项及新类型／私有包核验，旧包独立保留；Android语音队列原145输入候选已过46 JVM并构建应用／测试APK；后续迁移／收讫新包及独立模拟器实测按当前专项另记，不能沿用原仪器仅编译状态追认新源。具体范围及Web一次双标签页认证失败未归因的记录见[附件耐久发送](./JobsIM/项目相关报告/测试/前端/Web/20261117-附件耐久发送/README.md)与[Android录音与队列](./JobsIM/项目相关报告/实现进度/20261117-Android原生录音/README.md)。

**客户端版本**包含 [**iOS**](https://developer.apple.com/ios/) 原生版、Android 原生版、鸿蒙原生版、Web 网页版和 CLI 命令行版，另有管理后台与配套后端服务。

JobsIM当前原MacOS入口已在本机接续原实例并从最新源码重建，双数据库、消息代理及HTTPS运行；原身份、CA与数据保留，重启前加密备份已验真。iOS当前122项原生方法及1项演示UI通过，Android当前54 JVM／本地86原生及另3 HTTPS通过（16环境跳过）、Harmony双HAP构建；CLI135项本地通过并交付arm64成品，实际验证边界和Android／HarmonyOS新候选见[本轮闭环](./JobsIM/项目相关报告/实现进度/20261116-剩余能力闭环/README.md)。这不替代完整基础阶段、真机或跨端保护验收。

**产品版本／实施标段**按当前唯一需求分为以下三类：

| 版本简称 | 实施标段与主要范围 | 当前状态 |
| --- | --- | --- |
| 基础 | I 基础自托管：本地账户、单聊／群聊、多媒体消息与文件、搜索／置顶／@／收藏、多端同步、后台管理、基本备份恢复与部署。 | 已进入编码并有局部验证，完整基础标段尚未验收。 |
| 标准 | II 扩展集成：邮箱验证、第三方登录、远程推送、实时通话、地图位置、频道／话题／投票、机器人与计划任务；核心稳定后交付 SDK。 | 后续按真实渠道与资源逐项接入。 |
| 高级 | III 高级专项：端到端加密、阅后即焚、多入口切换、强保护灾备、组织／客服／复杂社区、AI、支付与钱包。 | 按具体场景单独建设和验收。 |

“基础／标准／高级”是同一套需求与代码的实施标段简称，并非三个独立工程或收费套餐；当前不另设“轻质版”。表中包含规划目标，实际完成度以验收记录为准。划分依据见[需求明细](./JobsIM/需求/IM需求明细表.md/IM需求明细表.md)。

详细入口：[项目说明](./JobsIM/README.md) · [工程导航](./JobsIM/代码/README.md) · [当前实现进度](./JobsIM/项目相关报告/实现进度/README.md)。

### 3.3、<span id="repair">啄木鸟维修平台 · 维修业务协同</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

项目别名为“李林哲项目”。面向维修服务及配件业务，围绕客户、调度、工程师、仓库、质检与财务的分工，组织工单受理、检测、报价、客户签署、维修、质检、交付、验收及收退款等流程，并涉及库存、借还、采购、发票和审计。

工程提供业务网页、移动客户端、管理后台与[**微信小程序**](https://developers.weixin.qq.com/miniprogram/dev/framework/)，共用业务服务端。用户端产品显示名统一为“啄木鸟维修平台”，管理端附加“管理工作台”或“管理端”等职责说明。目前处于开发验证阶段，演示采用合成账号和样例，真实支付尚未接入。

详细入口：[项目说明](./啄木鸟维修平台/README.md) · [工程与使用说明](./啄木鸟维修平台/代码/README.md) · [部署说明](./啄木鸟维修平台/部署/README.md) · [名称资产盘点](./啄木鸟维修平台/项目相关报告/README.md)。

### 3.4、<span id="academic">学术服务平台 · 自营课程与服务商城</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

项目别名为“陈泉羽项目”，实现暂名为“知序”。面向买方与站方运营，销售毕业论文、普刊、编译相关课程与服务。业务主线为商品浏览、详情、下单、付款、站方交付、买方确认和评价；后台管理商品内容、价格、图片、上下架及缺货状态。

工程包含桌面端、移动端、管理端、微信小程序及后端。当前[**支付宝**](https://open.alipay.com/)／[**微信支付**](https://pay.weixin.qq.com/)采用模拟收银台，确认后记录模拟付款成功，不会实际扣款；正式支付渠道仍待接入。

详细入口：[项目说明](./学术服务平台/README.md) · [工程说明](./学术服务平台/代码/README.md) · [部署说明](./学术服务平台/部署/README.md)。

### 3.5、<span id="language-learning">语言学习（单机版）· 多语种离线学习</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

工程名为 `JobsLanguageLearning`，面向中文学习者，提供分级英语词汇、日语汉字注音与例句点读，以及俄语、法语、西班牙语、朝鲜语、德语和阿拉伯语的基础拼读，配合系统语音点读与本机中文辅助翻译。

工程包含原生 iOS 版本、[**Flutter**](https://flutter.dev/) 跨端版本及 [**Python**](https://www.python.org/) 桌面工具。课程与词典使用各端打包语料；语音、翻译能力取决于平台及本机资源准备情况，具体范围见各版本说明。

详细入口：[项目说明](./语言学习（单机版）/README.md) · [原生 iOS 版本](./语言学习（单机版）/原生iOS版本/README.md) · [Flutter 版本](./语言学习（单机版）/Flutter版本/README.md)。

## 四、<span id="maintenance">总览维护</span> <a href="#前言" style="font-size:17px; color:green;"><b>🔼</b></a> <a href="#🔚" style="font-size:17px; color:green;"><b>🔽</b></a>

只要修改 `JobsProds/` 内任何内容，必须在同一任务中同步更新本页，核对工程数量、名称、用途、版本、功能状态与文档入口，并验证相对链接。具体约束见[本地维护规约](./AGENTS.md)。

<a id="🔚" href="#前言" style="font-size:17px; color:green; font-weight:bold;">我是有底线的➤点我回到首页</a>
