package repository

import (
	"runtime"
	"testing"
)

func TestMemStats_GetGauges(t *testing.T) {
	ms := MemStats{
		MemStats: runtime.MemStats{
			Alloc:         1,
			BuckHashSys:   2,
			Frees:         3,
			GCCPUFraction: 0.25,
			HeapAlloc:     4,
			Sys:           5,
			TotalAlloc:    6,
		},
	}

	got := ms.GetGauges()

	if len(got) != 27 {
		t.Fatalf("len(GetGauges()) = %d, ожидается 27", len(got))
	}
	if got["Alloc"] != 1 {
		t.Fatalf("Alloc = %v, ожидается 1", got["Alloc"])
	}
	if got["GCCPUFraction"] != 0.25 {
		t.Fatalf("GCCPUFraction = %v, ожидается 0.25", got["GCCPUFraction"])
	}
	if got["HeapAlloc"] != 4 {
		t.Fatalf("HeapAlloc = %v, ожидается 4", got["HeapAlloc"])
	}
	if got["Sys"] != 5 {
		t.Fatalf("Sys = %v, ожидается 5", got["Sys"])
	}
	if got["TotalAlloc"] != 6 {
		t.Fatalf("TotalAlloc = %v, ожидается 6", got["TotalAlloc"])
	}
	if _, ok := got["PollCount"]; ok {
		t.Fatal("PollCount неожиданно присутствует в gauge map")
	}
}
