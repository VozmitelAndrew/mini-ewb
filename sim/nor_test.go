package sim

import (
	"testing"

	"github.com/VozmitelAndrew/mini-ewb/circuit"
)

func TestNOR7427(t *testing.T) {
	for in := 0; in < 8; in++ {
		c := circuit.New()
		ic := c.Add("7427", 10, 0)
		g := c.Add("nor", 30, 0)
		g.Set("N", "3")
		for i, pin := range []string{"1A", "1B", "1C"} {
			src := c.Add("vcc", 0, 3*i)
			if in>>i&1 == 0 {
				src = c.Add("ground", 0, 3*i)
				c.Connect(circuit.PinRef{Part: src.ID, Pin: "G"}, circuit.PinRef{Part: ic.ID, Pin: pin})
				c.Connect(circuit.PinRef{Part: src.ID, Pin: "G"}, circuit.PinRef{Part: g.ID, Pin: string(rune('1' + i))})
				continue
			}
			c.Connect(circuit.PinRef{Part: src.ID, Pin: "+"}, circuit.PinRef{Part: ic.ID, Pin: pin})
			c.Connect(circuit.PinRef{Part: src.ID, Pin: "+"}, circuit.PinRef{Part: g.ID, Pin: string(rune('1' + i))})
		}
		s := New(c)
		want := in == 0
		if s.PinLit(ic.ID, "1Y") != want || s.PinLit(g.ID, "Y") != want {
			t.Fatalf("inputs %03b: 7427 %v, NOR %v, want %v", in, s.PinLit(ic.ID, "1Y"), s.PinLit(g.ID, "Y"), want)
		}
	}
}
