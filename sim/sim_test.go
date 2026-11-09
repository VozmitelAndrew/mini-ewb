package sim

import (
	"math"
	"strings"
	"testing"

	"github.com/VozmitelAndrew/mini-ewb/circuit"
	"github.com/VozmitelAndrew/mini-ewb/logic"
)

type bench struct {
	t  *testing.T
	c  *circuit.Circuit
	hi int
	lo int
}

func newBench(t *testing.T) *bench {
	c := circuit.New()
	b := &bench{t: t, c: c}
	b.hi = c.Add("vcc", 0, 0).ID
	b.lo = c.Add("ground", 0, 0).ID
	return b
}

func (b *bench) add(kind string, props ...string) int {
	p := b.c.Add(kind, 0, 0)
	for i := 0; i+1 < len(props); i += 2 {
		p.Set(props[i], props[i+1])
	}
	return p.ID
}

func (b *bench) wire(a int, ap string, c int, cp string) {
	if !b.c.Connect(circuit.PinRef{Part: a, Pin: ap}, circuit.PinRef{Part: c, Pin: cp}) {
		b.t.Fatalf("connect %d.%s %d.%s", a, ap, c, cp)
	}
}

// tie connects a pin to logic 1 or 0.
func (b *bench) tie(id int, pin string, v bool) {
	if v {
		b.wire(id, pin, b.hi, "+")
	} else {
		b.wire(id, pin, b.lo, "G")
	}
}

// keySwitch: COM -> pin, A = pull-up (1), B = ground (0). Starts at 1.
func (b *bench) keySwitch(id int, pin string) int {
	sw := b.add("switch")
	pu := b.add("pullup")
	b.wire(sw, "A", pu, "OUT")
	b.wire(sw, "B", b.lo, "G")
	b.wire(sw, "COM", id, pin)
	return sw
}

func setSwitch(s *Sim, sw int, one bool) {
	cur := s.C.Part(sw).Str("state") == "0"
	if cur != one {
		s.Toggle(sw)
	}
}

func bitsOf(s *Sim, id int, pins ...string) string {
	var b strings.Builder
	for _, p := range pins {
		if s.PinLit(id, p) {
			b.WriteByte('1')
		} else {
			b.WriteByte('0')
		}
	}
	return b.String()
}

func TestRSLatch(t *testing.T) {
	b := newBench(t)
	rs := b.add("rs")
	sw := b.keySwitch(rs, "S'")
	rw := b.keySwitch(rs, "R'")
	s := New(b.c)
	q := func() string { return bitsOf(s, rs, "Q", "Q'") }

	setSwitch(s, sw, false)
	if q() != "10" {
		t.Fatalf("set: %s", q())
	}
	setSwitch(s, sw, true)
	if q() != "10" {
		t.Fatalf("hold after set: %s", q())
	}
	setSwitch(s, rw, false)
	if q() != "01" {
		t.Fatalf("reset: %s", q())
	}
	setSwitch(s, sw, false)
	if q() != "11" {
		t.Fatalf("forbidden: %s", q())
	}
	setSwitch(s, rw, true)
	if q() != "10" {
		t.Fatalf("leave forbidden via S': %s", q())
	}
}

func TestRippleCounter32(t *testing.T) {
	b := newBench(t)
	ff := make([]int, 5)
	for i := range ff {
		ff[i] = b.add("dff")
		b.wire(ff[i], "Q'", ff[i], "D")
		if i > 0 {
			b.wire(ff[i-1], "Q'", ff[i], "CLK")
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
	for k := 1; k <= 40; k++ {
		setSwitch(s, clk, false)
		setSwitch(s, clk, true)
		if got := value(); got != k%32 {
			t.Fatalf("after %d pulses value %d", k, got)
		}
	}
}

func TestDecoder7447ThroughInverters(t *testing.T) {
	// Expected a..g from the lab 4 report table (after the 7404 inverters).
	want := []string{"1111110", "0110000", "1101101", "1111001", "0110011",
		"1011011", "0011111", "1110000", "1111111", "1110011"}
	for d, w := range want {
		b := newBench(t)
		dec := b.add("7447")
		inv := b.add("7404")
		inv2 := b.add("7404")
		for i, pin := range []string{"A", "B", "C", "D"} {
			b.tie(dec, pin, d>>i&1 == 1)
		}
		b.tie(dec, "LT'", true)
		b.tie(dec, "RBI'", true)
		outs := []string{"OA", "OB", "OC", "OD", "OE", "OF", "OG"}
		gates := []struct {
			id      int
			in, out string
		}{{inv, "1A", "1Y"}, {inv, "2A", "2Y"}, {inv, "3A", "3Y"}, {inv, "4A", "4Y"}, {inv, "5A", "5Y"}, {inv, "6A", "6Y"}, {inv2, "1A", "1Y"}}
		seg := b.add("seg7")
		for i, o := range outs {
			b.wire(dec, o, gates[i].id, gates[i].in)
			b.wire(gates[i].id, gates[i].out, seg, string(rune('a'+i)))
		}
		s := New(b.c)
		if got := bitsOf(s, seg, "a", "b", "c", "d", "e", "f", "g"); got != w {
			t.Errorf("digit %d: got %s want %s", d, got, w)
		}
	}
}

func Test7447BlankingAndLampTest(t *testing.T) {
	b := newBench(t)
	dec := b.add("7447")
	for _, p := range []string{"A", "B", "C", "D", "RBI'"} {
		b.tie(dec, p, false)
	}
	b.tie(dec, "LT'", true)
	s := New(b.c)
	if got := bitsOf(s, dec, "OA", "OB", "OC", "OD", "OE", "OF", "OG"); got != "1111111" {
		t.Fatalf("ripple blanking: %s", got)
	}
	if s.PinLit(dec, "BI'/RBO'") {
		t.Fatal("RBO' should be low")
	}
}

func TestEncoder74148(t *testing.T) {
	// Rows from the lab 4 report: inputs 0..7 -> A2 A1 A0 EO GS.
	rows := [][2]string{
		{"00000000", "00010"}, {"00000001", "00110"}, {"00000010", "00010"}, {"00000011", "01010"},
		{"00000111", "01110"}, {"00001000", "00010"}, {"00001111", "10010"},
	}
	for _, r := range rows {
		b := newBench(t)
		enc := b.add("74148")
		for i := 0; i < 8; i++ {
			b.tie(enc, string(rune('0'+i)), r[0][i] == '1')
		}
		b.tie(enc, "EI", false)
		s := New(b.c)
		if got := bitsOf(s, enc, "A2", "A1", "A0", "EO", "GS"); got != r[1] {
			t.Errorf("in %s: got %s want %s", r[0], got, r[1])
		}
	}
	b := newBench(t)
	enc := b.add("74148")
	b.tie(enc, "EI", true)
	s := New(b.c)
	if got := bitsOf(s, enc, "A2", "A1", "A0", "EO", "GS"); got != "11111" {
		t.Errorf("EI high: %s", got)
	}
}

func TestALU74181Lab5(t *testing.T) {
	// S3..S0 = 0100; results from the lab 5 report (variant 16).
	cases := []struct {
		a, b  int
		m, cn bool
		f     int
	}{
		{0b1010, 0b0101, false, false, 0b0101},
		{0b1010, 0b0101, false, true, 0b0100},
		{0b1100, 0b0101, true, true, 0b1011},
		{0b1100, 0b0101, false, true, 0b0100},
		{0b1100, 0b0101, false, false, 0b0101},
		{0b1010, 0b0011, true, true, 0b1101},
		{0b1010, 0b0011, false, true, 0b0010},
		{0b1010, 0b0011, false, false, 0b0011},
	}
	for _, c := range cases {
		b := newBench(t)
		alu := b.add("74181")
		for i := 0; i < 4; i++ {
			b.tie(alu, "A"+string(rune('0'+i))+"'", c.a>>i&1 == 1)
			b.tie(alu, "B"+string(rune('0'+i))+"'", c.b>>i&1 == 1)
		}
		for i, v := range []bool{false, false, true, false} { // S0..S3
			b.tie(alu, "S"+string(rune('0'+i)), v)
		}
		b.tie(alu, "M", c.m)
		b.tie(alu, "CN", c.cn)
		s := New(b.c)
		got := 0
		for i := 0; i < 4; i++ {
			if s.PinLit(alu, "F"+string(rune('0'+i))+"'") {
				got |= 1 << i
			}
		}
		if got != c.f {
			t.Errorf("A=%04b B=%04b M=%v CN=%v: got %04b want %04b", c.a, c.b, c.m, c.cn, got, c.f)
		}
	}
}

func TestDACLab6(t *testing.T) {
	// 8-bit DAC, reference 4 V, output through a 2 V battery to a 10k load and a voltmeter.
	for _, c := range []struct {
		code int
		want float64
	}{{4, -1.938}, {1, -1.984}} {
		b := newBench(t)
		d := b.add("dac")
		for i := 0; i < 8; i++ {
			b.tie(d, "D"+string(rune('0'+i)), c.code>>i&1 == 1)
		}
		ref := b.add("battery", "V", "4")
		b.wire(ref, "+", d, "+")
		b.wire(ref, "-", b.lo, "G")
		b.wire(d, "-", b.lo, "G")
		bat := b.add("battery", "V", "2")
		b.wire(bat, "+", d, "OUT")
		r := b.add("resistor", "R", "10000")
		vm := b.add("voltmeter")
		b.wire(bat, "-", r, "1")
		b.wire(r, "2", b.lo, "G")
		b.wire(vm, "+", r, "1")
		b.wire(vm, "-", b.lo, "G")
		s := New(b.c)
		if got := s.Diff(vm, "+", "-"); math.Abs(got-c.want) > 0.001 {
			t.Errorf("code %d: %.4f V, want %.3f", c.code, got, c.want)
		}
	}
}

func TestADCtoDAC(t *testing.T) {
	b := newBench(t)
	a := b.add("adc")
	in := b.add("battery", "V", "1")
	b.wire(in, "+", a, "VIN")
	b.wire(in, "-", b.lo, "G")
	rp := b.add("battery", "V", "2")
	b.wire(rp, "+", a, "VREF+")
	b.wire(rp, "-", b.lo, "G")
	rn := b.add("battery", "V", "2")
	b.wire(rn, "-", a, "VREF-")
	b.wire(rn, "+", b.lo, "G")
	soc := b.keySwitch(a, "SOC")
	d := b.add("dac")
	for i := 0; i < 8; i++ {
		p := "D" + string(rune('0'+i))
		b.wire(a, p, d, p)
	}
	b.wire(d, "+", a, "VREF+")
	b.wire(d, "-", a, "VREF-")
	s := New(b.c)
	setSwitch(s, soc, false)
	setSwitch(s, soc, true)
	want := ADCCode(1, 2, -2) // 192
	if want != 192 {
		t.Fatalf("ADCCode = %d", want)
	}
	v, _ := s.PinV(d, "OUT")
	if math.Abs(v-(-2+4*192.0/256)) > 1e-9 {
		t.Fatalf("DAC out %.4f", v)
	}
}

func TestFunctionGeneratorAndScope(t *testing.T) {
	b := newBench(t)
	fg := b.add("funcgen", "F", "200", "A", "2")
	b.wire(fg, "COM", b.lo, "G")
	sc := b.add("scope", "TB", "1")
	b.wire(sc, "A", fg, "+")
	s := New(b.c)
	s.Dt = 10e-6
	s.Run(1.25e-3) // quarter period of 200 Hz
	v, _ := s.PinV(fg, "+")
	if math.Abs(v-2) > 0.01 {
		t.Fatalf("peak %.3f", v)
	}
	if n := len(s.Device(sc).(*Scope).Samples()); n < 100 {
		t.Fatalf("scope samples %d", n)
	}
}

func TestClockSource(t *testing.T) {
	b := newBench(t)
	ck := b.add("clock", "F", "1000", "DUTY", "50")
	b.wire(ck, "-", b.lo, "G")
	p := b.add("probe")
	b.wire(p, "IN", ck, "+")
	s := New(b.c)
	s.Dt = 1e-5
	s.Run(0.2e-3)
	if !s.PinLit(p, "IN") {
		t.Fatal("first half should be high")
	}
	s.Run(0.5e-3)
	if s.PinLit(p, "IN") {
		t.Fatal("second half should be low")
	}
}

func TestLab1LogicConverter(t *testing.T) {
	// f = C + AB' in NAND basis: f = NAND(C', NAND(A, B')).
	b := newBench(t)
	lc := b.add("logicconv")
	nc := b.add("nand")
	nb := b.add("nand")
	nab := b.add("nand")
	f := b.add("nand")
	b.wire(lc, "C", nc, "1")
	b.wire(lc, "C", nc, "2")
	b.wire(lc, "B", nb, "1")
	b.wire(lc, "B", nb, "2")
	b.wire(lc, "A", nab, "1")
	b.wire(nb, "Y", nab, "2")
	b.wire(nc, "Y", f, "1")
	b.wire(nab, "Y", f, "2")
	b.wire(f, "Y", lc, "OUT")
	s := New(b.c)
	n, table, err := s.Analyze(lc)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 || table != "01011101" {
		t.Fatalf("n=%d table=%s", n, table)
	}
	if got := logic.Format(n, logic.Minimize(n, table)); got != "C+AB'" {
		t.Fatalf("minimized %s", got)
	}
}

func TestWordGenToLogicAnalyzer(t *testing.T) {
	b := newBench(t)
	wg := b.add("wordgen", "F", "1000", "WORDS", "0 1 2 3")
	la := b.add("logican", "RATE", "10000")
	b.wire(wg, "0", la, "1")
	b.wire(wg, "1", la, "2")
	s := New(b.c)
	s.Dt = 1e-5
	s.Run(4.5e-3)
	an := s.Device(la).(*LogicAn)
	if an.Mask != 3 {
		t.Fatalf("mask %b", an.Mask)
	}
	seen := map[uint16]bool{}
	for _, w := range an.Words {
		seen[w] = true
	}
	if len(seen) != 4 {
		t.Fatalf("words seen %v", seen)
	}
}

func TestShortIsReported(t *testing.T) {
	b := newBench(t)
	b.wire(b.hi, "+", b.lo, "G")
	s := New(b.c)
	if !s.Shorted {
		t.Fatal("short not detected")
	}
}
