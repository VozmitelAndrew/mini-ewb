package ui

import (
	"fmt"
	"math"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/VozmitelAndrew/mini-ewb/circuit"
	"github.com/VozmitelAndrew/mini-ewb/sim"
)

var (
	colBg      = rgba(250, 250, 248, 255)
	colGrid    = rgba(225, 225, 225, 255)
	colInk     = rgba(30, 30, 30, 255)
	colSel     = rgba(30, 110, 230, 255)
	colHigh    = rgba(210, 40, 40, 255)
	colLow     = rgba(40, 70, 160, 255)
	colFloat   = rgba(150, 150, 150, 255)
	colLabel   = rgba(90, 90, 200, 255)
	colPin     = rgba(60, 60, 60, 255)
	colSegOff  = rgba(70, 30, 30, 255)
	colSegOn   = rgba(255, 60, 60, 255)
	colPreview = rgba(30, 110, 230, 160)
)

func (a *App) toScreen(x, y float32) imgui.Vec2 {
	return imgui.Vec2{X: a.origin.X + a.pan.X + x*a.zoom, Y: a.origin.Y + a.pan.Y + y*a.zoom}
}

func (a *App) toWorld(p imgui.Vec2) imgui.Vec2 {
	return imgui.Vec2{X: (p.X - a.origin.X - a.pan.X) / a.zoom, Y: (p.Y - a.origin.Y - a.pan.Y) / a.zoom}
}

func (a *App) zoomAt(screen imgui.Vec2, k float32) {
	z := a.zoom * k
	if z < 6 || z > 60 {
		return
	}
	w := a.toWorld(screen)
	a.zoom = z
	a.pan = imgui.Vec2{X: screen.X - a.origin.X - w.X*z, Y: screen.Y - a.origin.Y - w.Y*z}
}

func (a *App) pinPos(r circuit.PinRef) (imgui.Vec2, bool) {
	p := a.c.Part(r.Part)
	if p == nil {
		return imgui.Vec2{}, false
	}
	_, _, pins := p.Geometry()
	for _, pd := range pins {
		if pd.Name == r.Pin {
			return imgui.Vec2{X: float32(p.X + pd.DX), Y: float32(p.Y + pd.DY)}, true
		}
	}
	return imgui.Vec2{}, false
}

func (a *App) canvas() {
	{
		a.origin = imgui.CursorScreenPos()
		size := imgui.ContentRegionAvail()
		if size.X < 10 {
			size.X = 10
		}
		if size.Y < 10 {
			size.Y = 10
		}
		imgui.InvisibleButton("canvas", size)
		hovered := imgui.IsItemHovered()
		dl := imgui.WindowDrawList()
		end := imgui.Vec2{X: a.origin.X + size.X, Y: a.origin.Y + size.Y}
		dl.AddRectFilled(a.origin, end, colBg)
		a.drawGrid(dl, end)
		for i, w := range a.c.Wires {
			a.drawWire(dl, w, i == a.selWire)
		}
		for _, p := range a.c.Parts {
			a.drawPart(dl, p)
		}
		a.drawPreview(dl)
		a.mouse(hovered)
	}
}

func (a *App) drawGrid(dl *imgui.DrawList, end imgui.Vec2) {
	if a.zoom < 9 {
		return
	}
	start := a.toWorld(a.origin)
	stop := a.toWorld(end)
	for x := float32(math.Floor(float64(start.X))); x <= stop.X; x++ {
		for y := float32(math.Floor(float64(start.Y))); y <= stop.Y; y++ {
			p := a.toScreen(x, y)
			dl.AddRectFilled(p, imgui.Vec2{X: p.X + 1, Y: p.Y + 1}, colGrid)
		}
	}
}

func (a *App) netColor(r circuit.PinRef) uint32 {
	v, st := a.s.PinV(r.Part, r.Pin)
	switch {
	case st == sim.Floating:
		return colFloat
	case v > sim.Threshold:
		return colHigh
	default:
		return colLow
	}
}

// wireRoute returns the corner of an L-shaped wire: horizontal first, then vertical.
func wireRoute(p1, p2 imgui.Vec2) imgui.Vec2 { return imgui.Vec2{X: p2.X, Y: p1.Y} }

func (a *App) drawWire(dl *imgui.DrawList, w circuit.Wire, sel bool) {
	p1, ok1 := a.pinPos(w.A)
	p2, ok2 := a.pinPos(w.B)
	if !ok1 || !ok2 {
		return
	}
	c := wireRoute(p1, p2)
	col, th := a.netColor(w.A), float32(1.5)
	if sel {
		col, th = colSel, 3
	}
	s1, sc, s2 := a.toScreen(p1.X, p1.Y), a.toScreen(c.X, c.Y), a.toScreen(p2.X, p2.Y)
	dl.AddLineArgs(s1, sc, col, th)
	dl.AddLineArgs(sc, s2, col, th)
}

func (a *App) drawPreview(dl *imgui.DrawList) {
	m := a.toWorld(imgui.MousePos())
	if a.wireFrom != nil {
		if p1, ok := a.pinPos(*a.wireFrom); ok {
			p2 := imgui.Vec2{X: float32(math.Round(float64(m.X))), Y: float32(math.Round(float64(m.Y)))}
			c := wireRoute(p1, p2)
			dl.AddLineArgs(a.toScreen(p1.X, p1.Y), a.toScreen(c.X, c.Y), colPreview, 2)
			dl.AddLineArgs(a.toScreen(c.X, c.Y), a.toScreen(p2.X, p2.Y), colPreview, 2)
		}
	}
	if a.placing != "" {
		d := circuit.Defs[a.placing]
		tmp := &circuit.Part{Kind: a.placing, X: int(math.Round(float64(m.X))), Y: int(math.Round(float64(m.Y)))}
		w, h, _ := d.Geometry(tmp)
		dl.AddRectV(a.toScreen(float32(tmp.X), float32(tmp.Y)), a.toScreen(float32(tmp.X+w), float32(tmp.Y+h)), colPreview, 0, 2, 0)
	}
}

// ---------- mouse ----------

func (a *App) mouse(hovered bool) {
	m := imgui.MousePos()
	w := a.toWorld(m)
	snap := func(v float32) int { return int(math.Round(float64(v))) }

	if hovered {
		if wheel := imgui.CurrentIO().MouseWheel(); wheel != 0 {
			k := float32(1.1)
			if wheel < 0 {
				k = 1 / k
			}
			a.zoomAt(m, k)
		}
		if a.hand {
			imgui.SetMouseCursor(imgui.MouseCursorHand)
		}
		if imgui.IsMouseClickedBool(imgui.MouseButtonRight) || imgui.IsMouseClickedBool(imgui.MouseButtonMiddle) ||
			(a.hand && imgui.IsMouseClickedBool(imgui.MouseButtonLeft)) {
			if imgui.IsMouseClickedBool(imgui.MouseButtonRight) {
				a.placing, a.wireFrom = "", nil
			}
			a.panning = true
			a.lastMouse = m
		}
	}
	if a.panning {
		if imgui.IsMouseDown(imgui.MouseButtonRight) || imgui.IsMouseDown(imgui.MouseButtonMiddle) ||
			(a.hand && imgui.IsMouseDown(imgui.MouseButtonLeft)) {
			a.pan.X += m.X - a.lastMouse.X
			a.pan.Y += m.Y - a.lastMouse.Y
			a.lastMouse = m
		} else {
			a.panning = false
		}
	}

	if a.dragging {
		if imgui.IsMouseDown(imgui.MouseButtonLeft) {
			if p := a.c.Part(a.selPart); p != nil {
				p.X, p.Y = snap(w.X-a.dragOff.X), snap(w.Y-a.dragOff.Y)
			}
		} else {
			a.dragging = false
			// A junction clicked without moving starts a wire from it.
			if a.pendingWire != nil {
				if p := a.c.Part(a.pendingWire.Part); p != nil && float32(p.X) == a.pendingAt.X && float32(p.Y) == a.pendingAt.Y {
					a.wireFrom = a.pendingWire
				}
				a.pendingWire = nil
			}
		}
	}

	if a.hand || !hovered || !imgui.IsMouseClickedBool(imgui.MouseButtonLeft) {
		return
	}
	if a.placing != "" {
		p := a.c.Add(a.placing, snap(w.X), snap(w.Y))
		a.selPart, a.selWire = p.ID, -1
		a.placing = ""
		a.dirty = true
		return
	}
	if pin, ok := a.hitPin(w); ok {
		// A junction is selected and can be dragged; a click without dragging starts a wire.
		if n := a.c.Part(pin.Part); a.wireFrom == nil && n != nil && n.Kind == "node" {
			a.selPart, a.selWire = n.ID, -1
			a.dragging = true
			a.dragOff = imgui.Vec2{X: w.X - float32(n.X), Y: w.Y - float32(n.Y)}
			a.pendingWire = &pin
			a.pendingAt = imgui.Vec2{X: float32(n.X), Y: float32(n.Y)}
			return
		}
		if a.wireFrom == nil {
			a.wireFrom = &pin
		} else {
			if a.c.Connect(*a.wireFrom, pin) {
				a.dirty = true
			}
			a.wireFrom = nil
		}
		return
	}
	if a.wireFrom != nil {
		n := a.c.Add("node", snap(w.X), snap(w.Y))
		ref := circuit.PinRef{Part: n.ID, Pin: "N"}
		a.c.Connect(*a.wireFrom, ref)
		a.wireFrom = &ref
		a.dirty = true
		return
	}
	if p := a.hitPart(w); p != nil {
		a.selPart, a.selWire = p.ID, -1
		a.dragging = true
		a.dragOff = imgui.Vec2{X: w.X - float32(p.X), Y: w.Y - float32(p.Y)}
		if isInstrument(p.Kind) {
			a.focusTab = p.ID
		}
		return
	}
	a.selPart = 0
	a.selWire = a.hitWire(w)
}

func dist(a, b imgui.Vec2) float32 {
	return float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
}

func (a *App) hitPin(w imgui.Vec2) (circuit.PinRef, bool) {
	best, bestD := circuit.PinRef{}, float32(0.4)
	found := false
	for _, p := range a.c.Parts {
		_, _, pins := p.Geometry()
		for _, pd := range pins {
			d := dist(w, imgui.Vec2{X: float32(p.X + pd.DX), Y: float32(p.Y + pd.DY)})
			if d < bestD {
				best, bestD, found = circuit.PinRef{Part: p.ID, Pin: pd.Name}, d, true
			}
		}
	}
	return best, found
}

func (a *App) hitPart(w imgui.Vec2) *circuit.Part {
	for i := len(a.c.Parts) - 1; i >= 0; i-- {
		p := a.c.Parts[i]
		pw, ph, _ := p.Geometry()
		x0, y0 := float32(p.X), float32(p.Y)
		if p.Kind == "node" {
			if dist(w, imgui.Vec2{X: x0, Y: y0}) < 0.5 {
				return p
			}
			continue
		}
		if w.X >= x0 && w.X <= x0+float32(pw) && w.Y >= y0 && w.Y <= y0+float32(ph) {
			return p
		}
	}
	return nil
}

func segDist(p, a, b imgui.Vec2) float32 {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := dx*dx + dy*dy
	t := float32(0)
	if l > 0 {
		t = ((p.X-a.X)*dx + (p.Y-a.Y)*dy) / l
		t = float32(math.Max(0, math.Min(1, float64(t))))
	}
	return dist(p, imgui.Vec2{X: a.X + t*dx, Y: a.Y + t*dy})
}

func (a *App) hitWire(w imgui.Vec2) int {
	for i, wr := range a.c.Wires {
		p1, ok1 := a.pinPos(wr.A)
		p2, ok2 := a.pinPos(wr.B)
		if !ok1 || !ok2 {
			continue
		}
		c := wireRoute(p1, p2)
		if segDist(w, p1, c) < 0.3 || segDist(w, c, p2) < 0.3 {
			return i
		}
	}
	return -1
}

// ---------- parts ----------

func (a *App) drawPart(dl *imgui.DrawList, p *circuit.Part) {
	w, h, pins := p.Geometry()
	x, y := float32(p.X), float32(p.Y)
	min, max := a.toScreen(x, y), a.toScreen(x+float32(w), y+float32(h))
	ink := colInk
	if p.ID == a.selPart {
		ink = colSel
	}
	z := a.zoom
	mid := imgui.Vec2{X: (min.X + max.X) / 2, Y: (min.Y + max.Y) / 2}
	boxed := false

	switch p.Kind {
	case "node":
		dl.AddCircleFilled(min, 3.5, ink)
	case "ground":
		top := a.toScreen(x+1, y)
		bar := top.Y + z*0.8
		dl.AddLineArgs(top, imgui.Vec2{X: top.X, Y: bar}, ink, 1.5)
		for i, k := range []float32{0.9, 0.55, 0.2} {
			yy := bar + float32(i)*z*0.3
			dl.AddLineArgs(imgui.Vec2{X: top.X - k*z, Y: yy}, imgui.Vec2{X: top.X + k*z, Y: yy}, ink, 1.5)
		}
	case "vcc", "pullup":
		bot := a.toScreen(x+1, y+float32(h))
		barY := min.Y + z*0.6
		dl.AddLineArgs(imgui.Vec2{X: bot.X - z*0.7, Y: barY}, imgui.Vec2{X: bot.X + z*0.7, Y: barY}, ink, 2)
		if p.Kind == "pullup" {
			r0, r1 := barY+z*0.5, bot.Y-z*0.3
			dl.AddLineArgs(imgui.Vec2{X: bot.X, Y: barY}, imgui.Vec2{X: bot.X, Y: r0}, ink, 1.5)
			dl.AddRectV(imgui.Vec2{X: bot.X - z*0.25, Y: r0}, imgui.Vec2{X: bot.X + z*0.25, Y: r1}, ink, 0, 1.5, 0)
			dl.AddLineArgs(imgui.Vec2{X: bot.X, Y: r1}, bot, ink, 1.5)
			a.text(dl, imgui.Vec2{X: max.X + 3, Y: min.Y}, colLabel, fmt.Sprintf("%s/%sV", si(p.Num("R"), "Ohm"), num(p.Num("V"))))
		} else {
			dl.AddLineArgs(imgui.Vec2{X: bot.X, Y: barY}, bot, ink, 1.5)
			a.text(dl, imgui.Vec2{X: max.X + 3, Y: min.Y}, colLabel, "+"+num(p.Num("V"))+"V")
		}
	case "battery":
		top, bot := a.toScreen(x+1, y), a.toScreen(x+1, y+float32(h))
		c := mid.Y
		dl.AddLineArgs(top, imgui.Vec2{X: top.X, Y: c - z*0.2}, ink, 1.5)
		dl.AddLineArgs(imgui.Vec2{X: top.X - z*0.8, Y: c - z*0.2}, imgui.Vec2{X: top.X + z*0.8, Y: c - z*0.2}, ink, 1.5)
		dl.AddLineArgs(imgui.Vec2{X: top.X - z*0.4, Y: c + z*0.2}, imgui.Vec2{X: top.X + z*0.4, Y: c + z*0.2}, ink, 3)
		dl.AddLineArgs(imgui.Vec2{X: top.X, Y: c + z*0.2}, bot, ink, 1.5)
		a.text(dl, imgui.Vec2{X: max.X + 3, Y: c - 7}, colLabel, num(p.Num("V"))+" V")
	case "resistor", "capacitor":
		hor := p.Num("HOR") == 1
		var p1, p2 imgui.Vec2
		if hor {
			p1, p2 = a.toScreen(x, y+1), a.toScreen(x+float32(w), y+1)
		} else {
			p1, p2 = a.toScreen(x+1, y), a.toScreen(x+1, y+float32(h))
		}
		// unit vector along the part and across it
		ax, ay := float32(0), float32(1)
		if hor {
			ax, ay = 1, 0
		}
		c := imgui.Vec2{X: (p1.X + p2.X) / 2, Y: (p1.Y + p2.Y) / 2}
		along := func(d float32) imgui.Vec2 { return imgui.Vec2{X: c.X + ax*d, Y: c.Y + ay*d} }
		cross := func(at imgui.Vec2, d float32) imgui.Vec2 { return imgui.Vec2{X: at.X + ay*d, Y: at.Y + ax*d} }
		label := si(p.Num("R"), "Ohm")
		if p.Kind == "resistor" {
			e1, e2 := along(-z), along(z)
			dl.AddLineArgs(p1, e1, ink, 1.5)
			dl.AddLineArgs(e2, p2, ink, 1.5)
			r0, r1 := cross(e1, -z*0.3), cross(e2, z*0.3)
			dl.AddRectV(imgui.Vec2{X: minf(r0.X, r1.X), Y: minf(r0.Y, r1.Y)}, imgui.Vec2{X: maxf(r0.X, r1.X), Y: maxf(r0.Y, r1.Y)}, ink, 0, 1.5, 0)
		} else {
			label = si(p.Num("C")*1e-6, "F")
			e1, e2 := along(-z*0.2), along(z*0.2)
			dl.AddLineArgs(p1, e1, ink, 1.5)
			dl.AddLineArgs(e2, p2, ink, 1.5)
			dl.AddLineArgs(cross(e1, -z*0.8), cross(e1, z*0.8), ink, 2)
			dl.AddLineArgs(cross(e2, -z*0.8), cross(e2, z*0.8), ink, 2)
		}
		if hor {
			a.textC(dl, imgui.Vec2{X: c.X, Y: min.Y - 6}, colLabel, label)
		} else {
			a.text(dl, imgui.Vec2{X: max.X + 3, Y: mid.Y - 7}, colLabel, label)
		}
	case "switch", "tdswitch":
		com := a.toScreen(x+4, y+2)
		ta, tb := a.toScreen(x, y+1), a.toScreen(x, y+3)
		pivot := imgui.Vec2{X: com.X - z, Y: com.Y}
		dl.AddLineArgs(com, pivot, ink, 1.5)
		toB := p.Str("state") == "1"
		if p.Kind == "tdswitch" {
			ms := a.s.T * 1e3
			toB = !(ms >= p.Num("TON") && ms < p.Num("TOFF"))
		}
		target := ta
		if toB {
			target = tb
		}
		lever := imgui.Vec2{X: target.X + z, Y: target.Y}
		dl.AddLineArgs(ta, imgui.Vec2{X: ta.X + z, Y: ta.Y}, ink, 1.5)
		dl.AddLineArgs(tb, imgui.Vec2{X: tb.X + z, Y: tb.Y}, ink, 1.5)
		dl.AddLineArgs(pivot, lever, ink, 2)
		dl.AddCircleFilled(pivot, 2.5, ink)
		if p.Kind == "switch" {
			a.text(dl, imgui.Vec2{X: min.X + z, Y: min.Y - 14}, colLabel, "["+p.Str("key")+"]")
		} else {
			a.text(dl, imgui.Vec2{X: min.X, Y: min.Y - 14}, colLabel, fmt.Sprintf("%s-%s ms", num(p.Num("TON")), num(p.Num("TOFF"))))
		}
	case "probe":
		r := z * 0.8
		fill := colBg
		if a.s.PinLit(p.ID, "IN") {
			fill = colHigh
		}
		c := imgui.Vec2{X: mid.X + z*0.2, Y: mid.Y}
		dl.AddCircleFilled(c, r, fill)
		dl.AddCircleV(c, r, ink, 0, 1.5)
		dl.AddLineArgs(a.toScreen(x, y+1), imgui.Vec2{X: c.X - r, Y: c.Y}, ink, 1.5)
		if l := p.Str("LABEL"); l != "" {
			a.text(dl, imgui.Vec2{X: c.X + r + 3, Y: c.Y - 7}, colLabel, l)
		}
	case "seg7":
		dl.AddRectFilled(min, max, rgba(20, 20, 20, 255))
		segs := 0
		for i, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
			if a.s.PinLit(p.ID, n) {
				segs |= 1 << i
			}
		}
		a.digit(dl, min, max, segs)
		boxed = true
	case "hexdisp":
		dl.AddRectFilled(min, max, rgba(20, 20, 20, 255))
		v, any := 0, false
		for i, n := range []string{"D0", "D1", "D2", "D3"} {
			if a.s.PinConnected(p.ID, n) {
				any = true
			}
			if a.s.PinLit(p.ID, n) {
				v |= 1 << i
			}
		}
		if any {
			if p.Num("STYLE") == 1 {
				a.glyph(dl, min, max, v)
			} else {
				a.digit(dl, min, max, sim.HexSegments[v])
			}
		}
		if l := p.Str("LABEL"); l != "" {
			a.text(dl, imgui.Vec2{X: max.X + 3, Y: min.Y}, colLabel, l)
		}
		boxed = true
	case "text":
		a.text(dl, min, ink, p.Str("TEXT"))
	case "opamp":
		in1, in2, out := a.toScreen(x, y+1), a.toScreen(x, y+3), a.toScreen(x+6, y+2)
		l, r := min.X+z, max.X-z
		t, b := min.Y, max.Y
		tip := imgui.Vec2{X: r, Y: mid.Y}
		dl.AddLineArgs(imgui.Vec2{X: l, Y: t}, imgui.Vec2{X: l, Y: b}, ink, 1.5)
		dl.AddLineArgs(imgui.Vec2{X: l, Y: t}, tip, ink, 1.5)
		dl.AddLineArgs(imgui.Vec2{X: l, Y: b}, tip, ink, 1.5)
		dl.AddLineArgs(in1, imgui.Vec2{X: l, Y: in1.Y}, ink, 1.5)
		dl.AddLineArgs(in2, imgui.Vec2{X: l, Y: in2.Y}, ink, 1.5)
		dl.AddLineArgs(tip, out, ink, 1.5)
		a.text(dl, imgui.Vec2{X: l + 3, Y: in1.Y - 7}, ink, "-")
		a.text(dl, imgui.Vec2{X: l + 3, Y: in2.Y - 7}, ink, "+")
	case "clock":
		dl.AddCircleV(mid, z*1.4, ink, 0, 1.5)
		s := z * 0.5
		pts := []imgui.Vec2{{X: mid.X - 2*s, Y: mid.Y + s}, {X: mid.X - s, Y: mid.Y + s}, {X: mid.X - s, Y: mid.Y - s},
			{X: mid.X + s, Y: mid.Y - s}, {X: mid.X + s, Y: mid.Y + s}, {X: mid.X + 2*s, Y: mid.Y + s}}
		for i := 0; i+1 < len(pts); i++ {
			dl.AddLineArgs(pts[i], pts[i+1], ink, 1.5)
		}
		dl.AddLineArgs(a.toScreen(x+2, y), imgui.Vec2{X: mid.X, Y: mid.Y - z*1.4}, ink, 1.5)
		dl.AddLineArgs(a.toScreen(x+2, y+4), imgui.Vec2{X: mid.X, Y: mid.Y + z*1.4}, ink, 1.5)
		a.text(dl, imgui.Vec2{X: max.X + 3, Y: mid.Y - 7}, colLabel, fmt.Sprintf("%s/%s%%", si(p.Num("F"), "Hz"), num(p.Num("DUTY"))))
	default:
		boxed = true
		dl.AddRectFilled(min, max, rgba(255, 255, 255, 255))
		switch p.Kind {
		case "not":
			a.text(dl, imgui.Vec2{X: mid.X - 4, Y: mid.Y - 7}, ink, "1")
		case "nand", "and":
			a.text(dl, imgui.Vec2{X: mid.X - 4, Y: mid.Y - 7}, ink, "&")
			if p.Kind == "nand" {
				dl.AddCircleV(imgui.Vec2{X: max.X, Y: a.toScreen(0, y+float32(pins[len(pins)-1].DY)).Y}, 3, ink, 0, 1.5)
			}
		case "or", "nor":
			a.text(dl, imgui.Vec2{X: mid.X - 8, Y: mid.Y - 7}, ink, ">=1")
			if p.Kind == "nor" {
				dl.AddCircleV(imgui.Vec2{X: max.X, Y: a.toScreen(0, y+float32(pins[len(pins)-1].DY)).Y}, 3, ink, 0, 1.5)
			}
		case "xor":
			a.text(dl, imgui.Vec2{X: mid.X - 8, Y: mid.Y - 7}, ink, "=1")
		case "voltmeter":
			a.textC(dl, mid, ink, fmt.Sprintf("%.3f V", a.s.Diff(p.ID, "+", "-")))
		case "funcgen":
			wave := [...]string{"~", "/\\", "_|-"}[int(p.Num("WAVE"))%3]
			a.textC(dl, mid, ink, fmt.Sprintf("%s %s %sV", wave, si(p.Num("F"), "Hz"), num(p.Num("A"))))
		}
	}
	if boxed {
		dl.AddRectV(min, max, ink, 0, 1.5, 0)
		title := circuit.Defs[p.Kind].Title
		if !isGate(p.Kind) && p.Kind != "voltmeter" && p.Kind != "funcgen" {
			if circuit.Defs[p.Kind].Group == "ICs" {
				a.text(dl, imgui.Vec2{X: min.X, Y: max.Y + 2}, ink, title)
			} else {
				a.text(dl, imgui.Vec2{X: min.X, Y: min.Y - 15}, ink, title)
			}
		}
	}

	for _, pd := range pins {
		pp := a.toScreen(x+float32(pd.DX), y+float32(pd.DY))
		dl.AddCircleFilled(pp, 2.5, colPin)
		if !boxed || isGate(p.Kind) || p.Kind == "seg7" || p.Kind == "hexdisp" {
			continue
		}
		ts := imgui.CalcTextSize(pd.Name)
		var tp imgui.Vec2
		switch {
		case pd.DX == 0:
			tp = imgui.Vec2{X: pp.X + 4, Y: pp.Y - ts.Y/2}
		case pd.DX == w:
			tp = imgui.Vec2{X: pp.X - ts.X - 4, Y: pp.Y - ts.Y/2}
		case pd.DY == 0:
			tp = imgui.Vec2{X: pp.X - ts.X/2, Y: pp.Y + 2}
		default:
			tp = imgui.Vec2{X: pp.X - ts.X/2, Y: pp.Y - ts.Y - 2}
		}
		dl.AddTextVec2(tp, colLabel, pd.Name)
	}
}

// digit draws seven segments inside a box; bit 0 = a ... bit 6 = g.
func (a *App) digit(dl *imgui.DrawList, min, max imgui.Vec2, segs int) {
	w, h := max.X-min.X, max.Y-min.Y
	x0, x1 := min.X+w*0.3, min.X+w*0.75
	y0, ym, y1 := min.Y+h*0.15, min.Y+h*0.5, min.Y+h*0.85
	t := w * 0.07
	hseg := func(y float32) (imgui.Vec2, imgui.Vec2) {
		return imgui.Vec2{X: x0 + t, Y: y - t}, imgui.Vec2{X: x1 - t, Y: y + t}
	}
	vseg := func(x, ya, yb float32) (imgui.Vec2, imgui.Vec2) {
		return imgui.Vec2{X: x - t, Y: ya + t}, imgui.Vec2{X: x + t, Y: yb - t}
	}
	rects := [7][2]imgui.Vec2{}
	rects[0][0], rects[0][1] = hseg(y0)
	rects[1][0], rects[1][1] = vseg(x1, y0, ym)
	rects[2][0], rects[2][1] = vseg(x1, ym, y1)
	rects[3][0], rects[3][1] = hseg(y1)
	rects[4][0], rects[4][1] = vseg(x0, ym, y1)
	rects[5][0], rects[5][1] = vseg(x0, y0, ym)
	rects[6][0], rects[6][1] = hseg(ym)
	for i, r := range rects {
		c := colSegOff
		if segs>>i&1 == 1 {
			c = colSegOn
		}
		dl.AddRectFilled(r[0], r[1], c)
	}
}

func (a *App) text(dl *imgui.DrawList, pos imgui.Vec2, col uint32, s string) {
	dl.AddTextVec2(pos, col, s)
}

func (a *App) textC(dl *imgui.DrawList, center imgui.Vec2, col uint32, s string) {
	ts := imgui.CalcTextSize(s)
	dl.AddTextVec2(imgui.Vec2{X: center.X - ts.X/2, Y: center.Y - ts.Y/2}, col, s)
}

func isGate(kind string) bool {
	switch kind {
	case "not", "nand", "and", "or", "nor", "xor":
		return true
	}
	return false
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

// hexGlyphs are 5x7 dot-matrix characters 0-9, A-F; each row is 5 bits, the MSB is the left column.
var hexGlyphs = [16][7]uint8{
	{0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E}, // 0
	{0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E}, // 1
	{0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F}, // 2
	{0x1E, 0x01, 0x01, 0x0E, 0x01, 0x01, 0x1E}, // 3
	{0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02}, // 4
	{0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E}, // 5
	{0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E}, // 6
	{0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08}, // 7
	{0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E}, // 8
	{0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C}, // 9
	{0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11}, // A
	{0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E}, // B
	{0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E}, // C
	{0x1C, 0x12, 0x11, 0x11, 0x11, 0x12, 0x1C}, // D
	{0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F}, // E
	{0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10}, // F
}

// glyph draws a hex character as a lit 5x7 dot matrix centred in the box.
func (a *App) glyph(dl *imgui.DrawList, min, max imgui.Vec2, v int) {
	cell := minf((max.X-min.X)/7, (max.Y-min.Y)/9)
	x0 := (min.X+max.X)/2 - cell*2.5
	y0 := (min.Y+max.Y)/2 - cell*3.5
	pad := cell * 0.12
	for r, bits := range hexGlyphs[v&15] {
		for c := 0; c < 5; c++ {
			col := colSegOff
			if bits>>(4-c)&1 == 1 {
				col = colSegOn
			}
			p := imgui.Vec2{X: x0 + float32(c)*cell + pad, Y: y0 + float32(r)*cell + pad}
			dl.AddRectFilled(p, imgui.Vec2{X: p.X + cell - 2*pad, Y: p.Y + cell - 2*pad}, col)
		}
	}
}
