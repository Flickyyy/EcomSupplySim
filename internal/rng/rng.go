// Package rng implements the xoshiro256** PRNG and distribution samplers.
//
// PRNG: xoshiro256** by David Blackman & Sebastiano Vigna (2018).
// Period: 2^256 - 1. State: 256 bits (4 x uint64).
// Seeding: SplitMix64 expands a single uint64 seed into the full state.
//
// All distributions are derived from the uniform output:
//   - Exponential : inverse-transform  F^-1(u) = -ln(1-u)/lambda
//   - Normal      : Box-Muller transform
//   - LogNormal   : exp(Normal)
//   - Bernoulli   : direct comparison U < p
//   - Uniform(a,b): linear scaling a + U*(b-a)
//   - Poisson     : Knuth counting algorithm (for lambda <= 30)
package rng

import "math"

// ---------- SplitMix64 (seeder) ----------

func splitMix64(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// ---------- Xoshiro256** ----------

// Xoshiro256ss is the xoshiro256** all-purpose PRNG.
type Xoshiro256ss struct {
	s0, s1, s2, s3 uint64
}

// New creates a Xoshiro256** generator seeded from a single uint64.
func New(seed uint64) *Xoshiro256ss {
	st := seed
	return &Xoshiro256ss{
		s0: splitMix64(&st),
		s1: splitMix64(&st),
		s2: splitMix64(&st),
		s3: splitMix64(&st),
	}
}

func rotl(x uint64, k uint) uint64 {
	return (x << k) | (x >> (64 - k))
}

// Uint64 returns a pseudo-random uint64.
func (r *Xoshiro256ss) Uint64() uint64 {
	result := rotl(r.s1*5, 7) * 9
	t := r.s1 << 17
	r.s2 ^= r.s0
	r.s3 ^= r.s1
	r.s1 ^= r.s2
	r.s0 ^= r.s3
	r.s2 ^= t
	r.s3 = rotl(r.s3, 45)
	return result
}

// Float64 returns a uniformly distributed float64 in [0, 1).
func (r *Xoshiro256ss) Float64() float64 {
	return float64(r.Uint64()>>11) / (1 << 53)
}

// ---------- Distribution samplers ----------

// Exponential returns X ~ Exp(lambda). Inverse-transform method.
func (r *Xoshiro256ss) Exponential(lambda float64) float64 {
	return -math.Log1p(-r.Float64()) / lambda
}

// Normal returns X ~ N(mu, sigma) via Box-Muller transform.
func (r *Xoshiro256ss) Normal(mu, sigma float64) float64 {
	u1 := r.Float64()
	u2 := r.Float64()
	z := math.Sqrt(-2*math.Log1p(-u1)) * math.Cos(2*math.Pi*u2)
	return mu + sigma*z
}

// NormalPositive returns max(floor, N(mu, sigma)).
func (r *Xoshiro256ss) NormalPositive(mu, sigma, floor float64) float64 {
	x := r.Normal(mu, sigma)
	if x < floor {
		return floor
	}
	return x
}

// LogNormal returns X ~ LogNormal(mu, sigma) = exp(N(mu, sigma)).
func (r *Xoshiro256ss) LogNormal(mu, sigma float64) float64 {
	return math.Exp(r.Normal(mu, sigma))
}

// Bernoulli returns true with probability p.
func (r *Xoshiro256ss) Bernoulli(p float64) bool {
	return r.Float64() < p
}

// UniformFloat returns X ~ U(a, b).
func (r *Xoshiro256ss) UniformFloat(a, b float64) float64 {
	return a + r.Float64()*(b-a)
}

// UniformInt returns a uniform random integer in [a, b].
func (r *Xoshiro256ss) UniformInt(a, b int) int {
	n := b - a + 1
	return a + int(r.Float64()*float64(n))
}

// Poisson returns X ~ Poisson(lambda) via Knuth counting algorithm.
func (r *Xoshiro256ss) Poisson(lambda float64) int {
	L := math.Exp(-lambda)
	k := 0
	p := 1.0
	for {
		k++
		p *= r.Float64()
		if p <= L {
			break
		}
	}
	return k - 1
}
