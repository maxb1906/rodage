package sysinfo

import "testing"

func TestCollect(t *testing.T) {
	in := Collect()
	if in.LogicalCPUs < 1 || in.CPUModel == "" {
		t.Fatalf("info incomplète : %+v", in)
	}
	t.Logf("%+v", in)
}

func TestParseSMBIOS(t *testing.T) {
	// Type 1 minimal : en-tête 8 octets (fabricant=1, produit=2, version=3, série=4).
	tbl := []byte{1, 8, 0, 0, 1, 2, 3, 4}
	tbl = append(tbl, "ACME\x00Box 9\x00v1\x00SN123\x00\x00"...)
	tbl = append(tbl, 127, 4, 0, 0, 0, 0)
	s := parseSMBIOS(tbl)
	if s.Manufacturer != "ACME" || s.Product != "Box 9" || s.Serial != "SN123" {
		t.Fatalf("%+v", s)
	}
}
