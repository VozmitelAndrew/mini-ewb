package sim

import (
	"math"
	"math/rand"
)

func newDevice(e *elem) Device {
	p := e.part
	switch p.Kind {
	case "ground":
		return &source{pin: e.ix("G")}
	case "vcc":
		return &source{pin: e.ix("+"), v: p.Num("V"), st: Strong}
	case "pullup":
		return &source{pin: e.ix("OUT"), v: p.Num("V"), st: Weak}
	case "battery":
		return &battery{p: e.ix("+"), n: e.ix("-"), v: p.Num("V")}
	case "resistor":
		return &resistor{a: e.ix("1"), b: e.ix("2")}
	case "clock":
		return &clock{p: e.ix("+"), n: e.ix("-"), f: p.Num("F"), duty: p.Num("DUTY") / 100, v: p.Num("V")}
	case "funcgen":
		return &funcGen{p: e.ix("+"), c: e.ix("COM"), n: e.ix("-"), f: p.Num("F"), a: p.Num("A"), off: p.Num("OFF")}
	case "not":
		return &inverters{in: e.ixs("A"), out: e.ixs("Y")}
	case "nand":
		in := make([]int, 0, len(e.pins)-1)
		for i, pd := range e.pins {
			if pd.Name != "Y" {
				in = append(in, i)
			}
		}
		return &nand{in: in, y: e.ix("Y")}
	case "7404":
		return &inverters{
			in:  e.ixs("1A", "2A", "3A", "4A", "5A", "6A"),
			out: e.ixs("1Y", "2Y", "3Y", "4Y", "5Y", "6Y")}
	case "rs":
		return &rsLatch{s: e.ix("S'"), r: e.ix("R'"), q: e.ix("Q"), nq: e.ix("Q'")}
	case "dff":
		return &dff{d: e.ix("D"), clk: e.ix("CLK"), s: e.ix("S'"), r: e.ix("R'"), q: e.ix("Q"), nq: e.ix("Q'")}
	case "7447":
		return &bcd7447{in: e.ixs("A", "B", "C", "D"), lt: e.ix("LT'"), rbi: e.ix("RBI'"), bi: e.ix("BI'/RBO'"),
			seg: e.ixs("OA", "OB", "OC", "OD", "OE", "OF", "OG")}
	case "74148":
		return &enc74148{in: e.ixs("0", "1", "2", "3", "4", "5", "6", "7"), ei: e.ix("EI"),
			a: e.ixs("A0", "A1", "A2"), gs: e.ix("GS"), eo: e.ix("EO")}
	case "74181":
		return &alu74181{a: e.ixs("A0'", "A1'", "A2'", "A3'"), b: e.ixs("B0'", "B1'", "B2'", "B3'"),
			s: e.ixs("S0", "S1", "S2", "S3"), m: e.ix("M"), cn: e.ix("CN"),
			f: e.ixs("F0'", "F1'", "F2'", "F3'"), eq: e.ix("A=B"), p: e.ix("P'"), g: e.ix("G'"), cn4: e.ix("CN+4")}
	case "dac":
		return &dac{d: e.ixs("D0", "D1", "D2", "D3", "D4", "D5", "D6", "D7"), p: e.ix("+"), n: e.ix("-"), out: e.ix("OUT")}
	case "adc":
		return &adc{vin: e.ix("VIN"), rp: e.ix("VREF+"), rn: e.ix("VREF-"), soc: e.ix("SOC"), oe: e.ix("OE"),
			d: e.ixs("D0", "D1", "D2", "D3", "D4", "D5", "D6", "D7"), eoc: e.ix("EOC")}
	case "scope":
		return newScope(e)
	case "wordgen":
		return newWordGen(e)
	case "logicconv":
		return newLogicConv(e)
	case "logican":
		return newLogicAn(e)
	}
	// switch, node, probe, displays, voltmeter: passive, the UI reads their nets.
	return passive{}
}

type passive struct{}

func (passive) Eval(*IO) {}

type source struct {
	pin int
	v   float64
	st  Strength
}

func (d *source) Eval(io *IO) {
	st := d.st
	if st == Floating {
		st = Strong
	}
	io.Drive(d.pin, d.v, st)
}

type battery struct {
	p, n int
	v    float64
}

func (*battery) Eval(*IO)        {}
func (d *battery) Links(io *IO)  { io.Link(d.p, d.n, d.v) }
func (*resistor) Eval(*IO)       {}
func (d *resistor) Links(io *IO) { io.WeakLink(d.a, d.b) }
func (*clock) Eval(*IO)          {}
func (*funcGen) Eval(*IO)        {}
func (d *inverters) Eval(io *IO) { d.eval(io) }
func (d *nand) Eval(io *IO)      { d.eval(io) }

type resistor struct{ a, b int }

type clock struct {
	p, n       int
	f, duty, v float64
}

func (d *clock) Links(io *IO) {
	v := 0.0
	if d.f > 0 {
		ph := io.T() * d.f
		if ph-math.Floor(ph) < d.duty {
			v = d.v
		}
	}
	io.Link(d.p, d.n, v)
}

type funcGen struct {
	p, c, n   int
	f, a, off float64
}

func (d *funcGen) Links(io *IO) {
	e := d.a*math.Sin(2*math.Pi*d.f*io.T()) + d.off
	io.Link(d.p, d.c, e)
	io.Link(d.c, d.n, e)
}

type inverters struct{ in, out []int }

func (d *inverters) eval(io *IO) {
	for i := range d.in {
		io.Out(d.out[i], !io.Hi(d.in[i]))
	}
}

type nand struct {
	in []int
	y  int
}

func (d *nand) eval(io *IO) {
	all := true
	for _, p := range d.in {
		all = all && io.Hi(p)
	}
	io.Out(d.y, !all)
}

// rsLatch is an asynchronous RS latch with active-low inputs.
// S'=R'=0 drives both outputs high; releasing both at once leaves a random state.
type rsLatch struct {
	s, r, q, nq int
	state       bool
	forbidden   bool
}

func (d *rsLatch) Eval(io *IO) {
	s, r := io.Hi(d.s), io.Hi(d.r)
	switch {
	case !s && !r:
		io.Out(d.q, true)
		io.Out(d.nq, true)
	case !s:
		io.Out(d.q, true)
		io.Out(d.nq, false)
	case !r:
		io.Out(d.q, false)
		io.Out(d.nq, true)
	default:
		io.Out(d.q, d.state)
		io.Out(d.nq, !d.state)
	}
}

func (d *rsLatch) Commit(io *IO) bool {
	s, r := io.Hi(d.s), io.Hi(d.r)
	old := d.state
	switch {
	case !s && !r:
		d.forbidden = true
	case !s:
		d.state, d.forbidden = true, false
	case !r:
		d.state, d.forbidden = false, false
	case d.forbidden:
		d.state, d.forbidden = rand.Intn(2) == 1, false
		return true
	}
	return d.state != old
}

// dff is a rising-edge D flip-flop with active-low asynchronous set and reset.
type dff struct {
	d, clk, s, r, q, nq int
	state, prev, inited bool
}

func (d *dff) Eval(io *IO) {
	s, r := io.Hi(d.s), io.Hi(d.r)
	switch {
	case !s && !r:
		io.Out(d.q, true)
		io.Out(d.nq, true)
	case !s:
		io.Out(d.q, true)
		io.Out(d.nq, false)
	case !r:
		io.Out(d.q, false)
		io.Out(d.nq, true)
	default:
		io.Out(d.q, d.state)
		io.Out(d.nq, !d.state)
	}
}

func (d *dff) Commit(io *IO) bool {
	clk := io.Hi(d.clk)
	old := d.state
	switch {
	case !io.Hi(d.s):
		d.state = true
	case !io.Hi(d.r):
		d.state = false
	case d.inited && clk && !d.prev:
		d.state = io.Hi(d.d)
	}
	d.prev, d.inited = clk, true
	return d.state != old
}

// Segment masks, bit 0 = a ... bit 6 = g.
var seg7447 = [16]int{0x3F, 0x06, 0x5B, 0x4F, 0x66, 0x6D, 0x7C, 0x07, 0x7F, 0x67, 0x58, 0x4C, 0x62, 0x69, 0x78, 0x00}

// HexSegments is the segment pattern of a decoded hex display.
var HexSegments = [16]int{0x3F, 0x06, 0x5B, 0x4F, 0x66, 0x6D, 0x7D, 0x07, 0x7F, 0x6F, 0x77, 0x7C, 0x39, 0x5E, 0x79, 0x71}

// bcd7447: BCD to 7-segment decoder with active-low outputs.
type bcd7447 struct {
	in          []int
	lt, rbi, bi int
	seg         []int
}

func (d *bcd7447) Eval(io *IO) {
	v := io.Bits(d.in)
	lt := io.Hi(d.lt)
	rbo := lt && !io.Hi(d.rbi) && v == 0
	if rbo {
		io.Drive(d.bi, 0, Strong)
	} else {
		io.Drive(d.bi, 5, Weak)
	}
	blankIn := !rbo && !io.Hi(d.bi)
	segs := seg7447[v]
	switch {
	case blankIn || rbo:
		segs = 0
	case !lt:
		segs = 0x7F
	}
	for i, p := range d.seg {
		io.Out(p, segs>>i&1 == 0)
	}
}

// enc74148: 8-to-3 priority encoder, active-low inputs and outputs.
type enc74148 struct {
	in     []int
	ei     int
	a      []int
	gs, eo int
}

func (d *enc74148) Eval(io *IO) {
	a, gs, eo := 7, true, true
	if !io.Hi(d.ei) {
		top := -1
		for i := 7; i >= 0; i-- {
			if !io.Hi(d.in[i]) {
				top = i
				break
			}
		}
		if top < 0 {
			eo = false
		} else {
			a, gs = ^top&7, false
		}
	}
	io.OutBits(d.a, a)
	io.Out(d.gs, gs)
	io.Out(d.eo, eo)
}

// alu74181 implements the active-high data function table of the 74181
// (pin levels taken as is, CN = 0 means carry in, CN+4 = 0 means carry out).
type alu74181 struct {
	a, b, s, f    []int
	m, cn         int
	eq, p, g, cn4 int
}

func (d *alu74181) Eval(io *IO) {
	A, B, S := io.Bits(d.a), io.Bits(d.b), io.Bits(d.s)
	nb := ^B & 15
	t1 := A
	if S&1 != 0 {
		t1 |= B
	}
	if S&2 != 0 {
		t1 |= nb
	}
	t2 := 0
	if S&4 != 0 {
		t2 |= A & nb
	}
	if S&8 != 0 {
		t2 |= A & B
	}
	cin := 0
	if !io.Hi(d.cn) {
		cin = 1
	}
	sum := t1 + t2 + cin
	F := sum & 15
	if io.Hi(d.m) {
		F = ^(t1 ^ t2) & 15
	}
	io.OutBits(d.f, F)
	io.Out(d.eq, F == 15)
	io.Out(d.cn4, sum <= 15)
	io.Out(d.g, t1+t2 <= 15)
	io.Out(d.p, (t1|t2) != 15)
}

// dac: Vout = V(-) + (V(+) - V(-)) * code / 256.
type dac struct {
	d         []int
	p, n, out int
}

func (d *dac) Eval(io *IO) {
	code := float64(io.Bits(d.d))
	lo := io.V(d.n)
	io.Drive(d.out, lo+(io.V(d.p)-lo)*code/256, Strong)
}

// adc samples VIN on the rising edge of SOC: code = floor(256 * (Vin - Vref-) / (Vref+ - Vref-)).
// EOC goes low for 1 us after the start of conversion. Outputs are driven while OE is high.
type adc struct {
	vin, rp, rn, soc, oe, eoc int
	d                         []int
	code                      int
	eocUntil                  float64
	prev, inited              bool
}

func (d *adc) Eval(io *IO) {
	if io.Hi(d.oe) {
		io.OutBits(d.d, d.code)
	}
	io.Out(d.eoc, io.T() >= d.eocUntil)
}

func (d *adc) Commit(io *IO) bool {
	soc := io.Hi(d.soc)
	changed := false
	if d.inited && soc && !d.prev {
		d.code = ADCCode(io.V(d.vin), io.V(d.rp), io.V(d.rn))
		d.eocUntil = io.T() + 1e-6
		changed = true
	}
	d.prev, d.inited = soc, true
	return changed
}

func ADCCode(vin, refP, refN float64) int {
	span := refP - refN
	if span <= 0 {
		return 0
	}
	c := int(math.Floor((vin - refN) * 256 / span))
	if c < 0 {
		c = 0
	}
	if c > 255 {
		c = 255
	}
	return c
}
