package sim

import (
	"math"
	"testing"
)

// Inverting summer with unit gains: Uout = -(5 - 9 + 12) = -8 V.
func TestOpAmpSummer(t *testing.T) {
	b := newBench(t)
	op := b.add("opamp")
	b.wire(op, "+", b.lo, "G")
	rf := b.add("resistor", "R", "10000")
	b.wire(rf, "1", op, "-")
	b.wire(rf, "2", op, "OUT")
	for _, v := range []string{"5", "-9", "12"} {
		bat := b.add("battery", "V", v)
		r := b.add("resistor", "R", "10000")
		b.wire(bat, "-", b.lo, "G")
		b.wire(bat, "+", r, "1")
		b.wire(r, "2", op, "-")
	}
	s := New(b.c)
	if !s.Analog {
		t.Fatal("analog mode expected")
	}
	if v, _ := s.PinV(op, "OUT"); math.Abs(v+8) > 1e-3 {
		t.Fatalf("Uout = %.4f", v)
	}
}

func TestOpAmpSaturates(t *testing.T) {
	b := newBench(t)
	op := b.add("opamp", "VSW", "15")
	bat := b.add("battery", "V", "1")
	b.wire(bat, "-", b.lo, "G")
	b.wire(bat, "+", op, "+")
	b.wire(op, "-", b.lo, "G")
	s := New(b.c)
	if v, _ := s.PinV(op, "OUT"); math.Abs(v-15) > 1e-6 {
		t.Fatalf("open loop output %.3f, want +15", v)
	}
}

// Real differentiator: triangle 1 V, 6 kHz; R2 = 5k, C = 0.05 uF -> square of R2*C*U/(T/4) = 6 V.
func TestDifferentiatorTriangle(t *testing.T) {
	b := newBench(t)
	fg := b.add("funcgen", "F", "6000", "A", "1", "WAVE", "1")
	b.wire(fg, "COM", b.lo, "G")
	r1 := b.add("resistor", "R", "80")
	c := b.add("capacitor", "C", "0.05")
	op := b.add("opamp")
	r2 := b.add("resistor", "R", "5000")
	b.wire(fg, "+", r1, "1")
	b.wire(r1, "2", c, "1")
	b.wire(c, "2", op, "-")
	b.wire(op, "+", b.lo, "G")
	b.wire(r2, "1", op, "-")
	b.wire(r2, "2", op, "OUT")
	s := New(b.c)
	s.Dt = 0.1e-6
	period := 1.0 / 6000
	s.Run(3 * period)
	// Middle of a falling slope (phase 0.5 of the period): output = +6 V; rising slope: -6 V.
	s.Run(0.5 * period)
	v1, _ := s.PinV(op, "OUT")
	s.Run(0.5 * period)
	v2, _ := s.PinV(op, "OUT")
	if math.Abs(math.Abs(v1)-6) > 0.15 || math.Abs(math.Abs(v2)-6) > 0.15 || v1*v2 > 0 {
		t.Fatalf("outputs %.3f and %.3f, want +-6 V", v1, v2)
	}
}

func TestTimeDelaySwitch(t *testing.T) {
	b := newBench(t)
	sw := b.add("tdswitch", "TON", "1", "TOFF", "3")
	b.wire(sw, "A", b.hi, "+")
	b.wire(sw, "B", b.lo, "G")
	p := b.add("probe")
	b.wire(p, "IN", sw, "COM")
	s := New(b.c)
	s.Dt = 1e-5
	s.Run(0.5e-3)
	if s.PinLit(p, "IN") {
		t.Fatal("before TON")
	}
	s.Run(1e-3)
	if !s.PinLit(p, "IN") {
		t.Fatal("between TON and TOFF")
	}
	s.Run(2e-3)
	if s.PinLit(p, "IN") {
		t.Fatal("after TOFF")
	}
}

func TestJKCounter16(t *testing.T) {
	b := newBench(t)
	ff := make([]int, 4)
	for i := range ff {
		ff[i] = b.add("jkff")
		b.tie(ff[i], "J", true)
		b.tie(ff[i], "K", true)
		if i > 0 {
			b.wire(ff[i-1], "Q", ff[i], "CLK")
		}
	}
	clk := b.keySwitch(ff[0], "CLK")
	s := New(b.c)
	value := func() int {
		v := 0
		for i, id := range ff {
			if s.PinLit(id, "Q") {
				v |= 1 << i
			}
		}
		return v
	}
	for k := 1; k <= 20; k++ {
		setSwitch(s, clk, false)
		setSwitch(s, clk, true)
		if got := value(); got != k%16 {
			t.Fatalf("after %d pulses %d", k, got)
		}
	}
}

func TestDFlipFlopDirectRS(t *testing.T) {
	b := newBench(t)
	d := b.add("dff", "RS", "1")
	sSw := b.keySwitch(d, "S")
	rSw := b.keySwitch(d, "R")
	s := New(b.c)
	setSwitch(s, sSw, false)
	setSwitch(s, rSw, false)
	q := func() string { return bitsOf(s, d, "Q", "Q'") }
	setSwitch(s, sSw, true)
	if q() != "10" {
		t.Fatalf("S=1: %s", q())
	}
	setSwitch(s, sSw, false)
	setSwitch(s, rSw, true)
	if q() != "01" {
		t.Fatalf("R=1: %s", q())
	}
}

func TestGates(t *testing.T) {
	for _, c := range []struct {
		kind string
		want string // outputs for inputs 00, 01, 10, 11
	}{{"and", "0001"}, {"or", "0111"}, {"xor", "0110"}, {"nand", "1110"}} {
		got := ""
		for v := 0; v < 4; v++ {
			b := newBench(t)
			g := b.add(c.kind)
			b.tie(g, "1", v&2 != 0)
			b.tie(g, "2", v&1 != 0)
			s := New(b.c)
			got += bitsOf(s, g, "Y")
		}
		if got != c.want {
			t.Errorf("%s: %s want %s", c.kind, got, c.want)
		}
	}
}
