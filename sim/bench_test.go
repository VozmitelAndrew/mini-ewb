package sim

import "testing"

func BenchmarkADCDACLab6(bm *testing.B) {
	b := newBench(&testing.T{})
	fg := b.add("funcgen", "F", "200", "A", "2")
	b.wire(fg, "COM", b.lo, "G")
	a := b.add("adc")
	b.wire(fg, "+", a, "VIN")
	rp := b.add("battery", "V", "2")
	b.wire(rp, "+", a, "VREF+")
	b.wire(rp, "-", b.lo, "G")
	rn := b.add("battery", "V", "2")
	b.wire(rn, "-", a, "VREF-")
	b.wire(rn, "+", b.lo, "G")
	ck := b.add("clock", "F", "16000")
	b.wire(ck, "-", b.lo, "G")
	b.wire(ck, "+", a, "SOC")
	inv := b.add("not")
	b.wire(ck, "+", inv, "A")
	b.wire(inv, "Y", a, "OE")
	d := b.add("dac")
	for i := 0; i < 8; i++ {
		p := "D" + string(rune('0'+i))
		b.wire(a, p, d, p)
		pr := b.add("probe")
		b.wire(pr, "IN", a, p)
	}
	b.wire(d, "+", a, "VREF+")
	b.wire(d, "-", a, "VREF-")
	sc := b.add("scope")
	b.wire(sc, "A", fg, "+")
	b.wire(sc, "B", d, "OUT")
	s := New(b.c)
	bm.ResetTimer()
	for i := 0; i < bm.N; i++ {
		s.Run(1e-3) // one frame of default sim time: 1000 steps
	}
}
