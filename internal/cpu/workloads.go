// Package cpu exécute les charges CPU vérifiées (stress) et les mesures (bench).
package cpu

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/cmplx"

	"github.com/maxb1906/rodage/internal/cpu/kernels"
)

// Workload est un calcul à résultat connu.
type Workload interface {
	Name() string
	// Unit et Ops décrivent la quantité de travail d'une exécution (ex. flops).
	Unit() string
	Ops() float64
	// New crée un état de travail propre à un fil.
	New() Runner
}

// Runner exécute le calcul et le compare au résultat attendu.
type Runner interface {
	// Run renvoie "" si le résultat est correct, sinon une description de l'écart.
	Run() string
}

// ----- SIMD flottant -----

type simdFloat struct {
	k      kernels.Kernel
	n      int
	rounds int
	seed   []float64
	golden []float64
}

// NewSIMDFloat prépare le noyau flottant ; la référence vient du Go pur.
func NewSIMDFloat(k kernels.Kernel) Workload {
	w := &simdFloat{k: k, n: 2048, rounds: 1500}
	w.seed = make([]float64, w.n)
	for i := range w.seed {
		w.seed[i] = 1 + float64(i)/float64(w.n) + math.Pi*float64(i%7)
	}
	w.golden = append([]float64(nil), w.seed...)
	k.FloatRef(w.golden, w.rounds, kernels.DefaultConsts)
	return w
}

func (w *simdFloat) Name() string { return "simd-float-" + string(w.k.Level) }
func (w *simdFloat) Unit() string { return "GFLOPS" }
func (w *simdFloat) Ops() float64 { return float64(w.n) * float64(w.rounds) * 4 }
func (w *simdFloat) New() Runner  { return &simdFloatRun{w: w, buf: make([]float64, w.n)} }

type simdFloatRun struct {
	w   *simdFloat
	buf []float64
}

func (r *simdFloatRun) Run() string {
	copy(r.buf, r.w.seed)
	r.w.k.Float(r.buf, r.w.rounds, kernels.DefaultConsts)
	for i, v := range r.buf {
		if math.Float64bits(v) != math.Float64bits(r.w.golden[i]) {
			return fmt.Sprintf("élément %d : %x au lieu de %x", i, math.Float64bits(v), math.Float64bits(r.w.golden[i]))
		}
	}
	return ""
}

// ----- SIMD entier -----

type simdInt struct {
	k      kernels.Kernel
	n      int
	rounds int
	seed   []uint32
	golden []uint32
}

// NewSIMDInt prépare le noyau entier (xorshift32).
func NewSIMDInt(k kernels.Kernel) Workload {
	w := &simdInt{k: k, n: 4096, rounds: 600}
	w.seed = make([]uint32, w.n)
	x := uint32(2463534242)
	for i := range w.seed {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		w.seed[i] = x | 1
	}
	w.golden = append([]uint32(nil), w.seed...)
	kernels.XorshiftGo(w.golden, w.rounds)
	return w
}

func (w *simdInt) Name() string { return "simd-int-" + string(w.k.Level) }
func (w *simdInt) Unit() string { return "Gops" }
func (w *simdInt) Ops() float64 { return float64(w.n) * float64(w.rounds) * 6 }
func (w *simdInt) New() Runner  { return &simdIntRun{w: w, buf: make([]uint32, w.n)} }

type simdIntRun struct {
	w   *simdInt
	buf []uint32
}

func (r *simdIntRun) Run() string {
	copy(r.buf, r.w.seed)
	r.w.k.Xorshift(r.buf, r.w.rounds)
	for i, v := range r.buf {
		if v != r.w.golden[i] {
			return fmt.Sprintf("mot %d : %#x au lieu de %#x", i, v, r.w.golden[i])
		}
	}
	return ""
}

// ----- FFT -----

const fftN = 4096

type fftWork struct {
	seed   []complex128
	golden []complex128
}

func fft(a []complex128, inverse bool) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		ang := 2 * math.Pi / float64(size)
		if !inverse {
			ang = -ang
		}
		wl := cmplx.Rect(1, ang)
		for i := 0; i < n; i += size {
			w := complex(1, 0)
			for k := 0; k < size/2; k++ {
				u, v := a[i+k], a[i+k+size/2]*w
				a[i+k], a[i+k+size/2] = u+v, u-v
				w *= wl
			}
		}
	}
	if inverse {
		inv := complex(1/float64(n), 0)
		for i := range a {
			a[i] *= inv
		}
	}
}

// NewFFT prépare une FFT aller-retour de 4096 points.
func NewFFT() (Workload, error) {
	w := &fftWork{seed: make([]complex128, fftN)}
	for i := range w.seed {
		t := float64(i)
		w.seed[i] = complex(math.Sin(t*0.05)+0.5*math.Cos(t*0.31), math.Sin(t*0.013))
	}
	g := w.compute(make([]complex128, fftN))
	// Contrôle indépendant : l'aller-retour doit redonner le signal d'origine.
	for i := range g {
		if cmplx.Abs(g[i]-w.seed[i]) > 1e-9 {
			return nil, fmt.Errorf("FFT : référence incohérente à l'indice %d", i)
		}
	}
	g2 := w.compute(make([]complex128, fftN))
	for i := range g {
		if g[i] != g2[i] {
			return nil, fmt.Errorf("FFT : référence non reproductible")
		}
	}
	w.golden = g
	return w, nil
}

func (w *fftWork) compute(buf []complex128) []complex128 {
	copy(buf, w.seed)
	fft(buf, false)
	fft(buf, true)
	return buf
}

func (w *fftWork) Name() string { return "fft" }
func (w *fftWork) Unit() string { return "GFLOPS" }
func (w *fftWork) Ops() float64 { return 2 * 5 * fftN * 12 } // 5 N log2 N par transformée
func (w *fftWork) New() Runner  { return &fftRun{w: w, buf: make([]complex128, fftN)} }

type fftRun struct {
	w   *fftWork
	buf []complex128
}

func (r *fftRun) Run() string {
	r.w.compute(r.buf)
	for i := range r.buf {
		if r.buf[i] != r.w.golden[i] {
			return fmt.Sprintf("indice %d : %v au lieu de %v", i, r.buf[i], r.w.golden[i])
		}
	}
	return ""
}

// ----- Matrices -----

const matN = 96

type matWork struct {
	a, b   []float64
	golden []float64
}

func matmul(a, b, c []float64) {
	for i := range c {
		c[i] = 0
	}
	for i := 0; i < matN; i++ {
		for k := 0; k < matN; k++ {
			aik := a[i*matN+k]
			row := c[i*matN : (i+1)*matN]
			brow := b[k*matN : (k+1)*matN]
			for j := range row {
				row[j] = math.FMA(aik, brow[j], row[j])
			}
		}
	}
}

// NewMatrix prépare un produit de matrices 96×96.
func NewMatrix() (Workload, error) {
	w := &matWork{a: make([]float64, matN*matN), b: make([]float64, matN*matN)}
	for i := range w.a {
		w.a[i] = float64(i%17)/17 - 0.5
		w.b[i] = float64(i%23)/23 - 0.25
	}
	// Contrôle indépendant : A × I = A.
	id := make([]float64, matN*matN)
	for i := 0; i < matN; i++ {
		id[i*matN+i] = 1
	}
	c := make([]float64, matN*matN)
	matmul(w.a, id, c)
	for i := range c {
		if c[i] != w.a[i] {
			return nil, fmt.Errorf("matrices : A×I ≠ A")
		}
	}
	w.golden = make([]float64, matN*matN)
	matmul(w.a, w.b, w.golden)
	return w, nil
}

func (w *matWork) Name() string { return "matrix" }
func (w *matWork) Unit() string { return "GFLOPS" }
func (w *matWork) Ops() float64 { return 2 * matN * matN * matN }
func (w *matWork) New() Runner  { return &matRun{w: w, c: make([]float64, matN*matN)} }

type matRun struct {
	w *matWork
	c []float64
}

func (r *matRun) Run() string {
	matmul(r.w.a, r.w.b, r.c)
	for i := range r.c {
		if math.Float64bits(r.c[i]) != math.Float64bits(r.w.golden[i]) {
			return fmt.Sprintf("élément %d : %v au lieu de %v", i, r.c[i], r.w.golden[i])
		}
	}
	return ""
}

// ----- Hachage -----

const hashLen = 1 << 20

type hashWork struct {
	data   []byte
	golden [32]byte
}

// sha256Vector : empreinte SHA-256 de référence (FIPS 180-2) de "abc".
var sha256Vector = [32]byte{0xba, 0x78, 0x16, 0xbf, 0x8f, 0x01, 0xcf, 0xea, 0x41, 0x41, 0x40, 0xde, 0x5d, 0xae, 0x22, 0x23, 0xb0, 0x03, 0x61, 0xa3, 0x96, 0x17, 0x7a, 0x9c, 0xb4, 0x10, 0xff, 0x61, 0xf2, 0x00, 0x15, 0xad}

// NewHash prépare un SHA-256 sur 1 Mio.
func NewHash() (Workload, error) {
	if sha256.Sum256([]byte("abc")) != sha256Vector {
		return nil, fmt.Errorf("SHA-256 : vecteur de test FIPS incorrect")
	}
	w := &hashWork{data: make([]byte, hashLen)}
	x := uint64(0x9E3779B97F4A7C15)
	for i := range w.data {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		w.data[i] = byte(x)
	}
	w.golden = sha256.Sum256(w.data)
	return w, nil
}

func (w *hashWork) Name() string { return "sha256" }
func (w *hashWork) Unit() string { return "Go/s" }
func (w *hashWork) Ops() float64 { return hashLen }
func (w *hashWork) New() Runner  { return &hashRun{w: w} }

type hashRun struct{ w *hashWork }

func (r *hashRun) Run() string {
	if h := sha256.Sum256(r.w.data); h != r.w.golden {
		return fmt.Sprintf("empreinte %x au lieu de %x", h[:8], r.w.golden[:8])
	}
	return ""
}

// DefaultWorkloads renvoie l'ensemble des calculs vérifiés pour un noyau.
func DefaultWorkloads(k kernels.Kernel) ([]Workload, error) {
	ws := []Workload{NewSIMDFloat(k), NewSIMDInt(k)}
	for _, f := range []func() (Workload, error){NewFFT, NewMatrix, NewHash} {
		w, err := f()
		if err != nil {
			return nil, err
		}
		ws = append(ws, w)
	}
	return ws, nil
}
