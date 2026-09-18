package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func exportConfig(root string) (string, error) {
	cfg, err := loadConfig(root)
	if err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return "", err
	}
	b = append(b, '\n')
	dir := "/sdcard/Download"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	path := filepath.Join(dir, fmt.Sprintf("JoooScheduler_Config_%s.json", stamp))
	if err := os.WriteFile(path, b, 0644); err != nil {
		return "", err
	}
	_ = os.WriteFile(filepath.Join(dir, "JoooScheduler_Config_latest.json"), b, 0644)
	return path, nil
}

func newestExportedConfig() string {
	paths, _ := filepath.Glob("/sdcard/Download/JoooScheduler_Config*.json")
	if len(paths) == 0 {
		return ""
	}
	sort.Slice(paths, func(i, j int) bool {
		a, _ := os.Stat(paths[i])
		b, _ := os.Stat(paths[j])
		if a == nil {
			return false
		}
		if b == nil {
			return true
		}
		return a.ModTime().After(b.ModTime())
	})
	return paths[0]
}

func importConfig(root, path string) (string, error) {
	if path == "" {
		path = newestExportedConfig()
	}
	if path == "" {
		return "", fmt.Errorf("Download中没有找到JoooScheduler_Config*.json")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	cfg := defaultConfig()
	var probe struct {
		Version        int   `json:"version"`
		OnboardingDone *bool `json:"onboarding_done"`
	}
	_ = json.Unmarshal(b, &probe)
	if err := json.Unmarshal(b, &cfg); err != nil {
		return "", fmt.Errorf("配置JSON无效: %w", err)
	}
	if probe.Version > 0 && probe.Version < 5 && probe.OnboardingDone == nil {
		cfg.OnboardingDone = true
	}
	normalizeConfig(&cfg)

	backupDir := filepath.Join(dataRoot(), "config_backups")
	_ = os.MkdirAll(backupDir, 0755)
	if old, e := os.ReadFile(configPath(root)); e == nil {
		backup := filepath.Join(backupDir, "config_"+time.Now().Format("20060102-150405")+".json")
		_ = os.WriteFile(backup, old, 0644)
	}
	if err := saveConfig(root, cfg); err != nil {
		return "", err
	}
	return path, nil
}
