# 上游来源与二次开发声明

Jooo Scheduler 并非从零开始的独立原创项目。它是一个面向 Snapdragon 8 Gen 3 / SM8650 的下游二次开发版本。

## 主要上游：Yumi

- 项目：`imacte/yumi`
- 地址：https://github.com/imacte/yumi
- 许可证：GNU General Public License v3.0
- 关系：Jooo Scheduler 基于 Yumi 的项目方向与源码进行二次开发，并针对 SM8650、Scene 接入、游戏识别、温度保护、WebUI 与配置系统等进行了修改和扩展。

本仓库不是 Yumi 官方分支，也不代表 Yumi 上游作者。

## 设计参考：uperf

- 项目：`yc9559/uperf`
- 地址：https://github.com/yc9559/uperf
- 许可证：Apache License 2.0
- 关系：开发过程中参考了 uperf 在 Android 用户态性能控制、场景识别、线程亲和性与动态调度方面的设计思路。

本仓库不是 uperf 官方分支，也不代表 uperf 上游作者。

## Jooo Scheduler 的修改

Jooo Scheduler 的下游维护、SM8650 适配、Scene 9.x 接入、WebUI、配置迁移、热保护、游戏模式与相关修改由 ZenJooo 维护。

## 许可说明

由于 Jooo Scheduler 是基于 GPL-3.0 上游 Yumi 的二次开发版本，本仓库整体按 **GNU GPL v3** 发布。上游项目原有版权声明和许可证继续有效，归各自作者所有。

如某个文件保留了单独的上游版权或许可声明，以该文件中的声明及兼容的上游许可要求为准。
