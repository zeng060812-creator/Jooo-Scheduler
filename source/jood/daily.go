package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DailyRestore struct {
	CPUSetPath string `json:"cpuset_path"`
	CPUSetOrig string `json:"cpuset_orig"`
	ZramAlg    string `json:"zram_algorithm"`
}

type DailyStatus struct {
	Enabled        bool   `json:"enabled"`
	Applied        bool   `json:"applied"`
	EfficiencyCPUs string `json:"efficiency_cpus"`
	CPUSetPath     string `json:"cpuset_path"`
	ZramStatus     string `json:"zram_status"`
}

func dailyRestorePath(root string) string { return filepath.Join(root, "state", "daily_restore.json") }

func findBackgroundCPUSet() string {
	for _, p := range []string{
		"/dev/cpuset/background/cpus",
		"/sys/fs/cgroup/background/cpuset.cpus",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func efficiencyCPUList() string {
	ps := discoverPolicies()
	return compressCPUList(roleCPUs(ps, roleEfficiency))
}

func loadDailyRestore(root string) DailyRestore {
	var r DailyRestore
	if b, err := os.ReadFile(dailyRestorePath(root)); err == nil {
		_ = json.Unmarshal(b, &r)
	}
	return r
}

func saveDailyRestore(root string, r DailyRestore) error {
	b, _ := json.MarshalIndent(r, "", "  ")
	return atomicWrite(dailyRestorePath(root), append(b, '\n'), 0644)
}

func zramCurrentAlgorithm() string {
	s := readTrim("/sys/block/zram0/comp_algorithm")
	for _, f := range strings.Fields(s) {
		if strings.HasPrefix(f, "[") && strings.HasSuffix(f, "]") {
			return strings.Trim(f, "[]")
		}
	}
	return ""
}

func zramSupports(name string) bool {
	s := readTrim("/sys/block/zram0/comp_algorithm")
	return strings.Contains(" "+strings.ReplaceAll(strings.ReplaceAll(s, "[", ""), "]", "")+" ", " "+name+" ")
}

func applyDailyOptimization(root string, cfg Config) error {
	if !cfg.DailyOptimize {
		return restoreDailyOptimization(root)
	}
	r := loadDailyRestore(root)
	changed := false

	if cfg.DailyBackgroundLimit {
		p := findBackgroundCPUSet()
		eff := efficiencyCPUList()
		if p != "" && eff != "" {
			if r.CPUSetPath == "" {
				r.CPUSetPath = p
				r.CPUSetOrig = readTrim(p)
				changed = true
			}
			if cur := readTrim(p); cur != eff {
				if err := writeString(p, eff); err != nil {
					logLine(root, 1, cfg, "日常优化：background cpuset写入失败 path=%s err=%v", p, err)
				} else {
					logLine(root, 1, cfg, "日常优化：background cpuset=%s", eff)
				}
			}
		}
	}

	if cfg.ZramPreferZstd && zramSupports("zstd") {
		disksize := readInt64("/sys/block/zram0/disksize")
		if disksize == 0 {
			cur := zramCurrentAlgorithm()
			if r.ZramAlg == "" && cur != "" {
				r.ZramAlg = cur
				changed = true
			}
			if cur != "zstd" {
				if err := writeString("/sys/block/zram0/comp_algorithm", "zstd"); err != nil {
					logLine(root, 2, cfg, "日常优化：zram未初始化但切换zstd失败: %v", err)
				} else {
					logLine(root, 1, cfg, "日常优化：zram算法切换为zstd")
				}
			}
		} else {
			logLine(root, 2, cfg, "日常优化：zram已初始化，跳过算法热切换")
		}
	}

	if changed {
		if err := saveDailyRestore(root, r); err != nil {
			return err
		}
	}
	return nil
}

func restoreDailyOptimization(root string) error {
	r := loadDailyRestore(root)
	if r.CPUSetPath != "" && r.CPUSetOrig != "" {
		_ = writeString(r.CPUSetPath, r.CPUSetOrig)
	}
	if r.ZramAlg != "" && readInt64("/sys/block/zram0/disksize") == 0 {
		_ = writeString("/sys/block/zram0/comp_algorithm", r.ZramAlg)
	}
	_ = os.Remove(dailyRestorePath(root))
	return nil
}

func dailyStatus(root string, cfg Config) DailyStatus {
	st := DailyStatus{Enabled: cfg.DailyOptimize, EfficiencyCPUs: efficiencyCPUList()}
	r := loadDailyRestore(root)
	st.Applied = r.CPUSetPath != "" || r.ZramAlg != ""
	st.CPUSetPath = r.CPUSetPath
	if !cfg.ZramPreferZstd {
		st.ZramStatus = "关闭"
	} else if zramCurrentAlgorithm() == "zstd" {
		st.ZramStatus = "zstd"
	} else if readInt64("/sys/block/zram0/disksize") > 0 {
		st.ZramStatus = "运行中，安全跳过热切换"
	} else if !zramSupports("zstd") {
		st.ZramStatus = "内核不支持zstd"
	} else {
		st.ZramStatus = fmt.Sprintf("当前%s", zramCurrentAlgorithm())
	}
	return st
}
