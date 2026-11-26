//go:build arm64 && !purego

#include "textflag.h"

// func xorshiftNEON(p *uint32, n int, rounds int)
// Blocs de 16 mots (4 registres V de 4 mots) ; n multiple de 64.
TEXT ·xorshiftNEON(SB), NOSPLIT, $0-24
	MOVD p+0(FP), R0
	MOVD n+8(FP), R1
	MOVD rounds+16(FP), R2
	LSR  $4, R1, R1

xs_block:
	CBZ  R1, xs_done
	VLD1 (R0), [V0.S4, V1.S4, V2.S4, V3.S4]
	MOVD R2, R3
	CBZ  R3, xs_store

xs_round:
	VSHL  $13, V0.S4, V4.S4
	VSHL  $13, V1.S4, V5.S4
	VSHL  $13, V2.S4, V6.S4
	VSHL  $13, V3.S4, V7.S4
	VEOR  V4.B16, V0.B16, V0.B16
	VEOR  V5.B16, V1.B16, V1.B16
	VEOR  V6.B16, V2.B16, V2.B16
	VEOR  V7.B16, V3.B16, V3.B16
	VUSHR $17, V0.S4, V4.S4
	VUSHR $17, V1.S4, V5.S4
	VUSHR $17, V2.S4, V6.S4
	VUSHR $17, V3.S4, V7.S4
	VEOR  V4.B16, V0.B16, V0.B16
	VEOR  V5.B16, V1.B16, V1.B16
	VEOR  V6.B16, V2.B16, V2.B16
	VEOR  V7.B16, V3.B16, V3.B16
	VSHL  $5, V0.S4, V4.S4
	VSHL  $5, V1.S4, V5.S4
	VSHL  $5, V2.S4, V6.S4
	VSHL  $5, V3.S4, V7.S4
	VEOR  V4.B16, V0.B16, V0.B16
	VEOR  V5.B16, V1.B16, V1.B16
	VEOR  V6.B16, V2.B16, V2.B16
	VEOR  V7.B16, V3.B16, V3.B16
	SUBS  $1, R3, R3
	BNE   xs_round

xs_store:
	VST1.P [V0.S4, V1.S4, V2.S4, V3.S4], 64(R0)
	SUB    $1, R1, R1
	B      xs_block

xs_done:
	RET

// func fmaNEON(p *float64, n int, rounds int, a, ainv, b, c float64)
// y = fma(y,a,b) ; y = fma(y,ainv,c). Blocs de 16 flottants (8 registres V).
// FMLA accumule : on charge la constante additive puis on ajoute y*a.
TEXT ·fmaNEON(SB), NOSPLIT, $0-56
	MOVD p+0(FP), R0
	MOVD n+8(FP), R1
	MOVD rounds+16(FP), R2
	MOVD a+24(FP), R4
	MOVD ainv+32(FP), R5
	MOVD b+40(FP), R6
	MOVD c+48(FP), R7
	VDUP R4, V24.D2
	VDUP R5, V25.D2
	VDUP R6, V26.D2
	VDUP R7, V27.D2
	LSR  $4, R1, R1

fma_block:
	CBZ  R1, fma_done
	VLD1 (R0), [V0.D2, V1.D2, V2.D2, V3.D2]
	ADD  $64, R0, R8
	VLD1 (R8), [V4.D2, V5.D2, V6.D2, V7.D2]
	MOVD R2, R3
	CBZ  R3, fma_store

fma_round:
	// t = b + y*a
	VORR  V26.B16, V26.B16, V8.B16
	VORR  V26.B16, V26.B16, V9.B16
	VORR  V26.B16, V26.B16, V10.B16
	VORR  V26.B16, V26.B16, V11.B16
	VORR  V26.B16, V26.B16, V12.B16
	VORR  V26.B16, V26.B16, V13.B16
	VORR  V26.B16, V26.B16, V14.B16
	VORR  V26.B16, V26.B16, V15.B16
	VFMLA V24.D2, V0.D2, V8.D2
	VFMLA V24.D2, V1.D2, V9.D2
	VFMLA V24.D2, V2.D2, V10.D2
	VFMLA V24.D2, V3.D2, V11.D2
	VFMLA V24.D2, V4.D2, V12.D2
	VFMLA V24.D2, V5.D2, V13.D2
	VFMLA V24.D2, V6.D2, V14.D2
	VFMLA V24.D2, V7.D2, V15.D2
	// y = c + t*ainv
	VORR  V27.B16, V27.B16, V0.B16
	VORR  V27.B16, V27.B16, V1.B16
	VORR  V27.B16, V27.B16, V2.B16
	VORR  V27.B16, V27.B16, V3.B16
	VORR  V27.B16, V27.B16, V4.B16
	VORR  V27.B16, V27.B16, V5.B16
	VORR  V27.B16, V27.B16, V6.B16
	VORR  V27.B16, V27.B16, V7.B16
	VFMLA V25.D2, V8.D2, V0.D2
	VFMLA V25.D2, V9.D2, V1.D2
	VFMLA V25.D2, V10.D2, V2.D2
	VFMLA V25.D2, V11.D2, V3.D2
	VFMLA V25.D2, V12.D2, V4.D2
	VFMLA V25.D2, V13.D2, V5.D2
	VFMLA V25.D2, V14.D2, V6.D2
	VFMLA V25.D2, V15.D2, V7.D2
	SUBS  $1, R3, R3
	BNE   fma_round

fma_store:
	VST1.P [V0.D2, V1.D2, V2.D2, V3.D2], 64(R0)
	VST1.P [V4.D2, V5.D2, V6.D2, V7.D2], 64(R0)
	SUB    $1, R1, R1
	B      fma_block

fma_done:
	RET
