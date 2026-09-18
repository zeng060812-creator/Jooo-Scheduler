#!/system/bin/sh
# Jooo Scheduler Scene9 wrapper installer
# v1.1.1: preserve Scene's original powercfg.json and forward original powercfg.sh.

MODDIR="${MODDIR:-/data/adb/modules/yumi_jooo}"
DATA_ROOT="${JOOO_DATA_ROOT:-/data}"
DATADIR="$DATA_ROOT/adb/yumi_jooo"
BACKUP="$DATADIR/scene_backup"
LOG="$DATADIR/logs/scene.log"
POWERCFG_SH="$DATA_ROOT/powercfg.sh"
POWERCFG_JSON="$DATA_ROOT/powercfg.json"

mkdir -p "$BACKUP" "$DATADIR/logs"

log_msg() {
  printf '%s %s\n' "$(date '+%F %T')" "$*" >> "$LOG"
}

# Migrate backup names used by older Jooo builds.
for OLD in \
  "$DATA_ROOT/adb/yumi_jooo_scene_backup" \
  "$DATA_ROOT/adb/zenjooo_yumi_scene_backup"; do
  [ -d "$OLD" ] || continue
  [ -f "$BACKUP/powercfg.sh.orig" ] || {
    [ -f "$OLD/powercfg.sh.orig" ] && cp -af "$OLD/powercfg.sh.orig" "$BACKUP/powercfg.sh.orig"
    [ -f "$BACKUP/powercfg.sh.orig" ] || [ ! -f "$OLD/powercfg.sh" ] || cp -af "$OLD/powercfg.sh" "$BACKUP/powercfg.sh.orig"
  }
  [ -f "$BACKUP/powercfg.json.orig" ] || {
    [ -f "$OLD/powercfg.json.orig" ] && cp -af "$OLD/powercfg.json.orig" "$BACKUP/powercfg.json.orig"
    [ -f "$BACKUP/powercfg.json.orig" ] || [ ! -f "$OLD/powercfg.json" ] || cp -af "$OLD/powercfg.json" "$BACKUP/powercfg.json.orig"
  }
done

# Migrate the v1.1.0 backup directory names.
if [ ! -f "$BACKUP/powercfg.sh.orig" ] && [ -f "$BACKUP/powercfg.sh" ]; then
  cp -af "$BACKUP/powercfg.sh" "$BACKUP/powercfg.sh.orig"
fi
if [ ! -f "$BACKUP/powercfg.json.orig" ] && [ -f "$BACKUP/powercfg.json" ]; then
  cp -af "$BACKUP/powercfg.json" "$BACKUP/powercfg.json.orig"
fi

# Backup the real pre-existing Scene/user script ONCE.
# Never backup Jooo's current or legacy wrapper/backend over the original.
if [ -f "$POWERCFG_SH" ] \
  && ! grep -q -E 'YUMI_JOOO_SCENE_WRAPPER|YUMI_JOOO_SCENE9_BACKEND|ZENJOOO_YUMI_SCENE9_BACKEND|ZENJOOO_YUMI_SCENE_N1_BACKEND' "$POWERCFG_SH" 2>/dev/null \
  && [ ! -f "$BACKUP/powercfg.sh.orig" ]; then
  cp -af "$POWERCFG_SH" "$BACKUP/powercfg.sh.orig"
  log_msg "已备份Scene原始powercfg.sh。"
fi

# powercfg.json: v1.1.1 NEVER installs/replaces it.
# Repair legacy Jooo installs: if current json is ours and an original backup exists,
# restore the original immediately so Scene metadata/statistics are visible again.
if [ -f "$POWERCFG_JSON" ]; then
  if grep -q -E 'yumi_jooo|Jooo Scheduler|ZenJooo.*SM8650' "$POWERCFG_JSON" 2>/dev/null; then
    if [ -f "$BACKUP/powercfg.json.orig" ]; then
      cp -af "$BACKUP/powercfg.json.orig" "$POWERCFG_JSON"
      chmod 0644 "$POWERCFG_JSON" 2>/dev/null
      log_msg "检测到旧Jooo powercfg.json，已恢复Scene原始JSON。"
    else
      log_msg "警告：当前powercfg.json属于旧Jooo，但未找到原始JSON备份；本版不再继续覆盖。"
    fi
  elif [ ! -f "$BACKUP/powercfg.json.orig" ]; then
    # Keep a safety copy, but never replace the live json.
    cp -af "$POWERCFG_JSON" "$BACKUP/powercfg.json.orig"
    log_msg "已备份当前Scene powercfg.json；运行文件保持不变。"
  fi
fi

# Install wrapper. It calls jood asynchronously, then forwards the exact original
# arguments/environment to Scene's original script. If no original exists, Jooo
# still receives mode notifications and exits normally.
cat > "$POWERCFG_SH" <<'JOOO_WRAPPER'
#!/system/bin/sh
# YUMI_JOOO_SCENE_WRAPPER
# Jooo包装层：旁路接收Scene事件，然后完整转发给原始Scene脚本。

DATA_ROOT="${JOOO_DATA_ROOT:-/data}"
MODDIR="${JOOO_MODDIR:-/data/adb/modules/yumi_jooo}"
ORIG="$DATA_ROOT/adb/yumi_jooo/scene_backup/powercfg.sh.orig"
BIN="$MODDIR/bin/jood"

MODE="${mode:-${1:-balance}}"
CATEGORY="${category:-}"
SCENE_NAME="${scene:-${top_app:-${2:-}}}"

# 不阻塞Scene原始逻辑。环境变量(top_app等)保持继承。
if [ -x "$BIN" ]; then
  "$BIN" scene "$MODE" "$CATEGORY" "$SCENE_NAME" >/dev/null 2>&1 &
fi

# 原始Scene/第三方逻辑继续完整执行，保持原有统计和行为。
if [ -f "$ORIG" ]; then
  exec sh "$ORIG" "$@"
fi

exit 0
JOOO_WRAPPER

chmod 0755 "$POWERCFG_SH" || exit 1
log_msg "Scene包装层已安装：powercfg.sh转发原逻辑，powercfg.json保持Scene原文件。"
exit 0
