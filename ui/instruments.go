package ui

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/VozmitelAndrew/mini-ewb/circuit"
	"github.com/VozmitelAndrew/mini-ewb/logic"
	"github.com/VozmitelAndrew/mini-ewb/sim"
)

type convState struct {
	n     int32
	table []byte
	expr  string
	err   string
}

func (a *App) instruments() {
	var parts []*circuit.Part
	for _, p := range a.c.Parts {
		if isInstrument(p.Kind) {
			parts = append(parts, p)
		}
	}
	imgui.SeparatorText("Instruments")
	if len(parts) == 0 {
		imgui.TextUnformatted("No instruments on the circuit.")
		return
	}
	if imgui.BeginTabBar("instruments") {
		for _, p := range parts {
			title := fmt.Sprintf("%s #%d###inst%d", circuit.Defs[p.Kind].Title, p.ID, p.ID)
			var flags imgui.TabItemFlags
			if a.focusTab == p.ID {
				flags = imgui.TabItemFlagsSetSelected
			}
			if imgui.BeginTabItemV(title, nil, flags) {
				imgui.PushIDInt(int32(p.ID))
				switch p.Kind {
				case "scope":
					a.scopeWin(p)
				case "logican":
					a.analyzerWin(p)
				case "wordgen":
					a.wordGenWin(p)
				case "logicconv":
					a.converterWin(p)
				}
				imgui.PopID()
				imgui.EndTabItem()
			}
		}
		imgui.EndTabBar()
	}
	a.focusTab = 0
}

// liveField edits a live property in place (no simulation restart).
func liveField(p *circuit.Part, key, label string) {
	buf := p.Str(key)
	imgui.SetNextItemWidth(70)
	if imgui.InputTextWithHint(label, "", &buf, imgui.InputTextFlagsEnterReturnsTrue, nil) {
		if _, err := strconv.ParseFloat(strings.TrimSpace(buf), 64); err == nil {
			p.Set(key, strings.TrimSpace(buf))
		}
	}
}

func (a *App) cursor(id int, c0, c1 float32) *[2]float32 {
	c, ok := a.cursors[id]
	if !ok {
		c = &[2]float32{c0, c1}
		a.cursors[id] = c
	}
	return c
}

// ---------- oscilloscope ----------

func (a *App) scopeWin(p *circuit.Part) {
	dev, _ := a.s.Device(p.ID).(*sim.Scope)
	if dev == nil {
		return
	}
	pos := func(key string, def float64) float64 {
		v := p.Num(key)
		if v <= 0 {
			return def
		}
		return v
	}
	tb := pos("TB", 1) * 1e-3
	vdiv := [2]float64{pos("VA", 1), pos("VB", 1)}
	ypos := [2]float64{p.Num("PA"), p.Num("PB")}

	size := imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: 300}
	o := imgui.CursorScreenPos()
	imgui.InvisibleButton("plot", size)
	dl := imgui.WindowDrawList()
	end := imgui.Vec2{X: o.X + size.X, Y: o.Y + size.Y}
	dl.AddRectFilled(o, end, rgba(16, 24, 20, 255))
	for i := 0; i <= 10; i++ {
		x := o.X + size.X*float32(i)/10
		dl.AddLine(imgui.Vec2{X: x, Y: o.Y}, imgui.Vec2{X: x, Y: end.Y}, rgba(60, 80, 70, 255))
	}
	for i := 0; i <= 8; i++ {
		y := o.Y + size.Y*float32(i)/8
		dl.AddLine(imgui.Vec2{X: o.X, Y: y}, imgui.Vec2{X: end.X, Y: y}, rgba(60, 80, 70, 255))
	}

	now := a.s.T
	t0 := now - 10*tb
	xOf := func(t float64) float32 { return o.X + float32((t-t0)/(10*tb))*size.X }
	yOf := func(v float64, ch int) float32 {
		y := float64(o.Y) + float64(size.Y)/2 - (v/vdiv[ch]+ypos[ch])*float64(size.Y)/8
		return float32(math.Max(float64(o.Y), math.Min(float64(end.Y), y)))
	}
	samples := dev.Samples()
	first := sort.Search(len(samples), func(i int) bool { return samples[i].T >= t0 })
	colors := [2]uint32{rgba(255, 70, 70, 255), rgba(90, 160, 255, 255)}
	for ch := 0; ch < 2; ch++ {
		var prev imgui.Vec2
		have := false
		for _, s := range samples[first:] {
			v := s.A
			if ch == 1 {
				v = s.B
			}
			if math.IsNaN(v) {
				have = false
				continue
			}
			pt := imgui.Vec2{X: xOf(s.T), Y: yOf(v, ch)}
			if have {
				dl.AddLineArgs(prev, pt, colors[ch], 1.5)
			}
			prev, have = pt, true
		}
	}

	c := a.cursor(p.ID, 2, 8)
	for i, cv := range c {
		x := o.X + size.X*cv/10
		col := rgba(255, 255, 255, 200)
		if i == 1 {
			col = rgba(255, 220, 80, 200)
		}
		dl.AddLine(imgui.Vec2{X: x, Y: o.Y}, imgui.Vec2{X: x, Y: end.Y}, col)
	}
	imgui.SetNextItemWidth(200)
	imgui.SliderFloat("T1 (div)", &c[0], 0, 10)
	imgui.SetNextItemWidth(200)
	imgui.SliderFloat("T2 (div)", &c[1], 0, 10)

	at := func(cv float32) (float64, float64, float64) {
		t := t0 + float64(cv)*tb
		i := sort.Search(len(samples), func(i int) bool { return samples[i].T >= t })
		if i >= len(samples) {
			return t, math.NaN(), math.NaN()
		}
		return t, samples[i].A, samples[i].B
	}
	t1, a1, b1 := at(c[0])
	t2, a2, b2 := at(c[1])
	imgui.TextUnformatted(fmt.Sprintf("T1 %10.4f ms   VA1 %8.4f V   VB1 %8.4f V", t1*1e3, a1, b1))
	imgui.TextUnformatted(fmt.Sprintf("T2 %10.4f ms   VA2 %8.4f V   VB2 %8.4f V", t2*1e3, a2, b2))
	imgui.TextUnformatted(fmt.Sprintf("T2-T1 %7.4f ms  VA2-VA1 %6.4f V  VB2-VB1 %6.4f V", (t2-t1)*1e3, a2-a1, b2-b1))

	liveField(p, "TB", "ms/div")
	imgui.SameLine()
	liveField(p, "VA", "A V/div")
	imgui.SameLine()
	liveField(p, "VB", "B V/div")
	liveField(p, "PA", "A Y pos")
	imgui.SameLine()
	liveField(p, "PB", "B Y pos")
	imgui.TextUnformatted("Channel A red, B blue. Enter applies a value.")
}

// ---------- logic analyzer ----------

func (a *App) analyzerWin(p *circuit.Part) {
	dev, _ := a.s.Device(p.ID).(*sim.LogicAn)
	if dev == nil {
		return
	}
	liveField(p, "RATE", "clock, Hz")
	imgui.SameLine()
	liveField(p, "CPD", "clocks/div")
	var chans []int
	for i := 0; i < 16; i++ {
		if dev.Mask>>i&1 == 1 {
			chans = append(chans, i)
		}
	}
	if len(chans) == 0 {
		imgui.TextUnformatted("Connect inputs 1..16 and run the simulation.")
		return
	}
	n := int(p.Num("CPD") * 10)
	if n < 10 {
		n = 10
	}
	words, times := dev.Words, dev.Times
	if len(words) > n {
		words, times = words[len(words)-n:], times[len(times)-n:]
	}
	const rowH = 24
	size := imgui.Vec2{X: imgui.ContentRegionAvail().X, Y: float32(len(chans)*rowH + 10)}
	o := imgui.CursorScreenPos()
	imgui.InvisibleButton("trace", size)
	dl := imgui.WindowDrawList()
	end := imgui.Vec2{X: o.X + size.X, Y: o.Y + size.Y}
	dl.AddRectFilled(o, end, rgba(10, 10, 10, 255))
	left := float32(34)
	plotW := size.X - left - 4
	xOf := func(i int) float32 { return o.X + left + plotW*float32(i)/float32(n) }
	for d := 0; d <= 10; d++ {
		x := o.X + left + plotW*float32(d)/10
		dl.AddLine(imgui.Vec2{X: x, Y: o.Y}, imgui.Vec2{X: x, Y: end.Y}, rgba(50, 50, 50, 255))
	}
	colors := []uint32{rgba(80, 140, 255, 255), rgba(80, 220, 80, 255), rgba(230, 60, 60, 255), rgba(230, 230, 230, 255), rgba(240, 200, 60, 255)}
	for r, ch := range chans {
		base := o.Y + float32(r*rowH) + rowH
		hi, lo := base-rowH+8, base-4
		dl.AddTextVec2(imgui.Vec2{X: o.X + 4, Y: hi - 2}, rgba(200, 200, 200, 255), strconv.Itoa(ch+1))
		col := colors[r%len(colors)]
		for i := range words {
			y := lo
			if words[i]>>ch&1 == 1 {
				y = hi
			}
			dl.AddLineArgs(imgui.Vec2{X: xOf(i), Y: y}, imgui.Vec2{X: xOf(i + 1), Y: y}, col, 1.5)
			if i > 0 && (words[i]^words[i-1])>>ch&1 == 1 {
				dl.AddLineArgs(imgui.Vec2{X: xOf(i), Y: hi}, imgui.Vec2{X: xOf(i), Y: lo}, col, 1.5)
			}
		}
	}
	c := a.cursor(p.ID, 2, 8)
	for _, cv := range c {
		x := o.X + left + plotW*cv/10
		dl.AddLine(imgui.Vec2{X: x, Y: o.Y}, imgui.Vec2{X: x, Y: end.Y}, rgba(255, 255, 0, 200))
	}
	imgui.SetNextItemWidth(200)
	imgui.SliderFloat("T1 (div)", &c[0], 0, 10)
	imgui.SetNextItemWidth(200)
	imgui.SliderFloat("T2 (div)", &c[1], 0, 10)
	at := func(cv float32) (float64, uint16, bool) {
		i := int(float32(n) * cv / 10)
		if i >= len(words) {
			return 0, 0, false
		}
		return times[i], words[i], true
	}
	t1, w1, ok1 := at(c[0])
	t2, w2, ok2 := at(c[1])
	if ok1 && ok2 {
		imgui.TextUnformatted(fmt.Sprintf("T1 %.4f ms  %04X     T2 %.4f ms  %04X     T2-T1 %.4f ms",
			t1*1e3, w1, t2*1e3, w2, (t2-t1)*1e3))
	}
}

// ---------- word generator ----------

func (a *App) wordGenWin(p *circuit.Part) {
	wg, _ := a.s.Device(p.ID).(*sim.WordGen)
	if wg == nil {
		return
	}
	w := wg.Words[wg.Index%len(wg.Words)]
	imgui.TextUnformatted(fmt.Sprintf("word %d of %d:  %04X   %016b", wg.Index, len(wg.Words), w, w))
	modes := []struct {
		name string
		m    sim.WGMode
	}{{"Cycle", sim.WGCycle}, {"Burst", sim.WGBurst}, {"Stop", sim.WGStop}}
	for _, m := range modes {
		label := m.name
		if wg.Mode == m.m {
			label = "[" + m.name + "]"
		}
		if imgui.Button(label + "##mode") {
			wg.SetMode(m.m, a.s.T)
			a.s.Refresh()
		}
		imgui.SameLine()
	}
	if imgui.Button("Step") {
		wg.StepOnce()
		a.s.Refresh()
	}
	liveField(p, "F", "frequency, Hz")

	buf, ok := a.wgText[p.ID]
	if !ok {
		s := p.Str("WORDS")
		buf = &s
		a.wgText[p.ID] = buf
	}
	imgui.TextUnformatted("Words, hex, separated by spaces:")
	imgui.InputTextMultiline("##words", buf, imgui.Vec2{X: -1, Y: 110}, 0, nil)
	if imgui.Button("Apply words") {
		p.Set("WORDS", strings.TrimSpace(*buf))
		a.dirty = true
	}
	imgui.SameLine()
	for _, bits := range []int{2, 3, 4, 5} {
		if imgui.Button(fmt.Sprintf("Count %d bits", bits)) {
			parts := make([]string, 1<<bits)
			for i := range parts {
				parts[i] = fmt.Sprintf("%X", i)
			}
			*buf = strings.Join(parts, " ")
			p.Set("WORDS", *buf)
			a.dirty = true
		}
		imgui.SameLine()
	}
	imgui.NewLine()
	imgui.TextUnformatted("Output 0 is the least significant bit.")
}

// ---------- logic converter ----------

func (a *App) converterWin(p *circuit.Part) {
	st, ok := a.conv[p.ID]
	if !ok {
		st = &convState{n: int32(p.Num("NV"))}
		if st.n < 1 || st.n > 8 {
			st.n = 3
		}
		st.table = []byte(p.Str("TABLE"))
		a.conv[p.ID] = st
	}
	if imgui.Button("Circuit -> Table") {
		n, table, err := a.s.Analyze(p.ID)
		if err != nil {
			st.err = err.Error()
		} else {
			st.n, st.table, st.err, st.expr = int32(n), []byte(table), "", ""
		}
	}
	imgui.SameLine()
	if imgui.Button("Table -> Simplified") {
		st.expr = logic.Format(int(st.n), logic.Minimize(int(st.n), string(st.table)))
	}
	imgui.SameLine()
	if imgui.Button("Clear") {
		st.table, st.expr, st.err = nil, "", ""
	}
	imgui.SetNextItemWidth(100)
	imgui.InputInt("variables", &st.n)
	if st.n < 1 {
		st.n = 1
	}
	if st.n > 8 {
		st.n = 8
	}
	rows := 1 << st.n
	if len(st.table) != rows {
		t := make([]byte, rows)
		for i := range t {
			t[i] = '0'
			if i < len(st.table) {
				t[i] = st.table[i]
			}
		}
		st.table = t
	}
	p.Set("NV", strconv.Itoa(int(st.n)))
	p.Set("TABLE", string(st.table))

	imgui.SetNextItemWidth(-1)
	imgui.InputTextWithHint("##expr", "simplified expression", &st.expr, imgui.InputTextFlagsReadOnly, nil)
	if st.err != "" {
		imgui.TextUnformatted("! " + st.err)
	}
	imgui.TextUnformatted("Click a value to change it: 0 -> 1 -> x. A is the most significant bit.")
	header := ""
	for i := 0; i < int(st.n); i++ {
		header += string(rune('A' + i))
	}
	imgui.TextUnformatted(fmt.Sprintf("%3s  %s  OUT", "#", header))
	for r := 0; r < rows; r++ {
		imgui.TextUnformatted(fmt.Sprintf("%3d  %0*b ", r, int(st.n), r))
		imgui.SameLine()
		if imgui.Button(fmt.Sprintf("%c##row%d", st.table[r], r)) {
			switch st.table[r] {
			case '0':
				st.table[r] = '1'
			case '1':
				st.table[r] = 'x'
			default:
				st.table[r] = '0'
			}
		}
	}
}
