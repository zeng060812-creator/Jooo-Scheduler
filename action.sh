#!/system/bin/sh
MODDIR=${0%/*}
echo "========================================"
echo " Jooo Scheduler · Snapdragon 8 Gen 3"
echo " v1.5.0 · 作者：ZenJooo"
echo "========================================"
echo
"$MODDIR/bin/jood" status --pretty
echo
echo "自检："
"$MODDIR/bin/jood" selftest
echo
echo "日志目录：/data/adb/yumi_jooo/logs/"
echo "完整控制与游戏添加请打开 KernelSU WebUI。"
