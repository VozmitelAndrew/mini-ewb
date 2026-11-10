// Package circuit describes a schematic: parts placed on a grid and wires between their pins.
package circuit

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type PinRef struct {
	Part int    `json:"part"`
	Pin  string `json:"pin"`
}

type Wire struct {
	A PinRef `json:"a"`
	B PinRef `json:"b"`
}

type Part struct {
	ID    int               `json:"id"`
	Kind  string            `json:"kind"`
	X     int               `json:"x"`
	Y     int               `json:"y"`
	Props map[string]string `json:"props,omitempty"`
}

func (p *Part) Def() *Def { return Defs[p.Kind] }

// Str returns a property value or its default from the catalog.
func (p *Part) Str(key string) string {
	if v, ok := p.Props[key]; ok {
		return v
	}
	if d := p.Def(); d != nil {
		for _, pd := range d.Props {
			if pd.Key == key {
				return pd.Default
			}
		}
	}
	return ""
}

func (p *Part) Num(key string) float64 {
	f, err := strconv.ParseFloat(p.Str(key), 64)
	if err != nil {
		return 0
	}
	return f
}

func (p *Part) Set(key, val string) {
	if p.Props == nil {
		p.Props = map[string]string{}
	}
	p.Props[key] = val
}

func (p *Part) Geometry() (w, h int, pins []PinDef) {
	d := p.Def()
	if d == nil {
		return 1, 1, nil
	}
	return d.Geometry(p)
}

func (p *Part) HasPin(name string) bool {
	_, _, pins := p.Geometry()
	for _, pd := range pins {
		if pd.Name == name {
			return true
		}
	}
	return false
}

type Circuit struct {
	Parts  []*Part `json:"parts"`
	Wires  []Wire  `json:"wires"`
	NextID int     `json:"next_id"`
	// Settings holds editor values stored with the circuit: "step_us", "frame_ms".
	Settings map[string]string `json:"settings,omitempty"`
}

func New() *Circuit { return &Circuit{NextID: 1} }

func (c *Circuit) Add(kind string, x, y int) *Part {
	p := &Part{ID: c.NextID, Kind: kind, X: x, Y: y, Props: map[string]string{}}
	c.NextID++
	c.Parts = append(c.Parts, p)
	return p
}

func (c *Circuit) Part(id int) *Part {
	for _, p := range c.Parts {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Remove deletes a part together with all wires attached to it.
func (c *Circuit) Remove(id int) {
	parts := c.Parts[:0]
	for _, p := range c.Parts {
		if p.ID != id {
			parts = append(parts, p)
		}
	}
	c.Parts = parts
	wires := c.Wires[:0]
	for _, w := range c.Wires {
		if w.A.Part != id && w.B.Part != id {
			wires = append(wires, w)
		}
	}
	c.Wires = wires
}

// Connect adds a wire unless it is degenerate or already present.
func (c *Circuit) Connect(a, b PinRef) bool {
	if a == b {
		return false
	}
	for _, w := range c.Wires {
		if (w.A == a && w.B == b) || (w.A == b && w.B == a) {
			return false
		}
	}
	c.Wires = append(c.Wires, Wire{A: a, B: b})
	return true
}

func (c *Circuit) RemoveWire(i int) {
	if i < 0 || i >= len(c.Wires) {
		return
	}
	c.Wires = append(c.Wires[:i], c.Wires[i+1:]...)
}

// Prune drops wires whose ends no longer exist (deleted part, pin removed by a property change)
// and fixes NextID after loading.
func (c *Circuit) Prune() {
	valid := func(r PinRef) bool {
		p := c.Part(r.Part)
		return p != nil && p.HasPin(r.Pin)
	}
	wires := c.Wires[:0]
	for _, w := range c.Wires {
		if valid(w.A) && valid(w.B) {
			wires = append(wires, w)
		}
	}
	c.Wires = wires
	for _, p := range c.Parts {
		if p.ID >= c.NextID {
			c.NextID = p.ID + 1
		}
	}
}

func (c *Circuit) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func Load(path string) (*Circuit, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := New()
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for _, p := range c.Parts {
		if Defs[p.Kind] == nil {
			return nil, fmt.Errorf("unknown part kind %q", p.Kind)
		}
		if p.Props == nil {
			p.Props = map[string]string{}
		}
	}
	c.Prune()
	return c, nil
}

// RemoveOrphanJunctions deletes junction nodes that have no wires left.
func (c *Circuit) RemoveOrphanJunctions() {
	used := map[int]bool{}
	for _, w := range c.Wires {
		used[w.A.Part] = true
		used[w.B.Part] = true
	}
	parts := c.Parts[:0]
	for _, p := range c.Parts {
		if p.Kind != "node" || used[p.ID] {
			parts = append(parts, p)
		}
	}
	c.Parts = parts
}
