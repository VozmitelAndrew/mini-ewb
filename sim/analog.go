package sim

import "math"

// Nodal analysis for circuits with op-amps and capacitors.
//
// Unknowns are the voltages of all non-ground nets plus one current per voltage source.
// Capacitors use the backward Euler companion model (conductance C/dt with a parallel current
// source), op-amps are voltage-controlled voltage sources with gain A that saturate at ±VSW.
// Digital parts are not simulated in this mode.

const gmin = 1e-12

type capDev struct {
	a, b int
	c    float64 // farads
	v    float64 // voltage a-b at the last committed step
}

func (*capDev) Eval(*IO) {}

type opampDev struct {
	p, n, out int
	gain, vsw float64
	sat       int // 0 linear, +1 / -1 saturated
}

func (*opampDev) Eval(*IO) {}

type vsource struct {
	a, b int // net ids; -1 is the ground reference
	v    float64
}

func (s *Sim) solveAnalog(commit bool) {
	nNets := len(s.V)
	ground := make([]bool, nNets)
	for _, e := range s.elems {
		if e.part.Kind == "ground" {
			ground[e.nets[0]] = true
		}
	}
	idx := make([]int, nNets)
	n := 0
	for i := range idx {
		if ground[i] {
			idx[i] = -1
		} else {
			idx[i] = n
			n++
		}
	}

	var sources []vsource
	var opamps []*opampDev
	for _, e := range s.elems {
		t := s.T
		switch d := e.dev.(type) {
		case *battery:
			sources = append(sources, vsource{e.nets[d.p], e.nets[d.n], d.v})
		case *source:
			if e.part.Kind == "vcc" {
				sources = append(sources, vsource{e.nets[d.pin], -1, d.v})
			}
		case *funcGen:
			emf := d.emf(t)
			sources = append(sources, vsource{e.nets[d.p], e.nets[d.c], emf}, vsource{e.nets[d.c], e.nets[d.n], emf})
		case *clock:
			v := 0.0
			if d.f > 0 {
				ph := t * d.f
				if ph-math.Floor(ph) < d.duty {
					v = d.v
				}
			}
			sources = append(sources, vsource{e.nets[d.p], e.nets[d.n], v})
		case *opampDev:
			opamps = append(opamps, d)
		}
	}

	size := n + len(sources) + len(opamps)
	var x []float64
	for pass := 0; pass < 8; pass++ {
		G := make([][]float64, size)
		for i := range G {
			G[i] = make([]float64, size)
		}
		rhs := make([]float64, size)
		node := func(net int) int {
			if net < 0 {
				return -1
			}
			return idx[net]
		}
		stampG := func(a, b int, g float64) {
			i, j := node(a), node(b)
			if i >= 0 {
				G[i][i] += g
			}
			if j >= 0 {
				G[j][j] += g
			}
			if i >= 0 && j >= 0 {
				G[i][j] -= g
				G[j][i] -= g
			}
		}
		inject := func(net int, cur float64) { // current into the net
			if i := node(net); i >= 0 {
				rhs[i] += cur
			}
		}
		for i := 0; i < n; i++ {
			G[i][i] += gmin
		}
		for _, e := range s.elems {
			switch d := e.dev.(type) {
			case *resistor:
				if r := e.part.Num("R"); r > 0 {
					stampG(e.nets[d.a], e.nets[d.b], 1/r)
				}
			case *source:
				if e.part.Kind == "pullup" {
					if r := e.part.Num("R"); r > 0 {
						stampG(e.nets[d.pin], -1, 1/r)
						inject(e.nets[d.pin], d.v/r)
					}
				}
			case *capDev:
				if d.c > 0 && s.Dt > 0 {
					g := d.c / s.Dt
					stampG(e.nets[d.a], e.nets[d.b], g)
					inject(e.nets[d.a], g*d.v)
					inject(e.nets[d.b], -g*d.v)
				}
			}
		}
		row := n
		for _, src := range sources {
			if i := node(src.a); i >= 0 {
				G[i][row] += 1
				G[row][i] += 1
			}
			if j := node(src.b); j >= 0 {
				G[j][row] -= 1
				G[row][j] -= 1
			}
			rhs[row] = src.v
			row++
		}
		netOf := map[*opampDev][3]int{}
		for _, e := range s.elems {
			if d, ok := e.dev.(*opampDev); ok {
				netOf[d] = [3]int{e.nets[d.p], e.nets[d.n], e.nets[d.out]}
			}
		}
		for _, d := range opamps {
			nets := netOf[d]
			o := node(nets[2])
			if o >= 0 {
				G[o][row] += 1
				G[row][o] += 1
			}
			if d.sat == 0 {
				if p := node(nets[0]); p >= 0 {
					G[row][p] -= d.gain
				}
				if m := node(nets[1]); m >= 0 {
					G[row][m] += d.gain
				}
			} else {
				rhs[row] = float64(d.sat) * d.vsw
			}
			row++
		}

		var ok bool
		x, ok = solveLinear(G, rhs)
		if !ok {
			s.Shorted = true
			return
		}
		volt := func(net int) float64 {
			if i := node(net); i >= 0 {
				return x[i]
			}
			return 0
		}
		changed := false
		for _, d := range opamps {
			nets := netOf[d]
			want := d.gain * (volt(nets[0]) - volt(nets[1]))
			switch {
			case d.sat == 0 && math.Abs(want) > d.vsw:
				d.sat = 1
				if want < 0 {
					d.sat = -1
				}
				changed = true
			case d.sat != 0 && math.Abs(want) < d.vsw:
				d.sat = 0
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	s.Shorted = false
	for i := range s.V {
		v := 0.0
		if idx[i] >= 0 {
			v = x[idx[i]]
		}
		s.V[i] = v
		s.Short[i] = false
		if ground[i] || s.pinsOnNet[i] > 1 {
			s.Str[i] = Strong
		} else {
			s.Str[i] = Floating
		}
	}
	if commit {
		for _, e := range s.elems {
			if d, ok := e.dev.(*capDev); ok {
				d.v = s.V[e.nets[d.a]] - s.V[e.nets[d.b]]
			}
		}
	}
}

// solveLinear solves G x = b by Gaussian elimination with partial pivoting.
func solveLinear(G [][]float64, b []float64) ([]float64, bool) {
	n := len(b)
	for c := 0; c < n; c++ {
		p := c
		for r := c + 1; r < n; r++ {
			if math.Abs(G[r][c]) > math.Abs(G[p][c]) {
				p = r
			}
		}
		if math.Abs(G[p][c]) < 1e-30 {
			return nil, false
		}
		G[c], G[p] = G[p], G[c]
		b[c], b[p] = b[p], b[c]
		for r := c + 1; r < n; r++ {
			f := G[r][c] / G[c][c]
			if f == 0 {
				continue
			}
			for k := c; k < n; k++ {
				G[r][k] -= f * G[c][k]
			}
			b[r] -= f * b[c]
		}
	}
	x := make([]float64, n)
	for r := n - 1; r >= 0; r-- {
		sum := b[r]
		for k := r + 1; k < n; k++ {
			sum -= G[r][k] * x[k]
		}
		x[r] = sum / G[r][r]
	}
	return x, true
}
