Jooo Scheduler v1.5.0 · Snapdragon 8 Gen 3 智能游戏调度

作者：ZenJooo
模块ID：yumi_jooo
平台：Qualcomm Snapdragon 8 Gen 3 / SM8650 系列

一、首次安装

首次打开 KernelSU WebUI 会出现三步引导：
1. 检测 Scene（com.omarea.vtools）是否安装；
2. 推荐控制来源：有 Scene 推荐 scene，没有 Scene 推荐 auto；
3. 推荐初始游戏类型：默认建议“均衡游戏”。

从 v1.4.0 及更早版本升级时不会强制再次弹出引导。

二、控制来源

1. auto：jood 独立前台监听，默认每 2000ms 判断一次前台APP；普通APP不写CPU频率，命中游戏才启动控制器。
2. scene：Scene负责场景切换；Jooo只使用wrapper旁路接收，不覆盖Scene的powercfg.json，并继续转发原powercfg.sh。
3. webui：完全手动，仅用于测试/排障。

三、游戏类型

v1.5.0 在原来的 Scene/固定性能档位之上增加“游戏类型”：

均衡游戏（默认）
- Prime Gate 基准默认 72%；
- 优先主性能簇和次性能簇；
- 更适合王者、和平精英、LOL手游等日常手游；
- 更重视温度和功耗。

极致游戏
- 在均衡基准上将 Prime Gate 提前约 22%，默认 72% -> 50%；
- 主性能/次性能簇获得更积极的最低频率地板；
- 适合原神、鸣潮、永劫、三角洲等大型游戏；
- 功耗和发热会更高。

WebUI“游戏助手”可对每个已安装游戏单独选择“均衡/极致”。未单独设置的游戏使用“默认游戏类型”。

四、游戏内负载自适应

默认开启。jood复用现有游戏线程和CPU簇负载采样，不额外启动第二套高频监控：
- light：大厅/剧情等持续轻载约1.8秒后，临时降低一个性能档并提高Prime Gate；
- normal：恢复游戏基础档位；
- heavy：团战/BOSS等重载持续约0.3秒后，临时提高一个性能档并适当降低Prime Gate；
- 状态切换带回差，避免瞬时尖峰造成频繁跳档。

WebUI游戏运行卡会实时显示：游戏类型、负载级别、基础档位、有效档位、有效Prime Gate。

温度保护优先级最高：即使极致游戏处于heavy，只要电池温度保护触发，仍会压制性能和Prime。

五、配置导入/导出

WebUI -> 诊断：
- “导出配置”：把完整 config.json 保存到 /sdcard/Download/JoooScheduler_Config_时间.json，同时更新 JoooScheduler_Config_latest.json；
- “导入配置”：自动读取 Download 中最新的 JoooScheduler_Config*.json；
- 导入前当前配置会自动备份到 /data/adb/yumi_jooo/config_backups/；
- 导入时会校验JSON并自动迁移旧版本配置字段。

命令行：
/data/adb/modules/yumi_jooo/bin/jood config export
/data/adb/modules/yumi_jooo/bin/jood config import
/data/adb/modules/yumi_jooo/bin/jood config import /sdcard/Download/xxx.json

六、SM8650全机型动态适配

不使用手机型号白名单，不写死小米14 policy0/2/5/7：
- 枚举 /sys/devices/system/cpu/cpufreq/policy*；
- 读取 related_cpus / affected_cpus、cpu_capacity、cpuinfo_max_freq；
- 自动识别 efficiency / performance_primary / performance_secondary / prime；
- 动态生成线程亲和性mask。

只要设备属于SM8650系列、标准cpufreq节点可用且权限允许，就会按本机实际拓扑运行。

七、日常轻量优化

Balance模式默认开启，但不接管CPU频率：
- background cpuset存在时优先收敛到动态识别的能效簇；
- 进入游戏前自动恢复；退出游戏回到Balance时重新应用；
- ZRAM仅在尚未初始化、且内核支持zstd时安全切换；
- 运行中的ZRAM不热改；
- 不通用修改swappiness、水位、OEM thermal、GPU等高风险参数。

八、温度保护

默认开启：
- 电池温度达到默认45°C后，限制高性能游戏状态；
- 当前实现使用2°C回差恢复，避免在阈值附近来回切换；
- WebUI可调整阈值或关闭；
- 不关闭OEM系统温控。

九、新游戏

WebUI游戏助手支持：
- 显示当前前台APP名称/包名；
- 一键加入custom_games；
- 5秒捕获目标APP；
- 从已安装应用中选择；
- 每个游戏单独设置均衡/极致类型。

十、日志与诊断

日志：/data/adb/yumi_jooo/logs/
jood.log约256KB轮转为jood.log.1。

诊断：
/data/adb/modules/yumi_jooo/bin/jood selftest
/data/adb/modules/yumi_jooo/bin/jood diag

十一、功耗说明

日常轻载1~2W仍是优化方向，不是硬性保证。整机功耗受屏幕亮度/刷新率、5G、Wi-Fi、GPS、后台同步、相机、充电等影响。
游戏内v1.5.0通过轻/正常/重负载自适应减少“不需要时仍维持高性能”的时间，但不会为了省电牺牲系统原生温控或强制锁频。
