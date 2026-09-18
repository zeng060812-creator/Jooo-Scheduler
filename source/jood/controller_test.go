package main

import "testing"

func TestTierFloors(t *testing.T) {
	if !(tierFloor("fast", rolePrimary) > tierFloor("balance", rolePrimary) &&
		tierFloor("fast", roleSecondary) > tierFloor("balance", roleSecondary) &&
		tierFloor("fast", rolePrime) > tierFloor("balance", rolePrime)) {
		t.Fatal("fast floors should be above balance")
	}
}

func testPolicies() []Policy {
	return assignPolicyRoles([]Policy{
		{ID: 0, CPUList: []int{0, 1}, CPUs: "0-1", Capacity: 379, HWMax: 2265600},
		{ID: 2, CPUList: []int{2, 3, 4}, CPUs: "2-4", Capacity: 923, HWMax: 3148800},
		{ID: 5, CPUList: []int{5, 6}, CPUs: "5-6", Capacity: 867, HWMax: 2956800},
		{ID: 7, CPUList: []int{7}, CPUs: "7", Capacity: 1024, HWMax: 3302400},
	})
}

func TestPrimeGate(t *testing.T) {
	c := &GameController{tier: "balance", cfg: defaultConfig(), policies: testPolicies()}
	low := c.computeRatios(map[int]float64{2: 0.3, 3: 0.2, 5: 0.2, 7: 0.1}, 0.35)
	high := c.computeRatios(map[int]float64{2: 0.9, 3: 0.9, 5: 0.8, 7: 0.2}, 0.95)
	if !(high[7] > low[7]) {
		t.Fatalf("prime should open: low=%v high=%v", low[7], high[7])
	}
	if low[7] > 0.15 {
		t.Fatalf("prime too eager: %v", low[7])
	}
}

func TestPackageValidation(t *testing.T) {
	good := []string{"com.tencent.tmgp.sgame", "com.miHoYo.GenshinImpact"}
	bad := []string{"com.tencent;rm -rf /", "abc", "../bad"}
	for _, p := range good {
		if !validPackage(p) {
			t.Fatalf("expected valid: %s", p)
		}
	}
	for _, p := range bad {
		if validPackage(p) {
			t.Fatalf("expected invalid: %s", p)
		}
	}
}

func TestChooseTier(t *testing.T) {
	cfg := defaultConfig()
	if got := chooseTier(cfg, "performance"); got != "performance" {
		t.Fatalf("follow scene tier failed: %s", got)
	}
	cfg.FollowSceneTier = false
	cfg.FixedGameTier = "powersave"
	if got := chooseTier(cfg, "fast"); got != "powersave" {
		t.Fatalf("fixed tier failed: %s", got)
	}
}

func TestConfigNormalize(t *testing.T) {
	cfg := defaultConfig()
	cfg.PrimeGate = 2
	cfg.SampleMS = 1
	normalizeConfig(&cfg)
	if cfg.PrimeGate != 0.95 || cfg.SampleMS != 80 {
		t.Fatalf("normalize failed: %+v", cfg)
	}
}

func TestThermalEffectiveTier(t *testing.T) {
	c := &GameController{tier: "fast", cfg: defaultConfig(), thermalLimited: true}
	if got := c.effectiveTier(); got != "balance" {
		t.Fatalf("thermal guard should reduce fast, got %s", got)
	}
	c.tier = "powersave"
	if got := c.effectiveTier(); got != "powersave" {
		t.Fatalf("powersave should remain powersave, got %s", got)
	}
}

func TestThermalConfigNormalize(t *testing.T) {
	c := defaultConfig()
	c.BatteryTempLimit = 99
	normalizeConfig(&c)
	if c.BatteryTempLimit != 50 {
		t.Fatal("thermal max clamp failed")
	}
	c.BatteryTempLimit = 1
	normalizeConfig(&c)
	if c.BatteryTempLimit != 42 {
		t.Fatal("thermal min clamp failed")
	}
}

func TestKnownGameAdditions(t *testing.T) {
	cfg := defaultConfig()
	for _, pkg := range []string{"com.tencent.tmgp.dfm", "com.netease.l22", "com.netease.l22.mi"} {
		if !isGame(pkg, cfg) {
			t.Fatalf("expected built-in game: %s", pkg)
		}
	}
}

func TestDefaultControlMode(t *testing.T) {
	c := defaultConfig()
	if c.ControlMode != "auto" || !c.WatchEnabled || c.WatchIntervalMS != 2000 {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestWatchConfigNormalize(t *testing.T) {
	c := defaultConfig()
	c.ControlMode = "broken"
	c.WatchIntervalMS = 1
	normalizeConfig(&c)
	if c.ControlMode != "auto" || c.WatchIntervalMS != 500 {
		t.Fatalf("watch normalize failed: %+v", c)
	}
	c.WatchIntervalMS = 99999
	normalizeConfig(&c)
	if c.WatchIntervalMS != 10000 {
		t.Fatalf("watch max clamp failed: %+v", c)
	}
}

func TestGameProfiles(t *testing.T) {
	cfg := defaultConfig()
	if got := profilePrimeGate(cfg, "balanced"); got != 0.72 {
		t.Fatalf("balanced gate=%v", got)
	}
	if got := profilePrimeGate(cfg, "extreme"); got != 0.50 {
		t.Fatalf("extreme gate=%v", got)
	}
	cfg.GameProfiles["com.test.game"] = "extreme"
	if got := gameProfileForPackage("com.test.game", cfg); got != "extreme" {
		t.Fatalf("profile override=%s", got)
	}
}

func TestAdaptiveTierSteps(t *testing.T) {
	c := &GameController{tier: "balance", cfg: defaultConfig(), loadLevel: "light"}
	if got := c.adaptiveTier(); got != "powersave" {
		t.Fatalf("light expected powersave, got %s", got)
	}
	c.loadLevel = "heavy"
	if got := c.adaptiveTier(); got != "performance" {
		t.Fatalf("heavy expected performance, got %s", got)
	}
	c.tier = "fast"
	if got := c.adaptiveTier(); got != "fast" {
		t.Fatalf("fast should clamp at fast, got %s", got)
	}
}

func TestLoadLevelTarget(t *testing.T) {
	if got := loadLevelTarget(.30, .25, .35, .25); got != "light" {
		t.Fatalf("expected light, got %s", got)
	}
	if got := loadLevelTarget(.85, .90, .80, .70); got != "heavy" {
		t.Fatalf("expected heavy, got %s", got)
	}
	if got := loadLevelTarget(.60, .55, .65, .40); got != "normal" {
		t.Fatalf("expected normal, got %s", got)
	}
}
