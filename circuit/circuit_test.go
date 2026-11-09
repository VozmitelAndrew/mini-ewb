package circuit

import (
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	c := New()
	g := c.Add("nand", 3, 4)
	g.Set("N", "3")
	v := c.Add("vcc", 0, 0)
	c.Connect(PinRef{v.ID, "+"}, PinRef{g.ID, "3"})
	path := filepath.Join(t.TempDir(), "c.json")
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	l, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Parts) != 2 || len(l.Wires) != 1 || l.NextID != 3 {
		t.Fatalf("parts %d wires %d next %d", len(l.Parts), len(l.Wires), l.NextID)
	}
}

func TestPruneDropsRemovedPins(t *testing.T) {
	c := New()
	g := c.Add("nand", 0, 0)
	g.Set("N", "4")
	v := c.Add("vcc", 0, 0)
	c.Connect(PinRef{v.ID, "+"}, PinRef{g.ID, "4"})
	g.Set("N", "2")
	c.Prune()
	if len(c.Wires) != 0 {
		t.Fatal("wire to removed pin kept")
	}
}

func TestCatalogPinsUnique(t *testing.T) {
	for kind, d := range Defs {
		_, _, pins := d.Geometry(&Part{Kind: kind})
		seen := map[string]bool{}
		for _, p := range pins {
			if seen[p.Name] {
				t.Errorf("%s: duplicate pin %s", kind, p.Name)
			}
			seen[p.Name] = true
		}
	}
}

func TestRemoveOrphanJunctions(t *testing.T) {
	c := New()
	v := c.Add("vcc", 0, 0)
	n1 := c.Add("node", 5, 0)
	n2 := c.Add("node", 9, 0)
	c.Connect(PinRef{v.ID, "+"}, PinRef{n1.ID, "N"})
	c.Connect(PinRef{n1.ID, "N"}, PinRef{n2.ID, "N"})
	c.RemoveWire(1)
	c.RemoveOrphanJunctions()
	if c.Part(n2.ID) != nil || c.Part(n1.ID) == nil {
		t.Fatal("only the junction without wires should be removed")
	}
}
