#!/system/bin/sh
# 安装阶段不检查SoC/cpufreq，避免KernelSU安装环境节点未就绪导致误判。
ui_print "----------------------------------------"
ui_print " Jooo Scheduler · Snapdragon 8 Gen 3"
ui_print " v1.5.0"
ui_print " 作者：ZenJooo"
ui_print "----------------------------------------"
ui_print "正在初始化模块..."

DATADIR=/data/adb/yumi_jooo
mkdir -p "$MODPATH/state" "$DATADIR/logs" "$DATADIR/scene_backup"

# 迁移旧版写在模块目录里的日志。
OLD_LOGDIR=/data/adb/modules/yumi_jooo/logs
if [ -d "$OLD_LOGDIR" ]; then
  for F in "$OLD_LOGDIR"/*.log; do
    [ -f "$F" ] || continue
    cat "$F" >> "$DATADIR/logs/${F##*/}" 2>/dev/null
  done
fi

# 同ID升级时尽量保留用户调校和custom_games。
OLD_CFG=/data/adb/modules/yumi_jooo/state/config.json
if [ -f "$OLD_CFG" ] && [ "$OLD_CFG" != "$MODPATH/state/config.json" ]; then
  cp -af "$OLD_CFG" "$MODPATH/state/config.json" 2>/dev/null
fi
[ -f "$MODPATH/state/config.json" ] || cp -f "$MODPATH/config/default.json" "$MODPATH/state/config.json"

printf 'balance\tapp\t\n' > "$MODPATH/state/scene_state.tsv"
printf 'balance\n' > "$MODPATH/state/scene_mode.txt"

set_perm "$MODPATH/bin/jood" 0 0 0755
set_perm "$MODPATH/service.sh" 0 0 0755
set_perm "$MODPATH/post-fs-data.sh" 0 0 0755
set_perm "$MODPATH/action.sh" 0 0 0755
set_perm "$MODPATH/uninstall.sh" 0 0 0755
for F in "$MODPATH/scripts"/*.sh; do
  [ -f "$F" ] && set_perm "$F" 0 0 0755
done

# 旧Yumi模块避免与本模块同时接管Scene/CPU。
if [ -d /data/adb/modules/yumi ] && [ ! -f /data/adb/modules/yumi/disable ]; then
  touch /data/adb/modules/yumi/disable 2>/dev/null
fi

ui_print "安装完成。重启后将按SM8650实际policy/capacity自动识别CPU簇。"
ui_print "运行日志已迁移至 /data/adb/yumi_jooo/logs/"
ui_print "Balance日常轻量优化默认开启，不修改CPU频率。"
ui_print "重启后从KernelSU模块页面打开WebUI。"
