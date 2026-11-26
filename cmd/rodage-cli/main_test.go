package main

import (
	"reflect"
	"testing"
)

func TestParseSize(t *testing.T) {
	for in, want := range map[string]uint64{"512M": 512 << 20, "50G": 50 << 30, "1.5G": 3 << 29, "4096": 4096, "2GiB": 2 << 30, "8Go": 8 << 30} {
		got, err := parseSize(in)
		if err != nil || got != want {
			t.Errorf("%s : %d %v", in, got, err)
		}
	}
	if _, err := parseSize("abc"); err == nil {
		t.Error("taille invalide acceptée")
	}
}

func TestReorder(t *testing.T) {
	got := reorder([]string{"a.json", "--html", "x.html", "b.json", "-json"})
	want := []string{"--html", "x.html", "-json", "a.json", "b.json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}
