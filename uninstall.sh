#!/system/bin/sh
MODDIR=${0%/*}
"$MODDIR/bin/jood" watch-stop >/dev/null 2>&1
"$MODDIR/bin/jood" stop >/dev/null 2>&1
"$MODDIR/bin/jood" restore >/dev/null 2>&1
sh "$MODDIR/scripts/restore_scene_backend.sh" >/dev/null 2>&1

echo "Jooo Scheduler（yumi_jooo）已卸载，Scene原始powercfg已恢复。请重启手机。"
