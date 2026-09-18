package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	moduleID = "yumi_jooo"
	version  = "1.5.0"
)

var packageRE = regexp.MustCompile(`^[A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)+$`)

func validPackage(s string) bool { return packageRE.MatchString(s) }

func moduleRoot() string {
	if r := os.Getenv("JOOO_ROOT"); r != "" {
		return r
	}
	exe, err := os.Executable()
	if err == nil {
		p := filepath.Clean(exe)
		// .../yumi_jooo/bin/jood
		if filepath.Base(filepath.Dir(p)) == "bin" {
			return filepath.Dir(filepath.Dir(p))
		}
	}
	return "/data/adb/modules/" + moduleID
}

func dataRoot() string {
	if r := os.Getenv("JOOO_DATA"); r != "" {
		return r
	}
	return "/data/adb/" + moduleID
}

func logDir() string { return filepath.Join(dataRoot(), "logs") }

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	tmp := path + fmt.Sprintf(".tmp.%d", os.Getpid())
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	_ = os.Chmod(tmp, mode)
	return os.Rename(tmp, path)
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func readInt64(path string) int64 {
	v, _ := strconv.ParseInt(readTrim(path), 10, 64)
	return v
}

func writeString(path, value string) error { return os.WriteFile(path, []byte(value), 0644) }

func getprop(name string) string {
	b, err := exec.Command("/system/bin/getprop", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func supportedHardware() (bool, string) {
	socs := strings.ToLower(strings.Join([]string{
		getprop("ro.soc.model"),
		getprop("ro.vendor.soc.model"),
		getprop("ro.product.soc.model"),
	}, " "))
	socOK := strings.Contains(socs, "sm8650")
	ps := discoverPolicies()
	cpus := totalPolicyCPUs(ps)
	topoOK := len(ps) >= 3 && len(cpus) >= 6
	detail := fmt.Sprintf("device=%s model=%s soc=%s policies=%d cpus=%s", getprop("ro.product.device"), getprop("ro.product.model"), strings.TrimSpace(socs), len(ps), compressCPUList(cpus))
	return socOK && topoOK, detail
}

func logLine(root string, level int, cfg Config, format string, args ...any) {
	if level > cfg.LogLevel {
		return
	}
	dir := logDir()
	_ = os.MkdirAll(dir, 0755)
	path := filepath.Join(dir, "jood.log")
	if st, err := os.Stat(path); err == nil && st.Size() > 256*1024 {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), fmt.Sprintf(format, args...))
}

type Policy struct {
	ID        int     `json:"id"`
	CPUs      string  `json:"cpus"`
	Role      string  `json:"role"`
	Capacity  int64   `json:"capacity"`
	HWMax     int64   `json:"hw_max_khz"`
	Cur       int64   `json:"cur_khz"`
	Min       int64   `json:"min_khz"`
	Max       int64   `json:"max_khz"`
	Governor  string  `json:"governor"`
	Available []int64 `json:"-"`
	CPUList   []int   `json:"-"`
}

func discoverPolicies() []Policy {
	paths, _ := filepath.Glob("/sys/devices/system/cpu/cpufreq/policy*")
	out := make([]Policy, 0, len(paths))
	for _, base := range paths {
		name := filepath.Base(base)
		id, err := strconv.Atoi(strings.TrimPrefix(name, "policy"))
		if err != nil {
			continue
		}
		raw := readTrim(filepath.Join(base, "related_cpus"))
		if raw == "" {
			raw = readTrim(filepath.Join(base, "affected_cpus"))
		}
		cpus := parseCPUList(raw)
		if len(cpus) == 0 {
			continue
		}
		p := Policy{ID: id, CPUList: cpus, CPUs: compressCPUList(cpus)}
		p.Cur = readInt64(filepath.Join(base, "scaling_cur_freq"))
		p.Min = readInt64(filepath.Join(base, "scaling_min_freq"))
		p.Max = readInt64(filepath.Join(base, "scaling_max_freq"))
		p.HWMax = readInt64(filepath.Join(base, "cpuinfo_max_freq"))
		p.Governor = readTrim(filepath.Join(base, "scaling_governor"))
		if len(cpus) > 0 {
			p.Capacity = readInt64(fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpu_capacity", cpus[0]))
		}
		for _, f := range strings.Fields(readTrim(filepath.Join(base, "scaling_available_frequencies"))) {
			if n, err := strconv.ParseInt(f, 10, 64); err == nil {
				p.Available = append(p.Available, n)
			}
		}
		for _, f := range strings.Fields(readTrim(filepath.Join(base, "scaling_boost_frequencies"))) {
			if n, err := strconv.ParseInt(f, 10, 64); err == nil {
				p.Available = append(p.Available, n)
			}
		}
		if len(p.Available) == 0 {
			lo := readInt64(filepath.Join(base, "cpuinfo_min_freq"))
			hi := p.HWMax
			if lo > 0 {
				p.Available = append(p.Available, lo)
			}
			if hi > 0 && hi != lo {
				p.Available = append(p.Available, hi)
			}
		}
		sort.Slice(p.Available, func(i, j int) bool { return p.Available[i] < p.Available[j] })
		p.Available = dedupe64(p.Available)
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return assignPolicyRoles(out)
}

func dedupe64(in []int64) []int64 {
	if len(in) == 0 {
		return in
	}
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}

func policyBase(id int) string { return fmt.Sprintf("/sys/devices/system/cpu/cpufreq/policy%d", id) }

func nearestRatio(p Policy, ratio float64) int64 {
	if len(p.Available) == 0 {
		if p.Max > 0 {
			return int64(float64(p.Min) + (float64(p.Max-p.Min) * ratio))
		}
		return p.Min
	}
	if ratio <= 0 {
		return p.Available[0]
	}
	if ratio >= 1 {
		return p.Available[len(p.Available)-1]
	}
	lo, hi := p.Available[0], p.Available[len(p.Available)-1]
	target := float64(lo) + float64(hi-lo)*ratio
	for _, f := range p.Available {
		if float64(f) >= target {
			return f
		}
	}
	return hi
}

func batteryPower() (w float64, currentMA float64, voltageV float64) {
	b := "/sys/class/power_supply/battery"
	cur := float64(readInt64(filepath.Join(b, "current_now")))
	vol := float64(readInt64(filepath.Join(b, "voltage_now")))
	if cur == 0 || vol == 0 {
		return 0, 0, 0
	}
	if cur < 0 {
		cur = -cur
	}
	if vol < 0 {
		vol = -vol
	}
	currentMA = cur / 1e3
	voltageV = vol / 1e6
	w = currentMA / 1e3 * voltageV
	return
}

func maxCPUTempC() float64 {
	max := 0.0
	entries, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp")
	for _, p := range entries {
		v := float64(readInt64(p))
		if v > 1000 {
			v /= 1000
		}
		if v > max && v < 150 {
			max = v
		}
	}
	return max
}

func batteryTempC() float64 {
	raw := float64(readInt64("/sys/class/power_supply/battery/temp"))
	if raw > 0 {
		switch {
		case raw > 10000:
			raw /= 1000
		case raw > 100:
			raw /= 10
		}
		if raw > 0 && raw < 100 {
			return raw
		}
	}
	zones, _ := filepath.Glob("/sys/class/thermal/thermal_zone*")
	for _, z := range zones {
		t := strings.ToLower(readTrim(filepath.Join(z, "type")))
		if !strings.Contains(t, "battery") && !strings.Contains(t, "batt") {
			continue
		}
		v := float64(readInt64(filepath.Join(z, "temp")))
		if v > 1000 {
			v /= 1000
		}
		if v > 0 && v < 100 {
			return v
		}
	}
	return 0
}

func capturedForeground(root string) string {
	pkg := strings.TrimSpace(readTrim(filepath.Join(root, "state", "captured_foreground.txt")))
	if validPackage(pkg) {
		return pkg
	}
	return ""
}

func sceneInstalled() bool {
	cmd := exec.Command("/system/bin/pm", "path", "com.omarea.vtools")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func foregroundPackage() string {
	cmds := [][]string{{"activity", "activities"}, {"window", "windows"}}
	for _, args := range cmds {
		b, err := exec.Command("/system/bin/dumpsys", args...).Output()
		if err != nil {
			continue
		}
		s := string(b)
		keys := []string{"topResumedActivity=", "mResumedActivity:", "mCurrentFocus="}
		for _, k := range keys {
			idx := strings.Index(s, k)
			if idx < 0 {
				continue
			}
			tail := s[idx+len(k):]
			line := strings.SplitN(tail, "\n", 2)[0]
			fields := strings.Fields(line)
			for _, f := range fields {
				f = strings.Trim(f, "{}[](),")
				if slash := strings.IndexByte(f, '/'); slash > 0 {
					pkg := f[:slash]
					if validPackage(pkg) {
						return pkg
					}
				}
			}
		}
	}
	return ""
}

var knownGames = map[string]string{
	"com.tencent.tmgp.dfm":               "三角洲行动",
	"com.netease.l22":                    "永劫无间手游",
	"com.netease.l22.mi":                 "永劫无间手游（小米）",
	"com.tencent.tmgp.sgame":             "王者荣耀",
	"com.tencent.tmgp.sgamece":           "王者荣耀体验服",
	"com.tencent.tmgp.pubgmhd":           "和平精英",
	"com.tencent.tmgp.pubgmhdce":         "和平精英体验服",
	"com.tencent.lolm":                   "英雄联盟手游",
	"com.tencent.tmgp.speedmobile":       "QQ飞车手游",
	"com.garena.game.fctw":               "QQ飞车国际服",
	"com.miHoYo.GenshinImpact":           "原神国际服",
	"com.miHoYo.Yuanshen":                "原神",
	"com.miHoYo.hkrpg":                   "崩坏：星穹铁道",
	"com.kurogame.mingchao":              "鸣潮",
	"com.kurogame.wutheringwaves.global": "鸣潮国际服",
	"com.pubg.imobile":                   "PUBG Mobile",
	"com.tencent.ig":                     "PUBG Mobile Global",
	"com.activision.callofduty.shooter":  "使命召唤手游",
}

func isGame(pkg string, cfg Config) bool {
	if _, ok := knownGames[pkg]; ok {
		return true
	}
	for _, p := range cfg.CustomGames {
		if p == pkg {
			return true
		}
	}
	return false
}

func installedGames(cfg Config) []map[string]any {
	b, _ := exec.Command("/system/bin/pm", "list", "packages").Output()
	installed := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	for scanner.Scan() {
		s := strings.TrimPrefix(strings.TrimSpace(scanner.Text()), "package:")
		if validPackage(s) {
			installed[s] = true
		}
	}
	all := map[string]string{}
	for p, n := range knownGames {
		all[p] = n
	}
	for _, p := range cfg.CustomGames {
		if _, ok := all[p]; !ok {
			all[p] = "自定义游戏"
		}
	}
	keys := make([]string, 0, len(all))
	for p := range all {
		keys = append(keys, p)
	}
	sort.Strings(keys)
	out := []map[string]any{}
	for _, p := range keys {
		if installed[p] {
			out = append(out, map[string]any{
				"package":             p,
				"name":                all[p],
				"custom":              contains(cfg.CustomGames, p),
				"profile":             gameProfileForPackage(p, cfg),
				"recommended_profile": recommendedGameProfile(p),
			})
		}
	}
	return out
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

type HistoryEntry struct {
	Time     string `json:"time"`
	Mode     string `json:"mode"`
	Category string `json:"category"`
	Package  string `json:"package"`
}

func appendHistory(root, mode, category, pkg string) {
	path := filepath.Join(root, "state", "history.log")
	line := fmt.Sprintf("%s\t%s\t%s\t%s\n", time.Now().Format("01-02 15:04:05"), sanitizeField(mode), sanitizeField(category), sanitizeField(pkg))
	old, _ := os.ReadFile(path)
	all := string(old) + line
	lines := strings.Split(strings.TrimSpace(all), "\n")
	if len(lines) > 64 {
		lines = lines[len(lines)-64:]
	}
	_ = atomicWrite(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}

func readHistory(root string, n int) []HistoryEntry {
	b, err := os.ReadFile(filepath.Join(root, "state", "history.log"))
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	out := make([]HistoryEntry, 0, len(lines))
	for i := len(lines) - 1; i >= 0; i-- {
		p := strings.Split(lines[i], "\t")
		if len(p) < 4 {
			continue
		}
		out = append(out, HistoryEntry{Time: p[0], Mode: p[1], Category: p[2], Package: p[3]})
	}
	return out
}

func readSceneState(root string) (mode, category, scene, pkg string) {
	b, err := os.ReadFile(filepath.Join(root, "state", "scene_state.tsv"))
	if err != nil {
		return "balance", "app", "", ""
	}
	parts := strings.Split(strings.TrimSpace(string(b)), "\t")
	if len(parts) > 0 {
		mode = parts[0]
	}
	if len(parts) > 1 {
		category = parts[1]
	}
	if len(parts) > 2 {
		scene = parts[2]
	}
	if mode == "" {
		mode = "balance"
	}
	if category == "" {
		category = "app"
	}
	pkg = strings.SplitN(scene, "/", 2)[0]
	return
}

func writeSceneState(root, mode, category, scene string) error {
	_ = os.MkdirAll(filepath.Join(root, "state"), 0755)
	if err := atomicWrite(filepath.Join(root, "state", "scene_mode.txt"), []byte(mode+"\n"), 0644); err != nil {
		return err
	}
	line := fmt.Sprintf("%s\t%s\t%s\n", sanitizeField(mode), sanitizeField(category), sanitizeField(scene))
	return atomicWrite(filepath.Join(root, "state", "scene_state.tsv"), []byte(line), 0644)
}

func sanitizeField(s string) string {
	return strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(strings.TrimSpace(s))
}

func pidAlive(pid int) bool {
	_, err := os.Stat(fmt.Sprintf("/proc/%d", pid))
	return pid > 1 && err == nil
}

func readPID(path string) int { n, _ := strconv.Atoi(readTrim(path)); return n }

func floatRound(v float64, n int) float64 { p := math.Pow10(n); return math.Round(v*p) / p }

func jsonPrint(v any, pretty bool) {
	var b []byte
	if pretty {
		b, _ = json.MarshalIndent(v, "", "  ")
	} else {
		b, _ = json.Marshal(v)
	}
	fmt.Println(string(b))
}
