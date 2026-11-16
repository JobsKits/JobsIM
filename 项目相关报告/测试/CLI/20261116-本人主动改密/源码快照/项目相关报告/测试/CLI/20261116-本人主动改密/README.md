# CLI本人主动改密

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

---

## 🔥 <span id="前言">前言</span>

最终产品Go源注册99个顶层测试，和原raw唯一PASS／SKIP名称集合完全相等：97 PASS／0 FAIL／2环境SKIP，Go退出0；MacOS arm64候选`71500985db27598cdd72e58ca326a1875cd4d23b672a4e7488b186f07101ca8a`构建通过。本轮仅CLI自有源码／外置测试／本端文档，实际SQLite与注入HTTP、fake vault；未连接真实API／PG、运行候选实际TTY或修改后端／中央资产／统计／合同。全I／243项仍未验收。

## 一、<span id="implementation">实现与入口</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

[CLI入口和本端资产](../../../../代码/前端/CLI/README.md#own-password-change)、[代码地图](../../../../代码/前端/CLI/代码地图/README.md#own-password-change)描述当前四命令，合同为[本人主动改密2.3](../../../../代码/后端/Go/接口契约/README.md#own-password-change)。`password_change_intents`位于当前账户Authentication SQLite，仅存原nonce／username／device／epoch／family／profile／physical／相对namespace及状态／结果，没有密码、OTP、密码请求hash或digest。密码对／OTP只在TTY内存，服务端NFC校验；不trim／截断、不把密码接到argv或日志。

新建捕获原已登录family，原责任先WAL／FULL耐久，先匿名原结果查询，原AT首次授权不自动refresh。明确retry先纯查，已有prepared仅匿名续尾；未见原单只有完整原family／token／设备／epoch／profile／namespace／physical仍匹配且cap可用才可首建。OTP提示后重核cap／上下文，发前再验同token。重启顶层resume只打开原元数据锁、不读取vault或激活session，重新输入原证明后匿名查询／续尾，不能替代新建授权。

送出前unknown和原auth状态保护，崩溃在两次元数据写之间仍由restore gate阻止自动业务；已成结果只停原family秘密，并核vault仍属原family才删，同原物理namespace才退休。旧结果不清后来family／vault／新namespace。安全提交需新登录，服务端不交付AT／RT，客户端不自动代登录；pending及current=false分别显示，不假报完成或回滚新状态。

## 二、<span id="evidence">独立轮次与计数复核</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- `01-local`／`02-build`为OTP后复核修订之前的原运行记录／7f98889a候选，原JSON所报96与原binary保持，不改成97；37项原输入按原SHA保留。
- `03-final-local`为最终围栏产品源，全Go退出0；[原日志](./03-final-local/SQLite测试.log)、[原SQLite JSON](./03-final-local/SQLite结果.json)和[原进程记录](./03-final-local/Process.json)原样保留。
- OTP与既有认证提示使用fmt.Print，与顶层PASS粘同一行，旧行锚定计数器在该最终轮少报95；这不是用例失败或少执行。独立[计数复核](./03-final-local/计数复核.json)及[顶层方法集合](./03-final-local/顶层方法集合.json)核源码99＝raw唯一97PASS＋2SKIP，无重复／缺失／FAIL，Go退出0，37项typed输入当前SHA全相等。该97只用于最终新轮，不重算旧96或旧88／80。
- `04-final-build`保存最终新Mac候选[路径及SHA](./04-final-build/Binary.json)、[门禁记录](./04-final-build/Process.json)。没有运行该候选实际联网认证。

运行器结果解析已改为按完整顶层名称识别且排除slash子项，使用原raw与3个轻量文本样本核对；磁盘再次跌破4GiB后未再Go／构建／原生或联网测试，仅轻量计数及源码核对。解析脚本变化不改变已执行／编译的产品Go输入，原95／96 JSON与日志未重写。

## 三、<span id="cases">八组范围与待验</span> <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

| 组 | 实际本地验证 |
| --- | --- |
| 原责任与秘密边界 | HTTP前SQLite原nonce／尝试数／unknown已耐久，payload无密码／OTP／hash；原family停用与fake vault删除 |
| pre-delivery丢请求 | transport显式拒投首请求，服务端未收到；匿名纯查404不写，同原family明确retry沿原nonce首次授权 |
| 后继family隔离 | 旧未见意图retry不附后来family AT，匿名401保原责任与新vault |
| 历史已成结果 | 旧changed/current=false原结果不清后继family或其凭据 |
| unknown交接 | 模拟原责任已写但auth状态未停的崩溃切点，restore gate持久拒恢复／自动业务；不是物理断电 |
| 匿名重开 | 原Authentication SQLite重新打开，无auth／消息store的原结果查询不首建 |
| 协议／cap | 撤回cap仍纯查；错epoch／计数／缺bool／429均不确认或创建 |
| SQL／OTP与发前围栏 | SQL失败不POST，明确首次拒绝不假报成功，无自动refresh；OTP等待时换family／cap后零授权POST，原unsent nonce不消费 |

证据边界：HTTP为本地替身；密码proof通过方法参数注入，不是实际term.ReadPassword／回显／原生TTY验证。OS库为fake vault，不是MacOS DPK entitlement或Windows／Linux安全库正向；真实HTTPS、管理员TOTP、后端worker／journal、真实撤全family、设备断电／满盘及目标OS仍待独立验证。两个SKIP为实际HTTPS协调器与系统vault诊断，不能借旧证据补齐。没有新依赖／SDK下载、API／PG／服务启停、缓存清理或Git提交。

<a id="🔚" href="#前言">回到首页</a>
