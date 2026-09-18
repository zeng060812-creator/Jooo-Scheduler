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

type RestorePolicy struct {
	ID       int    `json:"id"`
	Governor string `json:"governor"`
	Min      int64  `json:"min_khz"`
	Max      int64  `json:"max_khz"`
}
type RestoreState struct {
	Policies []RestorePolicy   `json:"policies"`
	Affinity map[string]string `json:"affinity"`
}

type DaemonStatus struct {
	Running            bool    `json:"running"`
	PID                int     `json:"pid"`
	Package            string  `json:"package"`
	Tier               string  `json:"tier"`
	StartedAt          int64   `json:"started_at"`
	TopThread          string  `json:"top_thread"`
	TopTID             int     `json:"top_tid"`
	TopUtil            float64 `json:"top_util"`
	Demand             float64 `json:"demand"`
	PrimeOpen          bool    `json:"prime_open"`
	Burst              bool    `json:"burst"`
	EffectiveTier      string  `json:"effective_tier"`
	AdaptiveTier       string  `json:"adaptive_tier"`
	GameProfile        string  `json:"game_profile"`
	LoadLevel          string  `json:"load_level"`
	EffectivePrimeGate float64 `json:"effective_prime_gate"`
	ThermalLimited     bool    `json:"thermal_limited"`
	BatteryTempC       float64 `json:"battery_temp_c"`
	ThermalLimitC      float64 `json:"thermal_limit_c"`
}

type GameController struct {
	root               string
	pkg                string
	tier               string
	cfg                Config
	policies           []Policy
	restore            RestoreState
	taskset            string
	status             DaemonStatus
	lastConfigMtime    time.Time
	burstUntil         time.Time
	lastAffinity       time.Time
	lastThermalCheck   time.Time
	thermalLimited     bool
	batteryTemp        float64
	loadLevel          string
	loadCandidate      string
	loadCandidateSince time.Time
}

func normalizeTier(s string) string {
	switch s {
	case "powersave", "balance", "performance", "fast":
		return s
	case "pedestal":
		return "fast"
	default:
		return "balance"
	}
}

func tierFloor(tier, role string) float64 {
	switch normalizeTier(tier) {
	case "powersave":
		switch role {
		case roleEfficiency:
			return 0.00
		case rolePrimary:
			return 0.10
		case roleSecondary:
			return 0.06
		case rolePrime:
			return 0.00
		}
	case "performance":
		switch role {
		case roleEfficiency:
			return 0.06
		case rolePrimary:
			return 0.25
		case roleSecondary:
			return 0.18
		case rolePrime:
			return 0.06
		}
	case "fast":
		switch role {
		case roleEfficiency:
			return 0.10
		case rolePrimary:
			return 0.32
		case roleSecondary:
			return 0.25
		case rolePrime:
			return 0.12
		}
	default:
		switch role {
		case roleEfficiency:
			return 0.03
		case rolePrimary:
			return 0.17
		case roleSecondary:
			return 0.11
		case rolePrime:
			return 0.00
		}
	}
	return 0
}

func newController(root, pkg, tier string) (*GameController, error) {
	cfg, err := loadConfig(root)
	if err != nil {
		return nil, err
	}
	c := &GameController{root: root, pkg: pkg, tier: normalizeTier(tier), cfg: cfg, policies: discoverPolicies()}
	c.restore = RestoreState{Affinity: map[string]string{}}
	c.taskset = findTaskset()
	c.loadLevel = "normal"
	profile := gameProfileForPackage(pkg, cfg)
	c.status = DaemonStatus{Running: true, PID: os.Getpid(), Package: pkg, Tier: c.tier, AdaptiveTier: c.tier, EffectiveTier: c.tier, GameProfile: profile, LoadLevel: c.loadLevel, EffectivePrimeGate: profilePrimeGate(cfg, profile), StartedAt: time.Now().Unix(), ThermalLimitC: cfg.BatteryTempLimit}
	if len(c.policies) < 3 || len(policyByRole(c.policies, roleEfficiency)) == 0 || len(policyByRole(c.policies, rolePrime)) == 0 || len(policyByRole(c.policies, rolePrimary)) == 0 {
		return nil, fmt.Errorf("SM8650动态拓扑识别失败，检测到%d个policy", len(c.policies))
	}
	return c, nil
}

func findTaskset() string {
	for _, p := range []string{"/system/bin/taskset", "/data/adb/ksu/bin/busybox", "/data/adb/magisk/busybox"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func (c *GameController) snapshot() error {
	for _, p := range c.policies {
		c.restore.Policies = append(c.restore.Policies, RestorePolicy{ID: p.ID, Governor: p.Governor, Min: p.Min, Max: p.Max})
	}
	return c.saveRestore()
}

func (c *GameController) saveRestore() error {
	b, _ := json.MarshalIndent(c.restore, "", "  ")
	return atomicWrite(filepath.Join(c.root, "state", "restore.json"), append(b, '\n'), 0644)
}

func restoreFromFile(root string) error {
	path := filepath.Join(root, "state", "restore.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var r RestoreState
	if json.Unmarshal(b, &r) != nil {
		return nil
	}
	for _, p := range r.Policies {
		base := policyBase(p.ID)
		// 放宽上限 -> 恢复下限 -> 恢复上限，避免min>max。
		hw := readInt64(filepath.Join(base, "cpuinfo_max_freq"))
		if hw <= 0 {
			hw = p.Max
		}
		if hw > 0 {
			_ = writeString(filepath.Join(base, "scaling_max_freq"), strconv.FormatInt(hw, 10))
		}
		if p.Min > 0 {
			_ = writeString(filepath.Join(base, "scaling_min_freq"), strconv.FormatInt(p.Min, 10))
		}
		if p.Max > 0 {
			_ = writeString(filepath.Join(base, "scaling_max_freq"), strconv.FormatInt(p.Max, 10))
		}
		if p.Governor != "" {
			_ = writeString(filepath.Join(base, "scaling_governor"), p.Governor)
		}
	}
	ts := findTaskset()
	if ts != "" {
		for tid, mask := range r.Affinity {
			if n, _ := strconv.Atoi(tid); n > 0 && pidAlive(n) {
				_ = tasksetApply(ts, n, mask)
			}
		}
	}
	_ = os.Remove(path)
	return nil
}

func tasksetApply(bin string, tid int, mask string) error {
	var cmd *exec.Cmd
	if strings.HasSuffix(bin, "busybox") {
		cmd = exec.Command(bin, "taskset", "-p", mask, strconv.Itoa(tid))
	} else {
		cmd = exec.Command(bin, "-p", mask, strconv.Itoa(tid))
	}
	return cmd.Run()
}

func readAffinityMask(tid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", tid))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "Cpus_allowed:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Cpus_allowed:"))
		}
	}
	return ""
}

func (c *GameController) setAffinity(tid int, mask string) bool {
	if !c.cfg.ThreadPlacement || c.taskset == "" || !pidAlive(tid) {
		return false
	}
	key := strconv.Itoa(tid)
	added := false
	if _, ok := c.restore.Affinity[key]; !ok {
		if cur := readAffinityMask(tid); cur != "" {
			c.restore.Affinity[key] = cur
			added = true
		}
	}
	_ = tasksetApply(c.taskset, tid, mask)
	return added
}

func (c *GameController) restoreAffinitiesOnly() {
	if c.taskset == "" {
		c.restore.Affinity = map[string]string{}
		_ = c.saveRestore()
		return
	}
	for tid, mask := range c.restore.Affinity {
		if n, _ := strconv.Atoi(tid); n > 0 && pidAlive(n) {
			_ = tasksetApply(c.taskset, n, mask)
		}
	}
	c.restore.Affinity = map[string]string{}
	_ = c.saveRestore()
}

func isMainThread(name string, tid, pid int) bool {
	if tid == pid {
		return true
	}
	n := strings.ToLower(name)
	for _, k := range []string{"unitymain", "uegamethread", "gamethread", "mainthread"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}
func isRenderThread(name string) bool {
	n := strings.ToLower(name)
	for _, k := range []string{"renderthread", "rhithread", "unitygfx", "corethread", "choreo"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}
func isWorkerThread(name string) bool {
	n := strings.ToLower(name)
	for _, k := range []string{"worker", "taskgraph", "nativethread", "asyncload", "fmod", "audiotrack"} {
		if strings.Contains(n, k) {
			return true
		}
	}
	return false
}

func (c *GameController) placeThreads(pid int, loads []ThreadLoad) {
	if !c.cfg.ThreadPlacement {
		return
	}
	// 动态拓扑：主线程优先所有非能效、非Prime性能簇；高需求时再放开Prime。
	perfCPUs := roleCPUs(c.policies, rolePrimary, roleSecondary)
	if len(perfCPUs) == 0 {
		perfCPUs = roleCPUs(c.policies, rolePrimary, rolePrime)
	}
	mainCPUs := append([]int(nil), perfCPUs...)
	effectiveTier := c.effectiveTier()
	if !c.thermalLimited && (c.status.PrimeOpen || effectiveTier == "performance" || effectiveTier == "fast") {
		mainCPUs = unionCPUs(mainCPUs, roleCPUs(c.policies, rolePrime))
	}
	mainMask := cpusToMask(mainCPUs)
	renderMask := cpusToMask(perfCPUs)
	if mainMask == "" {
		mainMask = cpusToMask(totalPolicyCPUs(c.policies))
	}
	if renderMask == "" {
		renderMask = mainMask
	}
	changed := false
	for i, t := range loads {
		if i > 40 {
			break
		}
		switch {
		case isMainThread(t.Name, t.TID, pid):
			changed = c.setAffinity(t.TID, mainMask) || changed
		case isRenderThread(t.Name):
			changed = c.setAffinity(t.TID, renderMask) || changed
		case isWorkerThread(t.Name):
			changed = c.setAffinity(t.TID, renderMask) || changed
		}
	}
	if changed {
		_ = c.saveRestore()
	}
}

func (c *GameController) reloadConfigIfChanged() bool {
	st, err := os.Stat(configPath(c.root))
	if err != nil {
		return false
	}
	if !st.ModTime().After(c.lastConfigMtime) {
		return false
	}
	cfg, err := loadConfig(c.root)
	if err != nil {
		return false
	}
	wasPlacement := c.cfg.ThreadPlacement
	oldSample := c.cfg.SampleMS
	c.cfg = cfg
	c.lastConfigMtime = st.ModTime()
	mode, _, _, _ := readSceneState(c.root)
	c.tier = chooseTier(c.cfg, mode)
	c.status.Tier = c.tier
	c.status.GameProfile = gameProfileForPackage(c.pkg, c.cfg)
	c.status.ThermalLimitC = c.cfg.BatteryTempLimit
	if wasPlacement && !cfg.ThreadPlacement {
		c.restoreAffinitiesOnly()
	}
	return oldSample != c.cfg.SampleMS
}

func (c *GameController) updateThermalGuard() {
	if time.Since(c.lastThermalCheck) < 1500*time.Millisecond {
		return
	}
	c.lastThermalCheck = time.Now()
	c.batteryTemp = batteryTempC()
	c.status.BatteryTempC = floatRound(c.batteryTemp, 1)
	c.status.ThermalLimitC = c.cfg.BatteryTempLimit
	if !c.cfg.ThermalGuard || c.batteryTemp <= 0 {
		if c.thermalLimited {
			logLine(c.root, 0, c.cfg, "ZenJooo温度保护已关闭，恢复Scene请求档位")
		}
		c.thermalLimited = false
		c.status.ThermalLimited = false
		return
	}
	if !c.thermalLimited && c.batteryTemp >= c.cfg.BatteryTempLimit {
		c.thermalLimited = true
		logLine(c.root, 0, c.cfg, "电池温度%.1f°C达到保护阈值%.1f°C，游戏有效档位降至均衡", c.batteryTemp, c.cfg.BatteryTempLimit)
	} else if c.thermalLimited && c.batteryTemp <= c.cfg.BatteryTempLimit-2.0 {
		c.thermalLimited = false
		logLine(c.root, 0, c.cfg, "电池温度%.1f°C已回落，恢复原游戏档位%s", c.batteryTemp, c.tier)
	}
	c.status.ThermalLimited = c.thermalLimited
}

func tierStep(tier string, delta int) string {
	order := []string{"powersave", "balance", "performance", "fast"}
	tier = normalizeTier(tier)
	idx := 1
	for i, v := range order {
		if v == tier {
			idx = i
			break
		}
	}
	idx += delta
	if idx < 0 {
		idx = 0
	}
	if idx >= len(order) {
		idx = len(order) - 1
	}
	return order[idx]
}

func profilePrimeGate(cfg Config, profile string) float64 {
	gate := cfg.PrimeGate
	if normalizeGameProfile(profile) == "extreme" {
		gate -= 0.22
	}
	return clamp(gate, 0.50, 0.95)
}

func profileFloorBoost(profile, role string) float64 {
	if normalizeGameProfile(profile) != "extreme" {
		return 0
	}
	switch role {
	case rolePrimary:
		return 0.04
	case roleSecondary:
		return 0.05
	case rolePrime:
		return 0.03
	default:
		return 0
	}
}

func loadLevelTarget(demand, top, primary, secondary float64) string {
	if top >= 0.88 || demand >= 0.84 || primary >= 0.90 || secondary >= 0.90 {
		return "heavy"
	}
	if demand <= 0.38 && top <= 0.42 && primary <= 0.46 && secondary <= 0.42 {
		return "light"
	}
	return "normal"
}

func (c *GameController) updateLoadLevel(demand, top, primary, secondary float64) {
	if !c.cfg.AdaptiveGameLoad {
		if c.loadLevel != "normal" {
			c.loadLevel = "normal"
			logLine(c.root, 1, c.cfg, "游戏负载自适应已关闭，恢复基础档位")
		}
		c.loadCandidate = ""
		c.status.LoadLevel = c.loadLevel
		return
	}
	target := loadLevelTarget(demand, top, primary, secondary)
	if target == c.loadLevel {
		c.loadCandidate = ""
		c.status.LoadLevel = c.loadLevel
		return
	}
	now := time.Now()
	if c.loadCandidate != target {
		c.loadCandidate = target
		c.loadCandidateSince = now
		return
	}
	wait := 800 * time.Millisecond
	switch target {
	case "heavy":
		wait = 300 * time.Millisecond
	case "light":
		wait = 1800 * time.Millisecond
	}
	if now.Sub(c.loadCandidateSince) < wait {
		return
	}
	old := c.loadLevel
	c.loadLevel = target
	c.loadCandidate = ""
	c.status.LoadLevel = c.loadLevel
	logLine(c.root, 1, c.cfg, "游戏负载级别 %s -> %s (demand=%.2f top=%.2f)", old, c.loadLevel, demand, top)
}

func (c *GameController) adaptiveTier() string {
	base := normalizeTier(c.tier)
	if !c.cfg.AdaptiveGameLoad {
		return base
	}
	switch c.loadLevel {
	case "light":
		return tierStep(base, -1)
	case "heavy":
		return tierStep(base, 1)
	default:
		return base
	}
}

func (c *GameController) effectiveTier() string {
	t := c.adaptiveTier()
	c.status.AdaptiveTier = t
	if c.thermalLimited && (t == "performance" || t == "fast") {
		return "balance"
	}
	return t
}

func (c *GameController) computeRatios(utils map[int]float64, top float64) map[int]float64 {
	primary := roleMaxUtil(c.policies, utils, rolePrimary)
	secondary := roleMaxUtil(c.policies, utils, roleSecondary)
	primeUtil := roleMaxUtil(c.policies, utils, rolePrime)
	// 负载自适应只关注游戏最重线程和性能簇，不让能效簇上的后台任务误触发“重载”。
	demand := maxf(top, maxf(primary, maxf(secondary, primeUtil)))
	c.updateLoadLevel(demand, top, primary, secondary)
	profile := gameProfileForPackage(c.pkg, c.cfg)
	c.status.GameProfile = profile
	effectiveTier := c.effectiveTier()
	c.status.EffectiveTier = effectiveTier
	if top >= 0.92 || (primary >= 0.90 && (secondary >= 0.75 || primeUtil >= 0.75)) {
		c.burstUntil = time.Now().Add(420 * time.Millisecond)
	}
	burst := time.Now().Before(c.burstUntil)
	r := map[int]float64{}
	primeOpen := false
	for _, p := range c.policies {
		base := clamp(tierFloor(effectiveTier, p.Role)+profileFloorBoost(profile, p.Role), 0, 0.95)
		switch p.Role {
		case roleEfficiency:
			r[p.ID] = clamp(base+demand*0.06, 0, 0.28)
		case rolePrimary:
			r[p.ID] = clamp(base+demand*0.48, base, 0.92)
		case roleSecondary:
			if demand > 0.42 {
				r[p.ID] = clamp(base+(demand-0.42)*0.62, base, 0.86)
			} else {
				r[p.ID] = base
			}
		case rolePrime:
			gate := profilePrimeGate(c.cfg, profile)
			if c.loadLevel == "light" {
				gate += 0.10
			} else if c.loadLevel == "heavy" {
				gate -= 0.06
			}
			gate = clamp(gate, 0.50, 0.95)
			if c.thermalLimited && gate < 0.88 {
				gate = 0.88
			}
			c.status.EffectivePrimeGate = floatRound(gate, 3)
			if top > gate || primary > 0.88 || secondary > 0.90 {
				x := maxf(top, maxf(primary, secondary))
				r[p.ID] = clamp(base+(x-gate)/(1-gate)*0.58, base, 0.78)
			} else {
				r[p.ID] = base
			}
			primeOpen = primeOpen || r[p.ID] > base+0.02
		default:
			r[p.ID] = base
		}
	}
	if c.thermalLimited {
		burst = false
	}
	if burst {
		for _, p := range c.policies {
			switch p.Role {
			case rolePrimary:
				r[p.ID] = maxf(r[p.ID], 0.72)
			case roleSecondary:
				r[p.ID] = maxf(r[p.ID], 0.60)
			case rolePrime:
				if normalizeTier(c.tier) != "powersave" {
					r[p.ID] = maxf(r[p.ID], 0.40)
					primeOpen = true
				}
			}
		}
	}
	if c.loadLevel == "light" {
		for _, p := range c.policies {
			switch p.Role {
			case rolePrimary, roleSecondary:
				r[p.ID] *= 0.88
			case rolePrime:
				r[p.ID] = tierFloor(effectiveTier, rolePrime)
			}
		}
	}
	if c.loadLevel == "heavy" {
		for _, p := range c.policies {
			switch p.Role {
			case rolePrimary:
				r[p.ID] = clamp(r[p.ID]+0.06, 0, 0.96)
			case roleSecondary:
				r[p.ID] = clamp(r[p.ID]+0.07, 0, 0.92)
			}
		}
	}
	if profile == "extreme" {
		for _, p := range c.policies {
			switch p.Role {
			case rolePrimary:
				r[p.ID] = clamp(r[p.ID]*1.05, 0, 0.97)
			case roleSecondary:
				r[p.ID] = clamp(r[p.ID]*1.08, 0, 0.94)
			}
		}
	}
	if effectiveTier == "fast" {
		for _, p := range c.policies {
			switch p.Role {
			case rolePrimary:
				r[p.ID] = clamp(r[p.ID]*1.08, 0, 1)
			case roleSecondary:
				r[p.ID] = clamp(r[p.ID]*1.10, 0, 0.95)
			case rolePrime:
				r[p.ID] = clamp(r[p.ID]*1.15, 0, 0.90)
			}
		}
	}
	if c.thermalLimited {
		for _, p := range c.policies {
			switch p.Role {
			case rolePrimary:
				r[p.ID] = clamp(r[p.ID], 0, 0.72)
			case roleSecondary:
				r[p.ID] = clamp(r[p.ID], 0, 0.58)
			case rolePrime:
				r[p.ID] = clamp(r[p.ID], 0, 0.22)
			}
		}
	}
	c.status.Demand = floatRound(demand, 3)
	c.status.LoadLevel = c.loadLevel
	c.status.PrimeOpen = primeOpen
	c.status.Burst = burst
	return r
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
func maxf(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (c *GameController) applyRatios(r map[int]float64) {
	for i := range c.policies {
		p := &c.policies[i]
		target := nearestRatio(*p, r[p.ID])
		if target <= 0 {
			continue
		}
		// 只抬最低频率，不锁死min=max，让原厂WALT/UAG继续做瞬态决策。
		if target < p.Min {
			target = p.Min
		}
		if target > p.Max && p.Max > 0 {
			target = p.Max
		}
		curMin := readInt64(filepath.Join(policyBase(p.ID), "scaling_min_freq"))
		if curMin != target {
			_ = writeString(filepath.Join(policyBase(p.ID), "scaling_min_freq"), strconv.FormatInt(target, 10))
		}
	}
}

func (c *GameController) writeStatus() {
	b, _ := json.Marshal(c.status)
	_ = atomicWrite(filepath.Join(c.root, "state", "daemon_status.json"), append(b, '\n'), 0644)
}

func (c *GameController) run() error {
	if err := c.snapshot(); err != nil {
		return err
	}
	defer func() {
		_ = restoreFromFile(c.root)
		_ = os.Remove(filepath.Join(c.root, "state", "daemon.pid"))
		_ = os.Remove(filepath.Join(c.root, "state", "daemon_status.json"))
	}()
	_ = atomicWrite(filepath.Join(c.root, "state", "daemon.pid"), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	prevCPU := readCPUStat()
	time.Sleep(120 * time.Millisecond)
	prevThreads := map[int]ThreadSample{}
	missingSince := time.Time{}
	ticker := time.NewTicker(time.Duration(c.cfg.SampleMS) * time.Millisecond)
	defer ticker.Stop()
	logLine(c.root, 0, c.cfg, "游戏控制器启动 pkg=%s tier=%s", c.pkg, c.tier)
	for {
		select {
		case <-sig:
			logLine(c.root, 0, c.cfg, "收到停止信号，恢复原厂状态")
			return nil
		case <-ticker.C:
			if c.reloadConfigIfChanged() {
				ticker.Reset(time.Duration(c.cfg.SampleMS) * time.Millisecond)
			}
			if !c.cfg.GameEngine {
				return nil
			}
			pid := findPackagePID(c.pkg)
			if pid <= 0 {
				if missingSince.IsZero() {
					missingSince = time.Now()
				}
				if time.Since(missingSince) > 4*time.Second {
					return nil
				}
				continue
			} else {
				missingSince = time.Time{}
			}
			curCPU := readCPUStat()
			utils := cpuUtils(prevCPU, curCPU)
			avg := avgCPUJiffies(prevCPU, curCPU)
			prevCPU = curCPU
			curThreads := readThreads(pid)
			loads := threadLoads(prevThreads, curThreads, avg)
			prevThreads = curThreads
			top := 0.0
			if len(loads) > 0 {
				top = loads[0].Util
				c.status.TopThread = loads[0].Name
				c.status.TopTID = loads[0].TID
				c.status.TopUtil = floatRound(top, 3)
			}
			c.updateThermalGuard()
			ratios := c.computeRatios(utils, top)
			c.applyRatios(ratios)
			if time.Since(c.lastAffinity) > 900*time.Millisecond {
				c.placeThreads(pid, loads)
				c.lastAffinity = time.Now()
			}
			c.writeStatus()
		}
	}
}
