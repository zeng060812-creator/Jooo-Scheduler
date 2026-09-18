package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Version              int               `json:"version"`
	OnboardingDone       bool              `json:"onboarding_done"`
	ControlMode          string            `json:"control_mode"`
	WatchEnabled         bool              `json:"watch_enabled"`
	WatchIntervalMS      int               `json:"watch_interval_ms"`
	SceneEnabled         bool              `json:"scene_enabled"`
	GameEngine           bool              `json:"game_engine"`
	FollowSceneTier      bool              `json:"follow_scene_tier"`
	FixedGameTier        string            `json:"fixed_game_tier"`
	GameProfileDefault   string            `json:"game_profile_default"`
	GameProfiles         map[string]string `json:"game_profiles"`
	AdaptiveGameLoad     bool              `json:"adaptive_game_load"`
	ThreadPlacement      bool              `json:"thread_placement"`
	PrimeGate            float64           `json:"prime_gate"`
	SampleMS             int               `json:"sample_ms"`
	LogLevel             int               `json:"log_level"`
	ThermalGuard         bool              `json:"thermal_guard"`
	BatteryTempLimit     float64           `json:"battery_temp_limit_c"`
	DailyOptimize        bool              `json:"daily_opt_enabled"`
	DailyBackgroundLimit bool              `json:"daily_bg_limit"`
	ZramPreferZstd       bool              `json:"zram_prefer_zstd"`
	CustomGames          []string          `json:"custom_games"`
}

func defaultConfig() Config {
	return Config{
		Version:              5,
		OnboardingDone:       false,
		ControlMode:          "auto",
		WatchEnabled:         true,
		WatchIntervalMS:      2000,
		SceneEnabled:         true,
		GameEngine:           true,
		FollowSceneTier:      true,
		FixedGameTier:        "balance",
		GameProfileDefault:   "balanced",
		GameProfiles:         map[string]string{},
		AdaptiveGameLoad:     true,
		ThreadPlacement:      true,
		PrimeGate:            0.72,
		SampleMS:             120,
		LogLevel:             1,
		ThermalGuard:         true,
		BatteryTempLimit:     45.0,
		DailyOptimize:        true,
		DailyBackgroundLimit: true,
		ZramPreferZstd:       true,
		CustomGames:          []string{},
	}
}

func configPath(root string) string { return filepath.Join(root, "state", "config.json") }

func loadConfig(root string) (Config, error) {
	cfg := defaultConfig()
	b, err := os.ReadFile(configPath(root))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := saveConfig(root, cfg); err != nil {
				return cfg, err
			}
			return cfg, nil
		}
		return cfg, err
	}
	var probe struct {
		Version        int     `json:"version"`
		ControlMode    *string `json:"control_mode"`
		OnboardingDone *bool   `json:"onboarding_done"`
	}
	_ = json.Unmarshal(b, &probe)
	if err := json.Unmarshal(b, &cfg); err != nil {
		return defaultConfig(), err
	}
	// v1.1.x 以前只有 Scene 控制。升级时保持原行为，避免突然被 auto 抢管；
	// 全新安装则使用 defaultConfig() 的 auto。
	if probe.Version < 3 && probe.ControlMode == nil {
		if cfg.SceneEnabled {
			cfg.ControlMode = "scene"
		} else {
			cfg.ControlMode = "auto"
		}
	}
	// v1.4及以前的老用户升级时不强制弹首次引导；全新安装保持 false。
	if probe.Version > 0 && probe.Version < 5 && probe.OnboardingDone == nil {
		cfg.OnboardingDone = true
	}
	normalizeConfig(&cfg)
	return cfg, nil
}

func normalizeConfig(c *Config) {
	c.Version = 5
	switch c.ControlMode {
	case "auto", "scene", "webui":
	default:
		c.ControlMode = "auto"
	}
	if c.WatchIntervalMS < 500 {
		c.WatchIntervalMS = 500
	}
	if c.WatchIntervalMS > 10000 {
		c.WatchIntervalMS = 10000
	}
	switch c.FixedGameTier {
	case "powersave", "balance", "performance", "fast":
	default:
		c.FixedGameTier = "balance"
	}
	c.GameProfileDefault = normalizeGameProfile(c.GameProfileDefault)
	if c.GameProfiles == nil {
		c.GameProfiles = map[string]string{}
	}
	profiles := make(map[string]string, len(c.GameProfiles))
	for pkg, profile := range c.GameProfiles {
		pkg = strings.TrimSpace(pkg)
		if validPackage(pkg) {
			profiles[pkg] = normalizeGameProfile(profile)
		}
	}
	c.GameProfiles = profiles
	if c.PrimeGate < 0.50 {
		c.PrimeGate = 0.50
	}
	if c.PrimeGate > 0.95 {
		c.PrimeGate = 0.95
	}
	if c.SampleMS < 80 {
		c.SampleMS = 80
	}
	if c.SampleMS > 500 {
		c.SampleMS = 500
	}
	if c.LogLevel < 0 {
		c.LogLevel = 0
	}
	if c.LogLevel > 2 {
		c.LogLevel = 2
	}
	if c.BatteryTempLimit < 42.0 {
		c.BatteryTempLimit = 42.0
	}
	if c.BatteryTempLimit > 50.0 {
		c.BatteryTempLimit = 50.0
	}
	seen := map[string]bool{}
	out := c.CustomGames[:0]
	for _, p := range c.CustomGames {
		p = strings.TrimSpace(p)
		if validPackage(p) && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	c.CustomGames = out
}

func saveConfig(root string, cfg Config) error {
	normalizeConfig(&cfg)
	if err := os.MkdirAll(filepath.Dir(configPath(root)), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return atomicWrite(configPath(root), b, 0644)
}

func configSet(root, key, value string) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	parseBool := func(v string) (bool, error) {
		switch strings.ToLower(v) {
		case "1", "true", "on", "yes":
			return true, nil
		case "0", "false", "off", "no":
			return false, nil
		default:
			return false, fmt.Errorf("无效布尔值: %s", v)
		}
	}
	switch key {
	case "onboarding_done":
		cfg.OnboardingDone, err = parseBool(value)
	case "control_mode":
		cfg.ControlMode = strings.ToLower(strings.TrimSpace(value))
		if cfg.ControlMode != "auto" && cfg.ControlMode != "scene" && cfg.ControlMode != "webui" {
			return fmt.Errorf("无效控制模式: %s", value)
		}
	case "watch_enabled":
		cfg.WatchEnabled, err = parseBool(value)
	case "watch_interval_ms":
		var n int
		n, err = strconv.Atoi(value)
		if err == nil {
			cfg.WatchIntervalMS = n
		}
	case "scene_enabled":
		cfg.SceneEnabled, err = parseBool(value)
	case "game_engine":
		cfg.GameEngine, err = parseBool(value)
	case "follow_scene_tier":
		cfg.FollowSceneTier, err = parseBool(value)
	case "thread_placement":
		cfg.ThreadPlacement, err = parseBool(value)
	case "fixed_game_tier":
		cfg.FixedGameTier = value
	case "game_profile_default":
		cfg.GameProfileDefault = normalizeGameProfile(value)
	case "adaptive_game_load":
		cfg.AdaptiveGameLoad, err = parseBool(value)
	case "prime_gate":
		var f float64
		f, err = strconv.ParseFloat(value, 64)
		if err == nil {
			cfg.PrimeGate = f
		}
	case "sample_ms":
		var n int
		n, err = strconv.Atoi(value)
		if err == nil {
			cfg.SampleMS = n
		}
	case "log_level":
		var n int
		n, err = strconv.Atoi(value)
		if err == nil {
			cfg.LogLevel = n
		}
	case "thermal_guard":
		cfg.ThermalGuard, err = parseBool(value)
	case "battery_temp_limit_c":
		var f float64
		f, err = strconv.ParseFloat(value, 64)
		if err == nil {
			cfg.BatteryTempLimit = f
		}
	case "daily_opt_enabled":
		cfg.DailyOptimize, err = parseBool(value)
	case "daily_bg_limit":
		cfg.DailyBackgroundLimit, err = parseBool(value)
	case "zram_prefer_zstd":
		cfg.ZramPreferZstd, err = parseBool(value)
	default:
		return fmt.Errorf("未知配置项: %s", key)
	}
	if err != nil {
		return err
	}
	normalizeConfig(&cfg)
	if key == "fixed_game_tier" && cfg.FixedGameTier != value {
		return fmt.Errorf("无效游戏档位: %s", value)
	}
	return saveConfig(root, cfg)
}

func normalizeGameProfile(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "extreme", "max", "extreme_game":
		return "extreme"
	default:
		return "balanced"
	}
}

func recommendedGameProfile(pkg string) string {
	switch pkg {
	case "com.miHoYo.GenshinImpact", "com.miHoYo.Yuanshen", "com.miHoYo.hkrpg",
		"com.kurogame.mingchao", "com.kurogame.wutheringwaves.global",
		"com.netease.l22", "com.netease.l22.mi", "com.tencent.tmgp.dfm":
		return "extreme"
	default:
		return "balanced"
	}
}

func gameProfileForPackage(pkg string, cfg Config) string {
	if p, ok := cfg.GameProfiles[pkg]; ok {
		return normalizeGameProfile(p)
	}
	return normalizeGameProfile(cfg.GameProfileDefault)
}

func configSetGameProfile(root, pkg, profile string) error {
	if !validPackage(pkg) {
		return fmt.Errorf("无效包名: %s", pkg)
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	if cfg.GameProfiles == nil {
		cfg.GameProfiles = map[string]string{}
	}
	cfg.GameProfiles[pkg] = normalizeGameProfile(profile)
	return saveConfig(root, cfg)
}

func configClearGameProfile(root, pkg string) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	delete(cfg.GameProfiles, pkg)
	return saveConfig(root, cfg)
}

func configAddGame(root, pkg string) error {
	if !validPackage(pkg) {
		return fmt.Errorf("无效包名: %s", pkg)
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	for _, p := range cfg.CustomGames {
		if p == pkg {
			return nil
		}
	}
	cfg.CustomGames = append(cfg.CustomGames, pkg)
	return saveConfig(root, cfg)
}

func configRemoveGame(root, pkg string) error {
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	out := cfg.CustomGames[:0]
	for _, p := range cfg.CustomGames {
		if p != pkg {
			out = append(out, p)
		}
	}
	cfg.CustomGames = out
	delete(cfg.GameProfiles, pkg)
	return saveConfig(root, cfg)
}
