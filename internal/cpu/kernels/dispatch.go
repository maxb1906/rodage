package kernels

import (
	"fmt"
	"unsafe"
)

// Level désigne un jeu d'instructions SIMD.
type Level string

const (
	LevelGo     Level = "go"     // Go pur, aucune instruction SIMD explicite
	LevelSSE2   Level = "sse2"   // x64
	LevelAVX2   Level = "avx2"   // x64 (AVX2 + FMA)
	LevelAVX512 Level = "avx512" // x64 (AVX-512F)
	LevelNEON   Level = "neon"   // ARM64
)

// Consts sont les constantes des noyaux flottants.
// a*ainv ≈ 1 et c ≈ -b*ainv : la valeur reste bornée et une erreur ne
// s'efface pas au fil des tours (l'application n'est pas contractante).
type Consts struct{ A, Ainv, B, C float64 }

// DefaultConsts est le jeu de constantes utilisé par Rodage.
var DefaultConsts = func() Consts {
	a := 1.0000001192092896
	ainv := 1 / a
	b := 0.3183098861837907
	return Consts{A: a, Ainv: ainv, B: b, C: -b * ainv}
}()

// Kernel est un ensemble de noyaux pour un niveau donné.
type Kernel struct {
	Level Level
	// Fused indique si le noyau flottant utilise un FMA (un arrondi).
	Fused    bool
	xorshift func(p *uint32, n, rounds int)
	float    func(p *float64, n, rounds int, a, ainv, b, c float64)
}

func checkLen(n int) {
	if n%Block != 0 {
		panic(fmt.Sprintf("kernels: longueur %d non multiple de %d", n, Block))
	}
}

// Xorshift applique le noyau entier sur p.
func (k Kernel) Xorshift(p []uint32, rounds int) {
	checkLen(len(p))
	if len(p) == 0 {
		return
	}
	if k.xorshift == nil {
		XorshiftGo(p, rounds)
		return
	}
	k.xorshift(unsafe.SliceData(p), len(p), rounds)
}

// Float applique le noyau flottant sur p.
func (k Kernel) Float(p []float64, rounds int, c Consts) {
	checkLen(len(p))
	if len(p) == 0 {
		return
	}
	if k.float == nil {
		k.FloatRef(p, rounds, c)
		return
	}
	k.float(unsafe.SliceData(p), len(p), rounds, c.A, c.Ainv, c.B, c.C)
}

// FloatRef est l'équivalent Go pur, au bit près, du noyau flottant.
func (k Kernel) FloatRef(p []float64, rounds int, c Consts) {
	if k.Fused {
		FMAGo(p, rounds, c.A, c.Ainv, c.B, c.C)
	} else {
		MulAddGo(p, rounds, c.A, c.Ainv, c.B, c.C)
	}
}

// GoKernel renvoie les noyaux en Go pur (FMA logiciel).
func GoKernel() Kernel { return Kernel{Level: LevelGo, Fused: true} }

// Available liste les niveaux utilisables sur cette machine, du plus simple
// au plus avancé. Le premier est toujours LevelGo.
func Available() []Kernel { return append([]Kernel{GoKernel()}, available()...) }

// Best renvoie le noyau le plus avancé disponible.
func Best() Kernel {
	a := Available()
	return a[len(a)-1]
}

// ByLevel renvoie le noyau demandé s'il est disponible.
func ByLevel(l Level) (Kernel, bool) {
	for _, k := range Available() {
		if k.Level == l {
			return k, true
		}
	}
	return Kernel{}, false
}

//go:generate sh -c "cd ../../../asm && go run ./gen -out ../internal/cpu/kernels/kernels_amd64.s -stubs ../internal/cpu/kernels/kernels_amd64.go -pkg kernels"
