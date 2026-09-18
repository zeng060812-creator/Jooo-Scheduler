package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	roleEfficiency = "efficiency"
	rolePrimary    = "performance_primary"
	roleSecondary  = "performance_secondary"
	rolePrime      = "prime"
)

func parseCPUList(raw string) []int {
	raw = strings.ReplaceAll(raw, ",", " ")
	out := []int{}
	seen := map[int]bool{}
	for _, token := range strings.Fields(raw) {
		if strings.Contains(token, "-") {
			p := strings.SplitN(token, "-", 2)
			a, e1 := strconv.Atoi(p[0])
			b, e2 := strconv.Atoi(p[1])
			if e1 != nil || e2 != nil || a < 0 || b < a || b > 255 {
				continue
			}
			for i := a; i <= b; i++ {
				if !seen[i] {
					seen[i] = true
					out = append(out, i)
				}
			}
			continue
		}
		if n, err := strconv.Atoi(token); err == nil && n >= 0 && n <= 255 && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

func compressCPUList(cpus []int) string {
	if len(cpus) == 0 {
		return ""
	}
	cpus = append([]int(nil), cpus...)
	sort.Ints(cpus)
	uniq := cpus[:0]
	last := -1
	for _, c := range cpus {
		if c != last {
			uniq = append(uniq, c)
			last = c
		}
	}
	var parts []string
	for i := 0; i < len(uniq); {
		j := i
		for j+1 < len(uniq) && uniq[j+1] == uniq[j]+1 {
			j++
		}
		if j == i {
			parts = append(parts, strconv.Itoa(uniq[i]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", uniq[i], uniq[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ",")
}

func unionCPUs(groups ...[]int) []int {
	seen := map[int]bool{}
	out := []int{}
	for _, g := range groups {
		for _, c := range g {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	sort.Ints(out)
	return out
}

func cpusToMask(cpus []int) string {
	var mask uint64
	for _, c := range cpus {
		if c >= 0 && c < 64 {
			mask |= uint64(1) << uint(c)
		}
	}
	if mask == 0 {
		return ""
	}
	return fmt.Sprintf("%x", mask)
}

func policyScore(p Policy, allCapacity bool) int64 {
	if allCapacity && p.Capacity > 0 {
		return p.Capacity*10_000_000 + p.HWMax
	}
	if p.HWMax > 0 {
		return p.HWMax
	}
	return p.Max
}

func assignPolicyRoles(ps []Policy) []Policy {
	if len(ps) == 0 {
		return ps
	}
	allCapacity := true
	for _, p := range ps {
		if p.Capacity <= 0 {
			allCapacity = false
			break
		}
	}
	idx := make([]int, len(ps))
	for i := range ps {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool {
		a, b := ps[idx[i]], ps[idx[j]]
		sa, sb := policyScore(a, allCapacity), policyScore(b, allCapacity)
		if sa == sb {
			return a.ID < b.ID
		}
		return sa < sb
	})
	if len(idx) == 1 {
		ps[idx[0]].Role = rolePrimary
		return ps
	}
	ps[idx[0]].Role = roleEfficiency
	ps[idx[len(idx)-1]].Role = rolePrime
	mids := idx[1 : len(idx)-1]
	sort.Slice(mids, func(i, j int) bool {
		return policyScore(ps[mids[i]], allCapacity) > policyScore(ps[mids[j]], allCapacity)
	})
	if len(mids) > 0 {
		ps[mids[0]].Role = rolePrimary
	}
	for _, i := range mids[1:] {
		ps[i].Role = roleSecondary
	}
	return ps
}

func policyByRole(ps []Policy, role string) []Policy {
	out := []Policy{}
	for _, p := range ps {
		if p.Role == role {
			out = append(out, p)
		}
	}
	return out
}

func roleCPUs(ps []Policy, roles ...string) []int {
	wanted := map[string]bool{}
	for _, r := range roles {
		wanted[r] = true
	}
	groups := [][]int{}
	for _, p := range ps {
		if wanted[p.Role] {
			groups = append(groups, p.CPUList)
		}
	}
	return unionCPUs(groups...)
}

func policyUtilFor(p Policy, utils map[int]float64) float64 {
	max := 0.0
	for _, c := range p.CPUList {
		if utils[c] > max {
			max = utils[c]
		}
	}
	return max
}

func roleMaxUtil(ps []Policy, utils map[int]float64, role string) float64 {
	max := 0.0
	for _, p := range ps {
		if p.Role != role {
			continue
		}
		if u := policyUtilFor(p, utils); u > max {
			max = u
		}
	}
	return max
}

func totalPolicyCPUs(ps []Policy) []int {
	groups := make([][]int, 0, len(ps))
	for _, p := range ps {
		groups = append(groups, p.CPUList)
	}
	return unionCPUs(groups...)
}
