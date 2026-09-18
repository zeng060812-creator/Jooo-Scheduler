# Jooo Scheduler

**Jooo Scheduler** 是一个面向 **Qualcomm Snapdragon 8 Gen 3 / SM8650 系列** Android 设备的开源调度项目，模块 ID 为 `yumi_jooo`，作者 **ZenJooo**。

当前版本：**v1.5.0**

> 项目目标不是长期锁高频，而是在保留 OEM 原生调度能力的基础上，对游戏场景做动态识别、线程放置、负载自适应与温度保护，并尽量降低非游戏场景额外开销。

## 主要功能

- SM8650 动态拓扑识别，不写死 `policy0/2/5/7`
- `auto / scene / webui` 三种控制来源
- Scene 9.x wrapper 旁路接入，不覆盖原 `powercfg.json`
- 均衡游戏 / 极致游戏逐游戏配置
- `light / normal / heavy` 游戏负载自适应
- 动态 Prime Gate
- 电池温度保护与回差恢复
- 日常 Balance 轻量优化，不接管 CPU 频率
- 新游戏一键加入、自定义游戏列表、5 秒捕获
- 配置导入 / 导出与导入前备份
- KernelSU WebUI
- 完整 Go 源码，Android ARM64 可直接构建

## 控制来源

### auto

`jood watch` 独立监听前台应用。默认每 2 秒判断一次前台 APP：普通应用保持 OEM 原生调度，命中游戏才进入游戏控制逻辑。

### scene

Scene 负责场景切换。Jooo Scheduler 通过 wrapper 接收 Scene 调用，并继续转发到安装前的原始 `powercfg.sh`，不替换 Scene 自己的 `powercfg.json`。

### webui

仅接受 WebUI 手动控制，主要用于测试与排障。

## 游戏类型

### 均衡游戏

默认 Prime Gate 基准为 `0.72`，优先性能簇，在需要时才进一步放行 Prime，适合王者荣耀、和平精英、LOL 手游等日常手游。

### 极致游戏

默认在均衡基准上提前 Prime 介入，典型基准约 `0.50`，适合原神、鸣潮、永劫无间手游、三角洲行动等大型游戏。功耗和发热会更高。

## 游戏负载自适应

游戏运行期间会根据游戏线程与性能簇负载做三态判断：

- `light`：大厅、剧情等轻负载持续一段时间后临时降低性能强度
- `normal`：恢复游戏基础档位
- `heavy`：团战、BOSS 等重负载持续确认后临时提高性能强度

温度保护优先级高于负载自适应。

## 温度保护

默认开启。达到设定电池温度后会限制高性能游戏状态，并使用回差避免阈值附近频繁切换。该功能不会关闭设备原厂 thermal 保护。

## SM8650 动态适配

Jooo Scheduler 不维护手机型号白名单，而是在运行时读取：

- `/sys/devices/system/cpu/cpufreq/policy*`
- `related_cpus` / `affected_cpus`
- `cpu_capacity`
- `cpuinfo_max_freq`
- 实际频率表

然后自动识别能效簇、主性能簇、次性能簇与 Prime，并动态生成线程 CPU affinity mask。

## 构建 jood

要求：**Go 1.23+**

```bash
cd source/jood
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go test ./...
CGO_ENABLED=0 GOOS=android GOARCH=arm64 \
  go build -trimpath -ldflags='-s -w' -o ../../bin/jood .
```

也可以执行：

```bash
sh source/build_android_arm64.sh
```

## 模块目录

```text
.
├── config/
│   └── default.json
├── docs/
├── scripts/
├── source/
│   └── jood/
├── webroot/
│   ├── assets/
│   └── index.html
├── action.sh
├── customize.sh
├── module.prop
├── post-fs-data.sh
├── service.sh
└── uninstall.sh
```

## 配置导入 / 导出

WebUI 的“诊断”页面支持完整配置导出和导入。

命令行：

```bash
/data/adb/modules/yumi_jooo/bin/jood config export
/data/adb/modules/yumi_jooo/bin/jood config import
/data/adb/modules/yumi_jooo/bin/jood config import /sdcard/Download/xxx.json
```

## 日志与诊断

运行日志：

```text
/data/adb/yumi_jooo/logs/
```

诊断：

```bash
/data/adb/modules/yumi_jooo/bin/jood selftest
/data/adb/modules/yumi_jooo/bin/jood diag
```

## 兼容性说明

项目目标平台为 Snapdragon 8 Gen 3 / SM8650 系列。不同 OEM 可能修改 sysfs 节点、权限或调度实现，因此“同 SoC”不等于所有 ROM 都能保证完全一致的行为。发现兼容问题时，请附上 `jood selftest` 与 `jood diag` 输出。

## 功耗说明

日常轻载低功耗是设计目标，但整机功耗还受屏幕亮度、刷新率、基带、5G/Wi‑Fi、GPS、后台任务、充电状态等影响。本项目不会承诺固定整机功耗值。

## 开源协议

本项目使用 [MIT License](LICENSE)。
