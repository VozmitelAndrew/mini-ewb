package logic

import "testing"

func table(n int, ones ...int) string {
	b := make([]byte, 1<<n)
	for i := range b {
		b[i] = '0'
	}
	for _, o := range ones {
		b[o] = '1'
	}
	return string(b)
}

func TestLab1ThreeVars(t *testing.T) {
	got := Format(3, Minimize(3, table(3, 1, 3, 4, 5, 7)))
	if got != "C+AB'" {
		t.Fatalf("got %s", got)
	}
}

func TestLab1FourVars(t *testing.T) {
	terms := Minimize(4, table(4, 2, 7, 9, 10, 12, 14))
	if len(terms) != 4 {
		t.Fatalf("want 4 terms, got %s", Format(4, terms))
	}
	tb := table(4, 2, 7, 9, 10, 12, 14)
	for m := 0; m < 16; m++ {
		hit := false
		for _, tr := range terms {
			hit = hit || tr.covers(uint32(m))
		}
		if hit != (tb[m] == '1') {
			t.Fatalf("row %d wrong in %s", m, Format(4, terms))
		}
	}
}

func TestConstants(t *testing.T) {
	if got := Format(2, Minimize(2, "0000")); got != "0" {
		t.Fatal(got)
	}
	if got := Format(2, Minimize(2, "1111")); got != "1" {
		t.Fatal(got)
	}
	if got := Format(2, Minimize(2, "1x0x")); got != "A'" {
		t.Fatal(got)
	}
}
