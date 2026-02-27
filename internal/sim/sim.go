package sim

import (
	"EcomSupplySim/internal/rng"
	"math"
)

// Status is the terminal status of an order.
type Status int

const (
	Delivered Status = iota
	Cancelled
	Restocked
	Disposed
)

func (s Status) String() string {
	return [...]string{"DELIVERED", "CANCELLED", "RESTOCKED", "DISPOSED"}[s]
}

// Priority represents delivery priority class.
type Priority int

const (
	Standard Priority = iota
	SameDay
	Express
)

func (p Priority) String() string {
	return [...]string{"standard", "same-day", "express"}[p]
}

// SLAMinutes returns the SLA deadline in minutes.
func (p Priority) SLAMinutes() float64 {
	return [...]float64{1440, 240, 60}[p]
}

// Config holds all simulation parameters.
type Config struct {
	NStores       int
	NCouriers     int
	SlotsPerStore int
	QMax          int
	MuAsm         float64
	SigmaAsm      float64
	PDefect       float64
	MeanSpeed     float64
	SigmaTraffic  float64
	LambdaBreak   float64
	MuRepair      float64
	PAbsent       float64
	PRetBase      float64
	Alpha         float64
	MaxAttempts   int
	BetaSLA       float64
	PStoreReturn  float64
	MuInspect     float64
	PDispose      float64
	MinDist       float64
	MaxDist       float64
	InitialStock  int
	LambdaMult    float64
}

// DefaultConfig returns the baseline scenario parameters.
func DefaultConfig() Config {
	return Config{
		NStores: 4, NCouriers: 15, SlotsPerStore: 3, QMax: 50,
		MuAsm: 15.0, SigmaAsm: 4.0, PDefect: 0.02,
		MeanSpeed: 25.0, SigmaTraffic: 0.35, LambdaBreak: 0.1, MuRepair: 30.0,
		PAbsent: 0.15, PRetBase: 0.08, Alpha: 0.5, MaxAttempts: 2, BetaSLA: 1.5,
		PStoreReturn: 0.60, MuInspect: 10.0, PDispose: 0.25,
		MinDist: 0.5, MaxDist: 8.0, InitialStock: 200, LambdaMult: 1.0,
	}
}

// PeakConfig - Black Friday scenario.
func PeakConfig() Config {
	c := DefaultConfig()
	c.LambdaMult = 2.5
	c.NCouriers = 25
	return c
}

// ChaosConfig - bad weather / mass breakdowns.
func ChaosConfig() Config {
	c := DefaultConfig()
	c.LambdaBreak *= 5
	c.SigmaTraffic *= 2
	return c
}

// UnderstaffedConfig - skeleton crew.
func UnderstaffedConfig() Config {
	c := DefaultConfig()
	c.NCouriers = 8
	return c
}

// lambdaBase returns base orders/hour for a given hour.
func lambdaBase(hour int) float64 {
	switch {
	case hour < 9:
		return 2
	case hour < 12:
		return 8
	case hour < 14:
		return 15
	case hour < 19:
		return 6
	case hour < 22:
		return 15
	default:
		return 3
	}
}

// Order represents a single customer order.
type Order struct {
	CreatedAt    float64
	Priority     Priority
	SLA          float64
	Status       Status
	DeliveryTime float64
	ReturnTime   float64
	QueueWait    float64
	SLABreached  bool
}

// PriorityStats accumulates per-priority metrics.
type PriorityStats struct {
	Count       int
	Delivered   int
	SLABreach   int
	DeliverySum float64
}

// Stats accumulates metrics across all simulated days.
type Stats struct {
	TotalOrders      int
	ByStatus         [4]int
	ByPriority       [3]PriorityStats
	DeliverySum      float64
	DeliverySum2     float64
	DeliveryN        int
	ReturnSum        float64
	ReturnN          int
	QueueWaitSum     float64
	AssemblyBusy     float64
	AssemblyCapacity float64
	CourierBusy      float64
	CourierCapacity  float64
	SLABreaches      int
}

// Record adds one order's outcome to the stats.
func (s *Stats) Record(o *Order) {
	s.TotalOrders++
	s.ByStatus[o.Status]++
	pi := int(o.Priority)
	s.ByPriority[pi].Count++
	if o.Status == Delivered {
		s.DeliverySum += o.DeliveryTime
		s.DeliverySum2 += o.DeliveryTime * o.DeliveryTime
		s.DeliveryN++
		s.ByPriority[pi].Delivered++
		s.ByPriority[pi].DeliverySum += o.DeliveryTime
	}
	if o.ReturnTime > 0 {
		s.ReturnSum += o.ReturnTime
		s.ReturnN++
	}
	s.QueueWaitSum += o.QueueWait
	if o.SLABreached {
		s.SLABreaches++
		s.ByPriority[pi].SLABreach++
	}
}

func (s *Stats) AvgDelivery() float64 {
	if s.DeliveryN == 0 {
		return 0
	}
	return s.DeliverySum / float64(s.DeliveryN)
}

func (s *Stats) StdDelivery() float64 {
	if s.DeliveryN < 2 {
		return 0
	}
	n := float64(s.DeliveryN)
	mean := s.DeliverySum / n
	v := s.DeliverySum2/n - mean*mean
	if v < 0 {
		v = 0
	}
	return math.Sqrt(v)
}

func (s *Stats) AvgReturn() float64 {
	if s.ReturnN == 0 {
		return 0
	}
	return s.ReturnSum / float64(s.ReturnN)
}

func (s *Stats) CourierUtil() float64 {
	if s.CourierCapacity == 0 {
		return 0
	}
	return s.CourierBusy / s.CourierCapacity * 100
}

func (s *Stats) AssemblyUtil() float64 {
	if s.AssemblyCapacity == 0 {
		return 0
	}
	return s.AssemblyBusy / s.AssemblyCapacity * 100
}

func (s *Stats) AvgQueueWait() float64 {
	if s.TotalOrders == 0 {
		return 0
	}
	return s.QueueWaitSum / float64(s.TotalOrders)
}

// dayState holds transient resource tracking for one simulated day.
type dayState struct {
	asmFree  [][]float64
	courFree []float64
	stock    []int
	asmBusy  float64
	courBusy float64
}

func newDayState(cfg *Config) *dayState {
	ds := &dayState{
		asmFree:  make([][]float64, cfg.NStores),
		courFree: make([]float64, cfg.NCouriers),
		stock:    make([]int, cfg.NStores),
	}
	for i := range ds.asmFree {
		ds.asmFree[i] = make([]float64, cfg.SlotsPerStore)
	}
	for i := range ds.stock {
		ds.stock[i] = cfg.InitialStock
	}
	return ds
}

// Engine drives the simulation and accumulates statistics.
type Engine struct {
	Cfg   Config
	Stats Stats
}

// NewEngine creates a simulation engine.
func NewEngine(cfg Config) *Engine {
	return &Engine{Cfg: cfg}
}

// SimulateDay runs one 24-hour day and returns number of orders processed.
func (e *Engine) SimulateDay(r *rng.Xoshiro256ss) int {
	cfg := &e.Cfg
	dayMin := 1440.0
	ds := newDayState(cfg)

	// B1: Generate orders via Poisson process
	type entry struct {
		t   float64
		pri Priority
	}
	var orders []entry
	t := 0.0
	for t < dayMin {
		hour := int(t / 60)
		if hour > 23 {
			hour = 23
		}
		lambda := lambdaBase(hour) * cfg.LambdaMult / 60.0
		interval := r.Exponential(lambda)
		t += interval
		if t >= dayMin {
			break
		}
		u := r.Float64()
		var pri Priority
		switch {
		case u < 0.05:
			pri = Express
		case u < 0.25:
			pri = SameDay
		default:
			pri = Standard
		}
		orders = append(orders, entry{t: t, pri: pri})
	}

	// Process each order sequentially
	for _, oe := range orders {
		o := Order{
			CreatedAt: oe.t,
			Priority:  oe.pri,
			SLA:       oe.pri.SLAMinutes(),
		}
		e.processOrder(r, &o, ds)
		e.Stats.Record(&o)
	}

	// Update capacity counters
	e.Stats.AssemblyBusy += ds.asmBusy
	e.Stats.AssemblyCapacity += float64(cfg.NStores*cfg.SlotsPerStore) * dayMin
	e.Stats.CourierBusy += ds.courBusy
	e.Stats.CourierCapacity += float64(cfg.NCouriers) * dayMin
	return len(orders)
}

// processOrder simulates an order through B2->B3->B4->B5->(B6).
func (e *Engine) processOrder(r *rng.Xoshiro256ss, o *Order, ds *dayState) {
	cfg := &e.Cfg

	// B2: Dispatch - find store with earliest slot and stock > 0
	bestStore := -1
	bestSlot := -1
	bestReady := math.Inf(1)
	for s := 0; s < cfg.NStores; s++ {
		if ds.stock[s] <= 0 {
			continue
		}
		for j := 0; j < cfg.SlotsPerStore; j++ {
			ready := math.Max(o.CreatedAt, ds.asmFree[s][j])
			if ready < bestReady {
				bestReady = ready
				bestStore = s
				bestSlot = j
			}
		}
	}
	if bestStore < 0 {
		o.Status = Cancelled
		return
	}
	ds.stock[bestStore]--

	// B3: Assembly - N(15, 4) * weight
	asmStart := math.Max(o.CreatedAt, ds.asmFree[bestStore][bestSlot])
	o.QueueWait = asmStart - o.CreatedAt
	nItems := r.UniformInt(1, 5)
	w := 1.0 + 0.05*float64(nItems-1)
	asmTime := r.NormalPositive(cfg.MuAsm, cfg.SigmaAsm, 1.0) * w
	asmEnd := asmStart + asmTime
	ds.asmFree[bestStore][bestSlot] = asmEnd
	ds.asmBusy += asmTime

	// Defect check: Bern(0.02)
	if r.Bernoulli(cfg.PDefect) {
		inspTime := r.Exponential(1.0 / cfg.MuInspect)
		o.ReturnTime = inspTime
		if r.Bernoulli(cfg.PDispose) {
			o.Status = Disposed
		} else {
			o.Status = Restocked
			ds.stock[bestStore]++
		}
		return
	}

	// B4: Courier delivery
	bestCour := 0
	for j := 1; j < cfg.NCouriers; j++ {
		if ds.courFree[j] < ds.courFree[bestCour] {
			bestCour = j
		}
	}
	delivStart := math.Max(asmEnd, ds.courFree[bestCour])
	dist := r.UniformFloat(cfg.MinDist, cfg.MaxDist)
	traffic := r.LogNormal(0, cfg.SigmaTraffic)
	travelTime := (dist / cfg.MeanSpeed) * 60.0 * traffic

	// Breakdown probability proportional to trip duration
	pBreak := 1.0 - math.Exp(-cfg.LambdaBreak*travelTime/1440.0)
	if r.Bernoulli(pBreak) {
		repairTime := r.Exponential(1.0 / cfg.MuRepair)
		travelTime += repairTime
	}

	delivEnd := delivStart + travelTime
	totalTime := delivEnd - o.CreatedAt
	if totalTime > o.SLA {
		o.SLABreached = true
	}

	// B5: Customer interaction
	customerTime := delivEnd
	delivered := false
	returnOrder := false
	expectedTime := cfg.MuAsm + (dist/cfg.MeanSpeed)*60.0

	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if r.Bernoulli(cfg.PAbsent) {
			waitTime := r.UniformFloat(60, 240)
			customerTime += waitTime
			totalTime = customerTime - o.CreatedAt
			if totalTime > o.SLA {
				o.SLABreached = true
			}
			continue
		}
		delay := math.Max(0, totalTime-expectedTime)
		delayRatio := delay / math.Max(expectedTime, 1)
		slaFactor := 1.0
		if o.SLABreached {
			slaFactor = cfg.BetaSLA
		}
		pRet := cfg.PRetBase * (1 + cfg.Alpha*delayRatio) * slaFactor
		if pRet > 1.0 {
			pRet = 1.0
		}
		if r.Bernoulli(pRet) {
			returnOrder = true
			break
		}
		delivered = true
		break
	}

	if delivered {
		o.Status = Delivered
		o.DeliveryTime = customerTime - o.CreatedAt
		ds.courFree[bestCour] = customerTime
		ds.courBusy += customerTime - delivStart
	} else if returnOrder {
		// B6: Return logistics
		returnTraffic := r.LogNormal(0, cfg.SigmaTraffic)
		returnTravel := (dist / cfg.MeanSpeed) * 60.0 * returnTraffic
		inspTime := r.Exponential(1.0 / cfg.MuInspect)
		o.ReturnTime = returnTravel + inspTime
		if r.Bernoulli(cfg.PDispose) {
			o.Status = Disposed
		} else {
			o.Status = Restocked
			ds.stock[bestStore]++
		}
		courEnd := customerTime + returnTravel
		ds.courFree[bestCour] = courEnd
		ds.courBusy += courEnd - delivStart
	} else {
		o.Status = Cancelled
		ds.courFree[bestCour] = customerTime
		ds.courBusy += customerTime - delivStart
	}
}
