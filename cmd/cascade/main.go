package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"

	"EcomSupplySim/internal/rng"
)

const (
	baseSeed uint64 = 20260320
	nNodes          = 400
	maxSteps        = 120
	replicas        = 30
)

type Params struct {
	Beta          float64 `json:"beta"`
	Gamma         float64 `json:"gamma"`
	InitialInfect int     `json:"initialInfect"`
	MaxSteps      int     `json:"maxSteps"`
}

type NetworkConfig struct {
	Type    string  `json:"type"`
	N       int     `json:"n"`
	Density float64 `json:"density,omitempty"`
	M       int     `json:"m,omitempty"`
}

type DynamicsPoint struct {
	Step int     `json:"step"`
	S    float64 `json:"s"`
	I    float64 `json:"i"`
	R    float64 `json:"r"`
}

type DynamicsScenario struct {
	Name         string          `json:"name"`
	NetworkType  string          `json:"networkType"`
	NetworkParam string          `json:"networkParam"`
	Params       Params          `json:"params"`
	Series       []DynamicsPoint `json:"series"`
	AttackRate   float64         `json:"attackRatePct"`
	PeakInfected float64         `json:"peakInfectedPct"`
	PeakStep     float64         `json:"peakStep"`
	Duration     float64         `json:"duration"`
	R0Approx     float64         `json:"r0Approx"`
	AvgDegree    float64         `json:"avgDegree"`
}

type SweepPoint struct {
	NetworkType string  `json:"networkType"`
	XName       string  `json:"xName"`
	X           float64 `json:"x"`
	Attack      float64 `json:"attackRatePct"`
	Peak        float64 `json:"peakInfectedPct"`
	Duration    float64 `json:"duration"`
	Outbreak    float64 `json:"outbreakProbPct"`
}

type ThresholdPoint struct {
	NetworkType string  `json:"networkType"`
	Beta        float64 `json:"beta"`
	Outbreak    float64 `json:"outbreakProbPct"`
}

type StrategyResult struct {
	NetworkType  string  `json:"networkType"`
	Strategy     string  `json:"strategy"`
	CoveragePct  float64 `json:"coveragePct"`
	AttackRate   float64 `json:"attackRatePct"`
	PeakInfected float64 `json:"peakInfectedPct"`
	Duration     float64 `json:"duration"`
	Reduction    float64 `json:"attackReductionPctVsBaseline"`
}

type Summary struct {
	Thresholds         map[string]float64 `json:"thresholdBeta"`
	MostImportant      []string           `json:"mostImportantFactors"`
	ContainmentAdvice  []string           `json:"containmentStrategies"`
	AccelerationAdvice []string           `json:"informationSpreadStrategies"`
}

type Output struct {
	Title       string             `json:"title"`
	Model       string             `json:"model"`
	Seed        uint64             `json:"seed"`
	Params      Params             `json:"baseParams"`
	Networks    []NetworkConfig    `json:"networks"`
	Dynamics    []DynamicsScenario `json:"dynamics"`
	Sweeps      []SweepPoint       `json:"sweeps"`
	Threshold   []ThresholdPoint   `json:"threshold"`
	Strategies  []StrategyResult   `json:"strategies"`
	Summary     Summary            `json:"summary"`
	MethodNotes []string           `json:"methodNotes"`
}

type simStats struct {
	seriesS   []float64
	seriesI   []float64
	seriesR   []float64
	attack    float64
	peak      float64
	peakStep  float64
	duration  float64
	outbreakP float64
}

func round(v float64, d int) float64 {
	p := math.Pow(10, float64(d))
	return math.Round(v*p) / p
}

func newGraph(n int) [][]int {
	return make([][]int, n)
}

func addEdge(g [][]int, u, v int) {
	if u == v {
		return
	}
	g[u] = append(g[u], v)
	g[v] = append(g[v], u)
}

func hasEdge(g [][]int, u, v int) bool {
	for _, x := range g[u] {
		if x == v {
			return true
		}
	}
	return false
}

func buildER(n int, p float64, r *rng.Xoshiro256ss) [][]int {
	g := newGraph(n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			if r.Float64() < p {
				addEdge(g, i, j)
			}
		}
	}
	return g
}

func buildScaleFreeBA(n, m int, r *rng.Xoshiro256ss) [][]int {
	if m < 1 {
		m = 1
	}
	m0 := m + 1
	if m0 < 3 {
		m0 = 3
	}
	if n <= m0 {
		n = m0 + 1
	}
	g := newGraph(n)

	for i := 0; i < m0; i++ {
		for j := i + 1; j < m0; j++ {
			addEdge(g, i, j)
		}
	}

	var pref []int
	for i := 0; i < m0; i++ {
		for k := 0; k < len(g[i]); k++ {
			pref = append(pref, i)
		}
	}

	for v := m0; v < n; v++ {
		targets := make(map[int]bool)
		for len(targets) < m {
			pick := pref[r.UniformInt(0, len(pref)-1)]
			targets[pick] = true
		}
		for t := range targets {
			if !hasEdge(g, v, t) {
				addEdge(g, v, t)
				pref = append(pref, v, t)
			}
		}
	}
	return g
}

func avgDegree(g [][]int) float64 {
	s := 0.0
	for i := range g {
		s += float64(len(g[i]))
	}
	return s / float64(len(g))
}

func topDegreeNodes(g [][]int, k int) []int {
	type pair struct {
		node int
		deg  int
	}
	arr := make([]pair, 0, len(g))
	for i := range g {
		arr = append(arr, pair{node: i, deg: len(g[i])})
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].deg > arr[j].deg })
	if k > len(arr) {
		k = len(arr)
	}
	out := make([]int, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, arr[i].node)
	}
	return out
}

func randomNodes(n, k int, r *rng.Xoshiro256ss) []int {
	if k > n {
		k = n
	}
	picked := make(map[int]bool)
	out := make([]int, 0, k)
	for len(out) < k {
		v := r.UniformInt(0, n-1)
		if !picked[v] {
			picked[v] = true
			out = append(out, v)
		}
	}
	return out
}

func runSIR(g [][]int, p Params, immuneSet map[int]bool, initial []int, r *rng.Xoshiro256ss) ([]float64, []float64, []float64) {
	n := len(g)
	state := make([]int, n) // 0 S, 1 I, 2 R
	for v := range immuneSet {
		state[v] = 2
	}
	for _, v := range initial {
		if state[v] == 0 {
			state[v] = 1
		}
	}

	S := make([]float64, 0, p.MaxSteps+1)
	I := make([]float64, 0, p.MaxSteps+1)
	R := make([]float64, 0, p.MaxSteps+1)

	count := func() (int, int, int) {
		s, i, rec := 0, 0, 0
		for _, st := range state {
			switch st {
			case 0:
				s++
			case 1:
				i++
			case 2:
				rec++
			}
		}
		return s, i, rec
	}

	s0, i0, r0 := count()
	S = append(S, float64(s0)/float64(n)*100)
	I = append(I, float64(i0)/float64(n)*100)
	R = append(R, float64(r0)/float64(n)*100)

	for step := 0; step < p.MaxSteps; step++ {
		infectNext := make(map[int]bool)
		recoverNow := make(map[int]bool)
		infCount := 0
		for v := 0; v < n; v++ {
			if state[v] != 1 {
				continue
			}
			infCount++
			for _, u := range g[v] {
				if state[u] == 0 && r.Float64() < p.Beta {
					infectNext[u] = true
				}
			}
			if r.Float64() < p.Gamma {
				recoverNow[v] = true
			}
		}
		if infCount == 0 {
			break
		}
		for u := range infectNext {
			if state[u] == 0 {
				state[u] = 1
			}
		}
		for v := range recoverNow {
			if state[v] == 1 {
				state[v] = 2
			}
		}
		s, i, rec := count()
		S = append(S, float64(s)/float64(n)*100)
		I = append(I, float64(i)/float64(n)*100)
		R = append(R, float64(rec)/float64(n)*100)
	}
	return S, I, R
}

func replicateScenario(gFactory func(seed uint64) [][]int, p Params, reps int, seedBase uint64) simStats {
	stats := simStats{}
	maxLen := 0
	var counts []float64
	for rep := 0; rep < reps; rep++ {
		r := rng.New(seedBase + uint64(rep)*1009)
		g := gFactory(seedBase + uint64(rep)*17)
		initial := randomNodes(len(g), p.InitialInfect, r)
		S, I, R := runSIR(g, p, map[int]bool{}, initial, r)

		if len(S) > maxLen {
			oldS, oldI, oldR, oldC := stats.seriesS, stats.seriesI, stats.seriesR, counts
			maxLen = len(S)
			stats.seriesS = make([]float64, maxLen)
			stats.seriesI = make([]float64, maxLen)
			stats.seriesR = make([]float64, maxLen)
			counts = make([]float64, maxLen)
			copy(stats.seriesS, oldS)
			copy(stats.seriesI, oldI)
			copy(stats.seriesR, oldR)
			copy(counts, oldC)
		}

		for i := 0; i < maxLen; i++ {
			idx := i
			if idx >= len(S) {
				idx = len(S) - 1
			}
			stats.seriesS[i] += S[idx]
			stats.seriesI[i] += I[idx]
			stats.seriesR[i] += R[idx]
			counts[i]++
		}

		peak := 0.0
		peakStep := 0
		for i := range I {
			if I[i] > peak {
				peak = I[i]
				peakStep = i
			}
		}
		attack := R[len(R)-1]
		stats.attack += attack
		stats.peak += peak
		stats.peakStep += float64(peakStep)
		stats.duration += float64(len(I) - 1)
		if attack >= 20 {
			stats.outbreakP += 1
		}
	}
	for i := 0; i < len(stats.seriesS); i++ {
		if counts[i] == 0 {
			continue
		}
		stats.seriesS[i] /= counts[i]
		stats.seriesI[i] /= counts[i]
		stats.seriesR[i] /= counts[i]
	}
	stats.attack /= float64(reps)
	stats.peak /= float64(reps)
	stats.peakStep /= float64(reps)
	stats.duration /= float64(reps)
	stats.outbreakP = stats.outbreakP / float64(reps) * 100
	return stats
}

func strategyEval(gFactory func(seed uint64) [][]int, p Params, strategy string, coverage float64, reps int, seedBase uint64) simStats {
	stats := simStats{}
	for rep := 0; rep < reps; rep++ {
		r := rng.New(seedBase + uint64(rep)*7001)
		g := gFactory(seedBase + uint64(rep)*19)
		n := len(g)
		k := int(float64(n) * coverage)
		immune := map[int]bool{}
		switch strategy {
		case "random":
			for _, v := range randomNodes(n, k, r) {
				immune[v] = true
			}
		case "targeted":
			for _, v := range topDegreeNodes(g, k) {
				immune[v] = true
			}
		}

		pool := make([]int, 0, n)
		for v := 0; v < n; v++ {
			if !immune[v] {
				pool = append(pool, v)
			}
		}
		initial := make([]int, 0, p.InitialInfect)
		for len(initial) < p.InitialInfect && len(pool) > 0 {
			ix := r.UniformInt(0, len(pool)-1)
			initial = append(initial, pool[ix])
			pool[ix] = pool[len(pool)-1]
			pool = pool[:len(pool)-1]
		}

		_, I, R := runSIR(g, p, immune, initial, r)
		peak := 0.0
		for _, v := range I {
			if v > peak {
				peak = v
			}
		}
		stats.attack += R[len(R)-1]
		stats.peak += peak
		stats.duration += float64(len(I) - 1)
	}
	stats.attack /= float64(reps)
	stats.peak /= float64(reps)
	stats.duration /= float64(reps)
	return stats
}

func main() {
	base := Params{Beta: 0.12, Gamma: 0.08, InitialInfect: 3, MaxSteps: maxSteps}
	vaccination := 0.10

	networks := []NetworkConfig{
		{Type: "erdos-renyi", N: nNodes, Density: 0.02},
		{Type: "scale-free", N: nNodes, M: 2},
	}

	mkER := func(p float64) func(seed uint64) [][]int {
		return func(seed uint64) [][]int { return buildER(nNodes, p, rng.New(seed)) }
	}
	mkBA := func(m int) func(seed uint64) [][]int {
		return func(seed uint64) [][]int { return buildScaleFreeBA(nNodes, m, rng.New(seed)) }
	}

	out := Output{
		Title:    "Практическая работа #5: каскадные процессы в сетевых моделях",
		Model:    "SIR на Erdos-Renyi и Scale-Free (Barabasi-Albert)",
		Seed:     baseSeed,
		Params:   base,
		Networks: networks,
		MethodNotes: []string{
			"Дискретное время, синхронное обновление состояний S/I/R.",
			"Вероятностная передача по ребру с вероятностью beta; выздоровление с вероятностью gamma.",
			"Все агрегаты усреднены по 30 независимым репликациям.",
			"Порог эпидемии оценивается как beta, при котором вероятность крупной вспышки (attack>=20%) достигает 50%.",
		},
	}

	dynCfg := []struct {
		name   string
		net    string
		param  string
		makeG  func(seed uint64) [][]int
		beta   float64
		gamma  float64
		initI  int
		seedOf uint64
	}{
		{"ER / baseline", "erdos-renyi", "p=0.02", mkER(0.02), 0.12, 0.08, 3, 100},
		{"ER / high beta", "erdos-renyi", "p=0.02", mkER(0.02), 0.20, 0.08, 3, 200},
		{"Scale-free / baseline", "scale-free", "m=2", mkBA(2), 0.12, 0.08, 3, 300},
		{"Scale-free / low beta", "scale-free", "m=2", mkBA(2), 0.08, 0.08, 3, 400},
	}
	for _, dc := range dynCfg {
		p := Params{Beta: dc.beta, Gamma: dc.gamma, InitialInfect: dc.initI, MaxSteps: maxSteps}
		st := replicateScenario(dc.makeG, p, replicas, baseSeed+dc.seedOf)
		series := make([]DynamicsPoint, 0, len(st.seriesS))
		avgK := avgDegree(dc.makeG(baseSeed + dc.seedOf + 999))
		r0 := (dc.beta / dc.gamma) * avgK
		for i := 0; i < len(st.seriesS); i++ {
			series = append(series, DynamicsPoint{Step: i, S: round(st.seriesS[i], 2), I: round(st.seriesI[i], 2), R: round(st.seriesR[i], 2)})
		}
		out.Dynamics = append(out.Dynamics, DynamicsScenario{
			Name:         dc.name,
			NetworkType:  dc.net,
			NetworkParam: dc.param,
			Params:       p,
			Series:       series,
			AttackRate:   round(st.attack, 2),
			PeakInfected: round(st.peak, 2),
			PeakStep:     round(st.peakStep, 2),
			Duration:     round(st.duration, 2),
			R0Approx:     round(r0, 2),
			AvgDegree:    round(avgK, 2),
		})
	}
	fmt.Fprintln(os.Stderr, "dynamics done")

	for _, p := range []float64{0.01, 0.02, 0.03, 0.05, 0.08} {
		st := replicateScenario(mkER(p), base, replicas, baseSeed+1000+uint64(p*1000))
		out.Sweeps = append(out.Sweeps, SweepPoint{NetworkType: "erdos-renyi", XName: "density", X: p, Attack: round(st.attack, 2), Peak: round(st.peak, 2), Duration: round(st.duration, 2), Outbreak: round(st.outbreakP, 2)})
	}
	for _, m := range []int{1, 2, 3, 4} {
		st := replicateScenario(mkBA(m), base, replicas, baseSeed+1100+uint64(m))
		out.Sweeps = append(out.Sweeps, SweepPoint{NetworkType: "scale-free", XName: "m", X: float64(m), Attack: round(st.attack, 2), Peak: round(st.peak, 2), Duration: round(st.duration, 2), Outbreak: round(st.outbreakP, 2)})
	}
	fmt.Fprintln(os.Stderr, "network sweeps done")

	for _, net := range []struct {
		name string
		mk   func(seed uint64) [][]int
	}{
		{"erdos-renyi", mkER(0.02)},
		{"scale-free", mkBA(2)},
	} {
		for _, b := range []float64{0.04, 0.06, 0.08, 0.10, 0.12, 0.14, 0.18, 0.22} {
			pp := base
			pp.Beta = b
			st := replicateScenario(net.mk, pp, replicas, baseSeed+2000+uint64(b*1000))
			out.Sweeps = append(out.Sweeps, SweepPoint{NetworkType: net.name, XName: "beta", X: b, Attack: round(st.attack, 2), Peak: round(st.peak, 2), Duration: round(st.duration, 2), Outbreak: round(st.outbreakP, 2)})
			out.Threshold = append(out.Threshold, ThresholdPoint{NetworkType: net.name, Beta: b, Outbreak: round(st.outbreakP, 2)})
		}
		for _, initI := range []int{1, 3, 5, 10, 20} {
			pp := base
			pp.InitialInfect = initI
			st := replicateScenario(net.mk, pp, replicas, baseSeed+2500+uint64(initI))
			out.Sweeps = append(out.Sweeps, SweepPoint{NetworkType: net.name, XName: "initialInfect", X: float64(initI), Attack: round(st.attack, 2), Peak: round(st.peak, 2), Duration: round(st.duration, 2), Outbreak: round(st.outbreakP, 2)})
		}
	}
	fmt.Fprintln(os.Stderr, "process sweeps done")

	baseER := strategyEval(mkER(0.02), base, "none", 0, replicas, baseSeed+3000)
	baseSF := strategyEval(mkBA(2), base, "none", 0, replicas, baseSeed+3001)

	appendStrat := func(networkType, strategy string, cov float64, st simStats, baseAttack float64) {
		reduction := 0.0
		if baseAttack > 0 {
			reduction = (baseAttack - st.attack) / baseAttack * 100
		}
		out.Strategies = append(out.Strategies, StrategyResult{
			NetworkType:  networkType,
			Strategy:     strategy,
			CoveragePct:  cov * 100,
			AttackRate:   round(st.attack, 2),
			PeakInfected: round(st.peak, 2),
			Duration:     round(st.duration, 2),
			Reduction:    round(reduction, 2),
		})
	}

	appendStrat("erdos-renyi", "none", 0, baseER, baseER.attack)
	appendStrat("erdos-renyi", "random", vaccination, strategyEval(mkER(0.02), base, "random", vaccination, replicas, baseSeed+3010), baseER.attack)
	appendStrat("erdos-renyi", "targeted", vaccination, strategyEval(mkER(0.02), base, "targeted", vaccination, replicas, baseSeed+3020), baseER.attack)
	appendStrat("scale-free", "none", 0, baseSF, baseSF.attack)
	appendStrat("scale-free", "random", vaccination, strategyEval(mkBA(2), base, "random", vaccination, replicas, baseSeed+3030), baseSF.attack)
	appendStrat("scale-free", "targeted", vaccination, strategyEval(mkBA(2), base, "targeted", vaccination, replicas, baseSeed+3040), baseSF.attack)

	thresholds := map[string]float64{}
	for _, net := range []string{"erdos-renyi", "scale-free"} {
		beta := 0.0
		for _, tp := range out.Threshold {
			if tp.NetworkType == net && tp.Outbreak >= 50 {
				beta = tp.Beta
				break
			}
		}
		thresholds[net] = beta
	}

	out.Summary = Summary{
		Thresholds: thresholds,
		MostImportant: []string{
			"Вероятность передачи beta: главный драйвер порога и масштаба вспышки.",
			"Средняя степень узлов: ускоряет рост I(t) и увеличивает peak.",
			"Структура сети: в scale-free хабы усиливают каскад при том же beta.",
		},
		ContainmentAdvice: []string{
			"Таргетированная изоляция/вакцинация хабов эффективнее случайной при одинаковом покрытии.",
			"Снижение beta (ограничение контактов) смещает систему ниже эпидемического порога.",
			"Раннее выявление очага (меньше initialInfect) снижает вероятность крупной вспышки.",
		},
		AccelerationAdvice: []string{
			"Для информационных кампаний: запуск через хабы и высокоцентральные узлы.",
			"Рост локальной связности в целевом сегменте увеличивает скорость каскада.",
			"Повторные контакты повышают эффективный beta распространения информации.",
		},
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		fmt.Fprintf(os.Stderr, "encode json: %v\n", err)
		os.Exit(1)
	}
}
