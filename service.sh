#!/system/bin/sh
MODDIR=${0%/*}
BIN="$MODDIR/bin/jood"
DATADIR=/data/adb/yumi_jooo
LOG="$DATADIR/logs/service.log"
mkdir -p "$MODDIR/state" "$DATADIR/logs"
# 兼容升级：把旧版模块目录日志迁出。
if [ -d "$MODDIR/logs" ]; then
  for F in "$MODDIR/logs"/*.log; do
    [ -f "$F" ] || continue
    cat "$F" >> "$DATADIR/logs/${F##*/}" 2>/dev/null
  done
  rm -rf "$MODDIR/logs" 2>/dev/null
fi

# 等待Android完整启动后再做硬件检查，避免安装阶段误判。
N=0
while [ "$(getprop sys.boot_completed)" != "1" ] && [ "$N" -lt 180 ]; do
  sleep 1
  N=$((N+1))
done
sleep 3

SOC="$(getprop ro.soc.model 2>/dev/null)"
VSOC="$(getprop ro.vendor.soc.model 2>/dev/null)"
PSOC="$(getprop ro.product.soc.model 2>/dev/null)"
DEVICE="$(getprop ro.product.device 2>/dev/null)"
MODEL="$(getprop ro.product.model 2>/dev/null)"
BOARD="$(getprop ro.board.platform 2>/dev/null)"

SOC_OK=0
case "$SOC $VSOC $PSOC" in *SM8650*|*sm8650*) SOC_OK=1;; esac
if [ "$SOC_OK" -ne 1 ]; then
  printf '%s 平台检查未通过：device=%s model=%s soc=%s/%s/%s board=%s。仅支持Snapdragon 8 Gen 3 / SM8650系列。
'     "$(date '+%F %T')" "$DEVICE" "$MODEL" "$SOC" "$VSOC" "$PSOC" "$BOARD" >> "$LOG"
  exit 0
fi

POLICY_COUNT=0
CPU_COUNT=0
CPU_SEEN=" "
for D in /sys/devices/system/cpu/cpufreq/policy*; do
  [ -d "$D" ] || continue
  [ -f "$D/scaling_min_freq" ] || continue
  [ -f "$D/scaling_max_freq" ] || continue
  CPUS="$(cat "$D/related_cpus" 2>/dev/null)"
  [ -n "$CPUS" ] || CPUS="$(cat "$D/affected_cpus" 2>/dev/null)"
  [ -n "$CPUS" ] || continue
  POLICY_COUNT=$((POLICY_COUNT+1))
  for C in $CPUS; do
    case "$CPU_SEEN" in *" $C "*) ;; *) CPU_SEEN="$CPU_SEEN$C "; CPU_COUNT=$((CPU_COUNT+1));; esac
  done
done
if [ "$POLICY_COUNT" -lt 3 ] || [ "$CPU_COUNT" -lt 6 ]; then
  printf '%s SM8650动态拓扑检查失败：policy=%s cpu=%s。模块保持静默。
' "$(date '+%F %T')" "$POLICY_COUNT" "$CPU_COUNT" >> "$LOG"
  exit 0
fi

chmod 0755 "$BIN" "$MODDIR"/*.sh "$MODDIR"/scripts/*.sh 2>/dev/null

# 初始化状态。日常不启动daemon，也不写CPU频率。
[ -f "$MODDIR/state/config.json" ] || cp -f "$MODDIR/config/default.json" "$MODDIR/state/config.json"
"$BIN" restore >> "$LOG" 2>&1
printf 'balance\tapp\t\n' > "$MODDIR/state/scene_state.tsv"
printf 'balance\n' > "$MODDIR/state/scene_mode.txt"

SCENE_ON=$("$BIN" config get 2>/dev/null | grep '"scene_enabled"' | grep -c true)
SCENE_INSTALLED=0
if /system/bin/pm path com.omarea.vtools >/dev/null 2>&1; then SCENE_INSTALLED=1; fi
if [ "$SCENE_ON" -gt 0 ] && [ "$SCENE_INSTALLED" -eq 1 ]; then
  MODDIR="$MODDIR" sh "$MODDIR/scripts/install_scene_backend.sh" >> "$LOG" 2>&1
elif [ "$SCENE_ON" -gt 0 ]; then
  printf '%s 未检测到Scene(com.omarea.vtools)，跳过powercfg包装层；auto模式可独立工作。\n' "$(date '+%F %T')" >> "$LOG"
fi

"$BIN" daily apply >> "$LOG" 2>&1
"$BIN" watch-start >> "$LOG" 2>&1
printf '%s 启动完成：control_mode由配置决定；auto仅低频识别前台；Balance日常优化不修改CPU频率。\n' "$(date '+%F %T')" >> "$LOG"
