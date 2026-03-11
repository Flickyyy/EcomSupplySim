package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"

	"EcomSupplySim/internal/rng"
	"EcomSupplySim/internal/sim"
)

// ---------- output types ----------

type Pt struct {
	C   int     `json:"c"`
	L   float64 `json:"l"`
	Del float64 `json:"del"`
	Avg float64 `json:"avg"`
	CU  float64 `json:"cu"`
	SLA float64 `json:"sla"`
}

type DPt struct {
	PD   float64 `json:"pd"`
	Del  float64 `json:"del"`
	Disp float64 `json:"disp"`
}

type VfRow struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
	Exp  string `json:"exp"`
	Act  string `json:"act"`
	Pass bool   `json:"pass"`
}

type MSt struct {
	Mean float64 `json:"mean"`
	Std  float64 `json:"std"`
	Vr   float64 `json:"var"`
	CI   float64 `json:"ci"`
	Lo3  float64 `json:"lo3"`
	Hi3  float64 `json:"hi3"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
}

type Hist struct {
	Bins []float64 `json:"bins"`
	Cnt  []int     `json:"cnt"`
}

type Roll struct {
	Lbl []int     `json:"lbl"`
	Del []float64 `json:"del"`
	Avg []float64 `json:"avg"`
	CU  []float64 `json:"cu"`
	SLA []float64 `json:"sla"`
}

type Scn struct {
	Name string          `json:"name"`
	N    int             `json:"n"`
	Tot  int             `json:"tot"`
	St   map[string]MSt  `json:"st"`
	Hs   map[string]Hist `json:"hs"`
	Ro   Roll            `json:"ro"`
}

type Output struct {
	Heat []Pt    `json:"heat"`
	HR   []int   `json:"hr"`
	HC   []float64 `json:"hc"`
	ByC  []Pt    `json:"byC"`
	ByL  []Pt    `json:"byL"`
	ByD  []DPt   `json:"byD"`
	Vf   []VfRow `json:"vf"`
	Sc   []Scn   `json:"sc"`
}

// ---------- helpers ----------

const seed0 = 42

func rd(v float64, d int) float64 {
	p := math.Pow(10, float64(d))
	return math.Round(v*p) / p
}

func runSim(cfg sim.Config, minN int) *sim.Engine {
	e := sim.NewEngine(cfg)
	r := rng.New(seed0)
	tot := 0
	for tot < minN {
		tot += e.SimulateDay(rng.New(r.Uint64()))
	}
	return e
}

func pt(c int, l float64, e *sim.Engine) Pt {
	s := &e.Stats
	t := float64(s.TotalOrders)
	return Pt{
		C: c, L: l,
		Del: rd(float64(s.ByStatus[sim.Delivered])/t*100, 1),
		Avg: rd(s.AvgDelivery(), 1),
		CU:  rd(s.CourierUtil(), 1),
		SLA: rd(float64(s.SLABreaches)/t*100, 1),
	}
}

func calcMSt(v []float64) MSt {
	n := float64(len(v))
	var s, s2 float64
	mn, mx := v[0], v[0]
	for _, x := range v {
		s += x
		s2 += x * x
		if x < mn {
			mn = x
		}
		if x > mx {
			mx = x
		}
	}
	mean := s / n
	vr := s2/n - mean*mean
	if vr < 0 {
		vr = 0
	}
	std := math.Sqrt(vr)
	return MSt{
		Mean: rd(mean, 2), Std: rd(std, 2), Vr: rd(vr, 2),
		CI:  rd(1.96*std/math.Sqrt(n), 3),
		Lo3: rd(mean-3*std, 1), Hi3: rd(mean+3*std, 1),
		Min: rd(mn, 1), Max: rd(mx, 1),
	}
}

func histogram(v []float64, nb int) Hist {
	mn, mx := v[0], v[0]
	for _, x := range v {
		if x < mn {
			mn = x
		}
		if x > mx {
			mx = x
		}
	}
	w := (mx - mn) / float64(nb)
	if w == 0 {
		w = 1
	}
	bins := make([]float64, nb)
	cnt := make([]int, nb)
	for i := range bins {
		bins[i] = rd(mn+w*(float64(i)+0.5), 1)
	}
	for _, x := range v {
		idx := int((x - mn) / w)
		if idx >= nb {
			idx = nb - 1
		}
		if idx < 0 {
			idx = 0
		}
		cnt[idx]++
	}
	return Hist{Bins: bins, Cnt: cnt}
}

func rolling(v []float64, w int) ([]int, []float64) {
	var lbl []int
	var out []float64
	for i := w; i <= len(v); i += w {
		s := 0.0
		for j := i - w; j < i; j++ {
			s += v[j]
		}
		lbl = append(lbl, i)
		out = append(out, rd(s/float64(w), 1))
	}
	return lbl, out
}

func buildScenario(name string, cfg sim.Config) Scn {
	e := runSim(cfg, 1_000_000)
	days := e.Days
	n := len(days)
	del := make([]float64, n)
	avg := make([]float64, n)
	cu := make([]float64, n)
	sla := make([]float64, n)
	for i, d := range days {
		if d.Orders > 0 {
			del[i] = float64(d.Delivered) / float64(d.Orders) * 100
		}
		avg[i] = d.AvgDelivery
		cu[i] = d.CourierUtil
		sla[i] = d.SLABreachPct
	}
	st := map[string]MSt{
		"del": calcMSt(del), "avg": calcMSt(avg),
		"cu": calcMSt(cu), "sla": calcMSt(sla),
	}
	hs := map[string]Hist{
		"del": histogram(del, 30), "avg": histogram(avg, 30),
		"cu": histogram(cu, 30), "sla": histogram(sla, 30),
	}
	lbl, delR := rolling(del, 50)
	_, avgR := rolling(avg, 50)
	_, cuR := rolling(cu, 50)
	_, slaR := rolling(sla, 50)
	return Scn{
		Name: name, N: n, Tot: e.Stats.TotalOrders,
		St: st, Hs: hs,
		Ro: Roll{Lbl: lbl, Del: delR, Avg: avgR, CU: cuR, SLA: slaR},
	}
}

// ---------- verification ----------

func verify() []VfRow {
	var vv []VfRow

	// 1. Order balance
	e := runSim(sim.DefaultConfig(), 10000)
	s := &e.Stats
	sum := s.ByStatus[0] + s.ByStatus[1] + s.ByStatus[2] + s.ByStatus[3]
	vv = append(vv, VfRow{
		Name: "Баланс заказов",
		Desc: "D+C+R+Disp = Total",
		Exp:  fmt.Sprint(s.TotalOrders),
		Act:  fmt.Sprint(sum),
		Pass: sum == s.TotalOrders,
	})

	// 2. Expected orders/day (analytical)
	avgOrd := float64(s.TotalOrders) / float64(len(e.Days))
	diff := math.Abs(avgOrd-153) / 153 * 100
	vv = append(vv, VfRow{
		Name: "Среднее заказов/день",
		Desc: "sum lambda(h)*1h = 153",
		Exp:  "~153",
		Act:  fmt.Sprintf("%.1f (delta %.1f%%)", avgOrd, diff),
		Pass: diff < 5,
	})

	// 3. Snapshot consistency
	var ss int
	for _, d := range e.Days {
		ss += d.Orders
	}
	vv = append(vv, VfRow{
		Name: "Целостность снимков",
		Desc: "sum(day.Orders) = TotalOrders",
		Exp:  fmt.Sprint(s.TotalOrders),
		Act:  fmt.Sprint(ss),
		Pass: ss == s.TotalOrders,
	})

	// 4. Per-day balance
	ok := true
	for _, d := range e.Days {
		if d.Delivered+d.Cancelled+d.Restocked+d.Disposed != d.Orders {
			ok = false
			break
		}
	}
	vv = append(vv, VfRow{
		Name: "Баланс каждого дня",
		Desc: "D+C+R+Disp = Orders for each day",
		Exp:  "OK",
		Act:  fmt.Sprintf("OK (%d days)", len(e.Days)),
		Pass: ok,
	})

	// 5. Ideal scenario -> ~100% delivery
	cfg := sim.DefaultConfig()
	cfg.NCouriers = 50
	cfg.NStores = 10
	cfg.SlotsPerStore = 5
	cfg.InitialStock = 10000
	cfg.PDefect = 0
	cfg.PAbsent = 0
	cfg.PRetBase = 0
	e2 := runSim(cfg, 10000)
	s2 := &e2.Stats
	dp := float64(s2.ByStatus[sim.Delivered]) / float64(s2.TotalOrders) * 100
	vv = append(vv, VfRow{
		Name: "Идеальные условия",
		Desc: "50 couriers, no defects/absence/returns",
		Exp:  "100%",
		Act:  fmt.Sprintf("%.1f%%", dp),
		Pass: dp > 99.5,
	})

	// 6. Assembly utilization (analytical)
	au := e.Stats.AssemblyUtil()
	ea := avgOrd * 15 / (4 * 3 * 1440) * 100
	vv = append(vv, VfRow{
		Name: "Загрузка сборки (аналит.)",
		Desc: "N_ord * mu_asm / (N_st*K_sl*1440)*100",
		Exp:  fmt.Sprintf("~%.1f%%", ea),
		Act:  fmt.Sprintf("%.1f%%", au),
		Pass: math.Abs(au-ea) < 3,
	})

	// 7. Monotonicity: more couriers -> more delivered
	prev := 0.0
	mono := true
	for nc := 1; nc <= 20; nc += 2 {
		c := sim.DefaultConfig()
		c.NCouriers = nc
		em := runSim(c, 20000)
		dp := float64(em.Stats.ByStatus[sim.Delivered]) / float64(em.Stats.TotalOrders) * 100
		if nc > 1 && dp < prev-2 {
			mono = false
		}
		prev = dp
	}
	act := "confirmed"
	if !mono {
		act = "violated"
	}
	vv = append(vv, VfRow{
		Name: "Монотонность (couriers up -> delivered up)",
		Desc: "More couriers -> higher delivery %",
		Exp:  "monotone",
		Act:  act,
		Pass: mono,
	})

	return vv
}

// ---------- main ----------

func main() {
	var out Output

	// 2D heatmap: NCouriers x LambdaMult
	out.HR = []int{1, 2, 3, 5, 8, 10, 15, 20, 25, 30}
	out.HC = []float64{0.5, 0.75, 1.0, 1.25, 1.5, 2.0, 2.5, 3.0}
	for _, c := range out.HR {
		for _, l := range out.HC {
			cfg := sim.DefaultConfig()
			cfg.NCouriers = c
			cfg.LambdaMult = l
			out.Heat = append(out.Heat, pt(c, l, runSim(cfg, 30000)))
		}
		fmt.Fprintf(os.Stderr, "  heat c=%d\n", c)
	}

	// 1D: vary couriers (1..30)
	for c := 1; c <= 30; c++ {
		cfg := sim.DefaultConfig()
		cfg.NCouriers = c
		out.ByC = append(out.ByC, pt(c, 1.0, runSim(cfg, 30000)))
	}
	fmt.Fprintln(os.Stderr, "  1D couriers done")

	// 1D: vary lambda (0.3..3.0)
	for li := 3; li <= 30; li++ {
		l := float64(li) / 10.0
		cfg := sim.DefaultConfig()
		cfg.LambdaMult = l
		out.ByL = append(out.ByL, pt(5, l, runSim(cfg, 30000)))
	}
	fmt.Fprintln(os.Stderr, "  1D lambda done")

	// 1D: vary PDefect (0..0.20)
	for di := 0; di <= 20; di += 2 {
		pd := float64(di) / 100.0
		cfg := sim.DefaultConfig()
		cfg.PDefect = pd
		e := runSim(cfg, 30000)
		s := &e.Stats
		t := float64(s.TotalOrders)
		out.ByD = append(out.ByD, DPt{
			PD:   pd,
			Del:  rd(float64(s.ByStatus[sim.Delivered])/t*100, 1),
			Disp: rd(float64(s.ByStatus[sim.Disposed])/t*100, 1),
		})
	}
	fmt.Fprintln(os.Stderr, "  1D defect done")

	// Verification tests
	out.Vf = verify()
	fmt.Fprintln(os.Stderr, "  verification done")

	// 4 scenarios with full statistical analysis
	scenarios := []struct {
		name string
		cfg  sim.Config
	}{
		{"default", sim.DefaultConfig()},
		{"peak", sim.PeakConfig()},
		{"chaos", sim.ChaosConfig()},
		{"understaffed", sim.UnderstaffedConfig()},
	}
	for _, sc := range scenarios {
		out.Sc = append(out.Sc, buildScenario(sc.name, sc.cfg))
		fmt.Fprintf(os.Stderr, "  scenario %s done\n", sc.name)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "json encode error: %v\n", err)
		os.Exit(1)
	}
}
