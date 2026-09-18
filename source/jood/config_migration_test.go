package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLegacyV2MigratesToScene(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "state"), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":2,"scene_enabled":true,"game_engine":true,"fixed_game_tier":"balance"}`
	if err := os.WriteFile(configPath(root), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ControlMode != "scene" {
		t.Fatalf("legacy v2 should preserve Scene ownership, got %s", cfg.ControlMode)
	}
}

func TestLegacyWithoutSceneMigratesToAuto(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "state"), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":2,"scene_enabled":false,"game_engine":true,"fixed_game_tier":"balance"}`
	if err := os.WriteFile(configPath(root), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ControlMode != "auto" {
		t.Fatalf("legacy without Scene should migrate to auto, got %s", cfg.ControlMode)
	}
}

func TestWatchDecision(t *testing.T) {
	cfg := defaultConfig()
	cfg.FixedGameTier = "performance"
	m, c, s := watchDecision(true, "com.tencent.tmgp.sgame", cfg)
	if m != "performance" || c != "game" || s != "com.tencent.tmgp.sgame" {
		t.Fatalf("game decision mismatch: %s %s %s", m, c, s)
	}
	m, c, s = watchDecision(true, "com.example.chat", cfg)
	if m != "balance" || c != "app" || s != "com.example.chat" {
		t.Fatalf("app decision mismatch: %s %s %s", m, c, s)
	}
	m, c, s = watchDecision(false, "com.tencent.tmgp.sgame", cfg)
	if m != "powersave" || c != "app" || s != "screen_off" {
		t.Fatalf("screen-off decision mismatch: %s %s %s", m, c, s)
	}
}

func TestV4MigrationSkipsOnboarding(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "state"), 0755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":4,"control_mode":"scene","scene_enabled":true,"game_engine":true}`
	if err := os.WriteFile(configPath(root), []byte(legacy), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.OnboardingDone || cfg.Version != 5 {
		t.Fatalf("v4 migration should skip onboarding and become v5: %+v", cfg)
	}
	if !cfg.AdaptiveGameLoad || cfg.GameProfileDefault != "balanced" {
		t.Fatalf("new defaults missing after migration: %+v", cfg)
	}
}

func TestGameProfileConfigPersistence(t *testing.T) {
	root := t.TempDir()
	if err := configSetGameProfile(root, "com.example.game", "extreme"); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GameProfiles["com.example.game"] != "extreme" {
		t.Fatalf("profile not persisted: %+v", cfg.GameProfiles)
	}
}

func TestImportConfigExplicitPath(t *testing.T) {
	root := t.TempDir()
	data := t.TempDir()
	t.Setenv("JOOO_DATA", data)
	if err := saveConfig(root, defaultConfig()); err != nil {
		t.Fatal(err)
	}
	importPath := filepath.Join(t.TempDir(), "config.json")
	payload := `{"version":5,"onboarding_done":true,"control_mode":"webui","game_profile_default":"extreme","adaptive_game_load":false,"custom_games":["com.example.game"]}`
	if err := os.WriteFile(importPath, []byte(payload), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := importConfig(root, importPath); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ControlMode != "webui" || cfg.GameProfileDefault != "extreme" || cfg.AdaptiveGameLoad {
		t.Fatalf("import mismatch: %+v", cfg)
	}
}
