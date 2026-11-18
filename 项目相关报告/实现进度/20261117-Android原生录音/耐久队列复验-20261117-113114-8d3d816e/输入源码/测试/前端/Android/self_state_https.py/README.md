# <span id="前言">Android 新实例本人状态／草稿真实 HTTPS 与界面</span>

![Jobs出品，必属精品](https://picsum.photos/1500/400)

[toc]

## 一、入口与边界 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

使用已经构建的独立测试 APK，默认产品SHA为早期a135；当前已过23项原生的5774必须显式传入 `--required-product-sha 5774b93a9c5c5d930cc682d91d55336addb414f53a8374cfb65dcd2106a6d13d`，并逐字核对封存构建与安装读回。只复用已确认的 `JobsIMReview_20261114_1424`／5582，不创建／擦除／启动模拟器，不安装SDK、不重启服务、不清用户库／原安全库。执行前必须与其它原生构建／模拟器协调，且Data至少4GiB。无`--execute`仅自述。

```sh
python3 测试/前端/Android/self_state_https.py/self_state_https.py \
  --build-report 项目相关报告/测试/前端/Android/20261116-S2本人状态/Build-本轮测试包 \
  --environment '/工程外本轮候选/Safety/restored-environment.json' \
  --execute
```

metadata必须仅本人可读；只提取URL、公开CA、部署／epoch及binary SHA，不复制其DB密码、密钥或其它秘密。仅允许新回环HTTPS域，明确拒绝固定8443／18443／20443／28443／36443／44443。设备使用本轮同端口`adb reverse`；冲突不覆盖，完成只撤本次创建的映射，不改系统信任／代理。所有URL、CA、部署及epoch都经实际Android客户端核验；不忽略证书错误。

用户已授权清理后重建时，可显式加`--install-missing-product`。必须先由`pm list packages`证实当前user0确无此包，才安装指定SHA的同一原包并读回SHA；存在未知SHA包时仍拒绝，不重签／卸载／清数据。包缺失不证明某一清理动作的原因，也不代表旧Keystore或原用户数据已恢复；测试始终采用新profile／family。

## 二、三项真实场景 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

- 原生Room／engine实际schema2快照安装，真实read／view／font接口，稀疏可见观察保留未见洞／独立手动未读；peer同CID不清原Outbox。
- 真实PUT已收到server结果后，只在客户端丢弃该响应；Room冷重开后携完整原body只读查单，确认没有第二个创建式PUT。另本轮peer真实编辑来源，使新草稿retired，验证独立文字保留／显式恢复，不能把HTTP200当保存成功。
- 实际Compose Activity及系统无障碍操作，操作本人同步面板、云草稿保存／显式载入、字号及深色主题，保存5张原始截图供人工查看。直接使用生产Activity持有的ChatModel，不另建模型或清ViewModelStore；使用本轮新profile，结束恢复原连接／显示偏好；只清自己的新安全库记录，不清原数据库／其它账号。

每方法仅注册自己新建的2个测试账号，密码／AT／RT只在进程与安全库存储，不进入参数／报告。方法结束明确登出并以原AT读取401核对。非秘密journal只保存本轮runId／用户名／原认证requestId／user与family/device ID／已验证关闭标识；主机逐项核6个家庭全部关闭，不把中断猜成已注销。仪器必须三项全过、无跳过；600秒总界、每步上界及4GiB门禁只终止自有adb子进程组，必要时force-stop此AVD自有app/test进程，不停模拟器。失败原raw保留，不把注入23或JVM11与本轮三项相加作完整验收。超时前未执行完的fixture会如实标失败，需要根据本轮非秘密journal显式清理，不假称已关闭。

## 三、报告 <a href="#前言">🔼</a> <a href="#🔚">🔽</a>

中央 `项目相关报告/测试/前端/Android/20261116-S2本人状态/HTTPS-<唯一时间>` 保存公开fixture摘要、原始仪器输出、安装读回SHA、资源门禁、端口映射处理及截图SHA。私密metadata、CA私钥、DB、整库或备份留在工程外，不复制到仓库。工具不会更新Dist／旧包／旧报告；截图仍需实际逐张查看，不能只凭PNG存在宣称视觉验收。

<a id="🔚" href="#前言">我是有底线的➤点我回到首页</a>
