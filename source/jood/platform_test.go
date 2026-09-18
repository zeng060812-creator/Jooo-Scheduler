package main

import "testing"

func TestAssignRolesXiaomi14Measured(t *testing.T) {
	ps := testPolicies()
	roles := map[int]string{}
	for _, p := range ps {
		roles[p.ID] = p.Role
	}
	if roles[0] != roleEfficiency || roles[2] != rolePrimary || roles[5] != roleSecondary || roles[7] != rolePrime {
		t.Fatalf("unexpected roles: %+v", roles)
	}
}

func TestAssignRolesThreePolicies(t *testing.T) {
	ps := assignPolicyRoles([]Policy{
		{ID: 0, CPUList: []int{0, 1}, Capacity: 400, HWMax: 2200000},
		{ID: 2, CPUList: []int{2, 3, 4}, Capacity: 850, HWMax: 3000000},
		{ID: 5, CPUList: []int{5}, Capacity: 1024, HWMax: 3300000},
	})
	if ps[0].Role != roleEfficiency || ps[1].Role != rolePrimary || ps[2].Role != rolePrime {
		t.Fatalf("3-policy classification failed: %+v", ps)
	}
}

func TestCPUListAndMask(t *testing.T) {
	cpus := parseCPUList("0-1 3,5-6")
	if got := compressCPUList(cpus); got != "0-1,3,5-6" {
		t.Fatalf("cpulist=%s", got)
	}
	if got := cpusToMask([]int{2, 3, 4, 5, 6}); got != "7c" {
		t.Fatalf("mask=%s", got)
	}
	if got := cpusToMask([]int{2, 3, 4, 5, 6, 7}); got != "fc" {
		t.Fatalf("mask=%s", got)
	}
}
