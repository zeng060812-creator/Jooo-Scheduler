#!/system/bin/sh
# YUMI_JOOO_SCENE9_BACKEND
# Scene 9.4.18 / Scene9 外部调度入口。
MODDIR=/data/adb/modules/yumi_jooo
BIN="$MODDIR/bin/jood"
[ -x "$BIN" ] || exit 2

MODE="${mode:-${1:-balance}}"
CATEGORY="${category:-}"
SCENE_NAME="${scene:-${top_app:-}}"

# 首次初始化只登记日常态，不根据当前前台APP误启动游戏控制器。
if [ "$MODE" = "init" ]; then
  exec "$BIN" scene balance app ""
fi
# standby 视作日常省电状态，Jooo日常本来就没有常驻控制器。
if [ "$SCENE_NAME" = "standby" ]; then
  exec "$BIN" scene powersave app standby
fi

# 老版本Scene若只传$1，jood会自行读取当前前台APP并判断是否游戏。
exec "$BIN" scene "$MODE" "$CATEGORY" "$SCENE_NAME"
