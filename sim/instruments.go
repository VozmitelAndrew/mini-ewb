package sim

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// ---------- Oscilloscope ----------

type Sample struct{ T, A, B float64 }

const scopePoints = 1000 // points per screen

type Scope struct {
	a, b, g int
	buf     []Sample
	head    int
	n       int
	last    float64
}

func newScope(e *elem) *Scope {
	return &Scope{a: e.ix("A"), b: e.ix("B"), g: e.ix("G"), buf: make([]Sample, 2*scopePoints), last: math.Inf(-1)}
}

func (*Scope) Eval(*IO) {}

func (sc *Scope) Record(io *IO) {
	window := io.Part().Num("TB") * 1e-3 * 10
	if io.T()-sc.last < window/scopePoints {
		return
	}
	sc.last = io.T()
	ref := 0.0
	if io.Connected(sc.g) {
		ref = io.V(sc.g)
	}
	s := Sample{T: io.T(), A: math.NaN(), B: math.NaN()}
	if io.Connected(sc.a) {
		s.A = io.V(sc.a) - ref
	}
	if io.Connected(sc.b) {
		s.B = io.V(sc.b) - ref
	}
	sc.buf[sc.head] = s
	sc.head = (sc.head + 1) % len(sc.buf)
	if sc.n < len(sc.buf) {
		sc.n++
	}
}

// Samples returns recorded points, oldest first.
func (sc *Scope) Samples() []Sample {
	out := make([]Sample, 0, sc.n)
	start := (sc.head - sc.n + len(sc.buf)) % len(sc.buf)
	for i := 0; i < sc.n; i++ {
		out = append(out, sc.buf[(start+i)%len(sc.buf)])
	}
	return out
}

// ---------- Word generator ----------

type WGMode int

const (
	WGStop WGMode = iota
	WGCycle
	WGBurst
	WGStep
)

type WordGen struct {
	off   bool // released while a logic converter analyzes the circuit
	out   []int
	Words []uint16
	Mode  WGMode
	Index int
	t0    float64
}

func newWordGen(e *elem) *WordGen {
	out := make([]int, 16)
	for i := range out {
		out[i] = e.ix(strconv.Itoa(i))
	}
	return &WordGen{out: out, Words: ParseWords(e.part.Str("WORDS")), Mode: WGCycle}
}

func ParseWords(s string) []uint16 {
	var w []uint16
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '\n' || r == '\t' || r == ';' }) {
		v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(f), "0x"), 16, 16)
		if err == nil {
			w = append(w, uint16(v))
		}
	}
	if len(w) == 0 {
		w = []uint16{0}
	}
	return w
}

func (wg *WordGen) Eval(io *IO) {
	if wg.off {
		return
	}
	f := io.Part().Num("F")
	if f > 0 && (wg.Mode == WGCycle || wg.Mode == WGBurst) {
		i := int(math.Floor((io.T() - wg.t0) * f))
		if i < 0 {
			i = 0
		}
		if wg.Mode == WGCycle {
			i %= len(wg.Words)
		} else if i >= len(wg.Words) {
			i = len(wg.Words) - 1
		}
		wg.Index = i
	}
	if wg.Index >= len(wg.Words) {
		wg.Index = 0
	}
	io.OutBits(wg.out, int(wg.Words[wg.Index]))
}

// SetMode switches the mode; cycle and burst restart from the first word at time t.
func (wg *WordGen) SetMode(m WGMode, t float64) {
	wg.Mode = m
	if m == WGCycle || m == WGBurst {
		wg.t0 = t
		wg.Index = 0
	}
}

// StepOnce outputs the next word (step mode).
func (wg *WordGen) StepOnce() {
	wg.Mode = WGStep
	wg.Index = (wg.Index + 1) % len(wg.Words)
}

// ---------- Logic converter ----------

type LogicConv struct {
	in     []int
	out    int
	active bool
	n      int
	row    int
}

func newLogicConv(e *elem) *LogicConv {
	return &LogicConv{in: e.ixs("A", "B", "C", "D", "E", "F", "G", "H"), out: e.ix("OUT")}
}

func (lc *LogicConv) Eval(io *IO) {
	if !lc.active {
		return
	}
	for i := 0; i < lc.n; i++ {
		io.Out(lc.in[i], lc.row>>(lc.n-1-i)&1 == 1)
	}
}

// Analyze builds the truth table of the circuit attached to a logic converter.
// Variables are the inputs A.. up to the last connected one; A is the most significant bit.
// The table has '0', '1' or 'x' (output floating) per row.
func (s *Sim) Analyze(partID int) (int, string, error) {
	e := s.byPart[partID]
	if e == nil {
		return 0, "", errors.New("no such part")
	}
	lc, ok := e.dev.(*LogicConv)
	if !ok {
		return 0, "", errors.New("not a logic converter")
	}
	n := 0
	for i, p := range lc.in {
		if e.io.Connected(p) {
			n = i + 1
		}
	}
	if n == 0 {
		return 0, "", errors.New("connect circuit inputs to A, B, C...")
	}
	if !e.io.Connected(lc.out) {
		return 0, "", errors.New("connect circuit output to OUT")
	}
	lc.active, lc.n = true, n
	setWG := func(off bool) {
		for _, el := range s.elems {
			if wg, ok := el.dev.(*WordGen); ok {
				wg.off = off
			}
		}
	}
	setWG(true)
	defer setWG(false)
	table := make([]byte, 1<<n)
	for r := range table {
		lc.row = r
		s.update()
		switch {
		case e.io.Floating(lc.out):
			table[r] = 'x'
		case e.io.Hi(lc.out):
			table[r] = '1'
		default:
			table[r] = '0'
		}
	}
	lc.active = false
	setWG(false)
	s.update()
	return n, string(table), nil
}

// ---------- Logic analyzer ----------

const laDepth = 4096

type LogicAn struct {
	in    []int
	Times []float64
	Words []uint16
	Mask  uint16 // connected channels
	next  float64
}

func newLogicAn(e *elem) *LogicAn {
	in := make([]int, 16)
	for i := range in {
		in[i] = e.ix(strconv.Itoa(i + 1))
	}
	return &LogicAn{in: in}
}

func (*LogicAn) Eval(*IO) {}

func (la *LogicAn) Record(io *IO) {
	rate := io.Part().Num("RATE")
	if rate <= 0 || io.T() < la.next {
		return
	}
	la.next = io.T() + 1/rate
	var w, mask uint16
	for i, p := range la.in {
		if io.Connected(p) {
			mask |= 1 << i
		}
		if io.Lit(p) {
			w |= 1 << i
		}
	}
	la.Mask = mask
	la.Times = append(la.Times, io.T())
	la.Words = append(la.Words, w)
	if len(la.Words) > laDepth {
		cut := len(la.Words) - laDepth
		la.Times = append(la.Times[:0], la.Times[cut:]...)
		la.Words = append(la.Words[:0], la.Words[cut:]...)
	}
}
