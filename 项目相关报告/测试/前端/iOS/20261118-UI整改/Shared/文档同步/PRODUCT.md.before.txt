# JobsIM Product

<!-- impeccable:product-schema 1 -->

## Platform

adaptive

## Stack

用户已确认原生优先：iOS Swift/UIKit、Android Kotlin/Compose、HarmonyOS ArkTS/ArkUI；Web沿既定React/TypeScript，CLI和后端Go。具体选型与验证由Codex负责。

## Users

自托管实例的日常聊天用户与部署管理员。

## Product Purpose

免费、完整源码公开、可独立部署的即时通讯。当前明确开始实现I基础自托管，先身份、会话与真实收发，再覆盖媒体、群、互动和五端。

## Capabilities and Constraints

单一现行需求见IM需求明细表.md/IM需求明细表.md；支付、SDK、地图、RTC、E2EE专项不在I。各端原生UI，不采用Flutter/React Native或WebView壳替代。所有正式写请求正常调用API，断网保留真实上下文、稳定待发身份和缓存；演示独立标记及隔离，不回放至真服务。具备简繁中、藏、英、日、法、西、德、俄、韩十语言及白天/黑夜/系统三主题。不能以构建或模拟器结果代替全量验收。

## Brand Commitments

名称JobsIM。界面用正式、直接的产品语言；用户未指定视觉稿，具体视觉参数是Codex的可逆工程默认。

## Evidence on Hand

七份既有需求/架构/验收文档；iOS本人Swift/UIKit模板；当前尚无软件验收证据。原始xlsx保留不覆盖。

## Product Principles

- 消息和会话是主角，跨端遵守同一授权/幂等合同。
- 安全资格按真实保护档启用，未实现不伪装成功。
- 正式账号与演示数据隔离。

## Accessibility & Inclusion

原生平台习惯、键盘/读屏、字号缩放、完整空态与重载、长文案与减少动效。
