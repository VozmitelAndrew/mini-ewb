// Package ui is the editor and instrument panels built on Dear ImGui (cimgui-go).
package ui

import (
	"fmt"
	"strings"

	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/VozmitelAndrew/mini-ewb/circuit"
	"github.com/VozmitelAndrew/mini-ewb/sim"
)

type App struct {
	c       *circuit.Circuit
	s       *sim.Sim
	dirty   bool
	running bool

	// canvas view
	origin imgui.Vec2
	pan    imgui.Vec2
	zoom   float32

	// editing state
	placing   string
	selPart   int
	selWire   int
	wireFrom  *circuit.PinRef
	dragging  bool
	dragOff   imgui.Vec2
	panning   bool
	lastMouse imgui.Vec2

	path    string
	status  string
	stepUs  string
	frameMs string

	focusTab int

	paletteW, sideW float32
	splitDrag       int
	splitLast       float32
	hand            bool
	pendingWire     *circuit.PinRef
	pendingAt       imgui.Vec2
	cursors         map[int]*[2]float32
	conv            map[int]*convState
	wgText          map[int]*string
}

func NewApp() *App {
	return &App{
		c:        circuit.New(),
		dirty:    true,
		zoom:     16,
		pan:      imgui.Vec2{X: 40, Y: 40},
		selWire:  -1,
		path:     "circuit.json",
		stepUs:   "1",
		paletteW: 165,
		sideW:    480,
		frameMs:  "1",
		cursors:  map[int]*[2]float32{},
		conv:     map[int]*convState{},
		wgText:   map[int]*string{},
	}
}

func (a *App) Load(path string) error {
	c, err := circuit.Load(path)
	if err != nil {
		return err
	}
	a.c = c
	a.path = path
	a.reset()
	return nil
}

func (a *App) reset() {
	a.dirty = true
	a.selPart, a.selWire = 0, -1
	a.wireFrom = nil
	a.placing = ""
	a.conv = map[int]*convState{}
	a.wgText = map[int]*string{}
}

func (a *App) rebuild() {
	a.c.Prune()
	dt := 1e-6
	if a.s != nil {
		dt = a.s.Dt
	}
	a.s = sim.New(a.c)
	a.s.Dt = dt
	a.dirty = false
}

// Frame is called once per rendered frame.
func (a *App) Frame() {
	// cimgui-go enables multi-viewports, which turns ImGui windows into separate OS windows
	// positioned in desktop coordinates. Everything must live inside the main window, so the
	// feature is switched off before any window is created.
	io := imgui.CurrentIO()
	if io.ConfigFlags()&imgui.ConfigFlagsViewportsEnable != 0 {
		io.SetConfigFlags(io.ConfigFlags() &^ imgui.ConfigFlagsViewportsEnable)
		return
	}
	if a.dirty || a.s == nil {
		a.rebuild()
	}
	if a.running {
		a.s.Dt = parseF(a.stepUs, 1) * 1e-6
		a.s.Run(parseF(a.frameMs, 1) * 1e-3)
	}
	a.keys()

	imgui.SetNextWindowPosV(imgui.Vec2{}, imgui.CondAlways, imgui.Vec2{})
	imgui.SetNextWindowSizeV(imgui.CurrentIO().DisplaySize(), imgui.CondAlways)
	const mainFlags = imgui.WindowFlagsNoTitleBar | imgui.WindowFlagsNoResize | imgui.WindowFlagsNoMove |
		imgui.WindowFlagsNoCollapse | imgui.WindowFlagsNoBringToFrontOnFocus | imgui.WindowFlagsNoSavedSettings |
		imgui.WindowFlagsNoScrollbar | imgui.WindowFlagsNoScrollWithMouse
	// The root panel fills the OS window edge to edge: no rounding, no border.
	imgui.PushStyleVarFloat(imgui.StyleVarWindowRounding, 0)
	imgui.PushStyleVarFloat(imgui.StyleVarWindowBorderSize, 0)
	open := imgui.BeginV("mini-EWB", nil, mainFlags)
	imgui.PopStyleVarV(2)
	if open {
		a.toolbar()
		avail := imgui.ContentRegionAvail()
		a.clampPanels(avail.X)
		shown := func(w float32) float32 {
			if w < minPanel {
				return 0
			}
			return w
		}
		canvasW := avail.X - shown(a.paletteW) - shown(a.sideW) - 2*splitW
		if canvasW < 100 {
			canvasW = 100
		}

		if a.paletteW >= minPanel {
			imgui.BeginChildStrV("parts", imgui.Vec2{X: a.paletteW, Y: avail.Y}, childBorders, 0)
			a.palette()
			imgui.EndChild()
			imgui.SameLineV(0, 0)
		}
		a.splitter("##split-left", avail.Y, 1)
		imgui.SameLineV(0, 0)

		imgui.BeginChildStrV("circuit", imgui.Vec2{X: canvasW, Y: avail.Y}, childBorders,
			imgui.WindowFlagsNoScrollbar|imgui.WindowFlagsNoScrollWithMouse)
		a.canvas()
		imgui.EndChild()
		imgui.SameLineV(0, 0)
		a.splitter("##split-right", avail.Y, 2)
		if a.sideW < minPanel {
			imgui.End()
			return
		}
		imgui.SameLineV(0, 0)

		imgui.BeginChildStrV("side", imgui.Vec2{X: 0, Y: avail.Y}, childBorders, 0)
		a.properties()
		a.instruments()
		imgui.EndChild()
	}
	imgui.End()
}

// childBorders is ImGuiChildFlags_Borders (1 << 0); the Go constant name differs between versions.
const childBorders = imgui.ChildFlags(1)

func (a *App) keys() {
	if imgui.IsAnyItemActive() {
		return
	}
	pressed := map[string]bool{}
	for _, p := range a.c.Parts {
		if p.Kind != "switch" {
			continue
		}
		name := strings.ToUpper(strings.TrimSpace(p.Str("key")))
		if name == "H" {
			continue // reserved for the hand tool
		}
		k, ok := keyOf(name)
		if !ok {
			continue
		}
		if _, seen := pressed[name]; !seen {
			pressed[name] = imgui.IsKeyPressedBoolV(k, false)
		}
		if pressed[name] {
			a.s.Toggle(p.ID)
		}
	}
	if imgui.IsKeyPressedBoolV(imgui.KeyDelete, false) || imgui.IsKeyPressedBoolV(imgui.KeyBackspace, false) {
		a.deleteSelection()
	}
	if imgui.IsKeyPressedBoolV(imgui.KeyH, false) {
		a.hand = !a.hand
	}
	if imgui.IsKeyPressedBoolV(imgui.KeyEscape, false) {
		a.placing, a.wireFrom = "", nil
	}
}

func keyOf(name string) (imgui.Key, bool) {
	switch {
	case name == "SPACE":
		return imgui.KeySpace, true
	case name == "ENTER":
		return imgui.KeyEnter, true
	case len(name) == 1 && name[0] >= 'A' && name[0] <= 'Z':
		return imgui.KeyA + imgui.Key(name[0]-'A'), true
	case len(name) == 1 && name[0] >= '0' && name[0] <= '9':
		return imgui.Key0 + imgui.Key(name[0]-'0'), true
	}
	return 0, false
}

func (a *App) deleteSelection() {
	switch {
	case a.selPart != 0:
		a.c.Remove(a.selPart)
		a.selPart = 0
	case a.selWire >= 0:
		a.c.RemoveWire(a.selWire)
		a.selWire = -1
	default:
		return
	}
	a.c.RemoveOrphanJunctions()
	a.wireFrom, a.pendingWire, a.dragging = nil, nil, false
	a.dirty = true
}

func (a *App) toolbar() {
	{
		label := "Run "
		if a.running {
			label = "Pause"
		}
		if imgui.Button(label) {
			a.running = !a.running
		}
		imgui.SameLine()
		if imgui.Button("Reset") {
			a.dirty = true
		}
		imgui.SameLine()
		if imgui.Button("New") {
			a.c = circuit.New()
			a.running = false
			a.reset()
		}
		imgui.SameLine()
		imgui.SetNextItemWidth(240)
		imgui.InputTextWithHint("##path", "file.json", &a.path, 0, nil)
		imgui.SameLine()
		if imgui.Button("Save") {
			a.c.Prune()
			if err := a.c.Save(a.path); err != nil {
				a.status = err.Error()
			} else {
				a.status = "saved " + a.path
			}
		}
		imgui.SameLine()
		if imgui.Button("Load") {
			if err := a.Load(a.path); err != nil {
				a.status = err.Error()
			} else {
				a.status = "loaded " + a.path
			}
		}
		imgui.SameLine()
		imgui.SetNextItemWidth(60)
		imgui.InputTextWithHint("step, us", "1", &a.stepUs, 0, nil)
		imgui.SameLine()
		imgui.SetNextItemWidth(60)
		imgui.InputTextWithHint("sim ms / frame", "1", &a.frameMs, 0, nil)
		imgui.SameLine()
		hand := "Hand (H)"
		if a.hand {
			hand = "[Hand (H)]"
		}
		if imgui.Button(hand + "##hand") {
			a.hand = !a.hand
		}
		imgui.SameLine()
		if imgui.Button(" - ") {
			a.zoomAt(a.origin, 1/1.25)
		}
		imgui.SameLine()
		if imgui.Button(" + ") {
			a.zoomAt(a.origin, 1.25)
		}

		state := "paused"
		if a.running {
			state = "running"
		}
		msg := fmt.Sprintf("t = %.6f s   %s", a.s.T, state)
		if a.s.Shorted {
			msg += "   SHORT CIRCUIT"
		}
		if a.s.Oscillating {
			msg += "   circuit does not settle"
		}
		if a.hand {
			msg += "   HAND: drag to move the canvas, H to leave"
		} else if a.placing != "" {
			msg += "   placing " + circuit.Defs[a.placing].Title + " (click on canvas, Esc to cancel)"
		} else if a.wireFrom != nil {
			msg += "   wiring: click a pin, empty space adds a junction, Esc cancels"
		}
		if a.status != "" {
			msg += "   | " + a.status
		}
		imgui.TextUnformatted(msg)
	}
}

func (a *App) palette() {
	{
		for _, g := range circuit.Groups {
			imgui.TextUnformatted(g)
			for _, kind := range circuit.Order {
				d := circuit.Defs[kind]
				if d.Group != g {
					continue
				}
				if imgui.ButtonV(d.Title+"##pal", imgui.Vec2{X: -1, Y: 0}) {
					a.placing = kind
					a.wireFrom = nil
				}
			}
			imgui.Separator()
		}
	}
}

func (a *App) properties() {
	imgui.SeparatorText("Properties")
	{
		p := a.c.Part(a.selPart)
		switch {
		case p != nil:
			a.partProps(p)
		case a.selWire >= 0 && a.selWire < len(a.c.Wires):
			imgui.TextUnformatted("Wire")
			if imgui.Button("Delete") {
				a.deleteSelection()
			}
		default:
			imgui.TextUnformatted("Nothing selected.")
			imgui.TextUnformatted("Click a part to edit it.")
			imgui.TextUnformatted("Click pin, then pin: wire.")
			imgui.TextUnformatted("Right drag or H (hand): pan.")
			imgui.TextUnformatted("Wheel: zoom.")
			imgui.TextUnformatted("Del: delete selection.")
		}
	}
}

func (a *App) partProps(p *circuit.Part) {
	d := p.Def()
	imgui.TextUnformatted(fmt.Sprintf("%s  #%d", d.Title, p.ID))
	imgui.PushIDInt(int32(p.ID))
	for _, pd := range d.Props {
		if pd.Key == "WORDS" {
			continue // edited in the word generator window
		}
		buf := p.Str(pd.Key)
		imgui.SetNextItemWidth(110)
		if imgui.InputTextWithHint(pd.Label, "", &buf, imgui.InputTextFlagsEnterReturnsTrue, nil) {
			p.Set(pd.Key, strings.TrimSpace(buf))
			switch {
			case pd.Key == "state":
				a.dirty = true
			case pd.Live, pd.Kind == circuit.PropKey:
			default:
				a.dirty = true
			}
		}
	}
	imgui.PopID()
	if len(d.Props) > 0 {
		imgui.TextUnformatted("(Enter applies a value)")
	}
	if p.Kind == "switch" && imgui.Button("Toggle") {
		a.s.Toggle(p.ID)
	}
	if p.Kind == "voltmeter" {
		imgui.TextUnformatted(fmt.Sprintf("U = %.4f V", a.s.Diff(p.ID, "+", "-")))
	}
	if isInstrument(p.Kind) && imgui.Button("Show panel") {
		a.focusTab = p.ID
	}
	if imgui.Button("Delete part") {
		a.deleteSelection()
	}
}

func isInstrument(kind string) bool {
	switch kind {
	case "scope", "wordgen", "logicconv", "logican":
		return true
	}
	return false
}

const (
	splitW   = 8
	minPanel = 24
)

func (a *App) clampPanels(total float32) {
	clamp := func(v, lo, hi float32) float32 {
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	a.paletteW = clamp(a.paletteW, 0, total*0.4)
	a.sideW = clamp(a.sideW, 0, total*0.7)
}

// splitter is a draggable vertical bar; which = 1 resizes the palette, 2 the right panel.
// Dragging a panel narrower than minPanel hides it; drag the bar back to show it again.
func (a *App) splitter(id string, h float32, which int) {
	imgui.InvisibleButton(id, imgui.Vec2{X: splitW, Y: h})
	hovered, active := imgui.IsItemHovered(), imgui.IsItemActive()
	min, max := imgui.ItemRectMin(), imgui.ItemRectMax()
	col := rgba(70, 70, 75, 255)
	if hovered || active {
		col = rgba(66, 150, 250, 255)
		imgui.SetMouseCursor(imgui.MouseCursorResizeEW)
	}
	imgui.WindowDrawList().AddRectFilled(imgui.Vec2{X: min.X + 3, Y: min.Y}, imgui.Vec2{X: max.X - 3, Y: max.Y}, col)
	m := imgui.MousePos().X
	switch {
	case active && a.splitDrag != which:
		a.splitDrag, a.splitLast = which, m
	case active:
		dx := m - a.splitLast
		a.splitLast = m
		if which == 1 {
			a.paletteW += dx
		} else {
			a.sideW -= dx
		}
	case a.splitDrag == which:
		a.splitDrag = 0
	}
}
