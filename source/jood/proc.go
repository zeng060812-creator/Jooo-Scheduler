package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type CPUTime struct {
	Total uint64
	Idle  uint64
}

func readCPUStat() map[int]CPUTime {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil
	}
	defer f.Close()
	out := map[int]CPUTime{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) < 5 || !strings.HasPrefix(fs[0], "cpu") || fs[0] == "cpu" {
			continue
		}
		id, err := strconv.Atoi(strings.TrimPrefix(fs[0], "cpu"))
		if err != nil {
			continue
		}
		vals := make([]uint64, 0, len(fs)-1)
		for _, s := range fs[1:] {
			v, _ := strconv.ParseUint(s, 10, 64)
			vals = append(vals, v)
		}
		var total uint64
		for _, v := range vals {
			total += v
		}
		idle := vals[3]
		if len(vals) > 4 {
			idle += vals[4]
		}
		out[id] = CPUTime{Total: total, Idle: idle}
	}
	return out
}

func cpuUtils(prev, cur map[int]CPUTime) map[int]float64 {
	out := map[int]float64{}
	for id, c := range cur {
		p, ok := prev[id]
		if !ok {
			continue
		}
		dt := c.Total - p.Total
		di := c.Idle - p.Idle
		if dt == 0 {
			continue
		}
		u := float64(dt-di) / float64(dt)
		if u < 0 {
			u = 0
		}
		if u > 1 {
			u = 1
		}
		out[id] = u
	}
	return out
}

func clusterUtil(utils map[int]float64, id int) float64 {
	cpus := map[int][]int{0: {0, 1}, 2: {2, 3, 4}, 5: {5, 6}, 7: {7}}[id]
	max := 0.0
	for _, c := range cpus {
		if utils[c] > max {
			max = utils[c]
		}
	}
	return max
}

type ThreadSample struct {
	TID   int
	Name  string
	Ticks uint64
}
type ThreadLoad struct {
	TID  int
	Name string
	Util float64
}

func findPackagePID(pkg string) int {
	entries, _ := os.ReadDir("/proc")
	fallback := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil {
			continue
		}
		cmd := strings.TrimSpace(strings.Split(string(b), "\x00")[0])
		if cmd == pkg {
			return pid
		}
		if fallback == 0 && strings.HasPrefix(cmd, pkg+":") {
			fallback = pid
		}
	}
	return fallback
}

func readThreads(pid int) map[int]ThreadSample {
	out := map[int]ThreadSample{}
	dirs, _ := os.ReadDir(fmt.Sprintf("/proc/%d/task", pid))
	for _, d := range dirs {
		tid, err := strconv.Atoi(d.Name())
		if err != nil {
			continue
		}
		base := fmt.Sprintf("/proc/%d/task/%d", pid, tid)
		stat, err := os.ReadFile(filepath.Join(base, "stat"))
		if err != nil {
			continue
		}
		s := string(stat)
		rp := strings.LastIndex(s, ")")
		if rp < 0 || rp+2 >= len(s) {
			continue
		}
		fields := strings.Fields(s[rp+2:])
		if len(fields) < 13 {
			continue
		}
		ut, _ := strconv.ParseUint(fields[11], 10, 64)
		st, _ := strconv.ParseUint(fields[12], 10, 64)
		name := strings.TrimSpace(readTrim(filepath.Join(base, "comm")))
		out[tid] = ThreadSample{TID: tid, Name: name, Ticks: ut + st}
	}
	return out
}

func threadLoads(prev, cur map[int]ThreadSample, avgCPUJiffies float64) []ThreadLoad {
	if avgCPUJiffies <= 0 {
		return nil
	}
	out := []ThreadLoad{}
	for tid, c := range cur {
		p, ok := prev[tid]
		if !ok || c.Ticks < p.Ticks {
			continue
		}
		u := float64(c.Ticks-p.Ticks) / avgCPUJiffies
		if u < 0 {
			u = 0
		}
		if u > 1.5 {
			u = 1.5
		}
		out = append(out, ThreadLoad{TID: tid, Name: c.Name, Util: u})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Util > out[j].Util })
	return out
}

func avgCPUJiffies(prev, cur map[int]CPUTime) float64 {
	var sum uint64
	n := 0
	for id, c := range cur {
		if p, ok := prev[id]; ok && c.Total >= p.Total {
			sum += c.Total - p.Total
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return float64(sum) / float64(n)
}
