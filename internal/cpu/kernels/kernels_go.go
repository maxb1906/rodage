// Package kernels regroupe les noyaux de calcul vérifiés de Rodage.
//
// Chaque noyau SIMD (SSE2, AVX2, AVX-512 générés avec avo ; NEON écrit à la
// main) a un équivalent en Go pur qui produit le même résultat au bit près.
// Le résultat attendu d'un calcul est donc connu : une différence révèle une
// erreur silencieuse du processeur.
package kernels

import "math"

// Block est la granularité imposée aux tableaux passés aux noyaux.
const Block = 64

// XorshiftGo applique xorshift32 rounds fois à chaque mot.
func XorshiftGo(p []uint32, rounds int) {
	for i := range p {
		x := p[i]
		for r := 0; r < rounds; r++ {
			x ^= x << 13
			x ^= x >> 17
			x ^= x << 5
		}
		p[i] = x
	}
}

// MulAddGo : y = y*a+b puis y = y*ainv+c, arrondis séparés (pas de FMA).
func MulAddGo(p []float64, rounds int, a, ainv, b, c float64) {
	for i := range p {
		y := p[i]
		for r := 0; r < rounds; r++ {
			// Les conversions explicites empêchent le compilateur de fusionner en FMA.
			y = float64(y*a) + b
			y = float64(y*ainv) + c
		}
		p[i] = y
	}
}

// FMAGo : y = fma(y,a,b) puis y = fma(y,ainv,c), un seul arrondi par étape.
func FMAGo(p []float64, rounds int, a, ainv, b, c float64) {
	for i := range p {
		y := p[i]
		for r := 0; r < rounds; r++ {
			y = math.FMA(y, a, b)
			y = math.FMA(y, ainv, c)
		}
		p[i] = y
	}
}
