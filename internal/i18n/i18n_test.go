package i18n

import "testing"

func TestT(t *testing.T) {
	if T(FR, "verdict.pass") != "Réussi" || T(EN, "verdict.pass") != "Passed" {
		t.Fatal()
	}
	if Parse("fr_FR.UTF-8") != FR || Parse("en_US") != EN {
		t.Fatal()
	}
	for k, v := range catalog {
		if v[0] == "" || v[1] == "" {
			t.Errorf("traduction manquante : %s", k)
		}
	}
}
