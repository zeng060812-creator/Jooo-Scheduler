package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func usage() {
	fmt.Print(`jood - ZenJooo SM8650 调度后端
命令：
  status [--pretty]          当前状态(JSON)
  scene <mode> <cat> <scene> Scene场景切换（control_mode=scene）
  manual <mode> <cat> <pkg>  WebUI手动切换（control_mode=webui）
  watch                      自动前台监听（前台运行）
  watch-start                后台启动自动监听
  watch-stop                 停止自动监听
  daemon <pkg> <tier>        游戏控制守护进程
  stop                       停止游戏控制并恢复
  restore                    强制恢复原厂CPU/线程/日常优化状态
  daily apply|restore|status  日常轻量优化
  config get                 读取配置
  config set <key> <value>   修改配置
  config set-game-profile <pkg> <balanced|extreme>
  config clear-game-profile <pkg>
  config add-game <pkg>      添加自定义游戏
  config remove-game <pkg>   删除自定义游戏
  config export              导出配置到 Download
  config import [path]       导入配置；省略path时导入Download最新文件
  games                      已安装游戏(JSON)
  foreground                 当前前台APP(JSON)
  capture [seconds]          延时捕获前台APP并记录
  selftest                   自检(JSON)
  diag                       导出诊断
`)
}

func main() {
	root := moduleRoot()
	_ = os.MkdirAll(filepath.Join(root, "state"), 0755)
	_ = os.MkdirAll(logDir(), 0755)
	if len(os.Args) < 2 {
		usage()
		return
	}
	var err error
	switch os.Args[1] {
	case "status":
		err = cmdStatus(root, len(os.Args) > 2 && os.Args[2] == "--pretty")
	case "scene":
		mode, cat, scene := "balance", "", ""
		if len(os.Args) > 2 {
			mode = os.Args[2]
		}
		if len(os.Args) > 3 {
			cat = os.Args[3]
		}
		if len(os.Args) > 4 {
			scene = os.Args[4]
		}
		err = cmdScene(root, mode, cat, scene)
	case "manual":
		mode, cat, scene := "balance", "app", ""
		if len(os.Args) > 2 {
			mode = os.Args[2]
		}
		if len(os.Args) > 3 {
			cat = os.Args[3]
		}
		if len(os.Args) > 4 {
			scene = os.Args[4]
		}
		err = cmdManual(root, mode, cat, scene)
	case "watch":
		err = cmdWatch(root)
	case "watch-start":
		err = startWatch(root)
	case "watch-stop":
		err = stopWatch(root, true)
	case "daemon":
		if len(os.Args) < 4 {
			err = fmt.Errorf("daemon需要包名和档位")
		} else {
			err = cmdDaemon(root, os.Args[2], os.Args[3])
		}
	case "stop":
		err = stopDaemon(root, true)
	case "restore":
		_ = stopDaemon(root, false)
		_ = restoreDailyOptimization(root)
		err = restoreFromFile(root)
	case "daily":
		cfg, e := loadConfig(root)
		if e != nil {
			err = e
			break
		}
		sub := "status"
		if len(os.Args) > 2 {
			sub = os.Args[2]
		}
		switch sub {
		case "apply":
			err = applyDailyOptimization(root, cfg)
		case "restore":
			err = restoreDailyOptimization(root)
		case "status":
			jsonPrint(dailyStatus(root, cfg), false)
		default:
			err = fmt.Errorf("未知daily命令: %s", sub)
		}
	case "config":
		err = cmdConfig(root, os.Args[2:])
	case "games":
		cfg, e := loadConfig(root)
		if e != nil {
			err = e
		} else {
			jsonPrint(installedGames(cfg), false)
		}
	case "foreground":
		cfg, e := loadConfig(root)
		if e != nil {
			err = e
		} else {
			pkg := foregroundPackage()
			jsonPrint(map[string]any{"package": pkg, "is_game": isGame(pkg, cfg), "captured": capturedForeground(root)}, false)
		}
	case "capture":
		delay := 5
		if len(os.Args) > 2 {
			if n, e := strconv.Atoi(os.Args[2]); e == nil {
				delay = n
			}
		}
		if delay < 1 {
			delay = 1
		}
		if delay > 15 {
			delay = 15
		}
		time.Sleep(time.Duration(delay) * time.Second)
		pkg := foregroundPackage()
		if pkg == "" {
			err = fmt.Errorf("未捕获到前台应用")
		} else {
			err = atomicWrite(filepath.Join(root, "state", "captured_foreground.txt"), []byte(pkg+"\n"), 0644)
			if err == nil {
				fmt.Println(pkg)
			}
		}
	case "selftest":
		err = cmdSelfTest(root)
	case "diag":
		err = cmdDiag(root)
	case "version":
		fmt.Println(version)
	default:
		usage()
		err = fmt.Errorf("未知命令: %s", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func chooseTier(cfg Config, sceneMode string) string {
	if cfg.FollowSceneTier {
		return normalizeTier(sceneMode)
	}
	return normalizeTier(cfg.FixedGameTier)
}

func applyControlEvent(root, source, mode, category, scene string) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	mode = normalizeTier(mode)
	pkg := strings.SplitN(scene, "/", 2)[0]
	if pkg == "" && scene != "screen_off" {
		pkg = foregroundPackage()
		scene = pkg
	}
	if category != "game" && category != "app" {
		category = "app"
	}
	if pkg != "" && isGame(pkg, cfg) {
		category = "game"
	}
	if scene == "screen_off" {
		category = "app"
		pkg = ""
		mode = "powersave"
	}
	if err := writeSceneState(root, mode, category, scene); err != nil {
		return err
	}
	appendHistory(root, mode, category, pkg)
	logLine(root, 0, cfg, "%s切换 mode=%s category=%s pkg=%s", source, mode, category, pkg)
	if category == "game" && cfg.GameEngine && pkg != "" {
		_ = restoreDailyOptimization(root)
		tier := mode
		src := strings.ToLower(source)
		if strings.HasPrefix(src, "scene") {
			tier = chooseTier(cfg, mode)
		}
		if strings.HasPrefix(src, "auto") {
			tier = normalizeTier(cfg.FixedGameTier)
		}
		return startDaemon(root, pkg, tier)
	}
	if err := stopDaemon(root, true); err != nil {
		return err
	}
	if mode == "balance" {
		return applyDailyOptimization(root, cfg)
	}
	return restoreDailyOptimization(root)
}

func cmdScene(root, mode, category, scene string) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	if cfg.ControlMode != "scene" {
		logLine(root, 2, cfg, "忽略Scene事件：control_mode=%s", cfg.ControlMode)
		return nil
	}
	if !cfg.SceneEnabled {
		return nil
	}
	return applyControlEvent(root, "Scene", mode, category, scene)
}

func cmdManual(root, mode, category, scene string) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	if cfg.ControlMode != "webui" {
		return fmt.Errorf("当前control_mode=%s，不接受WebUI手动切换", cfg.ControlMode)
	}
	if category == "game" && strings.TrimSpace(scene) == "" {
		scene = foregroundPackage()
	}
	return applyControlEvent(root, "WebUI", mode, category, scene)
}

func startDaemon(root, pkg, tier string) error {
	pidFile := filepath.Join(root, "state", "daemon.pid")
	if pid := readPID(pidFile); pidAlive(pid) {
		var st DaemonStatus
		if b, e := os.ReadFile(filepath.Join(root, "state", "daemon_status.json")); e == nil {
			_ = json.Unmarshal(b, &st)
		}
		if st.Package == pkg && st.Tier == tier {
			return nil
		}
		_ = stopDaemon(root, true)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "daemon", pkg, tier)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

func stopDaemon(root string, wait bool) error {
	pidFile := filepath.Join(root, "state", "daemon.pid")
	pid := readPID(pidFile)
	if pid > 1 && pidAlive(pid) {
		if p, e := os.FindProcess(pid); e == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
		if wait {
			deadline := time.Now().Add(2500 * time.Millisecond)
			for pidAlive(pid) && time.Now().Before(deadline) {
				time.Sleep(80 * time.Millisecond)
			}
		}
	}
	if pidAlive(pid) {
		return fmt.Errorf("游戏守护进程未能及时退出 pid=%d", pid)
	}
	_ = os.Remove(pidFile)
	return restoreFromFile(root)
}

func cmdDaemon(root, pkg, tier string) error {
	if ok, detail := supportedHardware(); !ok {
		return fmt.Errorf("仅支持Snapdragon 8 Gen 3 / SM8650系列: %s", detail)
	}
	if !validPackage(pkg) {
		return fmt.Errorf("无效游戏包名")
	}
	c, err := newController(root, pkg, tier)
	if err != nil {
		return err
	}
	return c.run()
}

func cmdConfig(root string, args []string) error {
	if len(args) == 0 || args[0] == "get" {
		cfg, e := loadConfig(root)
		if e != nil {
			return e
		}
		jsonPrint(cfg, true)
		return nil
	}
	switch args[0] {
	case "set":
		if len(args) < 3 {
			return fmt.Errorf("config set需要key value")
		}
		if err := configSet(root, args[1], args[2]); err != nil {
			return err
		}
		return applyConfigMutation(root, args[1])
	case "add-game":
		if len(args) < 2 {
			return fmt.Errorf("缺少包名")
		}
		if err := configAddGame(root, args[1]); err != nil {
			return err
		}
		return applyConfigMutation(root, "custom_games")
	case "remove-game":
		if len(args) < 2 {
			return fmt.Errorf("缺少包名")
		}
		if err := configRemoveGame(root, args[1]); err != nil {
			return err
		}
		return applyConfigMutation(root, "custom_games")
	case "set-game-profile":
		if len(args) < 3 {
			return fmt.Errorf("需要包名和游戏类型")
		}
		if err := configSetGameProfile(root, args[1], args[2]); err != nil {
			return err
		}
		return applyConfigMutation(root, "game_profiles")
	case "clear-game-profile":
		if len(args) < 2 {
			return fmt.Errorf("缺少包名")
		}
		if err := configClearGameProfile(root, args[1]); err != nil {
			return err
		}
		return applyConfigMutation(root, "game_profiles")
	case "export":
		path, err := exportConfig(root)
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	case "import":
		path := ""
		if len(args) > 1 {
			path = args[1]
		}
		used, err := importConfig(root, path)
		if err != nil {
			return err
		}
		if err := applyConfigMutation(root, "reset"); err != nil {
			return err
		}
		fmt.Println(used)
		return nil
	case "reset":
		cfg := defaultConfig()
		cfg.OnboardingDone = true
		if err := saveConfig(root, cfg); err != nil {
			return err
		}
		return applyConfigMutation(root, "reset")
	default:
		return fmt.Errorf("未知config命令")
	}
}

func applyConfigMutation(root, key string) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}

	runScript := func(name string) error {
		path := filepath.Join(root, "scripts", name)
		cmd := exec.Command("/system/bin/sh", path)
		cmd.Env = append(os.Environ(), "MODDIR="+root)
		out, e := cmd.CombinedOutput()
		if e != nil {
			return fmt.Errorf("%s执行失败: %v: %s", name, e, strings.TrimSpace(string(out)))
		}
		return nil
	}

	if key == "scene_enabled" || key == "reset" {
		if cfg.SceneEnabled {
			if err := runScript("install_scene_backend.sh"); err != nil {
				return err
			}
		} else if err := runScript("restore_scene_backend.sh"); err != nil {
			return err
		}
	}

	switch key {
	case "control_mode", "watch_enabled", "watch_interval_ms", "reset":
		_ = stopDaemon(root, true)
		if key == "control_mode" || key == "reset" {
			_ = stopWatch(root, false)
		}
		if err := syncWatch(root); err != nil {
			return err
		}
		if cfg.ControlMode == "scene" && cfg.SceneEnabled {
			if sceneInstalled() {
				if err := runScript("install_scene_backend.sh"); err != nil {
					return err
				}
			}
			mode := getprop("vtools.powercfg")
			if mode == "" {
				mode = "balance"
			}
			pkg := getprop("vtools.powercfg_app")
			cat := "app"
			if isGame(pkg, cfg) {
				cat = "game"
			}
			return applyControlEvent(root, "Scene恢复", mode, cat, pkg)
		}
		return nil
	case "scene_enabled":
		if cfg.ControlMode == "scene" && !cfg.SceneEnabled {
			return stopDaemon(root, true)
		}
		return syncWatch(root)
	case "game_engine":
		if !cfg.GameEngine {
			return stopDaemon(root, true)
		}
		if cfg.ControlMode == "auto" {
			return restartWatch(root)
		}
		if cfg.ControlMode == "scene" && cfg.SceneEnabled {
			mode, category, scene, _ := readSceneState(root)
			return applyControlEvent(root, "Scene恢复", mode, category, scene)
		}
	case "daily_opt_enabled", "daily_bg_limit", "zram_prefer_zstd":
		mode, category, _, _ := readSceneState(root)
		if category != "game" && mode == "balance" {
			return applyDailyOptimization(root, cfg)
		}
		return restoreDailyOptimization(root)
	case "follow_scene_tier", "fixed_game_tier", "game_profile_default", "game_profiles", "adaptive_game_load", "thermal_guard", "battery_temp_limit_c", "custom_games", "onboarding_done":
		if cfg.ControlMode == "auto" && (key == "fixed_game_tier" || key == "custom_games") {
			return restartWatch(root)
		}
		if cfg.ControlMode == "scene" && cfg.SceneEnabled && cfg.GameEngine {
			mode, category, scene, _ := readSceneState(root)
			return applyControlEvent(root, "Scene热重载", mode, category, scene)
		}
	}
	return nil
}

type Status struct {
	ModuleID          string         `json:"module_id"`
	Version           string         `json:"version"`
	Author            string         `json:"author"`
	Device            string         `json:"device"`
	Model             string         `json:"model"`
	SOC               string         `json:"soc"`
	SceneInstalled    bool           `json:"scene_installed"`
	SceneMode         string         `json:"scene_mode"`
	Category          string         `json:"category"`
	Scene             string         `json:"scene"`
	Package           string         `json:"package"`
	ForegroundPackage string         `json:"foreground_package"`
	CapturedPackage   string         `json:"captured_package"`
	DataDir           string         `json:"data_dir"`
	DailyPolicy       string         `json:"daily_policy"`
	PowerW            float64        `json:"power_w"`
	CurrentMA         float64        `json:"current_ma"`
	VoltageV          float64        `json:"voltage_v"`
	TempC             float64        `json:"temp_c"`
	BatteryTempC      float64        `json:"battery_temp_c"`
	Policies          []Policy       `json:"policies"`
	Daemon            DaemonStatus   `json:"daemon"`
	Watch             WatchStatus    `json:"watch"`
	History           []HistoryEntry `json:"history"`
	Config            Config         `json:"config"`
	Daily             DailyStatus    `json:"daily"`
}

func cmdStatus(root string, pretty bool) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	mode, cat, scene, pkg := readSceneState(root)
	w, ma, v := batteryPower()
	st := Status{ModuleID: moduleID, Version: version, Author: "ZenJooo", Device: getprop("ro.product.device"), Model: getprop("ro.product.model"), SOC: getprop("ro.soc.model"), SceneInstalled: sceneInstalled(), SceneMode: mode, Category: cat, Scene: scene, Package: pkg, ForegroundPackage: foregroundPackage(), CapturedPackage: capturedForeground(root), DataDir: dataRoot(), DailyPolicy: "OEM原厂CPU调速器", PowerW: floatRound(w, 2), CurrentMA: floatRound(ma, 0), VoltageV: floatRound(v, 2), TempC: floatRound(maxCPUTempC(), 1), BatteryTempC: floatRound(batteryTempC(), 1), Policies: discoverPolicies(), History: readHistory(root, 8), Config: cfg, Daily: dailyStatus(root, cfg)}
	st.Watch = readWatchStatus(root)
	if b, e := os.ReadFile(filepath.Join(root, "state", "daemon_status.json")); e == nil {
		_ = json.Unmarshal(b, &st.Daemon)
	}
	if pid := readPID(filepath.Join(root, "state", "daemon.pid")); pidAlive(pid) {
		st.Daemon.Running = true
		st.Daemon.PID = pid
	} else {
		st.Daemon.Running = false
	}
	jsonPrint(st, pretty)
	return nil
}

func cmdSelfTest(root string) error {
	cfg, _ := loadConfig(root)
	checks := []map[string]any{}
	add := func(name string, ok bool, detail string) {
		checks = append(checks, map[string]any{"name": name, "ok": ok, "detail": detail})
	}
	socs := strings.ToLower(strings.Join([]string{getprop("ro.soc.model"), getprop("ro.vendor.soc.model"), getprop("ro.product.soc.model")}, " "))
	add("SM8650 SoC", strings.Contains(socs, "sm8650"), strings.TrimSpace(socs))
	okHW, hwDetail := supportedHardware()
	add("8 Gen 3平台校验", okHW, hwDetail)
	ps := discoverPolicies()
	add("动态CPU policy", len(ps) >= 3, fmt.Sprintf("检测到%d个 policy / CPUs=%s", len(ps), compressCPUList(totalPolicyCPUs(ps))))
	rolesOK := len(policyByRole(ps, roleEfficiency)) > 0 && len(policyByRole(ps, rolePrimary)) > 0 && len(policyByRole(ps, rolePrime)) > 0
	roleText := []string{}
	for _, p := range ps {
		roleText = append(roleText, fmt.Sprintf("p%d:%s cpu=%s cap=%d max=%d", p.ID, p.Role, p.CPUs, p.Capacity, p.HWMax))
	}
	add("动态簇角色", rolesOK, strings.Join(roleText, " | "))
	backend := strings.Contains(readTrim("/data/powercfg.sh"), "YUMI_JOOO_SCENE_WRAPPER")
	needSceneBackend := cfg.ControlMode == "scene" && cfg.SceneEnabled
	add("Scene后端", !needSceneBackend || backend, fmt.Sprintf("需要=%v 路径=/data/powercfg.sh", needSceneBackend))
	add("Scene开关", cfg.SceneEnabled, fmt.Sprint(cfg.SceneEnabled))
	add("游戏引擎", cfg.GameEngine, fmt.Sprint(cfg.GameEngine))
	add("游戏类型", true, fmt.Sprintf("默认=%s 自适应=%v", cfg.GameProfileDefault, cfg.AdaptiveGameLoad))
	add("日常轻量优化", cfg.DailyOptimize, fmt.Sprintf("background=%v zram_zstd=%v", cfg.DailyBackgroundLimit, cfg.ZramPreferZstd))
	add("控制模式", cfg.ControlMode == "auto" || cfg.ControlMode == "scene" || cfg.ControlMode == "webui", cfg.ControlMode)
	ws := readWatchStatus(root)
	wantWatch := cfg.ControlMode == "auto" && cfg.WatchEnabled
	add("自动监听", !wantWatch || ws.Running, fmt.Sprintf("enabled=%v running=%v pid=%d", cfg.WatchEnabled, ws.Running, ws.PID))
	bt := batteryTempC()
	add("电池温度传感器", bt > 0, fmt.Sprintf("%.1f°C", bt))
	if e := os.MkdirAll(logDir(), 0755); e == nil {
		add("持久日志目录", true, logDir())
	} else {
		add("持久日志目录", false, e.Error())
	}
	ok := true
	for _, c := range checks {
		if v, _ := c["ok"].(bool); !v {
			ok = false
		}
	}
	jsonPrint(map[string]any{"ok": ok, "checks": checks}, false)
	return nil
}

func cmdDiag(root string) error {
	ts := time.Now().Format("20060102-150405")
	path := fmt.Sprintf("/sdcard/Download/JoooScheduler_Diag_%s.txt", ts)
	var sb strings.Builder
	sb.WriteString("ZenJooo Jooo Scheduler 诊断\n")
	sb.WriteString("生成时间: " + time.Now().Format(time.RFC3339) + "\n\n")
	sb.WriteString("=== STATUS ===\n")
	// Rebuild status as JSON by marshaling current data via a temporary object isn't exposed; call self binary status.
	exe, _ := os.Executable()
	if b, e := exec.Command(exe, "status", "--pretty").Output(); e == nil {
		sb.Write(b)
	}
	sb.WriteString("\n=== SELFTEST ===\n")
	if b, e := exec.Command(exe, "selftest").Output(); e == nil {
		sb.Write(b)
	}
	sb.WriteString("\n=== LOGS ===\n")
	for _, name := range []string{"service.log", "scene.log", "jood.log", "jood.log.1"} {
		path := filepath.Join(logDir(), name)
		if b, e := os.ReadFile(path); e == nil {
			if len(b) > 20000 {
				b = b[len(b)-20000:]
			}
			sb.WriteString("\n----- " + name + " -----\n")
			sb.Write(b)
		}
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func init() { _ = strconv.IntSize }
