package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"

	"EcomSupplySim/internal/rng"
	"EcomSupplySim/internal/sim"
)

const (
	baseSeed  uint64 = 20260319
	minOrders int    = 60_000
)

type lambdaCase struct {
	L      float64 `json:"l"`
	Weight float64 `json:"weight"`
}

type Params struct {
	NCouriers     int `json:"nCouriers"`
	NStores       int `json:"nStores"`
	SlotsPerStore int `json:"slotsPerStore"`
}

type ScenarioMetric struct {
	Lambda      float64 `json:"lambda"`
	Weight      float64 `json:"weight"`
	Orders      int     `json:"orders"`
	Delivered   float64 `json:"deliveredPct"`
	AvgDelivery float64 `json:"avgDeliveryMin"`
	SLA         float64 `json:"slaBreachPct"`
	CourUtil    float64 `json:"courierUtilPct"`
	AsmUtil     float64 `json:"assemblyUtilPct"`
	Cancelled   float64 `json:"cancelledPct"`
	Disposed    float64 `json:"disposedPct"`
}

type Aggregate struct {
	Delivered   float64 `json:"deliveredPct"`
	AvgDelivery float64 `json:"avgDeliveryMin"`
	SLA         float64 `json:"slaBreachPct"`
	CourUtil    float64 `json:"courierUtilPct"`
	AsmUtil     float64 `json:"assemblyUtilPct"`
	Cancelled   float64 `json:"cancelledPct"`
	Disposed    float64 `json:"disposedPct"`
}

type CostBreakdown struct {
	Couriers       float64 `json:"couriers"`
	Stores         float64 `json:"stores"`
	Slots          float64 `json:"slots"`
	SLAPenalty     float64 `json:"slaPenalty"`
	CancelPenalty  float64 `json:"cancelPenalty"`
	DisposePenalty float64 `json:"disposePenalty"`
	Total          float64 `json:"total"`
}

type ConstraintEval struct {
	Name  string  `json:"name"`
	Expr  string  `json:"expr"`
	Value float64 `json:"value"`
	Limit float64 `json:"limit"`
	Pass  bool    `json:"pass"`
}

type Candidate struct {
	Params      Params           `json:"params"`
	Scenarios   []ScenarioMetric `json:"scenarios"`
	Agg         Aggregate        `json:"agg"`
	Cost        CostBreakdown    `json:"cost"`
	Objective   float64          `json:"objective"`
	Feasible    bool             `json:"feasible"`
	Violations  int              `json:"violations"`
	SearchIndex int              `json:"searchIndex"`
}

type SearchStep struct {
	Step      int     `json:"step"`
	Params    Params  `json:"params"`
	Objective float64 `json:"objective"`
	Feasible  bool    `json:"feasible"`
}

type Summary struct {
	GridPoints         int                `json:"gridPoints"`
	FeasiblePoints     int                `json:"feasiblePoints"`
	BestFeasible       Candidate          `json:"bestFeasible"`
	Top5               []Candidate        `json:"top5"`
	DefaultPoint       Candidate          `json:"defaultPoint"`
	LocalSearchBest    Candidate          `json:"localSearchBest"`
	LocalSearchTrace   []SearchStep       `json:"localSearchTrace"`
	ParamInfluence     map[string]float64 `json:"paramInfluence"`
	ObjectiveFormula   string             `json:"objectiveFormula"`
	Constraints        []ConstraintEval   `json:"constraintsOnBest"`
	EstimatedEffectPct float64            `json:"estimatedEffectPct"`
}

type Output struct {
	Title        string       `json:"title"`
	Model        string       `json:"model"`
	Method       []string     `json:"method"`
	DecisionVars []string     `json:"decisionVars"`
	LambdaCases  []lambdaCase `json:"lambdaCases"`
	Grid         []Candidate  `json:"grid"`
	Summary      Summary      `json:"summary"`
}

func round(v float64, d int) float64 {
	p := math.Pow(10, float64(d))
	return math.Round(v*p) / p
}

func run(cfg sim.Config, seed uint64, minN int) *sim.Engine {
	e := sim.NewEngine(cfg)
	r := rng.New(seed)
	tot := 0
	for tot < minN {
		tot += e.SimulateDay(rng.New(r.Uint64()))
	}
	return e
}

func evalCandidate(p Params, lambdas []lambdaCase, idx int) Candidate {
	var sc []ScenarioMetric
	var agg Aggregate
	for i, lc := range lambdas {
		cfg := sim.DefaultConfig()
		cfg.NCouriers = p.NCouriers
		cfg.NStores = p.NStores
		cfg.SlotsPerStore = p.SlotsPerStore
		cfg.LambdaMult = lc.L
		seed := baseSeed + uint64(idx*100+i)
		e := run(cfg, seed, minOrders)
		s := e.Stats
		tot := float64(s.TotalOrders)

		m := ScenarioMetric{
			Lambda:      lc.L,
			Weight:      lc.Weight,
			Orders:      s.TotalOrders,
			Delivered:   round(float64(s.ByStatus[sim.Delivered])/tot*100, 2),
			AvgDelivery: round(s.AvgDelivery(), 2),
			SLA:         round(float64(s.SLABreaches)/tot*100, 2),
			CourUtil:    round(s.CourierUtil(), 2),
			AsmUtil:     round(s.AssemblyUtil(), 2),
			Cancelled:   round(float64(s.ByStatus[sim.Cancelled])/tot*100, 2),
			Disposed:    round(float64(s.ByStatus[sim.Disposed])/tot*100, 2),
		}
		sc = append(sc, m)

		agg.Delivered += m.Delivered * lc.Weight
		agg.AvgDelivery += m.AvgDelivery * lc.Weight
		agg.SLA += m.SLA * lc.Weight
		agg.CourUtil += m.CourUtil * lc.Weight
		agg.AsmUtil += m.AsmUtil * lc.Weight
		agg.Cancelled += m.Cancelled * lc.Weight
		agg.Disposed += m.Disposed * lc.Weight
	}

	cost := calcCost(p, agg)
	_, viol := checkConstraints(agg, cost)
	feasible := viol == 0

	obj := cost.Total
	if !feasible {
		obj += float64(viol) * 250_000
	}

	return Candidate{
		Params:      p,
		Scenarios:   sc,
		Agg:         agg,
		Cost:        cost,
		Objective:   round(obj, 2),
		Feasible:    feasible,
		Violations:  viol,
		SearchIndex: idx,
	}
}

func calcCost(p Params, a Aggregate) CostBreakdown {
	couriers := float64(p.NCouriers) * 2200
	stores := float64(p.NStores) * 5000
	slots := float64(p.NStores*p.SlotsPerStore) * 700
	slaPenalty := a.SLA * 1800
	cancelPenalty := a.Cancelled * 1000
	disposePenalty := a.Disposed * 2400
	total := couriers + stores + slots + slaPenalty + cancelPenalty + disposePenalty
	return CostBreakdown{
		Couriers:       round(couriers, 2),
		Stores:         round(stores, 2),
		Slots:          round(slots, 2),
		SLAPenalty:     round(slaPenalty, 2),
		CancelPenalty:  round(cancelPenalty, 2),
		DisposePenalty: round(disposePenalty, 2),
		Total:          round(total, 2),
	}
}

func checkConstraints(a Aggregate, c CostBreakdown) ([]ConstraintEval, int) {
	constraints := []ConstraintEval{
		{
			Name:  "Service level",
			Expr:  "Delivered% >= 80",
			Value: round(a.Delivered, 2),
			Limit: 80,
			Pass:  a.Delivered >= 80,
		},
		{
			Name:  "SLA",
			Expr:  "SLA breach% <= 6",
			Value: round(a.SLA, 2),
			Limit: 6,
			Pass:  a.SLA <= 6,
		},
		{
			Name:  "Speed",
			Expr:  "Avg delivery <= 75 min",
			Value: round(a.AvgDelivery, 2),
			Limit: 75,
			Pass:  a.AvgDelivery <= 75,
		},
		{
			Name:  "Budget",
			Expr:  "Total cost <= 110000",
			Value: round(c.Total, 2),
			Limit: 110000,
			Pass:  c.Total <= 110000,
		},
	}
	viol := 0
	for _, cc := range constraints {
		if !cc.Pass {
			viol++
		}
	}
	return constraints, viol
}

func better(a, b Candidate) bool {
	if a.Feasible != b.Feasible {
		return a.Feasible
	}
	if a.Violations != b.Violations {
		return a.Violations < b.Violations
	}
	return a.Objective < b.Objective
}

func within(p Params) bool {
	if p.NCouriers < 4 || p.NCouriers > 24 {
		return false
	}
	if p.NStores < 3 || p.NStores > 6 {
		return false
	}
	if p.SlotsPerStore < 2 || p.SlotsPerStore > 5 {
		return false
	}
	return true
}

func localSearch(start Candidate, lambdas []lambdaCase, startIdx int) (Candidate, []SearchStep) {
	best := start
	idx := startIdx
	trace := []SearchStep{{Step: 0, Params: best.Params, Objective: best.Objective, Feasible: best.Feasible}}
	step := 1

	for {
		improved := false
		neigh := []Params{
			{best.Params.NCouriers + 1, best.Params.NStores, best.Params.SlotsPerStore},
			{best.Params.NCouriers - 1, best.Params.NStores, best.Params.SlotsPerStore},
			{best.Params.NCouriers, best.Params.NStores + 1, best.Params.SlotsPerStore},
			{best.Params.NCouriers, best.Params.NStores - 1, best.Params.SlotsPerStore},
			{best.Params.NCouriers, best.Params.NStores, best.Params.SlotsPerStore + 1},
			{best.Params.NCouriers, best.Params.NStores, best.Params.SlotsPerStore - 1},
		}
		for _, np := range neigh {
			if !within(np) {
				continue
			}
			idx++
			cand := evalCandidate(np, lambdas, idx)
			if better(cand, best) {
				best = cand
				trace = append(trace, SearchStep{Step: step, Params: best.Params, Objective: best.Objective, Feasible: best.Feasible})
				step++
				improved = true
			}
		}
		if !improved {
			break
		}
	}
	return best, trace
}

func corr(xs, ys []float64) float64 {
	if len(xs) != len(ys) || len(xs) < 2 {
		return 0
	}
	n := float64(len(xs))
	var sx, sy, sxx, syy, sxy float64
	for i := range xs {
		x, y := xs[i], ys[i]
		sx += x
		sy += y
		sxx += x * x
		syy += y * y
		sxy += x * y
	}
	num := n*sxy - sx*sy
	den := math.Sqrt((n*sxx - sx*sx) * (n*syy - sy*sy))
	if den == 0 {
		return 0
	}
	return num / den
}

func topK(cands []Candidate, k int) []Candidate {
	cp := append([]Candidate(nil), cands...)
	sort.Slice(cp, func(i, j int) bool {
		return better(cp[i], cp[j])
	})
	if len(cp) < k {
		return cp
	}
	return cp[:k]
}

func main() {
	lambdas := []lambdaCase{
		{L: 1.0, Weight: 0.5},
		{L: 1.5, Weight: 0.3},
		{L: 2.0, Weight: 0.2},
	}

	var grid []Candidate
	idx := 0
	for cour := 4; cour <= 24; cour += 2 {
		for stores := 3; stores <= 6; stores++ {
			for slots := 2; slots <= 5; slots++ {
				idx++
				p := Params{NCouriers: cour, NStores: stores, SlotsPerStore: slots}
				grid = append(grid, evalCandidate(p, lambdas, idx))
			}
		}
		fmt.Fprintf(os.Stderr, "grid couriers=%d done\n", cour)
	}

	var feasible []Candidate
	for _, c := range grid {
		if c.Feasible {
			feasible = append(feasible, c)
		}
	}

	bestFeasible := topK(grid, 1)[0]
	if len(feasible) > 0 {
		bestFeasible = topK(feasible, 1)[0]
	}

	defaultCand := evalCandidate(Params{NCouriers: 5, NStores: 4, SlotsPerStore: 3}, lambdas, idx+1000)
	localBest, trace := localSearch(defaultCand, lambdas, idx+2000)

	var xsC, xsS, xsK, ys []float64
	for _, c := range grid {
		xsC = append(xsC, float64(c.Params.NCouriers))
		xsS = append(xsS, float64(c.Params.NStores))
		xsK = append(xsK, float64(c.Params.SlotsPerStore))
		ys = append(ys, c.Objective)
	}

	constraintsOnBest, _ := checkConstraints(bestFeasible.Agg, bestFeasible.Cost)
	effect := 0.0
	if defaultCand.Cost.Total > 0 {
		effect = (defaultCand.Cost.Total - bestFeasible.Cost.Total) / defaultCand.Cost.Total * 100
	}

	topFeasible := feasible
	if len(topFeasible) == 0 {
		topFeasible = grid
	}

	out := Output{
		Title:        "Практическая работа #4: оптимизация параметров на основе имитационной модели",
		Model:        "EcomSupplySim (логистическая система e-commerce)",
		Method:       []string{"Полный перебор (grid search)", "Локальный поиск (hill climbing)"},
		DecisionVars: []string{"NCouriers", "NStores", "SlotsPerStore"},
		LambdaCases:  lambdas,
		Grid:         grid,
		Summary: Summary{
			GridPoints:       len(grid),
			FeasiblePoints:   len(feasible),
			BestFeasible:     bestFeasible,
			Top5:             topK(topFeasible, 5),
			DefaultPoint:     defaultCand,
			LocalSearchBest:  localBest,
			LocalSearchTrace: trace,
			ParamInfluence: map[string]float64{
				"NCouriers_vs_Objective": round(corr(xsC, ys), 4),
				"NStores_vs_Objective":   round(corr(xsS, ys), 4),
				"Slots_vs_Objective":     round(corr(xsK, ys), 4),
			},
			ObjectiveFormula:   "Minimize total daily cost = fixed resource cost + SLA/cancel/disposal penalties, subject to service constraints",
			Constraints:        constraintsOnBest,
			EstimatedEffectPct: round(effect, 2),
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
		os.Exit(1)
	}
}
