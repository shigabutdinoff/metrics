package agent

import (
	"context"
	"fmt"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	"go.uber.org/zap"
)

// collectGopsutil собирает дополнительные gauge-метрики через пакет gopsutil
func (a *Agent) collectGopsutil(ctx context.Context) {
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		total := float64(vm.Total)
		free := float64(vm.Free)
		a.Storage.SetGauge(ctx, "TotalMemory", &total)
		a.Storage.SetGauge(ctx, "FreeMemory", &free)
	} else {
		a.Logger.Warn("Ошибка сбора метрик памяти gopsutil", zap.Error(err))
	}

	if per, err := cpu.PercentWithContext(ctx, 0, true); err == nil {
		for i := range per {
			v := per[i]
			a.Storage.SetGauge(ctx, fmt.Sprintf("CPUutilization%d", i+1), &v)
		}
	} else {
		a.Logger.Warn("Ошибка сбора метрик CPU gopsutil", zap.Error(err))
	}
}
