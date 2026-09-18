#!/system/bin/sh
MODDIR=${0%/*}
DATADIR=/data/adb/yumi_jooo
mkdir -p "$MODDIR/state" "$DATADIR/logs" "$DATADIR/scene_backup"
chmod 0755 "$MODDIR/bin/jood" 2>/dev/null
