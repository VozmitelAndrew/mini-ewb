// Package sim is a minimal mixed-signal simulator.
//
// There is no circuit physics (no Kirchhoff solver). Every net carries a voltage and a
// drive strength:
//   - Strong drivers (ground, sources, gate outputs, DAC output) fix the voltage of a net.
//   - Batteries and generators are offsets: V(+) - V(-) = E, propagated from known nets.
//   - Resistors and pull-ups are weak: a net without a strong driver takes the value
//     from the other side of the resistor.
//   - A net with no driver at all is floating; logic inputs read it as 1 (TTL), probes as 0.
//
// Gates have zero delay. Each step settles combinational logic to a fixed point,
// then lets edge-triggered elements latch, and repeats until nothing changes.
package sim

import (
	"fmt"
	"math"

	"github.com/VozmitelAndrew/mini-ewb/circuit"
)

type Strength uint8

const (
	Floating Strength = iota
	Weak
	Strong
)

// Threshold separates logic 0 and 1, volts.
const Threshold = 2.5

const (
	maxSettle = 100
	maxCommit = 64
	maxSteps  = 500000
)

type Device interface{ Eval(io *IO) }

// linker devices add voltage offsets (batteries, generators) or weak links (resistors).
type linker interface{ Links(io *IO) }

// committer devices hold state that changes on settled inputs (edges, latches).
type committer interface{ Commit(io *IO) bool }

// recorder devices sample nets after every time step.
type recorder interface{ Record(io *IO) }

type elem struct {
	part   *circuit.Part
	pins   []circuit.PinDef
	pinIdx map[string]int
	nets   []int
	dev    Device
	io     IO
}

func (e *elem) ix(name string) int {
	i, ok := e.pinIdx[name]
	if !ok {
		panic(fmt.Sprintf("sim: part %s has no pin %q", e.part.Kind, name))
	}
	return i
}

func (e *elem) ixs(names ...string) []int {
	r := make([]int, len(names))
	for i, n := range names {
		r[i] = e.ix(n)
	}
	return r
}

type link struct {
	a, b int
	v    float64 // V(a) - V(b) = v
	weak bool
}

func (l link) other(n int, vn float64) (int, float64) {
	if l.weak {
		if n == l.a {
			return l.b, vn
		}
		return l.a, vn
	}
	if n == l.a {
		return l.b, vn - l.v
	}
	return l.a, vn + l.v
}

type Sim struct {
	C  *circuit.Circuit
	T  float64
	Dt float64

	elems  []*elem
	byPart map[int]*elem

	V         []float64
	Str       []Strength
	Short     []bool
	pinsOnNet []int

	drvV   []float64
	drvS   []Strength
	drvBad []bool

	links []link
	adj   [][]int
	queue []int
	nV    []float64
	nS    []Strength
	nSh   []bool

	Oscillating bool
	Shorted     bool
}

func New(c *circuit.Circuit) *Sim {
	s := &Sim{C: c, Dt: 1e-6, byPart: map[int]*elem{}}
	for _, p := range c.Parts {
		d := circuit.Defs[p.Kind]
		if d == nil {
			continue
		}
		_, _, pins := d.Geometry(p)
		e := &elem{part: p, pins: pins, pinIdx: map[string]int{}, nets: make([]int, len(pins))}
		for i, pd := range pins {
			e.pinIdx[pd.Name] = i
		}
		e.io = IO{s: s, e: e}
		e.dev = newDevice(e)
		s.elems = append(s.elems, e)
		s.byPart[p.ID] = e
	}
	s.Connect()
	s.update()
	return s
}

// Connect rebuilds nets from wires and switch positions.
func (s *Sim) Connect() {
	off := make([]int, len(s.elems)+1)
	idx := make(map[int]int, len(s.elems))
	for i, e := range s.elems {
		off[i+1] = off[i] + len(e.pins)
		idx[e.part.ID] = i
	}
	uf := newUF(off[len(s.elems)])
	gid := func(r circuit.PinRef) int {
		i, ok := idx[r.Part]
		if !ok {
			return -1
		}
		k, ok := s.elems[i].pinIdx[r.Pin]
		if !ok {
			return -1
		}
		return off[i] + k
	}
	for _, w := range s.C.Wires {
		if a, b := gid(w.A), gid(w.B); a >= 0 && b >= 0 {
			uf.union(a, b)
		}
	}
	for i, e := range s.elems {
		if e.part.Kind == "switch" {
			throw := "A"
			if e.part.Str("state") == "1" {
				throw = "B"
			}
			uf.union(off[i]+e.ix("COM"), off[i]+e.ix(throw))
		}
	}
	ids := map[int]int{}
	for i, e := range s.elems {
		for k := range e.pins {
			r := uf.find(off[i] + k)
			id, ok := ids[r]
			if !ok {
				id = len(ids)
				ids[r] = id
			}
			e.nets[k] = id
		}
	}
	n := len(ids)
	s.V = make([]float64, n)
	s.Str = make([]Strength, n)
	s.Short = make([]bool, n)
	s.pinsOnNet = make([]int, n)
	s.drvV = make([]float64, n)
	s.drvS = make([]Strength, n)
	s.drvBad = make([]bool, n)
	s.adj = make([][]int, n)
	s.nV = make([]float64, n)
	s.nS = make([]Strength, n)
	s.nSh = make([]bool, n)
	for _, e := range s.elems {
		for _, id := range e.nets {
			s.pinsOnNet[id]++
		}
	}
}

// Step advances time by Dt.
func (s *Sim) Step() {
	s.T += s.Dt
	s.update()
	for _, e := range s.elems {
		if r, ok := e.dev.(recorder); ok {
			r.Record(&e.io)
		}
	}
}

// Run advances time by d seconds.
func (s *Sim) Run(d float64) {
	if s.Dt <= 0 {
		s.Dt = 1e-6
	}
	steps := int(d/s.Dt + 0.5)
	if steps > maxSteps {
		steps = maxSteps
	}
	for i := 0; i < steps; i++ {
		s.Step()
	}
}

// Refresh re-evaluates the circuit at the current time (after a switch or instrument change).
func (s *Sim) Refresh() { s.update() }

// Toggle flips a switch and re-evaluates the circuit immediately.
func (s *Sim) Toggle(partID int) {
	e := s.byPart[partID]
	if e == nil || e.part.Kind != "switch" {
		return
	}
	if e.part.Str("state") == "1" {
		e.part.Set("state", "0")
	} else {
		e.part.Set("state", "1")
	}
	s.Connect()
	s.update()
}

func (s *Sim) update() {
	s.collectLinks()
	for r := 0; r < maxCommit; r++ {
		s.settle()
		changed := false
		for _, e := range s.elems {
			if c, ok := e.dev.(committer); ok && c.Commit(&e.io) {
				changed = true
			}
		}
		if !changed {
			return
		}
	}
}

func (s *Sim) collectLinks() {
	s.links = s.links[:0]
	for _, e := range s.elems {
		if l, ok := e.dev.(linker); ok {
			l.Links(&e.io)
		}
	}
	for i := range s.adj {
		s.adj[i] = s.adj[i][:0]
	}
	for i, l := range s.links {
		s.adj[l.a] = append(s.adj[l.a], i)
		if l.b != l.a {
			s.adj[l.b] = append(s.adj[l.b], i)
		}
	}
}

func (s *Sim) settle() {
	for it := 0; it < maxSettle; it++ {
		for i := range s.drvS {
			s.drvS[i] = Floating
			s.drvBad[i] = false
		}
		for _, e := range s.elems {
			e.dev.Eval(&e.io)
		}
		if !s.resolve() {
			s.Oscillating = false
			return
		}
	}
	s.Oscillating = true
}

// resolve computes net voltages from drivers and links; reports whether anything changed.
func (s *Sim) resolve() bool {
	n := len(s.V)
	nV, nS, nSh := s.nV, s.nS, s.nSh
	q := s.queue[:0]
	for i := 0; i < n; i++ {
		nV[i], nS[i], nSh[i] = 0, Floating, s.drvBad[i]
		if s.drvS[i] != Floating {
			nV[i], nS[i] = s.drvV[i], s.drvS[i]
		}
		if nS[i] == Strong {
			q = append(q, i)
		}
	}
	// Strong values spread through sources only.
	for h := 0; h < len(q); h++ {
		a := q[h]
		for _, li := range s.adj[a] {
			l := s.links[li]
			if l.weak {
				continue
			}
			o, v := l.other(a, nV[a])
			if o == a {
				continue
			}
			if nS[o] == Strong {
				if math.Abs(nV[o]-v) > 1e-6 {
					nSh[o] = true
				}
				continue
			}
			nV[o], nS[o] = v, Strong
			q = append(q, o)
		}
	}
	spread := func(q []int) []int {
		for h := 0; h < len(q); h++ {
			a := q[h]
			for _, li := range s.adj[a] {
				o, v := s.links[li].other(a, nV[a])
				if o == a || nS[o] != Floating {
					continue
				}
				nV[o], nS[o] = v, Weak
				q = append(q, o)
			}
		}
		return q
	}
	q = q[:0]
	for i := 0; i < n; i++ {
		if nS[i] != Floating {
			q = append(q, i)
		}
	}
	q = spread(q)
	// A source group with no reference at all: take its minus side as 0 V.
	for _, l := range s.links {
		if !l.weak && nS[l.a] == Floating && nS[l.b] == Floating {
			nV[l.b], nS[l.b] = 0, Weak
			q = spread(append(q[:0], l.b))
		}
	}
	s.queue = q

	changed := false
	shorted := false
	for i := 0; i < n; i++ {
		if nS[i] != s.Str[i] || nSh[i] != s.Short[i] || math.Abs(nV[i]-s.V[i]) > 1e-9 {
			changed = true
		}
		s.V[i], s.Str[i], s.Short[i] = nV[i], nS[i], nSh[i]
		shorted = shorted || nSh[i]
	}
	s.Shorted = shorted
	return changed
}

// --- queries for the UI and tests ---

func (s *Sim) net(partID int, pin string) int {
	e := s.byPart[partID]
	if e == nil {
		return -1
	}
	k, ok := e.pinIdx[pin]
	if !ok {
		return -1
	}
	return e.nets[k]
}

// PinV returns the voltage of the net a pin is on and its drive strength.
func (s *Sim) PinV(partID int, pin string) (float64, Strength) {
	n := s.net(partID, pin)
	if n < 0 {
		return 0, Floating
	}
	return s.V[n], s.Str[n]
}

// PinLit reports a logic 1 the way a probe sees it (floating = off).
func (s *Sim) PinLit(partID int, pin string) bool {
	v, st := s.PinV(partID, pin)
	return st != Floating && v > Threshold
}

// PinConnected reports whether anything else is attached to the pin's net.
func (s *Sim) PinConnected(partID int, pin string) bool {
	n := s.net(partID, pin)
	return n >= 0 && s.pinsOnNet[n] > 1
}

// Diff returns V(a) - V(b) for two pins of a part; an unconnected b counts as ground.
func (s *Sim) Diff(partID int, a, b string) float64 {
	va, _ := s.PinV(partID, a)
	if !s.PinConnected(partID, b) {
		return va
	}
	vb, _ := s.PinV(partID, b)
	return va - vb
}

// Device returns the device model of a part (Scope, WordGen, LogicConv, LogicAn for instruments).
func (s *Sim) Device(partID int) Device {
	if e := s.byPart[partID]; e != nil {
		return e.dev
	}
	return nil
}

// --- IO is the view of the simulator a device gets ---

type IO struct {
	s *Sim
	e *elem
}

func (io *IO) T() float64           { return io.s.T }
func (io *IO) Part() *circuit.Part  { return io.e.part }
func (io *IO) V(p int) float64      { return io.s.V[io.e.nets[p]] }
func (io *IO) Floating(p int) bool  { return io.s.Str[io.e.nets[p]] == Floating }
func (io *IO) Connected(p int) bool { return io.s.pinsOnNet[io.e.nets[p]] > 1 }

// Hi reads a logic input; a floating input reads as 1 like a TTL input.
func (io *IO) Hi(p int) bool {
	n := io.e.nets[p]
	if io.s.Str[n] == Floating {
		return true
	}
	return io.s.V[n] > Threshold
}

// Lit reads a net the way a probe does: floating is 0.
func (io *IO) Lit(p int) bool {
	n := io.e.nets[p]
	return io.s.Str[n] != Floating && io.s.V[n] > Threshold
}

func (io *IO) Bits(pins []int) int {
	v := 0
	for i, p := range pins {
		if io.Hi(p) {
			v |= 1 << i
		}
	}
	return v
}

func (io *IO) Drive(p int, v float64, st Strength) {
	n := io.e.nets[p]
	s := io.s
	switch {
	case st > s.drvS[n]:
		s.drvV[n], s.drvS[n] = v, st
	case st == Strong && s.drvS[n] == Strong && math.Abs(v-s.drvV[n]) > 1e-6:
		s.drvBad[n] = true
	}
}

func (io *IO) Out(p int, hi bool) {
	v := 0.0
	if hi {
		v = 5
	}
	io.Drive(p, v, Strong)
}

func (io *IO) OutBits(pins []int, v int) {
	for i, p := range pins {
		io.Out(p, v>>i&1 == 1)
	}
}

// Link adds a source: V(a) - V(b) = v.
func (io *IO) Link(a, b int, v float64) {
	io.s.links = append(io.s.links, link{a: io.e.nets[a], b: io.e.nets[b], v: v})
}

// WeakLink adds a resistor between two pins.
func (io *IO) WeakLink(a, b int) {
	io.s.links = append(io.s.links, link{a: io.e.nets[a], b: io.e.nets[b], weak: true})
}

type uf []int

func newUF(n int) uf {
	u := make(uf, n)
	for i := range u {
		u[i] = i
	}
	return u
}

func (u uf) find(x int) int {
	for u[x] != x {
		u[x] = u[u[x]]
		x = u[x]
	}
	return x
}

func (u uf) union(a, b int) { u[u.find(a)] = u.find(b) }
