package circuit

import "strconv"

type PropKind int

const (
	PropNum PropKind = iota
	PropText
	PropKey
)

type PropDef struct {
	Key     string
	Label   string
	Default string
	Kind    PropKind
	// Live properties are read by the simulator on every step, so editing them
	// does not restart the simulation.
	Live bool
}

// PinDef is a pin position in grid cells relative to the part's top-left corner.
type PinDef struct {
	Name   string
	DX, DY int
}

type Def struct {
	Kind  string
	Title string
	Group string
	W, H  int
	Pins  []PinDef
	Props []PropDef
	// Layout overrides W, H and Pins for parts whose pins depend on properties.
	Layout func(p *Part) (w, h int, pins []PinDef)
}

func (d *Def) Geometry(p *Part) (int, int, []PinDef) {
	if d.Layout != nil {
		return d.Layout(p)
	}
	return d.W, d.H, d.Pins
}

var (
	Defs   = map[string]*Def{}
	Order  []string
	Groups = []string{"Sources", "Basic", "Indicators", "Gates", "Flip-flops", "ICs", "Converters", "Instruments"}
)

func reg(d *Def) {
	Defs[d.Kind] = d
	Order = append(Order, d.Kind)
}

func column(names []string, x, y0 int) []PinDef {
	pins := make([]PinDef, len(names))
	for i, n := range names {
		pins[i] = PinDef{n, x, y0 + i}
	}
	return pins
}

func row(names []string, x0, y int) []PinDef {
	pins := make([]PinDef, len(names))
	for i, n := range names {
		pins[i] = PinDef{n, x0 + i, y}
	}
	return pins
}

func numbered(prefix string, from, n int) []string {
	s := make([]string, n)
	for i := range s {
		s[i] = prefix + strconv.Itoa(from+i)
	}
	return s
}

// dip lays out a DIP package: left pins top to bottom (1..n/2), right pins top to bottom (n..n/2+1).
func dip(kind, title string, left, right []string) *Def {
	const w = 8
	pins := append(column(left, 0, 1), column(right, w, 1)...)
	return &Def{Kind: kind, Title: title, Group: "ICs", W: w, H: len(left) + 1, Pins: pins}
}

func num(key, label, def string) PropDef {
	return PropDef{Key: key, Label: label, Default: def, Kind: PropNum}
}
func live(key, label, def string) PropDef {
	return PropDef{Key: key, Label: label, Default: def, Kind: PropNum, Live: true}
}

func init() {
	// Sources
	reg(&Def{Kind: "ground", Title: "Ground", Group: "Sources", W: 2, H: 2,
		Pins: []PinDef{{"G", 1, 0}}})
	reg(&Def{Kind: "vcc", Title: "+Vcc", Group: "Sources", W: 2, H: 2,
		Pins: []PinDef{{"+", 1, 2}}, Props: []PropDef{num("V", "Voltage, V", "5")}})
	reg(&Def{Kind: "pullup", Title: "+V via R", Group: "Sources", W: 2, H: 3,
		Pins: []PinDef{{"OUT", 1, 3}}, Props: []PropDef{num("V", "Voltage, V", "5"), num("R", "Resistance, Ohm", "1000")}})
	reg(&Def{Kind: "battery", Title: "Battery", Group: "Sources", W: 2, H: 4,
		Pins: []PinDef{{"+", 1, 0}, {"-", 1, 4}}, Props: []PropDef{num("V", "Voltage, V", "2")}})
	reg(&Def{Kind: "clock", Title: "Clock", Group: "Sources", W: 4, H: 4,
		Pins:  []PinDef{{"+", 2, 0}, {"-", 2, 4}},
		Props: []PropDef{num("F", "Frequency, Hz", "1000"), num("DUTY", "Duty cycle, %", "50"), num("V", "Voltage, V", "5")}})
	reg(&Def{Kind: "funcgen", Title: "Function generator", Group: "Sources", W: 6, H: 4,
		Pins:  []PinDef{{"-", 1, 4}, {"COM", 3, 4}, {"+", 5, 4}},
		Props: []PropDef{num("F", "Frequency, Hz", "1000"), num("A", "Amplitude, V", "1"), num("OFF", "Offset, V", "0")}})

	// Basic
	reg(&Def{Kind: "resistor", Title: "Resistor", Group: "Basic", W: 2, H: 4,
		Pins: []PinDef{{"1", 1, 0}, {"2", 1, 4}}, Props: []PropDef{num("R", "Resistance, Ohm", "1000")}})
	reg(&Def{Kind: "switch", Title: "Switch", Group: "Basic", W: 4, H: 4,
		Pins: []PinDef{{"COM", 0, 2}, {"A", 4, 1}, {"B", 4, 3}},
		Props: []PropDef{{Key: "key", Label: "Key", Default: "Space", Kind: PropKey},
			{Key: "state", Label: "State (0=A, 1=B)", Default: "0", Kind: PropNum, Live: true}}})
	reg(&Def{Kind: "node", Title: "Junction", Group: "Basic", W: 0, H: 0,
		Pins: []PinDef{{"N", 0, 0}}})

	// Indicators
	reg(&Def{Kind: "probe", Title: "Probe", Group: "Indicators", W: 2, H: 2,
		Pins: []PinDef{{"IN", 1, 2}}})
	reg(&Def{Kind: "seg7", Title: "7-segment", Group: "Indicators", W: 5, H: 8,
		Pins: column([]string{"a", "b", "c", "d", "e", "f", "g"}, 0, 1)})
	reg(&Def{Kind: "hexdisp", Title: "Hex display", Group: "Indicators", W: 5, H: 7,
		Pins: row([]string{"D3", "D2", "D1", "D0"}, 1, 7)})
	reg(&Def{Kind: "voltmeter", Title: "Voltmeter", Group: "Indicators", W: 6, H: 2,
		Pins: []PinDef{{"+", 0, 1}, {"-", 6, 1}}})

	// Gates
	reg(&Def{Kind: "not", Title: "NOT", Group: "Gates", W: 4, H: 2,
		Pins: []PinDef{{"A", 0, 1}, {"Y", 4, 1}}})
	reg(&Def{Kind: "nand", Title: "NAND", Group: "Gates",
		Props: []PropDef{num("N", "Inputs (2-8)", "2")},
		Layout: func(p *Part) (int, int, []PinDef) {
			n := int(p.Num("N"))
			if n < 2 {
				n = 2
			}
			if n > 8 {
				n = 8
			}
			pins := column(numbered("", 1, n), 0, 1)
			pins = append(pins, PinDef{"Y", 4, (n + 1) / 2})
			return 4, n + 1, pins
		}})

	// Flip-flops
	reg(&Def{Kind: "rs", Title: "RS (S', R')", Group: "Flip-flops", W: 5, H: 4,
		Pins: []PinDef{{"S'", 0, 1}, {"R'", 0, 3}, {"Q", 5, 1}, {"Q'", 5, 3}}})
	reg(&Def{Kind: "dff", Title: "D flip-flop", Group: "Flip-flops", W: 5, H: 4,
		Pins: []PinDef{{"D", 0, 1}, {"CLK", 0, 3}, {"S'", 2, 0}, {"R'", 2, 4}, {"Q", 5, 1}, {"Q'", 5, 3}}})

	// ICs
	reg(dip("7404", "7404",
		[]string{"1A", "1Y", "2A", "2Y", "3A", "3Y", "GND"},
		[]string{"VCC", "6A", "6Y", "5A", "5Y", "4A", "4Y"}))
	reg(dip("7447", "7447",
		[]string{"B", "C", "LT'", "BI'/RBO'", "RBI'", "D", "A", "GND"},
		[]string{"VCC", "OF", "OG", "OA", "OB", "OC", "OD", "OE"}))
	reg(dip("74148", "74148",
		[]string{"4", "5", "6", "7", "EI", "A2", "A1", "GND"},
		[]string{"VCC", "EO", "GS", "3", "2", "1", "0", "A0"}))
	reg(dip("74181", "74181",
		[]string{"B0'", "A0'", "S3", "S2", "S1", "S0", "CN", "M", "F0'", "F1'", "F2'", "GND"},
		[]string{"VCC", "A1'", "B1'", "A2'", "B2'", "A3'", "B3'", "G'", "CN+4", "P'", "A=B", "F3'"}))

	// Converters
	reg(&Def{Kind: "dac", Title: "DAC (V)", Group: "Converters", W: 6, H: 9,
		Pins: append(column(numbered("D", 0, 8), 0, 1), PinDef{"+", 2, 9}, PinDef{"-", 4, 9}, PinDef{"OUT", 6, 4})})
	reg(&Def{Kind: "adc", Title: "ADC", Group: "Converters", W: 6, H: 10,
		Pins: append(append(column([]string{"VIN", "VREF+", "VREF-", "SOC", "OE"}, 0, 1),
			column(numbered("D", 0, 8), 6, 1)...), PinDef{"EOC", 6, 9})})

	// Instruments
	reg(&Def{Kind: "scope", Title: "Oscilloscope", Group: "Instruments", W: 6, H: 4,
		Pins: []PinDef{{"A", 1, 4}, {"B", 3, 4}, {"G", 5, 4}},
		Props: []PropDef{live("TB", "Time base, ms/div", "1"),
			live("VA", "Channel A, V/div", "1"), live("VB", "Channel B, V/div", "1"),
			live("PA", "Channel A Y pos, div", "0"), live("PB", "Channel B Y pos, div", "0")}})
	reg(&Def{Kind: "wordgen", Title: "Word generator", Group: "Instruments", W: 6, H: 17,
		Pins: column(numbered("", 0, 16), 6, 1),
		Props: []PropDef{live("F", "Frequency, Hz", "1000"),
			{Key: "WORDS", Label: "Words (hex)", Default: "0 1 2 3 4 5 6 7", Kind: PropText}}})
	reg(&Def{Kind: "logicconv", Title: "Logic converter", Group: "Instruments", W: 10, H: 2,
		Pins: append(row([]string{"A", "B", "C", "D", "E", "F", "G", "H"}, 1, 2), PinDef{"OUT", 9, 2})})
	reg(&Def{Kind: "logican", Title: "Logic analyzer", Group: "Instruments", W: 6, H: 17,
		Pins:  column(numbered("", 1, 16), 0, 1),
		Props: []PropDef{live("RATE", "Clock, Hz", "10000"), live("CPD", "Clocks/div", "20")}})
}
