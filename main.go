package main

import (
	"log"
	"os"

	"github.com/AllenDang/cimgui-go/backend"
	"github.com/AllenDang/cimgui-go/backend/glfwbackend"
	"github.com/AllenDang/cimgui-go/imgui"

	"github.com/VozmitelAndrew/mini-ewb/ui"
)

func main() {
	app := ui.NewApp()
	if len(os.Args) > 1 {
		if err := app.Load(os.Args[1]); err != nil {
			log.Fatal(err)
		}
	}
	b, err := backend.CreateBackend(glfwbackend.NewGLFWBackend())
	if err != nil {
		log.Fatal(err)
	}
	// Same color as the ImGui window background, so nothing shows around the root panel.
	b.SetBgColor(imgui.Vec4{X: 0.06, Y: 0.06, Z: 0.06, W: 1})
	b.CreateWindow("mini-EWB", 1500, 950)
	b.Run(app.Frame)
}
