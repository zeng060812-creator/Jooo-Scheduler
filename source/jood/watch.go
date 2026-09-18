package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type WatchStatus struct {
	Running    bool   `json:"running"`
	PID        int    `json:"pid"`
	Foreground string `json:"foreground"`
	Category   string `json:"category"`
	Mode       string `json:"mode"`
	ScreenOn   bool   `json:"screen_on"`
	UpdatedAt  int64  `json:"updated_at"`
}

func watchPIDPath(root string) string    { return filepath.Join(root, "state", "watch.pid") }
func watchStatusPath(root string) string { return filepath.Join(root, "state", "watch_status.json") }

func readWatchStatus(root string) WatchStatus {
	var st WatchStatus
	if b, err := os.ReadFile(watchStatusPath(root)); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	pid := readPID(watchPIDPath(root))
	st.Running = pidAlive(pid)
	if st.Running {
		st.PID = pid
	} else {
		st.PID = 0
	}
	return st
}

func writeWatchStatus(root string, st WatchStatus) {
	st.Running = true
	st.PID = os.Getpid()
	st.UpdatedAt = time.Now().Unix()
	b, _ := json.Marshal(st)
	_ = atomicWrite(watchStatusPath(root), append(b, '\n'), 0644)
}

func startWatch(root string) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	if cfg.ControlMode != "auto" || !cfg.WatchEnabled {
		return nil
	}
	if pid := readPID(watchPIDPath(root)); pidAlive(pid) {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, "watch")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

func stopWatch(root string, stopGame bool) error {
	pid := readPID(watchPIDPath(root))
	if pid > 1 && pidAlive(pid) {
		if p, e := os.FindProcess(pid); e == nil {
			_ = p.Signal(syscall.SIGTERM)
		}
		deadline := time.Now().Add(2200 * time.Millisecond)
		for pidAlive(pid) && time.Now().Before(deadline) {
			time.Sleep(60 * time.Millisecond)
		}
	}
	_ = os.Remove(watchPIDPath(root))
	_ = os.Remove(watchStatusPath(root))
	if stopGame {
		return stopDaemon(root, true)
	}
	return nil
}

func restartWatch(root string) error {
	_ = stopWatch(root, false)
	return startWatch(root)
}

func syncWatch(root string) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	if cfg.ControlMode == "auto" && cfg.WatchEnabled {
		return startWatch(root)
	}
	return stopWatch(root, false)
}

func screenOn() bool {
	// 优先读取背光电源状态，成本远低于 dumpsys。常见值：0=亮屏，4=关闭。
	if paths, _ := filepath.Glob("/sys/class/backlight/*/bl_power"); len(paths) > 0 {
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return readInt64(p) == 0
			}
		}
	}
	// 次选亮度节点。
	if paths, _ := filepath.Glob("/sys/class/backlight/*/brightness"); len(paths) > 0 {
		anyReadable := false
		for _, p := range paths {
			v := readInt64(p)
			if _, err := os.Stat(p); err == nil {
				anyReadable = true
			}
			if v > 0 {
				return true
			}
		}
		if anyReadable {
			return false
		}
	}
	b, err := exec.Command("/system/bin/dumpsys", "power").Output()
	if err != nil {
		return true
	}
	s := string(b)
	if strings.Contains(s, "mWakefulness=Asleep") || strings.Contains(s, "mWakefulness=Dozing") {
		return false
	}
	if strings.Contains(s, "mWakefulness=Awake") {
		return true
	}
	return true
}

func watchDecision(on bool, pkg string, cfg Config) (mode, category, scene string) {
	if !on {
		return "powersave", "app", "screen_off"
	}
	mode, category, scene = "balance", "app", pkg
	if pkg != "" && isGame(pkg, cfg) {
		mode = normalizeTier(cfg.FixedGameTier)
		category = "game"
	}
	return
}

func cmdWatch(root string) error {
	if ok, detail := supportedHardware(); !ok {
		return fmt.Errorf("仅支持Snapdragon 8 Gen 3 / SM8650系列: %s", detail)
	}
	if pid := readPID(watchPIDPath(root)); pidAlive(pid) && pid != os.Getpid() {
		return fmt.Errorf("watch已运行 pid=%d", pid)
	}
	if err := atomicWrite(watchPIDPath(root), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644); err != nil {
		return err
	}
	defer func() { _ = os.Remove(watchPIDPath(root)); _ = os.Remove(watchStatusPath(root)) }()

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	lastKey := ""
	cfg, _ := loadConfig(root)
	logLine(root, 0, cfg, "自动监听启动 interval=%dms", cfg.WatchIntervalMS)

	for {
		cfg, err := loadConfig(root)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		if cfg.ControlMode != "auto" || !cfg.WatchEnabled {
			logLine(root, 0, cfg, "自动监听退出 control_mode=%s enabled=%v", cfg.ControlMode, cfg.WatchEnabled)
			_ = stopDaemon(root, true)
			return nil
		}

		on := screenOn()
		pkg := ""
		if on {
			pkg = foregroundPackage()
		}
		mode, category, scene := watchDecision(on, pkg, cfg)
		key := fmt.Sprintf("%t|%s|%s|%s", on, mode, category, pkg)
		if key != lastKey {
			if err := applyControlEvent(root, "auto", mode, category, scene); err != nil {
				logLine(root, 0, cfg, "自动监听切换失败: %v", err)
			} else {
				lastKey = key
			}
		}
		writeWatchStatus(root, WatchStatus{Foreground: pkg, Category: category, Mode: mode, ScreenOn: on})

		wait := time.Duration(cfg.WatchIntervalMS) * time.Millisecond
		timer := time.NewTimer(wait)
		select {
		case <-sig:
			if !timer.Stop() {
				<-timer.C
			}
			logLine(root, 0, cfg, "自动监听收到停止信号")
			return nil
		case <-timer.C:
		}
	}
}
