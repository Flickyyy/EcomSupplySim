// EcomSupplySim — Практическая работа №2
// Имитационная модель службы e-commerce доставки.
//
// Запуск:
//
// go run ./cmd/sim [flags]
//
// Флаги:
//
// -scenario  default|peak|chaos|understaffed
// -seed      uint64 seed для ГПСЧ (default 42)
// -orders    минимальное количество заказов (default 1_000_000)
package main

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"EcomSupplySim/internal/rng"
	"EcomSupplySim/internal/sim"
)

func main() {
	scenario := flag.String("scenario", "default", "Сценарий: default, peak, chaos, understaffed")
	seed := flag.Uint64("seed", 42, "Seed ГПСЧ xoshiro256**")
	minOrders := flag.Int("orders", 1_000_000, "Минимальное число заказов для моделирования")
	flag.Parse()

	cfg := pickConfig(*scenario)
	engine := sim.NewEngine(cfg)
	masterRng := rng.New(*seed)

	// Run simulation
	start := time.Now()
	totalOrders := 0
	nDays := 0

	for totalOrders < *minOrders {
		daySeed := masterRng.Uint64()
		dayRng := rng.New(daySeed)
		n := engine.SimulateDay(dayRng)
		totalOrders += n
		nDays++
	}

	elapsed := time.Since(start)
	printResults(engine, *scenario, nDays, elapsed)
}

func pickConfig(name string) sim.Config {
	switch strings.ToLower(name) {
	case "peak":
		return sim.PeakConfig()
	case "chaos":
		return sim.ChaosConfig()
	case "understaffed":
		return sim.UnderstaffedConfig()
	default:
		return sim.DefaultConfig()
	}
}

func printResults(e *sim.Engine, scenario string, nDays int, elapsed time.Duration) {
	s := &e.Stats
	cfg := &e.Cfg

	line := strings.Repeat("=", 62)
	thin := strings.Repeat("-", 62)

	fmt.Println(line)
	fmt.Println("  EcomSupplySim | PR #2 | Имитационное моделирование")
	fmt.Println("  ГПСЧ: xoshiro256** (Blackman & Vigna, 2018)")
	fmt.Println(line)
	fmt.Printf("  Сценарий:            %s\n", scenario)
	fmt.Printf("  Магазинов:           %d   Курьеров: %d   Слотов/маг: %d\n",
		cfg.NStores, cfg.NCouriers, cfg.SlotsPerStore)
	fmt.Printf("  Дней смоделировано:  %d\n", nDays)
	fmt.Printf("  Всего заказов:       %d\n", s.TotalOrders)
	fmt.Printf("  Время работы:        %s\n", elapsed.Round(time.Millisecond))

	fmt.Println(thin)
	fmt.Println("                    СТАТУСЫ ЗАКАЗОВ")
	fmt.Println(thin)
	for i := 0; i < 4; i++ {
		st := sim.Status(i)
		cnt := s.ByStatus[i]
		pct := float64(cnt) / float64(s.TotalOrders) * 100
		fmt.Printf("  %-18s %9d  (%5.2f%%)\n", st.String(), cnt, pct)
	}

	fmt.Println(thin)
	fmt.Println("                    МЕТРИКИ")
	fmt.Println(thin)
	fmt.Printf("  Ср. время доставки:          %6.1f мин  (sigma = %.1f)\n",
		s.AvgDelivery(), s.StdDelivery())
	fmt.Printf("  Ср. время возврата:           %6.1f мин\n", s.AvgReturn())
	fmt.Printf("  Ср. ожидание в очереди:       %6.1f мин\n", s.AvgQueueWait())
	servedPerHour := float64(s.ByStatus[sim.Delivered]) / float64(nDays) / 24.0
	fmt.Printf("  Обслужено заказов/час:        %6.1f\n", servedPerHour)
	slaPct := float64(s.SLABreaches) / float64(s.TotalOrders) * 100
	fmt.Printf("  SLA нарушений:                %5.1f%%\n", slaPct)

	fmt.Println(thin)
	fmt.Println("                    ЗАГРУЗКА РЕСУРСОВ")
	fmt.Println(thin)
	fmt.Printf("  Сборочные слоты:             %5.1f%%\n", s.AssemblyUtil())
	fmt.Printf("  Курьеры:                     %5.1f%%\n", s.CourierUtil())

	fmt.Println(thin)
	fmt.Println("                    ПО ПРИОРИТЕТАМ")
	fmt.Println(thin)
	fmt.Printf("  %-10s %8s %10s %10s %9s\n", "Тип", "Заказов", "Доставлено", "Ср.время", "SLABreach")
	for i := 0; i < 3; i++ {
		ps := &s.ByPriority[i]
		pri := sim.Priority(i)
		avgT := 0.0
		if ps.Delivered > 0 {
			avgT = ps.DeliverySum / float64(ps.Delivered)
		}
		slaBr := 0.0
		if ps.Count > 0 {
			slaBr = float64(ps.SLABreach) / float64(ps.Count) * 100
		}
		fmt.Printf("  %-10s %8d %10d %8.1f мин %7.1f%%\n",
			pri.String(), ps.Count, ps.Delivered, avgT, slaBr)
	}

	fmt.Println(line)
}
