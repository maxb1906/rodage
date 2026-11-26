package kernels

import (
	"math"
	"testing"
)

func seedU32(n int) []uint32 {
	p := make([]uint32, n)
	x := uint32(2463534242)
	for i := range p {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		p[i] = x | 1
	}
	return p
}

func seedF64(n int) []float64 {
	p := make([]float64, n)
	for i := range p {
		p[i] = 1 + float64(i)/float64(n) + math.Pi*float64(i%7)
	}
	return p
}

// Chaque noyau SIMD doit produire exactement le résultat de son équivalent Go.
func TestKernelsMatchGo(t *testing.T) {
	for _, k := range Available() {
		for _, n := range []int{0, 64, 128, 1024} {
			for _, rounds := range []int{0, 1, 7, 100} {
				a, b := seedU32(n), seedU32(n)
				k.Xorshift(a, rounds)
				XorshiftGo(b, rounds)
				for i := range a {
					if a[i] != b[i] {
						t.Fatalf("%s xorshift n=%d r=%d i=%d : %#x != %#x", k.Level, n, rounds, i, a[i], b[i])
					}
				}
				f, g := seedF64(n), seedF64(n)
				k.Float(f, rounds, DefaultConsts)
				k.FloatRef(g, rounds, DefaultConsts)
				for i := range f {
					if math.Float64bits(f[i]) != math.Float64bits(g[i]) {
						t.Fatalf("%s float n=%d r=%d i=%d : %v != %v", k.Level, n, rounds, i, f[i], g[i])
					}
				}
			}
		}
	}
}

func TestFloatStaysBounded(t *testing.T) {
	k := Best()
	f := seedF64(64)
	k.Float(f, 1_000_000, DefaultConsts)
	for i, v := range f {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e6 {
			t.Fatalf("valeur %d divergente : %v", i, v)
		}
	}
}

func TestBadLengthPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("pas de panique")
		}
	}()
	Best().Xorshift(make([]uint32, 10), 1)
}

func BenchmarkFloat(b *testing.B) {
	for _, k := range Available() {
		b.Run(string(k.Level), func(b *testing.B) {
			p := seedF64(1024)
			b.SetBytes(int64(len(p)) * 8 * 4 * 100) // 4 flops × 100 tours par élément
			for i := 0; i < b.N; i++ {
				k.Float(p, 100, DefaultConsts)
			}
		})
	}
}
