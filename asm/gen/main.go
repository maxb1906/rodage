// Générateur avo des noyaux SIMD amd64 de Rodage.
//
// Usage (depuis le dossier asm/) :
//
//	go run ./gen -out ../internal/cpu/kernels/kernels_amd64.s -stubs ../internal/cpu/kernels/kernels_amd64.go -pkg kernels
//
// Chaque noyau a un équivalent en Go pur (internal/cpu/kernels/kernels_go.go)
// qui produit un résultat identique au bit près ; les tests le vérifient.
package main

import (
	"fmt"

	. "github.com/mmcloughlin/avo/build"
	. "github.com/mmcloughlin/avo/operand"
	"github.com/mmcloughlin/avo/reg"
)

func main() {
	ConstraintExpr("amd64,!purego")

	genXorshiftSSE2()
	genXorshiftAVX2()
	genXorshiftAVX512()
	genMulAddSSE2()
	genFMAAVX2()
	genFMAAVX512()

	Generate()
}

// ----- Xorshift32 (entiers) -----
// Pour chaque mot x : x ^= x<<13 ; x ^= x>>17 ; x ^= x<<5, répété rounds fois.

func xorshiftHeader(name, isa string) (reg.Register, reg.Register, reg.Register) {
	TEXT(name, NOSPLIT, "func(p *uint32, n int, rounds int)")
	Doc(fmt.Sprintf("%s applique xorshift32 %s fois ; n multiple de 64 (%s).", name, "rounds", isa))
	p := Load(Param("p"), GP64())
	n := Load(Param("n"), GP64())
	r := Load(Param("rounds"), GP64())
	return p, n, r
}

func genXorshiftSSE2() {
	p, n, rounds := xorshiftHeader("xorshiftSSE2", "SSE2")
	// Bloc de 8 registres XMM = 32 mots ; 2 blocs par tranche de 64.
	const regs = 8
	x := make([]reg.VecVirtual, regs)
	t := make([]reg.VecVirtual, regs)
	for i := range x {
		x[i] = XMM()
		t[i] = XMM()
	}
	SHRQ(U8(5), n) // nombre de blocs de 32 mots
	Label("xs_sse2_block")
	CMPQ(n, U8(0))
	JE(LabelRef("xs_sse2_done"))
	for i := range x {
		MOVOU(Mem{Base: p, Disp: 16 * i}, x[i])
	}
	k := GP64()
	MOVQ(rounds, k)
	TESTQ(k, k)
	JE(LabelRef("xs_sse2_store"))
	Label("xs_sse2_round")
	for _, step := range []struct {
		left bool
		sh   uint8
	}{{true, 13}, {false, 17}, {true, 5}} {
		for i := range x {
			MOVO(x[i], t[i])
			if step.left {
				PSLLL(U8(step.sh), t[i])
			} else {
				PSRLL(U8(step.sh), t[i])
			}
			PXOR(t[i], x[i])
		}
	}
	DECQ(k)
	JNE(LabelRef("xs_sse2_round"))
	Label("xs_sse2_store")
	for i := range x {
		MOVOU(x[i], Mem{Base: p, Disp: 16 * i})
	}
	ADDQ(U32(16*regs), p)
	DECQ(n)
	JMP(LabelRef("xs_sse2_block"))
	Label("xs_sse2_done")
	RET()
}

func genXorshiftAVX2() {
	p, n, rounds := xorshiftHeader("xorshiftAVX2", "AVX2")
	const regs = 8 // 8 YMM = 64 mots
	x := make([]reg.VecVirtual, regs)
	t := make([]reg.VecVirtual, regs)
	for i := range x {
		x[i] = YMM()
		t[i] = YMM()
	}
	SHRQ(U8(6), n)
	Label("xs_avx2_block")
	CMPQ(n, U8(0))
	JE(LabelRef("xs_avx2_done"))
	for i := range x {
		VMOVDQU(Mem{Base: p, Disp: 32 * i}, x[i])
	}
	k := GP64()
	MOVQ(rounds, k)
	TESTQ(k, k)
	JE(LabelRef("xs_avx2_store"))
	Label("xs_avx2_round")
	for _, step := range []struct {
		left bool
		sh   uint8
	}{{true, 13}, {false, 17}, {true, 5}} {
		for i := range x {
			if step.left {
				VPSLLD(U8(step.sh), x[i], t[i])
			} else {
				VPSRLD(U8(step.sh), x[i], t[i])
			}
			VPXOR(t[i], x[i], x[i])
		}
	}
	DECQ(k)
	JNE(LabelRef("xs_avx2_round"))
	Label("xs_avx2_store")
	for i := range x {
		VMOVDQU(x[i], Mem{Base: p, Disp: 32 * i})
	}
	ADDQ(U32(32*regs), p)
	DECQ(n)
	JMP(LabelRef("xs_avx2_block"))
	Label("xs_avx2_done")
	VZEROUPPER()
	RET()
}

func genXorshiftAVX512() {
	p, n, rounds := xorshiftHeader("xorshiftAVX512", "AVX-512F")
	const regs = 4 // 4 ZMM = 64 mots
	x := make([]reg.VecVirtual, regs)
	t := make([]reg.VecVirtual, regs)
	for i := range x {
		x[i] = ZMM()
		t[i] = ZMM()
	}
	SHRQ(U8(6), n)
	Label("xs_avx512_block")
	CMPQ(n, U8(0))
	JE(LabelRef("xs_avx512_done"))
	for i := range x {
		VMOVDQU32(Mem{Base: p, Disp: 64 * i}, x[i])
	}
	k := GP64()
	MOVQ(rounds, k)
	TESTQ(k, k)
	JE(LabelRef("xs_avx512_store"))
	Label("xs_avx512_round")
	for _, step := range []struct {
		left bool
		sh   uint8
	}{{true, 13}, {false, 17}, {true, 5}} {
		for i := range x {
			if step.left {
				VPSLLD(U8(step.sh), x[i], t[i])
			} else {
				VPSRLD(U8(step.sh), x[i], t[i])
			}
			VPXORD(t[i], x[i], x[i])
		}
	}
	DECQ(k)
	JNE(LabelRef("xs_avx512_round"))
	Label("xs_avx512_store")
	for i := range x {
		VMOVDQU32(x[i], Mem{Base: p, Disp: 64 * i})
	}
	ADDQ(U32(64*regs), p)
	DECQ(n)
	JMP(LabelRef("xs_avx512_block"))
	Label("xs_avx512_done")
	VZEROUPPER()
	RET()
}

// ----- Flottants -----
// Chaque tour : y = y*a + b puis y = y*ainv + c.
// SSE2 : multiplication et addition séparées (deux arrondis).
// AVX2/AVX-512 : FMA (un seul arrondi), comme math.FMA.

const floatSig = "func(p *float64, n int, rounds int, a, ainv, b, c float64)"

func genMulAddSSE2() {
	TEXT("mulAddSSE2", NOSPLIT, floatSig)
	Doc("mulAddSSE2 : y = y*a+b ; y = y*ainv+c (arrondis séparés) ; n multiple de 64.")
	p := Load(Param("p"), GP64())
	n := Load(Param("n"), GP64())
	rounds := Load(Param("rounds"), GP64())
	consts := make([]reg.Register, 4)
	for i, name := range []string{"a", "ainv", "b", "c"} {
		_ = i
		consts[i] = Load(Param(name), XMM())
		UNPCKLPD(consts[i], consts[i])
	}
	const regs = 8 // 8 XMM = 16 flottants
	y := make([]reg.VecVirtual, regs)
	for i := range y {
		y[i] = XMM()
	}
	SHRQ(U8(4), n)
	Label("ma_sse2_block")
	CMPQ(n, U8(0))
	JE(LabelRef("ma_sse2_done"))
	for i := range y {
		MOVUPD(Mem{Base: p, Disp: 16 * i}, y[i])
	}
	k := GP64()
	MOVQ(rounds, k)
	TESTQ(k, k)
	JE(LabelRef("ma_sse2_store"))
	Label("ma_sse2_round")
	for i := range y {
		MULPD(consts[0], y[i])
	}
	for i := range y {
		ADDPD(consts[2], y[i])
	}
	for i := range y {
		MULPD(consts[1], y[i])
	}
	for i := range y {
		ADDPD(consts[3], y[i])
	}
	DECQ(k)
	JNE(LabelRef("ma_sse2_round"))
	Label("ma_sse2_store")
	for i := range y {
		MOVUPD(y[i], Mem{Base: p, Disp: 16 * i})
	}
	ADDQ(U32(16*regs), p)
	DECQ(n)
	JMP(LabelRef("ma_sse2_block"))
	Label("ma_sse2_done")
	RET()
}

func genFMAAVX2() {
	TEXT("fmaAVX2", NOSPLIT, floatSig)
	Doc("fmaAVX2 : y = fma(y,a,b) ; y = fma(y,ainv,c) ; n multiple de 64.")
	p := Load(Param("p"), GP64())
	n := Load(Param("n"), GP64())
	rounds := Load(Param("rounds"), GP64())
	consts := make([]reg.Register, 4)
	for i, name := range []string{"a", "ainv", "b", "c"} {
		_ = i
		consts[i] = YMM()
		VBROADCASTSD(Load(Param(name), XMM()), consts[i])
	}
	const regs = 8 // 8 YMM = 32 flottants
	y := make([]reg.VecVirtual, regs)
	for i := range y {
		y[i] = YMM()
	}
	SHRQ(U8(5), n)
	Label("fma_avx2_block")
	CMPQ(n, U8(0))
	JE(LabelRef("fma_avx2_done"))
	for i := range y {
		VMOVUPD(Mem{Base: p, Disp: 32 * i}, y[i])
	}
	k := GP64()
	MOVQ(rounds, k)
	TESTQ(k, k)
	JE(LabelRef("fma_avx2_store"))
	Label("fma_avx2_round")
	for i := range y {
		VFMADD213PD(consts[2], consts[0], y[i]) // y = a*y + b
	}
	for i := range y {
		VFMADD213PD(consts[3], consts[1], y[i]) // y = ainv*y + c
	}
	DECQ(k)
	JNE(LabelRef("fma_avx2_round"))
	Label("fma_avx2_store")
	for i := range y {
		VMOVUPD(y[i], Mem{Base: p, Disp: 32 * i})
	}
	ADDQ(U32(32*regs), p)
	DECQ(n)
	JMP(LabelRef("fma_avx2_block"))
	Label("fma_avx2_done")
	VZEROUPPER()
	RET()
}

func genFMAAVX512() {
	TEXT("fmaAVX512", NOSPLIT, floatSig)
	Doc("fmaAVX512 : y = fma(y,a,b) ; y = fma(y,ainv,c) ; n multiple de 64.")
	p := Load(Param("p"), GP64())
	n := Load(Param("n"), GP64())
	rounds := Load(Param("rounds"), GP64())
	consts := make([]reg.Register, 4)
	for i, name := range []string{"a", "ainv", "b", "c"} {
		_ = i
		consts[i] = ZMM()
		VBROADCASTSD(Load(Param(name), XMM()), consts[i])
	}
	const regs = 8 // 8 ZMM = 64 flottants
	y := make([]reg.VecVirtual, regs)
	for i := range y {
		y[i] = ZMM()
	}
	SHRQ(U8(6), n)
	Label("fma_avx512_block")
	CMPQ(n, U8(0))
	JE(LabelRef("fma_avx512_done"))
	for i := range y {
		VMOVUPD(Mem{Base: p, Disp: 64 * i}, y[i])
	}
	k := GP64()
	MOVQ(rounds, k)
	TESTQ(k, k)
	JE(LabelRef("fma_avx512_store"))
	Label("fma_avx512_round")
	for i := range y {
		VFMADD213PD(consts[2], consts[0], y[i])
	}
	for i := range y {
		VFMADD213PD(consts[3], consts[1], y[i])
	}
	DECQ(k)
	JNE(LabelRef("fma_avx512_round"))
	Label("fma_avx512_store")
	for i := range y {
		VMOVUPD(y[i], Mem{Base: p, Disp: 64 * i})
	}
	ADDQ(U32(64*regs), p)
	DECQ(n)
	JMP(LabelRef("fma_avx512_block"))
	Label("fma_avx512_done")
	VZEROUPPER()
	RET()
}
