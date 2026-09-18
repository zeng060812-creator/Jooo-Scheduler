#!/system/bin/sh
# Restore only files Jooo owns. powercfg.json is never removed by v1.1.1.

DATA_ROOT="${JOOO_DATA_ROOT:-/data}"
BACKUP="$DATA_ROOT/adb/yumi_jooo/scene_backup"
POWERCFG_SH="$DATA_ROOT/powercfg.sh"
POWERCFG_JSON="$DATA_ROOT/powercfg.json"

# Only touch powercfg.sh if our wrapper/backend still owns it; never overwrite a
# scheduler the user installed after Jooo.
if grep -q -E 'YUMI_JOOO_SCENE_WRAPPER|YUMI_JOOO_SCENE9_BACKEND|ZENJOOO_YUMI_SCENE9_BACKEND|ZENJOOO_YUMI_SCENE_N1_BACKEND' "$POWERCFG_SH" 2>/dev/null; then
  if [ -f "$BACKUP/powercfg.sh.orig" ]; then
    cp -af "$BACKUP/powercfg.sh.orig" "$POWERCFG_SH"
    chmod 0755 "$POWERCFG_SH" 2>/dev/null
  else
    rm -f "$POWERCFG_SH"
  fi
fi

# Legacy cleanup only: if an old Jooo JSON is still live and we have the original,
# restore it. Otherwise leave the user's/Scene JSON untouched.
if [ -f "$POWERCFG_JSON" ] \
  && grep -q -E 'yumi_jooo|Jooo Scheduler|ZenJooo.*SM8650' "$POWERCFG_JSON" 2>/dev/null \
  && [ -f "$BACKUP/powercfg.json.orig" ]; then
  cp -af "$BACKUP/powercfg.json.orig" "$POWERCFG_JSON"
  chmod 0644 "$POWERCFG_JSON" 2>/dev/null
fi

exit 0
